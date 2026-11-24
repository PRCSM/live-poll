package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestSlidingWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := New(rdb)
	now := time.UnixMilli(1_000_000)
	l.now = func() time.Time { return now }
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		ok, _, err := l.Allow(ctx, "k", 3, time.Minute)
		if err != nil || !ok {
			t.Fatalf("hit %d: ok=%v err=%v", i, ok, err)
		}
	}
	ok, retry, err := l.Allow(ctx, "k", 3, time.Minute)
	if err != nil || ok {
		t.Fatalf("4th hit should be rejected: ok=%v err=%v", ok, err)
	}
	if retry != time.Minute {
		t.Errorf("retry = %v, want 1m", retry)
	}

	now = now.Add(time.Minute + time.Millisecond)
	if ok, _, _ := l.Allow(ctx, "k", 3, time.Minute); !ok {
		t.Error("window should have slid open")
	}
}
