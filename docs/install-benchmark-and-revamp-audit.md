# Install Benchmark & Revamp Audit

Status snapshot and prioritized recommendations beyond the v0.1.3/v0.1.4 install work. Complements [install-desktop.md](./install-desktop.md) and [scraper-cloudflare-and-proxy.md](./scraper-cloudflare-and-proxy.md).

---

## Status (as of audit)

Not fully benchmark-finished. The install path is much healthier, `VERSION` is pinned to `v0.1.4`, and local image-trim builds completed. Final cold install, cached restart, GHCR pull-size, HLS, and analytics benchmarks remain **blocked** because Docker Desktop’s Linux engine is stopped; `preflight-deps.ps1 -Json` fails in ~20s (docker info timeout) without auto-launching Docker Desktop.

### Run benchmarks when Docker is available

Start Docker Desktop manually (benchmarks **never** auto-launch it), then:

```powershell
powershell -File scripts\preflight-deps.ps1 -InstallHints
powershell -File scripts\benchmark-ghcr-pull.ps1 -ImageTag v0.1.4
powershell -File scripts\benchmark-exe-install.ps1 -SetupExe dist\Streamclone-Setup-v0.1.4.exe
powershell -File scripts\benchmark-restart.ps1
powershell -File scripts\smoke-core.ps1
powershell -File scripts\benchmark-hls-start.ps1
powershell -File scripts\benchmark-analytics-load.ps1 -Login <channel>
```

Record results in the benchmark table below before tagging `v0.1.4`.

### v0.1.4 release gate (core-analytics-truth workstream)

| Gate item | Status | Notes |
|-----------|--------|-------|
| Honest product tiers in docs | **Done** | `install-desktop.md` + `infrastructure-review-brief.md` — Core Watch / Analytics / Clip Studio |
| `profile-core.env` comment truth | **Done** | Helix/VOD + TT summary only; minute charts need scraper profile |
| Analytics empty state (core users) | **Done** | `Analytics.tsx` + `ServiceStatusBanner.tsx` — links to `scraper-cloudflare-and-proxy.md` |
| Setup-control token (frontend) | **Done** | `config.ts` + `api.ts` send `X-Streamclone-Setup-Token`; `setup-control.ps1` validates on POST |
| Setup-control token (backend) | **Done** | `setup-control.ps1` reads `SETUP_CONTROL_TOKEN` from `.env`; 401 on bad/missing header |
| Cold install + smoke benchmark | **Env-blocked** | Docker Desktop Linux engine stopped; use `benchmark-exe-install.ps1` when Docker is up |
| GHCR trim pull sizes documented | **Env-blocked** | Run `scripts/benchmark-ghcr-pull.ps1 -ImageTag v0.1.4` when Docker is up; local trim estimates in `install-desktop.md` |

**Product decision for v0.1.4:** Do not block on GHCR scraper. Block on **honest tiers** — Setup.exe ships Core Watch; minute TwitchTracker charts are Analytics tier (scraper profile).

**Executive summary:** Streamclone’s main install risk is not raw app complexity; it’s Windows Docker Desktop orchestration. The production `v0.1.3` installer failed on clean install. The patched path fixes several real Windows issues: Docker credential helper resolution, broken `PATHEXT`, hidden PowerShell/Docker invocation, scoped prune tag selection, no-browser benchmark mode, and quieter Docker calls.

### Image trim (local builds)

| Image | Before | After | Notes |
|-------|--------|-------|-------|
| `video` | ~924 MB | ~380 MB | Core profile |
| `emote` | ~430 MB | ~136 MB | Core profile |
| `clipper` | ~1.77 GB | ~1.01 GB | Optional profile only |

### Scores

| Area | Score | Notes |
|------|-------|-------|
| Install (current release) | 5/10 | `v0.1.3` Setup.exe failed on clean install |
| Install (with patches) | 7/10 | Pending final silent cold rerun |
| Infrastructure | 7/10 | Compose shape is reasonable; Windows Desktop startup, image size, tag drift, and installer failure reporting need hardening |

### Benchmark table

| Item | Result |
|------|--------|
| `v0.1.3` baseline Setup.exe | Failed, exit `5`, ~49.6s, orphan install-file issue |
| Patched manual backend setup | Succeeded; pulled images, started stack |
| `smoke-core.ps1` after manual setup | Passed all core checks |
| Final silent cold Setup.exe | **Env-blocked:** Docker Desktop Linux engine stopped |
| Cached Stop → Start (`benchmark-restart.ps1`) | **Env-blocked:** Docker engine not responding |
| GHCR pull sizes (`benchmark-ghcr-pull.ps1`) | **Env-blocked:** Docker engine not responding |
| HLS cold start | **Blocked** |
| Analytics latency | **Blocked** |
| Local trim build validation | `video` Streamlink/FFmpeg ok, `emote` vips ok, `clipper` streamlink/imageio-ffmpeg ok |

