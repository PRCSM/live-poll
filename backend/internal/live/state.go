// Package live owns the Redis side of a poll: live counts, voter dedupe, the event
// version, viewer presence, the auto-close schedule and the vote queue.
// MongoDB stays the source of truth; everything here can be rebuilt from it.
package live

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	StreamKey  = "votes:stream"
	closingKey = "polls:closing"
	closedTTL  = 7 * 24 * time.Hour
	viewerTTL  = 45 * time.Second
)

func metaKey(id string) string    { return "poll:" + id + ":meta" }
func countsKey(id string) string  { return "poll:" + id + ":counts" }
func votersKey(id string) string  { return "poll:" + id + ":voters" }
func verKey(id string) string     { return "poll:" + id + ":ver" }
func viewersKey(id string) string { return "poll:" + id + ":viewers" }
func lockKey(id string) string    { return "lock:rebuild:" + id }

// Channel is the pub/sub channel carrying a poll's events.
func Channel(id string) string { return "poll:" + id + ":events" }

// ChannelPattern matches every poll channel; each API instance subscribes once.
const ChannelPattern = "poll:*:events"

// PollID extracts the poll id from a channel name.
func PollID(channel string) string {
	return strings.TrimSuffix(strings.TrimPrefix(channel, "poll:"), ":events")
}

var (
	ErrNotLoaded    = errors.New("poll state not loaded")
	ErrPollGone     = errors.New("poll no longer exists")
	ErrClosed       = errors.New("poll is closed")
	ErrAlreadyVoted = errors.New("already voted")
)

// Meta is the slice of a poll that vote validation and the stream need on every request.
type Meta struct {
	Status       string
	ClosesAt     time.Time
	OptionIDs    []string
	MultiChoice  bool
	Visibility   string
	RequireLogin bool
	OwnerID      string
}

func (m Meta) Open(now time.Time) bool { return m.Status == "open" && now.Before(m.ClosesAt) }

func (m Meta) HasOption(id string) bool {
	for _, o := range m.OptionIDs {
		if o == id {
			return true
		}
	}
	return false
}

// Event is what clients receive over the stream.
type Event struct {
	Type     string           `json:"type"`
	Ver      int64            `json:"ver,omitempty"`
	Counts   map[string]int64 `json:"counts,omitempty"`
	Total    int64            `json:"total"`
	Status   string           `json:"status,omitempty"`
	ClosesAt *time.Time       `json:"closesAt,omitempty"`
	Viewers  int64            `json:"viewers,omitempty"`
}

// Source rebuilds Redis state from MongoDB.
type Source interface {
	LoadMeta(ctx context.Context, pollID string) (Meta, error) // ErrPollGone if deleted
	VoteState(ctx context.Context, pollID string) (counts map[string]int64, voterKeys []string, err error)
}

//go:embed vote.lua
var voteLua string

var voteScript = redis.NewScript(voteLua)

type State struct {
	rdb redis.UniversalClient
	src Source
	now func() time.Time
}

func NewState(rdb redis.UniversalClient, src Source) *State {
	return &State{rdb: rdb, src: src, now: time.Now}
}

// Init writes the state for a freshly created poll.
func (s *State) Init(ctx context.Context, id string, m Meta) error {
	_, err := s.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		zero := map[string]any{"_total": 0}
		for _, o := range m.OptionIDs {
			zero[o] = 0
		}
		p.HSet(ctx, countsKey(id), zero)
		p.Set(ctx, verKey(id), s.now().UnixMilli(), 0)
		p.ZAdd(ctx, closingKey, redis.Z{Score: float64(m.ClosesAt.UnixMilli()), Member: id})
		p.HSet(ctx, metaKey(id), metaFields(m))
		return nil
	})
	return err
}

