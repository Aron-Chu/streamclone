# Scraping & Cold Storage — Requirements

Status: **proposal / in progress**. Companion: [../scraper-cloudflare-and-proxy.md](../scraper-cloudflare-and-proxy.md).

Related code (today): `internal/analytics/sync.go`, `internal/metadata/api/api.go`, `internal/storygraph/ingest/`, `scripts/backup-streamclone.ps1`, `deploy/docker-compose.yml` (scraper profile).

Planned code: `cmd/archive` (export worker), `cmd/backfill` (bulk queue), Azure Blob export hooks.

---

## TL;DR

Streamclone should treat **Azure Blob Storage** as the **authoritative durable archive today** (Phase A infra: [azure-archive-setup.md](../azure-archive-setup.md)) and **Postgres** as a disposable hot cache (30–90 days). **Cloudflare R2** is planned for long-term artifact bytes after staged mirror verification — see [storage/azure-to-r2-migration.md](../storage/azure-to-r2-migration.md), [storage/tasks.md](../storage/tasks.md), and [storage/README.md](../storage/README.md).

**R2 guardrails (corpus / archive):**

- Azure remains authoritative for production reads/writes until read-through is verified behind flags.
- Do **not** bulk-copy `postgres/nightly/` to R2 until **STOR-R2-004** restore drill passes.
- Do **not** bulk-copy `vod_chat/` until sample mirror + read-through are proven on staging.
- Postgres `archive_exports` remains the manifest source of truth; R2 holds bytes only.

Historical Analytics depends on expensive sources — especially **TwitchTracker per-stream detail** (Camoufox) and **GQL VOD chat** — that must not be re-fetched after a Postgres reset.

Bulk collection targets **top 200–500 live streamers** in three scrape depths (Bronze / Silver / Gold). Browser scraping runs from a **residential IP** (home PC or verified residential proxy). API-only work (Helix, 7TV, directory sampling) can run anywhere.

**Immediate priorities**

1. Document and tier all scrape + storage artifacts (this file).
2. Benchmark residential proxies vs direct egress (`make scraper-proxy-benchmark`).
3. Benchmark Pydoll Turnstile handler vs passive Camoufox (`make scraper-turnstile-benchmark`).
4. Automate Postgres + rollup export to Azure Blob before retention purges.
5. Build bulk backfill queue (Helix index → Tracker detail → selective GQL chat).

### Viewer chart fidelity (implementation status)

Cross-ref: [scraping-phase Tier 0 / Tier 2](../scraping-phase.md) (Helix live samples + TT detail backfill).

| Item | Status |
|------|--------|
| TT HTML parser fixtures + table-driven tests | **Done** — `internal/analytics/testdata/twitchtracker/`, `go test -run TwitchTracker` |
| Live vs TT benchmark harness | **Done** — `scripts/benchmark-tt-vs-live.ps1` → JSON (MAE, peak delta, coverage) |
| Live-first minute merge (TT fills gaps only) | **Done** — `BulkPatchViewerRollups`, `viewerSource` on stream detail |
| TT-only median smooth before rollup | **Done** — `ANALYTICS_TT_VIEWER_SMOOTH_WINDOW` (default 0) |
| SVG / injected chart density rejection | **Done** — rejects sparse paths (`point_count < duration_min/5`) |
| Tier 0 directory top-N Helix roster | **Done** — `TIER0_ENABLED`, roster + viewer sampler in `cmd/analytics` |

Capture fresh TT HTML locally: `scripts/capture-tt-fixture.ps1 -Login … -StreamId …` (output under gitignored `docs/benchmarks/`).

---

## Goals & non-goals

### Goals

- Survive frequent Postgres resets without re-scraping TwitchTracker or re-paginating VOD chat.
- Backfill **top 200–500** channels with viewer minute charts (Silver tier minimum).
- Export every completed sync artifact to GCS incrementally and via nightly full dumps.
- Support residential proxy experiments (Flame Proxies premium + budget tiers) with reproducible benchmarks.
- Keep Core Watch and Analytics honest when scraper or archive tiers are unavailable.

### Non-goals

- Replacing Postgres as the live query engine for the UI (GCS is cold storage; optional lazy-load is P2).
- Scraping full VOD chat for every stream in the top 500 (Gold tier is selective).
- Running Camoufox/TwitchTracker bulk jobs from datacenter VPS IPs without residential proxy.
- Storing rendered emote WebP at all scales in GCS (metadata snapshots + CDN URLs are enough).
- Committing proxy credentials, GCS keys, or OAuth tokens to the repo.

---

## Problem statement

Local Postgres is reset often (`make nuke`, migrations, experiments). Retention workers purge data on schedule:

| Setting | Default | Purges |
|---------|---------|--------|
| `ANALYTICS_RETENTION_DAYS` | 30 | Old streams + rollups |
| `ANALYTICS_VOD_CHAT_RETENTION_DAYS` | 90 | VOD chat messages |
| `CHAT_LOG_RETENTION_DAYS` | 14 | Live chat archive |
| `PULSE_DIRECTORY_RETENTION_DAYS` | 30 | Directory samples |
| `SOCIAL_RETENTION_DAYS` | 90 | Pulse Wire social items |

Re-syncing after loss repeats the most expensive operations: TwitchTracker Camoufox per-stream pages and GQL chat pagination.

---

## Canonical analytics artifact

Everything on the Analytics page converges on:

| Table | Role |
|-------|------|
| `analytics_streams` | Stream index (login, title, category, dates, vod_id, peak/avg) |
| `analytics_minute_rollups` | **Per-minute** viewer_avg/max, chat_count, emote counts + JSON map |
| `stream_game_segments` | Category switches from TwitchTracker detail |
| `analytics_vod_chat_messages` | Optional full message replay (storage-heavy) |
| `analytics_sync_checkpoints` | GQL resume state |

**Requirement A1 (P0):** Any data worth keeping beyond retention MUST be exported to GCS before local purge.

**Requirement A2 (P0):** Export MUST be idempotent (same stream_id + minute_ts does not duplicate objects).

**Requirement A3 (P1):** Restore from GCS MUST be able to repopulate `analytics_streams` + `analytics_minute_rollups` without network calls.

**Requirement A4 (P0):** Retention purge MUST consult an **`archive_exports` manifest** (not a config flag alone) before deleting rows. Each purge path MUST verify a confirmed export row exists for the artifact natural key about to be deleted.

**Requirement A4a (P0):** `ARCHIVE_ENABLED=false` alone MUST **not** block retention on default desktop installs. Destructive purge blocking applies only when `ARCHIVE_PROTECT_RETENTION=true` (backfill / production archive profile) **or** when the manifest guard cannot confirm export for rows in scope.

### Archive export manifest (`archive_exports`)

Phase 1’s **first deliverable** — gives A4, B4, and all purge workers a shared floor.

| Column (proposed) | Purpose |
|-------------------|---------|
| `artifact_type` | e.g. `rollup_minute`, `vod_chat`, `social_item`, `directory_sample`, `chat_mod_event`, `pg_dump` |
| `natural_key` | e.g. `stream_id:minute_ts`, `source:external_id`, `sample_run_id`, nightly dump date |
| `gcs_uri` | Azure blob HTTPS URI (`https://{account}.blob.core.windows.net/{container}/streamclone/…`; column name is legacy) |
| `object_generation` / `etag` | Confirms object still exists before purge |
| `exported_at` | Wall time of successful upload |
| `row_count` / `byte_size` | Optional audit |
| `export_status` | `confirmed` \| `failed` \| `pending` |

**Purge call sites today (must consult manifest when `ARCHIVE_PROTECT_RETENTION=true`):**

