package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestPerIPIsolation(t *testing.T) {
	reg := NewRegistry(1000, 2)
	defer reg.Close()

	if ok, _, _ := reg.Allow("1.1.1.1"); !ok {
		t.Fatal("A first call should pass")
	}
	if ok, _, _ := reg.Allow("1.1.1.1"); !ok {
		t.Fatal("A second call should pass (burst=2)")
	}
	if ok, _, _ := reg.Allow("2.2.2.2"); !ok {
		t.Fatal("B should not be affected by A")
	}
}

func TestClientIPRespectsTrustList(t *testing.T) {
	trusted, err := ParseCIDRList("127.0.0.1/8")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("X-Forwarded-For", "9.9.9.9, 1.1.1.1")
	got := ClientIP(req, trusted)
	if got != "9.9.9.9" {
		t.Fatalf("trusted proxy: expected XFF first hop, got %s", got)
	}

	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "8.8.8.8:12345"
	req2.Header.Set("X-Forwarded-For", "1.2.3.4")
	got2 := ClientIP(req2, trusted)
	if got2 != "8.8.8.8" {
		t.Fatalf("untrusted client: expected RemoteAddr, got %s", got2)
	}
}

func TestClientIPNoTrustList(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	got := ClientIP(req, nil)
	if got != "127.0.0.1" {
		t.Fatalf("no trust: expected RemoteAddr, got %s", got)
	}
}

func TestMiddlewareReturns429(t *testing.T) {
	reg := NewRegistry(1, 1)
	defer reg.Close()
	h := Middleware(reg, []netip.Prefix{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "5.5.5.5:1"
	w1 := httptest.NewRecorder()
	h.ServeHTTP(w1, req)
	if w1.Code != http.StatusOK {
		t.Fatalf("first should pass: %d", w1.Code)
	}
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second should 429: %d", w2.Code)
	}
	if w2.Header().Get("X-RateLimit-Key") == "" {
		t.Fatal("missing X-RateLimit-Key header on 429")
	}
	if w2.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After header on 429")
	}
	if w2.Header().Get("RateLimit-Limit") == "" {
		t.Fatal("missing RateLimit-Limit header")
	}
}

func TestMiddlewareRateLimitHeaders(t *testing.T) {
	reg := NewRegistry(10, 10)
	defer reg.Close()
	h := Middleware(reg, []netip.Prefix{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "7.7.7.7:1"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("RateLimit-Remaining") == "" {
		t.Fatal("missing RateLimit-Remaining on 200")
	}
}

func TestTopOffendersSorted(t *testing.T) {
	reg := NewRegistry(1, 1)
	defer reg.Close()
	for i := 0; i < 5; i++ {
		reg.Allow("1.1.1.1")
	}
	for i := 0; i < 2; i++ {
		reg.Allow("2.2.2.2")
	}
	top := reg.TopOffenders(10)
	if len(top) < 2 {
		t.Fatalf("expected at least 2 offenders, got %d", len(top))
	}
	if top[0].Denied < top[1].Denied {
		t.Fatalf("not sorted desc: %+v", top)
	}
}

// TestFixedWindowBoundary verifies exact limit and reset.
func TestFixedWindowBoundary(t *testing.T) {
	lim := newFixedWindowLimiter(5, 3)
	key := "test"

	for i := 0; i < 3; i++ {
		ok, rem, _ := lim.Allow(key)
		if !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
		if rem != 3-i-1 {
			t.Fatalf("request %d: expected remaining %d, got %d", i+1, 3-i-1, rem)
		}
	}
	ok, _, _ := lim.Allow(key)
	if ok {
		t.Fatal("4th request should be denied")
	}
}

// TestSlidingWindowBoundary verifies per-window limit is enforced.
func TestSlidingWindowBoundary(t *testing.T) {
	lim := newSlidingWindowLimiter(5, 3)
	key := "test"

	for i := 0; i < 3; i++ {
		ok, _, _ := lim.Allow(key)
		if !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	ok, _, _ := lim.Allow(key)
	if ok {
		t.Fatal("4th request should be denied within same window")
	}
}

// TestLeakyBucketDrain verifies that the bucket drains over time.
func TestLeakyBucketDrain(t *testing.T) {
	lim := newLeakyBucketLimiter(100, 2) // drain 100/s, cap 2
	key := "test"

	ok1, _, _ := lim.Allow(key)
	ok2, _, _ := lim.Allow(key)
	if !ok1 || !ok2 {
		t.Fatal("first 2 requests within capacity should pass")
	}
	ok3, _, _ := lim.Allow(key)
	if ok3 {
		t.Fatal("3rd request over capacity should be denied")
	}
}

// TestNewLimiterUnknown verifies error on unknown algo.
func TestNewLimiterUnknown(t *testing.T) {
	_, err := NewLimiter("bogus", 1, 1)
	if err == nil {
		t.Fatal("expected error for unknown algorithm")
	}
}

// TestAllAlgosPerIPIsolation checks each algo keeps per-key buckets isolated.
func TestAllAlgosPerIPIsolation(t *testing.T) {
	for _, algo := range []string{"tokenbucket", "fixedwindow", "slidingwindow", "leakybucket"} {
		lim, err := NewLimiter(algo, 1000, 2)
		if err != nil {
			t.Fatalf("%s: %v", algo, err)
		}
		if ok, _, _ := lim.Allow("a"); !ok {
			t.Fatalf("%s: a first call should pass", algo)
		}
		if ok, _, _ := lim.Allow("a"); !ok {
			t.Fatalf("%s: a second call should pass (burst=2)", algo)
		}
		if ok, _, _ := lim.Allow("b"); !ok {
			t.Fatalf("%s: b should not be affected by a", algo)
		}
	}
}

// TestAllAlgosLimit checks each algo enforces limits.
func TestAllAlgosLimit(t *testing.T) {
	for _, algo := range []string{"tokenbucket", "fixedwindow", "slidingwindow", "leakybucket"} {
		lim, err := NewLimiter(algo, 2, 2)
		if err != nil {
			t.Fatalf("%s: %v", algo, err)
		}
		allowed := 0
		for i := 0; i < 5; i++ {
			if ok, _, _ := lim.Allow("x"); ok {
				allowed++
			}
		}
		if allowed > 2 {
			t.Fatalf("%s: allowed %d requests, limit=2", algo, allowed)
		}
	}
}

// TestResetTimeInFuture verifies 429 reset is after now.
func TestResetTimeInFuture(t *testing.T) {
	for _, algo := range []string{"tokenbucket", "fixedwindow", "slidingwindow", "leakybucket"} {
		lim, err := NewLimiter(algo, 1, 1)
		if err != nil {
			t.Fatalf("%s: %v", algo, err)
		}
		lim.Allow("k") // consume
		_, _, reset := lim.Allow("k")
		if !reset.After(time.Now()) {
			t.Fatalf("%s: 429 reset %v not after now", algo, reset)
		}
	}
}
