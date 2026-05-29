package replay

import (
	"os"
	"testing"
)

func TestParseLines(t *testing.T) {
	data := []byte("GET http://example.com/a\nPOST http://example.com/b\n# comment\n\nhttp://example.com/c\n")
	reqs, err := parseLines(data)
	if err != nil {
		t.Fatalf("parseLines: %v", err)
	}
	if len(reqs) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(reqs))
	}
	if reqs[0].Method != "GET" || reqs[0].URL != "http://example.com/a" {
		t.Fatalf("unexpected request[0]: %+v", reqs[0])
	}
	if reqs[2].Method != "GET" {
		t.Fatalf("URL-only line should default to GET, got %q", reqs[2].Method)
	}
}

func TestParseHAR(t *testing.T) {
	har := []byte(`{"log":{"entries":[{"request":{"method":"GET","url":"http://example.com/"}},{"request":{"method":"POST","url":"http://example.com/api"}}]}}`)
	reqs, err := parseHAR(har)
	if err != nil {
		t.Fatalf("parseHAR: %v", err)
	}
	if len(reqs) != 2 {
		t.Fatalf("expected 2, got %d", len(reqs))
	}
	if reqs[1].Method != "POST" {
		t.Fatalf("expected POST, got %q", reqs[1].Method)
	}
}

func TestValidateRequiresFile(t *testing.T) {
	r := &Replay{}
	if err := r.Validate(); err == nil {
		t.Fatal("expected error without --file")
	}
}

func TestValidateLoadFile(t *testing.T) {
	f, err := os.CreateTemp("", "replay-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("GET http://example.com/test\n")
	f.Close()

	r := &Replay{file: f.Name()}
	if err := r.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(r.reqs))
	}
}
