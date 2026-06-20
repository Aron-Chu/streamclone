# Analytics Steering

Analytics turns Twitch stream history, viewer data, chat rollups, emotes, VOD context, and clip jobs into local insight views.

## Boundaries

- Go service: `cmd/analytics`, `internal/analytics`.
- Metadata helpers: `internal/metadata/api`, `internal/metadata/helix`.
- Frontend: `frontend/src/components/Analytics.tsx` and related utilities.
- Optional scraper: TwitchTracker minute charts. Core Watch must still work without it.

## Current Rules

- Prefer synced local rollups over live-only guesses.
- Preserve honest empty states: missing scraper, no VOD, no chat coverage, auth issue, or upstream block should say what happened.
- Keep TwitchTracker scraping direct through Camoufox unless scraper routing proves otherwise.
- Reddit LSF is best-effort and must degrade cleanly.
- Shared IRC ingest is a future simplification; avoid adding more independent IRC clients.
- For parallel VOD GQL, keep `ANALYTICS_VOD_GQL_DEFER=true` unless you are explicitly debugging summary refreshes; the normal path is segment writes first, `finalizing` refresh last.

## Pulse (Grafana — not Pulse Wire)

**Pulse** here means optional Grafana/Influx over local Analytics rollups (emote/chat metrics). In-app Analytics remains canonical.

**Not Pulse Wire:** the streamer news feed at `/pulse-wire` is a separate optional tier (`storygraph`, `PULSE_WIRE_ENABLED`). For Pulse Wire work, read `.kiro/steering/pulse-wire.md` — not this section. Both tiers may share `streamclone-scraper` (Reddit LSF, YouTube, TwitchTracker); scraper health affects Analytics charts and Pulse Wire ingest independently of chat sync.

**Helm (recommended for Pulse dev):** `make pulse` deploys [`charts/pulse/`](../charts/pulse/) to Kubernetes (InfluxDB, Grafana, Prometheus). On WSL + Docker Desktop, Influx uses ClusterIP plus port-forward on `:18087` so Compose analytics can reach Influx via the WSL IP (`INFLUXDB_URL=http://<wsl-ip>:18087`). Wire with `make helm-pulse-wire`.

**Compose `pulse` profile:** same dashboards from `deploy/grafana/` plus Prometheus in [`deploy/docker-compose.yml`](../deploy/docker-compose.yml) — used by desktop **Start Pulse** when Helm/Grafana is not already on `:3000`/`:18086`.

Dashboards: **Emote Pulse** (Influx/Flux) and **Streamclone Ops** (Prometheus/PromQL). Influx export includes `stream_category` tag and `unique_emote_count` field. Re-sync or `TIMESERIES_BACKFILL_ON_START=true` after upgrade for historical tags.

**Emote thumbnails (Pulse / LiveStatsBand):** `TopEmotesFromRollups` and heatmap window emotes attach `imageUrl` via `internal/emoteimage`. Rollup keys are `provider:id:name` from chat tokenization — Twitch ids are **not** MinIO paths. Never build `/emotes/{twitchId}/1x.webp` for analytics responses; use `emoteimage.URL` (same rules as chat native URLs for Twitch, local proxy for synced sets).

For release installs, Pulse readiness is gated through Stack status: analytics should backfill existing local minute rollups into Influx when timeseries is enabled, writes must be idempotent, and the ready state should wait for Grafana, Influx, Prometheus, analytics timeseries, and backfill completion.

## GQL chat tail tuning (2026-06)

Parallel VOD GQL uses adaptive hot splits (`hotSegmentSplitReason`: `page_threshold`, `slow_advance`, `comments_per_page`), segment auto-close at boundaries (`tryAutoCloseSegment`), deferred `refreshStreamSummary` when `ANALYTICS_VOD_GQL_DEFER=true`, and coalesced rollup flush (`ANALYTICS_VOD_GQL_ROLLUP_FLUSH_SEGMENTS`, `ANALYTICS_VOD_GQL_ROLLUP_FLUSH_MS`). Sync progress exposes `hotSegmentSplitReason`, `autoClosedSegments`, `summaryRefreshDeferred`, and `indexPhase=finalizing` for the UI tail phase.

