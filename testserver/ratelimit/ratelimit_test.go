package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestPerIPIsolation(t *testing.T) {
	reg := NewRegistry(1000, 2) // rate 1000/s, burst 2 — plenty for 2 hits per IP
	defer reg.Close()

	// IP A burns its bucket
	if !reg.For("1.1.1.1").Allow() {
		t.Fatal("A first call should pass")
	}
	if !reg.For("1.1.1.1").Allow() {
		t.Fatal("A second call should pass (burst=2)")
	}
	// IP B should still have full bucket
	if !reg.For("2.2.2.2").Allow() {
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
	h := Middleware(reg, []netip.Prefix{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
}

func TestTopOffendersSorted(t *testing.T) {
	reg := NewRegistry(1, 1)
	defer reg.Close()
	for i := 0; i < 5; i++ {
		reg.For("1.1.1.1").Allow()
	}
	for i := 0; i < 2; i++ {
		reg.For("2.2.2.2").Allow()
	}
	top := reg.TopOffenders(10)
	if len(top) < 2 {
		t.Fatalf("expected at least 2 offenders, got %d", len(top))
	}
	if top[0].Denied < top[1].Denied {
		t.Fatalf("not sorted desc: %+v", top)
	}
}
