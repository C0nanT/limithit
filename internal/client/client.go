package client

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

type Config struct {
	URL               string
	Method            string
	Body              string
	Headers           http.Header
	Timeout           time.Duration
	DisableKeepAlives bool
}

type Result struct {
	Status   int
	Err      error
	Timeout  bool
	Duration time.Duration
}

// New returns an *http.Client whose Transport is tuned for the given
// concurrency level. The returned client honors cfg.Timeout per request.
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

	return &http.Client{
		Timeout:   cfg.Timeout,
		Transport: tr,
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
