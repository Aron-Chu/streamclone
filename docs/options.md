# Optional Features

Core Watch is the default: directory, live playback, chat read, emotes, basic stream stats, and Pulse Wire UI when enabled. No login is required.

## Profiles

| Profile | Adds | Notes |
|---------|------|-------|
| `core` | Directory, playback, chat, emotes, storygraph service | Default |
| `scraper` | TwitchTracker minute charts | Uses optional scraper repo/image |
| `full` | Same as `scraper` | **Scraper only** — does not start ReplayForge or Pulse Grafana |
| `pulse` | Grafana + Influx dashboards | Optional ops/export tier |

There is **no clipper compose profile** in Streamclone. Clip Studio runs in **[ReplayForge](../replayforge)** — install separately; API `:8095`, UI `:8096`. Stack status probes ReplayForge `/healthz` but does not start it.

Start Analytics and Pulse dashboards from Stack status in the app. Start ReplayForge separately when editing clips. The install helper wakes on button click when registered at install (see [install-desktop.md](install-desktop.md)); if the browser blocks the protocol prompt, run **Start Streamclone** once. Use setup profiles when scripting Analytics installs:

```powershell
powershell -File scripts\setup.ps1 -Profile full
```

```sh
scripts/setup.sh --profile full --non-interactive
```

## Twitch Login

Use `http://localhost:8090/` -> **Sign in (optional)**. Login is for chat send, follows, and Clip Studio token sync. Watching remains anonymous.

Official releases may ship bundled Twitch OAuth in `deploy/env/oauth-bundle.env`. Without it, Pulse Wire Reddit ingest still works when `REDDIT_COMMERCIAL_OK=true`. Developers: copy `deploy/env/oauth-bundle.env.example`.

## Analytics Scraper

Expected sibling layout for source builds:

```text
parent/
  streamclone/
  streamclone-scraper/
```

Setup can clone the scraper. Put `PROXY_*` experiments in `.env.local`; never commit proxy credentials.

See [scraper-cloudflare-and-proxy.md](scraper-cloudflare-and-proxy.md) for Cloudflare behavior and scraper routing. For tier boundaries, shared scraper policy, proxies, and Social spread behavior, see [tiers-scraper-and-social-spread.md](tiers-scraper-and-social-spread.md).

## Pulse Wire (Story Graph)

Pulse Wire is the `/pulse-wire` streamer news feed (separate from Pulse Grafana dashboards).

| Variable | Default | Purpose |
|----------|---------|---------|
| `PULSE_WIRE_ENABLED` | `false` | Gates ingest workers and public `/v1/pulse-wire/*` API |
| `PULSE_WIRE_SEMANTIC` | `false` | Enables optional pgvector semantic matcher (`pulse-wire-semantic` profile) |
| `REDDIT_COMMERCIAL_OK` | `false` | Legal gate for Reddit ingest (release bundle sets `true`) |
| `SOCIAL_RETENTION_DAYS` | `90` | Retention for social evidence rows |
| `STORYGRAPH_YT_KEYWORDS` | empty (release bundle: `kai cenat,xqc,caseoh,streamer drama`) | Comma-separated search terms for YouTube ingest |
| `YOUTUBE_API_KEY` | empty | YouTube Data API for evidence (primary path) |
| `SCRAPER_API_URL` / `SCRAPER_API_KEY` | scraper profile defaults | Scraper fallback for YouTube search when API key is absent or blocked. When `SCRAPER_API_KEY` is set in `.env`, `make up` auto-includes `--profile scraper`. |
| `MEDIA_MATCHER_URL` | optional | Sidecar when `pulse-wire` compose profile enabled |
| `STREAMERBANS_INGEST_ENABLED` | `false` | Master switch for StreamerBans ban ingest (`streamerbans` source) |
| `X_UNOFFICIAL_OK` | `false` | Legal/ToS gate for unofficial X ingest via emusks sidecar (required for tier 2 below) |
| `X_AUTH_TOKEN` / `EMUSKS_X_AUTH_TOKEN` | empty | Either token can authenticate `x-ingest` when unofficial X path is enabled; leave empty for tier 1 and CI-safe validation |
| `X_INGEST_URL` | `http://x-ingest:8098` | Bun+emusks sidecar base URL for `@StreamerBans` timeline |
| `METADATA_SERVICE_URL` | `http://metadata:8080` | Helix profile images for entity `avatarUrl` on story cards (Redis cache 24h) |
| `PULSE_DIRECTORY_SAMPLE_INTERVAL` | `10m` | How often storygraph samples top live streams into `directory_samples` |
| `PULSE_DIRECTORY_TOP_N` | `200` | Max ranked live streams per sample run (also seeds clip discovery + entity resolution) |
| `PULSE_DIRECTORY_RETENTION_DAYS` | `30` | Retention for directory samples and follower snapshots |
| `STORYGRAPH_STORE_TEST_DATABASE_URL` | unset | **Dev/tests only.** Postgres DSN for `go test ./internal/storygraph/store/...`. Tests **drop `public` schema** — use a dedicated database (e.g. `streamclone_storygraph_test`), never the live `streamclone` DB. Not read by storygraph at runtime. |

