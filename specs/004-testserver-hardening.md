# 004 — Testserver hardening & realism

**Status:** proposed
**Theme:** Target
**Depends on:** —

## Motivation

The `testserver/` target is the lab for validating attacks. To prove detections and to
exercise the new attacks in 002, the target needs more realistic limiting algorithms,
more endpoints, and configurable defenses so each attack has a "vulnerable" and a
"protected" mode to test against.

## Goals

- Pluggable rate-limit algorithms: token bucket (current), fixed window, sliding window,
  leaky bucket. Selectable via `--algo`.
- Standards-compliant rate-limit headers: `RateLimit-Limit`, `RateLimit-Remaining`,
  `RateLimit-Reset`, `Retry-After` on 429. Lets 003 assertions target real headers.
- Endpoints to exercise new attacks: HTTP/2 enabled, a WebSocket echo endpoint, a
  gzip-accepting endpoint with a configurable decompression cap.
- A `--vulnerable` switch that disables specific protections, so each attack can show
  both the failing and passing case.
- Concurrency/connection caps configurable (`--max-conns`, `--max-streams`) to make
  h2flood/wsflood/slowloris outcomes deterministic.

## Design

### Limiter abstraction

```go
// testserver/ratelimit/limiter.go
type Limiter interface {
    Allow(key string) (ok bool, remaining int, reset time.Time)
}
```

Implement `tokenbucket`, `fixedwindow`, `slidingwindow`, `leakybucket`. Registry by name
mirrors the attack registry style. `ClientIP()` keying logic stays unchanged.

### New endpoints

- `/ws/echo` — WebSocket echo (for wsflood). Respect `--max-conns`.
- `/api/gzip` — accepts gzip bodies, decompresses up to `--max-decompress` then 413.
- Enable HTTP/2 (h2c or TLS) on the listener for h2flood; gate behind `--http2`.

### Header middleware

A response middleware that writes the standard RateLimit-* headers from the limiter's
`remaining`/`reset` on every limited route, and `Retry-After` on 429.

### Vulnerable mode

`--vulnerable=slowloris,headerbomb,...` relaxes the matching protection
(`ReadHeaderTimeout`, `MaxHeaderBytes`, lockout, decompress cap) so attacks can
demonstrate impact. Default: all protections on.

## Tasks

1. Introduce `Limiter` interface; refactor current token bucket to implement it; add the
   three other algorithms with tests covering boundary behavior.
2. Add `--algo` flag + limiter registry.
3. Add RateLimit-* / Retry-After header middleware (+test).
4. Add `/ws/echo`, `/api/gzip` handlers; enable optional HTTP/2.
5. Add `--vulnerable` flag mapping to per-protection toggles.
6. Surface active algo + caps on the dashboard.

## Acceptance

- Switching `--algo` changes 429 timing in a way each algorithm's test asserts.
- 429 responses carry `Retry-After`; limited 200s carry `RateLimit-Remaining`.
- `--vulnerable=slowloris` makes the existing slowloris attack report
  `DroppedByServer = 0` with `AvgHold ≈ --hold`; without it, the server drops.
- wsflood and gzipbomb (002) have working target endpoints.
