package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// rateLimiter is a dependency-free in-memory fixed-window limiter. It is
// coarse by design: per-process, best-effort, enough to stop trivial burst
// flooding (comment spam) without external storage.
type rateLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	counters map[string]*windowCounter
}

type windowCounter struct {
	count   int
	resetAt time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		limit:    limit,
		window:   window,
		counters: make(map[string]*windowCounter),
	}
}

func (r *rateLimiter) allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	wc, ok := r.counters[key]
	if !ok || now.After(wc.resetAt) {
		// Sliding start: each new window resets the counter.
		r.counters[key] = &windowCounter{count: 1, resetAt: now.Add(r.window)}
		// Opportunistic cleanup to keep the map bounded (windows are short).
		if len(r.counters) > 10_000 {
			for k, c := range r.counters {
				if now.After(c.resetAt) {
					delete(r.counters, k)
				}
			}
		}
		return true
	}
	wc.count++
	return wc.count <= r.limit
}

// RateLimitPerMin rejects requests from a single client once they exceed
// limit requests within a minute. The key is the JWT user id when the caller
// is authenticated (tamper-proof, used for writes) and otherwise the direct
// peer IP (trusted-proxy list must be set to the reverse-proxy pool, or the
// IP is spoofable via X-Forwarded-For and the limiter is bypassable).
func RateLimitPerMin(limit int) gin.HandlerFunc {
	if limit <= 0 {
		limit = 30
	}
	rl := newRateLimiter(limit, time.Minute)
	return func(c *gin.Context) {
		key := "ip:" + c.ClientIP()
		if id := UserID(c); id != uuid.Nil {
			key = "u:" + id.String()
		}
		if !rl.allow(key) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			return
		}
		c.Next()
	}
}