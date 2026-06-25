# Azure Blob → Cloudflare R2 migration plan (read-only audit)

| | |
|---|---|
| **Status** | Phase 2B complete (31-object staging sample mirror) — **no production cutover** |
| **Owner** | Aron-Chu |
| **Date** | 2026-06-25 |
| **Product** | StreamPulse long-term analytics artifacts |
| **Source of truth (unchanged)** | BearHost Postgres — app rows, `archive_exports`, queues, VOD Library (future), saved moments |

---

## Executive summary

Streamclone/StreamPulse cold archive is **implemented in production against Azure Blob only**. The Go `archive` package writes through a `BlobStore` interface (`internal/archive/writer.go`); only `AzureBlobStore` is wired. Manifest rows live in Postgres `archive_exports` (`gcs_uri` column name is legacy — values are Azure HTTPS URIs).

**R2 (personal account `51dd8007…`):** enabled; **`streampulse-artifacts-staging`** holds **31 verified sample objects** (~5.2 KiB; rollups, streams, vod_catalog). **No production R2 buckets.** **No production read-path change.** Azure remains authoritative for all production reads/writes.

**Task ledger:** [tasks.md](./tasks.md) (`STOR-R2-001`–`004` done; `STOR-R2-005` pending).

**Recommended path:**

1. **Phase 0** — operator inventory on Azure (counts/sizes) using existing scripts + `az storage blob list` (read-only). **Done 2026-06-25.**
2. **Phase 1** — enable R2 on personal account + budget alerts; create **staging** bucket only. **Done 2026-06-25** (`streampulse-artifacts-staging`).
3. **Phase 2A** — one-object mirror + verify. **Done 2026-06-25.**
4. **Phase 2B** — mirror **31** sample objects ([`sample-manifest-phase2a.csv`](./sample-manifest-phase2a.csv)) — **STOR-R2-002**, **done 2026-06-25**.
5. **Phase 3** — `R2BlobStore` + `ReadThroughStore` in Go (**STOR-R2-003**, **done 2026-06-25**) — **flags default off**; production unchanged.
6. **Phase 3.5** — R2 restore drill on staging sample (**STOR-R2-004**, **done 2026-06-25**) — read-through validated locally; **no BearHost flag flip**.
7. **Phase 4** — batch migration by prefix — **STOR-R2-005**; keep Azure fallback; no Azure deletes until verified.

**Do not** migrate Postgres to D1, route VOD Library through Workers, or store user library rows in R2.

---

## Phase 0 — Azure inventory (repo audit)

### 0.1 Environment variables and config

| Variable / file | Purpose |
|-----------------|--------|
| `ARCHIVE_AZURE_STORAGE_ACCOUNT` | Storage account name |
| `ARCHIVE_AZURE_CONTAINER` | Default `streamclone-archive` |
| `ARCHIVE_AZURE_PREFIX` | Default `streamclone` (blob key prefix inside container) |
| `ARCHIVE_AZURE_CONNECTION_STRING` | Inline conn string (dev only) |
| `ARCHIVE_AZURE_CONNECTION_STRING_FILE` | File mount, e.g. `/run/streamclone-secrets/azure-archive-connection-string`, `~/.streamclone/azure-archive-connection-string` |
| `ARCHIVE_ENABLED` | Master switch for export workers |
| `.env.example` | Documents Azure archive vars |
| `deploy/env/profile-archive.env` | Enables archive on workers |
| `deploy/env/profile-bearhost-prod.env` | Secret file path for BearHost |
| `deploy/docker-compose.bearhost-prod.yml` | Mounts secrets into analytics-workers |
| `deploy/docker-compose.azure-archive-plane.yml` | Mode B Azure archive plane |

**Terraform / IaC**

| Path | Role |
|------|------|
| `deploy/terraform/azure/archive/` | Storage account, container, cool-tier lifecycle (90d), budget, smoke blob |
| `deploy/terraform/azure/compute/` | Optional Azure VM (scraper/archive plane) |
| `.github/workflows/azure-archive-terraform.yml` | CI for Terraform |
| `scripts/azure-archive-fresh-start.sh` | One-shot apply + local creds to `~/.streamclone/` |
| `scripts/install-azure-archive-tools.sh` | Operator tooling |

**Docs**

| Path | Role |
|------|------|
| `docs/azure-archive-setup.md` | Operator setup |
| `docs/azure-archive-plane.md` | Hybrid plane |
| `docs/azure-archive-cicd.md` | CI/CD |
| `docs/scraping-archive/requirements.md` | Archive requirements, env table |
| `docs/scraping-archive/corpus-requirements.md` | Corpus blob layout |
| `docs/scraping-archive/artifact-natural-keys.md` | Manifest natural keys ↔ blob paths |
| `docs/finalplan.md` | Example blob URLs (`ststreamclone3lf6tt`) |

**Scripts (read / list / upload — no migration yet)**

