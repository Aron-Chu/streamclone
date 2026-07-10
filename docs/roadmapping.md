# Streamclone core — roadmap (watch stack only)

| | |
|---|---|
| **Status** | Current scope note (rewritten 2026-07 — boundary split) |
| **Owns** | Public Streamclone desktop watch / HLS / chat / emotes |
| **Does not own** | StreamPulse BFF, portal, extension overlay, hosted ops, clip render |

## Product line

```text
Public Streamclone = Twitch-replica watch desk
StreamPulse (extension + portal + API) = private backend + streamclone-pulse
ReplayForge = clip jobs / studio / render
```

## Active work surfaces (this repo)

- Watch UI, player, directory, chat, emotes
- Desktop install / packaging for the watch stack
- Strict product-boundary gate (`make product-boundary-strict`)
- Legacy local adapters (`/v1/clipper`, `/studio`) — shrink only; no new features

## Where Pulse / analytics / ops live now

| Concern | Repo |
|---------|------|
| Live coverage / Protect / backfill UX | `streamclone-pulse` `docs/pulse-extension/` |
| Portal / hub / analytics UI | `streamclone-pulse` `docs/website-portal/` |
| Go BFF, ingest, hub, migrations | `streampulse-backend` |
| Hosted deploy / smoke / capacity env | `streampulse-ops` (private) |
| Clip Studio / import handoff | `replayforge` |

## HISTORICAL

Earlier drafts of this file described R0–R6 Pulse/Kick phases and BearHost analytics ownership inside Streamclone. That content is obsolete after the boundary split. Do not restore analytics API, rollups, hub ingest, or hosted deploy instructions here.

Archive: `docs/archive/agent-plans/`, plus any dated archaeology under `docs/`.
