package flood

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/conantorreswf/limithit/internal/attacks"
	"github.com/conantorreswf/limithit/internal/worker"
)

func init() {
	attacks.Register("flood", func() attacks.Attack { return &Flood{} })
}

type Flood struct {
	method string
	body   string
}

func (f *Flood) Name() string { return "flood" }
func (f *Flood) Synopsis() string {
	return "high-throughput request flood (basic load/rate-limit probe)"
}

func (f *Flood) Flags(fs *flag.FlagSet) {
	fs.StringVar(&f.method, "method", "GET", "HTTP method")
	fs.StringVar(&f.body, "body", "", "request body")
}

func (f *Flood) Validate() error {
	m, err := validateMethod(f.method)
	if err != nil {
		return err
	}
	f.method = m
	return nil
}

func (f *Flood) Run(ctx context.Context, base attacks.Base) (attacks.Report, error) {
	build := func(ctx context.Context, _ int) (*http.Request, string, error) {
		var body io.Reader
		if f.body != "" {
			body = strings.NewReader(f.body)
		}
		req, err := http.NewRequestWithContext(ctx, f.method, base.URL, body)
		if err != nil {
			return nil, "", err
		}
		for k, vs := range base.Common.Headers {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		return req, "", nil
	}

	return worker.Run(ctx, base.Client, build, worker.Config{
		Total:       base.Common.Total,
		Concurrency: base.Common.Concurrency,
		Tag:         "flood",
	}), nil
}

func validateMethod(m string) (string, error) {
	m = strings.ToUpper(m)
	switch m {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return m, nil
	}
	return "", fmt.Errorf("invalid method %q", m)
}
