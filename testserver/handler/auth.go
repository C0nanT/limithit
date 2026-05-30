package handler

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/conantorreswf/limithit/testserver/ratelimit"
)

type AuthRequest struct {
	User string `json:"user"`
	Pass string `json:"pass"`
}

type authAttempt struct {
	count       int
	firstFailAt time.Time
	lockedUntil time.Time
}

type AuthHandler struct {
	mu       sync.Mutex
	attempts map[string]*authAttempt

	username       string
	password       string
	maxFails       int
	failWindow     time.Duration
	lockoutWindow  time.Duration
	trustedProxies []netip.Prefix
	DisableLockout bool
}

func NewAuthHandler(user, pass string, trusted []netip.Prefix) *AuthHandler {
	return &AuthHandler{
		attempts:       make(map[string]*authAttempt),
		username:       user,
		password:       pass,
		maxFails:       5,
		failWindow:     time.Minute,
		lockoutWindow:  5 * time.Minute,
		trustedProxies: trusted,
	}
}

func (h *AuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	ip := ratelimit.ClientIP(r, h.trustedProxies)

	if !h.DisableLockout {
		if locked, until := h.locked(ip); locked {
			w.Header().Set("Retry-After", retryAfterSeconds(until))
			writeJSON(w, http.StatusLocked, map[string]string{"error": "account locked", "until": until.UTC().Format(time.RFC3339)})
			return
		}
	}

	var req AuthRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	userOK := subtle.ConstantTimeCompare([]byte(req.User), []byte(h.username)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(req.Pass), []byte(h.password)) == 1
	if userOK && passOK {
		h.reset(ip)
		writeJSON(w, http.StatusOK, map[string]string{"token": "session-ok"})
		return
	}

	if h.recordFail(ip) {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "too many attempts, locked"})
		return
	}
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
}

func (h *AuthHandler) locked(ip string) (bool, time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	a, ok := h.attempts[ip]
	if !ok {
		return false, time.Time{}
	}
	if a.lockedUntil.After(time.Now()) {
		return true, a.lockedUntil
	}
	return false, time.Time{}
}

// recordFail increments fail count; returns true if this attempt triggered lockout.
func (h *AuthHandler) recordFail(ip string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	a, ok := h.attempts[ip]
	if !ok || now.Sub(a.firstFailAt) > h.failWindow {
		h.attempts[ip] = &authAttempt{count: 1, firstFailAt: now}
		return false
	}
	a.count++
	if a.count >= h.maxFails {
		a.lockedUntil = now.Add(h.lockoutWindow)
		return true
	}
	return false
}

func (h *AuthHandler) reset(ip string) {
	h.mu.Lock()
	delete(h.attempts, ip)
	h.mu.Unlock()
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc, _ := json.Marshal(body)
	_, _ = w.Write(enc)
}

func retryAfterSeconds(until time.Time) string {
	d := time.Until(until).Round(time.Second)
	if d < time.Second {
		d = time.Second
	}
	return durationSeconds(d)
}

func durationSeconds(d time.Duration) string {
	s := int(d / time.Second)
	if s < 1 {
		s = 1
	}
	return itoa(s)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
