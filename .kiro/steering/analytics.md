# Analytics Steering

## Purpose

The analytics service collects per-minute viewer, chat, and 7TV emote rollups for tracked channels. The frontend Analytics page charts rollups, lists stream history, and can sync historical chat from Twitch VOD comments.

## Stack

- Service: `cmd/analytics`, package `internal/analytics`
- API: `/v1/analytics/channels/{login}/live`, `/streams`, `/streams/{streamId}`, sync POST
- Frontend: `frontend/src/components/Analytics.tsx`
- Historical enrichment: TwitchTracker stats + Twitch GQL VOD comment sync (`SyncService`)
- **Scraper**: sibling repo `streamclone-scraper`, Compose service `scraper:8000` (`SCRAPER_API_URL=http://scraper:8000/v2/scrape`). Default engine: **Camoufox** (`SCRAPER_BROWSER=camoufox`). Compose defaults use a **pooled browser** (`SCRAPER_EPHEMERAL_BROWSER=false`, `SCRAPER_MAX_CONCURRENT=2`); local Windows dev sets `SCRAPER_EPHEMERAL_BROWSER=true`, `SCRAPER_MAX_CONCURRENT=1`, `REDDIT_PROVIDER=off` in `.env` and runs `docker compose --env-file .env -f deploy/docker-compose.yml` so substitutions match (otherwise compose `environment:` overrides `env_file`). Profile volume `scraper-profile` at `/data/camoufox-profile`. Fallbacks: `SCRAPER_BROWSER=chromium` or `cdp` (host Chrome via `scripts/scraper-cdp.ps1`). Warm CF cookies once: `scripts/warm-camoufox-profile.ps1`. Analytics sync sends `useProxy:false` and tiered `maxAge` for Tracker URLs; set `PROXY_BYPASS=twitchtracker.com`. Windows: use `.\scripts\diagnose-scraper.ps1 -UseDocker` (avoids wslrelay on localhost:8000).
- **Stream list fallback**: metadata `insights` loads VOD rows from **Twitch Helix** first (fast). TwitchTracker HTML is optional enrichment for avg/peak viewers only. Minute-level viewer charts still need a successful TwitchTracker *detail* scrape (`meta#ecs`).
- **Emote images in analytics**: rollup keys store the local emote-service id (`/emotes/{uuid}/1x.webp`), not the 7TV provider id — do not point charts at `cdn.7tv.app/emote/{rollup-id}`.

### Scraper pitfalls (2026-06-09)

- Never treat Twitch stream IDs with prefix `3196` as mock — real 2026 IDs match that prefix.
- Mock HTML lacks `meta#ecs` → viewer chart is flat (peak-only). Real scrape must succeed for minute-level viewers.
- Set `SCRAPER_ALLOW_MOCK_FALLBACK=true` only for offline dev; default is real Playwright scrape + optional proxy/CDP.

## Local access

- Browser and API probes: **`http://localhost:8090/v1/analytics/...`** through Caddy `local-proxy`.
- On Windows, stale `wslrelay` on `127.0.0.1` can serve old containers — see `.kiro/steering/windows-dev.md`.

## Realtime boundary (shared with chat)

Analytics is a separate Compose service (`cmd/analytics`) but reuses chat packages for live ingest and VOD tokenization:

| Shared package | Used for |
|----------------|----------|
| `internal/chat/ircconn` | Own Twitch IRC WebSocket pool (independent of `cmd/chat`) |
| `internal/chat/parse` | IRCv3 PRIVMSG parsing into minute rollups |
| `internal/chat/enrich` | Redis dictionary Trie + `preloadChannelEmotes` invalidation |
| `internal/chat/batch` | Fragment schema for emote tokenization during VOD sync |

**Duplicate upstream connections:** when a channel is watched in the UI and tracked by analytics, Twitch sees **two IRC pools** — one in `cmd/chat`, one in `cmd/analytics`. Clipper adds a third via `clipper/liveclipper/irc.py` for spike detection. Prefer consolidating IRC ingest (merge chat+analytics realtime, or publish raw IRC lines on a Redis bus) before adding more IRC clients.

**Duplicate Helix clients:** `internal/analytics/helix.go` parallels `internal/metadata/helix` (used by metadata and emote). Clipper maintains `clipper/liveclipper/twitch.py`. Extract a shared `internal/twitch/helix` before adding more Helix surfaces.

**Go→Go HTTP hop:** VOD sync calls `POST {EMOTE_SERVICE_URL}/v1/channels/{login}/emotes/ensure` from `preloadChannelEmotes` in `sync.go`. This is the only direct inter-service HTTP call in the Go stack.

## Rollup merge rules

