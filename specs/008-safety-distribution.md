# 008 — Safety, ethics & distribution

**Status:** proposed
**Theme:** Ops
**Depends on:** —

## Motivation

limithit sends real attack traffic. To stay a legitimate authorized-testing tool (and
to be safely shareable), it needs explicit authorization guardrails, sane defaults, and
proper release artifacts. This also covers the amplification attacks from 002 (gzipbomb)
that must not run accidentally.

## Goals

- Authorization affirmation: destructive/amplifying attacks require an explicit
  `--i-understand` (or a config `authorized: true`) before sending traffic.
- Target allowlist / loopback-default: in the absence of config, warn loudly when the
  target is not localhost; optional `--allow-target` / allowlist file for CI.
- Built-in rate ceilings and a global `--max-rps` safety cap to prevent fat-finger
  floods; require an override flag to exceed it.
- Clear, non-evasive identification: default `User-Agent` includes `limithit/<version>`
  so blue teams can identify test traffic (overridable for spoofing tests, which are the
  legitimate exception).
- Release engineering: versioned builds, checksums, reproducible binaries.

## Design

### Authorization layer

- `internal/safety/guard.go`: `Confirm(attack, target, opts) error`. Called by the
  harness before any traffic. Blocks amplification attacks without affirmation; warns on
  non-loopback targets; enforces `--max-rps`.
- Affirmation can be satisfied non-interactively for CI via env/config (documented).

### Versioning & release

- Inject version via `-ldflags "-X main.version=..."`; `limithit --version`.
- GoReleaser config producing multi-arch binaries + `SHA256SUMS`.
- A `SECURITY.md` and a usage/ethics section in the README stating authorized-use-only.

### Logging

- Optional `--audit-log path` writing a JSONL record of every run (target, attack,
  flags, operator, timestamp) for engagement record-keeping.

## Tasks

1. Add `internal/safety` guard; invoke from harness (001) before traffic.
2. Add `--i-understand`, `--max-rps`, `--allow-target` flags + non-loopback warning.
3. Set identifying default `User-Agent`; keep spoof/header attacks able to override.
4. Add version injection + `--version`; wire into Makefile.
5. Add GoReleaser config + checksums; document install.
6. Add `--audit-log` JSONL output.
7. Write `SECURITY.md` + README ethics/authorized-use section.

## Acceptance

- gzipbomb and other amplifying attacks refuse to run without affirmation.
- Non-loopback target without `--allow-target` prints a clear warning.
- Exceeding `--max-rps` without override is blocked.
- `limithit --version` reports the injected version; release produces signed checksums.
