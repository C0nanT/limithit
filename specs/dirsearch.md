# Spec: `dirsearch` attack

Content/path discovery probe. Walks a wordlist against a target, sends **one request per
entry**, classifies each response, and reports the paths that exist. Wordlist source:
[SecLists](https://github.com/danielmiessler/seclists) (`Discovery/Web-Content/*`).

Status: **proposed**. This document defines behavior + contract; no code yet.

---

## 1. Why a new attack (not part of `fuzz`)

`fuzz` and `dirsearch` look similar but have opposite goals:

| | `fuzz` (existing) | `dirsearch` (this spec) |
|---|---|---|
| Goal | Stress rate-limit / cache with path variety | Discover which paths actually exist |
| Wordlist traversal | Round-robin `Next()`, cycles forever | Each entry exactly **once** |
| Request count | `--total` (independent of list size) | `wordlist_size × len(extensions+1)` |
| Cache | Cache-busts to bypass | Should hit cache normally |
| Output | Aggregate status distribution | **Per-path findings list** (status, size, redirect) |
| Noise handling | None | Soft-404 / wildcard baseline filtering |

They share infra (worker pool, client, wordlist loader, pacer, metrics) but the result
model and traversal differ enough to justify a separate package. Do **not** fold this into
`fuzz`.

---

## 2. Package layout

```
internal/attacks/dirsearch/
  dirsearch.go        # Options + Run(ctx, opts) (*Report, error)
  dirsearch_test.go   # matcher + baseline + extension-expansion tests
```

Follows the existing convention: self-contained package, `Options` struct, `Run` returning
a report + error. CLI wiring lives in `internal/cli/`.

---

## 3. Options

```go
type Options struct {
    BaseURL     string        // scheme://host[:port] — no path (like fuzz)
    WordlistPth string        // REQUIRED for real use; path to a SecLists file
    Extensions  []string      // e.g. ["php","html","bak"] — appended per word
    Method      string        // GET (default) or HEAD
    Headers     http.Header
    Timeout     time.Duration
    Concurrency int

    // Matching / filtering
    IncludeStatus []int       // statuses counted as "found" (default below)
    ExcludeStatus []int       // statuses explicitly dropped
    FilterSizes   []int64     // response sizes (bytes) to drop as noise

    // Noise control
    DetectWildcard bool       // default true — soft-404 baseline probe
    FollowRedirect bool       // default false — report 3xx instead of chasing

    // Rate-limit behavior (ties into project theme)
    Pacing   string           // none|uniform|poisson|zipf (reuse metrics.Pacer)
    MinDelay time.Duration
    MaxDelay time.Duration
    RPS      float64
    Backoff  bool             // default true — on 429, slow down (see §7)

    // Recursion (phase 2, may ship disabled)
    RecursionDepth int        // 0 = off; recurse into discovered dirs
}
```

Default `IncludeStatus`: `200, 204, 301, 302, 307, 401, 403, 405`.
`404` is never "found". `429` is throttle signal, not a finding (see §7).

---

## 4. Traversal & request building

1. Load wordlist via `metrics.LoadWordlist(opts.WordlistPth)`. Reuse existing loader —
   it already strips comments/blank lines and prepends `/`. SecLists entries are bare
   words (`admin`, `.git/HEAD`), so the leading-`/` normalization is exactly what's needed.
   - `WordlistPth` is effectively required; fall back to `metrics.DefaultWordlist()` only if
     empty, and warn that the embedded 104-path list is a smoke-test list, not discovery.
2. **Expand extensions**: for each entry `/foo`, emit `/foo` plus `/foo.<ext>` for every
   ext in `Extensions`. An entry already ending in `/` (directory) is not extension-expanded.
   Build the full candidate list up front; `Total = len(candidates)`.
3. Drive via the existing `worker.Run`. The `RequestBuilder` indexes the candidate slice by
   `idx` (do **not** use `Wordlist.Next()` round-robin — each candidate must fire once).
   - `Total = len(candidates)`, so the producer emits each index exactly once.
4. `path` return value of the builder = the candidate path, so per-path results land in the
   collector's `pathStatus` map.

---

## 5. Capturing response size (infra change required)

Current `client.Result` and `worker.doRequest` discard the body and record only status +
duration. dirsearch needs **response size** for soft-404 filtering and the findings report.

Required change (smallest viable):
- Add `Size int64` to `client.Result`.
- In `worker.doRequest`, capture bytes from the `io.Copy(io.Discard, resp.Body)` (it already
  copies — use `io.Copy`'s returned count instead of discarding it) and set `Result.Size`.
  Also capture `resp.Header.Get("Location")` into a new `Redirect string` field when status
  is 3xx.
- Thread `Size` (and optional redirect) through `Collector.RecordPath`. Extend the per-path
  bucket from `map[int]int` to also retain a representative size + redirect per (path,status).

This is additive and must not change existing attacks' behavior (they ignore the new fields).

Alternative if touching the shared worker is undesirable: dirsearch ships its **own** small
runner over the worker pattern. Prefer extending the shared path — it keeps one code path and
benefits other attacks. Decide at implementation time; note the choice in the PR.

---

## 6. Soft-404 / wildcard detection

Many servers return `200` (or a styled error page) for non-existent paths, producing false
positives. Before the main run, when `DetectWildcard` is true:

1. Send N (default 3) requests to random non-existent paths
   (e.g. `/<16-hex>`, `/<16-hex>.<firstExt>`).
2. Record the baseline: `(status, size)` of those responses.
3. If baselines are consistent (same status, size within a small tolerance), treat that
   `(status, ~size)` signature as **noise**: any candidate matching it is dropped from
   findings (but still counted in aggregate stats).
4. If baselines are 404/inconsistent, no wildcard — proceed normally.

Report the detected baseline in the summary so the user knows filtering was applied.

---

## 7. Rate-limit interaction (project-specific value)

This toolkit is about rate limits, so dirsearch must behave well against a limiter and
surface what it learns:

- Reuse `metrics.Pacer` (uniform/poisson/zipf/none) exactly like `spoof`.
- A `429` is **not** a finding and **not** an error — it means "couldn't probe this path,
  limiter blocked it." Track `Throttled` count separately.
- When `Backoff` is true and 429 rate climbs, increase inter-request delay (simple multiplicative
  backoff) so the scan can still complete instead of getting 429s for everything.
- Summary must report: how many candidates were throttled (i.e. the scan's reliability),
  so the user can tell "path not found" apart from "never actually got tested."

---

## 8. Report / output

Aggregate stats reuse `metrics.Report` (Sent / status distribution / RPS / duration). dirsearch
adds a **findings** view on top. Either extend `metrics.Report` with a `Findings []Finding`
field or wrap it in a dirsearch-local report. A finding:

```go
type Finding struct {
    Path     string
    Status   int
    Size     int64
    Redirect string // Location header, when 3xx
}
```

Console output (after the standard summary block):

```
=== limithit dirsearch (candidates=4520 throttled=0) summary ===
Sent:         4520
...status distribution...
Wildcard:     none detected

Findings (37):
  200    1542B  /admin/login
  301      0B   /api            -> /api/
  403      13B  /.git/
  401     219B  /api/v1/admin
  200    8841B  /robots.txt
```

Sort findings by status then path. Drop anything matching the wildcard baseline or
`FilterSizes`. Cap printed findings (e.g. top 200) with a "+N more" note, mirroring the
per-path cap already in `metrics.Report.String()`.

---

## 9. Redirect handling

Default `FollowRedirect=false`: discovery wants to *see* the 301/302, not chase it. The shared
`client.New` uses Go's default redirect policy (follows up to 10). Add a knob so dirsearch's
client sets `CheckRedirect = func(...) error { return http.ErrUseLastResponse }`. Implement as
a `NoFollowRedirect bool` in `client.Config` (additive; other attacks keep following).

---

## 10. Recursion (phase 2 — optional)

When `RecursionDepth > 0`: after a pass, any finding that is a directory (status 301/200 with
path ending `/`, or a 301 whose `Location` adds a trailing slash) becomes a new base prefix.
Re-run the wordlist under that prefix, up to `RecursionDepth` levels. Guard against loops and
explosion (cap total candidates). Ship phase 1 with this disabled if time-constrained.

---

## 11. CLI integration

- `internal/cli/cli.go`:
  - add `case "dirsearch": return runDirsearch(...)` to the dispatch switch.
  - add a line to `printRoot`: `dirsearch    Content/path discovery from a wordlist (SecLists)`.
  - implement `runDirsearch` mirroring `runFuzz`: position-agnostic URL via `extractURLArg`,
    flags below.
- `internal/cli/interactive.go`:
  - add option to the attack `huh.Select`.
  - add `interactiveDirsearch` form (URL, wordlist path, extensions, method, concurrency,
    timeout, follow-redirect confirm, pacing).

### Flags

| Flag | Type | Default | Notes |
|---|---|---|---|
| `--url` / positional | string | — | base URL, scheme+host |
| `--wordlist` | string | — | SecLists file path (required in practice) |
| `--extensions` | string | "" | comma list, e.g. `php,html,bak` |
| `--method` | string | GET | GET or HEAD only |
| `--concurrency` | int | 20 | |
| `--timeout` | int (s) | 10 | |
| `--include-status` | string | `200,204,301,302,307,401,403,405` | comma list |
| `--exclude-status` | string | "" | comma list |
| `--filter-size` | string | "" | comma list of byte sizes to drop |
| `--no-wildcard-detect` | bool | false | disables §6 |
| `--follow-redirect` | bool | false | |
| `--pacing` | string | none | uniform\|poisson\|zipf\|none |
| `--min-delay-ms` / `--max-delay-ms` | int | 0 / 50 | uniform/zipf |
| `--rps` | float | 50 | poisson |
| `--backoff` | bool | true | 429 backoff (§7) |
| `--recursion-depth` | int | 0 | phase 2 |
| `--header` | repeatable | — | `Key: Value` |

Example:

```bash
./limithit dirsearch http://localhost:8080 \
  --wordlist ~/seclists/Discovery/Web-Content/common.txt \
  --extensions php,bak,old --concurrency 30 --pacing uniform --max-delay-ms 20
```

---

## 12. Validation & errors

- Reuse `validateURL`; require scheme+host, reject a URL with a path (like `fuzz` semantics).
- `--method` restricted to GET/HEAD (extend or reuse `validateMethod`, then reject others).
- Empty/missing wordlist file → error (after the "use a real list" warning for the fallback).
- Parse `--include-status` / `--exclude-status` / `--filter-size` / `--extensions` with clear
  errors on malformed comma lists.

---

## 13. Tests

`internal/attacks/dirsearch/dirsearch_test.go`:
- **extension expansion**: `/foo` + `[php,bak]` → `[/foo,/foo.php,/foo.bak]`; dir entry `/x/`
  not expanded.
- **matcher**: include/exclude status + size-filter logic, table-driven.
- **wildcard baseline**: given mocked baseline responses, a candidate matching the signature is
  dropped; a differing one is kept.
- **throttle accounting**: 429 increments `Throttled`, not findings/errors.

Integration against `testserver` (manual / optional CI): known paths (`/api/ping`, `/api/echo`,
`/dashboard`) appear as findings; random paths do not. Use `--rate` high enough to avoid 429,
or assert that throttled count is surfaced.

If the shared `client`/`worker`/`metrics` changes land (§5, §9), add/extend their existing
tests to cover `Size` capture and the no-follow client option.

---

## 14. Docs

- `CLAUDE.md`: add a `dirsearch` run example under Commands and a bullet under the root-module
  attack list. Note SecLists as the wordlist source (not vendored — user supplies the path).
- `README.md`: add `dirsearch` to the attack table/sections.
- Do **not** vendor SecLists into the repo (large, licensing). Document the expected path and
  recommend `Discovery/Web-Content/common.txt` or `directory-list-2.3-medium.txt`.

---

## 15. Out of scope (for v1)

- Vendoring or embedding SecLists.
- Response-body content matching / tech fingerprinting / diffing (belongs to a recon tool).
- Auth flows beyond static `--header` (no login/session handling).
- Crawling/spidering links (this is wordlist-driven only).
