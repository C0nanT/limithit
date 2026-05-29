# limithit — Improvement Specs

Each spec is self-contained and can be picked up independently. Specs are ordered
roughly by foundational dependency (lower numbers unblock higher ones) but most are
parallelizable.

| # | Spec | Theme | Depends on |
|---|------|-------|-----------|
| 001 | [Architecture refactor](001-architecture-refactor.md) | Core | — |
| 002 | [Attack engine & new attacks](002-attack-engine-and-new-attacks.md) | Features | 001 |
| 003 | [Reporting & observability](003-reporting-observability.md) | Features | 001 |
| 004 | [Testserver hardening & realism](004-testserver-hardening.md) | Target | — |
| 005 | [Config, profiles & scenarios](005-config-profiles-scenarios.md) | UX | 001 |
| 006 | [Testing, CI & quality gates](006-testing-ci-quality.md) | Quality | — |
| 007 | [TUI & CLI UX](007-tui-cli-ux.md) | UX | 003 |
| 008 | [Safety, ethics & distribution](008-safety-distribution.md) | Ops | — |

## Conventions for all specs

- Two modules stay separate: root (`github.com/conantorreswf/limithit`, the attacker)
  and `testserver/` (the target). No cross-imports.
- Every attack keeps the `Options` struct + `Run(ctx, opts) → Report` contract.
- `gofmt -w`, `go vet ./...` clean before any commit (both modules).
- New behavior ships with tests in the matching package.

## Status legend

`proposed` → `accepted` → `in-progress` → `done`. All specs below start `proposed`.
