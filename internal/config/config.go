package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Defaults holds per-scenario default flag values, keyed by flag name.
type Defaults struct {
	m map[string]string
}

func (d *Defaults) UnmarshalYAML(value *yaml.Node) error {
	var raw map[string]interface{}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	d.m = make(map[string]string, len(raw))
	for k, v := range raw {
		d.m[k] = anyToStr(v)
	}
	return nil
}

// Get returns the default value for a flag name.
func (d *Defaults) Get(key string) (string, bool) {
	v, ok := d.m[key]
	return v, ok
}

// Each calls fn for every default key/value pair.
func (d *Defaults) Each(fn func(k, v string)) {
	for k, v := range d.m {
		fn(k, v)
	}
}

// Step is one attack entry in a scenario.
type Step struct {
	Attack string
	Flags  map[string]string // flag-name → value (excludes "attack" key)
}

func (s *Step) UnmarshalYAML(value *yaml.Node) error {
	var raw map[string]interface{}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	atk, ok := raw["attack"]
	if !ok {
		return fmt.Errorf("step missing required field 'attack'")
	}
	s.Attack = fmt.Sprintf("%v", atk)
	delete(raw, "attack")

	s.Flags = make(map[string]string, len(raw))
	for k, v := range raw {
		s.Flags[k] = anyToStr(v)
	}
	return nil
}

// Config is the top-level structure of a limithit.yaml file.
type Config struct {
	Target   string   `yaml:"target"`
	Defaults Defaults `yaml:"defaults"`
	Scenario []Step   `yaml:"scenario"`
}

var envRe = regexp.MustCompile(`\$\{([^}]+)\}`)

func expandStr(s string) string {
	return envRe.ReplaceAllStringFunc(s, func(m string) string {
		key := m[2 : len(m)-1]
		if v, ok := os.LookupEnv(key); ok {
			return v
		}
		return m
	})
}

func expand(cfg *Config) {
	cfg.Target = expandStr(cfg.Target)
	for k, v := range cfg.Defaults.m {
		cfg.Defaults.m[k] = expandStr(v)
	}
	for i := range cfg.Scenario {
		cfg.Scenario[i].Attack = expandStr(cfg.Scenario[i].Attack)
		for k, v := range cfg.Scenario[i].Flags {
			cfg.Scenario[i].Flags[k] = expandStr(v)
		}
	}
}

// Load reads and parses a YAML config file, expands env vars, and validates basic structure.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	expand(&cfg)

	if cfg.Target == "" {
		return nil, fmt.Errorf("config %q: missing required field 'target'", path)
	}
	if len(cfg.Scenario) == 0 {
		return nil, fmt.Errorf("config %q: scenario has no steps", path)
	}
	return &cfg, nil
}

// Scaffold returns a starter limithit.yaml template.
func Scaffold() string {
	return `# limithit scenario — edit to match your target
target: ${TARGET_URL}

defaults:
  concurrency: 20
  output: table

scenario:
  - attack: flood
    total: 1000
    expect-status: 429

  - attack: slowloris
    connections: 50
    hold: 15

  - attack: fuzz
    cache-bust: true
    total: 500
`
}

func anyToStr(v interface{}) string {
	switch x := v.(type) {
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprintf("%v", v)
	}
}