**Pulse Wire News MVP knobs**

| Variable | Default | Purpose |
|----------|---------|---------|
| `STORYGRAPH_INGEST_INTERVAL` | `5m` | Poll interval for social ingest + window score recompute |
| `STORYGRAPH_SOCIAL_BROWSER_FETCH_BUDGET` | `0` | Per-cycle browser fallback cap for Reddit, StreamerBans, and Reddit comment link hydration on the shared scraper; `0` disables browser fallback. Use `2` when Pulse Wire should corroborate clips with Reddit/YouTube (full profile sets this). |
| `STORYGRAPH_YOUTUBE_BROWSER_FETCH_BUDGET` | `0` | Per-cycle YouTube scraper fallback cap when `YOUTUBE_API_KEY` is absent; `0` disables browser fallback |
| `STORYGRAPH_SOCIAL_SCRAPE_USE_PROXY` | `false` | Pass `useProxy: true` on Pulse Wire social scraper requests (enable when scraper has `PROXY_*` and Docker egress is blocked) |
| `STORYGRAPH_YT_KEYWORDS` | empty (release bundle: tracked channels) | Base YouTube terms; workers also expand from directory logins + news terms (`drama`, `ban`, `clip`, …) |
| `PULSE_DIRECTORY_SAMPLE_INTERVAL` | `10m` | Directory sampler cadence; failures retry with backoff and surface in `/source-health` |

Window semantics: `today` = UTC midnight → now; `24h` = rolling 24 hours; `7d` = rolling 7 days. Feed ranking uses `story_window_scores` (`rankModel: window-native-v1`). Edition sections: `GET /v1/pulse-wire/edition?window=today|24h|7d`.

