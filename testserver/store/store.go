package store

import (
	"encoding/json"
	"sync"
	"time"
)

const (
	timelineSize = 60
	recentLogMax = 20
)

type TimeSlot struct {
	Ts    time.Time
	Count int64
}

type LogEntry struct {
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	Status     int       `json:"status"`
	DurationMs float64   `json:"duration_ms"`
	Timestamp  time.Time `json:"timestamp"`
}

type Snapshot struct {
	TotalGET       int64      `json:"total_get"`
	TotalPOST      int64      `json:"total_post"`
	Total429       int64      `json:"total_429"`
	TotalRequests  int64      `json:"total_requests"`
	AvgLatencyMs   float64    `json:"avg_latency_ms"`
	CurrentRPS     float64    `json:"current_rps"`
	Timeline       []int64    `json:"timeline"`
	LatencyBuckets []int64    `json:"latency_buckets"`
	RecentLog      []LogEntry `json:"recent_log"`
}

type MetricsStore struct {
	mu sync.RWMutex

	totalGET       int64
	totalPOST      int64
	total429       int64
	totalRequests  int64
	totalLatencyUs int64 // microseconds for precision

	// latency histogram buckets: <5, 5-10, 10-25, 25-50, 50-100, 100-250, 250+ms
	latencyBuckets [7]int64

	timeline     [timelineSize]TimeSlot
	timelineHead int

	recentLog []LogEntry
}

func New() *MetricsStore {
	s := &MetricsStore{}
	now := time.Now().Truncate(time.Second)
	for i := range s.timeline {
		s.timeline[i].Ts = now.Add(-time.Duration(timelineSize-i) * time.Second)
	}
	s.timelineHead = timelineSize - 1
	return s
}

func (s *MetricsStore) Record(method, path string, status int, d time.Duration) {
	ms := float64(d) / float64(time.Millisecond)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalRequests++
	switch method {
	case "GET":
		s.totalGET++
	case "POST":
		s.totalPOST++
	}
	if status == 429 {
		s.total429++
	}

	s.totalLatencyUs += d.Microseconds()

	// latency bucket
	switch {
	case ms < 5:
		s.latencyBuckets[0]++
	case ms < 10:
		s.latencyBuckets[1]++
	case ms < 25:
		s.latencyBuckets[2]++
	case ms < 50:
		s.latencyBuckets[3]++
	case ms < 100:
		s.latencyBuckets[4]++
	case ms < 250:
		s.latencyBuckets[5]++
	default:
		s.latencyBuckets[6]++
	}

	// timeline: advance to current second if needed
	now := time.Now().Truncate(time.Second)
	head := &s.timeline[s.timelineHead]
	if head.Ts.Before(now) {
		// advance ring buffer, zero-filling gaps
		gap := int(now.Sub(head.Ts).Seconds())
		if gap > timelineSize {
			gap = timelineSize
		}
		for i := 0; i < gap; i++ {
			s.timelineHead = (s.timelineHead + 1) % timelineSize
			s.timeline[s.timelineHead] = TimeSlot{Ts: head.Ts.Add(time.Duration(i+1) * time.Second), Count: 0}
		}
		s.timeline[s.timelineHead].Ts = now
	}
	s.timeline[s.timelineHead].Count++

	// recent log
	entry := LogEntry{
		Method:     method,
		Path:       path,
		Status:     status,
		DurationMs: ms,
		Timestamp:  time.Now(),
	}
	if len(s.recentLog) >= recentLogMax {
		s.recentLog = append(s.recentLog[1:], entry)
	} else {
		s.recentLog = append(s.recentLog, entry)
	}
}

func (s *MetricsStore) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var avgLatency float64
	if s.totalRequests > 0 {
		avgLatency = float64(s.totalLatencyUs) / float64(s.totalRequests) / 1000.0
	}

	// current RPS: average over last 5 filled seconds
	timeline := make([]int64, timelineSize)
	var rpsSum int64
	rpsSamples := 0
	for i := 0; i < timelineSize; i++ {
		idx := (s.timelineHead - i + timelineSize) % timelineSize
		timeline[timelineSize-1-i] = s.timeline[idx].Count
		if i < 5 {
			rpsSum += s.timeline[idx].Count
			rpsSamples++
		}
	}
	var currentRPS float64
	if rpsSamples > 0 {
		currentRPS = float64(rpsSum) / float64(rpsSamples)
	}

	buckets := make([]int64, 7)
	copy(buckets, s.latencyBuckets[:])

	log := make([]LogEntry, len(s.recentLog))
	copy(log, s.recentLog)
	// reverse so newest first
	for i, j := 0, len(log)-1; i < j; i, j = i+1, j-1 {
		log[i], log[j] = log[j], log[i]
	}

	return Snapshot{
		TotalGET:       s.totalGET,
		TotalPOST:      s.totalPOST,
		Total429:       s.total429,
		TotalRequests:  s.totalRequests,
		AvgLatencyMs:   avgLatency,
		CurrentRPS:     currentRPS,
		Timeline:       timeline,
		LatencyBuckets: buckets,
		RecentLog:      log,
	}
}

// SSEBroadcaster fans out JSON payloads to connected SSE clients.
type SSEBroadcaster struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
}

func NewBroadcaster() *SSEBroadcaster {
	return &SSEBroadcaster{clients: make(map[chan []byte]struct{})}
}

func (b *SSEBroadcaster) Register(ch chan []byte) {
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
}

func (b *SSEBroadcaster) Unregister(ch chan []byte) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
}

func (b *SSEBroadcaster) Broadcast(snap Snapshot) {
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- data:
		default:
		}
	}
}
