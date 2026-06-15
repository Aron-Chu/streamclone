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

Useful commands:

```sh
make up-scraper
make scraper-check
make scraper-preflight
```

Windows CDP fallback:

```powershell
powershell -File scripts\scraper-cdp.ps1
```

Then set `CDP_URL=http://host.docker.internal:9222` and `SCRAPER_BROWSER=cdp`.

## Proxy Findings

Datacenter proxies made TwitchTracker Cloudflare behavior worse in local tests. Analytics currently sends `useProxy: false` for TwitchTracker detail work, and scraper routing may bypass proxies for Tracker URLs.

Use `PROXY_*` only for experiments or other scrape targets. Keep proxy credentials in `.env.local`; never commit them.

## Operational Tips

- Empty charts in Core Watch usually mean the scraper tier is not running.
- Warm profile plus direct Camoufox is the expected local setup.
- Re-run `make scraper-preflight` after scraper, proxy, or Analytics sync changes.
- Keep scraper logs free of cookies, proxy credentials, and API keys.

## Related Files

- `.kiro/steering/analytics.md`
- `internal/analytics/sync.go`
- `internal/metadata/api/api.go`
- `scripts/scraper-preflight.*`
