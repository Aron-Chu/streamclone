# Azure Blob → Cloudflare R2 migration plan (read-only audit)

| | |
|---|---|
| **Status** | Phase 0.6 inventory complete + Phase 1 prep — **no cutover, no copies executed** |
| **Owner** | Aron-Chu |
| **Date** | 2026-06-25 |
| **Product** | StreamPulse long-term analytics artifacts |
| **Source of truth (unchanged)** | BearHost Postgres — app rows, `archive_exports`, queues, VOD Library (future), saved moments |

---

## Executive summary

Streamclone/StreamPulse cold archive is **implemented today against Azure Blob only**. The Go `archive` package writes through a `BlobStore` interface (`internal/archive/writer.go`); only `AzureBlobStore` exists. Manifest rows live in Postgres `archive_exports` (`gcs_uri` column name is legacy — values are Azure HTTPS URIs).

**R2 is not enabled** on either Cloudflare account checked (personal `51dd8007…`, ASU `513d8937…`) — API returns `code 10042`. **No R2 buckets exist.** Azure remains authoritative.

**Recommended path:**

1. **Phase 0** — operator inventory on Azure (counts/sizes) using existing scripts + `az storage blob list` (read-only). **Done 2026-06-25.**
2. **Phase 1** — enable R2 on chosen account + budget alerts; create staging/prod buckets (prepared, not executed).
3. **Phase 2** — mirror **10–50 sample objects** Azure → R2; verify checksums; Azure untouched.
4. **Phase 3** — read-through in BearHost API: R2 first → Azure fallback → metadata-only.
5. **Phase 4** — batch migration by prefix; keep Azure fallback window; no Azure deletes until verified.

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
| `scripts/tmp/batch-b-azure-list.sh` | Read-only `az storage blob list` samples |
| `scripts/tmp/azure-prefix-inventory.sh` | Phase 0.6 per-prefix aggregate (metadata only) |
| `scripts/tmp/batch-b2-azure-list-nightly.sh` | Nightly backup prefix list |
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
bash scripts/tmp/batch-b-azure-list.sh
bash scripts/tmp/azure-prefix-inventory.sh
bash scripts/tmp/azure-top-prefixes.sh
bash scripts/tmp/azure-extra-prefixes.sh
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
| **Personal** (`aron.chu90@gmail.com`) | `51dd8007b22ac92482388d8b6cdbb6e3` | DNS zone `streampulse.stream`, prod Pages, `api` tunnel DNS | **Disabled** — `wrangler r2 bucket list` → `code 10042` |
| **ASU** (`amchu2@asu.edu`) | `513d89373bebfc78b66e243b80f4debf` | Staging Pages, edge Worker, D1, KV | **Disabled** — `code 10042` |

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

**Bucket status:** none created (R2 not enabled).

### 1.3 R2 key layout

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

## Phase 3 — App integration plan (future)

`BlobStore` + `R2BlobStore` + `ReadThroughStore` — no code changes in this phase. See prior audit sections.

**No production read path change** until Phase 2 verification passes.

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
| Budget alerts | Required on Cloudflare **before** R2 enable; Azure budget already in Terraform |
| No mutations this phase | No Azure download/copy/delete/lifecycle change; no R2 uploads |
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
| `bash scripts/tmp/batch-b-azure-list.sh` | OK — samples under `postgres/nightly/` (3) and `rollups/` (5 shown, paginated) |
| `bash scripts/tmp/azure-prefix-inventory.sh` | OK — per-prefix counts for 7 required prefixes |
| `bash scripts/tmp/azure-top-prefixes.sh` | OK — 11 top-level virtual folders discovered |
| `bash scripts/tmp/azure-extra-prefixes.sh` | OK — `directory/`, `viewer_rollup/` counts |
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

## Document history

| Date | Change |
|------|--------|
| 2026-06-25 | Initial read-only audit + staging plan |
| 2026-06-25 | Phase 0.6 live inventory + Phase 1 prep + closeout report |