| Script | Role |
|--------|------|
| `scripts/backup-streamclone.ps1` | pg_dump gzip → optional Azure upload under `postgres/nightly/` |
| `scripts/archive-restore-drill.sh` | Restore drill (reads Azure) |
| `scripts/bearhost-corpus-smoke.sh` | Requires Azure secret file |
| `scripts/bearhost-corpus-preflight.sh` | Validates secret path |
| `scripts/storage/azure-prefix-inventory.sh` | Phase 0.6 per-prefix aggregate (metadata only) |
| `scripts/storage/azure-top-prefixes.sh` | Top-level virtual folders + sub-prefix counts |
| `scripts/storage/azure-extra-prefixes.sh` | Additional prefixes (`directory/`, `viewer_rollup/`) |
| `scripts/tmp/batch-b2-azure-list-nightly.sh` | Nightly backup prefix list (legacy tmp) |
| `scripts/azure-archive-acceptance.ps1` | Acceptance checks |
| `cmd/archive/main.go` | CLI restore/export against Azure |
| `cmd/backfill/main.go` | Archive client for backfill paths |
| `cmd/analytics/main.go` | Wires `NewAzureBlobStore` when `ARCHIVE_ENABLED` |

### 0.2 Azure storage layout (from code + docs)

**Container:** `streamclone-archive` (default)
**Prefix:** `streamclone/` (all keys relative to prefix in `AzureBlobStore.fullKey`)

Full object path pattern:

```text
https://{account}.blob.core.windows.net/streamclone-archive/streamclone/{key}
```

**Blob key domains** (`internal/archive/writer.go`, `artifact-natural-keys.md`):

| Prefix / key pattern | artifact_type(s) | Typical format | Reproducible? |
|----------------------|------------------|----------------|---------------|
| `rollups/stream_id={id}/part-000.jsonl.gz` | `analytics_rollups` | gzip JSONL | Re-export from Postgres if hot data kept |
| `streams/stream_id={id}/session.json.gz` | `analytics_stream` | gzip JSON | Same |
| `streams/channels/{login}/stream_id={id}.jsonl.gz` | channel index | gzip | Same |
| `vod_chat/stream_id={id}/messages.jsonl.gz` | `vod_chat_message`, gold | gzip JSONL | **Expensive** — GQL re-fetch |
| `rollups/chat/stream_id={id}/minute.jsonl.gz` | `gold_lite` | gzip | Re-export if rollups in PG |
| `tt-detail/{login}/{stream_id}/page.html.gz` | `tt_detail_html` | gzip HTML | Re-scrape (CF risk) |
| `raw/twitchtracker/stream_id={id}/chart.json.gz` | `tt_chart_json` | gzip | Re-scrape |
| `vod_catalog/v1/login={login}/date={date}/...` | `bronze_vod_catalog` | gzip JSONL | Re-Helix index |
| `channels/identity/...`, `channels/crosswalk/...` | bronze identity | gzip | Re-export |
| `channels/vod_index/{login}.jsonl.gz` | `bronze_vod_index` (legacy) | gzip | Re-export |
| `channels/top500.json.gz`, `channels/top200.json.gz` | roster | gzip JSON | Re-generate |
| `rosters/tier0/date={date}/...` | `bronze_roster` | gzip | Re-export |
| `emotes/snapshots/provider={p}/login={login}/date={date}/...` | `emote_snapshot`, `emote_snapshot_global` | gzip | Re-sync 7TV |
| `emotes/changelog/...` | `emote_changelog` | gzip | Append-only events |
| `postgres/nightly/{date}.sql.gz` | (backup script) | gzip SQL | **Must preserve** — full DB snapshot |
| `smoke-tests/` | smoke | small | Disposable |

### 0.3 Read paths (code)

| Consumer | Mechanism |
|----------|-----------|
| `internal/archive/restore.go` | `blob.Get` rollups + session for stream restore |
| `internal/archive/verify.go` | URI parse + existence check |
| `internal/analytics/silver_enqueuer.go` | `blob.Get` VOD index |
| `internal/archive/emote_exporter.go` | `blob.Get` prior emote snapshot for diff |
| `cmd/archive restore` | CLI restore |
| Retention guard | `archive_exports` confirmed rows before PG purge |

### 0.4 Write paths (code)

| Producer | Mechanism |
|----------|-----------|
| `internal/archive/writer.go` | `Put` / `putGzip` — all export types |
| `internal/archive/exporter.go` | Sync export on stream sync |
| `internal/archive/bronze_export.go` | Bronze corpus |
| `internal/archive/emote_exporter.go` | Emote snapshots/changelog |
| `internal/archive/chat_aggregate.go` | Gold-lite |
| `internal/archive/pulsewire_export.go` | Pulse Wire raw |
| `scripts/backup-streamclone.ps1` | Direct Azure SDK upload (parallel to Go writer) |

### 0.5 Lifecycle / retention (Azure)

From `deploy/terraform/azure/archive/main.tf`:

- **Cool tier** after **90 days** for blobs under `{archive_prefix}/`
- **Versioning** optional via `enable_versioning`
- **Monthly budget** alert (~$5 default) via Terraform
- **Retention guard** in app: Postgres purge blocked if `archive_exports` not `confirmed` (`internal/archive/manifest.go`)

