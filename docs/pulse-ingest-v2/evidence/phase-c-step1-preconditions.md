# Step 1 preconditions evidence — 2026-07-08

Run: `bash scripts/ingest-phase-c-preconditions.sh` (public API + optional local docker).

## Results (public API probe)

| Check | Result |
|-------|--------|
| Moments Cache-Control | **FAIL** — HTTP 200 with valid `bucketT` but no `Cache-Control` header on prod (`v0.3.0-rc19`) |
| Hub HTTP 200 | **PASS** |
| INGEST_CORE_ENABLED=0 | **SKIP locally** — verify on VPS analytics container |
| CORPUS/SCRAPER caps | **SKIP locally** — verify streampulse-ops overlay |
| Docker limits staged | **PENDING** — operator applies `hosted-resource-limits.compose.yml` one service at a time with 15–30 min soak |

## Blockers before Phase C shadow env flip

1. Deploy release containing `hub_historical_moments.go` Cache-Control headers to production.
2. Complete VPS Step 0 baseline (`scripts/ingest-phase-c-step0-baseline.sh`).
3. Apply Docker limits separately; do not combine with shadow restart.

## Corpus/scraper (code guards)

- `CORPUS_WORKERS_ENABLED=0` default in `internal/config/config.go`
- `SCRAPER_ENABLED_ON_API_NODE=0` guarded in `cmd/analytics/main.go`
