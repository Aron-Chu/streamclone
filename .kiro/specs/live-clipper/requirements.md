# Requirements Specification: Standalone Live Clipper

## Introduction

The live clipper is a standalone local application for stream moment automation. It watches selected Twitch channels for chat velocity spikes, accepts explicit webhooks from tools such as Streamer.bot, asks Twitch Helix to create a clip when allowed, downloads the resolved clip media with Streamlink, and renders a ready-to-post vertical MP4 with optional captions.

The app is intentionally separate from Streamclone's viewer services. Streamclone can continue to browse, restream, chat, and render emotes without the clipper installed. The clipper can live in the same repository for development convenience, but it owns its own runtime, queue, local database, and artifacts.

### Scope

In scope:

- Local FastAPI service and dashboard for clipper operation.
- Twitch IRC chat velocity monitor over secure WebSocket.
- Streamer.bot-compatible webhook trigger.
- Twitch Helix clip creation and readiness polling.
- Streamlink clip download using authenticated viewing headers when configured.
- Faster-whisper transcription with empty-transcript fallback.
- FFmpeg vertical rendering, trimming, caption burn-in, and output retention.
- SQLite job persistence and duplicate suppression.

Out of scope for V1:

- Mutating Streamclone's core chat, video, metadata, emote, or frontend services.
- Continuous local video ring-buffer recording.
- Full automatic webcam detection, saliency tracking, or game-aware camera movement.
- Hosted rendering workers or multi-user production control plane.
- Uploading directly to TikTok, YouTube Shorts, Instagram, or other publishing APIs.

## Project Conventions

- The clipper SHALL remain local-first and self-hostable.
- All secrets SHALL be environment-driven and redacted from logs, filenames, dashboard responses, and SQLite display fields.
- The clipper SHALL prefer reliable defaults over high-compute features.
- The clipper SHALL fail jobs into explicit states instead of silently retrying forever.
- The clipper SHALL keep temporary media bounded by cleanup and retention policies.

## 1. Local Runtime

**User Story:** As an operator, I want to run the clipper locally without starting the full Streamclone stack, so that clip automation does not depend on the viewer app.

### Acceptance Criteria

1. THE clipper SHALL expose an HTTP API on configurable host and port, defaulting to loopback.
2. THE clipper SHALL persist job state in a local SQLite database.
3. THE clipper SHALL store media artifacts under a configurable output directory.
4. THE clipper SHALL run without Redis, PostgreSQL, MediaMTX, MinIO, Caddy, or the Streamclone frontend.
5. THE clipper SHALL expose a small dashboard listing watched channels, recent triggers, job states, failures, suppression counts, and available final MP4 files.
6. THE clipper SHALL load all configuration from environment variables and optional local config files.

## 2. Trigger Inputs

**User Story:** As an operator, I want both automatic chat-spike triggers and manual webhook triggers, so that the clipper can work with or without Streamer.bot.

### Acceptance Criteria

1. THE clipper SHALL accept `POST /v1/triggers/streamerbot` with channel, optional broadcaster id, optional duration, optional title, and reason.
2. WHEN a webhook token is configured, THE trigger endpoint SHALL reject unauthenticated requests.
3. THE clipper SHALL support adding and removing watched Twitch channels through local API endpoints.
4. THE IRC monitor SHALL connect only to `wss://irc-ws.chat.twitch.tv:443`.
5. WHEN Twitch sends `PING :tmi.twitch.tv`, THE monitor SHALL respond with `PONG :tmi.twitch.tv`.
6. IF the IRC connection drops, THE monitor SHALL reconnect with jitter and resubscribe channels that remain watched.
7. THE chat velocity detector SHALL use a rolling window with configurable minimum messages, spike multiplier, and cooldown.
8. THE detector SHALL record the trigger detection time and peak chat timestamp for trimming decisions.

## 3. Clip Creation

**User Story:** As an operator, I want the clipper to create Twitch clips when the current user is allowed to, so that capture happens without continuous local recording.

### Acceptance Criteria

1. THE clipper SHALL resolve channel login to broadcaster id when the trigger does not provide one.
2. THE clipper SHALL call Twitch Helix clip creation with the broadcaster id and configured user access token.
3. THE clipper SHOULD include title and duration when configured and supported by the endpoint.
4. THE clipper SHALL treat clip creation as asynchronous and poll for readiness by clip id.
5. IF the clip is not ready within the configured timeout, THE job SHALL fail as `clip_not_ready`.
6. IF Twitch reports the broadcaster is offline, clips are disabled, permissions are restricted, or the token is invalid, THE job SHALL fail with a specific visible error code.
7. THE clipper SHALL not require editor/channel management scopes for V1.

