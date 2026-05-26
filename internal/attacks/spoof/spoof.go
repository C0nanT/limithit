package spoof

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/conantorreswf/ratelash/internal/client"
	"github.com/conantorreswf/ratelash/internal/metrics"
	"github.com/conantorreswf/ratelash/internal/worker"
)

type Options struct {
	URL         string
	Method      string
	Body        string
	Headers     http.Header
	Timeout     time.Duration
	Total       int
	Concurrency int

	IPPoolSpec string // e.g. "10.0.0.0/24" or "file:ips.txt" or "1.2.3.4,5.6.7.8"
	Pacing     string // uniform|poisson|zipf|none
	MinDelay   time.Duration
	MaxDelay   time.Duration
	RPS        float64 // for poisson
	XFFHeader  string  // default X-Forwarded-For
}

func Run(ctx context.Context, opts Options) (*metrics.Report, error) {
	if opts.IPPoolSpec == "" {
		return nil, errors.New("spoof: --ip-pool is required")
	}
	pool, err := metrics.NewIPPoolFromSpec(opts.IPPoolSpec)
	if err != nil {
		return nil, fmt.Errorf("spoof: %w", err)
	}
	pacer, err := metrics.NewPacer(opts.Pacing, opts.MinDelay, opts.MaxDelay, opts.RPS)
	if err != nil {
		return nil, fmt.Errorf("spoof: %w", err)
	}

	hdr := opts.XFFHeader
	if hdr == "" {
		hdr = "X-Forwarded-For"
	}

	hc := client.New(client.Config{Timeout: opts.Timeout}, opts.Concurrency)

	build := func(ctx context.Context, _ int) (*http.Request, string, error) {
		var body io.Reader
		if opts.Body != "" {
			body = strings.NewReader(opts.Body)
		}
		req, err := http.NewRequestWithContext(ctx, opts.Method, opts.URL, body)
		if err != nil {
			return nil, "", err
		}
		for k, vs := range opts.Headers {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		ip := pool.Next()
		req.Header.Set(hdr, ip)
		// also set X-Real-IP as commonly trusted alternative
		req.Header.Set("X-Real-IP", ip)
		return req, "", nil
	}

	report := worker.Run(ctx, hc, build, worker.Config{
		Total:       opts.Total,
		Concurrency: opts.Concurrency,
		Pacer:       pacer,
		Tag:         fmt.Sprintf("spoof (pool=%d pacing=%s)", pool.Size(), opts.Pacing),
	})
	return report, nil
}
