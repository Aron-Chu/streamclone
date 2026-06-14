# Implementation Plan: Open-Source Streaming & Chat Platform Clone

## Overview

Milestone-ordered, incremental task list derived from `design.md`. Each task is a discrete
coding step that builds on the previous ones; every task cites the requirements and design
sections it satisfies. All code is comment-free (PC-1). Backend is Go; frontend is Vite + React + TS (PC-2).

## Tasks

- [x] 1. Initialize the monorepo layout
  - Create a single Go module `streamclone` with `cmd/{metadata,video,chat,emote}` entrypoints and shared `internal/{config,log,httpx,upstream}` packages
  - Create `deploy/`, `migrations/`, and (later) `frontend/` directories
  - Add `.gitignore`, `.dockerignore`, `.env.example`, and a top-level `Makefile` with `up`, `down`, `migrate`, `test`, `build`, `tidy`

- [x] 2. Author the Docker Compose infrastructure
  - Define `redis`, `postgres`, `minio`, `mediamtx` services with named volumes and ports
  - Mount `deploy/mediamtx.yml`; wire healthchecks and a shared network

- [x] 3. Build the shared Go foundation package
  - Config loader using `caarlos0/env` reading the env vars in the §9 table
  - `slog` JSON logger factory with a correlation-id field helper
  - `chi` server bootstrap exposing `/healthz`, `/readyz`, `/metrics` (Prometheus registry)
  - Graceful shutdown on SIGINT/SIGTERM

- [x] 4. Set up migration tooling
  - Add the `migrate` one-shot service running `golang-migrate` against `DATABASE_URL`
  - Create an empty initial migration pair to validate up/down wiring

- [x] 5. Centralize upstream contract configuration
  - Single `upstream` config module holding GQL/Usher/IRC/7TV/CDN endpoints and the public Client-ID
  - All values overridable by env so contract drift is a one-place change

- [x] 6. Configure MediaMTX with a bounded HLS ring buffer
  - Author `deploy/mediamtx.yml` with `hlsSegmentCount: 5`, `hlsSegmentDuration: 1s`, publisher path `~^live/.*$`

- [x] 7. Implement the GQL playback-token client
  - POST `PlaybackAccessToken_Template` with Client-ID + browser UA headers; decode `value`/`signature`
  - Raise `ErrUpstreamSchema` on shape mismatch; unit-test against a recorded fixture

- [x] 8. Implement the Usher client and rendition parser
  - GET the master `.m3u8` with token/sig query params; parse variants into a rendition list (name, resolution, framerate)
  - Unit-test the m3u8 parser with a fixture

- [x] 9. Implement the Stream Worker with process-group isolation
  - Validate channel against `^[a-z0-9][a-z0-9_]{2,24}$` before exec
  - Spawn `streamlink ... --stdout` piped to `ffmpeg -c copy -f flv rtmp://mediamtx/live/{channel}` in a new process group (`Setpgid`)
  - Provide `killTree(pgid)` via `syscall.Kill(-pgid, SIGKILL)`

- [x] 10. Build the session registry and reaper
  - Registry maps channel → {PID, PGID, listeners (atomic), lastSeen (atomic), startedAt}
  - Reaper ticks every 10s, killing sessions with `listeners == 0` and idle beyond `STREAM_IDLE_TIMEOUT`
  - On startup, reconcile registry with live PIDs and kill untracked stream processes

- [x] 11. Expose the Video Orchestrator HTTP API
  - `POST /v1/stream/start` (dedupe existing session, concurrency semaphore `MAX_CONCURRENT_STREAMS`, return hls_url + renditions)
  - `POST /v1/stream/keepalive`, `POST /v1/stream/stop`, `GET /v1/stream/status`
  - Bounded automatic restarts on unexpected worker exit while listeners remain
  - Structured error (no worker spawned) when token/usher fails

- [x] 12. Implement the GQL client and HeaderProvider
  - `HeaderProvider` supplying Client-ID, browser UA, and optional `Client-Integrity`/`X-Device-Id`
  - `Refresh()` + single retry on `403`/integrity challenge before fallback

- [x] 13. Build the Redis cache layer with stale fallback
  - Namespaced keys per §6.2; write fresh (TTL) + `:stale` (long TTL) copies
  - Serve `:stale` annotated when upstream fails; structured error when no cache exists
  - Degrade gracefully if Redis is unavailable

- [x] 14. Add request coalescing
  - Wrap upstream calls in `singleflight` keyed by cache key
  - Document the node-local limitation and the multi-node Redis path (note only, not built)

