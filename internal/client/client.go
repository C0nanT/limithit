package client

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/conantorreswf/limithit/internal/version"
)

type Config struct {
	URL               string
	Method            string
	Body              string
	Headers           http.Header
	Timeout           time.Duration
	DisableKeepAlives bool
	UserAgent         string // overrides "limithit/<version>" when non-empty
}

type Result struct {
	Status   int
	Err      error
	Timeout  bool
	Duration time.Duration
}

// uaTransport injects a User-Agent header if the request has none set.
type uaTransport struct {
	next http.RoundTripper
	ua   string
}

func (t *uaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", t.ua)
	}
	return t.next.RoundTrip(req)
}

// New returns an *http.Client tuned for the given concurrency level.
// All requests get a default User-Agent of "limithit/<version>" unless the
// caller sets User-Agent explicitly (e.g. via --header or cfg.UserAgent).
func New(cfg Config, concurrency int) *http.Client {
	base, ok := http.DefaultTransport.(*http.Transport)
	var tr *http.Transport
	if ok {
		tr = base.Clone()
	} else {
		tr = &http.Transport{}
	}
	if concurrency < 1 {
		concurrency = 1
	}
	tr.MaxIdleConns = concurrency * 2
	tr.MaxIdleConnsPerHost = concurrency * 2
	tr.MaxConnsPerHost = concurrency * 2
	tr.DisableKeepAlives = cfg.DisableKeepAlives

	ua := cfg.UserAgent
	if ua == "" {
		ua = "limithit/" + version.Version
	}

	return &http.Client{
		Timeout:   cfg.Timeout,
		Transport: &uaTransport{next: tr, ua: ua},
	}
}

func IsTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}
