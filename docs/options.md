# Optional Features

Core Watch is the default: directory, live playback, chat read, emotes, basic stream stats. No login is required.

## Profiles

| Profile | Adds | Notes |
|---------|------|-------|
| `core` | Directory, playback, chat, emotes | Default |
| `scraper` | TwitchTracker minute charts | Uses optional scraper repo/image |
| `clipper` | Clip Studio at `/studio` | Sign in for Twitch clip creation |
| `pulse` | Grafana + Influx dashboards | Optional dashboard layer |
| `full` | Scraper + clipper | Does not include Pulse |

Start optional tiers from the app, or with setup:

```powershell
powershell -File scripts\setup.ps1 -Profile full
```

```sh
scripts/setup.sh --profile full --non-interactive
```

## Twitch Login

Use `http://localhost:8090/` -> **Sign in (optional)**. Login is for chat send, follows, and Clip Studio. Watching remains anonymous.

Developers with a custom Twitch app can fill `deploy/env/oauth-bundle.env`.

## Analytics Scraper

Expected sibling layout for source builds:

```text
parent/
  streamclone/
  streamclone-scraper/
```

Setup can clone the scraper. Put `PROXY_*` experiments in `.env.local`; never commit proxy credentials.

See [scraper-cloudflare-and-proxy.md](scraper-cloudflare-and-proxy.md) for Cloudflare behavior and scraper routing.

## Pulse Dashboards

Pulse exports local Analytics rollups to InfluxDB 2.7 and Grafana 11.5.

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

## Stack Control

| Goal | End users | Developers |
|------|-----------|------------|
| Pause | Stop launcher | `make down` |
| Resume | Start launcher | `make start` |
| Full teardown | Uninstall | `make nuke` |
| Validate env | Manager diagnostics | `make validate-env` |

Deployment hardening: [security.md](security.md).
