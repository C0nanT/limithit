# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build (or: make build / make all)
go build -o limithit .
cd testserver && go build ./...

# Format — required before commit (or: make fmt)
gofmt -w .
gofmt -w testserver/

# Vet (or: make vet)
go vet ./...
cd testserver && go vet ./...

# Test — root module
go test ./internal/...

# Test — single package or test
go test ./internal/metrics/... -run TestIPPool
cd testserver && go test ./ratelimit/... -run TestSlidingWindow

# Test — testserver module
cd testserver && go test ./...

# Run testserver (local target)
cd testserver && go run . --rate 5 --burst 5

# Run testserver with XFF trust (enables spoof testing)
cd testserver && go run . --rate 5 --burst 5 --trust-xff-cidr 127.0.0.0/8

# Run testserver with alternate rate-limit algorithm
cd testserver && go run . --rate 5 --burst 5 --algo slidingwindow

# Run attacks
./limithit flood http://localhost:8080/api/ping --total 200 --concurrency 20
./limithit slowloris http://localhost:8080 --connections 50 --hold 30
./limithit spoof http://localhost:8080/api/ping --ip-pool 10.0.0.0/28 --total 200
./limithit fuzz http://localhost:8080 --cache-bust --total 200
./limithit headerbomb http://localhost:8080/api/echo --header-count 100 --header-size 256
./limithit h2flood https://localhost:8443/api/ping --total 200
./limithit wsflood ws://localhost:8080/ws --total 100
./limithit gzipbomb http://localhost:8080/api/echo --i-understand
./limithit methodspray http://localhost:8080/api/ping --total 200
./limithit replay http://localhost:8080/api/ping --total 200

# Run scenario from YAML config
./limithit scenario run examples/scenario.yaml
./limithit scenario run examples/scenario.yaml --continue-on-fail
./limithit scenario validate examples/scenario.yaml

# Scaffold a new scenario config
./limithit scenario init > limithit.yaml
```

Git hooks in `.githooks/` run `gofmt` on commit and `build + vet + gofmt` on push — both root and `testserver/` module.

## Architecture

Two independent Go modules:

**Root module** (`github.com/conantorreswf/limithit`) — the CLI attacker (has TUI deps: bubbletea, huh, charmbracelet):
- `main.go` → `internal/cli.Run()` → dispatches subcommands or launches interactive TUI (bubbletea/huh) when called with no args
- `internal/cli/cli.go` — one `runX()` per attack; URL can be positional or `--url` flag
- `internal/attacks/<name>/` — each attack implements the `Attack` interface (`Name`, `Synopsis`, `Flags`, `Validate`, `Run`, `FormFields`); registers itself via `attacks.Register` in `init()`
- `internal/attacks/all/all.go` — blank imports every attack package to trigger registrations; imported by scenario runner and TUI
- `internal/attacks/registry.go` — `Register` / `Lookup` / `All`; `All()` returns sorted slice for TUI menus
- `internal/worker/worker.go` — generic worker pool; attacks send jobs on a channel, pool runs them with bounded concurrency
- `internal/metrics/` — thread-safe collectors: `metrics.go` (HTTP stats), `connreport.go` (slowloris), `pacer.go` (uniform/poisson/zipf delays), `ippool.go` (CIDR/file/list expansion), `wordlist.go` + embedded `paths.txt`, `progress.go` (live `Progress` snapshots via `Base.ProgressCh`)
- `internal/client/client.go` — shared `http.Client` with transport tuning
- `internal/config/config.go` — YAML scenario config (`limithit.yaml`): `Load`, `Scaffold`; env vars expand as `${VAR}`; `Defaults` apply to all steps, step flags override
- `internal/scenario/runner.go` — `Validate` (dry-run flag parse against registry) + `Run` (sequential step execution); `expect-status` assertions; JSON/table summary; `--continue-on-fail`
- `internal/safety/guard.go` — `safety.Confirm` pre-flight checks: `--i-understand` required for amplification attacks (gzipbomb); warns on non-localhost targets unless `--allow-target <host>` suppresses it

**`testserver/` module** — the local target with hardening and live dashboard:
- `main.go` — wires HTTP server with `ReadHeaderTimeout:5s`, `IdleTimeout:30s`, `MaxHeaderBytes:16KB`; parses `--rate`, `--burst`, `--trust-xff-cidr`, `--algo`
- `ratelimit/limiter.go` — `Limiter` interface (`Allow`, `Capacity`); factory `NewLimiter(algo, rate, burst)`; algorithms: `tokenbucket` (default), `fixedwindow`, `slidingwindow`, `leakybucket`
- `ratelimit/ratelimit.go` — per-IP registry wrapping any `Limiter`; `ClientIP()` resolves real IP vs XFF
- `store/store.go` — atomic counters + ring buffer for top-offender tracking
- `handler/` — `api.go` (ping/echo), `auth.go` (lockout after 5 failures/min, 5min block), `gzip.go` (gzip bomb target endpoint), `ws.go` (WebSocket flood target), `dashboard.go`, `metrics_sse.go` (SSE stream at 500ms)
- `dashboard/index.html` — real-time dashboard consuming SSE

## Key design decisions

- **Attack registry**: each attack calls `attacks.Register(name, ctor)` in `init()`; `attacks/all` aggregates them. Adding a new attack = new package + `Register` call + blank import in `all/all.go`.
- **URL position-agnostic**: `extractURLArg` in `cli/common.go` strips the URL from args before `flag.Parse`, so `--flag value <url>` and `<url> --flag value` both work.
- **Spoof bypass condition**: spoof only bypasses per-IP rate limit when `--trust-xff-cidr` includes the attacker's real IP. Without it, all traffic shares one bucket (real `RemoteAddr`).
- **Slowloris detection**: `DroppedByServer > 0` means the server actively closed the connection (protected). `DroppedByServer = 0` with `AvgHold ≈ --hold` means the server is vulnerable.
- **Safety guard**: `gzipbomb` is flagged as an amplification attack and requires `--i-understand`; non-localhost targets always warn unless suppressed with `--allow-target`.
- **Scenario `expect-status`**: asserts that at least one response with that HTTP status was observed in a step; only meaningful for attacks returning `*metrics.Report`.
- **Tests exist** in `internal/metrics/` (ippool, pacer, wordlist), `internal/config/`, `internal/attacks/` (registry, integration), and `testserver/` (ratelimit algorithms, auth).