---

## Top risks (original list)

1. Installer can fail backend setup while still looking partially installed.
2. Docker Desktop engine/context drift causes false benchmark results.
3. `video` image dominates core download time.
4. Unpinned third-party tags and GHCR test-tag gaps make runs hard to compare.
5. Live HLS/analytics tests depend on a live/data-backed Twitch channel.

## Top quick wins (original list)

1. Keep the Docker helper changes in `scripts/lib/env.ps1`.
2. Keep silent benchmark/no-browser behavior in `scripts/benchmark-exe-install.ps1` and `scripts/install-setup-progress.ps1`.
3. Ship Alpine video/emote Dockerfiles.
4. Keep clipper `git`/system FFmpeg removal, labeled optional-only.
5. Record Docker Desktop context and engine status before every install benchmark.

## Top strategic changes (original list)

1. Publish new GHCR tags for trimmed images and benchmark registry pulls, not just local builds.
2. Add installer CI smoke for `Setup.exe` exit code and progress-file completion.
3. Add a first-class “Docker Desktop not running” UX that does not silently continue.
4. Pin or record digests for third-party images.
5. Split core, clipper, and scraper install metrics in release notes.

## Do not do

- Do not use `docker system prune -a`.
- Do not claim clipper trims improve core install.
- Do not switch `caddy:2` against steering.
- Do not add IDM for Docker pulls.
- Do not treat `v0.1.4-test` as a production baseline.

## Alternative architectures

| Option | Pros | Cons |
|--------|------|------|
| Keep Docker Desktop compose | Best compatibility with current stack | Windows startup/context fragility |
| Pre-bundle lightweight core images | Faster first run | Bigger release artifact, update complexity |
| Native Windows services for core | Better UX | Major rewrite |
| WSL-managed stack | More deterministic Linux runtime | More user setup friction |

---

## First: unblock and finish the benchmark suite

Before new features, formalize a **pre-flight gate** every install/benchmark run must pass:

| Check | Why |
|-------|-----|
| `docker context` + `docker info` (Linux engine running) | Avoids false “install failed” when Docker Desktop is stopped |
| Record `IMAGE_TAG`, compose files used, GHCR vs local | Tag drift (`v0.1.4-test` + `IMAGE_TAG=v0.1.3`) makes comparisons meaningless without this |
| Exit code + progress file + `smoke-core.ps1` | Separates “wizard finished” from “stack actually healthy” |

That yields a trustworthy **cold / warm / HLS / analytics** matrix for v0.1.4.

---

## How to run benchmarks when Docker is up

Start **Docker Desktop** and wait until `docker info` succeeds, then from the repo root (or `%USERPROFILE%\streamclone` after install):

```powershell
# Pre-flight gate (JSON for scripts / logs)
powershell -ExecutionPolicy Bypass -File scripts\preflight-deps.ps1 -JsonSummary

# Cold Setup.exe install (silent, no browser; writes dist\benchmark-exe-install-preflight.json)
powershell -ExecutionPolicy Bypass -File scripts\benchmark-exe-install.ps1

# Warm stop → start → wait for http://localhost:8090 (200)
powershell -ExecutionPolicy Bypass -File scripts\benchmark-restart.ps1

# GHCR pull time + compressed size per core image (uses VERSION / IMAGE_TAG)
powershell -ExecutionPolicy Bypass -File scripts\benchmark-ghcr-pull.ps1
```

Optional: `-ImageTag v0.1.4` on `benchmark-ghcr-pull.ps1`; `-SetupExe` / `-LogFile` on `benchmark-exe-install.ps1`.

**Cold-install proof** remains **env-blocked** until Docker Desktop’s Linux engine is running on the benchmark machine — preflight will report `blocked: true` with `engine: stopped` or `not_responding`.

---

## Biggest product gap: core install vs analytics promise

**Partially addressed in v0.1.4 gate (core-analytics-truth):** docs, env comments, and UI now state that Setup.exe ships **Core Watch** only; minute charts require Analytics (scraper) tier. Remaining gap: optional scraper is still not on GHCR.

Highest-impact item beyond the original risk list:

**Setup.exe always installs `core` only**, but Analytics minute charts assume a **scraper profile + sibling repo** that release users do not get by default.

