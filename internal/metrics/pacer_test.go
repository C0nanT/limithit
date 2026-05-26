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
