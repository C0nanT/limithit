package metrics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultWordlistNonEmpty(t *testing.T) {
	w := DefaultWordlist()
	if w.Size() < 10 {
		t.Fatalf("default wordlist too small: %d", w.Size())
	}
	for _, p := range w.All() {
		if !strings.HasPrefix(p, "/") {
			t.Fatalf("entry missing leading /: %q", p)
		}
	}
}

func TestLoadWordlistFromFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "paths.txt")
	body := "# comment\n/admin\nusers\n   \n/api/v1\n"
	if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := LoadWordlist(f)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if w.Size() != 3 {
		t.Fatalf("expected 3 paths, got %d (%v)", w.Size(), w.All())
	}
	for _, p := range w.All() {
		if !strings.HasPrefix(p, "/") {
			t.Fatalf("expected leading slash, got %q", p)
		}
	}
}
