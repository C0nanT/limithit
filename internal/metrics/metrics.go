package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/conantorreswf/ratelash/internal/client"
)

type Collector struct {
	mu           sync.Mutex
	sent         int
	success      int
	clientErr    int
	serverErr    int
	tooMany      int
	timeouts     int
	otherErr     int
	statusCounts map[int]int
}

func NewCollector() *Collector {
	return &Collector{statusCounts: make(map[int]int)}
}

func (c *Collector) Record(r client.Result) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.sent++

	if r.Err != nil {
		c.otherErr++
		if r.Timeout {
			c.timeouts++
		}
		return
	}

	c.statusCounts[r.Status]++

	switch {
	case r.Status >= 200 && r.Status < 300:
		c.success++
	case r.Status == 429:
		c.tooMany++
		c.clientErr++
	case r.Status >= 400 && r.Status < 500:
		c.clientErr++
	case r.Status >= 500 && r.Status < 600:
		c.serverErr++
	}
}

type Report struct {
	Sent         int
	Success      int
	ClientErr    int
	ServerErr    int
	TooMany      int
	Timeouts     int
	OtherErr     int
	StatusCounts map[int]int
	Duration     time.Duration
	RPS          float64
}

func Finalize(c *Collector, dur time.Duration) *Report {
	c.mu.Lock()
	defer c.mu.Unlock()

	rps := 0.0
	if dur > 0 {
		rps = float64(c.sent) / dur.Seconds()
	}

	counts := make(map[int]int, len(c.statusCounts))
	for k, v := range c.statusCounts {
		counts[k] = v
	}

	return &Report{
		Sent:         c.sent,
		Success:      c.success,
		ClientErr:    c.clientErr,
		ServerErr:    c.serverErr,
		TooMany:      c.tooMany,
		Timeouts:     c.timeouts,
		OtherErr:     c.otherErr,
		StatusCounts: counts,
		Duration:     dur,
		RPS:          rps,
	}
}

func (r *Report) String() string {
	var b strings.Builder
	fmt.Fprintln(&b, "=== ratelash summary ===")
	fmt.Fprintf(&b, "Sent:         %d\n", r.Sent)
	fmt.Fprintf(&b, "Success(2xx): %d\n", r.Success)
	fmt.Fprintf(&b, "Client(4xx):  %d   (429: %d)\n", r.ClientErr, r.TooMany)
	fmt.Fprintf(&b, "Server(5xx):  %d\n", r.ServerErr)
	fmt.Fprintf(&b, "Errors:       %d   (timeouts: %d)\n", r.OtherErr, r.Timeouts)
	fmt.Fprintf(&b, "Duration:     %s\n", r.Duration.Round(time.Millisecond))
	fmt.Fprintf(&b, "RPS:          %.2f\n", r.RPS)

	if len(r.StatusCounts) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "Status distribution:")
		keys := make([]int, 0, len(r.StatusCounts))
		for k := range r.StatusCounts {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "  %d: %d\n", k, r.StatusCounts[k])
		}
	}
	return b.String()
}
