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
	attack          string
	target          string

	latencies []time.Duration
	startedAt time.Time
	perSecond map[int]*secBucket
}

type secBucket struct {
	sent, ok, rateLimited, errors int64
}

type LatencyStats struct {
	P50  time.Duration `json:"p50"`
	P90  time.Duration `json:"p90"`
	P95  time.Duration `json:"p95"`
	P99  time.Duration `json:"p99"`
	Max  time.Duration `json:"max"`
	Mean time.Duration `json:"mean"`
}

type Bucket struct {
	SecondOffset int   `json:"second_offset"`
	Sent         int64 `json:"sent"`
	OK           int64 `json:"ok"`
	RateLimited  int64 `json:"rate_limited"`
	Errors       int64 `json:"errors"`
}

func NewCollector() *Collector {
	return &Collector{
		statusCounts: make(map[int]int),
		pathStatus:   make(map[string]map[int]int),
		perSecond:    make(map[int]*secBucket),
	}
}

func (c *Collector) SetTag(t string) {
	c.mu.Lock()
	c.tag = t
	c.mu.Unlock()
}

func (c *Collector) SetMeta(attack, target string) {
	c.mu.Lock()
	c.attack = attack
	c.target = target
	c.mu.Unlock()
}

func (c *Collector) SetStartedAt(t time.Time) {
	c.mu.Lock()
	c.startedAt = t
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

	var sb *secBucket
	if !c.startedAt.IsZero() {
		offset := int(time.Since(c.startedAt).Seconds())
		sb = c.perSecond[offset]
		if sb == nil {
			sb = &secBucket{}
			c.perSecond[offset] = sb
		}
		sb.sent++
	}

	if r.Err != nil {
		c.otherErr++
		if r.Timeout {
			c.timeouts++
		}
		if sb != nil {
			sb.errors++
		}
		return
	}

	if r.Duration > 0 {
		c.latencies = append(c.latencies, r.Duration)
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
		if sb != nil {
			sb.ok++
		}
	case r.Status == 413:
		c.payloadTooLarge++
		c.clientErr++
	case r.Status == 429:
		c.tooMany++
		c.clientErr++
		if sb != nil {
			sb.rateLimited++
		}
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
	Tag             string            `json:"tag,omitempty"`
	Attack          string            `json:"attack,omitempty"`
	Target          string            `json:"target,omitempty"`
	StartedAt       time.Time         `json:"started_at,omitempty"`
	Sent            int               `json:"sent"`
	Success         int               `json:"success"`
	ClientErr       int               `json:"client_err"`
	ServerErr       int               `json:"server_err"`
	TooMany         int               `json:"too_many"`
	HeaderTooLarge  int               `json:"header_too_large"`
	PayloadTooLarge int               `json:"payload_too_large"`
	Timeouts        int               `json:"timeouts"`
	OtherErr        int               `json:"other_err"`
	BytesSent       int64             `json:"bytes_sent"`
	StatusCounts    map[int]int       `json:"status_counts"`
	PathStatus      map[string]map[int]int `json:"path_status,omitempty"`
	Duration        time.Duration     `json:"duration_ns"`
	RPS             float64           `json:"rps"`
	Latency         LatencyStats      `json:"latency"`
	PerSecond       []Bucket          `json:"per_second,omitempty"`
}

func computeLatency(latencies []time.Duration) LatencyStats {
	if len(latencies) == 0 {
		return LatencyStats{}
	}
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	percentile := func(p float64) time.Duration {
		if len(sorted) == 1 {
			return sorted[0]
		}
		idx := int(p / 100.0 * float64(len(sorted)-1) + 0.5)
		if idx >= len(sorted) {
			idx = len(sorted) - 1
		}
		return sorted[idx]
	}

	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}

	return LatencyStats{
		P50:  percentile(50),
		P90:  percentile(90),
		P95:  percentile(95),
		P99:  percentile(99),
		Max:  sorted[len(sorted)-1],
		Mean: sum / time.Duration(len(sorted)),
	}
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

	perSecond := make([]Bucket, 0, len(c.perSecond))
	for offset, sb := range c.perSecond {
		perSecond = append(perSecond, Bucket{
			SecondOffset: offset,
			Sent:         sb.sent,
			OK:           sb.ok,
			RateLimited:  sb.rateLimited,
			Errors:       sb.errors,
		})
	}
	sort.Slice(perSecond, func(i, j int) bool { return perSecond[i].SecondOffset < perSecond[j].SecondOffset })

	return &Report{
		Tag:             c.tag,
		Attack:          c.attack,
		Target:          c.target,
		StartedAt:       c.startedAt,
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
		Latency:         computeLatency(c.latencies),
		PerSecond:       perSecond,
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
	if r.Latency.Max > 0 {
		fmt.Fprintf(&b, "Latency:      p50=%-9s p90=%-9s p95=%-9s p99=%-9s max=%s\n",
			r.Latency.P50.Round(time.Millisecond),
			r.Latency.P90.Round(time.Millisecond),
			r.Latency.P95.Round(time.Millisecond),
			r.Latency.P99.Round(time.Millisecond),
			r.Latency.Max.Round(time.Millisecond),
		)
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
