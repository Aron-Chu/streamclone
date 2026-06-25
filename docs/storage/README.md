# Storage — operator index

StreamPulse long-term **artifact bytes** vs **queryable app state**. Read this before Azure/R2 migration or VOD Library storage work.

## Source of truth (unchanged)

| Data | Home | Notes |
|------|------|-------|
| App rows, queues, searchable indexes | **BearHost Postgres** | Includes `archive_exports`, saved moments, future VOD Library rows |
| Long-term immutable/semi-immutable blobs | **Azure Blob today** | Container `streamclone-archive`, prefix `streamclone/` |
| User library rows | **Postgres** | Never R2, never D1 for VOD Library MVP |
| Portal/extension user state (V2+) | Optional **D1** | Watchlist/device only — **deferred** (WATCH-100) |

## Current status (2026-06-25, Phase 2A — R2 live)

| Item | State |
|------|-------|
| **Authoritative archive** | **Azure Blob** — no cutover |
| **R2 (personal `51dd8007…`)** | **Enabled** — subscription active on aron.chu90 |
| **Staging bucket** | **`streampulse-artifacts-staging` created** |
| **Azure → R2 copies** | **One** staging object verified (131 B rollup); no bulk copy |
| **Sample manifest** | [`sample-manifest-phase2a.csv`](./sample-manifest-phase2a.csv) |
| **One-object dry run** | **Done** — 131 B rollup mirrored; SHA-256 match; Azure etag unchanged |
| **R2 config (local)** | `~/.streamclone/r2-staging-s3.env` (endpoint; S3 keys optional for CLI via Wrangler OAuth) |

## Hosting boundaries (no change)

| Surface | Account / host |
|---------|----------------|
| Production Pages | Personal Cloudflare → `https://streampulse.stream` |
| Staging Pages | ASU Cloudflare → `https://app.streampulse.stream` (when configured) |
| API | BearHost → `https://api.streampulse.stream` (tunnel; do not move) |
| Artifact API | **BearHost** analytics/archive — Workers are not the main blob API |

## VOD Library direction

- **MVP:** BearHost Postgres for library rows, metadata, queries, saved moments, progress, and pinned state.
- **Heavy artifacts (future):** Cloudflare R2 after mirror verification — chat exports, rollup bundles, thumbnails, recaps (referenced by Postgres rows, not stored as library state).
- **Not canonical:** D1 for VOD Library rows; Workers for primary artifact reads; R2 for user/saved-moment rows.
- **WATCH-100** (D1 watchlist on ASU edge): documented in streamclone-pulse [`cloudflare-asu-phase2-watch100.md`](https://github.com/Aron-Chu/streamclone-pulse/blob/master/docs/website-portal/cloudflare-asu-phase2-watch100.md) — **deferred** unless explicitly revived; unrelated to artifact migration.

## Docs

| Doc | Purpose |
|-----|---------|
| [azure-to-r2-migration.md](./azure-to-r2-migration.md) | Full Phase 0–4 audit, Phase 2A status, operator commands |
| [tasks.md](./tasks.md) | Task ledger `STOR-R2-001`–`005` |
| [sample-manifest-phase2a.csv](./sample-manifest-phase2a.csv) | Phase 2A BearHost sample manifest (31 data rows) |
| [../azure-archive-setup.md](../azure-archive-setup.md) | Azure archive operator setup |
| [../scraping-archive/requirements.md](../scraping-archive/requirements.md) | Scrape tiers + cold storage requirements |

## Read-only inventory scripts

Requires Azure CLI and connection string file (`~/.streamclone/azure-archive-connection-string` or `AZURE_CONN_FILE`). **Never commit secrets.**

```bash
bash scripts/storage/azure-prefix-inventory.sh   # required migration prefixes
bash scripts/storage/azure-top-prefixes.sh       # top-level virtual folders
bash scripts/storage/azure-extra-prefixes.sh     # directory/, viewer_rollup/
BEARHOST_SAMPLE_MANIFEST_REMOTE=1 bash scripts/storage/archive-exports-sample-manifest.sh --csv
bash scripts/storage/r2-one-object-dry-run.sh    # dry-run steps (prep only unless EXECUTE=1)
```

## Next operator actions

1. **STOR-R2-002** — mirror 10–50 sample objects from `sample-manifest-phase2a.csv` to staging (after approval).
2. **STOR-R2-003** — implement `R2BlobStore` + `ReadThroughStore` behind flags (BearHost; default off).
3. **STOR-R2-004** — R2 restore drill before any `postgres/nightly/` mirror.
4. **STOR-R2-005** — batch migration by prefix after verification.
5. Prod/backups buckets — not until separately approved.

See [tasks.md](./tasks.md) for guardrails.

**R2 CLI (personal account):**

```powershell
$env:CLOUDFLARE_ACCOUNT_ID = '51dd8007b22ac92482388d8b6cdbb6e3'
cd C:\Users\Aron\twitch-7tv-clone   # not streampulse-web — ASU account_id in wrangler.toml
npx wrangler r2 bucket list
```

## Out of scope until later phases

- Production `R2BlobStore` / `ReadThroughStore` cutover (**STOR-R2-003**)
- Prod/backups R2 buckets, bulk Azure copy, Azure delete/lifecycle change
- DNS, Pages deploy, API tunnel, D1, Workers, VOD Library implementation
- `archive_exports` schema migration for R2 URIs