**Do not raise `ANALYTICS_VOD_GQL_CONCURRENCY` / worker caps without checking Streamclone Ops Grafana panels first:** `analytics_vod_gql_throttle_total`, `analytics_vod_gql_backoff_seconds_total`, `analytics_vod_gql_worker_pages_total`, `analytics_vod_gql_hot_splits_total`, and `analytics_rollup_rows_written_total`. Rising throttle/backoff with flat worker pages means upstream is saturated — lower concurrency or widen segment windows instead of adding workers.

Detail: `internal/metrics/metrics.go`, `deploy/grafana/dashboards/streamclone-ops.json`, and GQL segment diagnostics in sync progress (`sync_status.go`, `sync_gql_parallel.go`).

## Codegraph Hints

- `get_ast_chunk("URL")` — `internal/emoteimage` provider/CDN resolver
- `get_blast_radius("TopEmotesFromRollups")`
- `get_call_chain("TopEmotesFromRollups", depth=2)`
- `get_blast_radius("mergeMinuteRollups")`
- `get_ast_chunk("gqlCommentText")`
- `get_ast_chunk("hasGoodChatCoverageFromRollups")`
- `get_ast_chunk("SyncProgressPanel")`

## Checks

```sh
go test ./internal/analytics/...
go test ./internal/archive/...
go test ./internal/emoteimage/...
cd frontend && npm run build
make scraper-check
```

For scraper or public/tunnel changes, also run `make security-scan`.

## Archive / Bronze (2026-06)

Azure cold storage is optional (`ARCHIVE_ENABLED=true`, `ARCHIVE_STORAGE_PROVIDER=azure`). Key env vars:

| Env | Default | Purpose |
|-----|---------|---------|
| `ARCHIVE_AZURE_CONNECTION_STRING_FILE` | — | Local file with Azure connection string (never commit) |
| `ARCHIVE_EXPORT_ON_SYNC` | `false` | Export rollups after each stream sync completes |
| `ARCHIVE_EXPORT_INTERVAL` | `1h` | Incremental export ticker (`internal/archive/incremental_worker.go`) |
| `ARCHIVE_PROTECT_RETENTION` | `false` | Block purge until `archive_exports` row is confirmed |
| `TIER0_ENABLED` | `false` | Roster sync + Helix viewer sampler (top-N) |
| `BRONZE_ENABLED` | `false` | Helix VOD index + TT summary bulk index worker |
| `BRONZE_TOP_N` | `500` | Channel list size for Bronze roster |
| `BRONZE_WORKER_INTERVAL` | `5m` | Bronze indexer tick interval |
| `ANALYTICS_TT_SYNC_TIMEOUT_MS` | `45000` | TT scrape deadline for `SyncHistoricalStream` / silver backfill; set `120000` in `profile-archive.env` to match tracker scrape timeout |
| `BACKFILL_ENABLED` | `false` | Post-end silver gap-fill worker (`backfill_worker.go`) |
| `GOLD_BACKFILL_ENABLED` | `false` | Gold GQL chat enqueuer + gold tier worker branch |
| `GOLD_MIN_PEAK_VIEWERS` | `0` | Min peak viewers for gold rules (0 = disabled) |
| `GOLD_MIN_DURATION_MINUTES` | `0` | Min stream duration for gold rules (0 = disabled) |
| `GOLD_ENQUEUER_INTERVAL` | `5m` | Silver-done → gold candidate scan interval |
| `GOLD_SYNC_TIMEOUT_MS` | `600000` | GQL sync deadline for gold backfill jobs |

Operator CLI: `go run ./cmd/backfill bronze run-once`, `go run ./cmd/backfill gold enqueue|eval`, `go run ./cmd/archive restore --stream-id <id>`. Profile overlay: `deploy/env/profile-archive.env`. Setup: `docs/azure-archive-setup.md`.
