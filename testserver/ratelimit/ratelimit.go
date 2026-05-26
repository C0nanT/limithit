package ratelimit

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Limiter — token bucket for a single key (IP).
type Limiter struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64
	rate       float64
	lastRefill time.Time
	lastSeen   atomic.Int64 // unix nanos, atomic for GC sweep without holding mu
	denied     atomic.Int64
}

func New(rps, burst float64) *Limiter {
	l := &Limiter{
		tokens:     burst,
		capacity:   burst,
		rate:       rps,
		lastRefill: time.Now(),
	}
	l.lastSeen.Store(time.Now().UnixNano())
	return l
}

func (l *Limiter) Allow() bool {
	l.lastSeen.Store(time.Now().UnixNano())
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
	l.denied.Add(1)
	return false
}

func (l *Limiter) Denied() int64  { return l.denied.Load() }
func (l *Limiter) LastSeen() int64 { return l.lastSeen.Load() }

// Registry keeps a per-key Limiter. Keys are typically client IPs.
type Registry struct {
	rate, burst float64
	mu          sync.Mutex
	limiters    map[string]*Limiter

	stopGC chan struct{}
	wg     sync.WaitGroup
}

func NewRegistry(rps, burst float64) *Registry {
	r := &Registry{
		rate:     rps,
		burst:    burst,
		limiters: make(map[string]*Limiter),
		stopGC:   make(chan struct{}),
	}
	r.wg.Add(1)
	go r.gcLoop()
	return r
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
	for k, l := range r.limiters {
		if l.LastSeen() < cutoff {
			delete(r.limiters, k)
		}
	}
}

func (r *Registry) For(key string) *Limiter {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.limiters[key]
	if !ok {
		l = New(r.rate, r.burst)
		r.limiters[key] = l
	}
	return l
}

// TopOffenders returns up to n keys with the highest deny counts.
func (r *Registry) TopOffenders(n int) []OffenderStat {
	r.mu.Lock()
	stats := make([]OffenderStat, 0, len(r.limiters))
	for k, l := range r.limiters {
		d := l.Denied()
		if d == 0 {
			continue
		}
		stats = append(stats, OffenderStat{Key: k, Denied: d})
	}
	r.mu.Unlock()
	// partial sort
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

// ClientIP returns the client IP for a request. If r.RemoteAddr matches one of
// the trusted CIDR ranges, the first parseable IP from X-Forwarded-For (or
// X-Real-IP) is returned. Otherwise r.RemoteAddr is returned.
//
// trustedCIDRs may be empty — in that case all XFF headers are ignored,
// neutralising spoofing from untrusted clients.
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

// Middleware wraps next with per-IP rate limiting. trustedCIDRs designates
// proxy ranges whose XFF headers will be honoured.
func Middleware(reg *Registry, trustedCIDRs []netip.Prefix, next http.Handler) http.Handler {
	if reg == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ClientIP(r, trustedCIDRs)
		if !reg.For(ip).Allow() {
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-RateLimit-Key", ip)
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