**Do not modify** production lifecycle/delete policies during this audit.

### 0.6 Live inventory (verified 2026-06-25)

Read-only listing via `az storage blob list` (metadata only). Connection string from `~/.streamclone/azure-archive-connection-string` (not committed). Storage account resolved from connection string (example name in docs: `ststreamclone3lf6tt`).

**Commands run:**

```bash
bash scripts/storage/azure-prefix-inventory.sh
bash scripts/storage/azure-top-prefixes.sh
bash scripts/storage/azure-extra-prefixes.sh
```

**Inventory table (required prefixes):**

| Prefix | Object count | Total bytes | Extensions | Last modified range | Tier | Hot/warm/cold | Preserve? |
|--------|-------------|-------------|------------|---------------------|------|---------------|-----------|
| `rollups/` | 1,758 | 4,133,865 (~3.9 MiB) | `.jsonl.gz` (1758) | 2026-06-20 → 2026-06-25 | Hot (1758) | **hot** — active export path | **yes** during migration; reproducible from PG if rollups retained |
| `vod_chat/` | 552 | 6,549,790 (~6.2 MiB) | `.jsonl.gz` + provenance (552) | 2026-06-21 → 2026-06-24 | Hot (552) | **warm** — expensive GQL rebuild | **yes — defer bulk copy until egress budget** |
| `emotes/snapshots/` | 12 | 445,306 (~435 KiB) | `.json.gz` (12) | 2026-06-20 → 2026-06-22 | Hot (12) | **warm** — 7TV re-sync | reproducible; preserve for diff convenience |
| `postgres/nightly/` | 3 | 68,781,090 (~65.6 MiB) | `.sql.gz` (3) | 2026-06-24 → 2026-06-25 | Hot (3) | **cold** — backup class | **yes — must preserve; do not sample-copy until R2 restore tested** |
| `tt-detail/` | 552 | 9,574,032 (~9.1 MiB) | `.html.gz` (552) | 2026-06-21 → 2026-06-25 | Hot (552) | **cold** — debug/deprecated scrape | optional; lowest migration priority |
| `channels/` | 1,571 | 2,681,841 (~2.6 MiB) | `.json.gz` (788), `.json` (783) | 2026-06-20 → 2026-06-25 | Hot (1571) | **warm** — bronze metadata | mostly reproducible; preserve during migration |
| `vod_catalog/` | 1 | 3,771 (~3.7 KiB) | `.jsonl.gz` (1) | 2026-06-21 | Hot (1) | **warm** | reproducible; good small sample candidate |

**Additional top-level domains** (not in required table; included in operator totals):

| Prefix | Object count | Total bytes | Notes |
|--------|-------------|-------------|-------|
| `streams/` | 3,520 | 1,102,274 | includes `streams/channels/` (1,760 objs / 551 KiB) + `session.json.gz` exports |
| `viewer_rollup/` | 1,206 | 3,533,279 | corpus viewer rollups — not in original table |
| `directory/` | 21 | 51,690 | small bronze directory snapshots |
| `rosters/` | 1 | 4,752 | tier0 roster |
| `raw/`, `emotes/changelog/`, `smoke-tests/` | 0 | 0 | empty today |

**Note:** All objects are **Hot** tier — archive container is young (~5 days); Terraform cool-tier (90d) not yet applied to these blobs.

### 0.7 Operator summary (Phase 0.6)

| Metric | Value |
|--------|-------|
| **Total object count** | **~9,197** (sum of listed domain prefixes; full paginated container walk not completed — `az` marker pagination required above 5k root listing) |
| **Total bytes** | **~96,861,690** (~92.4 MiB / ~0.09 GiB) |
| **Largest prefixes by bytes** | 1) `postgres/nightly/` 68.8 MiB (71%) · 2) `tt-detail/` 9.1 MiB · 3) `vod_chat/` 6.2 MiB · 4) `rollups/` 3.9 MiB · 5) `viewer_rollup/` 3.4 MiB |
| **Largest prefixes by count** | 1) `streams/` 3,520 · 2) `rollups/` 1,758 · 3) `channels/` 1,571 · 4) `viewer_rollup/` 1,206 · 5) `vod_chat/` + `tt-detail/` 552 each |
| **Likely Azure egress risk** | **Low today** (~93 MiB total). **Future risk is high** if `vod_chat/` and `postgres/nightly/` grow without compression/batching. First Phase 2 sample batch (~50 small rollups + sessions + 1 emote snapshot) ≈ **< 1 MiB egress**. Full `postgres/nightly/` mirror ≈ **69 MiB** per backup generation. |
| **Recommended first sample batch** | 10–20× `analytics_rollups` (1–3 KiB each) · 5–10× `analytics_stream` / `streams/stream_id=*/session.json.gz` · 1× smallest `emotes/snapshots/` (or `emote_snapshot_global` if confirmed in Postgres) · 1× `vod_catalog/` (3.7 KiB) · optional 1× `channels/crosswalk` gzip |
| **Prefixes — do not touch yet** | `postgres/nightly/` (until R2 enabled + backup restore drill on R2) · bulk `vod_chat/` (egress + size growth) · `tt-detail/` (optional/debug) · any prefix without matching `archive_exports` confirmed row |

