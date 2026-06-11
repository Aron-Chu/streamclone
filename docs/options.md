# Optional features

Default **core** profile: watch streams, chat, emotes, basic analytics. **No Twitch login required.**

---

## Profiles

| Profile | Adds | Extra setup |
|---------|------|-------------|
| `core` | Directory, playback, chat, emotes | None |
| `scraper` | TwitchTracker viewer charts | [streamclone-scraper](https://github.com/Aron-Chu/streamclone-scraper) sibling repo |
| `clipper` | Clip Studio (`/studio`) | Twitch CLI + device login |
| `full` | Scraper + clipper | Both above |

```powershell
# interactive
powershell -File scripts\setup.ps1

# or non-interactive
scripts\setup.sh --profile full --non-interactive
```

---

## Twitch login (chat write, Clip Studio)

1. Install [Twitch CLI](https://github.com/twitchdev/twitch-cli) → `twitch configure`
2. `make twitch-sync`
3. `make twitch-local-auth` (device code in browser)

Token refresh: `make refresh-auth`

---

## Scraper layout

```
parent/
  streamclone/
  streamclone-scraper/   ← optional
```

Setup can clone the scraper for you. Never commit `PROXY_*` credentials — use `.env.local`.

---

## Commands

| Task | Command |
|------|---------|
| Stop stack | `scripts\stop-streamclone.ps1` or **Stop Streamclone** |
| Health check | `make smoke` |
| Validate `.env` | `make validate-env` |
| Re-setup | `make down` then `make setup` |

Deployment hardening: [security.md](security.md)
