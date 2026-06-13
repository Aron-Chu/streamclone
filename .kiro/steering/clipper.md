# Live Clipper Steering

## Purpose

The live clipper is a standalone local automation app for detecting stream moments, creating Twitch clips, downloading the resulting media, and rendering short vertical MP4s with optional captions.

It is adjacent to Streamclone, not part of the core viewer loop. Streamclone remains a self-hosted directory, playback, chat, and emote platform. The clipper may reuse project conventions and implementation ideas, but it should not make Streamclone's viewer services depend on clip creation, transcription, or vertical rendering.

## App Boundary

- Keep the clipper **worker and API** in `clipper/` — own queue, SQLite, render pipeline, and Helix clip creation.
- Do not route clip creation, transcription, or FFmpeg rendering through `cmd/chat`, `cmd/video`, or `cmd/metadata`.
- Keep the clipper runnable without Redis, PostgreSQL, MediaMTX, or MinIO (SQLite + local disk by default).

### Streamclone UI integration (current)

These frontend surfaces are **allowed** — they are thin clients over the clipper HTTP API, not duplicates of clipper logic:

| Surface | Path / file | Role |
|---------|-------------|------|
| Clip Studio | `/studio/:jobId` → `ClipStudio.tsx` | Trim, captions, templates, export |
| Analytics clips | `Analytics.tsx` tabs | List jobs, queue from graph, link to studio; **Export Moment** for synced historical VODs (`trigger_type: vod_export`) |
| VOD playback | `Channel.tsx` + `POST /v1/stream/vod/start` | Relay archived broadcasts locally via Streamlink/ffmpeg (`live/vod_{id}` HLS path); Analytics **Play in Streamclone** deep-links with `?vod=&offset=` |
| Caddy proxy | `/v1/clipper/*` → `clipper:8095` | Same-origin API (`VITE_CLIPPER_URL: auto`) |

Do not move job state, rendering, or Helix writes into Go viewer services. New clipper features belong in `clipper/liveclipper/` first; add UI in `ClipStudio.tsx` / `frontend/src/components/clipStudio/` / `Analytics.tsx` only as API consumers.

### Upstream client duplication

Clipper currently reimplements Twitch clients that also exist in Go services:

| Clipper module | Go equivalent | Notes |
|----------------|---------------|-------|
| `clipper/liveclipper/twitch.py` | `internal/metadata/helix`, `internal/analytics/helix` | Separate Helix token refresh and clip APIs |
| `clipper/liveclipper/irc.py` | `internal/chat/ircconn` (chat + analytics) | Third IRC parser alongside Go `parse` |
| `clipper/liveclipper/emote_overlay.py` | emote CDN (`/emotes/{id}/1x.webp`) | Downloads pre-rendered URLs; does not call Go emote service |

When extending clipper, prefer consuming Go stack surfaces (webhook/SSE from chat for spikes, proxied Helix via shared client) over adding a fourth IRC or Helix implementation.
- Store clipper job state in local SQLite by default. The durable source of truth for clipper jobs is the clipper database, not Streamclone metadata cache or chat pub/sub.
- Keep render artifacts under a configurable local output directory. Object storage upload is optional and should be added as a later distribution concern, not as a V1 dependency.

## Twitch And Streamer.bot Guardrails

- Use Twitch Helix for clip creation. The creating token must be a user access token with the required clip creation scope, and the `Client-Id` header must match the token's client.
- Treat Helix clip creation as asynchronous. Poll the created clip before attempting download, and fail clearly if the clip does not become available within the configured timeout.
- Use Streamer.bot only as an optional trigger source. The clipper must also support its own Twitch IRC velocity monitor so it remains useful without Streamer.bot.
- The native IRC monitor must connect to `wss://irc-ws.chat.twitch.tv:443`, request tags/commands when needed, and respond to `PING :tmi.twitch.tv` with `PONG :tmi.twitch.tv`.
- Account for chat-to-stream latency. Chat spikes normally arrive several seconds after the on-stream moment; the clipper should record trigger timestamps and apply a configurable event latency offset before final trimming.
- Surface Twitch platform limitations directly: offline channels, disabled clips, follower/subscriber-only clip restrictions, invalid or under-scoped tokens, and clip download failures should be visible job states.
- **Historical VOD export:** when Analytics provides `moment_context.vod_id`, the worker downloads a segment with `clipper/liveclipper/vod.py` (streamlink URL resolve + ffmpeg window) instead of Helix `POST /clips`. Live channels still use Helix clip creation.

## Streamlink And FFmpeg Guardrails

