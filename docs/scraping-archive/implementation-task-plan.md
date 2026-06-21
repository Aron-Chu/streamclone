# Streamclone Global Archive — Implementation Task Plan

Status: **Release 1–2 code complete**; Release 3 mostly shipped; operator smoke + TASK-030 sign-off pending
Sources: [corpus-requirements.md](corpus-requirements.md) · [archive-observability.md](archive-observability.md) · [requirements.md](requirements.md) · [proxy-benchmark.md](proxy-benchmark.md)
Verified against repo: 2026-06-20

## Execution progress (live)

**Azure E2E probe:** YES — `C:\Users\Aron\.streamclone\azure-archive-connection-string` present on dev machine
**Last bearhost-config-check-local:** PASS (2026-06-20)
**Last check-quick:** PASS (2026-06-20, WSL `make check-quick` after R1/R2/R3 split: `c1fdcd3`, `901ea8b`, `a6efecb`)

### Release 1
- [x] TASK-001 — Baseline inventory sign-off (§2 verified 2026-06-20)
- [x] TASK-002 — BearHost corpus-plane decision documented
- [x] TASK-003 — BearHost env + preflight implemented
- [x] TASK-004 — `archive_exports` migration 000035
- [x] TASK-004B — Natural key contract
- [x] TASK-005 — ManifestStore expansion
- [x] TASK-006 — Writer SHA-256 + sidecar
- [x] TASK-007 — Bronze VOD catalog v1 (bronze_export + dual-write)
- [x] TASK-008 — Bronze identity + crosswalk exporters
- [x] TASK-010 — Global 7TV snapshot
- [x] TASK-016 — `archive_jobs` schema
- [x] TASK-016B — Reconcile `archive_jobs` ↔ `backfill_jobs`
- [x] TASK-017 — jobtracker library
- [x] TASK-018 — Wire jobtracker (bronze/backfill workers)
- [x] TASK-019 — CLI `jobs` + `coverage report`
- [ ] TASK-031 — BearHost corpus smoke (**BLOCKED** on VPS 2026-06-20 — see blockers log)
- [ ] TASK-031B — Restore drill smoke (**BLOCKED** on VPS 2026-06-20 — see blockers log)

### Release 2
- [x] TASK-033 — Caddy admin routes
- [x] TASK-021 — `ADMIN_ARCHIVE_TOKEN` middleware
- [x] TASK-022 — Admin REST handlers
- [x] TASK-023 — Admin audit events
- [x] TASK-020 — verify-blobs
- [x] TASK-009 — Roster/tombstones/coverage blob
- [x] TASK-012 — Silver TT chart JSON
- [x] TASK-013 — Silver partial manifests

### Release 3
- [x] TASK-024 — Admin UI routes + token gate
- [x] TASK-025 — Admin UI panels
- [x] TASK-021B — Disable admin UI on public non-HTTPS
- [x] TASK-026 — Archive Prometheus metrics
- [x] TASK-027 — observability compose profile
- [x] TASK-028 — Grafana archive dashboard
- [x] TASK-014 — Gold-lite aggregate export
- [x] TASK-015 — Gold-lite/full tier split
- [x] TASK-011A — 7TV snapshot diff changelog
- [x] TASK-011B — Emote snapshot schema
- [x] TASK-011C — FFZ per-channel snapshot
- [x] TASK-011D — BTTV per-channel snapshot
- [x] TASK-029 — Proxy benchmark sign-off (doc)
- [ ] **BLOCKED:** TASK-030 — `ANALYTICS_TT_USE_PROXY` gated routing (env exists; default **false**; operator sign-off required)
- [x] TASK-032 — PulseWire cold export (P2)

### Blockers log
| Date | Task | Reason | Owner action |
|------|------|--------|--------------|
| 2026-06-20 | TASK-031 | VPS rsync OK; corpus smoke exit **127** (`go: command not found` on host). Azure secret at `/etc/streamclone/secrets/` OK when exported; `CORPUS_WORKERS_ENABLED=1` set but analytics/workers restart-loop (wrong in-container `ARCHIVE_AZURE_CONNECTION_STRING_FILE` path vs `/run/streamclone-secrets/` mount). Missing Twitch creds in VPS `.env`. | Install host Go **or** wrap scripts with `docker run golang:1.25-alpine`; fix `profile-bearhost-prod.env` secret path for container; add Twitch creds; rerun `bash scripts/bearhost-corpus-smoke.sh` |
| 2026-06-20 | TASK-031B | Restore drill exit **127** (no host Go). `archive_jobs` count **0** on VPS — no `STREAM_ID` available yet. | Fix corpus smoke first; pick `STREAM_ID` from `go run ./cmd/backfill jobs list`; `STREAM_ID=<id> bash scripts/archive-restore-drill.sh` |
| 2026-06-20 | TASK-031/031B (deploy) | `bearhost-deploy-phased.sh` failed Phase 5 frontend build (admin UI TS on VPS tree). Smoke gate **10** returns **404** (want **401**); gate **1** fail (analytics restart). Origin currently **502**. | Complete phased deploy after frontend fix; verify Caddy `@admin_archive`; `curl /v1/admin/archive/jobs` → 401 |
| 2026-06-20 | TASK-030 | Proxy routing requires operator sign-off per proxy-benchmark.md | Keep `ANALYTICS_TT_USE_PROXY=false`; re-run budget benchmark before enable |

---

# 1. Executive summary

- Build **one global Azure-backed archive corpus** of VOD **intelligence** (catalog, identity, viewer charts, chat aggregates, emote metadata) — **not** Twitch video files.
- **v1 is shipped** for Azure upload, minimal `archive_exports`, Bronze VOD index + TT summary, silver/gold `backfill_jobs`, sync export, partial coverage CLI, and BearHost worker split.
- **Next wave** expands the manifest model (SHA-256, tier, provider, sidecars), Bronze identity/crosswalk/roster/tombstones, emote global 7TV + changelog diff, Silver TT provenance, Gold-lite/Gold-full split, and durable **`archive_jobs`** progress in Postgres.
- **Postgres is SoT** for exact job progress; **CLI first** (Release 1), then admin HTTP API (Release 2), then admin UI (Release 3); Grafana/Prometheus are optional trends only.
- **Three release gates** — do not implement all phases at once (see §3.1). Release 1 = manifests + bronze + global 7TV + job progress + CLI + BearHost smoke + restore drill.
- **Job model:** `archive_jobs` = operator-visible parent; `archive_job_items` = per-channel/stream progress; `backfill_jobs` = low-level sync queue — linked via TASK-016B so UI/CLI cannot drift from queue reality.
- **Natural keys:** TASK-004B contract before any writer changes (idempotency depends on `(artifact_type, natural_key)`).
- **BearHost safety first**: **`analytics-workers` = corpus plane** (not a fork) — corpus flags on only after Azure secret + Twitch creds preflight; `analytics` stays API-only. Fix Caddy **before** admin API (`/v1/admin/*` today routes to `metadata`). Admin auth uses **`ADMIN_ARCHIVE_TOKEN`** (never public `config.js`); observability via optional `observability` profile (not required for `pulse`).
- **Proxy gating**: benchmark baseline exists; do **not** enable `ANALYTICS_TT_USE_PROXY` until budget re-run + operator sign-off; Silver bulk may need residential proxy or home-PC egress.
- **Incremental merges:** manifest migration → natural key contract → writers → workers → job reconciliation → CLI → (Release 2) admin API → (Release 3) UI/metrics/proxy.
- **Operator tasks** (secrets, cron, smoke, SSH tunnel Grafana) are separate from implementation tasks.

---

# 2. Current-state findings

| Area | What docs say | Likely code area | Status | Verification task |
|------|---------------|------------------|--------|-------------------|
| Archive manifest model | Expand `archive_exports` + optional sidecars | `migrations/000030`, `internal/archive/manifest.go` | **Partial** — v1 upsert only | Confirm no `000035+` manifest expand migration |
| `archive_exports` expansion | tier, provider, sha256, metadata JSONB | `manifest.go`, `writer.go` | **Planned** | Grep `content_sha256`, `schema_version` in repo |
| Bronze VOD catalog | `vod_catalog/v1`, dated hive paths, Helix pagination | `bronze_indexer.go`, `writer.go` | **Partial** — legacy `channels/vod_index/{login}.jsonl.gz`, limit 80 | Inspect `bronzeHelixVODLimit` |
| Channel identity snapshots | Helix + 7TV identity blob | *none* | **Planned** | Grep `channel_identity` |
| Provider crosswalk | Twitch↔7TV↔FFZ↔BTTV map | *none* | **Planned** | Grep `crosswalk` |
| Roster snapshots | `rosters/tier0/date=…` | `ExportTop500`, `ExportTopRoster` | **Partial** — `channels/top500.json.gz` only | List blob keys in writer |
| Tombstone detection | Diff VOD catalog → tombstones | *none* | **Planned** | Grep `tombstone` |
| Silver viewer rollup export | Hive path + `viewer_rollup/v1` | `exporter.go`, `backfill_worker.go` | **Partial** — `rollups/stream_id={id}/part-000.jsonl.gz` | Export one stream; inspect blob |
| Raw/semi-raw TT artifact | HTML + chart JSON | `ExportTTDetail`, `sync.go` | **Partial** — HTML at `tt-detail/…` only | Grep `chart.json` |
| 7TV per-channel snapshots | Weekly roster export | `emote_exporter.go`, `workers.go` | **Shipped** | Run emote snapshot worker tick |
| Global 7TV snapshot | `login=global` | *none* | **Planned** | Grep `login=global` in emote_exporter |
| Emote changelog diffing | Snapshot diff → add/remove | `AppendChangelog` (EventAPI) | **Partial** — append only, no diff | Read `eventapi/subscriber.go` adapter |
| Optional emote media cache | One WebP per emote | *none* | **Planned** (P2) | — |
| Gold-lite chat/emote aggregates | Minute aggregates blob | *none* | **Planned** | Grep `GOLD_LITE` |
| Gold-full selective VOD chat | Full messages + gates | `ExportVODChat`, `gold_enqueuer.go` | **Partial** — single `gold` tier | Read `backfillSyncParams` |
| PulseWire export | `pulsewire/*` cold paths | `ArtifactSocialItem`, storygraph store | **Partial** — retention guard hooks exist | Grep `pulsewire/` export |
| Coverage reporting | Tier filters, verify-blobs, stale | `coverage_report.go`, `cmd/backfill` | **Partial** — `coverage report` only | Run `go run ./cmd/backfill coverage report` |
| Postgres job progress | `archive_jobs`, items, events | *none* | **Planned** | `\d archive_jobs` on dev DB |
| Job queue reconciliation | `archive_jobs` ↔ `backfill_jobs` | `backfill_jobs`, planned `archive_job_items` | **Gap** — two queues will drift without FK bridge | TASK-016B |
| Natural key contract | Per-artifact `natural_key` + overwrite rules | `manifest.go` upsert on `(artifact_type, natural_key)` | **Gap** — legacy keys inconsistent (`vod_index:`, `rollups:`) | TASK-004B |
| CLI job commands | `jobs list/show/retry/resume/cancel` | `cmd/backfill/main.go` | **Partial** — `backfill status` only | Read main.go switch |
| Admin API | `/v1/admin/archive/*` | `internal/analytics/api.go` | **Planned** | Grep `/v1/admin` |
| Admin UI | `/admin/archive`, jobs, coverage | `frontend/src/App.tsx` | **Planned** | Grep `/admin` routes |
| Prometheus metrics | `streamclone_archive_*` | `internal/metrics/archive.go` | **Partial** — v1 archive/backfill gauges | `curl analytics:8080/metrics` (internal) |
| Grafana dashboard | `streamclone-archive.json` | `deploy/grafana/dashboards/streamclone-ops.json` | **Partial** — Archive/Bronze row in ops | Open ops JSON panels |
| Proxy routing/benchmark gates | No global proxy until sign-off | `sync.go` (`useProxy: false`), `proxy-benchmark.md` | **Partial** — benchmark done; flag **not in code** | Grep `ANALYTICS_TT_USE_PROXY` |
| BearHost compose/deployment | Workers corpus gated; API-only analytics | `bearhost-prod.yml`, `profile-bearhost-prod.env`, `bearhost-corpus-preflight.sh` | **Partial** — TASK-003 corpus gating + preflight shipped; enable via `CORPUS_WORKERS_ENABLED=1` post-preflight | `make bearhost-config-check-local`; smoke gate 0 |
| Caddy admin routing | `/v1/admin/archive/*` → analytics | `deploy/Caddyfile`, `deploy/Caddyfile.bearhost` | **Gap** — only `/v1/analytics/*` → analytics; `/v1/*` → metadata | Inspect Caddy `@analytics` vs `@metadata` |
| Admin archive auth | Separate operator token, not browser config | `frontend/docker-entrypoint.d/40-streamclone-config.sh`, `docs/security.md` | **Gap** — plan initially reused `SETUP_CONTROL_TOKEN` in public `config.js` | Read security.md § Tunnels |

