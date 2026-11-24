package vote

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/PRCSM/live-poll/backend/internal/live"
)

const (
	group       = "persist"
	claimIdle   = time.Minute
	readBlock   = 5 * time.Second
	maxBackoff  = 30 * time.Second
	readBatches = 100
)

// Worker moves votes from the Redis stream into MongoDB. Entries are acked only
// after Mongo accepts them; anything left pending by a crash is reclaimed.
type Worker struct {
	rdb      redis.UniversalClient
	votes    *Store
	consumer string
}

func NewWorker(rdb redis.UniversalClient, votes *Store) *Worker {
	host, _ := os.Hostname()
	return &Worker{rdb: rdb, votes: votes, consumer: fmt.Sprintf("%s-%d", host, os.Getpid())}
}

func (w *Worker) ensureGroup(ctx context.Context) {
	err := w.rdb.XGroupCreateMkStream(ctx, live.StreamKey, group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		slog.Error("vote worker: create group", "err", err)
	}
}

func (w *Worker) Run(ctx context.Context) {
	w.ensureGroup(ctx)
	var err error
	backoff := time.Second
	lastClaim := time.Time{}
	for ctx.Err() == nil {
		var msgs []redis.XMessage
		if time.Since(lastClaim) > claimIdle/2 {
			lastClaim = time.Now()
			msgs, _, err = w.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
				Stream: live.StreamKey, Group: group, Consumer: w.consumer,
				MinIdle: claimIdle, Start: "0-0", Count: readBatches,
			}).Result()
		} else {
			var streams []redis.XStream
			streams, err = w.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group: group, Consumer: w.consumer, Streams: []string{live.StreamKey, ">"},
				Count: readBatches, Block: readBlock,
			}).Result()
			if len(streams) > 0 {
				msgs = streams[0].Messages
			}
		}
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err == nil {
			err = w.persist(ctx, msgs)
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// Redis lost the stream (restart without persistence): recreate the group.
			if strings.Contains(err.Error(), "NOGROUP") {
				w.ensureGroup(ctx)
			}
			slog.Error("vote worker", "err", err, "retry_in", backoff)
			sleep(ctx, backoff)
			backoff = min(backoff*2, maxBackoff)
			continue
		}
		backoff = time.Second
	}
}

func (w *Worker) persist(ctx context.Context, msgs []redis.XMessage) error {
	for _, m := range msgs {
		v, err := parseEntry(m)
		if err != nil {
			// A malformed entry would block the queue forever; log it and move on.
			slog.Error("vote worker: dropping malformed entry", "id", m.ID, "err", err)
		} else if err := w.votes.Insert(ctx, v); err != nil {
			return fmt.Errorf("insert vote %s: %w", m.ID, err)
		}
		if err := w.rdb.XAck(ctx, live.StreamKey, group, m.ID).Err(); err != nil {
			return fmt.Errorf("ack %s: %w", m.ID, err)
		}
	}
	return nil
}

func parseEntry(m redis.XMessage) (*Vote, error) {
	str := func(k string) string { s, _ := m.Values[k].(string); return s }
	pollID, err := bson.ObjectIDFromHex(str("pollId"))
	if err != nil {
		return nil, fmt.Errorf("pollId: %w", err)
	}
	at, err := strconv.ParseInt(str("at"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("at: %w", err)
	}
	v := &Vote{
		ID:        bson.NewObjectID(),
		PollID:    pollID,
		OptionIDs: strings.Split(str("optionIds"), ","),
		VoterKey:  str("voterKey"),
		IPHash:    str("ipHash"),
		CreatedAt: time.UnixMilli(at).UTC(),
	}
	if v.VoterKey == "" || str("optionIds") == "" {
		return nil, errors.New("missing voterKey or optionIds")
	}
	if u := str("userId"); u != "" {
		uid, err := bson.ObjectIDFromHex(u)
		if err != nil {
			return nil, fmt.Errorf("userId: %w", err)
		}
		v.UserID = &uid
	}
	return v, nil
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