- Use Streamlink to download the resolved clip URL for V1 "any allowed channel" coverage.
- Pass the user's Twitch viewing token to Streamlink with the Twitch plugin authorization header format `Authorization=Bearer <token>` when configured, then retry anonymous only when that is safe and useful.
- Keep Streamlink and FFmpeg invocations argument-array based. Avoid shell-string assembly for paths, tokens, URLs, and filter graphs.
- Escape subtitle paths before injecting them into FFmpeg filter arguments. Prefer job-local paths without spaces or Windows drive syntax when possible.
- If Whisper returns no transcript, render the video without a subtitle filter rather than burning an empty or invalid ASS file.
- Prefer CPU-safe defaults: `libx264` video encoding and faster-whisper `compute_type=int8`. Hardware encoders can be enabled through configuration after local validation.

## Token And Secret Rules

- Never put Twitch access tokens, refresh tokens, webhook tokens, or Streamlink authorization headers in URLs, filenames, logs, rendered dashboard HTML, or SQLite text fields intended for user display.
- Keep secrets environment-driven. Do not hardcode Twitch credentials, webhook shared secrets, object-storage credentials, or deployment hostnames.
- Validate webhook requests with a shared token before queueing jobs.
- Redact subprocess command logs before display. A job should show that a Twitch auth header was used, never the value.

## Queue, Duplicate, And Disk Safety

- Use a single render worker by default. Rendering is CPU/GPU heavy and should not compete aggressively with a gaming or streaming PC.
- Add duplicate suppression before queue insertion. If the same broadcaster already has a queued, running, or recently successful job inside the duplicate window, suppress the new trigger and record the suppression count.
- Keep chat-trigger cooldown separate from duplicate suppression. Cooldown prevents repeated auto-triggers; duplicate suppression protects the queue from webhook or reconnect bursts.
- Delete raw downloaded inputs after successful render.
- Run periodic cleanup for orphaned temporary files and old final MP4 files. Preserve SQLite job history after artifact purge.
- Make retention configurable, with a local-first default such as 48 hours for finished MP4s.

## Hosting Defaults

- V1 should host nothing by default. Run capture, transcription, and rendering locally.
- Use Cloudflare Tunnel or Tailscale only when remote webhook/dashboard access is needed. Avoid opening public inbound ports.
- Use Cloudflare R2 or Backblaze B2 only when final clips need cheap remote sharing or backup.
- Do not move rendering to cloud until local rendering blocks streaming, the local machine is often unavailable, or multiple editors/channels depend on the workflow. Hosted rendering changes the cost model and requires a separate deployment spec.
- By default, the clipper dashboard binds to `127.0.0.1`. In virtualized environments like WSL2, set `CLIPPER_HOST=0.0.0.0` in the parent `.env` file to bind to all interfaces, allowing access from the host machine's web browser.

## Clip Studio (frontend)

- Route: `frontend/src/App.tsx` → `/studio/:jobId`.
- API base: `CLIPPER` in `frontend/src/config.ts` (auto → same origin `/v1/clipper`).
- Job states: `queued` → `creating_clip` | `downloading` (VOD export skips Helix) → `transcribing` → `rendering` → `ready` | `failed`.
- After queueing from Analytics, follow **Open in Studio** or `/studio/{jobId}` directly.

## Historical VOD export (shipped)

- **Trigger:** `trigger_type: vod_export` when `moment_context.vod_id` is set (Analytics **Export moment** on synced historical streams).
- **Download:** `clipper/liveclipper/vod.py` resolves the Twitch VOD URL via Streamlink `-j`, then `ffmpeg -ss/-t` into `raw_path` — no Helix `POST /clips`.
- **Trim:** `render.compute_trim_start` centers on `vod_offset_seconds` using `vod_segment_start` from the downloaded window.
- **Dedup:** SQLite duplicate window matches `vod_id` + offset within 60s (not broadcaster-only).
- **Live path unchanged:** live analytics still uses Helix clip create → Streamlink clip URL download.

## Task Checklist

- Read `AGENTS.md`, this file, `.kiro/specs/live-clipper/requirements.md`, `.kiro/specs/live-clipper/design.md`, and `memories/repo/live-clipper-ast-graph.md` before changing clipper behavior.
- Decide whether the task affects triggers, Twitch clip creation, download, transcription, rendering, dashboard, storage, hosting, or job retention.
- Keep Streamclone viewer services untouched unless the task explicitly asks for integration.
- Add narrow tests for the changed layer, especially Helix failure states, IRC reconnect/PING behavior, duplicate suppression, empty transcript rendering, and path escaping.
- For docs-only changes, no build is required unless the docs describe a command or generated artifact that should be verified.
