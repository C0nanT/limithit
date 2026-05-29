package ratelimit

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// bucket is an internal token-bucket for a single key.
type bucket struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64
	rate       float64
	lastRefill time.Time
	lastSeen   atomic.Int64
	denied     atomic.Int64
}

func newBucket(rps, burst float64) *bucket {
	b := &bucket{
		tokens:     burst,
		capacity:   burst,
		rate:       rps,
		lastRefill: time.Now(),
	}
	b.lastSeen.Store(time.Now().UnixNano())
	return b
}

func (b *bucket) allow() (ok bool, remaining int, reset time.Time) {
	b.lastSeen.Store(time.Now().UnixNano())
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.lastRefill = now
	b.tokens += elapsed * b.rate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	if b.tokens >= 1 {
		b.tokens--
		remaining = int(math.Max(0, math.Floor(b.tokens)))
		return true, remaining, now
	}
	b.denied.Add(1)
	secs := (1 - b.tokens) / b.rate
	return false, 0, now.Add(time.Duration(secs * float64(time.Second)))
}

func (b *bucket) Denied() int64   { return b.denied.Load() }
func (b *bucket) LastSeen() int64 { return b.lastSeen.Load() }

// Registry is the token-bucket Limiter implementation.
type Registry struct {
	rate, burst float64
	mu          sync.Mutex
	limiters    map[string]*bucket
	stopGC      chan struct{}
	wg          sync.WaitGroup
}

func NewRegistry(rps, burst float64) *Registry {
	r := &Registry{
		rate:     rps,
		burst:    burst,
		limiters: make(map[string]*bucket),
		stopGC:   make(chan struct{}),
	}
	r.wg.Add(1)
	go r.gcLoop()
	return r
}

func (r *Registry) Capacity() int { return int(r.burst) }

func (r *Registry) Allow(key string) (ok bool, remaining int, reset time.Time) {
	r.mu.Lock()
	b, exists := r.limiters[key]
	if !exists {
		b = newBucket(r.rate, r.burst)
		r.limiters[key] = b
	}
	r.mu.Unlock()
	return b.allow()
}

func (r *Registry) Close() {
	close(r.stopGC)
	r.wg.Wait()
}

func (r *Registry) gcLoop() {
	defer r.wg.Done()
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-r.stopGC:
			return
		case <-t.C:
			r.sweep(10 * time.Minute)
		}
	}
}

func (r *Registry) sweep(maxIdle time.Duration) {
	cutoff := time.Now().Add(-maxIdle).UnixNano()
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, b := range r.limiters {
		if b.LastSeen() < cutoff {
			delete(r.limiters, k)
		}
	}
}

// TopOffenders returns up to n keys with the highest deny counts.
func (r *Registry) TopOffenders(n int) []OffenderStat {
	r.mu.Lock()
	stats := make([]OffenderStat, 0, len(r.limiters))
	for k, b := range r.limiters {
		d := b.Denied()
		if d == 0 {
			continue
		}
		stats = append(stats, OffenderStat{Key: k, Denied: d})
	}
	r.mu.Unlock()
	for i := 0; i < len(stats); i++ {
		for j := i + 1; j < len(stats); j++ {
			if stats[j].Denied > stats[i].Denied {
				stats[i], stats[j] = stats[j], stats[i]
			}
		}
	}
	if len(stats) > n {
		stats = stats[:n]
	}
	return stats
}

type OffenderStat struct {
	Key    string `json:"key"`
	Denied int64  `json:"denied"`
}

// newTokenBucketLimiter is the factory used by the algo registry.
func newTokenBucketLimiter(rate, burst float64) Limiter {
	return NewRegistry(rate, burst)
}

// ClientIP returns the effective client IP for a request. If RemoteAddr matches
// a trusted CIDR, the first parseable hop from X-Forwarded-For (or X-Real-IP)
// is returned; otherwise RemoteAddr is used.
func ClientIP(r *http.Request, trustedCIDRs []netip.Prefix) string {
	remote := remoteHost(r.RemoteAddr)
	if !ipInPrefixes(remote, trustedCIDRs) {
		return remote
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		first := strings.TrimSpace(strings.SplitN(v, ",", 2)[0])
		if first != "" {
			if _, err := netip.ParseAddr(first); err == nil {
				return first
			}
		}
	}
	if v := r.Header.Get("X-Real-IP"); v != "" {
		v = strings.TrimSpace(v)
		if _, err := netip.ParseAddr(v); err == nil {
			return v
		}
	}
	return remote
}

func remoteHost(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}

func ipInPrefixes(ip string, prefixes []netip.Prefix) bool {
	if len(prefixes) == 0 {
		return false
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// ParseCIDRList parses a comma-separated list of CIDRs into prefixes.
func ParseCIDRList(s string) ([]netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]netip.Prefix, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		pref, err := netip.ParsePrefix(p)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", p, err)
		}
		out = append(out, pref)
	}
	return out, nil
}

// Middleware wraps next with per-IP rate limiting and writes RateLimit-* headers.
func Middleware(lim Limiter, trustedCIDRs []netip.Prefix, next http.Handler) http.Handler {
	if lim == nil {
		return next
	}
	cap := lim.Capacity()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ClientIP(r, trustedCIDRs)
		ok, remaining, reset := lim.Allow(ip)
		resetUnix := fmt.Sprintf("%d", reset.Unix())
		if !ok {
			secs := int(time.Until(reset).Seconds())
			if secs < 1 {
				secs = 1
			}
			w.Header().Set("RateLimit-Limit", fmt.Sprintf("%d", cap))
			w.Header().Set("RateLimit-Remaining", "0")
			w.Header().Set("RateLimit-Reset", resetUnix)
			w.Header().Set("Retry-After", fmt.Sprintf("%d", secs))
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-RateLimit-Key", ip)
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded"}`))
			return
		}
		w.Header().Set("RateLimit-Limit", fmt.Sprintf("%d", cap))
		w.Header().Set("RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		w.Header().Set("RateLimit-Reset", resetUnix)
		next.ServeHTTP(w, r)
	})
}
