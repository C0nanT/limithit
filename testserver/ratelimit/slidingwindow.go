package ratelimit

import (
	"math"
	"sync"
	"time"
)

type slidingWindowEntry struct {
	prevCount   int
	curCount    int
	windowStart time.Time
}

type slidingWindowLimiter struct {
	mu      sync.Mutex
	entries map[string]*slidingWindowEntry
	limit   int
	window  time.Duration
}

func newSlidingWindowLimiter(_ float64, burst float64) Limiter {
	limit := int(math.Max(1, burst))
	return &slidingWindowLimiter{
		entries: make(map[string]*slidingWindowEntry),
		limit:   limit,
		window:  time.Second,
	}
}

func (l *slidingWindowLimiter) Capacity() int { return l.limit }

func (l *slidingWindowLimiter) Allow(key string) (ok bool, remaining int, reset time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()

	e, exists := l.entries[key]
	if !exists {
		e = &slidingWindowEntry{windowStart: now.Truncate(l.window)}
		l.entries[key] = e
	}

	curWindowStart := now.Truncate(l.window)
	switch {
	case curWindowStart.After(e.windowStart.Add(l.window)):
		e.prevCount = 0
		e.curCount = 0
		e.windowStart = curWindowStart
	case curWindowStart.After(e.windowStart):
		e.prevCount = e.curCount
		e.curCount = 0
		e.windowStart = curWindowStart
	}

	elapsed := now.Sub(e.windowStart).Seconds()
	weight := 1 - elapsed/l.window.Seconds()
	estimate := float64(e.prevCount)*weight + float64(e.curCount)

	reset = e.windowStart.Add(l.window)
	if estimate < float64(l.limit) {
		e.curCount++
		r := l.limit - int(math.Ceil(estimate)) - 1
		if r < 0 {
			r = 0
		}
		return true, r, reset
	}
	return false, 0, reset
}
