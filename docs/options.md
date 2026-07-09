# Optional Features

Core Watch is the default: directory, live playback, chat read, emotes, and basic stream stats. No login is required.

**StreamPulse** (hosted analytics, extension, portal) is a separate product — see [streampulse-product-boundary.md](streampulse-product-boundary.md).

## Profiles

| Profile | Adds | Notes |
|---------|------|-------|
| `core` | Directory, playback, chat, emotes | Default desktop install |
| `scraper` | TwitchTracker minute charts | Uses optional scraper repo/image |
| `full` | Same as `scraper` | Scraper only — does not start ReplayForge |

There is **no clipper compose profile** in Streamclone. Clip Studio runs in **[ReplayForge](../replayforge)** — install separately; API `:8095`, UI `:8096`. Stack status probes ReplayForge `/healthz` but does not start it.

Start ReplayForge separately when editing clips. The install helper wakes on button click when registered at install (see [install-desktop.md](install-desktop.md)); if the browser blocks the protocol prompt, run **Start Streamclone** once. Use setup profiles when scripting scraper installs:

```powershell
powershell -File scripts\setup.ps1 -Profile full
```

```sh
scripts/setup.sh --profile full --non-interactive
```

## Twitch Login

Use `http://localhost:8090/` -> **Sign in (optional)**. Login is for chat send, follows, and Clip Studio token sync. Watching remains anonymous.

Official releases may ship bundled Twitch OAuth in `deploy/env/oauth-bundle.env`. Developers: copy `deploy/env/oauth-bundle.env.example`.

## Optional Scraper

Expected sibling layout for source builds:

```text
parent/
  streamclone/
  streamclone-scraper/
```

Setup can clone the scraper. Put `PROXY_*` experiments in `.env.local`; never commit proxy credentials.

See [scraper-cloudflare-and-proxy.md](scraper-cloudflare-and-proxy.md) for Cloudflare behavior and scraper routing. Historical tier notes: [archive/tiers-scraper-and-social-spread.md](archive/tiers-scraper-and-social-spread.md).

### Reddit / LSF metadata

| Variable | Default | Purpose |
|----------|---------|---------|
| `REDDIT_PROVIDER` | `off` | `off` / `auto` / `scraper` / `oauth` for Browse LSF threads |
| `REDDIT_COMMERCIAL_OK` | `false` | Legal gate when using Reddit OAuth (`true` in release bundle) |
| `SCRAPER_API_URL` / `SCRAPER_API_KEY` | scraper profile defaults | HTML fallback when public JSON is blocked |

## Stack Control

| Goal | End users | Developers |
|------|-----------|------------|
| Pause | Stop launcher | `make down` |
| Resume | Start launcher | `make start` |
| Full teardown | Uninstall | `make nuke` |
| Validate env | Manager diagnostics | `make validate-env` |

Deployment hardening: [security.md](security.md).
