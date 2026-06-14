# Optional features

Default **core** profile: watch streams, chat, emotes, basic analytics. **No Twitch login required.**

---

## Profiles

| Profile | Adds | Extra setup |
|---------|------|-------------|
| `core` | Directory, playback, chat, emotes | None |
| `scraper` | TwitchTracker viewer charts | [streamclone-scraper](https://github.com/Aron-Chu/streamclone-scraper) sibling repo |
| `clipper` | Clip Studio (`/studio`) | Optional **Sign in** at localhost:8090 (or Twitch CLI for devs) |
| `full` | Scraper + clipper | Scraper sibling + optional sign-in for clips |

```powershell
# interactive
powershell -File scripts\setup.ps1

# or non-interactive
scripts\setup.sh --profile full --non-interactive
```

---

## Twitch login (chat write, Clip Studio)

**Easiest (desktop):** open **`http://localhost:8090/`** → **Sign in (optional)** → approve on Twitch. No CLI, no `.env` editing on official releases.

**Developers / custom OAuth app:**

1. Copy `deploy/env/oauth-bundle.env.example` → `deploy/env/oauth-bundle.env` and fill in Client ID + Secret from [dev.twitch.tv](https://dev.twitch.tv/console/apps) (no redirect URL needed).
2. Re-run `scripts/setup.ps1` or recreate `.env`.
3. Use **Sign in (optional)** in the app, or optionally `make twitch-local-auth` if you still use Twitch CLI.

Token refresh (clipper): automatic when refresh token is present; or `make refresh-auth`.

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
| Full teardown (compose + helm + integration) | — | `make nuke` |
| Complete removal | **Uninstall Streamclone** launcher | `scripts/uninstall-streamclone.ps1` |
| Re-test install (keep folder) | — | `scripts/reset-local-install.ps1 -RemoveVolumes` |

Deployment hardening: [security.md](security.md)