- [x] 15. Implement directory, category, and search endpoints
  - `GET /v1/streams`, `/v1/categories`, `/v1/categories/{id}/streams`, `/v1/search` with pagination/cursor
  - Normalize thumbnails to a width/height-substitutable URL template
  - Validate/bound query params (limit, cursor, query length)

- [x] 16. Implement channel-id resolution
  - `GET /v1/channels/{login}` resolving login → twitch id, cached as `meta:channelid:{login}`

- [x] 17. Capture and pin live GQL operations
  - Record the directory/category/search persisted-query hashes (or inline queries) from a live session into the `upstream` config/fixtures

- [x] 18. Implement the anonymous IRC connection with socket cap
  - Connect `wss://irc-ws.chat.twitch.tv`, handshake `PASS SCHMOOPIIE` / `NICK justinfan{rand}` / `CAP REQ :twitch.tv/tags twitch.tv/commands`
  - Respond to `PING` with `PONG`
  - Connection manager enforcing `MAX_CHANNELS_PER_SOCKET` (default 30), spinning up a new socket at capacity

- [x] 19. Implement the IRCv3 parser
  - Decode tags (`@k=v;...`), prefix, command, trailing into a struct: user, color, display-name, badges, ts, text
  - Tolerate unknown tags and non-PRIVMSG commands (`ROOMSTATE`, `USERNOTICE`, membership)
  - Unit-test against recorded IRC line fixtures

- [x] 20. Implement room management and reconnect
  - Single upstream room subscription per channel shared across subscribers
  - PART the room after a grace period once the last subscriber leaves
  - Exponential backoff + jitter reconnect, rejoining channels that still have subscribers

- [x] 21. Build the WS Hub with persistent per-session sockets
  - One `coder/websocket` per client session; maintain `conn↔channel` maps
  - Handle `subscribe`/`unsubscribe` control frames (JOIN on first subscriber, PART on last)
  - Bounded per-connection send queues that drop oldest on overflow
  - Sanitize/encode message content so payloads cannot inject markup

- [x] 22. Implement the batcher and Redis pub/sub seam
  - Publish parsed messages to `chat:{channel}`; Hub subscribes only for channels it serves
  - Accumulate per channel for `BATCH_WINDOW_MS` (50–100ms) and flush as one `batch` frame

- [x] 23. Write the PostgreSQL schema migrations
  - Migration creating `emotes`, `emote_sets`, `emote_set_items`, `channels`, `processing_jobs` with the §6.1 enums, flags, FKs, and indexes (including the partial `idx_jobs_claimable`)

- [x] 24. Implement the object-storage client
  - S3-compatible client (MinIO/R2/S3) selected via `S3_ENDPOINT`/credentials
  - `Put`/`Delete` under the `/emotes/{id}/{scale}.webp` key layout

- [x] 25. Implement the libvips asset worker
  - Claim jobs with `SELECT ... FOR UPDATE SKIP LOCKED`
  - For each scale {1x:32, 2x:64, 3x:96, 4x:128} resize by height and export WebP, preserving animation/alpha
  - All-or-nothing: flip emote `active` only when all scales succeed; on failure record `last_error`, retry to a cap, leave no partial objects
  - Idempotent by `(emote_id, source_hash)`

- [x] 26. Implement the curator API with auth
  - `CURATOR_API_TOKEN` bearer middleware on all write routes
  - `POST /v1/emotes` (multipart: validate type/size, hash, insert pending emote + queued job)
  - `POST /v1/sets`, `POST /v1/sets/{id}/items` (alias), `DELETE /v1/sets/{id}/items/{emote_id}`, `PUT /v1/channels/{twitch_id}/active-set`

- [x] 27. Implement the 7TV seeder
  - `POST /v1/seed/twitch/{twitch_id}` → fetch `7tv.io/v3/users/twitch/{id}`, resolve active set + emotes
  - Download originals from `cdn.7tv.app`, reprocess through the asset worker, upsert idempotently
  - Flag global emotes `is_global = true`

- [x] 28. Implement the dictionary builder and delta publisher
  - On set/item/active-set change, rebuild `channel:emotes:{login}` (hash field=name, value `{u,zw}`)
  - Publish `emotes:delta:{channel}` add/remove events
  - On emote delete, remove rows and orphan-collect objects per policy

- [x] 29. Implement the Trie tokenizer
  - Build a per-channel `Trie` from the Redis dictionary; whole-word match with `zw` flag
  - `tokenize` emits ordered `text`/`emote` fragments whose contents round-trip the original text/spacing
  - Unit-test fragment round-trip and zero-width flagging