- Multiple DB rows can exist for the same minute (sync at `:00`, live collector at offset seconds).
- **`mergeMinuteRollups`** merges viewer, chat, and emote fields per minute bucket — do not keep only the highest-viewer row.
- **`consolidateRollupsByMinute`** dedupes via merge, not max-viewer replacement.
- After changing merge logic, run `go test ./internal/analytics/...`.

## Stream routing (frontend)

- Live: `/analytics/{login}` (no stream id).
- Historical: `/analytics/{login}/{streamId}` or date slug when unique per day.
- **Streams sidebar** (left) is the only stream picker — do not re-add a header dropdown.
- **Synced** badge: `viewerSamples > 0` or `chatMessages > 0`. **Stats only**: TwitchTracker averages without minute data — user clicks **Sync chat/emotes** on the chart.

## Sync behavior

- `POST /v1/analytics/streams/{streamId}/sync?channel={login}` pulls VOD chat into rollups.
- Streams without sync show zero chat/7TV on the chart until sync runs.
- Game segments: `GET /v1/analytics/streams/{streamId}/games`.
- Long VOD syncs can take several minutes (comment cap ~200k); do not assume failure from duration alone.
- **Chat-only resync**: when `viewerSamples > 0`, full sync skips TwitchTracker scrape and patches chat rollups only (`BulkPatchChatRollups`). Cached `vod_id` starts GQL fetch in parallel with any remaining tracker work.
- Env: `ANALYTICS_VOD_GQL_PAGE_DELAY_MS` (default `0`), `ANALYTICS_VOD_GQL_CONCURRENCY` (default `3`, max `6`), `ANALYTICS_VOD_GQL_SEGMENT_SECONDS` (default `600`), `ANALYTICS_TRACKER_SCRAPE_TIMEOUT_MS` (default `120000`), `ANALYTICS_PASS_TT_MAXAGE` (default `true`), `ANALYTICS_TT_MAX_AGE_MS` (default `0` = tiered by stream age), `ANALYTICS_TT_DIRECT_HTTP_ENABLED` (default `true`), `ANALYTICS_TT_DIRECT_HTTP_TIMEOUT_MS` (default `1200`).
- **Tracker soft-fail**: TwitchTracker scrape failure no longer aborts chat GQL sync; `SyncStatus.viewerStatus` is `ok`, `failed`, `skipped`, or `pending`.
- Keep `ANALYTICS_TRACKER_SCRAPE_TIMEOUT_MS` above the scraper's TwitchTracker wait budget (`wait_for_function` + navigation overhead); avoid setting below ~95s or viewer charts may truncate at the tail.
- **Early stream row**: first-time sync upserts a placeholder `analytics_streams` row immediately (Helix `VideoByStreamID` for `started_at` / `vod_id`) so `GET .../streams/{id}?sparse=true` returns 200 during sync instead of 404.
- **Syncing API fallback**: if the row is still missing but Redis sync status is active, `streamDetail` returns `state: "syncing"` with empty rollups.

### VOD resolution

1. **DB cache** (`analytics_streams.vod_id`) — populated on stream close (Helix) and after sync.
2. **Helix** `VideoIDByStreamID` in `internal/analytics/helix.go` — primary resolver; runs **in parallel** with TwitchTracker scrape when `broadcaster_id` is known.
3. **TwitchTracker HTML** (`extractVodID`) — fallback only when cache + Helix miss.
4. **`vod_source`** on `analytics_streams`: `db_cache`, `helix_stream_match`, `tracker_html`.
5. Duration: `VideoDurationSeconds` — rollup window uses `max(tracker duration, vod duration)`.

On **stream close**, the live collector calls Helix and `SetStreamVodID` with delayed retries (30s / 2m / 5m) when the VOD is not published immediately.

### Sync phase timing (debug)

`SyncHistoricalStream` logs structured `sync phase complete` events with millisecond durations:

| Field | Phase |
|-------|--------|
| `tracker_scrape_ms` | TwitchTracker scrape |
| `vod_resolve_ms` | VOD ID resolution |
| `gql_fetch_ms` | GQL comment paging |
| `tokenize_ms` | Emote tokenization for rollups |
| `rollup_write_ms` | Postgres rollup writes |

Optional fields on Redis `SyncStatus.timing` for the Analytics sync progress panel.

### GQL VOD comments (`fetchVODComments` / `sync_gql_parallel.go`)

