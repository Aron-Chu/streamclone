# Scraper, Cloudflare, And Proxy Notes

The scraper powers optional TwitchTracker viewer charts for legacy analytics sync. **Core Watch** (directory, playback, chat, emotes) works without it.

## What It Does

- Fetches TwitchTracker history/detail pages.
- Uses Camoufox/Chromium to satisfy Cloudflare.
- Extracts Highcharts data from rendered pages.
- Feeds legacy analytics sync with viewer rollups when that tier is running locally.

## Why Plain HTTP Fails

TwitchTracker commonly blocks direct HTTP clients. The working path is a browser-like session with a persistent profile and warmed cookies.

## Current Recommended Path

1. Use Camoufox as the default browser engine.
2. Keep a persistent scraper profile.
3. Warm the profile before heavy syncs when possible.
4. Let legacy analytics request direct TwitchTracker egress when enabled.
5. Optional: enable `CHALLENGE_FALLBACK_ENABLED=true` so Pydoll handles Turnstile after passive waits fail. Run `make scraper-turnstile-benchmark` to compare handlers.

Useful commands:

```sh
docker compose --profile scraper --env-file .env -f deploy/docker-compose.yml up -d
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

Datacenter proxies made TwitchTracker Cloudflare behavior worse in local tests. Legacy analytics currently sends `useProxy: false` for TwitchTracker detail work, and scraper routing may bypass proxies for Tracker URLs.

Use `PROXY_*` only for experiments or other scrape targets. Keep proxy credentials in `.env.local`; never commit them.

Validate Flame API key and GB balance before proxy benchmarks: `make flame-proxy-preflight`.

Reddit/X social scrape paths are **not** part of public Streamclone scope — see [streampulse-product-boundary.md](streampulse-product-boundary.md).

## Operational Tips

- Empty optional charts usually mean the scraper tier is not running.
- Warm profile plus direct Camoufox is the expected local setup.
- Re-run `make scraper-preflight` after scraper, proxy, or sync changes.
- Keep scraper logs free of cookies, proxy credentials, and API keys.

## Related Files

- [docs/archive/tiers-scraper-and-social-spread.md](archive/tiers-scraper-and-social-spread.md) — historical tier detachment notes (archived)
- [docs/archive/scraping-archive/requirements.md](archive/scraping-archive/requirements.md) — bulk scrape tiers (archived)
- `internal/metadata/api/api.go`
- `scripts/scraper-preflight.*`
- `scripts/scraper-proxy-benchmark.ps1`
- `scripts/scraper-turnstile-benchmark.ps1`