When `STREAMERBANS_INGEST_ENABLED=true` but no X token is set (or sidecar is unhealthy), ingest falls back to parsing [streamerbans.com](https://streamerbans.com/).

**StreamerBans ingest tiers**

| Tier | Env / compose | Path |
|------|---------------|------|
| **1 — free (OSS default)** | `STREAMERBANS_INGEST_ENABLED=true` only | HTML parse of streamerbans.com — no token, no sidecar |
| **2 — richer / fresher (optional)** | Tier 1 + `X_UNOFFICIAL_OK=true` + `X_AUTH_TOKEN` or `EMUSKS_X_AUTH_TOKEN` + `docker compose --profile pulse-wire up x-ingest` | `@StreamerBans` timeline via Bun+emusks sidecar with exact tweet URLs when available; falls back to tier 1 on failure |

Default validation (`make validate-env`) is allowed to pass without X credentials. It should only warn on partial tier-2 config: `X_UNOFFICIAL_OK=true` without a token, an X token without `X_UNOFFICIAL_OK=true`, or tier-2 variables set while `STREAMERBANS_INGEST_ENABLED` is not true.

Tier-2 proof is credential-gated: start the `pulse-wire` profile with `x-ingest`, keep tokens in `.env.local` or a local `.env`, and verify source health plus story evidence/provenance preserves exact `@StreamerBans` tweet URLs. Never commit X tokens or token-bearing logs.

General **X wire trends** (not ban timeline) remain Phase 2/3 deferred — the `xrecent` stub is not scheduled in ingest workers. If pursued later, emusks could be a budgeted connector, but R16 still forbids filtered firehose in prod builds.

**Current source setup (Evidence Gallery)**

| Source | Required env / profile | Evidence path |
|--------|------------------------|---------------|
| Twitch clips | Wire enabled + Twitch OAuth client credentials (release bundle) | Helix global/top streams → clip URL + title links |
| Reddit/LSF | `REDDIT_COMMERCIAL_OK=true` | Public JSON at `reddit.com` by default; `--profile scraper` when JSON blocked; comment URL extraction on posts |
| YouTube | `STORYGRAPH_YT_KEYWORDS` or `ALWAYS_TRACKED_CHANNELS` + (`YOUTUBE_API_KEY` or `SCRAPER_API_URL`) | Search results + description URL extraction when API key set |
| StreamerBans | `STREAMERBANS_INGEST_ENABLED=true` | [streamerbans.com](https://streamerbans.com/) HTML; optional emusks tier below |
| X / TikTok / Instagram | No discovery ingest | Link-only via extracted URLs, manual add-evidence, or oEmbed |

Probe ingest health: `GET /v1/pulse-wire/source-health` (`active`, `link_only`, `off`, `error`, `deferred`).

**Historical backfill:** Reddit/YouTube backfill runs once on storygraph startup with a strict item budget (24 items/source); it does not synthesize historical trend snapshots. See `.kiro/steering/pulse-wire.md` for ingest behavior.


```powershell
docker compose --env-file .env -f deploy/docker-compose.yml --profile scraper --profile pulse-wire up -d storygraph x-ingest
```

**Ban category backfill (optional, manual):** re-tag legacy clusters that mention bans:

```sql
UPDATE story_clusters SET category = 'bans'
WHERE category <> 'bans'
  AND (title ILIKE '%ban%' OR title ILIKE '%suspend%');
```

Or call `POST /v1/pulse-wire/reclassify` with `X-Streamclone-Setup-Token` to re-run the classifier on recent clusters.

When disabled, `/pulse-wire` shows a hint to set `PULSE_WIRE_ENABLED=true`. When enabled with an empty feed, the UI polls while ingest warms up.

**Clip-only cluster cleanup (optional, manual):** if short Twitch clip slugs became wire headlines before title-quality gates, remove orphan clusters with:

```sql
DELETE FROM story_clusters c WHERE length(c.title) < 12 AND c.summary = 'Wire-native social evidence grouped from global source ingest.' AND NOT EXISTS (SELECT 1 FROM story_evidence e JOIN social_items i ON i.id = e.item_id WHERE e.cluster_id = c.id AND i.source <> 'twitchclips');
```

Run against local Postgres only when you want to prune stale clip-only rows; it does not affect core Watch data.

## Pulse Dashboards (Grafana)

Pulse is an optional local dashboard layer started with **Start Pulse dashboards** from Stack status. It exports local Analytics rollups to InfluxDB 2.7 and Grafana 11.5, then opens the Grafana dashboard from in-app Analytics.

Canonical measurements:

- viewer count
- chat message count
- 7TV emote count
- top emote names/counts when available

Rules:

- In-app Analytics works without Pulse.
- Pulse is local-only by default.
- Dashboard data is derived from synced local rollups.
- Do not expose Grafana/Influx publicly without strong credentials and firewalling.

## Chat logs (local)

| Variable | Default | Purpose |
|----------|---------|---------|
| `ANALYTICS_VOD_CHAT_PRESERVE_URLS` | off (`0`) | Keep URLs in synced VOD chat rows (local fidelity) |
| `CHAT_LOG_PERSIST_ENABLED` | `false` | Archive live chat + mod events to Postgres via Analytics |
| `CHAT_LOG_RETENTION_DAYS` | `14` | Purge live archive + mod events after N days |
| `ANALYTICS_SERVICE_URL` | `http://analytics:8080` | Chat service ingest target (compose internal) |

VOD chat replay remains opt-in via Analytics sync. The `/logs/:login` UI reads synced VOD history and, when enabled, recent live archive rows stored locally.

## Stack Control

| Goal | End users | Developers |
|------|-----------|------------|
| Pause | Stop launcher | `make down` |
| Resume | Start launcher | `make start` |
| Full teardown | Uninstall | `make nuke` |
| Validate env | Manager diagnostics | `make validate-env` |

Deployment hardening: [security.md](security.md).
