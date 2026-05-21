package worker

import (
	"context"
	"sync"
	"time"

	"github.com/conantorreswf/ratelash/internal/client"
	"github.com/conantorreswf/ratelash/internal/metrics"
)

func Run(ctx context.Context, cfg client.Config, total, concurrency int) *metrics.Report {
	if concurrency > total {
		concurrency = total
	}
	if concurrency < 1 {
		concurrency = 1
	}

	hc := client.New(cfg, concurrency)
	collector := metrics.NewCollector()

	jobs := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	start := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				if ctx.Err() != nil {
					return
				}
				res := client.Do(ctx, hc, cfg)
				collector.Record(res)
			}
		}()
	}

producer:
	for i := 0; i < total; i++ {
		select {
		case <-ctx.Done():
			break producer
		case jobs <- struct{}{}:
		}
	}
	close(jobs)

	wg.Wait()
	dur := time.Since(start)

	return metrics.Finalize(collector, dur)
}
