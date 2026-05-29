package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "limithit-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestLoad_basic(t *testing.T) {
	path := writeTemp(t, `
target: http://localhost:8080
defaults:
  concurrency: 20
  output: json
scenario:
  - attack: flood
    total: 500
    expect-status: 429
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Target != "http://localhost:8080" {
		t.Errorf("target = %q", cfg.Target)
	}
	v, ok := cfg.Defaults.Get("concurrency")
	if !ok || v != "20" {
		t.Errorf("defaults.concurrency = %q, ok=%v", v, ok)
	}
	v, ok = cfg.Defaults.Get("output")
	if !ok || v != "json" {
		t.Errorf("defaults.output = %q, ok=%v", v, ok)
	}
	if len(cfg.Scenario) != 1 {
		t.Fatalf("scenario len = %d", len(cfg.Scenario))
	}
	s := cfg.Scenario[0]
	if s.Attack != "flood" {
		t.Errorf("attack = %q", s.Attack)
	}
	if s.Flags["total"] != "500" {
		t.Errorf("total = %q", s.Flags["total"])
	}
	if s.Flags["expect-status"] != "429" {
		t.Errorf("expect-status = %q", s.Flags["expect-status"])
	}
}

func TestLoad_envExpansion(t *testing.T) {
	t.Setenv("MY_TARGET", "http://example.com")
	path := writeTemp(t, `
target: ${MY_TARGET}
scenario:
  - attack: flood
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Target != "http://example.com" {
		t.Errorf("target = %q, want expanded env", cfg.Target)
	}
}

func TestLoad_envMissing(t *testing.T) {
	os.Unsetenv("LIMITHIT_MISSING_VAR")
	path := writeTemp(t, `
target: ${LIMITHIT_MISSING_VAR}
scenario:
  - attack: flood
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Target != "${LIMITHIT_MISSING_VAR}" {
		t.Errorf("target = %q, want unexpanded placeholder", cfg.Target)
	}
}

func TestLoad_missingTarget(t *testing.T) {
	path := writeTemp(t, `
scenario:
  - attack: flood
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing target")
	}
}

func TestLoad_emptyScenario(t *testing.T) {
	path := writeTemp(t, `
target: http://localhost:8080
scenario: []
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for empty scenario")
	}
}

func TestLoad_stepMissingAttack(t *testing.T) {
	path := writeTemp(t, `
target: http://localhost:8080
scenario:
  - total: 500
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for step missing 'attack'")
	}
}

func TestLoad_notFound(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "noexist.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestScaffold_validYAML(t *testing.T) {
	s := Scaffold()
	if len(s) == 0 {
		t.Fatal("Scaffold returned empty string")
	}
	var raw interface{}
	if err := yaml.Unmarshal([]byte(s), &raw); err != nil {
		t.Fatalf("scaffold is invalid YAML: %v", err)
	}
	if !strings.Contains(s, "${TARGET_URL}") {
		t.Error("scaffold should contain ${TARGET_URL} placeholder")
	}
}

func TestDefaults_each(t *testing.T) {
	path := writeTemp(t, `
target: http://localhost
defaults:
  concurrency: 10
  output: json
scenario:
  - attack: flood
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	cfg.Defaults.Each(func(k, v string) { got[k] = v })
	if got["concurrency"] != "10" {
		t.Errorf("concurrency = %q", got["concurrency"])
	}
	if got["output"] != "json" {
		t.Errorf("output = %q", got["output"])
	}
}
