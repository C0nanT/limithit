package metrics

import (
	"testing"
	"time"
)

func TestComputeLatency_empty(t *testing.T) {
	s := computeLatency(nil)
	if s.Max != 0 || s.P99 != 0 {
		t.Fatalf("expected zero stats for empty input, got %+v", s)
	}
}

func TestComputeLatency_single(t *testing.T) {
	s := computeLatency([]time.Duration{50 * time.Millisecond})
	if s.P50 != 50*time.Millisecond {
		t.Fatalf("p50 want 50ms got %s", s.P50)
	}
	if s.Max != 50*time.Millisecond {
		t.Fatalf("max want 50ms got %s", s.Max)
	}
}

// Known dataset: 10 values 10ms..100ms (step 10ms).
// p50 → median = idx 5 of 10 = 50ms (nearest-rank with rounding)
// p99 → idx 9 = 100ms (max)
// mean = 550ms/10 = 55ms
func TestComputeLatency_knownDataset(t *testing.T) {
	in := make([]time.Duration, 10)
	for i := range in {
		in[i] = time.Duration(i+1) * 10 * time.Millisecond
	}
	s := computeLatency(in)

	if s.Max != 100*time.Millisecond {
		t.Errorf("max: want 100ms got %s", s.Max)
	}
	if s.Mean != 55*time.Millisecond {
		t.Errorf("mean: want 55ms got %s", s.Mean)
	}
	if s.P99 != 100*time.Millisecond {
		t.Errorf("p99: want 100ms got %s", s.P99)
	}
	// p50: idx = round(0.5 * 9) = 5 → in[5] = 60ms
	if s.P50 != 60*time.Millisecond {
		t.Errorf("p50: want 60ms got %s", s.P50)
	}
}

func TestComputeLatency_outOfOrder(t *testing.T) {
	in := []time.Duration{
		300 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond,
	}
	s := computeLatency(in)
	if s.Max != 500*time.Millisecond {
		t.Errorf("max: want 500ms got %s", s.Max)
	}
	if s.P50 != 300*time.Millisecond {
		t.Errorf("p50: want 300ms got %s", s.P50)
	}
}