func metaFields(m Meta) map[string]any {
	return map[string]any{
		"status":       m.Status,
		"closesAt":     m.ClosesAt.UnixMilli(),
		"options":      strings.Join(m.OptionIDs, ","),
		"multi":        boolStr(m.MultiChoice),
		"visibility":   m.Visibility,
		"requireLogin": boolStr(m.RequireLogin),
		"owner":        m.OwnerID,
	}
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// Meta returns cached poll metadata, rebuilding it from MongoDB on a miss.
func (s *State) Meta(ctx context.Context, id string) (Meta, error) {
	if err := s.Ensure(ctx, id); err != nil {
		return Meta{}, err
	}
	f, err := s.rdb.HGetAll(ctx, metaKey(id)).Result()
	if err != nil {
		return Meta{}, err
	}
	if len(f) == 0 {
		return Meta{}, ErrNotLoaded
	}
	ms, _ := strconv.ParseInt(f["closesAt"], 10, 64)
	m := Meta{
		Status:       f["status"],
		ClosesAt:     time.UnixMilli(ms),
		MultiChoice:  f["multi"] == "1",
		Visibility:   f["visibility"],
		RequireLogin: f["requireLogin"] == "1",
		OwnerID:      f["owner"],
	}
	if f["options"] != "" {
		m.OptionIDs = strings.Split(f["options"], ",")
	}
	return m, nil
}

// Ensure makes sure the poll's state is in Redis. If it is missing (expired or
// Redis lost data) one caller rebuilds it under a lock while the others wait.
func (s *State) Ensure(ctx context.Context, id string) error {
	for attempt := 0; attempt < 50; attempt++ {
		n, err := s.rdb.Exists(ctx, metaKey(id)).Result()
		if err != nil {
			return err
		}
		if n == 1 {
			return nil
		}
		got, err := s.rdb.SetNX(ctx, lockKey(id), 1, 10*time.Second).Result()
		if err != nil {
			return err
		}
		if got {
			err := s.rebuild(ctx, id)
			s.rdb.Del(context.WithoutCancel(ctx), lockKey(id))
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("rebuild of %s did not finish", id)
}

func (s *State) rebuild(ctx context.Context, id string) error {
	m, err := s.src.LoadMeta(ctx, id)
	if err != nil {
		return err
	}
	counts, voters, err := s.src.VoteState(ctx, id)
	if err != nil {
		return err
	}
	fields := map[string]any{"_total": len(voters)} // one vote per voter, even when multi-choice
	for _, o := range m.OptionIDs {
		fields[o] = counts[o]
	}

	_, err = s.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Del(ctx, countsKey(id), votersKey(id))
		p.HSet(ctx, countsKey(id), fields)
		for i := 0; i < len(voters); i += 1000 {
			end := min(i+1000, len(voters))
			members := make([]any, 0, end-i)
			for _, v := range voters[i:end] {
				members = append(members, v)
			}
			p.SAdd(ctx, votersKey(id), members...)
		}
		// A fresh millisecond-based version is always ahead of any version a client
		// saw before the state was lost, so clients never ignore post-rebuild events.
		p.Set(ctx, verKey(id), s.now().UnixMilli(), 0)
		p.HSet(ctx, metaKey(id), metaFields(m))
		if m.Status == "open" {
			p.ZAdd(ctx, closingKey, redis.Z{Score: float64(m.ClosesAt.UnixMilli()), Member: id})
		} else {
			for _, k := range []string{metaKey(id), countsKey(id), votersKey(id), verKey(id)} {
				p.Expire(ctx, k, closedTTL)
			}
		}
		return nil
	})
	return err
}

// Snapshot returns the current counts and version.
func (s *State) Snapshot(ctx context.Context, id string) (Event, error) {
	if err := s.Ensure(ctx, id); err != nil {
		return Event{}, err
	}
	var counts *redis.MapStringStringCmd
	var ver *redis.StringCmd
	_, err := s.rdb.Pipelined(ctx, func(p redis.Pipeliner) error {
		counts = p.HGetAll(ctx, countsKey(id))
		ver = p.Get(ctx, verKey(id))
		return nil
	})
	if err != nil && !errors.Is(err, redis.Nil) {
		return Event{}, err
	}
	e := Event{Type: "snapshot", Counts: map[string]int64{}}
	e.Ver, _ = ver.Int64()
	for k, v := range counts.Val() {
		n, _ := strconv.ParseInt(v, 10, 64)
		if k == "_total" {
			e.Total = n
		} else {
			e.Counts[k] = n
		}
	}
	return e, nil
}

func (s *State) HasVoted(ctx context.Context, id, voterKey string) (bool, error) {
	return s.rdb.SIsMember(ctx, votersKey(id), voterKey).Result()
}

type Ballot struct {
	PollID    string
	VoterKey  string
	OptionIDs []string
	UserID    string
	IPHash    string
}

// CastVote runs the vote script, rebuilding state once if it was missing.
func (s *State) CastVote(ctx context.Context, b Ballot) (Event, error) {
	for attempt := 0; attempt < 2; attempt++ {
		args := []any{b.VoterKey, s.now().UnixMilli(), Channel(b.PollID), b.PollID, b.UserID, b.IPHash}
		for _, o := range b.OptionIDs {
			args = append(args, o)
		}
		keys := []string{metaKey(b.PollID), countsKey(b.PollID), votersKey(b.PollID), verKey(b.PollID), StreamKey}
		res, err := voteScript.Run(ctx, s.rdb, keys, args...).StringSlice()
		if err != nil {
			return Event{}, err
		}
		switch res[0] {
		case "ok":
			var e Event
			if err := json.Unmarshal([]byte(res[1]), &e); err != nil {
				return Event{}, err
			}
			return e, nil
		case "closed":
			return Event{}, ErrClosed
		case "already_voted":
			return Event{}, ErrAlreadyVoted
		case "not_loaded":
			if err := s.Ensure(ctx, b.PollID); err != nil {
				return Event{}, err
			}
		default:
			return Event{}, fmt.Errorf("vote script returned %q", res[0])
		}
	}
	return Event{}, ErrNotLoaded
}

// SetStatus updates status in Redis, reschedules auto-close and tells every viewer.
func (s *State) SetStatus(ctx context.Context, id, status string, closesAt time.Time) error {
	if err := s.Ensure(ctx, id); err != nil {
		return err
	}
	_, err := s.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.HSet(ctx, metaKey(id), "status", status, "closesAt", closesAt.UnixMilli())
		keys := []string{metaKey(id), countsKey(id), votersKey(id), verKey(id)}
		if status == "open" {
			p.ZAdd(ctx, closingKey, redis.Z{Score: float64(closesAt.UnixMilli()), Member: id})
			for _, k := range keys {
				p.Persist(ctx, k)
			}
		} else {
			p.ZRem(ctx, closingKey, id)
			for _, k := range keys {
				p.Expire(ctx, k, closedTTL)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.Publish(ctx, id, Event{Type: "status", Status: status, ClosesAt: &closesAt})
}

// Remove deletes all live state for a poll and tells viewers it is gone.
func (s *State) Remove(ctx context.Context, id string) error {
	_, err := s.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Del(ctx, metaKey(id), countsKey(id), votersKey(id), verKey(id), viewersKey(id))
		p.ZRem(ctx, closingKey, id)
		return nil
	})
	if err != nil {
		return err
	}
	return s.Publish(ctx, id, Event{Type: "deleted"})
}

func (s *State) Publish(ctx context.Context, id string, e Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return s.rdb.Publish(ctx, Channel(id), b).Err()
}

// Invalidate drops cached metadata so the next access rebuilds it from MongoDB.
// Used when a Mongo write succeeded but the matching Redis write did not.
func (s *State) Invalidate(ctx context.Context, id string) error {
	return s.rdb.Del(ctx, metaKey(id)).Err()
}

// Totals returns vote totals for polls whose state is cached; others are omitted.
func (s *State) Totals(ctx context.Context, ids []string) (map[string]int64, error) {
	cmds := make([]*redis.StringCmd, len(ids))
	_, err := s.rdb.Pipelined(ctx, func(p redis.Pipeliner) error {
		for i, id := range ids {
			cmds[i] = p.HGet(ctx, countsKey(id), "_total")
		}
		return nil
	})
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	out := make(map[string]int64, len(ids))
	for i, c := range cmds {
		if n, err := c.Int64(); err == nil {
			out[ids[i]] = n
		}
	}
	return out, nil
}

// Schedule (re)queues a poll for auto-close, e.g. after a failed close attempt.
func (s *State) Schedule(ctx context.Context, id string, at time.Time) error {
	return s.rdb.ZAdd(ctx, closingKey, redis.Z{Score: float64(at.UnixMilli()), Member: id}).Err()
}

// DueForClose returns polls whose closing time has passed.
func (s *State) DueForClose(ctx context.Context) ([]string, error) {
	return s.rdb.ZRangeByScore(ctx, closingKey, &redis.ZRangeBy{
		Min: "-inf", Max: strconv.FormatInt(s.now().UnixMilli(), 10), Count: 100,
	}).Result()
}

// ClaimClose removes id from the schedule; only the instance that removed it closes it.
func (s *State) ClaimClose(ctx context.Context, id string) (bool, error) {
	n, err := s.rdb.ZRem(ctx, closingKey, id).Result()
	return n == 1, err
}

// TouchViewer records a live viewer; members not refreshed within viewerTTL drop out.
func (s *State) TouchViewer(ctx context.Context, id, conn string) error {
	return s.rdb.ZAdd(ctx, viewersKey(id), redis.Z{Score: float64(s.now().UnixMilli()), Member: conn}).Err()
}

func (s *State) LeaveViewer(ctx context.Context, id, conn string) error {
	return s.rdb.ZRem(ctx, viewersKey(id), conn).Err()
}

func (s *State) Viewers(ctx context.Context, id string) (int64, error) {
	cutoff := strconv.FormatInt(s.now().Add(-viewerTTL).UnixMilli(), 10)
	var card *redis.IntCmd
	_, err := s.rdb.Pipelined(ctx, func(p redis.Pipeliner) error {
		p.ZRemRangeByScore(ctx, viewersKey(id), "-inf", "("+cutoff)
		card = p.ZCard(ctx, viewersKey(id))
		return nil
	})
	if err != nil {
		return 0, err
	}
	return card.Val(), nil
}
