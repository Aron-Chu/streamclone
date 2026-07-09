# Scraper proxy benchmark (Phase 0 baseline)

Operator summary for Flame residential proxy vs direct Camoufox egress on the local Streamclone scraper (`streamclone-scraper`). Raw JSON stays under gitignored `docs/benchmarks/`; this file is the committed baseline.

**Status:** Baseline captured **2026-06-20** (host `DESKTOP-64B27T6`). Flame credentials were present in `.env.local`. Scraper restored to **direct egress** after the run.

## Prerequisites

| Item | Location | Notes |
|------|----------|-------|
| `PROXY_FLAME_PREMIUM_*` | `.env.local` | Server, username, password (premium / residential package) |
| `PROXY_FLAME_BUDGET_*` | `.env.local` | Server, username, password (budget / residential-lite package) |
| `PROXY_API_KEY` | `.env.local` | Optional; enables `make flame-proxy-preflight` and `-UseFlameApi` on the benchmark script |
| Scraper profile | Docker compose `--profile scraper` | Container `streamclone-scraper` must be running |

Example shape (no live secrets): [`deploy/env/proxy-flame.env.example`](../../deploy/env/proxy-flame.env.example).

**If creds are missing:** set the six `PROXY_FLAME_*` vars in `.env.local`, run `make flame-proxy-preflight`, then re-run the benchmark. Do not commit passwords.

## How to reproduce

```powershell
cd <streamclone-checkout>
# Preflight (direct TT scrape)
powershell -ExecutionPolicy Bypass -File scripts/scraper-preflight.ps1

# Benchmark premium + budget (PowerShell array syntax required)
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
powershell -ExecutionPolicy Bypass -File scripts/scraper-proxy-benchmark.ps1 `
  -Profiles @('premium','budget') `
  -OutFile "docs/benchmarks/scraper-proxy-$stamp.json"
```

Makefile equivalents (WSL/Linux): `make scraper-preflight`, `make scraper-proxy-benchmark`.

Probe definitions live in [`scripts/scraper-proxy-benchmark.ps1`](../../scripts/scraper-proxy-benchmark.ps1) and [`scripts/scrape-test-inline.py`](../../scripts/scrape-test-inline.py).

## Probe matrix

| Probe ID | URL pattern | Pass criteria (summary) |
|----------|-------------|-------------------------|
| `tt_detail_1` | TwitchTracker stream detail (jynxzi) | Chart/injected data or `meta#ecs` |
| `tt_detail_2` | TwitchTracker stream detail (ishowspeed) | Same as detail |
| `tt_list` | TwitchTracker `/xqc/streams` list | Stream list markers in HTML |
| `reddit_json` | `old.reddit.com/.../hot.json` | Reddit JSON/posts in response |
| `reddit_search` | LSF search HTML | Reddit posts in HTML |

Timeout per probe: **120s** (`SCRAPE_TIMEOUT_MS`). One Camoufox attempt per probe unless retry flags are set.

## Baseline results (2026-06-20)

### Pass rates

| Profile | Probes passed | Notes |
|---------|---------------|-------|
| **premium** | **5 / 5** | All TT + Reddit probes OK via Flame premium residential |
| **budget** | **4 / 5** | `tt_list` failed (browser context closed mid-scrape); TT detail OK |
| **direct** (restore run) | **4 / 5** | `reddit_json` failed on direct (196 ms); TT probes OK |

First combined run aborted the **budget** profile when Docker lost the scraper container during profile switch (`RWLayer … unexpectedly nil`). Budget was re-run in a second pass (see JSON paths below).

### Latencies (ms)

| Probe | Premium | Budget | Direct (restore) |
|-------|--------:|-------:|-----------------:|
| `tt_detail_1` | 8,024 | 5,882 | 5,951 |
| `tt_detail_2` | 16,402 | 12,853 | 17,088 |
| `tt_list` | 9,923 | **FAIL** (16,381) | 7,188 |
| `reddit_json` | 14,970 | 3,688 | **FAIL** (196) |
| `reddit_search` | 8,790 | 15,469 | 6,992 |

Cloudflare challenge flag (`cloudflare: true`) was **false** on all completed probes.

### TwitchTracker detail (primary Phase 0 gate)

Both residential profiles successfully parsed Tracker **detail** pages (injected chart data; `meta#ecs` absent but accepted by preflight parity). Premium TT detail latencies were in the **8–16s** range; budget **6–13s**. Direct TT detail on the restore pass was **6–17s**.

**Conclusion:** Premium residential is a viable baseline for Tracker detail experiments. Budget is promising on detail but needs a stable re-run for `tt_list` before production routing. Do **not** enable `ANALYTICS_TT_USE_PROXY` until a second clean budget run and operator sign-off.

## Flame profile notes

| Profile | Product (Flame) | Proxy server | Source |
|---------|-----------------|--------------|--------|
| `premium` | Residential (premium package username) | `http://proxy.flameproxies.com:8989` | Static `.env.local` |
| `budget` | Residential-lite | Same host | Static `.env.local` |

Sticky session suffixes (`-session-{id}`) are applied by the benchmark script when retry flags are enabled; this run used single-attempt probes (`MaxAttemptsPerProbe=1`).

## Local JSON artifacts (gitignored)

| File | Contents |
|------|----------|
| `docs/benchmarks/scraper-proxy-20260620-133834.json` | Premium 5/5; budget profile aborted (Docker) |
| `docs/benchmarks/scraper-proxy-budget-retry-20260620-134004.json` | Budget 4/5 (authoritative budget row) |
| `docs/benchmarks/scraper-proxy-restore-direct.json` | Direct egress verification after restore |

## Restore direct scraper

After any proxy benchmark, return the scraper to direct egress so Analytics TT sync matches normal local dev:

```powershell
cd <streamclone-checkout>

# Recreate scraper with empty PROXY_* (same as benchmark "direct" profile)
powershell -ExecutionPolicy Bypass -File scripts/scraper-proxy-benchmark.ps1 `
  -Profiles @('direct') `
  -OutFile docs/benchmarks/scraper-proxy-restore-direct.json

# Confirm Tracker scrape without proxy
powershell -ExecutionPolicy Bypass -File scripts/scraper-preflight.ps1

# Optional: verify container env is cleared
docker exec streamclone-scraper printenv PROXY_SERVER   # should be empty
docker exec streamclone-scraper printenv PROXY_USERNAME # should be empty
```

Manual alternative: unset `PROXY_SERVER`, `PROXY_USERNAME`, `PROXY_PASSWORD`, and `PROXY_POOL` in compose env, then `docker compose --profile scraper up -d --no-deps --force-recreate scraper`.

**Restore status (2026-06-20):** Direct profile recreate completed; `scraper-preflight` passed; `PROXY_SERVER` / `PROXY_USERNAME` empty in container.

## Related

- Requirements: [requirements.md](requirements.md) Phase 0
- Script: [`scripts/scraper-proxy-benchmark.ps1`](../../scripts/scraper-proxy-benchmark.ps1)
- Operator notes: [docs/scraper-cloudflare-and-proxy.md](../scraper-cloudflare-and-proxy.md)