---

## Phase 1 — R2 target design (prep only)

### 1.1 Cloudflare account ownership

| Account | ID | Current use | R2 status (2026-06-25) |
|---------|-----|-------------|------------------------|
| **Personal** (`aron.chu90@gmail.com`) | `51dd8007b22ac92482388d8b6cdbb6e3` | DNS zone `streampulse.stream`, prod Pages, `api` tunnel DNS, **R2 staging** | **Enabled** — staging bucket live |
| **ASU** (`amchu2@asu.edu`) | `513d89373bebfc78b66e243b80f4debf` | Staging Pages, edge Worker, D1, KV | **R2 disabled** — not used for artifact migration |

**Operator action required before any bucket create:**

1. Cloudflare Dashboard → R2 → **Enable** on personal account (recommended prod owner).
2. Set **billing budget alerts** on personal account (and ASU if staging bucket needed there).
3. Re-run: `CLOUDFLARE_ACCOUNT_ID=51dd8007b22ac92482388d8b6cdbb6e3 npx wrangler r2 bucket list`

**Recommendation**

| Bucket | Account | Rationale |
|--------|---------|-----------|
| `streampulse-artifacts-staging` | ASU (after R2 enable) **or** personal | Staging mirror tests |
| `streampulse-artifacts-prod` | **Personal** | Same trust boundary as zone; long-lived prod artifacts |
| `streampulse-backups-prod` | **Personal** | pg_dump mirrors; separate lifecycle rules |

### 1.2 Proposed R2 buckets — PREPARED, NOT EXECUTED

```powershell
# Prerequisites: R2 enabled + budget alerts in dashboard (personal account)
$env:CLOUDFLARE_ACCOUNT_ID = '51dd8007b22ac92482388d8b6cdbb6e3'

# Production artifact bucket
npx wrangler r2 bucket create streampulse-artifacts-prod

# Postgres nightly / backup-class objects (optional split)
npx wrangler r2 bucket create streampulse-backups-prod

# Optional staging (personal — use while ASU R2 disabled)
npx wrangler r2 bucket create streampulse-artifacts-staging
```

```powershell
# Optional: ASU staging bucket (only after R2 enabled on 513d8937…)
$env:CLOUDFLARE_ACCOUNT_ID = '513d89373bebfc78b66e243b80f4debf'
npx wrangler r2 bucket create streampulse-artifacts-staging
```

**Bucket status (2026-06-25):** `streampulse-artifacts-staging` **created** on personal account. Prod/backups buckets **not** created.

---

## Phase 2A — R2 enable, sample manifest, staging bucket, one-object dry run

**Status:** R2 **enabled** on personal account. Staging bucket **created**. Phase 2A one-object proof **done**. Phase 2B sample batch **done** (**STOR-R2-002**, 2026-06-25).

### 2A.1 R2 verification

| Check | Result |
|-------|--------|
| Command | `CLOUDFLARE_ACCOUNT_ID=51dd8007b22ac92482388d8b6cdbb6e3 npx wrangler r2 bucket list` |
| After personal R2 subscription | **Success** — empty list, then bucket visible after create |
| Wrangler note | Run R2 commands from **streamclone repo root** (no `wrangler.toml`) — `streampulse-web/wrangler.toml` pins ASU `513d8937…` and caused `10042` on create |

### 2A.2 Bucket status

| Bucket | Account | Status |
|--------|---------|--------|
| `streampulse-artifacts-staging` | Personal `51dd8007…` | **Created** 2026-06-25 (`wrangler r2 bucket create` from streamclone repo root) |
| `streampulse-artifacts-prod` | — | Not created (not approved) |
| `streampulse-backups-prod` | — | Not created (not approved) |

**Wrangler (personal account — use streamclone repo root, not streampulse-web/):**

```powershell
$env:CLOUDFLARE_ACCOUNT_ID = '51dd8007b22ac92482388d8b6cdbb6e3'
cd C:\Users\Aron\twitch-7tv-clone
npx wrangler r2 bucket list
```

### 2A.3 Sample manifest (BearHost Postgres — read-only)

**Source:** BearHost `archive_exports` via `BEARHOST_SAMPLE_MANIFEST_REMOTE=1 bash scripts/storage/archive-exports-sample-manifest.sh --csv`

**Artifact file:** [`sample-manifest-phase2a.csv`](./sample-manifest-phase2a.csv) — **31 data rows** (20× `analytics_rollups`, 10× `analytics_stream`, 1× `bronze_vod_catalog`)

Excludes: `vod_chat/`, `postgres/nightly/`, `tt-detail/`, `viewer_rollup/` hive rows (sample SQL filters to canonical `rollups/` and `streams/` paths).

