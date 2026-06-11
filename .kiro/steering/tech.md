# Technical Steering

Agents: start at `AGENTS.md`, then this file. Use codegraph MCP (`make codegraph`) for symbol lookup.

## Stack

- Backend: Go services using `net/http`, chi, `coder/websocket`, Redis, PostgreSQL via pgx, and slog.
- Frontend: Vite, React, TypeScript, TailwindCSS, hls.js, and Zustand.
- Infra: Docker Compose, Redis, PostgreSQL, MinIO/S3-compatible object storage, MediaMTX, a Caddy reverse proxy, and a standalone Playwright-based python scraper. Use the standard `caddy:2` image in compose; `caddy:2-alpine` produced intermittent Docker DNS websocket failures in this repo.
- HLS: MediaMTX 1.18+ session cookies break plain HTTP localhost unless `hlsCDNSecret` in `deploy/mediamtx.yml` matches the `Authorization: Bearer` header Caddy sends on `/live/*` — see `.kiro/steering/playback.md`.
- Image pipeline: libvips CLI through `vips thumbnail`, producing WebP variants `1x`, `2x`, `3x`, and `4x`.

## Runtime boundaries

| Runtime | Role | Heavy work delegated to |
|---------|------|-------------------------|
| Go (`cmd/*`) | HTTP/WS services, Redis/Postgres glue | Streamlink, FFmpeg, MediaMTX, `vips` CLI |
| Python (`clipper/`) | Clip jobs, ASR, vertical render | FFmpeg, Streamlink, faster-whisper |
| Python (scraper sibling) | Browser scrape for Tracker/Reddit | Camoufox/Chromium CDP |
| Browser (React) | Directory, player, chat UI | hls.js |

Go services are I/O-bound glue around upstream Twitch loops and external media tools. Consolidate duplicate IRC/Helix clients in Go before introducing new languages. See `.kiro/steering/playback.md` (HLS subprocess map) and `.kiro/steering/analytics.md` (chat IRC sharing).

## Local Commands

- Human setup guide: `docs/install-desktop.md` (options: `docs/options.md`)
- Backend build: `make build`
- Backend tests: `make test`
- Backend vet: `make vet`
- Full stack: `make up`
- Stop stack (keep data): `make down` or `scripts/stop-streamclone.ps1` / **Stop Streamclone** launcher
- Remove volumes only: `make down-clean`
- Complete uninstall: `scripts/uninstall-streamclone.ps1` / **Uninstall Streamclone** launcher
- Migrations: `make migrate`
- Instant local Twitch login: `make twitch-local-auth`
- Frontend dev from WSL: `cd /mnt/c/Users/Aron/twitch-7tv-clone/frontend && npm run dev -- --host 127.0.0.1 --port 5174`
- Frontend build: `cd frontend && npm run build`
- Scraper Dashboard Web UI: `http://localhost:8000/`

## Code Conventions

- Backend code belongs in the existing `cmd/*` service entrypoints and `internal/*` packages.
- Keep code comments out of source unless the user explicitly asks for comments. Explain behavior in docs instead.
- Keep configs environment-driven. Do not hardcode secrets, upstream credentials, or deployment-specific hostnames.
- Local frontend, auth, chat, and HLS should default to the single-origin proxy at `http://localhost:8090`; do not point the browser at `5174`, `8081`, `8082`, or `8083` unless intentionally bypassing the proxy for development.
- The nginx frontend image generates `/config.js` from `VITE_*` runtime env vars so API origins can change without rebuilding.
- On the proxied local stack, keep frontend runtime URLs on same-origin/`auto` values so `GET /v1/me`, chat cookies, and websocket URLs stay aligned.
- The localhost-only token import button is backend-driven through `GET /v1/me` and `canImportLocalToken`; do not treat it as a frontend-only feature flag.
- Prepared local Twitch login uses one-time backend claims; do not pass OAuth access or refresh tokens through URLs.
- Playback quality UI should use backend-discovered requestable renditions once available and show loaded backend quality separately from the user's requested target.
- Keep channel names, IDs, and client input validated at service boundaries.
- Prefer bounded queues, context cancellation, retry caps, and backoff for anything involving upstream networks or worker loops.
- Reddit LSF is multi-provider and uses an automatic prioritized fallback chain. Even if a specific primary provider (like `firecrawl` or `third_party`) is selected via `REDDIT_PROVIDER` in `.env`, the metadata service will automatically fall back to other available providers (such as public JSON or HTML scraping) if the primary is blocked or fails. Setting `REDDIT_PROVIDER=auto` tries all providers in order of availability. Blocked or erroring providers back off for about 45 seconds before retry.

## Data Conventions

- PostgreSQL is the durable source for emotes, emote sets, channel mappings, and processing jobs.
- Redis is the hot cache and pub/sub bus for metadata, chat fan-out, and per-channel emote dictionaries.
- Object storage keys for rendered emotes are `{emote_id}/{scale}.webp`; source uploads are `{emote_id}/src`.
- Redis emote dictionaries use `channel:emotes:{login}` with field `emote_name` and JSON value `{"u":"url","zw":false}`.
- Redis emote deltas publish to `emotes:delta:{login}` and chat delivery publishes to `chat:{login}`.

## Testing Defaults

- For backend changes, run the narrow package tests first, then `go test ./...` when the change crosses package boundaries.
- For frontend changes, run `npm run build` from `frontend/`.
- For local proxy, auth, or chat transport changes, validate `docker compose ... config`, `http://localhost:8090/v1/auth/debug`, and a websocket subscribe through `ws://localhost:8090/v1/ws`.
- For channel playback/UI changes, verify against the proxied local bundle at `http://localhost:8090`, not just the standalone Vite dev server.
- For docs-only changes, no build is required unless the docs describe a command or generated artifact that should be verified.
