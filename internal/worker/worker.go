package worker

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/conantorreswf/limithit/internal/client"
	"github.com/conantorreswf/limithit/internal/metrics"
)

// RequestBuilder builds the i-th request. Implementations are called from
// many goroutines and MUST be safe for concurrent use.
//
// The path return value tags the result inside the collector's per-path map
// (used by fuzz mode). Pass "" if path-level breakdown isn't wanted.
type RequestBuilder func(ctx context.Context, idx int) (req *http.Request, path string, err error)

type Config struct {
	Total       int
	Concurrency int
	Pacer       metrics.Pacer
	Tag         string
}

func Run(ctx context.Context, hc *http.Client, build RequestBuilder, cfg Config) *metrics.Report {
	if cfg.Concurrency > cfg.Total {
		cfg.Concurrency = cfg.Total
	}
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}

	collector := metrics.NewCollector()
	collector.SetTag(cfg.Tag)
	pacer := cfg.Pacer
	if pacer == nil {
		pacer = metrics.NoopPacer()
	}

	type job struct{ idx int }
	jobs := make(chan job, cfg.Concurrency)
	var wg sync.WaitGroup

	start := time.Now()

	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if ctx.Err() != nil {
					return
				}
				if d := pacer.Next(); d > 0 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(d):
					}
				}
				req, path, err := build(ctx, j.idx)
				if err != nil {
					collector.RecordPath(path, client.Result{Err: err})
					continue
				}
				res := doRequest(hc, req)
				collector.RecordPath(path, res)
			}
		}()
	}

producer:
	for i := 0; i < cfg.Total; i++ {
		select {
		case <-ctx.Done():
			break producer
		case jobs <- job{idx: i}:
		}
	}
	close(jobs)

	wg.Wait()
	dur := time.Since(start)

	return metrics.Finalize(collector, dur)
}

func doRequest(hc *http.Client, req *http.Request) client.Result {
	start := time.Now()
	resp, err := hc.Do(req)
	if err != nil {
		return client.Result{
			Err:      err,
			Timeout:  isTimeout(err),
			Duration: time.Since(start),
		}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return client.Result{
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
