package metrics

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ConnCollector tracks connection-level outcomes for slowloris-style attacks.
type ConnCollector struct {
	mu              sync.Mutex
	attempted       int
	established     int
	droppedByServer int
	droppedClient   int
	holdDurations   []time.Duration
	bytesSent       int64
	errs            map[string]int
}

func NewConnCollector() *ConnCollector {
	return &ConnCollector{errs: make(map[string]int)}
}

func (c *ConnCollector) Attempt()     { c.mu.Lock(); c.attempted++; c.mu.Unlock() }
func (c *ConnCollector) Established() { c.mu.Lock(); c.established++; c.mu.Unlock() }

func (c *ConnCollector) DroppedByServer(hold time.Duration) {
	c.mu.Lock()
	c.droppedByServer++
	c.holdDurations = append(c.holdDurations, hold)
	c.mu.Unlock()
}

func (c *ConnCollector) DroppedClient(hold time.Duration) {
	c.mu.Lock()
	c.droppedClient++
	c.holdDurations = append(c.holdDurations, hold)
	c.mu.Unlock()
}

func (c *ConnCollector) Error(kind string) {
	c.mu.Lock()
	c.errs[kind]++
	c.mu.Unlock()
}

func (c *ConnCollector) AddBytes(n int64) {
	c.mu.Lock()
	c.bytesSent += n
	c.mu.Unlock()
}

type ConnReport struct {
	Attempted       int
	Established     int
	DroppedByServer int
	DroppedClient   int
	AvgHold         time.Duration
	MaxHold         time.Duration
	BytesSent       int64
	Errors          map[string]int
	Duration        time.Duration
}

func (c *ConnCollector) Finalize(dur time.Duration) *ConnReport {
	c.mu.Lock()
	defer c.mu.Unlock()

	var sum, maxDur time.Duration
	for _, d := range c.holdDurations {
		sum += d
		if d > maxDur {
			maxDur = d
		}
	}
	var avg time.Duration
	if n := len(c.holdDurations); n > 0 {
		avg = sum / time.Duration(n)
	}

	errs := make(map[string]int, len(c.errs))
	for k, v := range c.errs {
		errs[k] = v
	}

	return &ConnReport{
		Attempted:       c.attempted,
		Established:     c.established,
		DroppedByServer: c.droppedByServer,
		DroppedClient:   c.droppedClient,
		AvgHold:         avg,
		MaxHold:         maxDur,
		BytesSent:       c.bytesSent,
		Errors:          errs,
		Duration:        dur,
	}
}

func (r *ConnReport) String() string {
	var b strings.Builder
	fmt.Fprintln(&b, "=== limithit slowloris summary ===")
	fmt.Fprintf(&b, "Attempted:        %d\n", r.Attempted)
	fmt.Fprintf(&b, "Established:      %d\n", r.Established)
	fmt.Fprintf(&b, "DroppedByServer:  %d\n", r.DroppedByServer)
	fmt.Fprintf(&b, "DroppedByClient:  %d\n", r.DroppedClient)
	fmt.Fprintf(&b, "AvgHold:          %s\n", r.AvgHold.Round(time.Millisecond))
	fmt.Fprintf(&b, "MaxHold:          %s\n", r.MaxHold.Round(time.Millisecond))
	fmt.Fprintf(&b, "BytesSent:        %d\n", r.BytesSent)
	fmt.Fprintf(&b, "Duration:         %s\n", r.Duration.Round(time.Millisecond))
	if len(r.Errors) > 0 {
		fmt.Fprintln(&b, "\nErrors:")
		for k, v := range r.Errors {
			fmt.Fprintf(&b, "  %-20s %d\n", k, v)
		}
	}
	return b.String()
}
