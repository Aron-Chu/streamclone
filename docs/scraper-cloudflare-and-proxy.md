# Scraper, Cloudflare, And Proxy Notes

The scraper powers Analytics minute-level TwitchTracker charts. Core Watch works without it.

## What It Does

- Fetches TwitchTracker history/detail pages.
- Uses Camoufox/Chromium to satisfy Cloudflare.
- Extracts Highcharts data from rendered pages.
- Feeds Analytics sync with viewer rollups.

## Why Plain HTTP Fails

TwitchTracker commonly blocks direct HTTP clients. The working path is a browser-like session with a persistent profile and warmed cookies.

## Current Recommended Path

1. Use Camoufox as the default browser engine.
2. Keep a persistent scraper profile.
3. Warm the profile before heavy syncs when possible.
4. Let Analytics request direct TwitchTracker egress.
5. Optional: enable `CHALLENGE_FALLBACK_ENABLED=true` so Pydoll handles Turnstile after passive waits fail. Run `make scraper-turnstile-benchmark` to compare handlers.

Useful commands:

```sh
make up-scraper
make scraper-check
make scraper-preflight
make scraper-turnstile-benchmark
```

Windows CDP fallback (Camoufox Playwright CDP or Pydoll):

```powershell
powershell -File scripts\scraper-cdp.ps1
```

Then set `CDP_URL=http://host.docker.internal:9222` and `SCRAPER_BROWSER=cdp`, or set `PYDOLL_CDP_URL` with `CHALLENGE_FALLBACK_ENABLED=true` / `SCRAPER_BROWSER=pydoll`.

## Proxy Findings

Datacenter proxies made TwitchTracker Cloudflare behavior worse in local tests. Analytics currently sends `useProxy: false` for TwitchTracker detail work, and scraper routing may bypass proxies for Tracker URLs.

Use `PROXY_*` only for experiments or other scrape targets. Keep proxy credentials in `.env.local`; never commit them.

Validate Flame API key and GB balance before proxy benchmarks: `make flame-proxy-preflight`.

Reddit/X social paths: `make social-probe`.

Social browser fallbacks are bounded separately from item counts. `social.Budget.MaxBrowserFetches`
caps scraper-backed YouTube, Reddit fallback, and StreamerBans fallback calls so a small
`MaxItems` budget cannot fan out into many browser fetches across expanded keywords. Storygraph
sets those caps with `STORYGRAPH_SOCIAL_BROWSER_FETCH_BUDGET` and
`STORYGRAPH_YOUTUBE_BROWSER_FETCH_BUDGET`. Both default to `0` on the shared local
scraper, which disables social browser fallback unless explicitly opted in.

## Operational Tips

- Empty charts in Core Watch usually mean the scraper tier is not running.
- Warm profile plus direct Camoufox is the expected local setup.
- Re-run `make scraper-preflight` after scraper, proxy, or Analytics sync changes.
- Keep scraper logs free of cookies, proxy credentials, and API keys.

## Related Files

- [docs/tiers-scraper-and-social-spread.md](tiers-scraper-and-social-spread.md) — tier detachment, shared scraper, proxies, Social spread
- [docs/scraping-archive/requirements.md](scraping-archive/requirements.md) — bulk scrape tiers, Azure blob archive, backfill phases, benchmark procedures
- `.kiro/steering/analytics.md`
- `internal/analytics/sync.go`
- `internal/metadata/api/api.go`
- `scripts/scraper-preflight.*`
- `scripts/scraper-proxy-benchmark.ps1`
- `scripts/scraper-turnstile-benchmark.ps1`
