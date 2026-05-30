package metrics

import "time"

// Progress is a point-in-time snapshot of an in-flight attack run, emitted
// periodically via the ProgressCh field in attacks.Base.
type Progress struct {
	Sent        int
	Total       int
	Success     int
	RateLimited int
	OtherErr    int
	Elapsed     time.Duration
	RPS         float64
}

// Snapshot returns a progress summary. total is the target request count;
// the collector does not track it so the caller must supply it.
func (c *Collector) Snapshot(total int, elapsed time.Duration) Progress {
	c.mu.Lock()
	defer c.mu.Unlock()
	rps := 0.0
	if elapsed > 0 {
		rps = float64(c.sent) / elapsed.Seconds()
	}
	return Progress{
		Sent:        c.sent,
		Total:       total,
		Success:     c.success,
		RateLimited: c.tooMany,
		OtherErr:    c.otherErr,
		Elapsed:     elapsed,
		RPS:         rps,
	}
}
