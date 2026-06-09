---
name: streamclone-analytics-sync
description: Debug Streamclone analytics rollups, TwitchTracker scrapes, and flat viewer charts. Use for sync failures, mock stream IDs, meta#ecs missing, or scraper Cloudflare blocks.
---

# Streamclone analytics sync

Read `.kiro/steering/analytics.md` first.

## Pitfalls (check first)

- Real 2026 Twitch stream IDs can start with `3196` — not mock
- Flat viewer chart → scrape lacks `meta#ecs` (minute data missing)
- `SCRAPER_ALLOW_MOCK_FALLBACK=true` is offline-only

## Runtime probes

1. MCP **`streamclone-stack`** → `scraper_probe`
2. MCP **`streamclone-stack`** → `compose_logs(service="scraper")`
3. MCP **`streamclone-stack`** → `stack_health` (stale localhost / wslrelay)

## Windows TwitchTracker fix

When container ephemeral browser fails on Tracker detail pages:

1. `scripts/scraper-cdp.ps1` (host headless Chrome)
2. Set `CDP_URL=http://host.docker.internal:9222`
3. Set `PROXY_BYPASS=twitchtracker.com`

## Code + tests

- **`streamclone-codegraph`**: `get_blast_radius("mergeMinuteRollups")`, `get_ast_chunk("gqlCommentText")`
- After merge logic changes: `go test ./internal/analytics/...`

## Data inspection

MCP **`streamclone-data`** → `postgres_query` on rollup tables or `data_status` if DB unreachable.
