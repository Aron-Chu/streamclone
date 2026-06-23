---
description: Debug Streamclone scraper — Camoufox, Cloudflare, TwitchTracker timings, proxy/cache, concurrency caps. Do not raise SCRAPER_MAX_CONCURRENT blindly.
---

# Scraper Debug

Read `AGENTS.md`, `.kiro/steering/analytics.md`, and `docs/scraper-cloudflare-and-proxy.md`.

## First checks

- Scraper is optional compose profile `scraper` — sibling repo `streamclone-scraper` when using source overlay.
- Use `http://localhost:8090` for API probes; scraper direct port `:8000` only when intentionally bypassing Caddy.
- **Do not raise `SCRAPER_MAX_CONCURRENT` blindly** — Camoufox browser pool and host RAM/CPU are the real limits; raising concurrency often triggers CF challenges and pool lock contention.

## Camoufox / Cloudflare / TwitchTracker

- CF wait states: `just a moment`, Turnstile, `performing security verification` — see `scraper_probe` hints.
- TwitchTracker minute charts need `meta#ecs` in HTML — flat charts often mean CF block or stale cache, not Analytics CPU.
- Timings: tracker scrape intervals vs Analytics sync expectations in `.kiro/steering/analytics.md`.

## Proxy / cache / concurrency

- Proxy benchmarks: `make scraper-proxy-benchmark`, `docs/scraping-archive/requirements.md`
- Turnstile / CF scripts: `make scraper-turnstile-benchmark`, `scripts/scraper-cdp.ps1`
- Preflight: `make scraper-preflight`, `make scraper-check`
- Concurrency caps live in compose env (`SCRAPER_MAX_CONCURRENT`, browser pool size) — tune after proxy/CF baseline, not before.

## MCP / runtime

- `scraper_probe()` — direct vs proxy TwitchTracker scrape analysis
- `compose_logs("scraper")` — bounded tail via stack MCP
- `stack_health` — scraper container presence

## Codegraph

- `get_ast_chunk("SyncService")` — archive/backfill paths
- `search_symbols("scraper")` — scraper integration touchpoints

## Tests

```sh
make scraper-preflight
make scraper-check
go test ./internal/analytics/...
```