| artifact_type | natural_key | byte_size | proposed R2 key |
|---------------|-------------|----------:|-----------------|
| `analytics_rollups` | `316787476195:twitchtracker` | 131 | `archive/rollups/stream_id=316787476195/part-000.jsonl.gz` |
| `analytics_stream` | `316070541810` | 240 | `archive/streams/stream_id=316070541810/session.json.gz` |
| `bronze_vod_catalog` | `gorizontradio:2026-06-25` | 23 | `archive/channels/vod_index/gorizontradio.jsonl.gz` |

Regenerate:

```bash
BEARHOST_SAMPLE_MANIFEST_REMOTE=1 bash scripts/storage/archive-exports-sample-manifest.sh --csv \
  > docs/storage/sample-manifest-phase2a.csv
```

### 2A.4 One-object dry-run candidate

| Field | Value |
|-------|-------|
| **Selected** | Smallest canonical `analytics_rollups` / `rollups/` object |
| `natural_key` | `316787476195:twitchtracker` |
| `byte_size` | **131** |
| Azure blob | `streamclone/rollups/stream_id=316787476195/part-000.jsonl.gz` |
| R2 key | `archive/rollups/stream_id=316787476195/part-000.jsonl.gz` |
| `gcs_uri` | `https://ststreamclone3lf6tt.blob.core.windows.net/streamclone-archive/streamclone/rollups/stream_id=316787476195/part-000.jsonl.gz` |

### 2A.5 One-object dry run — **EXECUTED** (2026-06-25)

| Step | Result |
|------|--------|
| Azure metadata | 131 B, etag `"0x8DED095FEE6D766"` |
| Download + SHA-256 | `8c0fd0d6a814325beb752a0a5caa20905cedf5f8078c20df9d2e2c83b1519056` |
| R2 upload | `streampulse-artifacts-staging/archive/rollups/stream_id=316787476195/part-000.jsonl.gz` |
| R2 round-trip | Same SHA-256 |
| gzip -t | ok |
| Azure etag after | **unchanged** |
| `archive_exports` / read-path | **not modified** |

Script: `scripts/storage/r2-one-object-dry-run.sh` (Wrangler OAuth on Windows; optional S3 keys in `~/.streamclone/r2-staging-s3.env`).

### 2A.6 Phase 2A closeout (2026-06-25 — dry run executed)

| Item | Result |
|------|--------|
| R2 enabled (API) | **Yes** — personal account after subscription |
| `streampulse-artifacts-staging` | **Created** |
| One-object dry run | **Success** (Wrangler OAuth on Windows; WSL used for Azure download) |
| Object | `archive/rollups/stream_id=316787476195/part-000.jsonl.gz` (131 B) |
| SHA-256 | `8c0fd0d6a814325beb752a0a5caa20905cedf5f8078c20df9d2e2c83b1519056` (Azure = R2 round-trip) |
| gzip test | **ok** |
| Azure etag | **unchanged** `"0x8DED095FEE6D766"` |
| `archive_exports` / read-path | **Not modified** |

**Env file (local, not committed):** `~/.streamclone/r2-staging-s3.env` — endpoint + account ID. S3 access keys optional (Wrangler OAuth works for CLI; BearHost will need S3 keys later).

**Wrangler notes:**

- R2 bucket CLI: run from **streamclone repo root** (`CLOUDFLARE_ACCOUNT_ID=51dd8007…`) — not `streampulse-web/` (ASU `wrangler.toml`).
- WSL non-interactive Wrangler needs `CLOUDFLARE_API_TOKEN`; Windows OAuth session works for `wrangler r2 object put/get`.

---

## Phase 2B — sample batch mirror (STOR-R2-002)

**Status:** **Complete** 2026-06-25. **31/31** objects verified.

### 2B.1 Scope

| Included | Count | R2 prefix |
|----------|------:|-----------|
| `analytics_rollups` | 20 | `archive/rollups/` |
| `analytics_stream` | 10 | `archive/streams/` |
| `bronze_vod_catalog` | 1 | `archive/channels/vod_index/` |
| **Total** | **31** | **~5,232 bytes** |

**Skipped (explicit):** `postgres/nightly/`, `vod_chat/`, `tt-detail/`, `emote_snapshot` (none in manifest).

### 2B.2 Script and log

```bash
EXECUTE=1 CONCURRENCY=3 bash scripts/storage/r2-sample-mirror-phase2b.sh
```

**Mirror log:** [`sample-mirror-phase2b.csv`](./sample-mirror-phase2b.csv).

### 2B.3 Verification summary

| Check | Result |
|-------|--------|
| Objects copied | **31** |
| SHA-256 match (Azure download = R2 round-trip) | **31/31** |
| Byte size match | **31/31** |
| gzip -t (all `.gz`) | **31/31** |
| Azure etag/size unchanged after mirror | **31/31** |
| Failed objects | **0** |
| `archive_exports` / read-path | **not modified** |

One manifest row had stale CSV `byte_size`; script validates against **live Azure metadata**.

### 2B.4 Mutations confirmation

