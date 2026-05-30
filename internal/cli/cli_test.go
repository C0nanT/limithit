package cli_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/conantorreswf/limithit/internal/cli"
)

func TestHelpExitsZero(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		var out, errOut bytes.Buffer
		if code := cli.Run(args, &out, &errOut); code != 0 {
			t.Errorf("Run(%v) = %d, want 0", args, code)
		}
	}
}

func TestUnknownCommandExitsTwo(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"no-such-cmd"}, &out, &errOut)
	if code != 2 {
		t.Errorf("unknown cmd: got %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "no-such-cmd") {
		t.Errorf("stderr should mention the unknown command; got %q", errOut.String())
	}
}

func TestFloodMissingURLExitsTwo(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"flood"}, &out, &errOut); code != 2 {
		t.Errorf("flood without URL: got %d, want 2", code)
	}
}

func TestFloodURLPositions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cases := []struct {
		name string
		args []string
	}{
		{"url-first", []string{"flood", srv.URL, "--total", "3", "--concurrency", "2"}},
		{"url-last", []string{"flood", "--total", "3", "--concurrency", "2", srv.URL}},
		{"url-middle", []string{"flood", "--total", "3", srv.URL, "--concurrency", "2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := cli.Run(tc.args, &out, &errOut); code != 0 {
				t.Errorf("Run(%v) = %d, want 0; stderr: %s", tc.args, code, errOut.String())
			}
		})
	}
}

func TestInitCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "limithit.yaml")
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"init", path}, &out, &errOut); code != 0 {
		t.Fatalf("init: got %d; stderr: %s", code, errOut.String())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if !strings.Contains(out.String(), path) {
		t.Errorf("stdout should mention %q; got %q", path, out.String())
	}
}

func TestInitExistingFileExitsTwo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "limithit.yaml")
	if err := os.WriteFile(path, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"init", path}, &out, &errOut); code != 2 {
		t.Errorf("init existing file: got %d, want 2", code)
	}
}

func TestRunMissingScenarioExitsTwo(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"run"}, &out, &errOut); code != 2 {
		t.Errorf("run without file: got %d, want 2", code)
	}
}

func TestRunNonExistentScenarioExitsTwo(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"run", "/nonexistent/path.yaml"}, &out, &errOut); code != 2 {
		t.Errorf("run with missing file: got %d, want 2", code)
	}
}
