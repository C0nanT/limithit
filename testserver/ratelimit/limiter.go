package ratelimit

import (
	"fmt"
	"time"
)

// Limiter is the common interface for all rate-limiting algorithms.
type Limiter interface {
	Allow(key string) (ok bool, remaining int, reset time.Time)
	Capacity() int
}

var algos = map[string]func(rate, burst float64) Limiter{
	"tokenbucket":   newTokenBucketLimiter,
	"fixedwindow":   newFixedWindowLimiter,
	"slidingwindow": newSlidingWindowLimiter,
	"leakybucket":   newLeakyBucketLimiter,
}

// NewLimiter constructs a Limiter for the named algorithm.
// Valid names: tokenbucket, fixedwindow, slidingwindow, leakybucket.
func NewLimiter(algo string, rate, burst float64) (Limiter, error) {
	fn, ok := algos[algo]
	if !ok {
		return nil, fmt.Errorf("unknown algorithm %q; valid: tokenbucket, fixedwindow, slidingwindow, leakybucket", algo)
	}
	return fn(rate, burst), nil
}
