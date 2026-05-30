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

func (s *Spoof) FormFields() []attacks.FormField {
	url := ""
	total := "1000"
	concurrency := "20"
	method := "GET"
	timeout := "10"
	ipPool := ""
	xffHeader := "X-Forwarded-For"
	pacing := "none"
	rps := "50"
	minDelayMs := "0"
	maxDelayMs := "50"
	return []attacks.FormField{
		{Flag: "url", Label: "Target URL", Kind: attacks.FieldURL, Default: "", Value: &url},
		{Flag: "total", Label: "Total requests", Kind: attacks.FieldInt, Default: "1000", Validate: attacks.ValidatePosInt, Value: &total},
		{Flag: "concurrency", Label: "Concurrency (workers)", Kind: attacks.FieldInt, Default: "20", Validate: attacks.ValidatePosInt, Value: &concurrency},
		{Flag: "method", Label: "HTTP method", Kind: attacks.FieldSelect, Default: "GET", Choices: attacks.HTTPMethodChoices(), Value: &method},
		{Flag: "timeout", Label: "Timeout (s)", Kind: attacks.FieldInt, Default: "10", Validate: attacks.ValidatePosInt, Value: &timeout},
		{Flag: "ip-pool", Label: "IP pool", Help: `CIDR "10.0.0.0/24", "file:ips.txt", or comma list (required)`, Kind: attacks.FieldString, Default: "", Value: &ipPool},
		{Flag: "xff-header", Label: "XFF header name", Help: "Header used to inject the spoofed IP", Kind: attacks.FieldString, Default: "X-Forwarded-For", Value: &xffHeader},
		{Flag: "pacing", Label: "Pacing", Kind: attacks.FieldSelect, Default: "none", Choices: []string{"none", "uniform", "poisson", "zipf"}, Value: &pacing},
		{Flag: "rps", Label: "Target RPS (poisson)", Kind: attacks.FieldFloat, Default: "50", Validate: attacks.ValidatePosFloat, Value: &rps},
		{Flag: "min-delay-ms", Label: "Min delay ms (uniform/zipf)", Kind: attacks.FieldInt, Default: "0", Validate: attacks.ValidateNonNegInt, Value: &minDelayMs},
		{Flag: "max-delay-ms", Label: "Max delay ms (uniform/zipf)", Kind: attacks.FieldInt, Default: "50", Validate: attacks.ValidatePosInt, Value: &maxDelayMs},
	}
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
		Attack:      "spoof",
		Target:      base.URL,
		ProgressCh:  base.ProgressCh,
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
