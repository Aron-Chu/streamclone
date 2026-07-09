# Testing guide

Streamclone checks range from sub-minute unit tests to multi-minute benchmarks. Prefer **narrow, domain-specific** runs while iterating; use **`make check-quick`** before most PRs and **`make check`** before merge-worthy changes.

Skill shortcut: `.cursor/skills/streamclone/test-by-domain/SKILL.md`.

**Scope:** core watch stack (metadata, video, chat, emote, frontend). Legacy `make test-analytics` targets may still exist during the boundary split — do not use them for Streamclone product work. See [streampulse-product-boundary.md](streampulse-product-boundary.md).

---

## Quick checks (agents / local iteration)

```sh
make check-quick          # Go test+vet, frontend tests, compose config, boundary preflight
make product-boundary-preflight   # report-only Step 7 grep hits
make product-boundary-strict      # fail on non-legacy boundary hits (before deletion PR)
make test                 # all Go packages
make vet                  # go vet ./...
make test-video           # playback only
make test-emote           # emote pipeline only
make test-metadata        # directory / metadata API only
cd frontend && npm test   # Vitest unit tests
make compose-config-check # docker compose merge validation
make agent-smoke          # stack_health + smoke (stack must be up)
```

Typical time: a few minutes (no security scan, no full `go build ./...`, no npm audit).

---

## Frontend-only

```sh
cd frontend && npm test              # unit tests (Vitest)
cd frontend && npm run build         # production build / TS check
make frontend-build
make frontend-test
```

Playwright (needs stack + browser):

```sh
make smoke-ui
cd frontend && npx playwright test --config tests/playwright/playwright.config.ts
```

---

## Backend-only (core)

```sh
make test
make vet
go test ./internal/video/...         # playback
go test ./internal/emote/...         # emote pipeline
go test ./internal/metadata/...      # directory / metadata
go test ./internal/chat/...          # chat / auth
make build                           # compile all cmd/*
```

Docker-backed Go (CI parity):

```sh
make go-test-docker
make go-vet-docker
```

Integration (optional, slower):

```sh
make integration-up integration-test integration-down
```

---

## Compose / env checks

```sh
make compose-config-check            # core + release + prod overlays
make validate-env PROFILE=core       # profile env merge
```

After compose or Caddy edits, always run `compose-config-check`.

Auth / env reload smoke:

```sh
make reload-env                      # recreates affected services
make twitch-debug                    # auth + clips probe
```

---

## Docker smoke tests

Requires running stack (`make up`).

```sh
make smoke                           # scripts/smoke-core.sh — API paths
make smoke-ui                        # smoke + Playwright screenshots path
make agent-smoke                     # stack_health snapshot + make smoke
make ports                           # listening port summary
```

MCP (stack up): `stack_health`, `playback_probe`, `twitch_auth_status`.

---

## HLS latency benchmark

```powershell
.\scripts\measure-hls-latency.ps1 -Channel sodapoppin
```

Steering: `.kiro/steering/playback.md`, `docs/low-latency-relay/requirements.md`.

---

## Full PR gate

```sh
make check-quick     # daily / agent default
make check           # full: security-scan, frontend build+audit+test, clipper-test, go build
make install-hooks   # pre-commit (optional)
pre-commit run check-quick-light --all-files
pre-commit run --all-files
```

| Target | Includes |
|--------|----------|
| `check-quick` | `test`, `vet`, `frontend-test`, `compose-config-check` |
| `check` | above + `security-scan`, `frontend-build`, `frontend-audit`, `clipper-test`, `build` |

**CI:** pull requests run `make check-quick`; pushes to `main`/`master` run `make check` plus compose smoke (see `.github/workflows/ci.yml`).

Security-sensitive changes (auth, compose secrets): always add `make security-scan` even if using `check-quick`.

---

## What to run by changed files

| Changed area | Minimum | Before PR |
|--------------|---------|-----------|
| `internal/*`, `cmd/*` (shared) | `make test`, `make vet` | `make check-quick` |
| `internal/video/*`, `frontend/src/playback.ts`, channel player | `make test-video`, `npm test` | `make check-quick`, `playback_probe` |
| `internal/emote/*`, emote frontend | `make test-emote` | `make check-quick`, `emote_jobs` MCP |
| `internal/metadata/*` | `make test-metadata` | `make check-quick` |
| `internal/chat/*` | `go test ./internal/chat/...` | `make check-quick` |
| `frontend/src/**` only | `npm test` or `npm run build` | `make check-quick` |
| `deploy/*`, Caddy, compose | `make compose-config-check` | + `make security-scan`, `make smoke` |
| `.env.example`, profile env | `make validate-env` | `compose-config-check` |
| `docs/**` only | `git diff --check -- '*.md'` | — |
| Install / bootstrap scripts | manual installer test | append row to `docs/repo-maintenance.md` |

StreamPulse backend tests → private **streampulse-backend** checkout.

---

## Docs / screenshots (optional)

```sh
make docs-screenshots    # README Playwright captures
make docs-media          # docs/media/*.webm assets
```

These are slow and need a healthy stack; not part of `check-quick`.
