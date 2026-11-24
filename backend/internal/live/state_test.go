package live

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type fakeSource struct {
	meta   Meta
	counts map[string]int64
	voters []string
	gone   bool
}

func (f *fakeSource) LoadMeta(context.Context, string) (Meta, error) {
	if f.gone {
		return Meta{}, ErrPollGone
	}
	return f.meta, nil
}

func (f *fakeSource) VoteState(context.Context, string) (map[string]int64, []string, error) {
	return f.counts, f.voters, nil
}

func setup(t *testing.T) (*State, *miniredis.Miniredis, *fakeSource) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	src := &fakeSource{meta: Meta{
		Status: "open", ClosesAt: time.Now().Add(time.Hour), OptionIDs: []string{"a", "b"}, Visibility: "always",
	}}
	return NewState(rdb, src), mr, src
}

func TestCastVoteCountsOnceUnderConcurrency(t *testing.T) {
	s, _, src := setup(t)
	ctx := context.Background()
	if err := s.Init(ctx, "p1", src.meta); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	results := map[error]int{}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.CastVote(ctx, Ballot{PollID: "p1", VoterKey: "same-voter", OptionIDs: []string{"a"}})
			mu.Lock()
			results[err]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if results[nil] != 1 || results[ErrAlreadyVoted] != 49 {
		t.Fatalf("results = %v", results)
	}

	snap, err := s.Snapshot(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Counts["a"] != 1 || snap.Counts["b"] != 0 || snap.Total != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestCastVoteEventAndVersion(t *testing.T) {
	s, _, src := setup(t)
	ctx := context.Background()
	_ = s.Init(ctx, "p1", src.meta)

	e1, err := s.CastVote(ctx, Ballot{PollID: "p1", VoterKey: "v1", OptionIDs: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	e2, err := s.CastVote(ctx, Ballot{PollID: "p1", VoterKey: "v2", OptionIDs: []string{"b"}})
	if err != nil {
		t.Fatal(err)
	}
	if e2.Ver != e1.Ver+1 {
		t.Errorf("versions not sequential: %d then %d", e1.Ver, e2.Ver)
	}
	if e2.Type != "counts" || e2.Counts["a"] != 1 || e2.Counts["b"] != 1 || e2.Total != 2 {
		t.Errorf("event = %+v", e2)
	}
}

func TestCastVoteRejectsClosed(t *testing.T) {
	s, _, src := setup(t)
	ctx := context.Background()

	src.meta.ClosesAt = time.Now().Add(-time.Second)
	_ = s.Init(ctx, "expired", src.meta)
	if _, err := s.CastVote(ctx, Ballot{PollID: "expired", VoterKey: "v", OptionIDs: []string{"a"}}); !errors.Is(err, ErrClosed) {
		t.Errorf("expired poll: err = %v", err)
	}

	src.meta.ClosesAt = time.Now().Add(time.Hour)
	src.meta.Status = "closed"
	_ = s.Init(ctx, "closed", src.meta)
	if _, err := s.CastVote(ctx, Ballot{PollID: "closed", VoterKey: "v", OptionIDs: []string{"a"}}); !errors.Is(err, ErrClosed) {
		t.Errorf("closed poll: err = %v", err)
	}
}

func TestRebuildAfterRedisLoss(t *testing.T) {
	s, mr, src := setup(t)
	ctx := context.Background()
	src.counts = map[string]int64{"a": 2, "b": 1}
	src.voters = []string{"v1", "v2", "v3"}

	mr.FlushAll()

	if _, err := s.CastVote(ctx, Ballot{PollID: "p1", VoterKey: "v1", OptionIDs: []string{"b"}}); !errors.Is(err, ErrAlreadyVoted) {
		t.Fatalf("rebuilt voter set should block v1: err = %v", err)
	}
	e, err := s.CastVote(ctx, Ballot{PollID: "p1", VoterKey: "v4", OptionIDs: []string{"b"}})
	if err != nil {
		t.Fatal(err)
	}
	if e.Counts["a"] != 2 || e.Counts["b"] != 2 || e.Total != 4 {
		t.Errorf("counts after rebuild = %+v", e)
	}
}

func TestRebuildOfDeletedPoll(t *testing.T) {
	s, _, src := setup(t)
	src.gone = true
	if _, err := s.Meta(context.Background(), "gone"); !errors.Is(err, ErrPollGone) {
		t.Errorf("err = %v", err)
	}
}

func TestCloseSchedule(t *testing.T) {
	s, _, src := setup(t)
	ctx := context.Background()
	src.meta.ClosesAt = time.Now().Add(-time.Second)
	for i := 0; i < 3; i++ {
		_ = s.Init(ctx, fmt.Sprint("p", i), src.meta)
	}
	due, err := s.DueForClose(ctx)
	if err != nil || len(due) != 3 {
		t.Fatalf("due = %v err = %v", due, err)
	}
	first, _ := s.ClaimClose(ctx, "p0")
	second, _ := s.ClaimClose(ctx, "p0")
	if !first || second {
		t.Errorf("claim should succeed once: %v %v", first, second)
	}
}
