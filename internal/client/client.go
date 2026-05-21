package client

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	URL     string
	Method  string
	Body    string
	Headers http.Header
	Timeout time.Duration
}

type Result struct {
	Status   int
	Err      error
	Timeout  bool
	Duration time.Duration
}

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
	tr.DisableKeepAlives = false

	return &http.Client{
		Timeout:   cfg.Timeout,
		Transport: tr,
	}
}

func Do(ctx context.Context, hc *http.Client, cfg Config) Result {
	start := time.Now()

	var bodyReader io.Reader
	if cfg.Body != "" {
		bodyReader = strings.NewReader(cfg.Body)
	}

	req, err := http.NewRequestWithContext(ctx, cfg.Method, cfg.URL, bodyReader)
	if err != nil {
		return Result{Err: err, Duration: time.Since(start)}
	}
	for k, vs := range cfg.Headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	resp, err := hc.Do(req)
	if err != nil {
		return Result{
			Err:      err,
			Timeout:  isTimeout(err),
			Duration: time.Since(start),
		}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return Result{
		Status:   resp.StatusCode,
		Duration: time.Since(start),
	}
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}
