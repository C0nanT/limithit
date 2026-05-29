package wsflood

import (
	"testing"
)

func TestWSKeyLength(t *testing.T) {
	k := wsKey()
	// Base64-encoded 16 bytes = 24 chars.
	if len(k) != 24 {
		t.Fatalf("wsKey length: got %d want 24", len(k))
	}
}

func TestWSPingFrameStructure(t *testing.T) {
	frame := wsPingFrame()
	if len(frame) != 6 {
		t.Fatalf("ping frame length: got %d want 6", len(frame))
	}
	if frame[0] != 0x89 {
		t.Fatalf("byte 0: got 0x%02x want 0x89 (FIN+ping)", frame[0])
	}
	if frame[1] != 0x80 {
		t.Fatalf("byte 1: got 0x%02x want 0x80 (MASK+len=0)", frame[1])
	}
}

func TestWSFloodValidate(t *testing.T) {
	w := &WSFlood{connections: 10, hold: 5, messageRate: 0, dialTimeout: 3}
	if err := w.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWSFloodValidateBadConnections(t *testing.T) {
	w := &WSFlood{connections: 0}
	if err := w.Validate(); err == nil {
		t.Fatal("expected error for connections=0")
	}
}