### Doc contradictions — proposed resolutions

| Contradiction | Resolution |
|---------------|------------|
| `requirements.md` marks Phases 1–4 **done**; `corpus-requirements.md` marks many items **planned** | Treat **code as truth**: v1 backfill/export is done; **corpus expansion + Part II** are the new scope. Update `requirements.md` gap table when Phase 1 lands. |
| `profile-archive.env` enables archive on workers; `bearhost-prod.yml` disables archive **only on `analytics`** | **Locked decision (TASK-002/003):** `analytics-workers` = corpus plane; corpus enabled only after Azure secret + Twitch creds preflight; otherwise workers explicitly disabled (no crash loop). |
| `/v1/admin/archive/*` on analytics but Caddy sends generic `/v1/*` to metadata | **TASK-033** before TASK-022/TASK-024: add `@admin_archive` block in `Caddyfile` + `Caddyfile.bearhost`; extend smoke tests. |
| Admin auth: setup-control vs archive token | **Do not** reuse `SETUP_CONTROL_TOKEN` for BearHost admin (public `config.js`). Use **`ADMIN_ARCHIVE_TOKEN`** + header **`X-Admin-Archive-Token`**; operator manual entry or CLI-only on public hosts. |
| Header name: `X-Setup-Control-Token` vs `X-Streamclone-Setup-Token` | **Standardize docs on `X-Streamclone-Setup-Token`** for PulseWire/setup-control only. Archive admin uses **`X-Admin-Archive-Token`** (separate secret). Fix stale `X-Setup-Control-Token` in corpus/observability docs. |
| `requirements.md` says hourly export “not wired”; code has `incremental_worker.go` | Mark **shipped** in docs after verify tick in `archive_workers.go`. |
| `ANALYTICS_TT_USE_PROXY` in docs only | Implement in **Phase 10** behind explicit env + benchmark gate task. |
| `pulse` vs `observability` profile | New **`observability`** overlay for archive metrics; keep `pulse` for Influx/Emote Pulse; BearHost defaults to neither. |
| `gcs_uri` vs Azure | Keep column name; store Azure HTTPS URIs (document in migration comments). |
| `archive_jobs` vs `backfill_jobs` | **Locked (TASK-016B):** `archive_jobs` = operator parent; `archive_job_items` = progress; `backfill_jobs` = sync worker queue; nullable FKs both directions. |
| Gold-lite “cheap” assumption | **Storage-light ≠ scrape-light.** Gold-lite export is cheap when chat rollups exist; otherwise requires GQL fetch like gold-full. No top-200 bulk without explicit gates. |

---

# 3. Dependency graph