- Persisted-query hash must match current Twitch GQL (update when sync returns empty comment bodies).
- **`postGQLVideoComments`**: checks HTTP status before JSON decode; honors `Retry-After`; exponential backoff + jitter on 429/503 (max 5 retries); logs throttle counts. Parallel workers share a global `gqlRateCoordinator` pause on 429/503.
- **Parallel paging** (when `ANALYTICS_VOD_GQL_CONCURRENCY` > 1): splits VOD duration into time segments (`ANALYTICS_VOD_GQL_SEGMENT_SECONDS`), fetches each with offset-only pagination and worker pool; merges with mutex + `seenCommentIDs` dedupe. On repeated integrity errors or worker failure, falls back to serial cursor/offset loop.
- **Checkpoints**: `analytics_sync_checkpoints` stores serial cursor/offset or parallel `segments_json` + `fetch_mode`; resume skips completed segments; cleared on success.
- **Dedupe**: skips duplicate GQL comment `id` when present.
- **Serial fallback pagination**: first page offset `0`; subsequent pages prefer cursor from the last edge. On integrity failures, falls back to offset (`contentOffsetSeconds + 1`). Keep-alive `gqlClient`; optional inter-page delay via `ANALYTICS_VOD_GQL_PAGE_DELAY_MS`.
- **Comment text**: `gqlCommentText()` joins `message.fragments[].text` when `message.body` is empty (common on current GQL payloads). Tests: `sync_gql_test.go`, `sync_gql_parallel_test.go`.
- **Typical speed**: ~8k comments ≈ 270 GQL pages; concurrency `3` often halves wall time vs serial (~90–150s → ~45–70s) when Twitch does not throttle heavily.
- **Long VOD tuning**: for 6h+ streams, set `ANALYTICS_VOD_GQL_CONCURRENCY=6` first, then monitor 429 frequency before lowering; this is the highest safe setting in the sync worker.

### 7TV emotes during sync

- `preloadChannelEmotes()` POSTs to emote service `ensure` before comment ingestion.
- `enricher.Invalidate(login)` after preload so sync sees fresh Redis dictionaries.
- Compose: `EMOTE_SERVICE_URL: http://emote:8080` on the analytics service.

## Frontend routing guards

- **`targetQueryStreamId`** in `Analytics.tsx`: numeric `streamId` passes through; date slugs (`YYYY-MM-DD`) resolve via `matchedStream` only — **never** call the API with the date string as `streamId` (caused 404s).
- While streams/insights load for a date slug, `targetQueryStreamId` is `undefined` (query disabled).
- Unresolved date slug shows a stream-not-found state (`dateSlugUnresolved`), not a bad API request.

## Clipper panel

- Analytics embeds **Clipper Edits** and **Twitch Clips** tabs; links to **Clip Studio** at `/studio/{jobId}`.
- Clipper worker/API is a separate app (`clipper/`); Caddy strips `/v1/clipper` → clipper `:8095`. See `.kiro/steering/clipper.md`.

## Load benchmarks

Scripts: `scripts/benchmark-analytics-load.ps1`, `scripts/benchmark-hls-start.ps1`, `scripts/benchmark-scraper.ps1` (scraper HTTP API via Docker).

| Endpoint / workload | Baseline p50 (pre-tune) | Post-tune p50 (jynxzi, 2026-06-09) | After scraper.md plan (2026-06-09) | Target p50 |
|----------|-------------------------|-----------------------------------|-------------------------------------|------------|
| `POST /v2/scrape` TT detail (Camoufox ephemeral, warm profile) | — | **4.4s** p50 (session 1.6s, profile wait 2.5s) | **9.5s** p50 xqc / **8.0s** p50 plaqueboymax (P0 bridge, Reddit off, queue 0ms); pooled **5.9s** p50 xqc | &lt;45s warm profile |
| `POST /v2/scrape` TT list | — | **11.2s** p50 | **2.8s** p50 / **6.1s** p95 baseline; **6.2s** p50 after | &lt;20s |
| `insights?period=all` | 10–60s | 23ms (cached) / 14s cold | — (avoid on Analytics page) |
| `streams/history?period=all` | — | 18ms | &lt;3s |
| `analytics/streams/{id}?sparse=true` | 5–30s | 96ms (~20 KB, 52 rollups) | &lt;2s |
| `analytics/streams/{id}?sparse=false` | — | 130ms (~59 KB) | — |
| Analytics page → chart (numeric URL) | up to 60s | &lt;2s (history + detail parallel) | &lt;5s |

Re-run after API/UI changes and append measured p50/p95 to this table.

## Task checklist

- Read this file and `.kiro/steering/tech.md` before analytics changes.
- Use codegraph `get_blast_radius("mergeMinuteRollups")`, `get_ast_chunk("gqlCommentText")`, or `get_ast_chunk("targetQueryStreamId")` before editing sync or rollup code.
- Verify via `curl http://localhost:8090/v1/analytics/streams/{streamId}` — check nonzero `chatCount` / `seventvEmoteCount` on synced streams.
- Frontend changes: `cd frontend && npm run build`.
