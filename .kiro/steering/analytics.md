# Analytics Steering

## Purpose

The analytics service collects per-minute viewer, chat, and 7TV emote rollups for tracked channels. The frontend Analytics page charts rollups, lists stream history, and can sync historical chat from Twitch VOD comments.

## Stack

- Service: `cmd/analytics`, package `internal/analytics`
- API: `/v1/analytics/channels/{login}/live`, `/streams`, `/streams/{streamId}`, sync POST
- Frontend: `frontend/src/components/Analytics.tsx`
- Historical enrichment: TwitchTracker stats + Twitch GQL VOD comment sync (`SyncService`)
- **Scraper** (replaces cloud Firecrawl locally): sibling repo `streamclone-scraper`, Compose service `scraper:8000`. Default engine: **Camoufox** (`SCRAPER_BROWSER=camoufox`) — patched Firefox via Playwright, ephemeral per scrape, profile volume `scraper-profile` at `/data/camoufox-profile`. Fallbacks: `SCRAPER_BROWSER=chromium` or `cdp` (host Chrome via `scripts/scraper-cdp.ps1`). Warm CF cookies once: `scripts/warm-camoufox-profile.ps1`. Benchmark engines: `python benchmark_browsers.py` in scraper repo. Analytics sync sends `useProxy:false` for Tracker URLs; set `PROXY_BYPASS=twitchtracker.com`. Windows: use `.\scripts\diagnose-scraper.ps1 -UseDocker` (avoids wslrelay on localhost:8000).
- **Stream list fallback**: metadata `insights` loads VOD rows from **Twitch Helix** first (fast). TwitchTracker HTML is optional enrichment for avg/peak viewers only. Minute-level viewer charts still need a successful TwitchTracker *detail* scrape (`meta#ecs`).
- **Emote images in analytics**: rollup keys store the local emote-service id (`/emotes/{uuid}/1x.webp`), not the 7TV provider id — do not point charts at `cdn.7tv.app/emote/{rollup-id}`.

### Scraper pitfalls (2026-06-09)

- Never treat Twitch stream IDs with prefix `3196` as mock — real 2026 IDs match that prefix.
- Mock HTML lacks `meta#ecs` → viewer chart is flat (peak-only). Real scrape must succeed for minute-level viewers.
- Set `SCRAPER_ALLOW_MOCK_FALLBACK=true` only for offline dev; default is real Playwright scrape + optional proxy/CDP.

## Local access

- Browser and API probes: **`http://localhost:8090/v1/analytics/...`** through Caddy `local-proxy`.
- On Windows, stale `wslrelay` on `127.0.0.1` can serve old containers — see `.kiro/steering/windows-dev.md`.

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
- Env: `ANALYTICS_VOD_GQL_PAGE_DELAY_MS` (default `0`), `ANALYTICS_TRACKER_SCRAPE_TIMEOUT_MS` (default `60000`).

### VOD resolution

1. TwitchTracker HTML scrape (`extractVodID`) for `video=` / `/videos/{id}`.
2. Helix fallback: `VideoIDByStreamID` in `internal/analytics/helix.go` (paginated archive lookup when HTML has no VOD id).
3. Duration: `VideoDurationSeconds` — rollup window uses `max(tracker duration, vod duration)`.

### GQL VOD comments (`fetchVODComments` in `sync.go`)

- Persisted-query hash must match current Twitch GQL (update when sync returns empty comment bodies).
- **Pagination**: first page uses offset `0`; subsequent pages prefer cursor from the last edge. On integrity failures, falls back to offset (`contentOffsetSeconds + 1`). Keep-alive `gqlClient`; optional inter-page delay via `ANALYTICS_VOD_GQL_PAGE_DELAY_MS`.
- **Comment text**: `gqlCommentText()` joins `message.fragments[].text` when `message.body` is empty (common on current GQL payloads). Tests: `sync_gql_test.go`.

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

Scripts: `scripts/benchmark-analytics-load.ps1`, `scripts/benchmark-hls-start.ps1` (via Caddy `http://localhost:8090`).

| Endpoint | Baseline p50 (pre-tune) | Post-tune p50 (jynxzi, 2026-06-09) | Target p50 |
|----------|-------------------------|-----------------------------------|------------|
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
