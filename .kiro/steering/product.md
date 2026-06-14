# Product Steering

## Purpose

Streamclone is a self-hosted educational streaming and chat platform that mirrors the core Twitch viewing loop: browse live channels, start a local HLS relay, connect to live chat, and render custom emote-rich chat.

The application intentionally keeps upstream access server-side. The browser talks to our metadata, video, chat, emote, MediaMTX, and object-storage surfaces, never directly to Twitch, 7TV, or other emote providers.

## Core Experience

- Directory and search come from the Metadata service, backed by Redis caching and Twitch internal GraphQL.
- Video starts on demand through the Video Orchestrator, which obtains playback tokens, starts a Streamlink/FFmpeg worker, and publishes to MediaMTX. If Streamlink cannot produce a ready local HLS relay, the orchestrator can fall back to a server-side direct Usher HLS to FFmpeg worker.
- The default local browser path is the single-origin proxy at `http://localhost:8090`. Anonymous browsing, playback, and chat read should work there without any first-party account (**optional sign-in**); authenticated Twitch chat sending and clipper scopes require the localhost-only device-code / token import flow.
- First-run **WelcomeOverlay** shows **SystemHealthPanel** (full variant) plus optional-service controls. **Settings → Stack status** shows the compact health panel. Health data comes from `useSystemHealth` and `useOptionalServices`; do not duplicate compose probes in the UI. Optional service status should fall back from `/v1/setup/welcome` to `/v1/setup/diagnostics`, avoid repeated setup-control probes in compact status surfaces, and treat missing scraper/clipper as a friendly offline tier instead of noisy failure.
- **Start Analytics** and optional clipper toggles start compose profiles through the host **setup-control** API (`:9191`, proxied at `/v1/setup-control/*`). Minute-level viewer charts require the scraper profile; core tier alone serves Helix/VOD summary only.
- The channel workspace is not just a player shell. It includes a **two-tier player overlay** (`LivePlayerControls` in `Channel.tsx`): a primary row (play, mute, volume, quality, **Theater**, fullscreen, **Settings**) and a secondary scrollable panel for latency, fit, density, and metrics when Settings is open. Theater toggles layout mode from the control bar — not a floating chip. **Comfort** / **Dense** density affects the lower workspace and chat chrome only; the default player frame stays 16:9 (`aspect-video`). Opening Settings must not shrink the video. On desktop, the center column (player + stream meta + tabs) scrolls as one unified column; chat stays fixed-height with its own internal scroll. Separate requested vs loaded quality state, diagnostics, LSF, and emote management remain in the lower workspace.
- **VOD playback:** synced historical streams can open in the channel workspace via Analytics **Play in Streamclone** (`?vod=&offset=`). Relay uses `POST /v1/stream/vod/start` (Streamlink/ffmpeg → `live/vod_{id}` HLS). See `.kiro/steering/clipper.md` for export/clip paths on the same VOD.
- Chat is read through the Chat Gateway, which joins Twitch IRC over WebSocket, parses IRCv3 tags, enriches messages, batches frames, and serves one WebSocket per client session.
- Custom emotes are managed by the Emote service, stored in PostgreSQL, rendered into WebP scales, served from MinIO/S3-compatible storage, and exposed to chat through Redis dictionaries.

## Product Guardrails

- Keep the project local-first and self-hostable with Docker Compose.
- Keep upstream endpoint details configurable through environment variables.
- Keep third-party platform compliance risk visible. This project is educational and personal self-hosting oriented; do not add language that implies Twitch or 7TV endorsement.
- Prefer graceful degradation over hard failure: stale metadata, structured stream errors, chat reconnects, and clear emote processing states.
- Keep video fallbacks server-side by default; do not use browser-side Twitch embeds unless a future spec explicitly changes the upstream-boundary guardrail.
- Viewer-facing flows should not require first-party accounts. Authenticated Twitch chat sending is optional and isolated to chat auth paths.
- Preserve the localhost token-import affordance and the emote provider controls when changing auth flows or the channel workspace layout.
- Keep playback controls honest to backend state. Requested quality and loaded quality are separate concepts, and metadata source/provider failures should stay visible instead of being silently collapsed into a generic empty state.

## Future Task Defaults

- Use the existing service boundaries before adding a new service.
- Prefer server-side enrichment and caching over browser-side upstream calls.
- For emote work, read `emote-pipeline.md` before changing schema, seed behavior, tokenizer behavior, or frontend rendering.