| Code | What it purges |
|------|----------------|
| `internal/analytics/store.go` | Stream + rollup retention (`ANALYTICS_RETENTION_DAYS`) |
| `internal/analytics/chatreplay/logs.go` | Live chat + mod events (`CHAT_LOG_RETENTION_DAYS`) |
| `internal/analytics/chatreplay/retention.go` | VOD chat messages (`ANALYTICS_VOD_CHAT_RETENTION_DAYS`) |
| `internal/storygraph/store/store.go` | Social items (`SOCIAL_RETENTION_DAYS`) |
| `internal/storygraph/store/stats.go` | Directory samples + follower snapshots (`PULSE_DIRECTORY_RETENTION_DAYS`) |

**Requirement MAN1 (P0):** Export worker SHALL upsert `archive_exports` on every successful artifact upload.

**Requirement MAN2 (P0):** All retention deletes SHALL go through a shared guard that queries `archive_exports` (or nightly `pg_dump` manifest row for whole-DB safety net).

---

## Data tier list (what to scrape & store)

### S-Tier — Analytics core (must archive)

| Data | Source | Stored in | Scrape cost | GCS path (proposed) |
|------|--------|-----------|-------------|---------------------|
| Minute rollups | Derived | Postgres | — | `rollups/stream_id={id}/part-*.jsonl.gz` (Parquet P2+) |
| Stream index | Helix + Tracker | `analytics_streams` | Low–Med | `streams/channels/{login}.jsonl.gz` |
| TwitchTracker **per-stream detail** | Camoufox `/{login}/streams/{streamId}` | Rollups (viewers) + game segments | **High** | `tracker_html/{login}/{streamId}.html.gz` (optional) + rollups |
| GQL VOD chat | Twitch GQL | Rollups (chat/emotes) + optional messages | **Very high** | `vod_chat/stream_id={id}.jsonl.gz` |
| Game segments | Parsed from Tracker detail | `stream_game_segments` | Bundled | `game_segments/stream_id={id}.jsonl` |

Without S-tier, Analytics charts are empty or viewer-only.

### A-Tier — High value, cheap index (bulk backfill phase 1)

| Data | Source | Scrape cost | GCS path |
|------|--------|-------------|----------|
| TwitchTracker channel summary | HTTP `twitchtracker.com/api/channels/summary/{login}` | Very low | `channels/summary/{login}.json` |
| Helix VOD archive list | Twitch Helix | Low | `channels/vod_index/{login}.jsonl.gz` |
| TwitchTracker stream list table | Camoufox or HTTP `/{login}/streams` | Medium | `channels/stream_list/{login}.jsonl.gz` |
| 7TV / FFZ / BTTV emote sets | Provider APIs | Low–Med | `emotes/snapshots/{login}/{date}.json` |
| Directory samples | Metadata `/v1/streams` top-N | Low | `directory/date={date}/part-*.jsonl.gz` |
| Follower snapshots | TwitchTracker summary / Helix (planned) | Very low | `followers/login={login}/part-*.jsonl.gz` |

`streamer_follower_snapshots` (migration `000019`) stores periodic follower counts per channel — one `BIGINT` per sample, ideal for long-term growth curves. **Today:** `InsertFollowerSnapshot` exists but has **no active caller**; `StreamerStatProfile` reads the table when rows exist. Archive plan should export snapshots alongside directory samples once a sampler wires inserts (TT summary enrichment or Helix poll on the rising set).

### B-Tier — Analytics-adjacent (archive if Pulse Wire enabled)

| Data | Source | Notes |
|------|--------|-------|
| Live IRC + Helix poll | Real-time while watching | Forward-only; max 50 concurrent tracked channels |
| Helix clips metadata | Twitch API | Channel Insights + moment correlation |
| Reddit r/LivestreamFail | JSON → scraper fallback | IP-sensitive; Insights panel |
| YouTube search metadata | API or scraper | Pulse Wire; residential proxy from servers |
| Live mod events | IRC while watching | `chat_mod_events` — bans, timeouts, clears; **not** in GQL VOD chat or rollups |

`chat_mod_events` is tiny and **irreplaceable** after the moment passes. Export before `CHAT_LOG_RETENTION_DAYS` purge (same guard as rollups). Prefer **B-tier** unless mod-audit replay is a core Analytics feature (then promote to A-tier).

### C-Tier — Skip or compress

| Data | Reason |
|------|--------|
| Raw `live_chat_messages` | Redundant with GQL VOD chat + minute rollups; fragments differ slightly from GQL but rollups capture emote usage |
| Scraper HTML cache (ephemeral) | Re-fetchable |
| All emote WebP scales | CDN + re-render; skip MinIO→GCS mirror — CDN redownload beats slow upload |
| TwitchTracker channel pages when summary API suffices | API covers 90% |

### User state — not scrape data (preserve via pg_dump)

These tables hold **local personalization**, not third-party scrape artifacts. No separate GCS export is required unless selective restore without full DB reload is needed; nightly `postgres/nightly/` pg_dump is sufficient.

| Table | Role |
|-------|------|
| `local_follows` | Followed channel list (Core Watch sidebar) |
| `story_follows` | Pulse Wire saved / tracked stories |
| `story_watch_entries` | Saved filters and watchlist entries |
| `story_operator_actions` | Operator audit log (marks, notes) |

---

## Bulk backfill tiers (top 200–500 streamers)

| Tier | Channels | Streams window | Scrape depth | Est. duration |
|------|----------|----------------|--------------|---------------|
| **Bronze** | 500 | 90 days | Summary API + Helix VOD index + directory samples | ~1 day |
| **Silver** | 500 | 60 days | Bronze + **Tracker per-stream detail** (viewers-only sync) | ~1–2 weeks @ home IP |
| **Gold** | 50 favorites | 90 days | Silver + **GQL VOD chat** + emote preload | ~1 week incremental |

**Requirement B1 (P0):** Channel list SHALL be persisted to Azure Blob (`channels/top500.json.gz`; today `channels/top200.json.gz` via `ExportTopRoster`) and reproducible from `PULSE_DIRECTORY_TOP_N` + `ALWAYS_TRACKED_CHANNELS`.

**Requirement B2 (P0):** Bulk Tracker scrape SHALL use `viewersOnly=true` (or equivalent) before chat sync.

**Requirement B3 (P1):** Gold-tier GQL chat SHALL run only for streams matching configurable rules (min duration, min peak viewers, or explicit login list).

**Requirement B4 (P0):** Stream sync completion MUST NOT mark `SyncPhaseCompleted` until export is **confirmed** in `archive_exports` (when `ARCHIVE_EXPORT_ON_SYNC=true`). Today `runSyncJob` marks completed immediately after `SyncHistoricalStream` returns (`sync_status.go`) — add an archive hook between sync success and terminal status; export failure leaves job **export-pending** / retryable.

**Requirement B5 (P1):** `cmd/backfill` SHALL be a **durable queue** (Postgres `backfill_jobs` or Redis streams), not a thin wrapper around the sync HTTP endpoint. Job row fields: `tier`, `stream_id`, `login`, `egress_slot`, `attempt`, `export_status`, `next_run_at`, `error`. Worker calls the same sync path as UI but owns retries and export-on-completion.

---

## Cold storage architecture (Azure Blob)

```
https://{account}.blob.core.windows.net/{ARCHIVE_AZURE_CONTAINER}/{ARCHIVE_AZURE_PREFIX}/
  channels/
    top500.json
    summary/{login}.json
    vod_index/{login}.jsonl.gz
    stream_list/{login}.jsonl.gz
  rollups/stream_id={id}/part-000.jsonl.gz   # Phase 1; Parquet optional Phase 2+
  streams/channels/{login}.jsonl.gz
  game_segments/stream_id={id}.jsonl
  vod_chat/stream_id={id}.jsonl.gz
  emotes/snapshots/{login}/date={date}.json
  directory/date={date}/part-*.jsonl.gz      # Parquet optional later
  followers/login={login}/part-*.jsonl.gz
  chat_mod_events/date={date}/part-*.jsonl.gz
  emotes/changelog/date={date}.jsonl.gz   # EventAPI deltas (optional)
  social/{source}/date={date}/*.jsonl.gz  # per social.Register name; plug-in path
  pulsewire/…                             # optional selective exports (storygraph tables)
  manifest/archive_exports.jsonl.gz       # or Postgres table mirror export
  postgres/nightly/{date}.sql.gz          # full pg_dump (includes user state + unmigrated exports)
  minio/emotes/                           # optional; usually skip — see C-tier
```

