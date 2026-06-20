# Testing guide

Streamclone checks range from sub-minute unit tests to multi-minute benchmarks. Prefer **narrow, domain-specific** runs while iterating; use **`make check-quick`** before most PRs and **`make check`** before merge-worthy changes.

Skill shortcut: `.cursor/skills/streamclone/test-by-domain/SKILL.md`.

---

## Quick checks (agents / local iteration)

```sh
make check-quick          # Go test+vet, frontend tests, compose config
make test                 # all Go packages
make vet                  # go vet ./...
make test-video           # playback only
make test-analytics       # analytics / rollups only
make test-emote           # emote pipeline only
make test-storygraph      # Pulse Wire backend only
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

Pulse Wire UI fixtures: `frontend/tests/fixtures/pulseWireFixtures.ts`, `frontend/tests/playwright/pulse-wire.spec.ts`.

---

## Backend-only

```sh
make test
make vet
go test ./internal/video/...         # playback
go test ./internal/analytics/...     # analytics / rollups
go test ./internal/storygraph/...    # Pulse Wire
go test ./internal/emote/...         # emote pipeline
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
make validate-env PROFILE=full
bash scripts/validate-env.sh --profile scraper
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

Manual benchmark (Windows script; stack must be up with a live channel):

```powershell
.\scripts\measure-hls-latency.ps1 -Channel sodapoppin
```

Artifacts write to the ignored `docs/benchmarks/` folder. Methodology and guardrails live in `docs/low-latency-relay/requirements.md`.

Steering: `.kiro/steering/playback.md`.

---

## Scraper benchmark

Requires scraper profile or sibling repo image:

```sh
make up-scraper
make scraper-preflight
make scraper-proxy-benchmark         # direct vs proxy matrix
make scraper-turnstile-benchmark     # Camoufox / challenge fallback
make social-probe                    # Reddit + X ingest probes
make flame-proxy-preflight         # Flame API balance (needs PROXY_API_KEY)
```

Docs: `docs/scraping-archive/requirements.md`, `docs/scraper-cloudflare-and-proxy.md`.

---

## Full PR gate

```sh
make check-quick     # daily / agent default
make check           # full: security-scan, frontend build+audit+test, clipper-test, go build
make install-hooks   # pre-commit (optional)
pre-commit run check-quick-light --all-files   # opt-in lightweight agent checks
pre-commit run --all-files
```

| Target | Includes |
|--------|----------|
| `check-quick` | `test`, `vet`, `frontend-test`, `compose-config-check` |
| `check` | above + `security-scan`, `frontend-build`, `frontend-audit`, `clipper-test`, `build` |

**CI:** pull requests run `make check-quick`; pushes to `main`/`master` run `make check` plus compose smoke (see `.github/workflows/ci.yml`). Code graph rebuild runs when `internal/`, `frontend/src/`, or `cmd/` change.

Security-sensitive changes (auth, compose secrets, public deploy): always add `make security-scan` even if using `check-quick`.

---

## What to run by changed files

| Changed area | Minimum | Before PR |
|--------------|---------|-----------|
| `internal/*`, `cmd/*` (shared) | `make test`, `make vet` | `make check-quick` |
| `internal/video/*`, `frontend/src/playback.ts`, channel player | `make test-video`, `npm test` | `make check-quick`, `playback_probe` |
| `internal/analytics/*` | `make test-analytics` | `make check-quick` |
| `internal/storygraph/*`, `internal/social/*`, Pulse Wire UI | `make test-storygraph`, `npm test` | `make check-quick`, pulse-wire Playwright if UI |
| `internal/emote/*`, emote frontend | `make test-emote` | `make check-quick`, `emote_jobs` MCP |
| `internal/metadata/*` | `make test-metadata` | `make check-quick` |
| `frontend/src/**` only | `npm test` or `npm run build` | `make check-quick` |
| `deploy/*`, Caddy, compose | `make compose-config-check` | + `make security-scan`, `make smoke` |
| `.env.example`, profile env | `make validate-env` | `compose-config-check` |
| `docs/**` only | `git diff --check -- '*.md'` | — |
| Scraper / proxy scripts | `make scraper-preflight` | benchmark docs updated if results change |
| Install / bootstrap scripts | manual `make bootstrap` or installer test | append row to `docs/repo-maintenance.md` |

---

## Docs / screenshots (optional)

```sh
make docs-screenshots    # README Playwright captures
make docs-media          # docs/media/*.webm assets
```

These are slow and need a healthy stack; not part of `check-quick`.