Staging R2 uploads only. No Azure delete/lifecycle change. No prod buckets. No Go read-path changes.

---

**Phase A — mirror existing Azure layout**

```text
archive/rollups/stream_id={id}/part-000.jsonl.gz
archive/vod_chat/stream_id={id}/messages.jsonl.gz
...
```

**Phase B — StreamPulse-normalized layout (new artifacts only)**

```text
vod-chat-archives/{login}/{vod_id}/chat.jsonl.zst
vod-rollups/{login}/{stream_id}/minute-rollups.json.zst
vod-emotes/{login}/{stream_id}/7tv-lanes.json.zst
vod-recaps/{login}/{stream_id}/recap.json
vod-thumbnails/{login}/{vod_id}/poster.webp
```

### 1.4 Postgres pointer / manifest design (future — not applied)

Extend `archive_exports` or add `artifact_objects` — see Phase 0 audit notes. Postgres remains SoT; R2 holds bytes only.

### 1.5 Sample manifest query — PREPARED, NOT EXECUTED

Run on **BearHost Postgres** (read-only). Prioritizes small confirmed exports for Phase 2 sample; excludes large `vod_chat/` and `postgres/nightly/` unless operator approves.

```sql
-- Phase 2 sample manifest (10–50 rows) — operator run on BearHost
WITH ranked AS (
  SELECT
    artifact_type,
    natural_key,
    gcs_uri,
    byte_size,
    etag,
    exported_at,
    ROW_NUMBER() OVER (
      PARTITION BY artifact_type
      ORDER BY byte_size ASC NULLS LAST, exported_at DESC
    ) AS rn
  FROM archive_exports
  WHERE export_status = 'confirmed'
    AND artifact_type IN (
      'analytics_rollups',
      'analytics_stream',
      'emote_snapshot',
      'emote_snapshot_global',
      'bronze_vod_catalog'
    )
)
SELECT artifact_type, natural_key, gcs_uri, byte_size, etag, exported_at
FROM ranked
WHERE rn <= 10
UNION ALL
SELECT artifact_type, natural_key, gcs_uri, byte_size, etag, exported_at
FROM archive_exports
WHERE export_status = 'confirmed'
  AND artifact_type = 'emote_snapshot_global'
ORDER BY artifact_type, byte_size ASC NULLS LAST
LIMIT 50;
```

**Postgres query status:** not run from this workstation (BearHost psql / MCP not available in agent session). Operator should run and attach CSV to Phase 2 ticket.

---

## Phase 2 — Staging mirror plan (commands generated, NOT executed)

### 2.1 Sample selection criteria

See §1.5 SQL. Target mix:

- 10–20× `analytics_rollups`
- 5–10× `analytics_stream`
- 1× `emote_snapshot_global` or smallest channel emote snapshot
- 1× `bronze_vod_catalog` (matches live 3.7 KiB object)
- **Exclude** from first batch: `vod_chat_message`, `postgres/nightly/`, bulk `tt-detail/`

### 2.2 Copy command candidates — APPROVAL REQUIRED

**Single-object Azure download → R2 upload (after R2 enabled + S3 credentials):**

```bash
# Example — replace KEY from archive_exports sample manifest
KEY="rollups/stream_id=315253283182/part-000.jsonl.gz"
AZ_CONTAINER=streamclone-archive
AZ_PREFIX=streamclone
R2_BUCKET=streampulse-artifacts-staging
CLOUDFLARE_ACCOUNT_ID=51dd8007b22ac92482388d8b6cdbb6e3

az storage blob download \
  --container-name "$AZ_CONTAINER" \
  --name "${AZ_PREFIX}/${KEY}" \
  --file "/tmp/mirror-$(echo "$KEY" | tr '/' '_')" \
  --connection-string "$AZURE_STORAGE_CONNECTION_STRING"

aws s3 cp "/tmp/mirror-$(echo "$KEY" | tr '/' '_')" "s3://${R2_BUCKET}/archive/${KEY}" \
  --endpoint-url "https://${CLOUDFLARE_ACCOUNT_ID}.r2.cloudflarestorage.com"
```

**Batch mirror (Phase 2 after sample manifest CSV exists):**

```bash
# rclone (operator config — not committed):
# rclone copy azure:streamclone-archive/streamclone/rollups/ \
#   r2:streampulse-artifacts-staging/archive/rollups/ \
#   --include "stream_id=315253*/**" --dry-run
```

### 2.3 Verification checklist (per object)

- [ ] Object exists in R2 at expected key
- [ ] Byte size matches Azure
- [ ] SHA-256 matches
- [ ] `archive_exports` row updated (future schema)
- [ ] Gzip decompress succeeds
- [ ] Azure original unchanged

---

## Phase 3 — App integration (STOR-R2-003)

**Status:** **Code complete** 2026-06-25. **Production behavior unchanged** — all flags default to Azure-only.

### 3.1 Implementation

