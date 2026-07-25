# Optional Features

Core Watch is the default: directory, live playback, chat read, emotes, and basic stream stats. No login is required.

**StreamPulse** (hosted analytics, extension, portal) and **ReplayForge** (Clip Studio) are separate products — see [streampulse-product-boundary.md](streampulse-product-boundary.md). Streamclone does not start or advertise them.

## Profiles

| Profile | Adds | Notes |
|---------|------|-------|
| `core` | Directory, playback, chat, emotes | Default desktop install |
| `scraper` | TwitchTracker minute charts | Uses optional scraper repo/image |
| `full` | Same as `scraper` | Scraper only |

```powershell
powershell -File scripts\setup.ps1 -Profile full
```

```sh
scripts/setup.sh --profile full --non-interactive
```

## Twitch Login

Use `http://localhost:8090/` -> **Sign in (optional)**. Login is for chat send and follows. Watching remains anonymous.

Provide your own Twitch OAuth credentials locally via `deploy/env/oauth-bundle.env` (copy from `oauth-bundle.env.example`). Releases never embed a client secret. Developers: `twitch configure` then setup sync also works.

## Optional Scraper

Expected sibling layout for source builds:

```text
parent/
  streamclone/
  streamclone-scraper/
```

Setup can clone the scraper. Put `PROXY_*` experiments in `.env.local`; never commit proxy credentials.

See [scraper-cloudflare-and-proxy.md](scraper-cloudflare-and-proxy.md) for Cloudflare behavior and scraper routing. Historical tier notes: [archive/tiers-scraper-and-social-spread.md](archive/tiers-scraper-and-social-spread.md).
