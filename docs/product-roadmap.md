# Product Roadmap

Streamclone is a local-first Twitch-style watch surface: reliable playback, chat, emotes, Analytics, and optional creation tools.

## Current Product Tiers

| Tier | User value |
|------|------------|
| Core Watch | Browse, watch, chat read, emotes |
| Analytics | Viewer/chat/emote rollups and VOD context |
| Clip Studio | Turn live or synced moments into vertical clips |
| Pulse | Optional Grafana view over local Analytics rollups |

## Near-Term Priorities

1. **Channel workspace polish**
   - Keep playback controls stable.
   - Show requested vs loaded quality honestly.
   - Improve theater/mobile ergonomics without adding new modes.

2. **Tier honesty**
   - Make missing optional services obvious.
   - Keep empty Analytics states actionable.
   - Start optional tiers from the app, not from raw compose commands.

3. **VOD and moment loop**
   - Keep Analytics -> Play in Streamclone -> Export Moment working.
   - Improve error copy for unavailable VODs and auth failures.
   - Add narrower resync paths before adding broader sync features.

4. **Install reliability**
   - Keep manager Update/Repair as the primary support path.
   - Avoid changes that only work in the source checkout but not release images.
   - Log install lifecycle fixes in [repo-maintenance.md](repo-maintenance.md).

5. **Clip Studio adoption**
   - Keep templates, captions, trim, and export fast on desktop and usable on mobile.
   - Surface Twitch/Streamlink/FFmpeg failures as job states.

## Later Ideas

- A personalized "My streams" home.
- Data export for synced rollups.
- Picture-in-picture watch mode.
- Better Analytics operations views for scraper/profile health.
- Shared IRC ingest to reduce duplicate upstream connections.

## Guardrails

- Prefer refinement of existing flows before new surfaces.
- Keep Core Watch useful without login, scraper, clipper, or Pulse.
- Keep public/tunnel behavior aligned with [security.md](security.md).
- Update [options.md](options.md) when a user-visible tier changes.