- [x] 30. Implement the atomic dictionary swap
  - Hold the `Trie` behind `atomic.Pointer`; lock-free reads, race-free `Swap`
  - Lazy-load + build the dictionary on first subscribe; rebuild from PostgreSQL on Redis miss

- [x] 31. Implement the debounced delta consumer
  - Subscribe to `emotes:delta:{channel}`; debounce `DELTA_DEBOUNCE_MS` (default 300ms) to coalesce burst edits into one rebuild-and-swap

- [x] 32. Wire enrichment into the gateway pipeline
  - Replace the M3 plain-text path: tokenize each parsed message, attach `fragments`, then publish to `chat:{channel}`
  - Forward `emote_delta` and `status`/`error` frames to subscribed connections

- [x] 33. Scaffold the Vite + React + TS app
  - Vite project, TailwindCSS, `@tanstack/react-query` provider, `zustand` stores, base layout/routing
  - `CDN_PUBLIC_BASE` and service base URLs from env

- [x] 34. Build the directory, category, and search views
  - Directory grid (thumbnails, title, category, viewer count) via the Metadata API
  - Category navigation and search input backed by the metadata endpoints

- [x] 35. Build the HLS player with keep-alive
  - On channel select, `POST /stream/start`, play the local HLS URL via `hls.js`
  - Periodic keep-alive while playing; stop player + cease keep-alive + send `unsubscribe` on leave
  - Error/retry affordance on start failure or stall

- [x] 36. Build the persistent chat socket client
  - One persistent `WebSocket` per session; `subscribe`/`unsubscribe` on navigation
  - Reconnect with backoff, re-`subscribe` current channel, show disconnected/reconnecting state

- [x] 37. Build the virtualized emote chat list
  - `@tanstack/react-virtual` list; render `fragments` (text → text node, emote → `<img>` with name alt; zero-width overlaps previous)
  - Enforce `MAX_RETAINED_MESSAGES` (default 200) rolling buffer, shifting oldest on overflow
  - Apply incoming `emote_delta` to the client emote view

- [x] 38. Add metrics and correlation across services
  - Register the §10.1 metrics (streams_active, listeners, reaped, chat in/out, tokenize histogram, cache hit/miss, upstream result, asset jobs)
  - Propagate a correlation id through logs across service hops; log spawn/reap and integrity/schema errors at alertable severity

- [x] 39. Add rate limiting and input validation middleware
  - Per-IP token bucket on `stream/start`, `search`, and WS connect
  - Centralized validation for channel names, pagination, search length, upload type/size

- [x] 40. Add timeouts, retries, and circuit breakers
  - Context deadlines on all upstream/inter-service calls; backoff + circuit breaker per upstream
  - Open breaker on metadata serves `:stale`; verify per-channel failure isolation

- [x] 41. Write the unit and contract test suites
  - Unit: tokenizer round-trip, reaper rule, header rotation, cache stale fallback, IRC parser, usher parser, scale count
  - Contract: replay recorded GQL/Usher/IRC/7TV fixtures against parsers

- [x] 42. Write integration and load tests
  - Testcontainers (PostgreSQL/Redis/MinIO): upload→active and set-change→delta
  - Load: synthetic high-velocity chat asserting bounded memory and batch latency under cap

- [x] 43. Write the README and operator docs
  - Setup/run via Docker Compose, env var reference, the anonymous (auth-less viewer) design note, and curator-auth setup
  - Prominent legal/ToS disclaimer: educational/personal self-hosting; operator is responsible for compliance (C-1)

## Task Dependency Graph

```json
{
  "waves": [
    { "tasks": [1] },
    { "tasks": [2, 3, 5] },
    { "tasks": [4, 6, 12, 18, 24, 33] },
    { "tasks": [7, 13, 19, 23, 25] },
    { "tasks": [8, 14, 20, 26, 27] },
    { "tasks": [9, 15, 21, 28] },
    { "tasks": [10, 16, 17, 22, 29, 34, 35] },
    { "tasks": [11, 30, 36] },
    { "tasks": [31, 37] },
    { "tasks": [32] },
    { "tasks": [38] },
    { "tasks": [39, 40, 43] },
    { "tasks": [41] },
    { "tasks": [42] }
  ]
}
```

## Notes

- Tasks 1–43 are marked complete in this checklist; integration/load tests live in `internal/integration/`.
- Run `make integration-up` then `INTEGRATION=1 go test ./internal/integration/ -v` for container-backed tests.
- `TestUploadToActive` requires the `vips` CLI (libvips) on the test runner.
