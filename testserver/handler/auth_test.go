package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthSuccess(t *testing.T) {
	h := NewAuthHandler("admin", "changeme", nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/auth", bytes.NewReader([]byte(`{"user":"admin","pass":"changeme"}`)))
	r.RemoteAddr = "1.1.1.1:1234"
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthLockoutAfter5Fails(t *testing.T) {
	h := NewAuthHandler("admin", "changeme", nil)
	r := func() *http.Request {
		req := httptest.NewRequest("POST", "/api/auth", bytes.NewReader([]byte(`{"user":"admin","pass":"wrong"}`)))
		req.RemoteAddr = "2.2.2.2:1234"
		return req
	}
	for i := 0; i < 4; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r())
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("fail %d should be 401, got %d", i, w.Code)
		}
	}
	// 5th failure triggers lockout
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r())
	if w.Code != http.StatusLocked {
		t.Fatalf("5th fail should be 423, got %d", w.Code)
	}
	// Subsequent attempts also locked
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, r())
	if w2.Code != http.StatusLocked {
		t.Fatalf("locked follow-up should be 423, got %d", w2.Code)
	}
}

func TestAuthMethodNotAllowed(t *testing.T) {
	h := NewAuthHandler("admin", "changeme", nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/auth", nil)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
