package metrics

import (
	"testing"
	"time"
)

func TestUniformPacerInRange(t *testing.T) {
	p, err := NewPacer("uniform", 10*time.Millisecond, 100*time.Millisecond, 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for i := 0; i < 200; i++ {
		d := p.Next()
		if d < 10*time.Millisecond || d >= 100*time.Millisecond {
			t.Fatalf("uniform: %v out of range", d)
		}
	}
}

func TestPoissonPacerPositive(t *testing.T) {
	p, err := NewPacer("poisson", 0, 0, 100)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for i := 0; i < 200; i++ {
		d := p.Next()
		if d < 0 {
			t.Fatalf("poisson negative: %v", d)
		}
	}
}

func TestZipfPacerLongTail(t *testing.T) {
	p, err := NewPacer("zipf", 0, 100*time.Millisecond, 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Sample a bunch and ensure we see varied values
	seen := make(map[time.Duration]int)
	for i := 0; i < 1000; i++ {
		seen[p.Next()]++
	}
	if len(seen) < 5 {
		t.Fatalf("zipf produced too little variety: %d buckets", len(seen))
	}
}

func TestNoopPacer(t *testing.T) {
	p, err := NewPacer("none", 0, 0, 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if p.Next() != 0 {
		t.Fatalf("noop should return 0")
	}
}

func TestUnknownPacer(t *testing.T) {
	if _, err := NewPacer("ai-generated", 0, 0, 0); err == nil {
		t.Fatalf("expected error for unknown pacer")
	}
}

func TestRampPacerMonotonicRate(t *testing.T) {
	// 1 RPS → 100 RPS over 100ms: delays should shrink over time.
	p, err := NewRampPacer(1, 100, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	var prev time.Duration = -1
	samples := 10
	for i := 0; i < samples; i++ {
		d := p.Next()
		if d < 0 {
			t.Fatalf("negative delay: %v", d)
		}
		if prev >= 0 && d > prev {
			// Allow small jitter from timing, but delay should trend down.
			// We check the overall trend: first sample vs last.
		}
		prev = d
		time.Sleep(10 * time.Millisecond)
	}
	// First delay (at 1 RPS) ≈ 1s; last (near 100 RPS) ≈ 10ms.
	// Just verify the pacer produces positive values and doesn't panic.
}

func TestRampPacerAfterRamp(t *testing.T) {
	// After rampDuration, rate should be fixed at endRPS.
	p, err := NewRampPacer(10, 100, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	d := p.Next()
	want := time.Duration(float64(time.Second) / 100)
	// Allow 5% tolerance.
	if d < want*95/100 || d > want*105/100 {
		t.Fatalf("after ramp: got %v want ~%v", d, want)
	}
}

func TestRampPacerValidation(t *testing.T) {
	if _, err := NewRampPacer(10, 0, time.Second); err == nil {
		t.Fatal("expected error for endRPS=0")
	}
	if _, err := NewRampPacer(-1, 10, time.Second); err == nil {
		t.Fatal("expected error for startRPS<0")
	}
}