- Installer hard-codes `-Profile core` in `scripts/install-setup-progress.ps1`
- Scraper is not in the GHCR release matrix; release compose still expects `../streamclone-scraper`
- ~~`ServiceStatusBanner` only shows for `scraper`/`full` — **core users with empty charts get no guidance**~~ → fixed: core users see info banner + Analytics empty state with scraper doc link

**Decision (v0.1.4):**

| Option | Tradeoff |
|--------|----------|
| **A. Core = watch only** ✅ chosen for v0.1.4 | Analytics shows honest “needs scraper setup” for core users; docs/installer say so |
| **B. Publish scraper to GHCR** | Bigger image, Camoufox complexity, but analytics works from Setup.exe |
| **C. Optional wizard step** | “Include viewer charts?” → pulls scraper image or clones sibling |

Until cold-install benchmarks pass, install score stays capped regardless of Docker helper fixes.

---

## Security surfaces worth hardening

1. **`setup-control` is unauthenticated on localhost** — ~~proxied through Caddy~~ → mitigated: `SETUP_CONTROL_TOKEN` + `X-Streamclone-Setup-Token` on POST (`scripts/setup-control.ps1`, `frontend/src/api.ts`).

2. **`TWITCH_DEV_TOKEN_IMPORT_ENABLED=true` in release templates** — contradicts `docs/security.md`. Ship `false` for end users; dev-only in `.env.dev` (`deploy/env/profile-core.env`, `scripts/package-release.sh`).

3. **Secret generation uses `Get-Random`** in `scripts/lib/env.ps1` — use a crypto RNG for `CURATOR_API_TOKEN`, cookie secrets, scraper keys.

---

## CI / release pipeline gaps

| Missing today | Suggested check |
|---------------|-----------------|
| CI never validates `docker-compose.release.yml` | `compose config` + `pull` tagged images on release |
| Playwright / HLS smoke not in CI | Run `smoke-core` with `--ui` on release candidates |
| Scraper smoke is manual/cron only | Pin sibling SHA; block releases if contract breaks |
| No tag consistency gate | Assert `git tag == IMAGE_TAG == GHCR tags == Setup.exe version` |
| `govulncheck` / `npm audit` are non-blocking | At least fail on critical CVEs |

**Single `VERSION` source** — `VERSION` at repo root (`v0.1.4`); `package-release.sh`, `build-windows-installer.ps1`, and preflight read it; `dist/release-manifest.json` generated on package.

Relevant workflows: `.github/workflows/ci.yml`, `.github/workflows/release-images.yml`, `.github/workflows/smoke-scraper.yml`.

---

## Compose / script consistency

Several host scripts do not use the same compose stack as setup:

- `setup-control.ps1` and `reload-env-if-stale.ps1` use `Get-StreamcloneComposeArgs` (release overlay when `IMAGE_TAG` / `VERSION` present)
- ~~`reload-env-if-stale.ps1` hardcodes container names like `streamclone-emote-1`~~ → fixed: `Get-StreamcloneRunningContainerName` via compose
- Uninstall always passes `--profile scraper` + `--profile clipper` even for core-only installs → confusing errors (`scripts/uninstall-streamclone.ps1`)

**Revamp:** one compose-builder function used everywhere — setup, start, stop, uninstall, setup-control, reload-env.

---

## Health checks: installer says “ready” too early

Today “ready” ≈ Caddy returns 2xx. Gaps:

- Smoke polls `/healthz`, not `/readyz`
- MinIO and MediaMTX lack healthchecks; emote can start before MinIO is actually serving
- HLS “ready” does not mean playlist is playable

**Revamp for install UX:**

1. **Tiered readiness:** infra (postgres/minio) → app services → HLS probe → optional scraper
2. **Fail setup** (or show blocking banner) when a **required** tier fails — not just `Write-Warning` in `scripts/setup.ps1`
3. Record per-tier timings in benchmark output (split core/clipper/scraper metrics)

---

## Frontend / analytics reliability

Beyond live-channel dependency:

- **Core users hitting Analytics** should see clear copy: “Viewer charts need the scraper profile” + link to [scraper-cloudflare-and-proxy.md](./scraper-cloudflare-and-proxy.md), not empty charts
- **Camoufox warmup** is documented but not in setup/first-sync flow — first analytics sync often fails cold
- **HLS recovery** in `frontend/src/playback.ts` is good, but MediaMTX/Caddy shared-secret drift is not self-healing — add stream diagnostics or smoke that catches auth mismatch

Contract tests between `analytics` ↔ `scraper` API would catch sibling-repo drift before users do.

---

## Image trim: what’s left after local builds

