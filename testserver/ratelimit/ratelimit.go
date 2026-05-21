package ratelimit

import (
	"net/http"
	"sync"
	"time"
)

type Limiter struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64
	rate       float64
	lastRefill time.Time
}

func New(rps, burst float64) *Limiter {
	return &Limiter{
		tokens:     burst,
		capacity:   burst,
		rate:       rps,
		lastRefill: time.Now(),
	}
}

func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(l.lastRefill).Seconds()
	l.lastRefill = now

	l.tokens += elapsed * l.rate
	if l.tokens > l.capacity {
		l.tokens = l.capacity
	}

	if l.tokens >= 1 {
		l.tokens--
		return true
	}
	return false
}

func Middleware(limiter *Limiter, next http.Handler) http.Handler {
	if limiter == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