### Archive writer (implementation)

**Shipped (Phase 1):** `internal/archive.Writer` + Azure Blob SDK (`internal/archive/writer.go`), JSONL.gz rollups, optional pg_dump gzip, manifest upsert via `ManifestStore`. Selective restore: `go run ./cmd/archive restore --stream-id <id>` — see [azure-archive-setup.md](../azure-archive-setup.md#restore-rollups).

**Requirement WR1 (P1):** ~~Define `internal/archive.Writer`~~ **Done** — Put artifact, manifest upsert, idempotent keys; JSONL + gzip and pg_dump gzip paths live.

**Requirement WR2 (P2):** Add Parquet+zstd encoder once manifest + selective restore contract is verified in CI/manual restore tests.

**Requirement S1 (P0):** Azure Blob SHALL be the system of record for data older than local retention.

**Requirement S2 (P0):** Nightly backup SHALL upload **gzip** `pg_dump` to `postgres/nightly/`, write manifest row, and verify restore on a test DB. **Shipped:** `scripts/backup-streamclone.ps1` writes gzip + Azure upload instructions; operator E2E restore smoke still pending.

**Requirement S2a (P1):** MinIO→Azure emote mirror remains optional (C-tier); do not block Phase 1 on MinIO upload.

**Requirement S3 (P1):** Incremental export worker SHALL run on a fixed interval (default `ARCHIVE_EXPORT_INTERVAL=1h`) and on sync completion events. Sync hook shipped; hourly ticker not wired yet (Phase 1 checklist #7).

**Requirement S4 (P2):** Analytics API MAY lazy-load rollups from Azure Blob when stream absent locally (import-on-demand).

**Requirement S5 (P0):** Azure connection string SHALL live in a local file only (`ARCHIVE_AZURE_CONNECTION_STRING_FILE` → e.g. `~/.streamclone/azure-archive-connection-string`); never committed.

---

## Extensibility — plugging in new data (Pulse Wire, etc.)

The plan is **additive**, not a closed checklist. You do not need to restructure phases or GCS layout when a new Pulse Wire feature lands — only document the artifact and wire export when it matters.

### What stays fixed (the contract)

| Rule | Meaning |
|------|---------|
| **A1** | If Postgres retention would delete it and you care → export to GCS first (or block purge via A4 manifest when `ARCHIVE_PROTECT_RETENTION=true`). |
| **Path convention** | `https://{account}.blob.core.windows.net/{container}/streamclone/{domain}/…` — new domains get a new prefix; do not nest under `rollups/` unless it is minute analytics. |
| **Idempotent keys** | Export objects keyed by natural id + time (`cluster_id`, `source` + `external_id`, `stream_id` + `minute_ts`, etc.). |
| **pg_dump safety net** | Nightly `postgres/nightly/` captures **every** table until a selective exporter exists. New migrations are archived automatically at DB level. |
| **Tier list** | S/A/B/C is guidance, not code. Add a row; promote/demote as cost or value becomes clear. |

### What is *not* fixed (safe to evolve)

- **Bulk backfill Bronze/Silver/Gold** — Analytics + TwitchTracker depth only. Pulse Wire ingest, story clusters, notifications, etc. do **not** need to fit these tiers.
- **Per-table GCS paths** — proposed paths in this doc are defaults. A new Pulse Wire table can use e.g. `pulsewire/{table}/date={date}/part-*.parquet` without touching Analytics paths.
- **Export timing** — `ARCHIVE_EXPORT_ON_SYNC` for stream sync; social ingest can use post-ingest hook or batch; immature features can rely on pg_dump until export is worth building.
- **Lazy-load / restore** — P2; cold data can sit in GCS until a read path exists.

### Plug-in checklist (new table, source, or feature)

1. **Assign tier** — S/A/B/C or “user state” (pg_dump only).
2. **Note retention** — which `*_RETENTION_DAYS` env purges it (or add a new one in code + document here).
3. **Pick GCS prefix** — e.g. `social/{source}/` for `social.Register` adapters; `pulsewire/…` for storygraph tables.
4. **Export** — register in archive worker table list **or** defer to nightly pg_dump while the feature is still moving.
5. **One line in this doc** — tier table row + path; keeps agents and future you aligned.

### Pulse Wire specifically

- **New social source** — implement `SocialSource` + `social.Register("yoursource", …)`; export target is already `social/{source}/date={date}/*.jsonl.gz`. Examples today: `reddit`, `youtube`, `twitchclips`, `streamerbans`; stubs: `xrecent`, `kick`.
- **New storygraph tables** (clusters, receipts, scores, watch state) — covered by pg_dump immediately; add selective export when row volume or restore granularity warrants it.
- **Ingest-only / experimental** — `ARCHIVE_ENABLED=false`, `ARCHIVE_PROTECT_RETENTION=false`; rely on pg_dump + normal retention until schema stabilizes.

**Requirement EX1 (P1):** Archive export worker SHALL support a **registry** of table → GCS prefix + retention env (config or code list), so new artifacts are a registry entry—not a redesign.

**Requirement EX2 (P2):** New `social.Register` sources SHOULD document their GCS `social/{name}/` path and `SOCIAL_RETENTION_DAYS` interaction in this file when ingest ships.

---

### TwitchTracker per-stream detail (Camoufox)

- URL: `https://twitchtracker.com/{login}/streams/{streamId}`
- Parses: `meta#ecs` viewer chart, duration, peak, avg, game segments (`internal/analytics/sync.go`).
- Analytics currently sends `useProxy: false` for Tracker detail (`sync.go`); residential proxy is experimental.
- Requires scraper profile: `make up-scraper`, warmed Camoufox profile, `SCRAPER_API_KEY`.

**Requirement TT1 (P0):** Scraper SHALL expose success/failure metrics (chart present, Cloudflare, bytes, duration, `usedPydollFallback`, `timing.challengeHandlerMs` when Pydoll runs).

**Requirement TT2 (P1):** Bulk backfill SHALL respect scraper concurrency (`SCRAPER_MAX_CONCURRENT`, pool size) and cache TTL (`ANALYTICS_TT_STALE_MAX_AGE_MS`, `SCRAPER_TT_DETAIL_CACHE_MS`).

**Requirement TT3 (P1):** Proxy benchmark SHALL compare direct vs residential before enabling proxy for Tracker in production sync.

### TwitchTracker channel summary API

- Direct HTTP: `TWITCHTRACKER_API_URL/channels/summary/{login}` — no Camoufox.
- Safe for bulk 500-channel fetch from any IP (subject to rate limits).

**Requirement TT4 (P0):** Bronze backfill SHALL use summary API without browser scrape.

### Helix (Twitch API)

- VOD archive list, broadcaster IDs, clip metadata, live directory.
- Requires OAuth / app token; rate limits with backoff already in codebase.

**Requirement HX1 (P0):** Bulk index pass SHALL use Helix as primary stream list; Tracker list is enrichment only.

### GQL VOD chat

- Paginated video comments → minute rollups + optional `analytics_vod_chat_messages`.
- Concurrency: `ANALYTICS_VOD_GQL_CONCURRENCY_*` (cap 8).

**Requirement GQL1 (P1):** Gold tier only unless explicitly forced per channel.

**Requirement GQL2 (P0):** Emote preload (`EMOTE_SERVICE_URL`) SHALL run before GQL sync for accurate 7TV/FFZ tokenization.

### 7TV / FFZ / BTTV

- On-demand per channel + optional `SEVENTV_EVENTAPI_ENABLED=true` for deltas.
- Weekly hash snapshot (`ProviderSnapshotNeedsRefresh`) for historical emote set state.
- EventAPI (`SEVENTV_EVENTAPI_ENABLED=true`): websocket deltas trigger `ApplyChannelSet` into Postgres; there is **no separate changelog table** today.

**Requirement EM1 (P1):** Emote set snapshots SHALL be exported to GCS weekly per tracked channel.

**Requirement EM2 (P1):** When EventAPI is enabled, add/remove/rename deltas SHALL either (a) trigger an immediate snapshot export for that channel, or (b) append to `emotes/changelog/date={date}.jsonl.gz` so granularity between weekly snapshots survives Postgres reset.

### Directory sampling

- `PULSE_DIRECTORY_TOP_N` (default 200), interval `PULSE_DIRECTORY_SAMPLE_INTERVAL` (default 10m).
- For bulk archive, increase to 500 and optionally 5m interval during backfill window.

**Requirement DS1 (P1):** Directory samples SHALL be exported to GCS; retention extended or purge only after export confirmed.

### Pulse Wire social (when enabled)

- Reddit LSF, YouTube, Twitch clips, StreamerBans — see `.kiro/steering/pulse-wire.md`.
- YouTube from servers: residential proxy per design (R15.4).
- **X / Twitter (`xrecent`):** stub registered in `cmd/storygraph` (`internal/social/xrecent`); `Healthy()` returns *phase 2 not enabled*. Not scheduled in ingest workers. If enabled later, residential proxy requirements are **stricter** than TwitchTracker and API cost ceilings apply (R15.5, R16).

#### Pulse Wire scrape volume today (not bulk — by design)

`ingestAll` runs every `STORYGRAPH_INGEST_INTERVAL` (default **5m**). Per cycle **item budgets**:

| Source | Max items/cycle | Scrape? | Notes |
|--------|-----------------|---------|-------|
| Reddit LSF | 28 | Sometimes | JSON/OAuth first; Camoufox fallback when blocked |
| StreamerBans | 8 | Sometimes | HTML, emusks sidecar, or scraper fallback |
| Twitch clips | ~12–20 | No | Helix API; seeded from `directory_samples` logins |
| YouTube | 12 | **Yes if no API key** | Up to **48 keywords** expanded; scrape path can hit **one browser page per keyword** until budget filled |
| Directory sampler | top-N rows | No | Metadata `/v1/streams` every **10m** — not social scrape |

**Hidden scrape load:** per Reddit post → up to **8 comments** + **preview HTTP** per evidence URL; per item → clustering + window score recompute on every cycle. **Not** suited for “scrape all of Twitter/Instagram” without redesign.

**Shared scraper contention:** Reddit LSF, YouTube (scrape mode), and Analytics TwitchTracker share one `streamclone-scraper`. YouTube runs **last** and is **skipped** when scraper unhealthy.

**Improvements before scaling social volume:** separate scraper pool or priority queue (Pulse vs Analytics); `YOUTUBE_API_KEY` for discovery; cap keyword scrapes when `use_scraper`; async preview/comment hydration; per-source job queue + GCS export; `social.Register` for new platforms with explicit `Budget.MaxCost` / residential slot.

**Requirement PW1 (P2):** Social items export to GCS before `SOCIAL_RETENTION_DAYS` purge.

**Requirement PW2 (P1):** High-volume social ingest SHALL not share the same scraper queue as TwitchTracker detail without priority tiers or separate egress slots.

**Requirement PW3 (P2):** New `social.Register` sources SHALL declare per-cycle `Budget.MaxItems` plus **`MaxBrowserFetches`** (and `RequiresResidentialEgress` when applicable) before increasing YouTube/Reddit/X scrape volume. `social.Budget.MaxBrowserFetches` now exists in `source.go`; storygraph disables shared-browser social fallback by default with `STORYGRAPH_SOCIAL_BROWSER_FETCH_BUDGET=0` and `STORYGRAPH_YOUTUBE_BROWSER_FETCH_BUDGET=0`, then allows explicit positive caps when the scraper tier has enough capacity.

---

## Residential proxy requirements (Flame Proxies)

Proxy credentials MUST be stored in `.env.local` (gitignored). Run `make scraper-proxy-benchmark` and `make flame-proxy-preflight` before enabling proxy routing.

**Requirement P1 (P0):** Repo SHALL NOT contain live proxy passwords or GCS keys.

**Requirement P2 (P0):** Benchmark script SHALL test at minimum:
- Direct egress (no proxy)
- Flame premium residential profile
- Flame budget residential profile

**Requirement P3 (P0):** Benchmark SHALL probe:
- TwitchTracker stream **detail** (2 URLs)
- TwitchTracker stream **list** page
- Reddit public JSON or LSF HTML fallback target

**Requirement P4 (P1):** Benchmark results SHALL be written to `docs/benchmarks/scraper-proxy-{timestamp}.json`.

**Requirement P5 (P1):** If residential proxy beats direct on Tracker detail with stable chart parse, MAY enable `useProxy` for Tracker via feature flag (`ANALYTICS_TT_USE_PROXY`, TBD).

**Known constraint:** Prior local tests showed **datacenter** proxies worsened Cloudflare on TwitchTracker. Residential proxies are a separate hypothesis — must be measured, not assumed.

Flame Proxies connection shape (from provider dashboard):

```text
PROXY_SERVER=http://proxy.flameproxies.com:8989
PROXY_USERNAME={flame-user-id}-package-{tier}   # e.g. premium package username
PROXY_PASSWORD={password}
```

Optional pool format for `PROXY_POOL` (streamclone-scraper): comma-separated `host:port:user:pass` entries — see scraper README.

---

## Environment & configuration

### Existing (analytics + scraper)

| Variable | Purpose |
|----------|---------|
| `SCRAPER_API_URL` / `SCRAPER_API_KEY` | Browser scraper endpoint |
| `PROXY_SERVER`, `PROXY_USERNAME`, `PROXY_PASSWORD`, `PROXY_POOL` | Scraper egress (`.env.local`) |
| `ANALYTICS_RETENTION_DAYS` | Local rollup retention |
| `ANALYTICS_VOD_CHAT_RETENTION_DAYS` | VOD message retention |
| `PULSE_DIRECTORY_TOP_N` | Directory sample breadth (raise to 500 for bulk) |
| `ALWAYS_TRACKED_CHANNELS` | Gold-tier channel list |
| `ANALYTICS_TT_*` | Tracker cache, prefetch, direct HTTP experiments |

### New (archive — proposed)

| Variable | Default | Purpose |
|----------|---------|---------|
| `ARCHIVE_ENABLED` | `false` | Master switch for **export worker** (upload + manifest writes) |
| `ARCHIVE_STORAGE_PROVIDER` | `azure` | Cold storage backend (`azure` first; GCS later if added) |
| `ARCHIVE_PROTECT_RETENTION` | `false` | When `true`, retention purges consult `archive_exports` manifest; use in backfill/full archive profiles — **not** default desktop |
| `ARCHIVE_AZURE_STORAGE_ACCOUNT` | — | Storage account name |
| `ARCHIVE_AZURE_CONTAINER` | `streamclone-archive` | Private blob container |
| `ARCHIVE_AZURE_PREFIX` | `streamclone` | Blob path prefix |
| `ARCHIVE_AZURE_CONNECTION_STRING_FILE` | — | Path to connection string file (local only; never commit) |
| `ARCHIVE_EXPORT_INTERVAL` | `1h` | Incremental export ticker |
| `ARCHIVE_EXPORT_ON_SYNC` | `true` | Export after each stream sync completes |
| `ARCHIVE_PG_DUMP_NIGHTLY` | `true` | Full Postgres dump to blob storage |
| `BACKFILL_ENABLED` | `false` | Bulk queue worker |
| `BACKFILL_CHANNEL_LIST` | — | Path or blob URI to channel JSON |
| `BACKFILL_TIER` | `silver` | `bronze` \| `silver` \| `gold` |
| `BACKFILL_STREAM_DAYS` | `60` | VOD window for index |
| `PROXY_FLAME_PREMIUM_*` | — | Benchmark profile A (`.env.local`) |
| `PROXY_FLAME_BUDGET_*` | — | Benchmark profile B (`.env.local`) |

During bulk backfill, recommended overrides:

```env
PULSE_DIRECTORY_TOP_N=500
ANALYTICS_RETENTION_DAYS=90
ANALYTICS_VOD_CHAT_RETENTION_DAYS=365
ARCHIVE_ENABLED=true
ARCHIVE_PROTECT_RETENTION=true
ARCHIVE_EXPORT_ON_SYNC=true
```

Low-bandwidth overrides (add to above):

```env
ANALYTICS_VOD_GQL_CONCURRENCY=2
ARCHIVE_EXPORT_INTERVAL=168h
BACKFILL_TIER=silver
# Gold only for ALWAYS_TRACKED_CHANNELS — keep list small
```

---

## IP & deployment split

| Workload | Where to run |
|----------|--------------|
| TwitchTracker Camoufox detail | Home residential IP **or** verified residential proxy |
| TwitchTracker summary API, Helix, 7TV APIs | Any (home, VPS, cloud) |
| GQL VOD chat bulk | Home or cloud with valid Twitch OAuth |
| Reddit / YouTube browser scrape | Residential proxy recommended |
| GCS export / pg_dump | Home (reads local Postgres) |
| Directory sampling | Local stack (metadata service) |

---

## Infrastructure choices & throughput (big scrapes)

### Kubernetes — when it helps vs when it hurts

| Workload | K8s value | Why |
|----------|-----------|-----|
| TwitchTracker Camoufox detail | **Low** | Bottleneck is **residential IP + browser session**, not pod count. One warmed profile per egress IP; scaling replicas without distinct proxies duplicates Cloudflare risk. |
| Helix / TT summary API / 7TV APIs | **Medium** | Stateless; a job queue + N workers on a VPS works in Compose too. K8s helps if you already run a cluster. |
| GQL VOD chat bulk | **Medium** | Parallel workers per stream already exist (up to 8). Cross-stream parallelism is a **queue design** problem, not orchestration. |
| GCS export / `cmd/archive` | **Medium** | CronJob or sidecar is natural on K8s; a nightly script on the home PC is fine for single-operator archive. |
| Grafana / Pulse dashboards | **Already there** | `charts/pulse/` — telemetry only, not scrape path. |

**Verdict:** Compose + home residential IP is the right default for Streamclone scraping. K8s does **not** make Camoufox faster. It helps only if you split **API workers** (cloud) from **browser workers** (residential) and already want cluster ops. Do not move the scraper to K8s first — move the **backfill queue** and **export worker** first.

### Tool choices (current stack)

| Choice | Fit for bulk scrape | Notes |
|--------|---------------------|-------|
| Camoufox + sibling `streamclone-scraper` | **Correct** | Cloudflare requires real browser; persistent profile + cache TTL (`SCRAPER_TT_DETAIL_CACHE_MS`) is the right model. |
| Postgres hot cache | **Correct** | UI + Analytics need fast local queries; GCS is cold tier (this doc). |
| pgx `SendBatch` rollup upserts | **Good** | `BulkUpsertMinuteRollups` / `BulkPatchChatRollups` batch per write. |
| Redis sync locks | **Good** | Per-stream `SetNX`; enables parallel streams at orchestration layer. |
| Parallel GQL inside one VOD | **Good** | Priority heap, hot-segment split, adaptive concurrency — mature path in `sync_gql_parallel.go`. |
| `cmd/backfill` queue | **Missing (planned)** | Today sync is **per-stream API triggered**; no central Silver-tier worker — main gap for “500 channels fast”. |
| GCS + Parquet export | **Planned** | Archive-first during bulk would reduce Postgres write pressure. |

### SQL schema — speed & scale

Schemas are **reasonable for single-home scale** (500 channels × ~60 days), not yet tuned for multi-year warehouse scale.

| Area | Status | Recommendation |
|------|--------|----------------|
| `analytics_minute_rollups` PK `(stream_id, minute_ts)` | **Good** | Right access pattern. `idx_analytics_rollups_stream_minute` largely duplicates PK — harmless but redundant. |
| `emotes_json` JSONB per minute | **Heavy** | Fine at current scale; for bulk Gold, consider top-N emotes per minute cap (`ANALYTICS_TOP_EMOTES_PER_MINUTE`) or export rollups to Parquet and trim JSON in Postgres. |
| `refreshStreamSummary` | **Mitigated hot-path cost** | During incremental GQL, `ANALYTICS_VOD_GQL_DEFER=true` defers summary refresh until the finalizing tail step instead of rescanning rollups after every segment batch. Keep this enabled for Gold-tier backfills unless you are specifically validating summary writes. |
| Time partitioning | **Not yet** | No monthly partitions on rollups / `directory_samples`. Add when row count hurts retention purge or vacuum; not blocking first bulk backfill. |
| BRIN on `minute_ts` / `sampled_at` | **Optional** | Useful for cross-stream time-range analytics; per-stream queries already hit PK. |
| Pulse Wire tables | **Fine** | Indexes on `expires_at`, `(user_ref, created_at)`, window scores — appropriate for ingest + UI. |
| `streamer_follower_snapshots` | **Unused** | Schema OK; no writer wired — see open question #7. |

### Scraper throughput — what actually limits speed

```
Bronze (API)  ──► network + Helix rate limits     (~hours)
Silver (TT)   ──► Camoufox pages × egress IP       (~1–2 weeks @ 1 concurrent)
Gold (GQL)    ──► Twitch GQL + DB rollup writes    (~days–weeks selective)
```

| Knob | Dev default | Bulk Silver target | Risk |
|------|-------------|-------------------|------|
| `SCRAPER_EPHEMERAL_BROWSER` | `true` (Windows) | `false` + warmed profile on Linux/home | Ephemeral avoids deadlocks but **serializes** browser |
| `SCRAPER_MAX_CONCURRENT` | `1` | `2–4` per scraper **instance** | &gt;1 with ephemeral=true deadlocks (compose comment) |
| `SCRAPER_BROWSER_POOL_SIZE` | `1` | `2` with separate contexts | Shares one profile — tune carefully |
| Scraper instances | `1` | **N instances × N residential IPs** (or `PROXY_POOL`) | True horizontal scale for Silver |
| Streams in parallel | Manual / API | **`cmd/backfill` queue** with `M` stream jobs | Each stream can run; TT detail still serial per scraper IP |
| `ANALYTICS_VOD_GQL_CONCURRENCY` | `8` | `4–8` Gold only | Twitch rate limits; DB writes if incremental DB on |

### Priority order for “big scrapes fast”

1. **Ship `cmd/backfill` queue** — Bronze index → Silver viewers-only jobs with persistence, retry, progress (Requirement B4).
2. **Scale scraper egress, not analytics pods** — second home machine, or `PROXY_POOL` with verified residential profiles, each bound to one scraper container + profile dir.
3. **Linux pooled browser** — `SCRAPER_EPHEMERAL_BROWSER=false`, warm profile (`make scraper-preflight`), `SCRAPER_MAX_CONCURRENT=2`.
4. **Keep deferred summary refresh on** during incremental GQL (`ANALYTICS_VOD_GQL_DEFER=true`) — keeps Gold from saturating Postgres.
5. **Archive-first for Silver** — write rollups to GCS Parquet during scrape; Postgres as optional hot cache (reduces local I/O).
6. **Bronze parallel** — Helix + TT summary are embarrassingly parallel; run from any VPS while Silver runs at home.
7. K8s / warehouse partitioning — only after queue + residential scale are proven.

**Requirement PERF1 (P1):** Bulk backfill worker SHALL support configurable concurrency separate from UI-triggered sync (queue depth, scraper routing, viewers-only default).

**Requirement PERF2 (P1):** Rollup summary refresh (`refreshStreamSummary` in `store.go` — called after every `BulkUpsert` / `BulkPatch` / `BulkPatchViewer`) SHOULD be deferrable during incremental GQL (`ANALYTICS_VOD_GQL_INCREMENTAL_DB=true`) until segment or job completion. Required before selective Gold at scale. **Shipped (2026-06):** `ANALYTICS_VOD_GQL_DEFER=true` defers summary refresh until parallel GQL job finalization (`deferred_finalize` mode in `sync_gql_parallel.go`).

---

## Scaling Camoufox + proxies together

Bulk Silver needs **1:1 binding**: one residential egress IP ↔ one warmed browser profile ↔ one Camoufox context (or one scraper container). Sharing a profile across proxies or running many browsers on one IP breaks Cloudflare trust.

### Target scrape-plane layout

```text
                    ┌─────────────────┐
                    │  Job queue      │  Redis / SQS / Postgres jobs table
                    │  (TT URL jobs)  │  priority, retry, scraper affinity
                    └────────┬────────┘
           ┌─────────────────┼─────────────────┐
           ▼                 ▼                 ▼
   ┌───────────────┐ ┌───────────────┐ ┌───────────────┐
   │ scraper-0     │ │ scraper-1     │ │ scraper-N     │
   │ PROXY_POOL[0] │ │ PROXY_POOL[1] │ │ PROXY_POOL[N] │
   │ profile-0/    │ │ profile-1/    │ │ profile-N/    │
   │ MAX_CONC=2    │ │ MAX_CONC=2    │ │ MAX_CONC=2    │
   └───────┬───────┘ └───────┬───────┘ └───────┬───────┘
           └─────────────────┼─────────────────┘
                             ▼
                    ┌─────────────────┐
                    │ ingest workers  │  rollups → Postgres + GCS
                    └─────────────────┘
```

**What helps (in order):**

| Layer | Tool / pattern | Role |
|-------|----------------|------|
| Orchestration | **Job queue** + worker pool | `cmd/backfill` assigns each TT detail URL to a scraper slot; without this, `PROXY_POOL` is unused capacity. |
| Scraper fleet | **N containers**, not N threads in one | Each with `CAMOUFOX_PERSISTENT_PROFILE=/data/profile-{i}`, one `PROXY_*` or `PROXY_POOL[i]`, `SCRAPER_EPHEMERAL_BROWSER=false`. |
| Pool inside container | `SCRAPER_BROWSER_POOL_SIZE` + `SCRAPER_MAX_CONCURRENT` | 2–4 **only** when contexts do not share one profile incorrectly — verify in `streamclone-scraper` sibling repo. |
| Routing | Sticky affinity | Same login’s stream list + details → same scraper when possible (cookie warmth). |
| K8s (optional) | **Deployment per proxy slot** or StatefulSet + volume per profile | Same as N compose stacks; K8s adds scheduling/health, not speed. |
| VPS + home | **Split plane** | Bronze/API on VPS; Silver Camoufox on home IP or residential proxy fleet. |

**Requirement SCALE1 (P1):** Backfill queue SHALL assign browser jobs to scraper workers with explicit **egress slot** id (proxy index or host id); jobs MUST NOT share a profile across different egress IPs.

**Requirement SCALE2 (P2):** Scraper service SHOULD expose per-slot metrics (success, CF block, latency, bytes) keyed by egress slot for proxy benchmarking at scale.

---

## Multi-user data access (read plane)

When others consume data (API, dashboards, exports), **separate scrape plane from read plane**. Scraping stays residential-bound; reads scale on cloud storage and stateless API.

### Layered storage (recommended)

| Tier | Store | Consumers | Notes |
|------|-------|-----------|-------|
| **Cold archive** | **GCS Parquet** (system of record) | Warehouse, batch API, restore | Cheap, durable; bulk scrape should land here. |
| **Hot analytics** | **Postgres** (30–90d) | Streamclone UI, low-latency API | Per-install or small shared instance; not multi-tenant warehouse. |
| **Pulse Wire** | Postgres + retention | Wire UI, story API | Social items, clusters — relational, not time-series. |
| **Dashboards** | **InfluxDB** (optional) | Grafana Emote Pulse only | Already wired: async mirror of minute rollups for Flux charts. |
| **Query API** (future) | **BigQuery / ClickHouse / DuckDB-on-GCS** | External users, heavy analytics | Query Parquet without loading Postgres. |

### InfluxDB — keep, but not as the public API

Today Influx is a **best-effort telemetry sidecar** (`internal/timeseries`) for Grafana — emote/chat/viewer fields per minute, not full `emotes_json` maps. That is the right use.

| Use Influx for | Do **not** use Influx for |
|----------------|---------------------------|
| Ops dashboards, Emote Pulse charts | Primary multi-user data API |
| Real-time “last hour” monitoring while scraping | Pulse Wire stories, social items, receipts |
| Cheap rollups mirror with tags (`stream_category`) | Full VOD chat replay, mod events |
| Alerting on scrape/export health (future) | Long-term cold archive |

If external users need rollups at scale, prefer **GCS Parquet + query engine** or a **read replica Postgres** fed from export — not expanding Influx as source of truth.

### Public / shared data API (sketch)

```text
  External clients
        │
        ▼
  API gateway (auth, rate limits, keys)
        │
   ┌────┴────┐
   ▼         ▼
 Postgres    GCS / warehouse
 (recent)    (historical Parquet)
   │              │
   └──────┬───────┘
          ▼
   Existing shapes: /streams/{id}, rollups series,
   directory samples, pulsewire stories (separate service)
```

- **Reuse** existing `internal/analytics` route shapes where possible; add API keys and strip sync/mutate endpoints for public tier.
- **Historical** endpoints read through GCS (lazy import) or warehouse — Requirement S4 lazy-load.
- **Pulse Wire** stays on `storygraph` API; different retention and auth — do not merge into analytics DB for external API.

### Other infra worth considering (not replacing core choices)

| Tool | When |
|------|------|
| **Redis** (queue) | Backfill job queue, scraper affinity, rate-limit counters for public API |
| **BigQuery** external tables on GCS | Ad-hoc multi-user analytics without ops-heavy ClickHouse |
| **ClickHouse** | High-QPS rollup queries if BigQuery latency/cost hurts |
| **Cloud CDN / signed URLs** | Large Parquet or jsonl.gz downloads for bulk clients |
| **Separate GCS bucket + IAM** | Per-tenant or per-product data access |
| **Prometheus** (already in Pulse chart) | Scraper fleet + export worker SLOs |

**Requirement API1 (P2):** Public data API SHALL read from **export-confirmed** GCS objects or warehouse tables, not from scrape workers directly.

**Requirement API2 (P2):** Multi-user read scaling SHALL not require scaling Camoufox; scrape fleet scales independently on egress slots.

---

## Hosted deployment — what to run in the cloud vs at home

The requirements doc is **flexible** for new data sources (extensibility section) and for **where** each tier runs. Hosting does not mean moving the whole Compose stack to a VPS — it means splitting **three planes**.

### Three planes

```text
┌─────────────────────────────────────────────────────────────┐
│  USER PLANE (what subscribers / visitors use)               │
│  Web app · public API · CDN emotes · (optional) watch relay   │
└───────────────────────────┬─────────────────────────────────┘
                            │ read only
┌───────────────────────────▼─────────────────────────────────┐
│  DATA PLANE (host in cloud — scales with users)              │
│  API gateway · Postgres read replica OR warehouse · GCS     │
│  Redis cache · MinIO/CDN emotes · storygraph + analytics API│
└───────────────────────────┬─────────────────────────────────┘
                            │ ingest / export
┌───────────────────────────▼─────────────────────────────────┐
│  SCRAPE PLANE (residential-bound — home or proxy fleet)       │
│  Camoufox scrapers · backfill queue · ingest workers · export│
└─────────────────────────────────────────────────────────────┘
```

### What to **host** (cloud / managed)

| Component | Why host | Product |
|-----------|----------|---------|
| **Public web app** (Vite SPA behind CDN) | Static assets, global latency | All surfaces |
| **Caddy / API gateway** | TLS, auth, rate limits, same-origin routing | User entry |
| **storygraph API** | Pulse Wire read paths (`/v1/pulse-wire/*`) | Pulse Wire |
| **analytics API** (read-only tier) | Stream rollups, charts JSON | Analytics |
| **metadata API** | Directory, Helix-backed lists | Wire Rising + directory |
| **emote service** + **MinIO** (or GCS + CDN) | Shared 7TV/FFZ/BTTV render cache | Chat emotes for all users |
| **Postgres** (managed) | Hot 30–90d social + stories + rollups | Data plane |
| **Redis** | Chat dict cache, API cache, rate limits | Performance |
| **GCS** | Cold archive, Parquet exports, large downloads | Archive + API historical |
| **Warehouse** (optional BigQuery/ClickHouse) | Historical rollup queries at scale | Data API |
| **Grafana + Influx + Prometheus** (`pulse` profile) | **Operators only** — not subscriber UI | Pulse ops |

### What **not** to host in a datacenter (without residential proxy)

| Component | Why keep off vanilla VPS |
|-----------|--------------------------|
| **TwitchTracker Camoufox detail** | Cloudflare; needs home IP or residential proxy per slot |
| **YouTube search scrape** | Browser scrape; use `YOUTUBE_API_KEY` in cloud or scrape at home |
| **Reddit scraper fallback** | Prefer OAuth/API in cloud; Camoufox fallback on scrape plane |
| **Bulk Silver/Gold backfill** | Same egress constraints as Analytics |
| **Primary scrape Postgres writes** | Can run in cloud **if** ingest is API-only; browser scrape jobs stay on scrape plane |

Scrape plane can be: your home PC, a small fleet of machines with residential IPs, or proxy-bound K8s/compose **slots** — not “10 scraper replicas on one DC IP.”

### What **users** use vs what **operators** use

| Audience | Uses | Does not need |
|----------|------|----------------|
| **End users / API subscribers** | `https://yourdomain/` web app; optional API keys for `/v1/pulse-wire/*`, read-only `/v1/analytics/*`; emotes at `/emotes/…`; Twitch login only if chat send / follows | Grafana, scraper ports, Postgres admin, compose profiles, residential proxies |
| **Power users (local Streamclone)** | Full desktop install — watch + chat + local Analytics sync + optional local scrape | Your cloud scrape fleet (unless you centralize data) |
| **Operators / you** | Grafana Pulse, Prometheus, source-health, backfill CLI, scraper logs, GCS console, proxy benchmarks | — |

Two coexistence models:

1. **Central data + local watch** — Users subscribe to Wire/Analytics API and web; power users still run local Streamclone for HLS/chat/emotes on their machine, pulling your APIs for charts and news.
2. **Full hosted Streamclone** — You also host video relay, chat IRC, per-viewer emote sync — highest cost and ops; only if watch desk in the cloud is the product.

Default recommendation for your roadmap: **model 1** first (data publisher), keep Core Watch as local or Twitch-native for viewers.

### How each product surface works when hosted

| Surface | User sees | Backend | Scraping / compute |
|---------|-----------|---------|-------------------|
| **Core Watch** | Directory, player, chat, emotes | Local install **or** hosted relay (heavy) | Live: Helix + IRC; not archive bulk |
| **Analytics** | Charts, stream picker, sync UI | `analytics` service + Postgres rollups; historical from GCS lazy-load | TT detail + GQL on **scrape plane**; API serves merged rollups |
| **Pulse Wire** | `/pulse-wire` Trending + Wire | `storygraph` + `social_items` / clusters | `ingestAll` on scrape plane or co-located with API if API-only sources |
| **Pulse (Grafana)** | `:3000` Emote Pulse dashboards | Influx mirror of rollups | **Internal ops**; async export from analytics writes |
| **7TV / emotes** | Images in chat + Wire thumbs | `emote` service: sync 7TV/FFZ/BTTV APIs → render WebP → MinIO; Redis dict per channel | On-demand per channel + EventAPI deltas; **host emote service + object storage** scales well for many users |

**7TV load pattern for multi-user:** One shared emote service preloads sets for `ALWAYS_TRACKED_CHANNELS` + directory top-N + channels users open. Redis holds hot dictionaries; MinIO/CDN serves `{emote_id}/{scale}.webp`. Do not run per-user emote workers — run **shared cache** with bounded preload + EventAPI for tracked channels. Archive exports **metadata snapshots** to GCS, not every WebP (C-tier).

### Suggested hosting groups (compose / K8s profiles)

| Group | Services | Profile name (today) |
|-------|----------|----------------------|
| **edge** | Caddy, frontend | core (always) |
| **data-api** | metadata, analytics, storygraph, emote, chat, video (if hosted watch) | core |
| **data-store** | Postgres, Redis, MinIO | core |
| **scrape** | scraper, x-ingest, backfill worker, archive export | scraper + pulse-wire |
| **ops** | Grafana, Influx, Prometheus | pulse |
| **not in group** | MediaMTX, streamlink workers | Only if hosting live relay |

**Requirement HOST1 (P2):** Production deployment docs SHALL treat scrape plane and data plane as independently scalable groups.

**Requirement HOST2 (P2):** Public user API SHALL not expose scrape-worker or database admin endpoints.

**Requirement HOST3 (P1):** Central hosted emote service SHALL use shared Redis + object storage; per-install emote SQLite/MinIO is local-dev only.

---

## Bandwidth & low-bandwidth mode

Rough transfer volumes for a full Bronze → Silver → Gold backfill on **top 500** channels (home residential IP):

| Phase | Data transferred | Direction | Notes |
|-------|------------------|-----------|-------|
| Bronze (Helix index + TT summary) | ~50–100 MB | Download | Trivial |
| Silver (TT detail × ~10k streams) | ~5 GB | Download | ~500 KB/page; Camoufox speed dominates, not bandwidth (~30 MB/h @ 60/h) |
| Gold (GQL chat × ~2k streams) | ~10–30 GB | Download | Dense 10h stream can be 50–100 MB JSON; main bandwidth hog |
| GCS uploads (rollups + metadata) | ~5–20 GB | Upload | zstd Parquet / `.jsonl.gz` compress ~5–10× vs raw JSON |
| Ongoing daily incremental | ~200–500 MB/day | Both | Depends on live tracking breadth |

**Low bandwidth recommendations** (&lt;10 Mbps upload or &lt;25 Mbps down):

1. **Silver is safe** — sustained ~3 MB/h download at typical Camoufox concurrency.
2. **Gold hurts** — mitigate with:
   - `ANALYTICS_VOD_GQL_CONCURRENCY` 2–4 instead of 8 (less burst traffic).
   - Gold tier only for `ALWAYS_TRACKED_CHANNELS` (e.g. 50 logins, not 500).
   - Run GQL during off-hours.
3. **GCS upload** — run overnight; prefer Parquet+zstd over raw JSON.
4. **Skip MinIO→GCS emote mirror** — CDN redownload is faster than uploading local WebP.
5. **Export cadence** — on slow upload, prefer weekly compressed batch (`ARCHIVE_EXPORT_INTERVAL=168h` or nightly pg_dump + selective table export) over hourly small deltas that waste connection setup.

**Requirement BW1 (P1):** Documented env overrides for low-bandwidth backfill SHALL include reduced GQL concurrency, explicit Gold channel list, and configurable `ARCHIVE_EXPORT_INTERVAL`.

---

## Implementation phases

**Priority:** manifest + purge guard + sync export hook **before** broad backfill — floor that prevents re-scraping expensive Tracker/GQL data.

### Phase 0 — Requirements & benchmark (current)

- [x] This requirements doc
- [x] Proxy benchmark script + baseline JSON results (summary in [`proxy-benchmark.md`](proxy-benchmark.md); raw JSON local under `docs/benchmarks/` — 2026-06-20: premium 5/5, budget 4/5)
- [x] `.env.local` proxy profiles (user-provided, not committed)

### Phase 1 — Export floor (manifest + guard + backup)

1. [x] `archive_exports` manifest (Postgres table + migration)
   - Implemented in migration `000030_archive_exports` plus `internal/archive.ManifestStore`.
2. [x] `internal/archive.Writer` — JSONL.gz + pg_dump gzip to Azure Blob; manifest upsert on success
   - `internal/archive/writer.go`, `exporter.go`, Azure SDK; wired in `cmd/analytics` when `ARCHIVE_ENABLED=true`.
3. [x] Shared retention guard; wire all five purge call sites (see A4 table)
   - Guard is opt-in through `ARCHIVE_PROTECT_RETENTION` and blocks deletes when confirmed manifest rows are missing.
4. [x] `ARCHIVE_PROTECT_RETENTION` env (default `false`); document desktop vs backfill profiles
5. [x] Extend `scripts/backup-streamclone.ps1`: gzip, Azure upload instructions, restore smoke path
6. [x] Sync export hook: between `SyncHistoricalStream` success and `SyncPhaseCompleted`; export-pending on failure
   - `ARCHIVE_EXPORT_ON_SYNC=true` now enters `exporting_archive`; without a configured exporter, sync becomes terminal `export_pending` instead of falsely completing.
7. [x] Hourly incremental rollup export (optional ticker; manifest-backed)
   - `ARCHIVE_EXPORT_INTERVAL` ticker in `cmd/analytics` scans rollups missing/stale confirmed manifest rows and calls `internal/archive/exporter.go`.

### Phase 2 — Bronze bulk index

- [x] Channel list generator (top 500 + always-tracked)
- [x] Helix VOD index + Tracker summary for all channels
- [x] Persist to Azure Blob + manifest

### Phase 3 — Silver Tracker backfill

- [x] Durable `backfill_jobs` queue (not sync-endpoint wrapper)
- [x] Viewers-only sync per stream; egress_slot assignment
- [x] Export + manifest before job `done`
- [x] Progress API / CLI (`cmd/backfill status`)

### Phase 4 — Gold selective chat

- [x] Defer `refreshStreamSummary` during incremental GQL (PERF2)
- [x] GQL chat queue with rules engine (`gold_rules.go`, `gold_enqueuer.go`, gold tier in `backfill_worker.go`)
- [x] VOD chat export to Azure (`ExportVODChat` → `vod_chat/stream_id={id}/messages.jsonl.gz`)
- [ ] Parquet rollups (WR2) after restore tests pass

### Phase 5 — Restore & lazy load

- [x] Selective `archive restore --stream-id` from JSONL.gz / manifest
- [ ] Optional Analytics read-through cache (S4)
- [ ] Warehouse / BigQuery external tables (read plane — optional)

---

## Acceptance criteria

1. Postgres reset + Azure Blob restore (`archive restore --stream-id`) yields Analytics charts for previously synced streams **without** TwitchTracker re-scrape. Operator steps: [azure-archive-setup.md](../azure-archive-setup.md#restore-rollups).
2. Bronze index for 500 channels completes in < 24h on residential home IP.
3. Proxy benchmark produces comparable JSON for direct vs premium vs budget profiles.
4. No secrets in git history from this work.
5. Retention purge never deletes rows lacking a confirmed `archive_exports` row when `ARCHIVE_PROTECT_RETENTION=true` (MAN2).
6. `chat_mod_events` exported before `CHAT_LOG_RETENTION_DAYS` purge when mod-audit history is enabled.
7. Default desktop install (`ARCHIVE_PROTECT_RETENTION=false`) retains current retention behavior without unbounded growth from blocked purges.

---

## Implementation gaps (code review 2026-06-20)

| Priority | Gap | Shipped | Remaining |
|----------|-----|---------|-----------|
| **P0** | Export manifest | Migration `000030`, `internal/archive.ManifestStore`, MAN1 upsert on upload | — |
| **P0** | Retention guard scope | `ARCHIVE_PROTECT_RETENTION` + manifest consult at five purge call sites | — |
| **P0** | Sync → export hook | `exporting_archive` / `export_pending` between sync success and terminal status (B4) | — |
| **P1** | Backup script | `backup-streamclone.ps1`: gzip dump + connection-string Azure upload | Nightly automation verification |
| **P1** | Archive writer | `internal/archive.Writer` + Azure SDK; rollups, session, TT detail, roster, Bronze artifacts | Parquet encoder (WR2) |
| **P1** | Backfill queue | Migration `000033`, `backfill_jobs`, `BackfillWorker`, `cmd/backfill status` | — |
| **P1** | Hourly incremental export | `ARCHIVE_EXPORT_INTERVAL` ticker in `cmd/analytics` | — |
| **P1** | Bronze bulk index | Migration `000034`, `BronzeIndexer`, `cmd/backfill bronze run-once` | Full 500-channel production run |
| **Done** | Summary refresh | `ANALYTICS_VOD_GQL_DEFER=true` defers refresh during parallel GQL (PERF2) | — |
| **Done** | Social scrape budget | `MaxItems` + `MaxBrowserFetches` cap browser fetches | — |
| **Done** | Canonical sessions + Tier-0 | Migrations `000031`–`000032`, `ResolveOrCreateSession`, prefetch stub cleanup | — |
| **Done** | Post-end Silver queue | Migration `000033`, `post_end.go`, Silver TT gap-fill worker | — |

**What’s already strong:** Azure Blob cold / Postgres hot split, residential scrape plane, Camoufox for Tracker, measured proxy benchmark, scrape vs read plane separation, extensibility for Pulse Wire sources.

---

## Open questions

1. Does Flame premium residential pass TwitchTracker `meta#ecs` parse more reliably than direct home IP after Cloudflare challenges?
2. Tracker detail: store raw HTML in GCS for re-parse/debug, or only derived rollups + game segments?
3. Phase 1 restore: selective artifacts first, or nightly `pg_dump` sufficient until Phase 3 Silver?
4. Public read plane: BigQuery external tables on GCS, or restore-to-Postgres + API cache for v1?
5. Separate GCS bucket vs prefix in existing 5TB bucket?
6. Low bandwidth: weekly compressed batch vs hourly incremental export default?
7. `streamer_follower_snapshots`: wire `InsertFollowerSnapshot` or remove dead path?
8. `xrecent` Phase 2: residential proxy budget vs TwitchTracker?
9. Hosted product: central data API only vs full hosted watch desk?
10. Single Postgres for Wire + Analytics in cloud, or split databases?
11. **A4 default:** `ARCHIVE_PROTECT_RETENTION=false` on desktop — confirmed in doc; backfill profile sets `true`.

---

## Coverage checklist (review 2026-06)

| Status | Item |
|--------|------|
| Covered | Rollups, streams, TT detail, GQL chat, game segments, emote set snapshots, directory samples, social items |
| Added | Follower snapshots, mod events, EventAPI delta logging (EM2), user-state pg_dump callout, X stub note |
| Added | Manifest-based A4, ARCHIVE_PROTECT_RETENTION, sync export hook (B4), archive.Writer JSONL-first |
| Added | Implementation gaps table, Phase 1 reorder (floor before backfill) |
| Architecture | Azure Blob layout, phasing, connection-string credentials, residential vs datacenter split unchanged |

---

## Related files

| Path | Role |
|------|------|
| `docs/scraper-cloudflare-and-proxy.md` | Operator scraper/proxy notes |
| `scripts/scraper-proxy-benchmark.ps1` | Proxy benchmark runner |
| `scripts/scraper-turnstile-benchmark.ps1` | Pydoll Turnstile benchmark runner |
| `scripts/scraper-preflight.ps1` | Direct egress health check |
| `scripts/backup-streamclone.ps1` | Postgres + MinIO backup |
| `internal/analytics/sync.go` | Tracker + GQL sync |
| `deploy/env/profile-scraper.env` | Scraper compose defaults |