```
Phase 0 verification
  └─► Phase 1 manifest foundation (000035 expand archive_exports, writer hash/sidecar)
        ├─► Phase 2 Bronze corpus (identity, crosswalk, catalog v1, tombstones)
        ├─► Phase 3 Emote corpus (global 7TV, diff changelog, multi-provider)
        ├─► Phase 4 Silver provenance (chart JSON, partial manifests, hive paths)
        └─► Phase 5 Gold-lite / Gold-full split
  Phase 1 manifest foundation
        ├─► TASK-004B natural key contract (before TASK-005)
        └─► writers + SHA-256 (TASK-005–006)
  Phase 1 + Phase 6 job tables (000036 archive_jobs)
        ├─► TASK-016B reconcile archive_jobs ↔ backfill_jobs
        └─► Wire jobtracker into bronze/backfill/emote/coverage workers
              ├─► Release 1 stop: CLI (TASK-019) + smoke (TASK-031/031B)
              ├─► Phase 7 coverage verify-blobs (Release 2)
              └─► Phase 9 metrics (Release 3)
  Release 2 prerequisites (before admin HTTP)
        ├─► TASK-033 Caddy `/v1/admin/*` → analytics
        └─► TASK-021 ADMIN_ARCHIVE_TOKEN auth
  Release 2 admin API (TASK-022–023) — CLI remains primary on BearHost
  Release 3 admin UI (TASK-024–025, TASK-021B) + observability + gold split + emote FFZ/BTTV + proxy
  Phase 10 proxy gate (independent until Silver bulk on BearHost)
  Phase 11 BearHost ops (after Phase 1 + env fix + smoke scripts)
```

**Hard rules**

- Manifest schema before expanded export writers (manifest rows need new columns).
- **`TASK-004B` natural key contract before TASK-005** — writers must not ship without documented keys + legacy mapping.
- **`TASK-016B` job reconciliation before TASK-018 wiring** — parent job progress must trace to `backfill_jobs` where applicable.
- **`TASK-033` + TASK-021 before TASK-022** (Release 2) — not required for Release 1 CLI.
- **`ADMIN_ARCHIVE_TOKEN` before admin UI (Release 3)** — never ship archive admin creds in `config.js` on non-loopback origins ([security.md](../security.md)).
- `archive_jobs` before job-scoped Prometheus gauges (Release 3).
- Coverage `verify-blobs` after manifest has `content_sha256` (or etag-only v1).
- Grafana after Prometheus metrics exist.
- Proxy routing after benchmark sign-off task (operator approval).
- Do **not** enable Gold-lite/Gold-full bulk on top-200 without operator approval and rollup-exists checks (TASK-014).

---

# 3.1 Release scope (MVP gates)

Implement in **three releases**. Stop and test at each gate before starting the next.

## Release 1 — Corpus foundation + CLI (stop here first)

**Outcome:** Durable manifests, Bronze archive, global 7TV, job progress in Postgres, CLI visibility, BearHost-safe workers, restore proof.

| Task | Summary |
|------|---------|
| TASK-001 | Inventory sign-off |
| TASK-002 | BearHost corpus-plane decision (locked) |
| TASK-003 | BearHost env + preflight |
| TASK-004 | `archive_exports` migration |
| TASK-004B | Natural key contract |
| TASK-005 | ManifestStore expansion |
| TASK-006 | Writer SHA-256 + sidecar |
| TASK-007 | Bronze VOD catalog v1 |
| TASK-008 | Bronze identity + crosswalk |
| TASK-010 | Global 7TV snapshot |
| TASK-016 | `archive_jobs` schema |
| TASK-016B | Reconcile `archive_jobs` ↔ `backfill_jobs` |
| TASK-017 | jobtracker library |
| TASK-018 | Wire jobtracker (bronze/emote/backfill) |
| TASK-019 | CLI `jobs` + `coverage` commands |
| TASK-031 | BearHost corpus smoke |
| TASK-031B | Restore drill smoke |

**Not in Release 1:** admin API/UI, Caddy admin routes, verify-blobs, silver chart JSON, tombstones/roster hive (TASK-009), emote diff/FFZ/BTTV, gold-lite/full, Grafana, proxy enable.

**Operator surface:** `go run ./cmd/backfill jobs list|show|retry-failed`, `coverage report`, `archive restore`.

## Release 2 — Admin HTTP + silver provenance + verify

| Task | Summary |
|------|---------|
| TASK-033 | Caddy admin routes |
| TASK-021 | `ADMIN_ARCHIVE_TOKEN` middleware |
| TASK-022 | Admin REST handlers |
| TASK-023 | Admin audit events |
| TASK-020 | verify-blobs |
| TASK-009 | Roster/tombstones/coverage blob |
| TASK-012 | Silver TT chart JSON |
| TASK-013 | Silver partial manifests |

**BearHost:** Prefer **CLI + SSH tunnel** for admin mutations until TLS; admin API over raw HTTP is sniffable even without UI.

## Release 3 — UI, observability, gold split, emote providers, proxy

| Task | Summary |
|------|---------|
| TASK-024 | Admin UI routes + token gate |
| TASK-025 | Admin UI panels |
| TASK-021B | Disable admin UI on public non-HTTPS |
| TASK-026–028 | Prometheus/Grafana observability profile |
| TASK-014–015 | Gold-lite/full split |
| TASK-011A | 7TV snapshot diff changelog (P1) |
| TASK-011B–D | Emote schema + FFZ/BTTV (P2) |
| TASK-029–030 | Proxy benchmark + gated routing |
| TASK-032 | PulseWire cold export (P2) |

**Admin UI:** Release 3 / P2 — backend + CLI must be stable first.

---

# 4. Implementation phases

## Phase 0 — Codebase discovery and verification

**Goal:** Confirm shipped surface; baseline BearHost compose merge; no code changes required for sign-off.

**Tasks:** TASK-001, TASK-002 (decision locked), TASK-003
**Files:** `migrations/00003*`, `internal/archive/*`, `deploy/docker-compose.bearhost-prod.yml`, `cmd/backfill/main.go`
**Migrations:** none
**Env:** none
**Acceptance:** Written status matrix (§2) signed off; `make bearhost-config-check` passes
**Rollback:** n/a
**Risks:** Stale docs mis-route agents — fix in Phase 11

## Phase 1 — Archive manifest foundation

**Goal:** Expand `archive_exports`, SHA-256 on upload, optional sidecar JSON, hive path helpers (dual-write legacy keys one release).

**Tasks:** TASK-004, TASK-004B, TASK-005, TASK-006
**Files:** `migrations/000035_archive_manifest_expand.up.sql`, `internal/archive/manifest.go`, `writer.go`, `internal/config/config.go`, `deploy/env/profile-archive.env`
**Migrations:** `000035_archive_manifest_expand`
**Env:** `ARCHIVE_CONTENT_HASH_ENABLED`, `ARCHIVE_WRITE_SIDECAR_MANIFEST`, `ARCHIVE_MANIFEST_SCHEMA_VERSION`, `ARCHIVE_PARSER_VERSION`
**Acceptance:** Re-export one stream; row has `content_sha256`, `tier`, `provider`; sidecar blob optional
**Rollback:** Disable hash/sidecar env; migration columns nullable
**Risks:** Wide row migration on large `archive_exports` — add columns nullable, backfill on re-export

## Phase 2 — Expanded Bronze corpus

**Goal:** VOD catalog v1, identity, crosswalk, dated roster, tombstones, bronze coverage blob.

**Tasks:** TASK-007, TASK-008, TASK-009
**Files:** `internal/archive/bronze_export.go` (new), `bronze_indexer.go`, `migrations/000037_bronze_state_expand.up.sql`
**Migrations:** extend `bronze_index_state`; optional `vod_catalog_state`, `channel_identity_state`
**Env:** `BRONZE_VOD_INDEX_SINCE_DAYS`, `BRONZE_VOD_INDEX_MAX_PAGES`, `BRONZE_IDENTITY_ENABLED`, `BRONZE_CROSSWALK_ENABLED`, `BRONZE_TOMBSTONE_ENABLED`
**Acceptance:** 5-channel bronze run-once → hive paths + manifest rows for catalog/identity/crosswalk
**Rollback:** `BRONZE_*_ENABLED=false`
**Risks:** Helix rate limits — keep `channelsPerTick` caps

## Phase 3 — Emote corpus expansion

**Goal:** Global 7TV snapshot (Release 1); diff changelog + FFZ/BTTV (Release 3).

**Tasks:** TASK-010 (R1); TASK-011A–D (R3, split)
**Files:** `emote_exporter.go`, `internal/emote/seeder/seeder.go`, `workers.go`
**Migrations:** none (reuse `archive_exports`)
**Env:** `EMOTE_GLOBAL_7TV_ENABLED`, `EMOTE_CHANGELOG_DIFF_ENABLED`, `EMOTE_MEDIA_CACHE_ENABLED=false`
**Acceptance (R1):** Global snapshot blob + manifest row
**Rollback:** Disable global/diff flags
**Risks:** FFZ/BTTV must not block Release 1 — keep 011C/D in Release 3

## Phase 4 — Silver provenance and raw TT artifacts

**Goal:** Normalized viewer export metadata, TT chart JSON, partial manifest status.

**Tasks:** TASK-012, TASK-013
**Files:** `exporter.go`, `sync.go`, `silver_manifest.go` (new), `backfill_worker.go`
**Env:** `SILVER_RAW_TT_HTML`, `SILVER_RAW_TT_CHART_JSON`, `SILVER_PARTIAL_MIN_COVERAGE`
**Acceptance:** One TT stream → rollups + HTML and/or chart JSON + `partial`/`complete` manifest
**Rollback:** Legacy paths remain readable
**Risks:** Large HTML — respect `SILVER_RAW_TT_MAX_BYTES`

## Phase 5 — Gold-lite / Gold-full split (Release 3)

**Goal:** Separate tiers; aggregates export when rollups exist; selective gold-full only.

**Storage vs scrape:** Gold-lite is **storage-light, not necessarily scrape-light**. If chat/emote minute rollups already exist in Postgres (post-silver/sync), export aggregates cheaply. If rollups do not exist, Gold-lite still requires GQL VOD chat fetch (same cost path as gold-full). Do **not** treat Gold-lite as safe for top-200 bulk unless `GOLD_LITE_REQUIRE_ROLLUPS=true` and queue/proxy/concurrency rules are explicit.

**Tasks:** TASK-014, TASK-015
**Files:** `internal/config/config.go`, `gold_rules.go`, `backfill_worker.go`, `chat_aggregate.go` (new)
**Env:** `GOLD_LITE_ENABLED`, `GOLD_LITE_REQUIRE_ROLLUPS`, `GOLD_FULL_ENABLED`, `GOLD_FULL_OPERATOR_ONLY`, thresholds
**Acceptance:** Gold-lite blob when rollups exist; gold-full requires explicit enqueue/rules
**Rollback:** `GOLD_FULL_ENABLED=false`; `GOLD_LITE_ENABLED=false`
**Risks:** GQL cost explosion if bulk enqueue without rollup gate — default gold-full off; gold-lite bulk operator-only

## Phase 6 — Durable job progress (Release 1)

**Goal:** `archive_jobs` / items / events + reconciliation with `backfill_jobs` + jobtracker wiring.

**Tasks:** TASK-016, TASK-016B, TASK-017, TASK-018, TASK-019
**Files:** `migrations/000036_archive_jobs.up.sql`, `migrations/000036b_backfill_job_link.up.sql`, `internal/archive/jobtracker/*`, workers, `cmd/backfill/main.go`
**Migrations:** `000036_archive_jobs`; optional `000036b` adds `backfill_jobs.archive_job_id`, `archive_job_items.backfill_job_id`
**Env:** `ARCHIVE_JOB_PROGRESS_ENABLED`, `ARCHIVE_JOB_HEARTBEAT_INTERVAL`, `ARCHIVE_JOB_STALE_AFTER`
**Acceptance:** Bronze run creates parent job + items; silver enqueue sets `backfill_jobs.archive_job_id`; `jobs show` matches queue reality
**Rollback:** `ARCHIVE_JOB_PROGRESS_ENABLED=false`
**Risks:** Postgres write amplification — batch item updates; drift if 016B skipped

## Phase 7 — Coverage reporting and blob verification

**Goal:** Tier filters, verify-blobs, stale channels, daily coverage snapshots.

**Tasks:** TASK-019, TASK-020
**Files:** `coverage_report.go`, `internal/archive/verify.go` (new), `cmd/backfill/main.go`
**Migrations:** `000038_archive_coverage_snapshots.up.sql`
**Env:** `CORPUS_COVERAGE_SNAPSHOT_ENABLED`, `CORPUS_BLOB_VERIFY_INTERVAL`
**Acceptance:** `coverage verify-blobs` reports orphans both directions
**Rollback:** Disable verify cron
**Risks:** Azure list API cost — sample or paginate

## Phase 8 — Admin API (Release 2) and admin UI (Release 3)

**Goal (R2):** Protected `/v1/admin/archive/*` via Caddy + `ADMIN_ARCHIVE_TOKEN` — CLI remains primary on BearHost.
**Goal (R3):** Operator browser pages; disabled on public non-HTTPS (TASK-021B).

**Tasks (R2):** TASK-033, TASK-021, TASK-022, TASK-023
**Tasks (R3):** TASK-024, TASK-025, TASK-021B
**Files:** `deploy/Caddyfile`, `deploy/Caddyfile.bearhost`, `admin_archive_handler.go`, `frontend/src/pages/admin/*`, `App.tsx`
**Env:** `ADMIN_ARCHIVE_ENABLED`, `ADMIN_ARCHIVE_REQUIRE_TOKEN`, `ADMIN_ARCHIVE_TOKEN`, `VITE_ADMIN_ARCHIVE_UI_ENABLED` (default false on BearHost)
**Acceptance (R2):** Caddy smoke 401-not-404; curl with token returns job list
**Acceptance (R3):** UI on localhost HTTPS or paste gate; BearHost HTTP shows CLI/SSH instructions only
**Rollback:** `ADMIN_ARCHIVE_ENABLED=false`; `VITE_ADMIN_ARCHIVE_UI_ENABLED=false`
**Risks:** Token over HTTP; UI/API drift — CLI is SoT until R3 stable

## Phase 9 — Prometheus/Grafana optional observability

**Goal:** `observability` compose profile, archive metrics, dashboard, alerts.

**Tasks:** TASK-026, TASK-027, TASK-028
**Files:** `docker-compose.observability.yml`, `metrics/archive_jobs.go`, `grafana/dashboards/streamclone-archive.json`, `prometheus/alerts/archive.yml`
**Env:** `OBSERVABILITY_PROFILE_ENABLED` (doc only)
**Acceptance:** Profile starts on low-RAM host; `/metrics` not on public Caddy
**Rollback:** Don't enable profile
**Risks:** RAM on 8 GB VPS

## Phase 10 — Proxy/egress gating and production rollout

**Goal:** Benchmark sign-off, optional `ANALYTICS_TT_USE_PROXY`, Silver concurrency caps.

**Tasks:** TASK-029, TASK-030
**Files:** `sync.go`, `internal/config/config.go`, `proxy-benchmark.md`
**Env:** `ANALYTICS_TT_USE_PROXY` (new, default false)
**Acceptance:** Operator sign-off doc row; proxy off by default in prod env
**Rollback:** Env false
**Risks:** TT block on datacenter IP

## Phase 11 — Docs, smoke tests, rollback

**Goal:** `bearhost-corpus-smoke.sh`, update steering, acceptance automation.

**Tasks:** TASK-031, TASK-031B, TASK-032
**Files:** `scripts/bearhost-corpus-smoke.sh`, `docs/bearhost-production.md`, `.kiro/steering/analytics.md`
**Acceptance:** Smoke checklist §13 passes on staging
**Rollback:** Documented in [archive-observability.md](archive-observability.md)

---

# 5. Concrete task tickets

## TASK-001: Baseline archive inventory sign-off

Priority: P0
Type: docs
Depends on: —
Goal: Lock shipped vs planned matrix for all agents.
Context: Subagent verification 2026-06-20; docs disagree on Phase completion.
Implementation steps: Run grep/migration list; document in §2; link from `requirements.md`.
Files to inspect/change: `migrations/000030-000034`, `internal/archive/`, `cmd/analytics/main.go`
Database changes: none
Env/config changes: none
Acceptance criteria: §2 table reviewed; no "planned" row marked shipped without code proof
Test commands: `go test ./internal/archive/...`; `make bearhost-config-check`
Rollback: n/a
Risks: Agents start wrong phase
Notes for agents: Output is this doc §2; do not code.

## TASK-002: BearHost corpus-plane decision (locked)

Priority: P0
Type: infra / docs
Depends on: TASK-001
Goal: **Document the locked BearHost worker model** — no A/B fork for agents.
Context: `scripts/bearhost-deploy-phased.sh` always merges `profile-archive.env` (`ARCHIVE_ENABLED=true`). `deploy/docker-compose.bearhost-prod.yml` disables archive flags on **`analytics` only**; `analytics-workers` inherits enabled flags and `cmd/analytics/main.go` exits without Azure secret.
Evidence: `deploy/env/profile-archive.env:5`, `deploy/docker-compose.bearhost-prod.yml:16-23`, `scripts/bearhost-deploy-phased.sh:17-21`, `cmd/analytics/main.go` (Azure init when `ARCHIVE_ENABLED=true`).

**Decision (locked):**

| Service | Role | Corpus flags |
|---------|------|--------------|
| `analytics` | HTTP API (streams, sync status, admin routes) | **Always off** on BearHost (`ARCHIVE_ENABLED=false`, bronze/backfill/gold off) |
| `analytics-workers` | Long-running corpus plane | **On only after preflight** (Azure secret file + Twitch OAuth client creds); otherwise explicitly **off** (no crash loop) |

Implementation steps:

1. Add § corpus plane to `docs/bearhost-production.md` stating the table above.
2. Specify preflight gate in `scripts/bearhost-deploy-phased.sh` or `scripts/bearhost-smoke.sh` (fail closed or set `CORPUS_WORKERS_ENABLED=0`).
3. Document operator steps: install secret → run preflight → enable workers corpus env.

Files to inspect/change: `deploy/docker-compose.bearhost-prod.yml`, `deploy/env/profile-bearhost-prod.env`, `docs/bearhost-production.md`, `scripts/bearhost-deploy-phased.sh`
Database changes: none
Env/config changes: `CORPUS_WORKERS_ENABLED` (or equivalent explicit override on `analytics-workers` only)
Acceptance criteria: Written decision in bearhost runbook; TASK-003 implements without reopening fork
Test commands: `make bearhost-config-check-local`
Rollback: Revert doc + env pattern
Risks: Accidental double workers if both services run corpus — prevented by analytics disable
Notes for agents: **Do not** choose option A (disable workers entirely as default product path). Corpus plane lives on workers.

## TASK-003: Implement BearHost corpus-plane env + preflight

Priority: P0
Type: infra
Depends on: TASK-002
Goal: Workers-as-corpus-plane with preflight; analytics API-only; no crash loop without Azure secret.
Context: Implements locked TASK-002 decision.
Implementation steps:

1. `analytics`: keep explicit disable of `ARCHIVE_ENABLED`, `BRONZE_ENABLED`, `BACKFILL_ENABLED`, `GOLD_BACKFILL_ENABLED`, `TIER0_ENABLED`, `ARCHIVE_PG_DUMP_NIGHTLY`.
2. `analytics-workers`: set corpus flags from `CORPUS_WORKERS_ENABLED` (default `false` in `profile-bearhost-prod.env` until operator enables post-preflight) OR compose override that only turns on when preflight script exports `CORPUS_WORKERS_ENABLED=1`.
3. Preflight checks: `test -f "$ARCHIVE_AZURE_CONNECTION_STRING_FILE"`; Twitch client id/secret present in `.env`.
4. Extend `scripts/bearhost-smoke.sh` with corpus preflight gate and document in runbook.

Files to inspect/change: `deploy/docker-compose.bearhost-prod.yml`, `deploy/env/profile-bearhost-prod.env`, `scripts/bearhost-smoke.sh`, `scripts/bearhost-deploy-phased.sh`, `docs/bearhost-production.md`
Database changes: none
Env/config changes: `CORPUS_WORKERS_ENABLED`, per-service archive flag overrides
Acceptance criteria: Without secret → workers healthy with corpus off; with secret + creds + enabled → workers run bronze/backfill; analytics never runs corpus workers
Test commands: `bash scripts/bearhost-smoke.sh`; `docker compose … config` diff
Rollback: `CORPUS_WORKERS_ENABLED=false`
Risks: Operator forgets to enable after secret install — document in §12
Notes: Do not commit secrets.

## TASK-004: Migration 000035 expand archive_exports

Priority: P0
Type: backend
Depends on: TASK-001
Goal: Add manifest columns per corpus-requirements § Manifest model.
Context: MAN-C2; keep `gcs_uri` name.
Implementation steps: Add nullable columns; widen status check; indexes on `(tier, channel_login)`, `(stream_id, tier)`; forward-only `.down.sql` drops columns only.
Files to inspect/change: `migrations/000035_archive_manifest_expand.up.sql`, `.down.sql`
Database changes: `artifact_id UUID`, `tier`, `provider`, `channel_login`, `channel_id`, `stream_id`, `vod_id`, `source_url`, `content_sha256`, `uncompressed_size_bytes`, rename map `byte_size`→document as compressed, `failure_reason`, `metadata JSONB`; extend `export_status` check
Env/config changes: none
Acceptance criteria: Migrate on dev DB; existing rows remain readable
Test commands: `make migrate` (verify in code); `go test ./internal/archive/...`
Rollback: Down migration in dev only
Risks: Constraint migration on prod — use nullable + app mapping `confirmed`→`complete`

## TASK-004B: Artifact natural key specification

Priority: P0 · **Release 1**
Type: backend / docs
Depends on: TASK-004
Goal: Define `natural_key` format for every `archive_exports` artifact type **before** writer implementation (TASK-005+).
Context: Upsert PK is `(artifact_type, natural_key)`. Shipped keys are inconsistent (`vod_index:{login}`, `rollups:{stream_id}`, `emote_snapshot:{provider}:{login}:{date}`). Without a contract, hive path moves create duplicate blobs and orphan manifest rows.
Implementation steps:

1. Add `docs/scraping-archive/artifact-natural-keys.md` (or section in corpus-requirements) with table: `artifact_type` → `natural_key` pattern → blob path → overwrite behavior.
2. Document **legacy → canonical** mapping and one-release dual-write where keys change.
3. Add unit tests per key helper (pattern: existing `rollupsNaturalKey` tests).

**Canonical examples (target):**

| artifact_type | natural_key pattern | On re-run |
|---------------|---------------------|-----------|
| bronze_vod_catalog | `{login}:{date}` | overwrite same date |
| channel_identity | `{channel_id}:{date}` | overwrite same date |
| provider_crosswalk | `{login}:{date}` | overwrite same date |
| viewer_rollup | `{stream_id}:twitchtracker` | overwrite (canonical stream artifact) |
| tt_detail_html | `{stream_id}:twitchtracker` | overwrite |
| tt_chart_json | `{stream_id}:{fetched_at}` | version by fetch time |
| emote_snapshot | `{provider}:{login}:{date}` | overwrite same date |
| emote_snapshot_global | `7tv:global:{date}` | overwrite same date |
| emote_changelog | `{provider}:{login}:{event}:{unix}` | append (immutable event) |
| gold_lite | `{stream_id}` | overwrite |
| gold_full | `{stream_id}:part:{part_no}` | overwrite part |
| pulsewire_raw | `{source}:{date}:{part_no}` | overwrite part |

**Rules:**

- Daily snapshots → versioned by **date** in key.
- Per-stream canonical artifacts → idempotent on `stream_id` + type.
- Raw fetch artifacts → versioned by **fetched_at** (or part number).

Files to inspect/change: `docs/scraping-archive/artifact-natural-keys.md` (new), `internal/archive/natural_keys.go` (new), `exporter.go`, `emote_exporter.go`, `incremental_worker.go`
Database changes: none
Env/config changes: none
Acceptance criteria: Documented table signed off; legacy keys mapped; tests for each new helper
Test commands: `go test ./internal/archive/... -run NaturalKey`
Rollback: n/a (docs); writers fall back to legacy keys via env if needed
Risks: Premature path freeze — lock keys + semantics, not optional hive experiments
Notes: **Blocks TASK-005** — do not expand ManifestStore writers until 004B merges.

## TASK-005: ExportRecord + ManifestStore upsert expansion

Priority: P0 · **Release 1**
Type: backend
Depends on: TASK-004, TASK-004B
Goal: Write new manifest fields from writers.
Context: `internal/archive/manifest.go` ExportRecord today is minimal.
Implementation steps: Extend struct; update Upsert SQL; normalize status enum.
Files to inspect/change: `internal/archive/manifest.go`, `manifest_test.go`
Database changes: uses 000035
Env/config changes: none
Acceptance criteria: Unit tests pass; upsert sets tier/provider when provided
Test commands: `go test ./internal/archive/... -run Manifest`
Rollback: Old code ignores new columns
Risks: SQL column count drift

## TASK-006: Writer SHA-256 + optional sidecar manifest

Priority: P0
Type: backend
Depends on: TASK-005
Goal: Content hash and sidecar JSON on upload.
Context: MAN-C1, WR1 shipped partially (ETag only).
Implementation steps: Hash gzip bytes in `putGzip`; add `WriteSidecarManifest`; config gates.
Files to inspect/change: `internal/archive/writer.go`, `internal/config/config.go`, `deploy/env/profile-archive.env`
Database changes: none
Env/config changes: `ARCHIVE_CONTENT_HASH_ENABLED`, `ARCHIVE_WRITE_SIDECAR_MANIFEST`, `ARCHIVE_PARSER_VERSION`
Acceptance criteria: Upload sets `content_sha256`; sidecar at `manifests/…` when enabled
Test commands: `go test ./internal/archive/... -run Writer`
Rollback: Env false
Risks: CPU on large blobs — hash streamed

## TASK-007: Bronze VOD catalog v1 + Helix pagination

Priority: P0
Type: backend
Depends on: TASK-006
Goal: Rich catalog lines + dated hive path.
Context: BR-C3; today `bronzeHelixVODLimit=80`.
Implementation steps: Paginate Helix in `exportHelixIndex`; emit `vod_catalog/v1` JSONL; dual-write legacy key; manifest tier=bronze.
Files to inspect/change: `internal/analytics/bronze_indexer.go`, `internal/archive/bronze_export.go` (new), `writer.go` blob keys
Database changes: none
Env/config changes: `BRONZE_VOD_INDEX_SINCE_DAYS`, `BRONZE_VOD_INDEX_MAX_PAGES`
Acceptance criteria: Catalog blob has `availability`, `rawHelix`, `schemaVersion`
Test commands: `go run ./cmd/backfill bronze run-once` (5 channels)
Rollback: Legacy path only via env
Risks: Helix 429 — backoff in indexer

## TASK-008: Bronze identity + crosswalk exporters

Priority: P0
Type: backend
Depends on: TASK-006
Goal: Identity and crosswalk blobs per login.
Context: Bronze artifacts #2–#3 in corpus-requirements.
Implementation steps: Helix Get Users + 7TV user lookup; write JSON; upsert manifest.
Files to inspect/change: `bronze_export.go`, `bronze_indexer.go`, optional `000037` state columns
Database changes: optional `last_identity_at`, `last_crosswalk_at` on `bronze_index_state`
Env/config changes: `BRONZE_IDENTITY_ENABLED`, `BRONZE_CROSSWALK_ENABLED`
Acceptance criteria: Blobs under `channels/identity/…`, `channels/crosswalk/…`
Test commands: bronze run-once + Azure list
Rollback: Env false
Risks: Missing 7TV user — write partial manifest

## TASK-009: Roster hive path + tombstones + bronze coverage blob

Priority: P1
Type: backend
Depends on: TASK-007
Goal: Dated roster snapshot; VOD deletion detection; daily bronze coverage JSON.
Implementation steps: Write `rosters/tier0/date=…`; maintain `vod_catalog_state`; diff for tombstones; export `coverage/bronze/date=…`.
Files to inspect/change: `bronze_export.go`, `bronze_indexer.go`, `coverage_report.go`
Database changes: optional `vod_catalog_state` table in 000037
Env/config changes: `BRONZE_TOMBSTONE_ENABLED`, `BRONZE_COVERAGE_EXPORT_ENABLED`
Acceptance criteria: Tombstone when VOD disappears from catalog
Test commands: Unit test diff logic
Rollback: Disable tombstone env
Risks: False tombstones on Helix pagination gaps

## TASK-010: Global 7TV snapshot export

Priority: P0
Type: backend
Depends on: TASK-006
Goal: `emotes/snapshots/provider=7tv/login=global/…`.
Context: EM-C2; seeder already fetches global in `fetchSevenTVGlobal`.
Implementation steps: Add `ExportGlobalSevenTVSnapshot`; weekly worker calls it.
Files to inspect/change: `internal/archive/emote_exporter.go`, `internal/emote/seeder/seeder.go`, `workers.go`
Database changes: none
Env/config changes: `EMOTE_GLOBAL_7TV_ENABLED=true`
Acceptance criteria: Global blob + manifest row
Test commands: Trigger weekly worker or CLI (TASK-018)
Rollback: Env false
Risks: Large global set — gzip JSONL

## TASK-011A: 7TV snapshot diff changelog

Priority: P1 · **Release 3**
Type: backend
Depends on: TASK-010
Goal: Diff consecutive 7TV snapshots → add/remove changelog lines.
Context: EM-C1; today EventAPI append only (`AppendChangelog`).
Implementation steps: Load prior snapshot from manifest by natural key; diff emote ids/names; write changelog JSONL; optional link to `archive_job_items`.
Files to inspect/change: `emote_exporter.go`, `pgx_emote_snapshot.go`
Database changes: none
Env/config changes: `EMOTE_CHANGELOG_DIFF_ENABLED`
Acceptance criteria: Simulated emote add after second snapshot produces diff event
Test commands: `go test ./internal/archive/... -run EmoteDiff`
Rollback: Diff off; keep EventAPI append
Risks: First snapshot has no prior — skip diff

## TASK-011B: Provider-agnostic emote snapshot schema

Priority: P2 · **Release 3**
Type: backend
Depends on: TASK-010
Goal: Normalize emote snapshot JSON shape across providers before FFZ/BTTV export.
Implementation steps: Shared struct + `schemaVersion`; migrate 7TV writer; document in artifact-natural-keys.
Files to inspect/change: `emote_exporter.go`, `internal/archive/emote_snapshot_schema.go` (new)
Database changes: none
Env/config changes: none
Acceptance criteria: 7TV snapshot validates against shared schema test fixture
Test commands: `go test ./internal/archive/... -run EmoteSchema`
Rollback: Provider-specific shapes until 011C/D
Risks: None for Release 1 — **must not block** bronze/manifest track

## TASK-011C: FFZ per-channel snapshot export

Priority: P2 · **Release 3**
Type: backend
Depends on: TASK-011B
Goal: Weekly FFZ roster snapshots for top-N channels.
Implementation steps: Reuse emote seeder FFZ path; write blob + manifest with keys from TASK-004B.
Files to inspect/change: `emote_exporter.go`, `workers.go`
Database changes: none
Env/config changes: `EMOTE_FFZ_SNAPSHOT_ENABLED=false` default
Acceptance criteria: FFZ blob for test channel
Test commands: manual worker tick
Rollback: Env false
Risks: Low priority — do not pull into Release 1

## TASK-011D: BTTV per-channel snapshot export

Priority: P2 · **Release 3**
Type: backend
Depends on: TASK-011B
Goal: Weekly BTTV roster snapshots for top-N channels.
Implementation steps: Same as 011C for BTTV provider.
Files to inspect/change: `emote_exporter.go`, `workers.go`
Database changes: none
Env/config changes: `EMOTE_BTTV_SNAPSHOT_ENABLED=false` default
Acceptance criteria: BTTV blob for test channel
Test commands: manual worker tick
Rollback: Env false
Risks: Low priority

## TASK-012: Silver TT chart JSON capture

Priority: P1
Type: backend
Depends on: TASK-006
Goal: Store semi-raw chart payload for re-parse.
Context: SV-C1; HTML export partial today.
Implementation steps: Extract chart JSON in sync path; upload `raw/twitchtracker/…/chart.json.gz`.
Files to inspect/change: `internal/analytics/sync.go`, `internal/archive/exporter.go`
Database changes: none
Env/config changes: `SILVER_RAW_TT_CHART_JSON`, `SILVER_RAW_TT_MAX_BYTES`
Acceptance criteria: Chart blob exists for test stream
Test commands: Silver backfill one stream
Rollback: Env false
Risks: Parser fragility — store raw JSON

## TASK-013: Silver partial manifests + hive rollup path

Priority: P1
Type: backend
Depends on: TASK-012
Goal: `partial` status with coverage_ratio; hive rollup key.
Context: SV-C2.
Implementation steps: Compute coverage from rollups vs duration; dual-write rollup path; sidecar `manifests/…/silver.json`.
Files to inspect/change: `exporter.go`, `silver_manifest.go` (new), `backfill_worker.go`
Database changes: none
Env/config changes: `SILVER_PARTIAL_MIN_COVERAGE`
Acceptance criteria: Low coverage → manifest `partial`
Test commands: `go test ./internal/analytics/...`
Rollback: Legacy rollup path only
Risks: Misclassified partial

## TASK-014: Gold-lite aggregate export

Priority: P1 · **Release 3**
Type: backend
Depends on: TASK-006
Goal: Minute chat/emote aggregates blob without full message export **when rollups exist**.
Context: GD-C1 gold-lite tier. **Storage-light ≠ scrape-light** — see Phase 5.
Implementation steps:

1. **Primary path:** Derive aggregates from existing Postgres minute rollups → upload `rollups/chat/…/minute.jsonl.gz`.
2. **Fallback path (explicit opt-in):** If rollups missing and `GOLD_LITE_REQUIRE_ROLLUPS=false`, run GQL fetch (same cost as gold-full) — log warning.
3. Default `GOLD_LITE_REQUIRE_ROLLUPS=true` on BearHost.

Files to inspect/change: `internal/archive/chat_aggregate.go` (new), `exporter.go`, `backfill_worker.go`
Database changes: none
Env/config changes: `GOLD_LITE_ENABLED`, `GOLD_LITE_REQUIRE_ROLLUPS=true`
Acceptance criteria: Gold-lite blob for stream **with** rollups; no GQL when REQUIRE_ROLLUPS=true and rollups absent (skip/fail item)
Test commands: Export after sync with rollups; export without rollups → skipped
Rollback: `GOLD_LITE_ENABLED=false`
Risks: Accidental GQL bulk — never auto-enqueue gold-lite for top-200 without operator approval

## TASK-015: Gold-full vs gold-lite tier split in backfill

Priority: P1
Type: backend
Depends on: TASK-014
Goal: Separate queue tiers and env gates.
Context: Today single `gold` tier in `backfill_jobs`.
Implementation steps: Add `gold_lite`/`gold_full` tier strings; update rules engine; `GOLD_FULL_OPERATOR_ONLY`.
Files to inspect/change: `gold_rules.go`, `gold_enqueuer.go`, `backfill_worker.go`, `migrations/000033` (optional new tier values only — no schema change)
Database changes: none if tier is free text
Env/config changes: `GOLD_FULL_ENABLED`, `GOLD_FULL_*` thresholds
Acceptance criteria: Auto-enqueue never gold-full when OPERATOR_ONLY=true
Test commands: `go run ./cmd/backfill gold eval --stream-id=…`
Rollback: `GOLD_FULL_ENABLED=false`
Risks: Cost if thresholds too low

## TASK-016: Migration 000036 archive_jobs + items + events

Priority: P0 · **Release 1**
Type: backend
Depends on: TASK-001
Goal: Durable job progress schema (Part II).
Context: archive-observability.md § DB schema.
Implementation steps: Create tables + indexes per spec; **keep `backfill_jobs` table** — link in TASK-016B.
Files to inspect/change: `migrations/000036_archive_jobs.up.sql`
Database changes: `archive_jobs`, `archive_job_items`, optional `archive_job_events`
Env/config changes: none
Acceptance criteria: Migrate clean; FK cascade on items
Test commands: apply migration on dev
Rollback: Down drops new tables only
Risks: UUID gen on PG

## TASK-016B: Reconcile archive_jobs and backfill_jobs

Priority: P0 · **Release 1**
Type: backend
Depends on: TASK-016
Goal: Prevent drift between operator-visible `archive_jobs` and existing `backfill_jobs` sync queue.
Context: Without a bridge, UI/CLI can show “archive job running” while `backfill_jobs` queue disagrees.

**Locked model:**

| Layer | Table | Role |
|-------|-------|------|
| Parent | `archive_jobs` | Operator-visible job (bronze batch, emote weekly, silver enqueue batch) |
| Progress | `archive_job_items` | Per-login / per-stream / per-artifact item status |
| Worker queue | `backfill_jobs` | Low-level sync/export queue (unchanged semantics) |

**Not** Option A (wrap only) or pure Option C (fully separate with no links).

Implementation steps:

1. Migration `000036b_backfill_job_link.up.sql`:
   - `backfill_jobs.archive_job_id UUID NULL REFERENCES archive_jobs(id)`
   - `archive_job_items.backfill_job_id BIGINT NULL REFERENCES backfill_jobs(id)`
   - Index on `backfill_jobs(archive_job_id)` where not null.
2. When enqueueing silver/gold from a parent job, set `backfill_jobs.archive_job_id`.
3. When backfill worker picks up job, update linked `archive_job_items` status.
4. `jobs show` and CLI aggregate: parent counters derived from items; item row shows linked `backfill_job_id` + queue status.
5. Jobs with no backfill (bronze identity, global 7TV) use items only — `backfill_job_id` stays null.

Files to inspect/change: `migrations/000036b_backfill_job_link.up.sql`, `internal/archive/jobtracker/`, `backfill_worker.go`, `bronze_indexer.go`, `cmd/backfill/main.go`
Database changes: nullable FKs above
Env/config changes: none
Acceptance criteria: Bronze parent job → N items; silver enqueue from job → each `backfill_jobs` row has `archive_job_id`; `jobs show` matches `backfill status` for linked streams
Test commands: Enqueue batch; `jobs show`; query `backfill_jobs` for FK
Rollback: Nullable columns; jobtracker works without FK population
Risks: **Blocks TASK-018** — wire jobtracker only after 016B lands
Notes: Rejected alternatives: Option A (archive_jobs wraps without item FK — opaque); Option C (no link — guaranteed drift).

## TASK-017: internal/archive/jobtracker library

Priority: P0 · **Release 1**
Type: backend
Depends on: TASK-016, TASK-016B
Goal: Tracker, ItemHandle, heartbeat, stale detector.
Context: OBS-C1–C3.
Implementation steps: Postgres store; heartbeat goroutine; Finish/Partial/Failed semantics.
Files to inspect/change: `internal/archive/jobtracker/*.go`, tests
Database changes: uses 000036
Env/config changes: `ARCHIVE_JOB_*`
Acceptance criteria: Unit tests for counter updates and stale marking
Test commands: `go test ./internal/archive/jobtracker/...`
Rollback: Library unused if env false
Risks: Long TT scrape vs stale timeout — tune `ARCHIVE_JOB_STALE_AFTER`

## TASK-018: Wire jobtracker to bronze, backfill, emote, coverage workers

Priority: P0 · **Release 1**
Type: backend
Depends on: TASK-017, TASK-016B, TASK-007, TASK-010
Goal: Every long-running job writes Postgres progress; backfill queue stays linked to parent job.
Context: Part II worker wiring; must populate `archive_job_id` / `backfill_job_id` per TASK-016B.
Implementation steps: Wrap `BronzeIndexer.RunOnce`, backfill batch, emote weekly, coverage report in jobs; update item status from `backfill_worker` when FK set.
Files to inspect/change: `bronze_indexer.go`, `backfill_worker.go`, `workers.go`, `coverage_report.go`, `cmd/analytics/archive_workers.go`
Database changes: none
Env/config changes: `ARCHIVE_JOB_PROGRESS_ENABLED=true`
Acceptance criteria: §13 resume/retry + job/backfill reconciliation tests pass
Test commands: Kill worker mid-bronze; resume; verify FK on enqueued silver
Rollback: Env false
Risks: Merge conflicts with exporter changes

## TASK-019: CLI jobs + coverage subcommands

Priority: P0 · **Release 1**
Type: backend
Depends on: TASK-017, TASK-016B
Goal: Operator CLI — **primary admin surface until Release 3 UI**.
Context: cmd/backfill today lacks `jobs *`. Release 1 ships `jobs list|show|retry-failed|resume|cancel` and `coverage report` (verify-blobs in Release 2).
Implementation steps: Add job subcommands; human-readable progress + JSON; show linked `backfill_job_id` per item when present.
Files to inspect/change: `cmd/backfill/main.go`, `internal/analytics/jobs_cli.go` (new)
Database changes: none
Env/config changes: none
Acceptance criteria: `jobs show` counters match `archive_job_items` and linked `backfill_jobs`
Test commands: `go run ./cmd/backfill jobs list`; `jobs show --job-id=…`
Rollback: Remove subcommands
Risks: None

## TASK-020: verify-blobs Postgres ↔ Azure reconcile

Priority: P1 · **Release 2**
Type: backend
Depends on: TASK-006, TASK-019
Goal: Detect orphan index rows and orphan blobs.
Context: COV-C1 verify-blobs.
Implementation steps: Implement `internal/archive/verify.go`; list/compare hashes; CLI hook.
Files to inspect/change: `verify.go`, `cmd/archive/main.go`, `cmd/backfill/main.go`
Database changes: optional `000038_archive_coverage_snapshots`
Env/config changes: `CORPUS_BLOB_VERIFY_INTERVAL`
Acceptance criteria: Reports missing both directions
Test commands: `go run ./cmd/backfill coverage verify-blobs`
Rollback: Disable command
Risks: Full container list expensive — scope to manifest keys

## TASK-033: Caddy route `/v1/admin/*` to analytics

Priority: P0 · **Release 2**
Type: infra
Depends on: TASK-001
Goal: Ensure admin archive API reaches **analytics**, not metadata, on local Caddy and BearHost.
Context: Release 1 uses CLI only — this task gates Release 2 admin HTTP, not Release 1 corpus work.
Implementation steps:

1. Add `@admin_archive` matcher **before** `@metadata`:
   - `path /v1/admin/archive /v1/admin/archive/* /v1/admin/system /v1/admin/system/*`
2. `reverse_proxy @admin_archive analytics:8080` (same timeouts as `@analytics` if needed).
3. Mirror in `deploy/Caddyfile` (TLS/local dev parity).
4. Add smoke: curl through Caddy `:8090` or BearHost `:80` with `X-Admin-Archive-Token` → analytics handler (401 without token is OK; must **not** hit metadata 404).
5. Update `scripts/bearhost-smoke.sh` and `make smoke` docs.

Files to inspect/change: `deploy/Caddyfile`, `deploy/Caddyfile.bearhost`, `scripts/bearhost-smoke.sh`, `docs/bearhost-production.md`
Database changes: none
Env/config changes: none
Acceptance criteria: `curl -s -o /dev/null -w '%{http_code}' http://legacy-rollback-host/v1/admin/archive/jobs` returns **401** (auth middleware) not **404** from metadata
Test commands: `make bearhost-config-check`; manual curl via Caddy
Rollback: Remove `@admin_archive` block
Risks: Route order — admin block must precede `@metadata`
Notes: **Blocks TASK-022** (Release 2). Not required for Release 1 CLI.

## TASK-021: Admin archive auth middleware (`ADMIN_ARCHIVE_TOKEN`)

Priority: P0 · **Release 2**
Type: backend
Depends on: TASK-001
Goal: Gate `/v1/admin/*` with a **dedicated** archive operator token — not public setup-control.
Context: `SETUP_CONTROL_TOKEN` is emitted in public `config.js` (`frontend/docker-entrypoint.d/40-streamclone-config.sh:17`). `docs/security.md` treats it as browser-visible on non-loopback origins. BearHost (`legacy-rollback-host`) must **not** reuse it for archive admin mutations. PulseWire uses `X-Streamclone-Setup-Token` for a different surface — archive admin is separate.
Implementation steps:

1. Add `ADMIN_ARCHIVE_TOKEN` env (and optional `ADMIN_ARCHIVE_TOKEN_FILE` on BearHost secrets mount).
2. Middleware validates header **`X-Admin-Archive-Token`** (constant in shared Go + TS helper).
3. **Do not** read `SETUP_CONTROL_TOKEN` for archive admin routes.
4. **Do not** add `ADMIN_ARCHIVE_TOKEN` to frontend compose env or `config.js` on BearHost/prod.
5. Localhost dev: token may live in `.env` for curl/Playwright only.
6. Rate-limit admin POSTs (10/min per token).

Files to inspect/change: `internal/analytics/admin_auth.go` (new), `internal/config/config.go`, `deploy/env/profile-bearhost-prod.env` (document: token server-side only)
Database changes: none
Env/config changes: `ADMIN_ARCHIVE_ENABLED`, `ADMIN_ARCHIVE_REQUIRE_TOKEN`, `ADMIN_ARCHIVE_TOKEN` / `_FILE`
Acceptance criteria: 401 without header; 200 with valid token via Caddy after TASK-033
Test commands: `curl -H 'X-Admin-Archive-Token: …' http://localhost:8090/v1/admin/archive/jobs`
Rollback: `ADMIN_ARCHIVE_ENABLED=false`
Risks: Token committed to repo — use secrets file only; never frontend env on public host

## TASK-022: Admin archive REST handlers (jobs + coverage + queues)

Priority: P0 · **Release 2**
Type: backend
Depends on: TASK-021, TASK-033, TASK-016, TASK-016B
Goal: Implement API plan §8 endpoints.
Context: API-C1; handlers mount on analytics; Caddy must route first (TASK-033).
Implementation steps: Handlers read Postgres; computed `progressRatio`, `elapsedSeconds`; audit log on POST.
Files to inspect/change: `internal/analytics/admin_archive_handler.go`, `api.go` Routes mount
Database changes: none
Env/config changes: none
Acceptance criteria: JSON shapes match §8 examples; reachable through Caddy
Test commands: curl admin jobs list with `X-Admin-Archive-Token`
Rollback: Disable routes
Risks: Pagination on large item lists

## TASK-023: Admin audit events for operator actions

Priority: P1
Type: backend
Depends on: TASK-022
Goal: Audit retry/resume/cancel/enqueue.
Context: API-C2 audit requirement.
Implementation steps: Insert into `story_operator_actions` or new `admin_audit_events` table (prefer existing if schema fits).
Files to inspect/change: verify `story_operator_actions` schema in migrations
Database changes: optional small migration if needed
Env/config changes: none
Acceptance criteria: POST retry creates audit row
Test commands: API test
Rollback: Skip audit insert
Risks: PII in audit message — sanitize

## TASK-021B: Disable admin UI on public non-HTTPS origins

Priority: P0 · **Release 3** (when TASK-024 ships)
Type: security / frontend / infra
Depends on: TASK-021, TASK-024
Goal: Prevent archive admin token entry over raw HTTP on BearHost (`legacy-rollback-host`, no TLS).
Context: Token in browser over HTTP is sniffable; disabling UI reduces accident risk. **Does not secure admin API** — pair with CLI/tunnel-only guidance for Release 2 API on BearHost.
Implementation steps:

1. Detect non-loopback + `http:` origin (or `VITE_ADMIN_ARCHIVE_UI_ENABLED=false` default on BearHost).
2. `/admin/*` routes show static page: use CLI commands + SSH tunnel instructions — no token paste field.
3. Localhost / HTTPS: show `AdminTokenGate` paste UI.

Files to inspect/change: `frontend/src/components/admin/AdminTokenGate.tsx`, `frontend/src/pages/admin/*`, `deploy/env/profile-bearhost-prod.env`
Database changes: none
Env/config changes: `VITE_ADMIN_ARCHIVE_UI_ENABLED=false` on BearHost
Acceptance criteria: `http://legacy-rollback-host/admin/archive` shows CLI help, not token input; localhost shows gate
Test commands: manual browser; `npm run build`
Rollback: Enable UI env for staging with TLS only
Risks: Operators expect UI — document CLI as Release 1–2 primary

## TASK-024: Frontend admin routes + manual token gate

Priority: P2 · **Release 3**
Type: frontend
Depends on: TASK-021, TASK-033, TASK-022
Goal: `/admin/archive`, `/admin/jobs`, `/admin/coverage` — **after CLI + admin API stable**.
Context: Nice-to-have; introduces auth, Caddy, browser token handling, frontend/API drift. Release 1–2 operators use CLI.
Implementation steps:

1. Routes in `App.tsx`; guard component checks sessionStorage for `adminArchiveToken` (operator paste).
2. Token entry screen: password-style input + “Save for session” → `sessionStorage` only; clear on tab close optional.
3. API hook sends `X-Admin-Archive-Token` header from sessionStorage — never from `config.js`.
4. Localhost: allow paste of dev token; document that prod uses CLI or SSH tunnel if UI disabled.
5. Optional: hide `/admin/*` nav on public profile unless `VITE_ADMIN_ARCHIVE_UI_ENABLED=true` (default false on BearHost).

Files to inspect/change: `frontend/src/App.tsx`, `frontend/src/pages/admin/*`, `frontend/src/hooks/useArchiveJobs.ts`, `frontend/src/components/admin/AdminTokenGate.tsx` (new)
Database changes: none
Env/config changes: `VITE_ADMIN_ARCHIVE_UI_ENABLED` (optional); **no** `ADMIN_ARCHIVE_TOKEN` in frontend service env on prod
Acceptance criteria: Public `config.js` contains no archive admin token; pasted token unlocks UI; missing token shows instructions (CLI + SSH tunnel)
Test commands: `npm run build`; manual browser on localhost
Rollback: Remove routes
Risks: Token in sessionStorage on shared browser — document “clear session”; prefer CLI on untrusted machines

## TASK-025: Admin UI panels (jobs, coverage, controls)

Priority: P2 · **Release 3**
Type: frontend
Depends on: TASK-022, TASK-024, TASK-021B
Goal: Progress bars, failure table, retry buttons, coverage cards.
Context: §9 UI spec.
Implementation steps: Build three pages; wire to admin API; loading/error/empty states.
Files to inspect/change: `ArchiveAdminPage.tsx`, `AdminJobsPage.tsx`, `AdminCoveragePage.tsx`
Database changes: none
Env/config changes: none
Acceptance criteria: Active job shows heartbeat age; retry calls API
Test commands: Playwright smoke (optional TASK-032)
Rollback: Hide nav links
Risks: API drift — share TypeScript types

## TASK-026: Archive Prometheus metrics + refresh loop

Priority: P1
Type: backend
Depends on: TASK-016, TASK-018
Goal: `streamclone_archive_*` metrics per observability spec.
Context: Partial v1 metrics exist.
Implementation steps: Add `internal/metrics/archive_jobs.go`; refresh from Postgres every 30s in analytics-workers.
Files to inspect/change: `cmd/analytics/main.go`, `metrics/archive_jobs.go`
Database changes: none
Env/config changes: `ARCHIVE_METRICS_REFRESH_INTERVAL`
Acceptance criteria: `/metrics` exposes job gauges after job run
Test commands: scrape analytics internally
Rollback: Stop refresh goroutine
Risks: Cardinality — no per-job_id labels in prod

## TASK-027: docker-compose.observability.yml profile

Priority: P1
Type: infra
Depends on: TASK-026
Goal: Optional Prometheus/Grafana without full pulse/Influx.
Context: OBS-C6; BearHost low RAM.
Implementation steps: New overlay; scrape analytics-workers; localhost bind; no Caddy /metrics.
Files to inspect/change: `deploy/docker-compose.observability.yml`, `deploy/prometheus/prometheus.yml`, `docs/bearhost-production.md`
Database changes: none
Env/config changes: document `--profile observability`
Acceptance criteria: `docker compose --profile observability config` valid
Test commands: `make compose-config-check`
Rollback: Don't enable profile
Risks: +512MB RAM

## TASK-028: Grafana streamclone-archive dashboard + alerts

Priority: P1
Type: infra
Depends on: TASK-027
Goal: Archive panels + alert rules.
Context: Part II dashboard list.
Implementation steps: Add `streamclone-archive.json`, provisioning yml, `prometheus/alerts/archive.yml`.
Files to inspect/change: `deploy/grafana/dashboards/`, `deploy/prometheus/alerts/`
Database changes: none
Env/config changes: none
Acceptance criteria: Panels show job depth, upload failures, coverage ratio
Test commands: SSH tunnel Grafana; view dashboard
Rollback: Remove provisioning
Risks: Stale panel queries if metric names change

## TASK-029: Second proxy benchmark + sign-off doc

Priority: P1
Type: ops
Depends on: TASK-001
Goal: Close budget `tt_list` gap before any proxy routing.
Context: proxy-benchmark.md 2026-06-20.
Implementation steps: Re-run budget profile; update doc table; operator sign-off checkbox.
Files to inspect/change: `docs/scraping-archive/proxy-benchmark.md`, `docs/benchmarks/*.json` (gitignored)
Database changes: none
Env/config changes: `.env.local` PROXY_FLAME_* only
Acceptance criteria: Budget tt_list pass or documented waiver
Test commands: `make scraper-proxy-benchmark`
Rollback: Keep direct egress
Risks: **Requires operator approval** to proceed to TASK-030

## TASK-030: Implement ANALYTICS_TT_USE_PROXY (gated)

Priority: P2
Type: backend
Depends on: TASK-029 + operator approval
Goal: Wire sync.go useProxy from env.
Context: P5 requirements; currently hardcoded false.
Implementation steps: Add config bool; pass to scraper request; metrics for proxy path.
Files to inspect/change: `internal/config/config.go`, `internal/analytics/sync.go`
Database changes: none
Env/config changes: `ANALYTICS_TT_USE_PROXY=false` default
Acceptance criteria: false = identical behavior; true = scraper uses PROXY_* on BearHost
Test commands: benchmark + one silver stream
Rollback: Env false
Risks: Global enable forbidden — env per-host only

## TASK-031: scripts/bearhost-corpus-smoke.sh

Priority: P0 · **Release 1**
Type: test
Depends on: TASK-003, TASK-007, TASK-016B, TASK-018, TASK-019
Goal: Automate Release 1 acceptance gate on VPS/staging.
Context: corpus-requirements acceptance list; run at end of Release 1 before Release 2.
Implementation steps: Bronze 5ch, job rows + FK reconciliation, `jobs list`, coverage report, no mp4 in prefix.
Files to inspect/change: `scripts/bearhost-corpus-smoke.sh`, `Makefile` target
Database changes: none
Env/config changes: none
Acceptance criteria: Exit 0 on green staging
Test commands: `bash scripts/bearhost-corpus-smoke.sh`
Rollback: n/a
Risks: Needs Azure creds on host

## TASK-031B: Restore drill smoke

Priority: P1 · **Release 1** (gate before Release 2)
Type: ops / test
Depends on: TASK-006, TASK-007
Goal: Prove archive is restorable — not backup theater.
Context: `cmd/archive restore` exists today for stream rollups; extend smoke to verify re-index from Azure without scraper.
Implementation steps:

1. Export or use existing archived stream artifacts in Azure.
2. Fresh/dev Postgres (or truncated stream rows).
3. Run `archive restore --stream-id=<id>` (or documented equivalent).
4. Assert rollups/catalog reappear; manifest rows still valid.
5. Add to `bearhost-corpus-smoke.sh` or separate `scripts/archive-restore-drill.sh`.

Files to inspect/change: `scripts/archive-restore-drill.sh` (new), `cmd/archive/main.go`, `internal/archive/restore.go`, `scripts/bearhost-corpus-smoke.sh`
Database changes: none
Env/config changes: none
Acceptance criteria: Restore one stream without TT/GQL scrape; documented in runbook
Test commands: `bash scripts/archive-restore-drill.sh`
Rollback: n/a
Risks: Restore scope may lag bronze artifacts — test what restore actually implements; expand with bronze in Release 2

## TASK-032: PulseWire cold export registry entry (P2)

Priority: P2
Type: backend
Depends on: TASK-006
Goal: Export social/story tables before retention purge.
Context: PW1, EX1 registry.
Implementation steps: Add exporter module under `internal/storygraph/archive/`; register paths `pulsewire/raw/…`.
Files to inspect/change: storygraph store, `internal/archive/exporter.go` registry
Database changes: none
Env/config changes: `PULSEWIRE_ARCHIVE_ENABLED`
Acceptance criteria: Reddit batch exports with manifest rows
Test commands: storygraph ingest + export tick
Rollback: Env false
Risks: Volume — start with one source

---

# 6. Suggested task sequencing for parallel agents

| Track | Tasks | Blocked by | Merge conflicts likely | Branch |
|-------|-------|------------|------------------------|--------|
| **A** manifest/writer | 004, 004B, 005–006 | 001; **004B before 005** | `writer.go`, `manifest.go`, migrations | `feat/archive-manifest` |
| **B** Bronze/emote R1 | 007–008, 010 | 006 | `bronze_indexer.go`, `emote_exporter.go` | `feat/archive-bronze-emote` |
| **B2** emote R3 | 011A–D | 010, 011B before C/D | `emote_exporter.go` | `feat/archive-emote-providers` |
| **C** Silver/Gold R3 | 012–015 | 006 | `exporter.go`, `sync.go`, `backfill_worker.go` | `feat/archive-silver-gold` |
| **D** job progress R1 | 016, 016B, 017–019 | **016B before 018** | `cmd/backfill`, jobtracker | `feat/archive-jobs-cli` |
| **D2** admin API R2 | 033, 021–023, 020 | 033 before 022 | Caddyfile, admin handlers | `feat/archive-admin-api` |
| **E** frontend admin R3 | 024–025, 021B | 022, 033, 021 | `App.tsx`, new pages | `feat/admin-archive-ui` |
| **F** observability R3 | 026–028 | 016, 018 | compose, grafana JSON | `feat/archive-observability` |
| **G** BearHost ops R1 | 002–003, 031, 031B | 002 before 003 | compose, smoke scripts | `feat/bearhost-corpus-ops` |
| **H** docs/tests | 001, 004B, 032 | varies | docs only | `docs/archive-task-plan` |

**Release 1 parallel tracks:** A + B + D + G after TASK-006 (004B gates A writers). **Do not start D2/E until Release 1 smoke green.**

**Release 2:** D2 + TASK-009, 012–013, 020.

**Release 3:** E + F + C (gold) + B2 + proxy + TASK-032.

---

# 7. Database migration plan

| Migration | Purpose | Key columns/indexes | Backfill | Rollback |
|-----------|---------|---------------------|----------|----------|
| **000035_archive_manifest_expand** | Manifest v2 columns | See TASK-004; indexes `(tier, channel_login, exported_at DESC)`, `(stream_id, tier)`, `(provider, artifact_type)` | Re-export populates sha256/tier; old rows stay valid | Drop added columns |
| **000036_archive_jobs** | Job progress SoT | `archive_jobs`, `archive_job_items` UNIQUE(job_id, item_key), optional `archive_job_events`; indexes per spec | Empty at create | Drop tables |
| **000036b_backfill_job_link** | Queue reconciliation (TASK-016B) | `backfill_jobs.archive_job_id UUID NULL FK`; `archive_job_items.backfill_job_id BIGINT NULL FK`; partial indexes | Empty; populated by workers | Drop columns |
| **000037_bronze_state_expand** | Hot cache for bronze/tombstones | Extend `bronze_index_state`; optional `vod_catalog_state(login, vod_id, …)`, `channel_identity_state(login, …)` | Empty; filled by workers | Drop new tables/columns |
| **000038_archive_coverage_snapshots** | Daily coverage JSON | `(snapshot_date PRIMARY KEY, report JSONB, created_at)` | Cron writes daily | Drop table |

**Compatibility:** Keep `archive_exports` PK `(artifact_type, natural_key)`; natural key semantics in TASK-004B; keep `gcs_uri` column name; map `export_status` `confirmed`↔`complete` in app during transition.

---

# 8. API and CLI plan

### CLI (extend `cmd/backfill`)

| Command | Args | Output |
|---------|------|--------|
| `jobs list` | `--status`, `--tier`, `--limit` | JSON array + summary table |
| `jobs show` | `--job-id=UUID` | Job + recent events + last errors |
| `jobs retry-failed` | `--job-id=UUID` | Updated job status |
| `jobs resume` | `--job-id=UUID` | Resumes stale/paused |
| `jobs cancel` | `--job-id=UUID` | Cooperative cancel |
| `coverage report` | `--tier`, `--since`, `--out` | **exists**; add `--tier` |
| `coverage verify-blobs` | `--sample=N` | Orphan report JSON |
| `coverage stale` | `--older-than=7d` | Stale channel list |
| `bronze run-once` | `--top-n=200` | Creates archive_job |
| `emotes snapshot` | `--top-n=200` | Job + counts |
| `silver enqueue` | `--top-n`, `--days=90` | Enqueues backfill_jobs batch |
| `gold-lite enqueue` | `--top-n`, `--days=90` | Gold-lite jobs |
| `gold-full enqueue` | `--min-peak-viewers=5000` | Selective gold-full |

### Admin API

**Auth:** Header **`X-Admin-Archive-Token: <ADMIN_ARCHIVE_TOKEN>`** — dedicated archive operator secret.

| Context | Token source | Header |
|---------|--------------|--------|
| BearHost / public VM | Server env or secrets file only; operator pastes into admin UI session or uses CLI | `X-Admin-Archive-Token` |
| Localhost dev | `.env` for curl/tests; optional UI paste | same |
| PulseWire / setup-control | Separate — **`X-Streamclone-Setup-Token`** + `SETUP_CONTROL_TOKEN` in `config.js` | **not** used for archive admin |

401 if missing when `ADMIN_ARCHIVE_REQUIRE_TOKEN=true`. See [security.md](../security.md) — do not put `ADMIN_ARCHIVE_TOKEN` in `config.js`.

**Caddy:** `/v1/admin/archive/*` and `/v1/admin/system/*` must reverse_proxy to **analytics** (TASK-033) before handlers ship.

**Endpoints:**

- `GET /v1/admin/archive/jobs?status=&tier=&limit=50`
- `GET /v1/admin/archive/jobs/{id}`
- `GET /v1/admin/archive/jobs/{id}/items?status=&limit=&offset=`
- `POST /v1/admin/archive/jobs/{id}/retry-failed`
- `POST /v1/admin/archive/jobs/{id}/resume`
- `POST /v1/admin/archive/jobs/{id}/cancel`
- `POST /v1/admin/archive/jobs/enqueue` body: `{ "jobType", "topN", "days" }`
- `GET /v1/admin/archive/coverage`
- `GET /v1/admin/archive/coverage/stale?olderThan=7d`
- `GET /v1/admin/archive/coverage/missing-blobs`
- `GET /v1/admin/system/queues`
- `GET /v1/admin/system/workers`

**Example job JSON:**

```json
{
  "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "jobType": "bronze_roster",
  "tier": "bronze",
  "status": "running",
  "totalItems": 200,
  "completedItems": 42,
  "failedItems": 1,
  "skippedItems": 0,
  "progressRatio": 0.21,
  "currentChannel": "xqc",
  "heartbeatAt": "2026-06-20T18:30:00Z",
  "elapsedSeconds": 754,
  "etaSeconds": 2800
}
```

**Errors:** 400 invalid id; 404 unknown job; 409 cancel on terminal job; 429 rate limit on POST.

---

# 9. Frontend/admin UI plan (Release 3 — optional)

**Release 1–2 operators use CLI** (`jobs list|show|retry-failed`, `coverage report`). UI is convenience only after API stabilizes.

### `/admin/archive` (overview)

- **Components:** `ArchiveOverview`, `QueueDepthCards`, `LatestArtifactsTable`, `QuickControls`
- **Data:** `GET /v1/admin/system/queues`, `GET /v1/admin/archive/jobs?status=running`, latest `archive_exports` via coverage endpoint
- **Empty:** No jobs → "Start Bronze run-once from controls"
- **Loading:** Skeleton cards
- **Error:** Token missing → `AdminTokenGate` with paste field + link to CLI (`cmd/backfill jobs list`) and SSH tunnel option
- **Security:** Token from **sessionStorage only** (operator paste); never from `window.__STREAMCLONE_CONFIG__` on public hosts

### `/admin/jobs`

- **Components:** `JobList`, `JobDetailDrawer`, `ProgressBar`, `FailureTable`, `RetryButton`
- **Data:** jobs list + show + items
- **Empty:** No jobs in 7d
- **Acceptance:** Progress bar = completed/total; heartbeat age shown

### `/admin/coverage`

- **Components:** `CoverageByTierChart`, `StaleChannelsList`, `MissingBlobsList`
- **Data:** coverage + stale + missing-blobs endpoints
- **Acceptance:** Bronze/7TV/Silver/Gold-lite percentages match CLI report

---

# 10. Observability plan

**Metrics (add to v1):** `streamclone_archive_jobs_running`, `_queued`, `_completed_total`, `_failed_total`, `streamclone_archive_job_duration_seconds`, `streamclone_archive_items_*_total`, `streamclone_queue_depth`, `streamclone_worker_heartbeat_age_seconds`, `streamclone_archive_uploads_total`, `_upload_failures_total`, `_upload_bytes_total`, `streamclone_archive_coverage_ratio`, scraper counters (if emitted from scraper or proxy), PulseWire ingest counters (existing storygraph metrics if any — verify in code).

**Compose profile `observability`:** prometheus + grafana (+ optional node-exporter); **not** merged in BearHost default; bind `127.0.0.1:3000`, `127.0.0.1:9090`.

**Access:** SSH tunnel `ssh -L 3000:127.0.0.1:3000 streamclone@legacy-rollback-host`; **no** Caddy route to `/metrics`.

**Dashboard panels:** job status, throughput, coverage by tier, queue depth, scraper health, Azure uploads, worker heartbeat, host CPU/RAM (if node-exporter).

**Alerts:** ArchiveJobFailed, ArchiveJobStale, QueueDepthHigh, ScraperFailureRateHigh, AzureUploadFailures, DiskUsageHigh, MemoryUsageHigh.

---

# 11. Proxy and egress plan

1. Preserve gitignored JSON under `docs/benchmarks/` (already in `.gitignore`).
2. **TASK-029:** Re-run budget `tt_list`; document pass/fail.
3. **Sign-off criteria for any proxy enablement:** Premium TT detail ≥95% pass; budget tt_list pass OR explicit waiver; Cloudflare flag false on completed probes; operator written approval in `proxy-benchmark.md`.
4. **Routing policy:** Default **direct** on BearHost; premium residential for Silver bulk only when TASK-030 enabled on that host; never global env in shared `.env` committed.
5. **Silver defaults:** `SCRAPER_MAX_CONCURRENT=1`, `BACKFILL` one stream at a time on 8 GB VPS.
6. **Fallback:** Document home-PC egress (`ARCHIVE_EGRESS_SLOT=home` — planned env) for TT if VPS fails smoke gate 3.
7. **Metrics:** scraper failure rate, CF detection counter, TT scrape duration p95 — wire in TASK-026/030.

**Do not enable proxy globally without TASK-029 sign-off + TASK-030 + operator approval.**

---

# 12. BearHost rollout plan (operator tasks)

| # | Task | Command / check |
|---|------|-----------------|
| 1 | Env validation | `make bearhost-config-check-local` |
| 2 | Azure secret | `test -f /etc/streamclone/secrets/azure-archive-connection-string` |
| 3 | Compose merge | Phased script with profile-archive + bearhost-prod + build |
| 3b | Caddy admin routes | TASK-033: `/v1/admin/*` → analytics (smoke before admin API) |
| 4 | Scraper internal | `ss -tlnp` — no public :8000 |
| 5 | Postgres/Redis | UFW + compose `ports: !reset` |
| 6 | Corpus preflight | Azure secret + Twitch creds → `CORPUS_WORKERS_ENABLED=1` |
| 7 | Archive smoke | `bash scripts/bearhost-corpus-smoke.sh` |
| 8 | Admin token | Install `ADMIN_ARCHIVE_TOKEN` server-side only; operator paste or CLI — **not** in frontend `config.js` |
| 9 | Observability | Optional `--profile observability` |
| 10 | Cron | coverage report, verify-blobs, pg backup (see archive-observability.md) |
| 11 | Backup verify | Restore drill to test DB quarterly |
| 12 | Rollback | Local dev for 48h; `CORPUS_WORKERS_ENABLED=false`; keep Azure corpus |

---

# 13. Acceptance test matrix

| Name | Scope | Command | Expected | Priority |
|------|-------|---------|----------|----------|
| archive unit | Go | `go test ./internal/archive/...` | pass | P0 |
| analytics unit | Go | `go test ./internal/analytics/... -run Coverage` | pass | P0 |
| manifest migration | DB | apply 000035 on dev | success | P0 |
| bronze 5ch | integration | corpus smoke / bronze run-once | blobs + manifests | P0 |
| job progress | integration | bronze job + `jobs show` | counters update | P0 |
| stale/resume | integration | kill worker, resume | skip succeeded | P0 |
| retention guard | Go | existing store tests + ARCHIVE_PROTECT_RETENTION | purge blocked | P0 |
| job/backfill reconciliation | integration | enqueue silver under parent job | `backfill_jobs.archive_job_id` set; `jobs show` matches queue | P0 |
| restore drill | ops | `scripts/archive-restore-drill.sh` | stream rehydrated without scrape | P1 |
| CLI jobs | CLI | `backfill jobs list` | JSON | P0 |
| admin API auth | API | curl without token | 401 | R2 |
| admin API routing | infra | curl via Caddy | 401 not 404 | R2 |
| admin API happy | API | curl with token | 200 + job list | R2 |
| admin UI disabled HTTP | frontend | BearHost `/admin/archive` | CLI instructions, no token field (TASK-021B) | R3 |
| admin token not in config | frontend | inspect `/config.js` on BearHost | no archive admin token | P0 |
| frontend build | frontend | `npm run build` | pass | R3 |
| compose config | infra | `make bearhost-config-check` | valid | P0 |
| bearhost smoke | ops | `bearhost-smoke.sh` | gates green | P0 |
| azure upload | ops | bronze with secret | confirmed manifest | P0 |
| verify-blobs | integration | `coverage verify-blobs` | report | R2 |
| no vod video | ops | list blob prefix | no .mp4/.m3u8 | P0 |
| proxy benchmark | ops | scraper-proxy-benchmark | JSON artifact | P1 |
| metrics internal | ops | curl from docker network only | 200 | P1 |
| grafana tunnel | ops | SSH tunnel | dashboard loads | P2 |

---

# 14. Risks and mitigations

| Risk | Mitigation |
|------|------------|
| BearHost RAM pressure | observability optional; concurrency=1; no pulse on VPS |
| Camoufox/TT blocking | proxy gate; partial manifests; home egress fallback |
| Azure upload failures | retry per item; manifest `failed`; alerts |
| Manifest/blob drift | verify-blobs cron; sha256 |
| DB migration risk | nullable columns; forward-only; test on copy |
| Admin route exposure | TASK-033 Caddy → analytics; token middleware; no public metrics |
| Admin token in public JS | `ADMIN_ARCHIVE_TOKEN` server-only; UI sessionStorage paste; CLI fallback |
| Caddy misroutes admin to metadata | TASK-033 before TASK-022; smoke 401-not-404 |
| Grafana exposure | localhost bind only |
| Stale jobs | tunable stale interval; heartbeat on long TT |
| Duplicate exports | TASK-004B natural keys + content hash |
| Job/queue drift | TASK-016B FK bridge; CLI shows linked `backfill_job_id` |
| Gold-lite GQL surprise | `GOLD_LITE_REQUIRE_ROLLUPS`; no top-200 bulk without operator approval |
| Gold-full cost | OPERATOR_ONLY; high thresholds; off by default |
| Archive not restorable | TASK-031B restore drill in Release 1 gate |
| Retention deleting unarchived | ARCHIVE_PROTECT_RETENTION + manifest guard (shipped) |
| Proxy instability | benchmark sign-off; default off |
| Full VOD video creep | acceptance test lists prefix; code review gate |

---

# 15. Final recommended execution order

### Release 1 (implement → stop → test)

1. TASK-001 inventory
2. TASK-002/003 BearHost corpus-plane + preflight
3. TASK-004 → **TASK-004B** → TASK-005 → TASK-006 manifest foundation
4. TASK-016 → **TASK-016B** → TASK-017 → TASK-018 job progress (with backfill link)
5. Parallel: TASK-007, TASK-008, TASK-010 (bronze + global 7TV)
6. TASK-019 CLI jobs + coverage report
7. TASK-031 corpus smoke + TASK-031B restore drill
8. **Stop.** Do not start admin API, Caddy admin routes, silver chart, or gold split until Release 1 green.

### Release 2

1. TASK-033 Caddy + TASK-021 auth + TASK-022/023 admin API
2. TASK-020 verify-blobs, TASK-009 tombstones/roster, TASK-012/013 silver provenance
3. BearHost: CLI/tunnel for admin mutations; raw HTTP API still sniffable — document ops practice.

### Release 3

1. TASK-024/025/021B admin UI (localhost/TLS only by default)
2. TASK-026–028 observability
3. TASK-014/015 gold-lite/full (rollup gate enforced)
4. TASK-011A–D emote diff + FFZ/BTTV
5. TASK-029/030 proxy (operator approval)
6. TASK-032 PulseWire export (P2)

### Never without operator approval

Azure secret + `CORPUS_WORKERS_ENABLED=1`, `ADMIN_ARCHIVE_TOKEN` rotation, gold-lite/gold-full top-200 bulk, TASK-030 proxy enable, observability on 8 GB VPS, Silver bulk on datacenter IP without proxy.

---

*End of implementation task plan.*
