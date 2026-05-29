# 007 — TUI & CLI UX

**Status:** proposed
**Theme:** UX
**Depends on:** 003 (report data for live view), 001 (registry-driven menus)

## Motivation

`internal/cli/tui.go` is the largest file in the root module and hardcodes the attack
list and forms. With the registry (001) and richer reports (003), the TUI can be
generated from metadata and show live progress + percentile results instead of a static
summary.

## Goals

- Registry-driven attack menu and forms — adding an attack needs no TUI edits.
- Live progress during a run: throughput, status breakdown, elapsed/remaining, a small
  latency sparkline (charmbracelet deps already present).
- Post-run results screen showing 003 percentiles + status map, with "export JSON" and
  "run again" actions.
- Better non-TUI ergonomics: `--quiet`, `--no-color`, progress to stderr so stdout stays
  pipe-clean; `-h` per attack lists only that attack's flags.

## Design

### Form generation

Derive huh form fields from each attack's flag definitions (001). Add lightweight flag
metadata (label, help, type, default) so the form and `flag.FlagSet` come from one
declaration. Avoid a second source of truth.

### Live view

The attack run already returns a `Report`; add a progress channel
(`<-chan metrics.Progress`) that the collector emits on. TUI consumes it via a bubbletea
`tea.Cmd`; CLI `--live` (003) renders the same data without the full TUI.

### Results screen

Reuse 003 renderers for the export action. Keymap: `e` export, `r` rerun, `q` quit.

## Tasks

1. Add flag metadata to attack definitions; generate huh forms from it.
2. Replace hardcoded TUI attack list with `attacks.All()`.
3. Add `metrics.Progress` channel + collector emission; render live in TUI.
4. Build results screen consuming 003 `Report`; wire export/rerun.
5. Add `--quiet`, `--no-color`; route progress to stderr.
6. Trim `tui.go` size; extract reusable widgets.

## Acceptance

- New attack appears in the TUI menu with a correct form, zero TUI edits.
- Live throughput/status update during a run in both TUI and `--live` CLI.
- `limithit flood -h` shows flood flags only; `--quiet` produces clean pipeable stdout.
