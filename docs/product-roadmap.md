# Streamclone product roadmap

Compact roadmap for Streamclone: a local-first Twitch viewing workspace with HLS relay playback, chat, emotes, analytics, VOD replay, and optional Clip Studio.

## Product position

Streamclone optimizes for:

- Local playback through the Streamclone stack at `http://localhost:8090`
- Read-only viewing without a Twitch login
- Server-side chat, emote, and viewer analytics that the operator owns
- Honest optional tiers: core watch, analytics scraper, and clipper
- Clear diagnostics when a local service, scraper, relay, or sync job is not ready

The browser should talk to Streamclone services through Caddy. Do not send users to raw service ports as the default path.

## Current tiers

| Tier | Profile | User value |
|------|---------|------------|
| Core Watch | `core` | Directory, HLS relay, chat read, emote rendering, summary stream history |
| Analytics | `scraper` | Minute viewer charts, VOD chat sync, replay heatmap, moment scoring |
| Clip Studio | `clipper` | Analytics-to-clip queue, local render/export workflow |

Core install should stay useful without optional tiers. Optional tiers should advertise missing requirements plainly and offer a one-click start path when available.

## P0 quality bar

- No browser console spam on primary routes
- No stale hashed assets after a normal hard refresh
- VOD deep links stay on localhost and use `POST /v1/stream/vod/start`
- Twitch is an explicit fallback action, never an automatic analytics redirect
- Moment score is canonical: backend replay heatmap score wins, frontend estimates are marked with `~`
- Theater/settings controls do not resize the theater player; `Shrink` exits theater
- Security scans block committed local debug ingest URLs and debug session headers

## Near-term priorities

| Area | Priority | Notes |
|------|----------|-------|
| Analytics moments | P0 | Shared score model, one selected-moment action cluster, backend heatmap detail breakdown |
| VOD review | P0 | Local VOD playback, VOD mode banner, activity strip, chat replay when `sid` is present |
| Chart robustness | P0 | Guard empty/one-point rollups, invalid game segments, invalid heatmap durations |
| Live analytics | P0 | Wire live stats and most-reacted states with honest collecting/empty labels |
| Stream status labels | P0 | Use `Current live`, `Synced`, `Stats only`, `Not tracked`, `Sync interrupted` consistently |
| Docs/security | P0 | Keep user docs short, delete scratch artifacts, scan for debug instrumentation |

## Next wave

- Improve sync recovery: clearer stale-job retry, chat-only resync, and partial coverage explanations.
- Add analytics export: CSV/JSON for minute rollups, top emotes, and selected moments.
- Add better VOD relay diagnostics: upstream status, retry reason, and Twitch fallback copy in one place.
- Improve optional-services UX: show exact profile/service impact before starting Analytics or Clip Studio.
- Add browser smoke coverage for analytics, VOD mode, theater/settings, and sync status panels.

## Deferred ideas

- Tray app for start/stop/logs
- Offline installer bundle
- macOS/Linux packaged installers
- Cross-channel comparisons and creator-style dashboards
- Managed cloud deployment

## Source of truth

- User docs: [README](../README.md), [install](install-desktop.md), [options](options.md), [security](security.md), [scraper notes](scraper-cloudflare-and-proxy.md)
- Maintainer docs: [repo maintenance](repo-maintenance.md), `AGENTS.md`, `.kiro/steering/*`
- Feature specs and design notes belong in `.kiro/specs/`
