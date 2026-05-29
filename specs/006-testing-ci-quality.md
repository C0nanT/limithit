# 006 — Testing, CI & quality gates

**Status:** proposed
**Theme:** Quality
**Depends on:** —

## Motivation

Tests exist only in `internal/metrics/` and `testserver/`. Attack packages, the CLI
dispatcher, and the TUI are untested. Git hooks run gofmt/build/vet locally but there is
no CI. This spec raises the safety net so 001–005 can land without regressions.

## Goals

- Integration tests: each attack run against an in-process `httptest` target asserting
  the resulting `Report` shape.
- Coverage for the CLI dispatcher and flag/URL parsing (position-agnostic cases).
- GitHub Actions CI for both modules: build, vet, gofmt-check, test, race detector.
- Lint via `golangci-lint`.
- Coverage reporting with a floor that ratchets up.

## Design

### Test layers

1. **Unit** — keep existing metrics/ratelimit/auth tests; add for new pacer modes,
   percentile math, limiter algorithms.
2. **Attack integration** — spin `httptest.NewServer` (and the real testserver handlers
   where HTTP/2/WS needed), run each attack with small `--total`, assert
   counts/status-breakdown. Table-driven over the attack registry (001) so new attacks
   are auto-covered.
3. **CLI** — drive `cli.Run([]string{...})` and assert exit/parse behavior incl.
   `--flag value <url>` and `<url> --flag value`.

### CI workflow (`.github/workflows/ci.yml`)

```
jobs:
  test:
    strategy.matrix.module: [".", "testserver"]
    steps: checkout → setup-go → go build ./... → go vet ./... →
           gofmt -l (fail if output) → go test -race -cover ./...
  lint:
    golangci-lint run on both modules
```

### Quality gates

- `gofmt -l` must be empty (mirrors the commit hook in CI).
- `-race` on all tests.
- Coverage uploaded as artifact; PR fails if it drops below the recorded floor.

## Tasks

1. Add attack integration test harness (registry-driven).
2. Add CLI dispatcher/parse tests.
3. Add `.github/workflows/ci.yml` with the two-module matrix.
4. Add `.golangci.yml` and fix surfaced lint.
5. Add a `make test-all` / `make ci` target mirroring CI locally.
6. Record initial coverage floor; document the ratchet rule.

## Acceptance

- CI runs on PR and on main; red on fmt/vet/lint/test/race failure.
- Every registered attack has at least one integration test (enforced by the
  table-driven test iterating `attacks.All()`).
- `make ci` reproduces the CI result locally.
