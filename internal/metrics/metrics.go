package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/conantorreswf/limithit/internal/client"
)

type Collector struct {
	mu              sync.Mutex
	sent            int
	success         int
	clientErr       int
	serverErr       int
	tooMany         int
	timeouts        int
	otherErr        int
	headerTooLarge  int // 431
	payloadTooLarge int // 413
	bytesSent       int64
	statusCounts    map[int]int
	pathStatus      map[string]map[int]int
	tag             string
}

func NewCollector() *Collector {
	return &Collector{
		statusCounts: make(map[int]int),
		pathStatus:   make(map[string]map[int]int),
	}
}

func (c *Collector) SetTag(t string) {
	c.mu.Lock()
	c.tag = t
	c.mu.Unlock()
}

func (c *Collector) AddBytes(n int64) {
	c.mu.Lock()
	c.bytesSent += n
	c.mu.Unlock()
}

func (c *Collector) Record(r client.Result) {
	c.RecordPath("", r)
}

func (c *Collector) RecordPath(path string, r client.Result) {
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
	if path != "" {
		bucket, ok := c.pathStatus[path]
		if !ok {
			bucket = make(map[int]int)
			c.pathStatus[path] = bucket
		}
		bucket[r.Status]++
	}

	switch {
	case r.Status >= 200 && r.Status < 300:
		c.success++
	case r.Status == 413:
		c.payloadTooLarge++
		c.clientErr++
	case r.Status == 429:
		c.tooMany++
		c.clientErr++
	case r.Status == 431:
		c.headerTooLarge++
		c.clientErr++
	case r.Status >= 400 && r.Status < 500:
		c.clientErr++
	case r.Status >= 500 && r.Status < 600:
		c.serverErr++
	}
}

type Report struct {
	Tag             string
	Sent            int
	Success         int
	ClientErr       int
	ServerErr       int
	TooMany         int
	HeaderTooLarge  int
	PayloadTooLarge int
	Timeouts        int
	OtherErr        int
	BytesSent       int64
	StatusCounts    map[int]int
	PathStatus      map[string]map[int]int
	Duration        time.Duration
	RPS             float64
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

	pathCounts := make(map[string]map[int]int, len(c.pathStatus))
	for p, bucket := range c.pathStatus {
		nb := make(map[int]int, len(bucket))
		for k, v := range bucket {
			nb[k] = v
		}
		pathCounts[p] = nb
	}

	return &Report{
		Tag:             c.tag,
		Sent:            c.sent,
		Success:         c.success,
		ClientErr:       c.clientErr,
		ServerErr:       c.serverErr,
		TooMany:         c.tooMany,
		HeaderTooLarge:  c.headerTooLarge,
		PayloadTooLarge: c.payloadTooLarge,
		Timeouts:        c.timeouts,
		OtherErr:        c.otherErr,
		BytesSent:       c.bytesSent,
		StatusCounts:    counts,
		PathStatus:      pathCounts,
		Duration:        dur,
		RPS:             rps,
	}
}

func (r *Report) String() string {
	var b strings.Builder
	title := "limithit summary"
	if r.Tag != "" {
		title = "limithit " + r.Tag + " summary"
	}
	fmt.Fprintf(&b, "=== %s ===\n", title)
	fmt.Fprintf(&b, "Sent:         %d\n", r.Sent)
	fmt.Fprintf(&b, "Success(2xx): %d\n", r.Success)
	fmt.Fprintf(&b, "Client(4xx):  %d   (429: %d, 431: %d, 413: %d)\n",
		r.ClientErr, r.TooMany, r.HeaderTooLarge, r.PayloadTooLarge)
	fmt.Fprintf(&b, "Server(5xx):  %d\n", r.ServerErr)
	fmt.Fprintf(&b, "Errors:       %d   (timeouts: %d)\n", r.OtherErr, r.Timeouts)
	fmt.Fprintf(&b, "Duration:     %s\n", r.Duration.Round(time.Millisecond))
	fmt.Fprintf(&b, "RPS:          %.2f\n", r.RPS)
	if r.BytesSent > 0 {
		fmt.Fprintf(&b, "BytesSent:    %d (%.2f MB)\n", r.BytesSent, float64(r.BytesSent)/(1<<20))
	}

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

	if len(r.PathStatus) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "Per-path status (top 20 by hits):")
		type pathTotal struct {
			path  string
			total int
		}
		totals := make([]pathTotal, 0, len(r.PathStatus))
		for p, bucket := range r.PathStatus {
			sum := 0
			for _, v := range bucket {
				sum += v
			}
			totals = append(totals, pathTotal{p, sum})
		}
		sort.Slice(totals, func(i, j int) bool { return totals[i].total > totals[j].total })
		limit := 20
		if len(totals) < limit {
			limit = len(totals)
		}
		for i := 0; i < limit; i++ {
			pt := totals[i]
			bucket := r.PathStatus[pt.path]
			keys := make([]int, 0, len(bucket))
			for k := range bucket {
				keys = append(keys, k)
			}
			sort.Ints(keys)
			parts := make([]string, 0, len(keys))
			for _, k := range keys {
				parts = append(parts, fmt.Sprintf("%d:%d", k, bucket[k]))
			}
			fmt.Fprintf(&b, "  %-40s  %s\n", pt.path, strings.Join(parts, " "))
		}
	}
	return b.String()
}
