package middleware

import (
	"net/http"
	"sync"
	"time"
)

type bucket struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

func (b *bucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens = minFloat(b.maxTokens, b.tokens+elapsed*b.refillRate)
	b.lastRefill = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	max     float64
	rate    float64
}

func NewRateLimiter(maxTokens, refillRatePerSec float64) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*bucket),
		max:     maxTokens,
		rate:    refillRatePerSec,
	}
}

func (rl *RateLimiter) getBucket(key string) *bucket {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: rl.max, maxTokens: rl.max, refillRate: rl.rate, lastRefill: time.Now()}
		rl.buckets[key] = b
	}
	return b
}

// Middleware applies rate limiting keyed by X-Tenant-ID header.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.Header.Get("X-Tenant-ID")
		if tenantID == "" {
			tenantID = "default"
		}
		if !rl.getBucket(tenantID).allow() {
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