| Component | Path |
|-----------|------|
| `R2BlobStore` | `internal/archive/r2_store.go` (S3-compatible via minio-go) |
| `ReadThroughStore` | `internal/archive/read_through_store.go` |
| Factory | `internal/archive/store_factory.go` → `NewBlobStore(StoreConfig)` |
| Config mapping | `internal/config/archive_store.go` → `Config.ArchiveBlobStoreConfig()` |
| Wired in | `cmd/archive`, `cmd/analytics`, `cmd/backfill` |

### 3.2 Env flags (defaults preserve production)

| Variable | Default | Effect |
|----------|---------|--------|
| `ARCHIVE_PRIMARY_PROVIDER` | `azure` | `r2` uses R2 for Put/Get/BlobURI (staging only) |
| `ARCHIVE_READ_THROUGH` | `false` | `true`: Get tries R2 first, Azure fallback on miss |
| `ARCHIVE_DUAL_WRITE` | `false` | `true`: Put writes Azure then R2 (non-prod only) |
| `ARCHIVE_R2_BUCKET` | — | e.g. `streampulse-artifacts-staging` |
| `ARCHIVE_R2_ACCOUNT_ID` | — | Cloudflare account ID |
| `ARCHIVE_R2_PREFIX` | `archive` | R2 key prefix (matches staging mirror) |
| `ARCHIVE_R2_ENDPOINT` | derived | `https://{account}.r2.cloudflarestorage.com` |
| `ARCHIVE_R2_ACCESS_KEY_ID_FILE` | — | Secret file (never commit) |
| `ARCHIVE_R2_SECRET_ACCESS_KEY_FILE` | — | Secret file (never commit) |

When all R2 flags are off and `ARCHIVE_PRIMARY_PROVIDER=azure`, `NewBlobStore` returns plain `AzureBlobStore` (no wrapper).

### 3.3 Behavior summary

| Operation | Default (prod) | `ARCHIVE_READ_THROUGH=true` |
|-----------|----------------|----------------------------|
| **Get** | Azure only | R2 → Azure fallback |
| **Put** | Azure only | Azure only (unless `DUAL_WRITE` or `PRIMARY=r2`) |
| **BlobURI** | Azure HTTPS | Azure HTTPS (unless `PRIMARY=r2`) |

### 3.4 Tests

```bash
go test ./internal/archive/...
# STOR-R2-004 restore drill (local secrets, read-only):
bash scripts/storage/r2-restore-drill.sh
# Or manual:
ARCHIVE_R2_LIVE_TEST=1 go test ./internal/archive/ -run TestR2RestoreDrillLive -count=1 -v
```

**No production read-path enablement** until operator explicitly sets `ARCHIVE_READ_THROUGH=true` on BearHost after reviewing [r2-restore-drill-log.md](./r2-restore-drill-log.md).

---

## Phase 3.5 — R2 restore drill (**STOR-R2-004**, done 2026-06-25)

Read-only validation against **`streampulse-artifacts-staging`** and the **31** mirrored keys in [`sample-mirror-phase2b.csv`](./sample-mirror-phase2b.csv).

| Check | Result |
|-------|--------|
| Direct `R2BlobStore.Get` | **PASS** — rollups, stream session, vod index samples |
| `ReadThroughStore` R2 hit | **PASS** — same bytes/SHA-256 as direct R2 |
| Azure fallback on R2 miss | **PASS** — e.g. `rollups/stream_id=317014684259/part-000.jsonl.gz` (Azure-only) |
| Gzip decompress | **PASS** |
| Payload shape | **PASS** — stream session JSON; rollups/vod stubs accept valid gzip JSON |

**Operator replay (local secrets only):**

```bash
bash scripts/storage/r2-restore-drill.sh
```

**Go test:** `internal/archive/r2_restore_drill_test.go` (`TestR2RestoreDrillLive`, gated on `ARCHIVE_R2_LIVE_TEST=1`).

**Log:** [r2-restore-drill-log.md](./r2-restore-drill-log.md).

**Production:** BearHost still `ARCHIVE_READ_THROUGH=false`. Azure authoritative. No `archive_exports` updates during drill.

---

## Phase 4 — Migration execution plan (future)

| Order | Prefix | Priority |
|------:|--------|----------|
| 1 | small `analytics_stream`, `rollups/` | validate pipeline |
| 2 | `emotes/snapshots/`, `vod_catalog/`, `channels/` | low risk |
| 3 | `viewer_rollup/`, `streams/` | medium |
| 4 | `vod_chat/` | high egress |
| 5 | `postgres/nightly/` → `streampulse-backups-prod` | high — restore test required |
| 6 | `tt-detail/` | optional |

Azure fallback minimum **90 days** after R2 read verified per batch.

---

## Cost and safety guardrails

| Guardrail | Detail |
|-----------|--------|
| Budget alerts | Required on Cloudflare before R2 enable (**done** on personal account); Azure budget already in Terraform |
| Staging mutations only | Phase 2A/2B staging sample mirrors allowed; **no** prod cutover, Azure delete/lifecycle change, or prefix batch without **STOR-R2-005** approval |
| No DNS / Pages / tunnel / D1 changes | Out of scope |
| Secrets | Never commit connection strings or R2 keys |

