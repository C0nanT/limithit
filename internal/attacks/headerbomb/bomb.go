package headerbomb

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/conantorreswf/limithit/internal/client"
	"github.com/conantorreswf/limithit/internal/metrics"
	"github.com/conantorreswf/limithit/internal/worker"
)

type Options struct {
	URL         string
	Method      string
	Headers     http.Header
	Timeout     time.Duration
	Total       int
	Concurrency int

	HeaderCount int // how many X-Junk-N headers per request
	HeaderSize  int // length of each header value
	BodyStart   int // initial body size in bytes
	BodyMax     int // max body size (capped)
	BodyStep    int // bytes to add per step (0 = double each time)
}

func Run(ctx context.Context, opts Options) (*metrics.Report, error) {
	if opts.HeaderCount < 0 || opts.HeaderSize < 0 {
		return nil, fmt.Errorf("headerbomb: count and size must be >= 0")
	}
	if opts.BodyMax < opts.BodyStart {
		opts.BodyMax = opts.BodyStart
	}

	junkVal := strings.Repeat("A", opts.HeaderSize)
	method := opts.Method
	if method == "" {
		if opts.BodyMax > 0 {
			method = http.MethodPost
		} else {
			method = http.MethodGet
		}
	}

	var currentBody atomic.Int64
	currentBody.Store(int64(opts.BodyStart))

	hc := client.New(client.Config{Timeout: opts.Timeout}, opts.Concurrency)

	build := func(ctx context.Context, idx int) (*http.Request, string, error) {
		bodySize := int(currentBody.Load())
		// progressive growth: bump after each Concurrency-sized chunk
		if bodySize < opts.BodyMax {
			step := opts.BodyStep
			if step <= 0 {
				step = bodySize // double
				if step == 0 {
					step = 1024
				}
			}
			next := bodySize + step
			if next > opts.BodyMax {
				next = opts.BodyMax
			}
			currentBody.CompareAndSwap(int64(bodySize), int64(next))
		}

		var body *bytes.Reader
		if bodySize > 0 {
			body = bytes.NewReader(bytes.Repeat([]byte{'A'}, bodySize))
		}

		var req *http.Request
		var err error
		if body != nil {
			req, err = http.NewRequestWithContext(ctx, method, opts.URL, body)
		} else {
			req, err = http.NewRequestWithContext(ctx, method, opts.URL, nil)
		}
		if err != nil {
			return nil, "", err
		}
		for k, vs := range opts.Headers {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		for i := 0; i < opts.HeaderCount; i++ {
			req.Header.Add(fmt.Sprintf("X-Junk-%d", i), junkVal)
		}
		return req, "", nil
	}

	tag := fmt.Sprintf("headerbomb (hdrs=%dx%dB body=%d→%dB)",
		opts.HeaderCount, opts.HeaderSize, opts.BodyStart, opts.BodyMax)
	report := worker.Run(ctx, hc, build, worker.Config{
		Total:       opts.Total,
		Concurrency: opts.Concurrency,
		Tag:         tag,
	})
	return report, nil
}
