package ratelimit

import (
	"math"
	"sync"
	"time"
)

type fixedWindowEntry struct {
	count     int
	windowEnd time.Time
}

type fixedWindowLimiter struct {
	mu      sync.Mutex
	entries map[string]*fixedWindowEntry
	limit   int
	window  time.Duration
}

func newFixedWindowLimiter(_ float64, burst float64) Limiter {
	limit := int(math.Max(1, burst))
	return &fixedWindowLimiter{
		entries: make(map[string]*fixedWindowEntry),
		limit:   limit,
		window:  time.Second,
	}
}

func (l *fixedWindowLimiter) Capacity() int { return l.limit }

func (l *fixedWindowLimiter) Allow(key string) (ok bool, remaining int, reset time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	e, exists := l.entries[key]
	if !exists || now.After(e.windowEnd) {
		e = &fixedWindowEntry{windowEnd: now.Add(l.window)}
		l.entries[key] = e
	}
	reset = e.windowEnd
	if e.count < l.limit {
		e.count++
		return true, l.limit - e.count, reset
	}
	return false, 0, reset
}
