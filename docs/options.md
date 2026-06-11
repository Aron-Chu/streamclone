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

## Stack control

| Goal | End users | Developers |
|------|-----------|------------|
| Pause (keep data) | **Stop Streamclone** launcher | `make down` or `scripts/stop-streamclone.ps1` |
| Resume | **Start Streamclone** launcher | `make start` |
| Remove volumes only | — | `make down-clean` |
| Complete removal | **Uninstall Streamclone** launcher | `scripts/uninstall-streamclone.ps1` |
| Re-test install (keep folder) | — | `scripts/reset-local-install.ps1 -RemoveVolumes` |

Deployment hardening: [security.md](security.md)
