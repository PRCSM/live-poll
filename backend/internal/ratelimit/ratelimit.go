// Package ratelimit implements a sliding-window limiter in Redis so limits hold
// across every API instance, not per process.
package ratelimit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/PRCSM/live-poll/backend/internal/apperr"
	"github.com/PRCSM/live-poll/backend/internal/httpx"
)

var slidingWindow = redis.NewScript(`
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
redis.call('ZREMRANGEBYSCORE', KEYS[1], 0, now - window)
if redis.call('ZCARD', KEYS[1]) >= limit then
  local oldest = redis.call('ZRANGE', KEYS[1], 0, 0, 'WITHSCORES')
  return {0, tonumber(oldest[2]) + window - now}
end
redis.call('ZADD', KEYS[1], now, ARGV[4])
redis.call('PEXPIRE', KEYS[1], window)
return {1, 0}
`)

type Limiter struct {
	rdb redis.Scripter
	now func() time.Time
}

func New(rdb redis.Scripter) *Limiter {
	return &Limiter{rdb: rdb, now: time.Now}
}

// Allow records a hit for key and reports whether it fits within limit per window.
func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	member := make([]byte, 8)
	_, _ = rand.Read(member)
	res, err := slidingWindow.Run(ctx, l.rdb, []string{"rl:" + key},
		l.now().UnixMilli(), window.Milliseconds(), limit, hex.EncodeToString(member)).Int64Slice()
	if err != nil {
		return false, 0, fmt.Errorf("rate limit %s: %w", key, err)
	}
	return res[0] == 1, time.Duration(res[1]) * time.Millisecond, nil
}

// Middleware limits by the key returned from keyFn. An empty key skips the check.
func (l *Limiter) Middleware(scope string, limit int, window time.Duration, keyFn func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		k := keyFn(c)
		if k == "" {
			c.Next()
			return
		}
		if err := l.Check(c, scope+":"+k, limit, window); err != nil {
			httpx.Fail(c, err)
			return
		}
		c.Next()
	}
}

// Check is Allow for handlers: it sets Retry-After and returns an apperr on rejection.
// Redis failures are returned as errors (fail closed), never treated as allowed.
func (l *Limiter) Check(c *gin.Context, key string, limit int, window time.Duration) error {
	ok, retry, err := l.Allow(c.Request.Context(), key, limit, window)
	if err != nil {
		return apperr.ErrUnavailable
	}
	if !ok {
		c.Header("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
		return apperr.ErrRateLimited
	}
	return nil
}

func ByIP(c *gin.Context) string { return c.ClientIP() }
