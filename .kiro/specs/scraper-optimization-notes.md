# Scraper optimization notes

Benchmarks and config matrix for TwitchTracker analytics sync. See also [`.kiro/steering/analytics.md`](../steering/analytics.md).

## P0 bridge profile (2026-06-09)

Local stack aligned with `scraper.md` bridge-safe defaults:

| Setting | Value |
|---------|-------|
| `SCRAPER_EPHEMERAL_BROWSER` | `true` |
| `SCRAPER_MAX_CONCURRENT` | `1` |
| `ANALYTICS_TT_DIRECT_HTTP_ENABLED` | `false` |
| `REDDIT_PROVIDER` | `off` |

**Compose note:** run `docker compose --env-file .env -f deploy/docker-compose.yml …` from repo root so scraper `environment:` substitutions pick up workspace `.env` (otherwise compose defaults `ephemeral=false`, `max_concurrent=2` override `env_file`).

### Health probe (docker exec)

```text
ephemeral=true, max_concurrent=1, browser=camoufox, inject_highcharts via compose default
```

### Benchmark matrix A — TT detail (no Reddit queue contention)

| Stream | Login | p50 | p95 | queueWait p50 | cloudflare | chart | viewerExtraction |
|--------|-------|-----|-----|---------------|------------|-------|------------------|
| `319638832474` | xqc | **9.5s** | 9.5s | **0ms** | resolved | ok | injection, 81 pts, 354 min |
| `316746299127` | plaqueboymax | **8.0s** | 8.0s | **0ms** | resolved | ok | (viewers-only path) |

Success criteria met: TT detail &lt;30s queue wait, no 79s Reddit block, `protection_state=resolved`.

Raw JSON: `benchmark-p0-xqc.json`, `benchmark-p0-pbm.json` (repo root).

### Pooled profile smoke (optional Phase 4)

| Profile | Stream | p50 | queueWait p50 | sessionOpen p50 |
|---------|--------|-----|---------------|-----------------|
| `ephemeral=false`, `max_concurrent=2` | xqc `319638832474` | **5.9s** | 0ms | **0ms** (warm pool) |

Pooled mode stable on single TT detail run; faster than ephemeral bridge due to reused browser session. Keep bridge profile for Windows dev unless pool verified under mixed workloads.

## Code polish (same session)

- `viewerExtraction` block in scraper validation payload (`method`, `pointCount`, `durationMinutes`, `injectedHighcharts`).
- TT detail `scrape_cache.put()` gated on ecs or ≥3 chart points.
- `GET .../sync/status` returns `200 {"phase":"idle"}` when Redis key missing.

## TT network + proxy comparison (2026-06-09 evening)

**What analytics needs from TT detail:** `viewerExtraction` with ≥3 chart points (injection or `meta#ecs`), `cloudflareState=resolved`, ~80 sample points for minute rollups.

### Docker internal (`docker exec streamclone-scraper`) — authoritative

| Stream | useProxy requested | usedProxy | p50 | chartPoints | method | CF |
|--------|-------------------|-----------|-----|-------------|--------|-----|
| xqc `319638832474` | false | false | 8.1s | 81 | injection | resolved |
| xqc `319638832474` | true | **false** | 9.7s | 81 | injection | resolved |
| plaqueboymax `316746299127` | false | false | 6.8s | 81 | injection | resolved |

Camoufox **forces direct** for TwitchTracker (`proxy_attempts → [false]`); `proxy_pool_count=0` so proxy is unavailable anyway. Requesting `useProxy:true` does not change routing — only adds queue wait if concurrent scrapes overlap.

**Burst test (5 rapid scrapes, same URL):** 5/5 success, 7–9s each, 81 points — no rate-limit/firewall in current window.

### Host `http://127.0.0.1:8000` (Windows wslrelay risk)

| Path | success | chartPoints | notes |
|------|---------|-------------|-------|
| Host localhost:8000 | true | **0** | ~4–5s — SVG shell passes old validation, no injection data |
| Docker internal | true | **81** | ~7–9s — full Highcharts injection |

**Do not benchmark or debug via host `:8000` on Windows.** Use `docker exec streamclone-scraper` or analytics at `:8090` (compose network). Host path may hit wslrelay/stale listener.

### Humanized retry (added)

Env defaults: `SCRAPER_RETRY_MAX=3`, `SCRAPER_RETRY_MIN_MS=2000`, `SCRAPER_RETRY_MAX_MS=12000`. Random delay between retries on Cloudflare block, validation failure (sparse chart shell), or navigation errors. TT detail validation now rejects pages with Highcharts SVG shell but &lt;3 injected/ecs points.
