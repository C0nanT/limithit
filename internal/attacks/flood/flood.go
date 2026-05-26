package flood

import (
	"context"
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
}

func Run(ctx context.Context, opts Options) *metrics.Report {
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
		return req, "", nil
	}

	return worker.Run(ctx, hc, build, worker.Config{
		Total:       opts.Total,
		Concurrency: opts.Concurrency,
		Tag:         "flood",
	})
}
