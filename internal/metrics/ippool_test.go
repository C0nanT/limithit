package metrics

import "testing"

func TestExpandCIDR24(t *testing.T) {
	p, err := NewIPPoolFromSpec("10.0.0.0/24")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if p.Size() != 256 {
		t.Fatalf("expected 256 IPs, got %d", p.Size())
	}
}

func TestExpandCIDRTooBigCaps(t *testing.T) {
	p, err := NewIPPoolFromSpec("10.0.0.0/8")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if p.Size() != maxCIDRExpansion {
		t.Fatalf("expected %d, got %d", maxCIDRExpansion, p.Size())
	}
}

func TestCommaList(t *testing.T) {
	p, err := NewIPPoolFromSpec("1.1.1.1,2.2.2.2,3.3.3.3")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if p.Size() != 3 {
		t.Fatalf("got %d", p.Size())
	}
	seen := map[string]bool{}
	for i := 0; i < 6; i++ {
		seen[p.Next()] = true
	}
	if len(seen) != 3 {
		t.Fatalf("rotation broken: %v", seen)
	}
}

func TestInvalidSpecs(t *testing.T) {
	if _, err := NewIPPoolFromSpec(""); err == nil {
		t.Fatal("expected err for empty")
	}
	if _, err := NewIPPoolFromSpec("not-an-ip"); err == nil {
		t.Fatal("expected err for junk")
	}
	if _, err := NewIPPoolFromSpec("256.256.256.256/24"); err == nil {
		t.Fatal("expected err for bad CIDR")
	}
}
