package ratelimit

import (
	"math"
	"sync"
	"time"
)

type leakyEntry struct {
	count    int     // pending requests (integer)
	frac     float64 // fractional drain accumulator
	lastTick time.Time
}

type leakyBucketLimiter struct {
	mu       sync.Mutex
	entries  map[string]*leakyEntry
	rate     float64
	capacity int
}

func newLeakyBucketLimiter(rate, burst float64) Limiter {
	cap := int(math.Max(1, burst))
	return &leakyBucketLimiter{
		entries:  make(map[string]*leakyEntry),
		rate:     rate,
		capacity: cap,
	}
}

func (l *leakyBucketLimiter) Capacity() int { return l.capacity }

func (l *leakyBucketLimiter) Allow(key string) (ok bool, remaining int, reset time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()

	e, exists := l.entries[key]
	if !exists {
		e = &leakyEntry{lastTick: now}
		l.entries[key] = e
	}

	// Drain: accumulate fractional units, apply whole drains.
	elapsed := now.Sub(e.lastTick).Seconds()
	drain := elapsed*l.rate + e.frac
	full := int(math.Floor(drain))
	e.frac = drain - float64(full)
	e.count -= full
	if e.count < 0 {
		e.count = 0
	}
	e.lastTick = now

	if e.count < l.capacity {
		e.count++
		return true, l.capacity - e.count, now
	}
	// time until one slot drains
	drainSecs := (float64(e.count-l.capacity) + 1 - e.frac) / l.rate
	return false, 0, now.Add(time.Duration(drainSecs * float64(time.Second)))
}