## 4. Download

**User Story:** As an operator, I want the clipper to download the created clip with the same viewing permission as my Twitch user when possible, so that gated clip pages fail less often.

### Acceptance Criteria

1. THE clipper SHALL download the ready clip URL with Streamlink using `best` quality by default.
2. WHEN a Twitch user token is configured, THE clipper SHALL pass it to Streamlink as `Authorization=Bearer <token>` through the Twitch plugin API header option.
3. IF authenticated Streamlink download fails, THE clipper MAY retry anonymously once.
4. THE clipper SHALL mark restricted or failed downloads with specific job states.
5. THE clipper SHALL keep downloaded raw files job-local and delete them after successful final render.

## 5. Transcription

**User Story:** As an operator, I want optional captions without making speech recognition a hard dependency for successful video output.

### Acceptance Criteria

1. THE clipper SHALL support enabling or disabling transcription.
2. THE default transcription backend SHALL be faster-whisper with `compute_type=int8`.
3. THE default model SHOULD be small enough for local CPU use while remaining configurable.
4. THE transcriber SHALL write timestamped caption data in a format renderable by FFmpeg.
5. IF transcription returns no words, THE renderer SHALL skip subtitle burn-in and continue.
6. IF transcription fails and captions are optional, THE job SHALL continue with a visible warning.

## 6. Rendering

**User Story:** As an operator, I want a vertical MP4 that focuses on the likely event moment and is ready for manual posting.

### Acceptance Criteria

1. THE renderer SHALL output `1080x1920` MP4 by default.
2. THE renderer SHALL support configurable source duration and final duration.
3. THE renderer SHALL apply a configurable event latency offset when trimming chat-triggered clips.
4. THE default layout SHALL use a blurred background and a centered source crop.
5. THE renderer SHALL burn ASS captions only when a non-empty caption file exists.
6. THE renderer SHALL escape subtitle paths before inserting them into FFmpeg filter arguments.
7. THE renderer SHALL preserve audio in the final MP4.
8. THE default encoder SHALL be `libx264`; hardware encoders SHALL be opt-in.

## 7. Queue And Duplicate Safety

**User Story:** As an operator, I want hype trains and webhook bursts to avoid flooding my local renderer with duplicate jobs.

### Acceptance Criteria

1. THE clipper SHALL run one render job at a time by default.
2. THE clipper SHALL reject or suppress duplicate triggers for the same broadcaster inside a configurable duplicate window.
3. THE duplicate check SHALL consider queued, running, and recently successful jobs.
4. THE clipper SHALL track suppressed trigger counts for dashboard visibility.
5. THE clipper SHALL keep chat cooldown separate from duplicate suppression.
6. THE job state machine SHALL survive process restart through SQLite persistence.

## 8. Retention And Disk Safety

**User Story:** As an operator, I want clip automation to avoid filling my disk during a long stream.

### Acceptance Criteria

1. THE clipper SHALL delete raw input files after successful render.
2. THE clipper SHALL run periodic cleanup for orphaned temp files.
3. THE clipper SHALL purge final MP4 files older than a configurable retention period.
4. THE clipper SHALL keep historical SQLite job rows after artifact purge.
5. THE dashboard SHALL distinguish available artifacts from purged artifacts.

## 9. Hosting

**User Story:** As an operator, I want clear rules for when to host pieces of the clipper, so that costs stay low.

### Acceptance Criteria

1. THE default deployment SHALL be local-only.
2. IF remote dashboard or webhook access is needed, THE recommended first step SHALL be Cloudflare Tunnel or Tailscale with the renderer still local.
3. IF final clips need remote sharing, THE clipper MAY upload final MP4 files to cheap object storage such as Cloudflare R2 or Backblaze B2.
4. Hosted rendering SHALL remain out of V1 unless a future spec defines cost controls, GPU/CPU sizing, token storage, and artifact lifecycle.

## 10. Testing

**User Story:** As a maintainer, I want focused tests around platform failures and media edge cases, so that the local app is trustworthy during live streams.

### Acceptance Criteria

1. Unit tests SHALL cover Helix success, async timeout, offline channel, clip restrictions, invalid token, and missing scope.
2. Unit tests SHALL cover IRC PING/PONG, reconnect, spike detection, quiet streams, cooldown, and duplicate suppression.
3. Unit tests SHALL cover empty transcript behavior.
4. Unit tests SHALL cover FFmpeg command construction and subtitle path escaping.
5. An integration-style fixture test SHALL render a tiny video and verify `1080x1920` output with audio.
