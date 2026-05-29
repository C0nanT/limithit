# 001 — Architecture refactor

**Status:** proposed
**Theme:** Core
**Depends on:** —

## Motivation

The CLI grew organically: `internal/cli/cli.go` has one `runX()` per attack with
hand-rolled flag parsing, and each attack package re-implements the same plumbing
(build client, build pacer, spin worker pool, collect metrics, return report). Adding
an attack means touching the dispatcher, the TUI, and copy-pasting the run loop. This
spec makes attacks pluggable and removes duplication so 002+ are cheap to build.

## Goals

- A single attack registry so the dispatcher, help text, and TUI are generated from
  one source of truth.
- A shared run harness that owns the client/pacer/worker/metrics lifecycle, leaving
  each attack to define only its per-request behavior.
- Uniform flag handling (common flags defined once; attack-specific flags declared
  per attack).
- No behavior change for existing attacks (flood, slowloris, spoof, fuzz, headerbomb).

## Non-goals

- New attacks (see 002).
- Report format changes (see 003).

## Design

### Attack interface

```go
// internal/attacks/attack.go
type Attack interface {
    Name() string
    Synopsis() string
    Flags(fs *flag.FlagSet)        // register attack-specific flags
    Validate() error               // after parse
    Run(ctx context.Context, base Base) metrics.Report
}

// Base carries the shared, already-constructed dependencies.
type Base struct {
    URL    string
    Client *http.Client
    Pacer  *metrics.Pacer
    Common CommonOpts             // total, concurrency, rate, burst, duration, method, body
}
```

Attacks that need raw connections (slowloris, headerbomb) may ignore `Client` and use
`Base` only for `URL`/`Common`.

### Registry

```go
// internal/attacks/registry.go
var registry = map[string]func() Attack{}
func Register(name string, ctor func() Attack)
func Lookup(name string) (Attack, bool)
func All() []Attack
```

Each attack package calls `Register` in `init()`. `cli.go` dispatch becomes:

```go
a, ok := attacks.Lookup(args[0])
if !ok { return usageError(args[0]) }
```

Help text and the TUI attack menu iterate `attacks.All()`.

### Shared harness

Move the client/pacer/worker/metrics wiring out of each attack into a
`harness.Execute(ctx, attack, base)` helper, OR keep `Run` per attack but provide
`worker.RunBounded(ctx, total, concurrency, pacer, fn)` so the loop is one call. Pick
the lighter option during implementation; the worker-helper route is less invasive.

### Flag plumbing

- `internal/cli/common.go` already strips the URL (`extractURLArg`). Keep it.
- Add `registerCommonFlags(fs)` returning a `*CommonOpts`. Each `runX` becomes:
  build flagset → `registerCommonFlags` → `attack.Flags(fs)` → parse → validate → run.
- Collapse the five `runFlood/runSpoof/...` into one generic `runAttack(name, args)`.

## Tasks

1. Add `internal/attacks/attack.go` (interface + `Base`/`CommonOpts`).
2. Add `internal/attacks/registry.go` with tests.
3. Add `worker.RunBounded` helper; cover with a test.
4. Migrate flood → new contract (reference migration); keep old output identical.
5. Migrate slowloris, spoof, fuzz, headerbomb.
6. Replace per-attack `runX` with generic `runAttack`; generate usage from registry.
7. Update TUI to build its menu from `attacks.All()`.
8. Delete dead duplication.

## Acceptance

- `limithit <each existing attack>` produces the same report fields as before.
- Adding a no-op attack requires only a new package with `init()+Register`, no edits to
  `cli.go` or `tui.go`.
- `go test ./...` green in both modules; `go vet` + `gofmt` clean.
