# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build -o ratelash .
cd testserver && go build ./...

# Format (required before commit)
gofmt -w .
gofmt -w testserver/

# Vet
go vet ./...
cd testserver && go vet ./...

# Run testserver (local target)
cd testserver && go run . --rate 5 --burst 5

# Run testserver with XFF trust (enables spoof testing)
cd testserver && go run . --rate 5 --burst 5 --trust-xff-cidr 127.0.0.0/8

# Run attacks
./ratelash flood http://localhost:8080/api/ping --total 200 --concurrency 20
./ratelash slowloris http://localhost:8080 --connections 50 --hold 30
./ratelash spoof http://localhost:8080/api/ping --ip-pool 10.0.0.0/28 --total 200
./ratelash fuzz http://localhost:8080 --cache-bust --total 200
./ratelash headerbomb http://localhost:8080/api/echo --header-count 100 --header-size 256
```

Git hooks in `.githooks/` run `gofmt` on commit and `build + vet + gofmt` on push — both root and `testserver/` module.

## Architecture

Two independent Go modules:

**Root module** (`github.com/conantorreswf/ratelash`) — the CLI attacker:
- `main.go` → `internal/cli.Run()` → dispatches subcommands or launches interactive TUI (bubbletea/huh) when called with no args
- `internal/cli/cli.go` — one `runX()` per attack; URL can be positional or `--url` flag
- `internal/attacks/<name>/` — each attack is a self-contained package with an `Options` struct and `Run(ctx, opts) → Report`
- `internal/worker/worker.go` — generic worker pool; attacks send jobs on a channel, pool runs them with bounded concurrency
- `internal/metrics/` — thread-safe collectors: `metrics.go` (HTTP stats), `connreport.go` (slowloris), `pacer.go` (uniform/poisson/zipf delays), `ippool.go` (CIDR/file/list expansion), `wordlist.go` + embedded `paths.txt`
- `internal/client/client.go` — shared `http.Client` with transport tuning

**`testserver/` module** — the local target with hardening and live dashboard:
- `main.go` — wires HTTP server with `ReadHeaderTimeout:5s`, `IdleTimeout:30s`, `MaxHeaderBytes:16KB`; parses `--rate`, `--burst`, `--trust-xff-cidr`
- `ratelimit/ratelimit.go` — per-IP token bucket registry; `ClientIP()` resolves real IP vs XFF based on trust list
- `store/store.go` — atomic counters + ring buffer for top-offender tracking
- `handler/` — `api.go` (ping/echo), `auth.go` (lockout after 5 failures/min, 5min block), `dashboard.go`, `metrics_sse.go` (SSE stream at 500ms)
- `dashboard/index.html` — real-time dashboard consuming SSE

## Key design decisions

- **URL position-agnostic**: `extractURLArg` in `cli/common.go` strips the URL from args before `flag.Parse`, so `--flag value <url>` and `<url> --flag value` both work.
- **Spoof bypass condition**: spoof only bypasses per-IP rate limit when `--trust-xff-cidr` includes the attacker's real IP. Without it, all traffic shares one bucket (real `RemoteAddr`).
- **Slowloris detection**: `DroppedByServer > 0` means the server actively closed the connection (protected). `DroppedByServer = 0` with `AvgHold ≈ --hold` means the server is vulnerable.
- **No test files exist yet** — `future.md` lists planned attacks (HTTP/2 flood, WebSocket exhaustion, gzip bomb, etc.).
