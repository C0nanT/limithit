# 002 — Attack engine & new attacks

**Status:** proposed
**Theme:** Features
**Depends on:** 001 (attack registry + shared harness)

## Motivation

`future.md` listed planned attacks but is now empty. With the registry from 001,
new attacks are cheap. This spec defines the next attack set plus engine features they
share (ramp profiles, keep-alive control, response assertions).

## Engine features (shared, build first)

1. **Ramp / load shaping.** Extend `metrics.Pacer` (already does uniform/poisson/zipf)
   with a `--ramp` mode: linearly increase rate from `--rate-start` to `--rate` over
   `--ramp-duration`. Useful for finding the exact RPS where rate-limiting kicks in.
2. **Connection reuse toggle.** `--keepalive=false` forces a new TCP/TLS handshake per
   request (stresses connection setup, defeats keep-alive-based limits). Wire through
   `client.New(opts)`.
3. **Response assertions.** `--expect-status 429` / `--expect-header X-RateLimit-*` so a
   run can *assert* the target rate-limits correctly and exit non-zero if not. Turns the
   tool into a CI-runnable rate-limit regression check.
4. **Threshold detection.** Optional binary-search mode that auto-finds the RPS at which
   the target starts returning 429, reported as `RateLimitThreshold`.

## New attacks

Each is a package under `internal/attacks/<name>/` implementing the 001 `Attack`
interface, with an `Options`/flags set and tests.

### a. `h2flood` — HTTP/2 multiplexed flood
- Open one (or few) HTTP/2 connections, fan out many concurrent streams.
- Flags: `--streams`, `--connections`, `--total`, `--duration`.
- Detects servers with weak per-stream limits / `MaxConcurrentStreams` gaps.

### b. `wsflood` — WebSocket connection exhaustion
- Open and hold N WebSocket connections; optional periodic ping/payload.
- Flags: `--connections`, `--hold`, `--message-rate`.
- Report: `Established`, `Rejected`, `AvgHold` (mirror slowloris connreport).

### c. `gzipbomb` — decompression amplification
- Send small `Content-Encoding: gzip` bodies that expand large server-side.
- Flags: `--ratio`, `--total`. Report whether server rejected (413) or accepted.
- **Safety gate:** require `--i-understand` confirmation (see 008).

### d. `replay` — captured-request replay
- Read a HAR or newline-delimited request file and replay at controlled rate.
- Flags: `--file`, `--rate`, `--loop`. Reuses flood's run loop.

### e. `methodspray` — verb/route matrix
- Cross product of `--methods GET,POST,PUT,...` × wordlist paths to find unprotected
  routes/verbs that skip the rate limiter. Reuses `metrics.Wordlist`.

## Tasks

1. Engine: add ramp mode to pacer (+test), keepalive toggle, assertion layer,
   threshold detection helper.
2. Implement each attack package (a–e) with `init()+Register` and unit tests.
3. Add testserver endpoints/handlers needed to exercise h2/ws (see 004).
4. Document each attack in CLAUDE.md command list + spec acceptance examples.

## Acceptance

- Each new attack runs against the local testserver and produces a populated `Report`.
- `--expect-status 429` exits non-zero when the target fails to rate-limit.
- Ramp mode produces a monotonically increasing send rate (verified in pacer test).
- gzipbomb refuses to run without the safety flag.
