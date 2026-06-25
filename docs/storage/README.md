# Storage — operator index

StreamPulse long-term **artifact bytes** vs **queryable app state**. Read this before Azure/R2 migration or VOD Library storage work.

## Source of truth (unchanged)

| Data | Home | Notes |
|------|------|-------|
| App rows, queues, searchable indexes | **BearHost Postgres** | Includes `archive_exports`, saved moments, future VOD Library rows |
| Long-term immutable/semi-immutable blobs | **Azure Blob today** | Container `streamclone-archive`, prefix `streamclone/` |
| User library rows | **Postgres** | Never R2, never D1 for VOD Library MVP |
| Portal/extension user state (V2+) | Optional **D1** | Watchlist/device only — **deferred** (WATCH-100) |

## Current status (2026-06-25)

| Item | State |
|------|-------|
| **Authoritative archive** | **Azure Blob** — no cutover |
| **R2** | **Disabled** on personal (`51dd8007…`) and ASU (`513d8937…`) accounts — API `code 10042` |
| **R2 buckets** | None created |
| **Azure → R2 copies** | None executed |
| **Inventory** | ~9,197 objects, ~96.9 MiB (Phase 0.6 verified) |
| **Largest byte class** | `postgres/nightly/` (~65.6 MiB) — do not mirror until R2 restore tested |
| **First sample batch** | Tiny: `rollups/`, `streams/`, one emote snapshot, one `vod_catalog/` object |
| **Defer** | Bulk `vod_chat/`, `tt-detail/`, `postgres/nightly/` |

## Hosting boundaries (no change)

| Surface | Account / host |
|---------|----------------|
| Production Pages | Personal Cloudflare → `https://streampulse.stream` |
| Staging Pages | ASU Cloudflare → `https://app.streampulse.stream` (when configured) |
| API | BearHost → `https://api.streampulse.stream` (tunnel; do not move) |
| Artifact API | **BearHost** analytics/archive — Workers are not the main blob API |

## VOD Library direction

- **MVP:** BearHost Postgres for library rows, metadata, and queries.
- **Heavy artifacts (future):** Cloudflare R2 after mirror verification — chat exports, rollups bundles, thumbnails, recaps.
- **Not canonical:** D1 for VOD Library rows; Workers for primary artifact reads.
- **WATCH-100** (D1 watchlist on ASU edge): documented in streamclone-pulse [`cloudflare-asu-phase2-watch100.md`](https://github.com/Aron-Chu/streamclone-pulse/blob/master/docs/website-portal/cloudflare-asu-phase2-watch100.md) — **deferred** unless explicitly revived; unrelated to artifact migration.

## Docs

| Doc | Purpose |
|-----|---------|
| [azure-to-r2-migration.md](./azure-to-r2-migration.md) | Full Phase 0–4 audit, inventory table, R2 prep, sample SQL |
| [../azure-archive-setup.md](../azure-archive-setup.md) | Azure archive operator setup |
| [../scraping-archive/requirements.md](../scraping-archive/requirements.md) | Scrape tiers + cold storage requirements |

## Read-only inventory scripts

Requires Azure CLI and connection string file (`~/.streamclone/azure-archive-connection-string` or `AZURE_CONN_FILE`). **Never commit secrets.**

```bash
bash scripts/storage/azure-prefix-inventory.sh   # required migration prefixes
bash scripts/storage/azure-top-prefixes.sh       # top-level virtual folders
bash scripts/storage/azure-extra-prefixes.sh     # directory/, viewer_rollup/
```

## Next operator actions

1. **Enable R2** in Cloudflare dashboard (personal account recommended) and set **billing budget alerts**.
2. Verify: `CLOUDFLARE_ACCOUNT_ID=51dd8007b22ac92482388d8b6cdbb6e3 npx wrangler r2 bucket list`
3. Run sample manifest SQL on BearHost Postgres (see migration doc §1.5).
4. After approval only: create staging bucket + mirror **10–50** small confirmed objects (no `postgres/nightly/` or bulk `vod_chat/`).

## Out of scope until later phases

- `R2BlobStore`, `ReadThroughStore`, `archive_exports` schema migration
- R2 bucket creation, Azure copy/download/delete
- DNS, Pages deploy, API tunnel, production read-path flags
- VOD Library routes/UI implementation
