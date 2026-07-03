# StreamPulse Ops Boundary Migration Plan (revised)

Conservative, phased migration of host-specific production ops from public [`Aron-Chu/streamclone`](https://github.com/Aron-Chu/streamclone) into private `Aron-Chu/streampulse-ops`. App monolith stays public. Ops deploys **immutable GHCR images** by `IMAGE_TAG`.

**Revision notes (2026-07-03):** Incorporates guardrails for env CLI, BearHost overlay isolation, migrate-image runtime validation, dirty-tree triage, release tag clarity, branch protection API, release workflow ordering, and ops validation depth.

---

## Phase 0 Baseline (read-only audit — completed)

| Item | Current state |
|------|----------------|
| Local checkout | `twitch-7tv-clone`, git repo, branch `master`, remote `origin` → `Aron-Chu/streamclone` |
| Working tree | **Large dirty tree** — migration on dedicated branch only |
| GitHub visibility | **Public**, default branch `master` |
| Branch protection | **None** (`404 Branch not protected`) |
| Latest **stable** release | `v0.2.10` (`gh api releases/latest`) |
| Latest **prerelease** | `v0.3.0-rc5` (2026-06-15) — use only after migrate image exists and is smoke-tested |
| `streampulse-ops` | **Does not exist** — create via `gh repo create Aron-Chu/streampulse-ops --private` in Phase 5 |
| Production host | **streampulse-vps** (`23.173.152.156`); BearHost (`141.11.243.103`) **rollback-only** |
| Deploy model today | rsync + source build via `scripts/streampulse-vps-production-deploy.sh`; optional `STREAMPULSE_USE_RELEASE_IMAGES=1` + `IMAGE_TAG` in `scripts/lib/streampulse-vps-production-compose.sh` |

### CI state on latest `master` push (run [28636434856](https://github.com/Aron-Chu/streamclone/actions/runs/28636434856))

| Job | Result | Blocker class |
|-----|--------|---------------|
| Secret scan | pass | — |
| Full gate (`make check`) | pass | — |
| Code graph rebuild | **fail** | codegraph/tooling |
| Core compose smoke | **fail** (Wait for health endpoints) | compose/runtime |

**Gate:** Do not delete public ops files or cut over until smoke + codegraph are fixed or documented as unrelated.

### `.env.dev` drift

- `.env.dev` exists locally but is **not tracked**.
- CI uses `scripts/ci-bootstrap-env.sh` → `.env.example`.
- Still broken: `Makefile`, `scripts/lib/env.sh`, `release-images.yml`, `package-release.sh`, bootstrap/validate scripts, docs.

---

## Pre-PR0: Dirty tree triage (hard gate before PR1)

**Do not write migration manifests or copy files into `streampulse-ops` until this table exists.**

Inventory **all** untracked, modified, and deleted paths in the working tree. Produce a table in `migration-baseline.md` (local, uncommitted until PR1):

| path | state (tracked/untracked/modified/deleted) | needed for migration? | action |
|------|--------------------------------------------|----------------------|--------|

**Allowed actions:** `keep`, `commit-before-migration`, `copy-to-ops`, `archive`, `ignore`, `human-review`

**Rules:**

- Do **not** copy **untracked** files into `streampulse-ops` until explicitly classified.
- Do **not** treat untracked WIP as stable repo inputs in the manifest.
- Ops-relevant untracked paths observed in current tree (classify before PR1):

| path | preliminary action |
|------|-------------------|
| `.github/workflows/hub-health-monitor.yml` | `human-review` — public API probe; likely **stay** in streamclone if committed |
| `scripts/streampulse-vps-cron-install.sh` | `commit-before-migration` or `copy-to-ops` |
| `scripts/streampulse-vps-pg-backup.sh` | same |
| `scripts/streampulse-vps-r2-corpus-cutover.sh` | same |
| `scripts/streampulse-vps-top500-scale.sh` | same |
| `scripts/bearhost-worker.sh` | `archive` or `copy-to-ops` (BearHost rollback) |
| `deploy/docker-compose.bearhost-worker.yml` | `archive/bearhost` |
| `scripts/hosted-data-mcp.sh`, `scripts/hosted-data-tunnel.sh` | `copy-to-ops` (operator MCP tunnel) |
| `deploy/alerts/top500-hosted.proposal.yml` | `archive` or `ignore` |
| Agent/skill mirror dirs (`.claude/`, duplicate `.cursor/skills/`) | `ignore` for ops migration |

Re-run triage after any large local changes.

---

## PR 1 — Baseline note + move manifest (Phase 0–1)

**Branch:** `chore/ops-migration-manifest`

1. Write `migration-baseline.md` (summarized audit + dirty-tree triage table).
2. Add `docs/ops-migration-manifest.md` with exact-path classification (six sections per original spec).

Manifest uses **only tracked paths** plus triage-approved untracked paths.

---

## PR 2 — Fix `.env.dev` drift (Phase 2)

**Branch:** `fix/env-dev-removal`

### Step 2a — Callable env synthesis entrypoint (required before Makefile change)

`scripts/lib/env.sh` defines functions only (e.g. `env_synthesize` at line 270); it has **no CLI dispatcher**. Do not call `bash scripts/lib/env.sh synthesize core` directly.

**Preferred:** add `scripts/env-synthesize.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/env.sh
source "${ROOT}/scripts/lib/env.sh"
PROFILE="${1:-core}"
OUTFILE="${2:-${ROOT}/.env}"
env_synthesize "$PROFILE" "$OUTFILE"
```

**Alternative:** add `case "$1" in synthesize) ... esac` CLI to `scripts/lib/env.sh` (only if wrapper is rejected).

### Step 2b — Update consumers

| File | Change |
|------|--------|
| `Makefile` | `env` target: `bash scripts/env-synthesize.sh core .env` (not `cp .env.dev`) |
| `scripts/lib/env.sh` | Replace `.env.dev` source with `.env.example` + optional `deploy/env/profile-dev.env` |
| `scripts/lib/env.ps1` | Mirror bash; call equivalent wrapper |
| `.github/workflows/release-images.yml` | `bash scripts/ci-bootstrap-env.sh` or `env-synthesize.sh` |
| `.github/workflows/smoke-scraper.yml` | Same |
| `scripts/package-release.sh` | Ship `.env.example` + `profile-dev.env`, not `.env.dev` |
| Bootstrap/validate scripts + docs | Update messages |

### New file: `deploy/env/profile-dev.env`

Tracked non-secret dev defaults (e.g. `TWITCH_DEV_TOKEN_IMPORT_ENABLED=true` for localhost).

### Validation

```bash
rm -f /tmp/test.env
bash scripts/env-synthesize.sh core /tmp/test.env
test -s /tmp/test.env
rg "\.env\.dev"   # only intentional "removed" notes
make compose-config-check
make check-quick
```

---

## PR 3 — CI green + branch protection + artifact contract (Phase 3–4)

### Fix CI failures

1. **Code graph:** fix `make codegraph-incremental` / `make codegraph-smoke` failure in `.github/workflows/ci.yml`.
2. **Core compose smoke:** diagnose `scripts/smoke-core.sh` health wait; use failure dump artifact.

### Branch protection (exact)

**Primary:** enable via GitHub API. Required status check names must match **job `name:` fields** in `.github/workflows/ci.yml`:

- `Secret scan`
- `Full gate (make check)` — required on `push` to `master`
- `Code graph rebuild`
- `Core compose smoke`

```bash
gh api \
  --method PUT \
  -H "Accept: application/vnd.github+json" \
  repos/Aron-Chu/streamclone/branches/master/protection \
  -f required_status_checks[strict]=true \
  -f required_status_checks[contexts][]='Secret scan' \
  -f required_status_checks[contexts][]='Full gate (make check)' \
  -f required_status_checks[contexts][]='Code graph rebuild' \
  -f required_status_checks[contexts][]='Core compose smoke' \
  -f enforce_admins=true \
  -f required_pull_request_reviews[required_approving_review_count]=1 \
  -f restrictions=null \
  -f allow_force_pushes=false \
  -f allow_deletions=false
```

Also enable CodeQL required check if `codeql.yml` runs on PRs (add `Analyze (go)` / workflow job name after verifying in Actions UI).

**Fallback:** if API returns 403/404, document exact settings in `docs/repo-maintenance.md` and stop — do not claim protection is enabled.

**Verify:**

```bash
gh api repos/Aron-Chu/streamclone/branches/master/protection --jq '.required_status_checks.contexts'
```

### Production Artifact Contract

Add `docs/production-artifact-contract.md` (or section in `docs/ENVIRONMENT.md`):

- `IMAGE_TAG` = immutable release id (git tag / `VERSION`)
- GHCR images from streamclone: `metadata`, `chat`, `video`, `analytics`, `emote`, `frontend`, `scraper` (optional), **`migrate`**
- **Invariant:** `analytics` == `analytics-workers` == `migrate` == deployed `IMAGE_TAG`
- Ops pulls by tag; rollback = redeploy previous `IMAGE_TAG`
- Host topology / secrets in private `streampulse-ops` only

---

## PR 4 — GHCR migrate image (prerequisite for image-only ops)

**Branch:** `feat(release): publish migrate image`

### Dockerfile

`deploy/Dockerfile.migrate`:

```dockerfile
FROM migrate/migrate
COPY migrations /migrations
```

### Compose contract (must match existing service)

Current migrate service in `deploy/docker-compose.yml`:

```yaml
image: migrate/migrate
volumes: ["../migrations:/migrations:ro"]
command: ["-path", "/migrations", "-database", "postgres://app:app@postgres:5432/streamclone?sslmode=disable", "up"]
```

Image-based overlay must preserve **identical runtime**:

```yaml
migrate:
  image: ghcr.io/aron-chu/streamclone/migrate:${IMAGE_TAG}
  volumes: !reset []
  command: ["-path", "/migrations", "-database", "${DATABASE_URL}", "up"]
  depends_on:
    postgres:
      condition: service_healthy
```

`DATABASE_URL` comes from env file (same as other services). Do not change `-path /migrations` — migrations are baked at `/migrations` in the image.

### Release workflow ordering (exact)

In `.github/workflows/release-images.yml`:

1. **Build and push** all matrix images including `migrate` to GHCR with resolved `IMAGE_TAG`.
2. **Post-publish release smoke job** (depends on `publish`):
   - `docker compose ... pull` all images including `migrate` at `IMAGE_TAG`
   - Start Postgres only (or minimal stack)
   - Run: `docker compose run --rm migrate` (must exit 0)
   - Assert `schema_migrations` row exists / version advances on fresh DB
   - Run existing `scripts/smoke-core.sh` against full release stack
3. Mark release usable only after post-publish smoke passes.

Do **not** claim migrate is validated before the image is pushed unless testing with a local `docker build` tag in the same job before push.

### Local validation (before merge)

```bash
docker build -f deploy/Dockerfile.migrate -t migrate-test:local .
docker compose -f deploy/docker-compose.yml up -d postgres
# wait healthy
docker run --rm --network container:... migrate-test:local \
  -path /migrations -database 'postgres://app:app@postgres:5432/streamclone?sslmode=disable' up
```

Break-glass: checkout-mounted `migrate/migrate` + volume → `streampulse-ops/archive/break-glass/` only.

---

## PR 5 — Create `streampulse-ops` + scaffold (Phase 5)

```bash
gh repo create Aron-Chu/streampulse-ops --private \
  --description "StreamPulse production ops (private)"
```

Scaffold per original spec (`README.md`, `AGENTS.md`, `.gitignore`, `compose/`, `scripts/`, `docs/`, `archive/`).

---

## PR 6 — Copy ops files + image-based production compose (Phase 6–7)

### Copy rules

- Copy, do not move, from streamclone
- **Only triage-approved paths** (tracked + explicitly classified untracked)
- No secrets, `.env`, `*.local.env`, `runtime/`

### Active production compose (prescriptive)

**Default production compose must NOT merge BearHost overlays.**

Active path:

1. Image-only service definitions (GHCR `IMAGE_TAG` required) — pattern from `deploy/docker-compose.release.yml`
2. Production overlay from **`deploy/docker-compose.streampulse-vps-production.yml` only** (streampulse-vps settings: workers, ports, corpus flags)
3. Generic prod hardening from `deploy/docker-compose.prod.yml` where applicable (no host publish on data stores)
4. `migrate` image at same `IMAGE_TAG`
5. Scraper: separate `SCRAPER_IMAGE_TAG` when not bundled

**Do not** include in default active stack:

- `deploy/docker-compose.bearhost-build.yml`
- `deploy/docker-compose.bearhost-prod.yml`
- `deploy/docker-compose.bearhost-pulse.yml`
- Any rsync/source-build overlay

BearHost compose files → `streampulse-ops/archive/bearhost/` or `archive/break-glass/` **only**. Promote a BearHost file to active only after file-level review proves a setting is required on **current** streampulse-vps prod (document evidence in `import-manifest.md`).

Rewrite `scripts/lib/streampulse-vps-production-compose.sh` in ops to drop BearHost merge from default args.

### Validation

```bash
IMAGE_TAG=v0.2.10 docker compose \
  --env-file env/examples/production.env.example \
  -f compose/production/docker-compose.yml config
```

**Additional checks (P3):**

```bash
# No build: contexts in default production render
docker compose ... config | rg 'build:' && exit 1 || true

# Placeholder scan on example env (must fail if CHANGE_ME etc. in required keys)
bash scripts/smoke/validate-production-env.sh --example env/examples/production.env.example

# On host before deploy (private non-committed env):
bash scripts/smoke/validate-production-env.sh --strict env/production.local.env
# Checks: no CHANGE_ME/replace-me, required secret file paths exist on host, IMAGE_TAG set
```

### Import manifest

`docs/migration/import-manifest.md` — source, dest, status, reason, validation command per file.

---

## PR 7 — Ops smoke + rollback (Phase 8)

`scripts/smoke/production-smoke.sh`:

- Public API health (`/v1/extension/health`)
- Hub health if applicable
- Migration status via app health
- Scraper/worker health if enabled
- Unauthenticated admin → expected auth failure
- `--dry-run` mode

`docs/runbooks/rollback.md` — current/previous `IMAGE_TAG`, redeploy, smoke, logs, non-reversible migration warning.

---

## PR 8 — Public stubs + agent routing (Phase 9–10)

Stub template: hosted ops in private `streampulse-ops`; public repo = local/self-hosted examples only.

Update `AGENTS.md`, steering, `docs/workspace.md`, `docs/ENVIRONMENT.md`, `docs/repo-maintenance.md`, `docs/security.md`.

Makefile: bearhost/grafana/streampulse-vps targets → redirect message.

---

## PR 9 — Validation gate (Phase 11)

### streamclone

```bash
rg "\.env\.dev"
make compose-config-check
make check-quick
make check
```

### streampulse-ops

```bash
IMAGE_TAG=v0.2.10 docker compose --env-file env/examples/production.env.example \
  -f compose/production/docker-compose.yml config
bash scripts/smoke/validate-production-env.sh --example env/examples/production.env.example
bash scripts/smoke/production-smoke.sh --dry-run
# On operator host with real env:
bash scripts/smoke/validate-production-env.sh --strict env/production.local.env
```

Checklist: active runbooks committed in ops, public stubs present, no secrets, no `build:` in default prod config, import manifest complete.

**Do not delete public ops files until this gate passes.**

---

## PR 10 — Production cutover (Phase 12)

Use **latest stable** (`v0.2.10`) or newer tag **after** migrate image + post-publish smoke exist. Prerelease `v0.3.0-rc5` only if explicitly chosen and GHCR images verified.

1. Confirm all GHCR images at `IMAGE_TAG` including `migrate`
2. Backup via ops scripts
3. Deploy from `streampulse-ops` (image-only default)
4. Run migrate container from pinned migrate image
5. `production-smoke.sh`
6. Record `docs/deployments/YYYY-MM-DD-<tag>.md` with operator, tags, migration/smoke results, rollback tag

---

## PR 11 — Cleanup (Phase 13)

Delete moved public ops files only after one successful image-based deploy + rollback plan confirmed.

---

## Risk register

| Risk | Mitigation |
|------|------------|
| Non-callable `env.sh` | `scripts/env-synthesize.sh` wrapper (PR2 step 2a) |
| BearHost legacy in active compose | Active = streampulse-vps + image-only; BearHost → archive only |
| Migrate image builds but fails at runtime | Same `command` as compose.yml; post-publish DB smoke |
| Untracked WIP copied to ops | Pre-PR0 dirty tree triage gate |
| Wrong release tag for cutover | Distinguish stable `v0.2.10` vs prerelease `v0.3.0-rc5` |
| Partial branch protection | Exact `gh api` payload + verify `contexts` |
| Placeholder env passes `config` | `validate-production-env.sh` strict mode on host |
| Scraper separate repo | `SCRAPER_IMAGE_TAG` in ops env |

## Execution order

1. **Pre-PR0** dirty tree triage
2. PR1 manifest
3. PR2 env (`env-synthesize.sh` first)
4. PR3 CI + branch protection + artifact contract
5. PR4 migrate image + post-publish smoke
6. PR5 create private repo
7. PR6 copy + image-only compose (no BearHost in active path)
8. PR7 smoke + rollback
9. PR8 public stubs + routing
10. PR9 validate both repos
11. PR10 cutover with evidence
12. PR11 cleanup