---

## Rollback plan

Set `ARCHIVE_PRIMARY_PROVIDER=azure` / `ARCHIVE_READ_THROUGH=false`. Azure untouched during Phase 0–2.

---

## Phase 0.6 closeout report (2026-06-25)

### Commands run

| Command | Result |
|---------|--------|
| `git status` | `docs/storage/` did not exist; audit doc recreated with inventory |
| `bash scripts/storage/azure-prefix-inventory.sh` | OK — per-prefix counts for 7 required prefixes |
| `bash scripts/storage/azure-top-prefixes.sh` | OK — 11 top-level virtual folders discovered |
| `bash scripts/storage/azure-extra-prefixes.sh` | OK — `directory/`, `viewer_rollup/` counts |
| `npx wrangler r2 bucket list` (personal `51dd8007…`) | **R2 disabled** (10042) |
| `npx wrangler r2 bucket list` (ASU `513d8937…`) | **R2 disabled** (10042) |

### Inventory results

- **~9,197 objects**, **~96.9 MiB** under `streamclone/` (domain sum).
- **71% of bytes** in `postgres/nightly/` (3 backups).
- All blobs **Hot** tier (container age < 90d).
- Additional domains: `streams/`, `viewer_rollup/`, `directory/`, `rosters/`.

### Doc changes

- Created `docs/storage/azure-to-r2-migration.md` with Phase 0 repo audit, **verified §0.6 inventory table**, operator summary, Phase 1 prep (bucket commands + sample SQL), closeout report.

### R2 dashboard status

- Personal account `51dd8007b22ac92482388d8b6cdbb6e3`: **R2 not enabled**
- ASU account `513d89373bebfc78b66e243b80f4debf`: **R2 not enabled**

### Bucket status

- `streampulse-artifacts-prod` — **not created**
- `streampulse-backups-prod` — **not created**
- `streampulse-artifacts-staging` — **not created**

### Exact next copy command candidates (after operator enables R2 + budget alerts)

1. Enable R2 in Cloudflare dashboard (personal account).
2. `npx wrangler r2 bucket create streampulse-artifacts-staging`
3. Create R2 S3 API token → store in BearHost secrets (not git).
4. Run §1.5 SQL on BearHost; export CSV.
5. Single-object dry run: §2.2 `az storage blob download` + `aws s3 cp` for one `analytics_rollups` key (~2 KiB).

### Remaining risks

| Risk | Mitigation |
|------|------------|
| R2 not enabled | Dashboard enable + budget alerts before any copy |
| `archive_exports` sample not queried | Operator psql on BearHost |
| Root listing >5k objects | Use per-prefix inventory scripts for migration batches |
| `postgres/nightly/` restore untested on R2 | Defer backup bucket until restore drill |
| Azure egress on bulk `vod_chat/` | Batch after sample pass; monitor Azure cost |

### Mutations confirmation

**No Azure blob downloads, copies, deletes, or lifecycle changes were performed.**
**No R2 buckets created. No R2 object uploads.**
**No Postgres schema changes. No production read path changes.**

---

## Phase 2A docs alignment closeout (2026-06-25)

### Current storage truth

| Item | State |
|------|-------|
| Production archive reads/writes | **Azure Blob** (`AzureBlobStore`) |
| R2 personal account | **Enabled** |
| Staging bucket | **`streampulse-artifacts-staging`** |
| Verified staging objects | **1** (131 B `analytics_rollups`) |
| Prod / backups R2 buckets | **Not created** |
| Production read-path cutover | **No** |
| BearHost Postgres SoT | `archive_exports`, jobs, queues, VOD Library rows (future) |

### Task IDs added

| ID | Summary |
|----|---------|
| STOR-R2-001 | Done — inventory + one-object proof |
| STOR-R2-002 | Mirror 10–50 sample `archive_exports` objects |
| STOR-R2-003 | `R2BlobStore` + `ReadThroughStore` behind flags |
| STOR-R2-004 | R2 restore drill |
| STOR-R2-005 | Batch migration by prefix |

Ledger: [tasks.md](./tasks.md).

### Mutations confirmation (this alignment pass)

**Docs/task ledger updates only.** No new R2 uploads, no prod buckets, no Azure lifecycle/delete, no production read-path flags, no DNS/Pages/D1/Workers/VOD Library code changes.

---

## Document history

| Date | Change |
|------|--------|
| 2026-06-25 | Initial read-only audit + staging plan |
| 2026-06-25 | Phase 0.6 live inventory + Phase 1 prep + closeout report |
| 2026-06-25 | Handoff: `docs/storage/README.md`, stable `scripts/storage/*` inventory scripts |
| 2026-06-25 | Phase 2A continuation recheck: API still `10042`; staging bucket not created |
| 2026-06-25 | Phase 2B complete: 31-object sample mirror verified (`STOR-R2-002`) |
| 2026-06-25 | Phase 3.5: STOR-R2-004 restore drill — read-through + Azure fallback validated on staging sample |
