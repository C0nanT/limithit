package gzipbomb

import (
	"compress/gzip"
	"io"
	"testing"
)

func TestBuildBombCompresses(t *testing.T) {
	bomb, err := buildBomb(1)
	if err != nil {
		t.Fatalf("buildBomb: %v", err)
	}
	if len(bomb) == 0 {
		t.Fatal("bomb is empty")
	}
	// Compressed size must be much smaller than 1 MB.
	if len(bomb) >= 1<<20 {
		t.Fatalf("expected compressed size < 1MB, got %d bytes", len(bomb))
	}
}

func TestBombDecompressesCorrectly(t *testing.T) {
	bomb, err := buildBomb(1)
	if err != nil {
		t.Fatalf("buildBomb: %v", err)
	}
	br := bytesReader(bomb)
	r, err := gzip.NewReader(io.NopCloser(&br))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	n, err := io.Copy(io.Discard, r)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	want := int64(1 << 20)
	if n != want {
		t.Fatalf("decompressed size: got %d want %d", n, want)
	}
}

func TestValidateSafetyGate(t *testing.T) {
	g := &GzipBomb{expandedMB: 1, iUnderstand: false}
	if err := g.Validate(); err == nil {
		t.Fatal("expected error without --i-understand")
	}
}

func TestValidatePass(t *testing.T) {
	g := &GzipBomb{expandedMB: 1, iUnderstand: true}
	if err := g.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// bytesReader wraps []byte as io.Reader.
type bytesReader []byte

func (b *bytesReader) Read(p []byte) (int, error) {
	if len(*b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, *b)
	*b = (*b)[n:]
	return n, nil
}