1. **Publish trimmed tags to GHCR** (`v0.1.4` or `v0.1.4-slim`)
2. **Benchmark registry pulls**, not local `docker build`
3. **Release notes:** core download size before/after; explicitly **exclude clipper** from core metrics
4. **Pin or record digests** for third-party images (MinIO, MediaMTX, Caddy)

---

## Docs / ops revamp

[scraper-cloudflare-and-proxy.md](./scraper-cloudflare-and-proxy.md) is strong technically. Missing **installer integration:**

- When scraper is optional, doc should state “not included in default Setup.exe”
- Windows CDP / Camoufox first-run steps should live in install flow or a one-click “warm scraper” launcher
- Proxy section: clarify experimental status for TwitchTracker so users do not waste time on bash proxy prompts

---

## Additional prioritized recommendations

### P0 — Release / scraper path

| # | Item | Files |
|---|------|-------|
| 1 | Setup.exe always `core`; analytics charts cannot work OOTB | `scripts/install-setup-progress.ps1`, `deploy/installer/streamclone-setup.iss` |
| 2 | Scraper excluded from GHCR matrix; release overlay assumes local build | `.github/workflows/release-images.yml`, `deploy/docker-compose.release.yml` |
| 3 | On-demand scraper start requires sibling repo outside install dir | `scripts/setup-control.ps1`, `scripts/lib/env.ps1`, `frontend/src/components/ServiceStatusBanner.tsx` |

### P0 — Security

| # | Item | Files |
|---|------|-------|
| 4 | Unauthenticated `setup-control` via browser origin (CSRF) | `scripts/setup-control.ps1`, `deploy/Caddyfile.local-tunnel` |
| 5 | Dev token import enabled in shipped release templates | `deploy/env/profile-core.env`, `scripts/package-release.sh` |
| 6 | Non-cryptographic secret RNG | `scripts/lib/env.ps1` |

### P1 — CI/CD

| # | Item | Files |
|---|------|-------|
| 7 | CI never validates release install path | `.github/workflows/ci.yml` |
| 8 | No `validate-env`, Playwright, or clipper profile in CI | `scripts/validate-env.sh`, `frontend/e2e/smoke-core.spec.ts` |
| 9 | Scraper smoke manual only; unpinned sibling clone | `.github/workflows/smoke-scraper.yml` |
| 10 | Release workflow does not verify pull+run after publish | `.github/workflows/release-images.yml` |

### P1 — Error handling / observability

| # | Item | Files |
|---|------|-------|
| 11 | `setup-control` / `reload-env-if-stale` omit release overlay | `scripts/setup-control.ps1`, `scripts/reload-env-if-stale.ps1` |
| 12 | Setup smoke warnings do not fail; start ignores validate-env | `scripts/setup.ps1`, `scripts/start-streamclone.ps1` |
| 13 | Uninstall compose down swallows Docker errors | `scripts/uninstall-streamclone.ps1` |
| 14 | Shallow readiness; infra lacks healthchecks | `scripts/lib/wait-stack.ps1`, `scripts/smoke-core.ps1`, `deploy/docker-compose.yml` |
| 15 | Core profile ships analytics without in-app scraper guidance | `deploy/env/profile-core.env`, `frontend/src/components/ServiceStatusBanner.tsx` |

---

## Suggested priority order

### This week (release-blocking)

1. Docker pre-flight gate + finish cold/warm/HLS/analytics benchmarks
2. Decide scraper delivery model for Windows (A / B / C above)
3. Single VERSION source + tag consistency
4. Unify compose overlays across all PowerShell scripts
5. Ship trimmed GHCR tags and benchmark pulls

### Next sprint (trust + safety)

6. `setup-control` auth + disable dev token import in release
7. Crypto RNG for secrets
8. Installer CI smoke (exit code + progress file + `smoke-core`)
9. Core-user Analytics empty-state / scraper guidance
10. Deeper readiness checks in setup

### Strategic (if analytics is first-class)

11. GHCR scraper image + optional wizard profile
12. Playwright HLS smoke in release pipeline
13. Analytics ↔ scraper contract tests
14. Observability bundle for power users (optional, not default install)

---

## What not to revamp yet

- Native Windows services (major rewrite; Docker Desktop fixes are the right bet for now)
- IDM-style pull accelerators
- Replacing Caddy
- Observability stack in default Setup.exe (adds weight; keep optional)

---

## Bottom line

Docker orchestration fixes are necessary but not sufficient. The next leap is **honest product boundaries** (core vs scraper), **release-path consistency** (compose overlays, tags, GHCR), and **benchmark discipline** (Docker pre-flight + registry pulls).

**Changed surfaces (current work):** Dockerfiles plus installer/setup scripts. Test installer: `dist/Streamclone-Setup-v0.1.4-test.exe`.
