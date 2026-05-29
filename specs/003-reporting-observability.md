# 003 — Reporting & observability

**Status:** proposed
**Theme:** Features
**Depends on:** 001 (uniform Report contract helps)

## Motivation

Today a `metrics.Report` is printed as a human summary. To use limithit in CI, to
compare runs, and to feed dashboards, reports need structured output, latency
percentiles, and machine-readable export.

## Goals

- Latency histogram + percentiles (p50/p90/p95/p99/max) in every report.
- Status-code breakdown map (e.g. `{200: 1421, 429: 579, 503: 12}`).
- Throughput over time (per-second buckets) so ramp/threshold runs are interpretable.
- Output formats: `--output table|json|csv` and `--output-file path`.
- A `--compare baseline.json` mode that diffs the current run vs a saved baseline and
  flags regressions (e.g. p99 up >X%, 429 ratio dropped).

## Design

### Report struct extensions

```go
type Report struct {
    // existing fields ...
    Latency      LatencyStats          // p50,p90,p95,p99,max,mean
    ByStatus     map[int]int64
    PerSecond    []Bucket              // {SecondOffset, Sent, OK, RateLimited, Errors}
    StartedAt    time.Time
    Duration     time.Duration
    Attack       string
    Target       string
}
```

### Latency capture

Collector records each request latency (the current `flood` already computes
`time.Since(start)` but discards it — wire it into the collector). Use an HdrHistogram-
style bucketed structure or a simple sorted-slice percentile if memory is bounded by
`--total`. Keep it lock-light (per-worker buffers merged at end).

### Serialization

- `internal/report/render.go`: `Table(w)`, `JSON(w)`, `CSV(w)`.
- JSON is the canonical interchange format; `--compare` reads it back.

### Live metrics (optional, behind flag)

`--live` renders an in-place updating summary during long runs (reuse charmbracelet
deps already in the root module). Off by default to keep piped output clean.

## Tasks

1. Extend `metrics.Collector` to capture per-request latency + status into buckets.
2. Add `LatencyStats` percentile computation (+test with known input).
3. Add `internal/report` package with table/json/csv renderers (+golden tests).
4. Wire `--output`, `--output-file` flags into the common flag set (001).
5. Implement `--compare` diff with configurable thresholds; exit non-zero on regression.
6. Update existing attacks to populate `Attack`/`Target`/`StartedAt`.

## Acceptance

- `limithit flood ... --output json` emits valid JSON with non-empty `Latency` and
  `ByStatus`.
- Percentile math verified against a fixed dataset in tests.
- `--compare` exits non-zero when p99 regresses beyond threshold; zero otherwise.
- CSV opens cleanly in a spreadsheet; one row per per-second bucket.
