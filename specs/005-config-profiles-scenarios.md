# 005 — Config, profiles & scenarios

**Status:** proposed
**Theme:** UX
**Depends on:** 001 (registry + common flags)

## Motivation

Real testing means running several attacks in sequence with consistent settings against
one target. Re-typing flags is error-prone. This spec adds config files and multi-step
scenarios so a full assessment is one reproducible artifact.

## Goals

- A config file (`limithit.yaml` / `.toml`) holding target + default flags, overridable
  by CLI flags (flag > env > config > built-in default precedence).
- Named **scenarios**: an ordered list of attack steps with per-step options, run via
  `limithit run scenario.yaml`. Aggregate report at the end.
- Environment-variable expansion in config (`${TARGET_URL}`) for CI secrets.
- A `limithit init` command that scaffolds a starter config.

## Design

### Config schema (YAML)

```yaml
target: ${TARGET_URL}
defaults:
  concurrency: 20
  rate: 50
  output: json
scenario:
  - attack: flood
    total: 2000
    expect-status: 429
  - attack: slowloris
    connections: 100
    hold: 30s
  - attack: spoof
    ip-pool: 10.0.0.0/24
    total: 500
```

### Loading

- `internal/config/config.go`: load → merge defaults → expand env → validate against the
  attack registry (unknown attack or flag = error before any traffic is sent).
- Each step maps onto the 001 `Attack.Flags` set; reuse the same parser so config and CLI
  can't drift.

### Scenario runner

- `internal/scenario/runner.go`: run steps sequentially (parallel later), collect each
  `Report`, emit a combined report (ties into 003 JSON output).
- `--continue-on-fail` vs default fail-fast on assertion failure.

## Tasks

1. Add config package with precedence merge + env expansion (+tests).
2. Add `limithit init` scaffold command.
3. Add `limithit run <file>` scenario runner; combined report via 003 renderers.
4. Validate config against registry before execution.
5. Document config format in CLAUDE.md + an example `examples/scenario.yaml`.

## Acceptance

- A scenario file runs all steps against the testserver and emits one combined report.
- Unknown attack/flag in config fails fast with a clear error, no traffic sent.
- CLI flag overrides config value; env var fills `${...}` placeholders.
