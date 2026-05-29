package h2flood

import (
	"testing"
)

func TestH2FloodValidate(t *testing.T) {
	h := &H2Flood{connections: 1, streams: 10, method: "GET"}
	if err := h.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.method != "GET" {
		t.Fatalf("expected GET, got %q", h.method)
	}
}

func TestH2FloodValidateBadMethod(t *testing.T) {
	h := &H2Flood{connections: 1, streams: 10, method: "FOOBAR"}
	if err := h.Validate(); err == nil {
		t.Fatal("expected error for invalid method")
	}
}

func TestH2FloodValidateBadConnections(t *testing.T) {
	h := &H2Flood{connections: 0, streams: 10, method: "GET"}
	if err := h.Validate(); err == nil {
		t.Fatal("expected error for connections=0")
	}
}
