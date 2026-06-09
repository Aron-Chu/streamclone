# Implementation Plan: Open-Source Streaming & Chat Platform Clone

Milestone-ordered, incremental task list derived from `design.md`. Each task is a discrete
coding step that builds on the previous ones; every task cites the requirements and design
sections it satisfies. The milestone order follows the roadmap (Video Core → Metadata → Chat →
7TV → Integration), bracketed by Foundation first and Frontend/Hardening last so each milestone
ends in something demonstrable.

Conventions:
- All code is comment-free (PC-1). Backend is Go; frontend is Vite + React + TS (PC-2).
- Reference key: `_Requirements: N.x_` maps to `requirements.md`; `_Design: §x_` maps to `design.md`.
- Each milestone closes with a Demo line stating the observable outcome.

---

## Milestone 0 — Foundation & Scaffolding

- [x] 0.1 Initialize the monorepo layout
  - Create a single Go module `streamclone` with `cmd/{metadata,video,chat,emote}` entrypoints and shared `internal/{config,log,httpx,upstream}` packages
  - Create `deploy/`, `migrations/`, and (later) `frontend/` directories
  - Add `.gitignore`, `.dockerignore`, `.env.example`, and a top-level `Makefile` with `up`, `down`, `migrate`, `test`, `build`, `tidy`
  - _Requirements: PC-2, PC-3, 9.1_ · _Design: §2, §3, §9_

- [x] 0.2 Author the Docker Compose infrastructure
  - Define `redis`, `postgres`, `minio`, `mediamtx` services with named volumes and ports
  - Mount `deploy/mediamtx.yml`; wire healthchecks and a shared network
  - _Requirements: 9.1, 9.6_ · _Design: §9_

- [x] 0.3 Build the shared Go foundation package
  - Config loader using `caarlos0/env` reading the env vars in the §9 table
  - `slog` JSON logger factory with a correlation-id field helper
  - `chi` server bootstrap exposing `/healthz`, `/readyz`, `/metrics` (Prometheus registry)
  - Graceful shutdown on SIGINT/SIGTERM
  - _Requirements: 9.2, 10.1, 10.2_ · _Design: §2, §10.1_

- [x] 0.4 Set up migration tooling
  - Add the `migrate` one-shot service running `golang-migrate` against `DATABASE_URL`
  - Create an empty initial migration pair to validate up/down wiring
  - _Requirements: 9.3_ · _Design: §6.1, §9_

- [x] 0.5 Centralize upstream contract configuration
  - Single `upstream` config module holding GQL/Usher/IRC/7TV/CDN endpoints and the public Client-ID
  - All values overridable by env so contract drift is a one-place change
  - _Requirements: 8.1, 8.4_ · _Design: §4, §9_

Demo: `make up` brings up all infrastructure; each (stub) service answers `/healthz` and exposes
`/metrics`; `make migrate` applies the baseline migration cleanly.

---

## Milestone 1 — Video Core

- [x] 1.1 Configure MediaMTX with a bounded HLS ring buffer
  - Author `deploy/mediamtx.yml` with `hlsSegmentCount: 5`, `hlsSegmentDuration: 1s`, publisher path `~^live/.*$`
  - _Requirements: 2.4, 2.15_ · _Design: §7.4_

- [x] 1.2 Implement the GQL playback-token client
  - POST `PlaybackAccessToken_Template` with Client-ID + browser UA headers; decode `value`/`signature`
  - Raise `ErrUpstreamSchema` on shape mismatch; unit-test against a recorded fixture
  - _Requirements: 2.1, 8.1, 8.3_ · _Design: §3.2, §4.1_

- [x] 1.3 Implement the Usher client and rendition parser
  - GET the master `.m3u8` with token/sig query params; parse variants into a rendition list (name, resolution, framerate)
  - Unit-test the m3u8 parser with a fixture
  - _Requirements: 2.2, 2.6_ · _Design: §3.2, §4.2_

- [x] 1.4 Implement the Stream Worker with process-group isolation
  - Validate channel against `^[a-z0-9][a-z0-9_]{2,24}$` before exec
  - Spawn `streamlink ... --stdout` piped to `ffmpeg -c copy -f flv rtmp://mediamtx/live/{channel}` in a new process group (`Setpgid`)
  - Provide `killTree(pgid)` via `syscall.Kill(-pgid, SIGKILL)`
  - _Requirements: 2.3, 2.9, 12.7, 13.4_ · _Design: §3.2, §7.2_

- [x] 1.5 Build the session registry and reaper
  - Registry maps channel → {PID, PGID, listeners (atomic), lastSeen (atomic), startedAt}
  - Reaper ticks every 10s, killing sessions with `listeners == 0` and idle beyond `STREAM_IDLE_TIMEOUT`
  - On startup, reconcile registry with live PIDs and kill untracked stream processes
  - _Requirements: 2.7, 2.8, 2.10, 2.14, 13.4, 13.5_ · _Design: §3.2, §7.2_

- [x] 1.6 Expose the Video Orchestrator HTTP API
  - `POST /v1/stream/start` (dedupe existing session, concurrency semaphore `MAX_CONCURRENT_STREAMS`, return hls_url + renditions)
  - `POST /v1/stream/keepalive`, `POST /v1/stream/stop`, `GET /v1/stream/status`
  - Bounded automatic restarts on unexpected worker exit while listeners remain
  - Structured error (no worker spawned) when token/usher fails
  - _Requirements: 2.5, 2.11, 2.12, 2.13, 13.1, 13.3_ · _Design: §3.2, §8.1_

Demo: `POST /v1/stream/start` for a live channel returns an HLS URL playable in a plain
`<video>`/hls.js page; stopping keep-alive reaps the worker within the timeout and leaves no
orphaned `streamlink`/`ffmpeg` processes.

---

## Milestone 2 — Metadata Service

- [x] 2.1 Implement the GQL client and HeaderProvider
  - `HeaderProvider` supplying Client-ID, browser UA, and optional `Client-Integrity`/`X-Device-Id`
  - `Refresh()` + single retry on `403`/integrity challenge before fallback
  - _Requirements: 1.5, 1.8, 8.2_ · _Design: §3.1, §4.1_

- [x] 2.2 Build the Redis cache layer with stale fallback
  - Namespaced keys per §6.2; write fresh (TTL) + `:stale` (long TTL) copies
  - Serve `:stale` annotated when upstream fails; structured error when no cache exists
  - Degrade gracefully if Redis is unavailable
  - _Requirements: 1.6, 1.7, 1.9, 1.10, 7.1, 7.4, 7.5_ · _Design: §3.1, §6.2, §10.3_

- [x] 2.3 Add request coalescing
  - Wrap upstream calls in `singleflight` keyed by cache key
  - Document the node-local limitation and the multi-node Redis path (note only, not built)
  - _Requirements: 1.11_ · _Design: §3.1_

- [x] 2.4 Implement directory, category, and search endpoints
  - `GET /v1/streams`, `/v1/categories`, `/v1/categories/{id}/streams`, `/v1/search` with pagination/cursor
  - Normalize thumbnails to a width/height-substitutable URL template
  - Validate/bound query params (limit, cursor, query length)
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 12.3_ · _Design: §3.1, §4.1_

- [x] 2.5 Implement channel-id resolution
  - `GET /v1/channels/{login}` resolving login → twitch id, cached as `meta:channelid:{login}`
  - _Requirements: 1.12_ · _Design: §3.1, §6.2_

- [x] 2.6 Capture and pin live GQL operations
  - Record the directory/category/search persisted-query hashes (or inline queries) from a live session into the `upstream` config/fixtures
  - _Requirements: 8.1_ · _Design: §4.1, §11 (deferred item)_

Demo: the directory, category, and search endpoints return normalized JSON from cache on repeat
calls; killing upstream connectivity still serves the last-good `:stale` payload.

---

## Milestone 3 — Chat Gateway (transport)

This milestone delivers raw chat end-to-end; emote enrichment is added in Milestone 5.

- [x] 3.1 Implement the anonymous IRC connection with socket cap
  - Connect `wss://irc-ws.chat.twitch.tv`, handshake `PASS SCHMOOPIIE` / `NICK justinfan{rand}` / `CAP REQ :twitch.tv/tags twitch.tv/commands`
  - Respond to `PING` with `PONG`
  - Connection manager enforcing `MAX_CHANNELS_PER_SOCKET` (default 30), spinning up a new socket at capacity
  - _Requirements: 3.1, 3.2, 3.3_ · _Design: §3.3, §4.3_

- [x] 3.2 Implement the IRCv3 parser
  - Decode tags (`@k=v;...`), prefix, command, trailing into a struct: user, color, display-name, badges, ts, text
  - Tolerate unknown tags and non-PRIVMSG commands (`ROOMSTATE`, `USERNOTICE`, membership)
  - Unit-test against recorded IRC line fixtures
  - _Requirements: 3.4, 3.11_ · _Design: §3.3, §4.3_

- [x] 3.3 Implement room management and reconnect
  - Single upstream room subscription per channel shared across subscribers
  - PART the room after a grace period once the last subscriber leaves
  - Exponential backoff + jitter reconnect, rejoining channels that still have subscribers
  - _Requirements: 3.7, 3.8, 3.9_ · _Design: §3.3_

- [x] 3.4 Build the WS Hub with persistent per-session sockets
  - One `coder/websocket` per client session; maintain `conn↔channel` maps
  - Handle `subscribe`/`unsubscribe` control frames (JOIN on first subscriber, PART on last)
  - Bounded per-connection send queues that drop oldest on overflow
  - Sanitize/encode message content so payloads cannot inject markup
  - _Requirements: 3.6, 3.10, 3.12, 12.2_ · _Design: §3.3, §5_

- [x] 3.5 Implement the batcher and Redis pub/sub seam
  - Publish parsed messages to `chat:{channel}`; Hub subscribes only for channels it serves
  - Accumulate per channel for `BATCH_WINDOW_MS` (50–100ms) and flush as one `batch` frame
  - _Requirements: 3.5 (transport), 5.7, 5.8, 7.2, 11.1, 11.3_ · _Design: §3.3, §5, §8.2_

Demo: a test page opens one WebSocket, sends `subscribe`, and renders live (plain-text) batched
chat for a busy channel; `unsubscribe` stops delivery and the room is parted.

---

## Milestone 4 — 7TV Emote System

- [x] 4.1 Write the PostgreSQL schema migrations
  - Migration creating `emotes`, `emote_sets`, `emote_set_items`, `channels`, `processing_jobs` with the §6.1 enums, flags, FKs, and indexes (including the partial `idx_jobs_claimable`)
  - _Requirements: 4.1, 4.2, 4.3, 4.4, 14.2_ · _Design: §6.1_

- [x] 4.2 Implement the object-storage client
  - S3-compatible client (MinIO/R2/S3) selected via `S3_ENDPOINT`/credentials
  - `Put`/`Delete` under the `/emotes/{id}/{scale}.webp` key layout
  - _Requirements: 4.8, 4.9, 9.4, 14.4_ · _Design: §3.4, §6.1, §9_

- [x] 4.3 Implement the libvips asset worker
  - Claim jobs with `SELECT ... FOR UPDATE SKIP LOCKED`
  - For each scale {1x:32, 2x:64, 3x:96, 4x:128} resize by height and export WebP, preserving animation/alpha
  - All-or-nothing: flip emote `active` only when all scales succeed; on failure record `last_error`, retry to a cap, leave no partial objects
  - Idempotent by `(emote_id, source_hash)`
  - _Requirements: 4.6, 4.7, 4.12, 4.13_ · _Design: §3.4, §7.3, §D5_

- [x] 4.4 Implement the curator API with auth
  - `CURATOR_API_TOKEN` bearer middleware on all write routes
  - `POST /v1/emotes` (multipart: validate type/size, hash, insert pending emote + queued job)
  - `POST /v1/sets`, `POST /v1/sets/{id}/items` (alias), `DELETE /v1/sets/{id}/items/{emote_id}`, `PUT /v1/channels/{twitch_id}/active-set`
  - _Requirements: 4.5, 4.14, 12.3, 12.5_ · _Design: §3.4, §10.2_

- [x] 4.5 Implement the 7TV seeder
  - `POST /v1/seed/twitch/{twitch_id}` → fetch `7tv.io/v3/users/twitch/{id}`, resolve active set + emotes
  - Download originals from `cdn.7tv.app`, reprocess through the asset worker, upsert idempotently
  - Flag global emotes `is_global = true`
  - _Requirements: 4.10, 4.11, 14.3_ · _Design: §3.4, §4.4_

- [x] 4.6 Implement the dictionary builder and delta publisher
  - On set/item/active-set change, rebuild `channel:emotes:{login}` (hash field=name, value `{u,zw}`)
  - Publish `emotes:delta:{channel}` add/remove events
  - On emote delete, remove rows and orphan-collect objects per policy
  - _Requirements: 4.14, 5.1, 5.2, 5.9, 7.2, 14.1_ · _Design: §3.4, §6.2, §8.3_

Demo: seeding a Twitch id imports its 7TV emote set; scaled WebP assets appear in MinIO under
`/emotes/{id}/`; the channel's Redis dictionary is populated and a delta is published on edits.

---

## Milestone 5 — Integration & Tokenization

This milestone connects the Chat Gateway (M3) to the emote system (M4) to produce enriched chat.

- [x] 5.1 Implement the Trie tokenizer
  - Build a per-channel `Trie` from the Redis dictionary; whole-word match with `zw` flag
  - `tokenize` emits ordered `text`/`emote` fragments whose contents round-trip the original text/spacing
  - Unit-test fragment round-trip and zero-width flagging
  - _Requirements: 5.3, 5.4, 5.5, 5.6, 5.10, 5.11, 11.2_ · _Design: §5, §7.1_

- [x] 5.2 Implement the atomic dictionary swap
  - Hold the `Trie` behind `atomic.Pointer`; lock-free reads, race-free `Swap`
  - Lazy-load + build the dictionary on first subscribe; rebuild from PostgreSQL on Redis miss
  - _Requirements: 5.1, 5.2, 5.12, 7.5_ · _Design: §7.1, §6.2_

- [x] 5.3 Implement the debounced delta consumer
  - Subscribe to `emotes:delta:{channel}`; debounce `DELTA_DEBOUNCE_MS` (default 300ms) to coalesce burst edits into one rebuild-and-swap
  - _Requirements: 5.9, 5.12_ · _Design: §7.1_

- [x] 5.4 Wire enrichment into the gateway pipeline
  - Replace the M3 plain-text path: tokenize each parsed message, attach `fragments`, then publish to `chat:{channel}`
  - Forward `emote_delta` and `status`/`error` frames to subscribed connections
  - _Requirements: 3.5, 5.5, 5.7_ · _Design: §3.3, §5, §8.2_

Demo: live chat for a seeded channel arrives at the test page as `fragments`, rendering text and
emote image references; adding an emote via the curator API updates the dictionary and new
messages tokenize it within the debounce window.

---

## Milestone 6 — Frontend Application

- [x] 6.1 Scaffold the Vite + React + TS app
  - Vite project, TailwindCSS, `@tanstack/react-query` provider, `zustand` stores, base layout/routing
  - `CDN_PUBLIC_BASE` and service base URLs from env
  - _Requirements: PC-2, 6.13_ · _Design: §2_

- [x] 6.2 Build the directory, category, and search views
  - Directory grid (thumbnails, title, category, viewer count) via the Metadata API
  - Category navigation and search input backed by the metadata endpoints
  - _Requirements: 6.1, 6.2_ · _Design: §3.1, §8_

- [x] 6.3 Build the HLS player with keep-alive
  - On channel select, `POST /stream/start`, play the local HLS URL via `hls.js`
  - Periodic keep-alive while playing; stop player + cease keep-alive + send `unsubscribe` on leave
  - Error/retry affordance on start failure or stall
  - _Requirements: 6.3, 6.4, 6.5, 6.11_ · _Design: §3.2, §8.1_

- [x] 6.4 Build the persistent chat socket client
  - One persistent `WebSocket` per session; `subscribe`/`unsubscribe` on navigation
  - Reconnect with backoff, re-`subscribe` current channel, show disconnected/reconnecting state
  - _Requirements: 6.6, 6.10_ · _Design: §5_

- [x] 6.5 Build the virtualized emote chat list
  - `@tanstack/react-virtual` list; render `fragments` (text → text node, emote → `<img>` with name alt; zero-width overlaps previous)
  - Enforce `MAX_RETAINED_MESSAGES` (default 200) rolling buffer, shifting oldest on overflow
  - Apply incoming `emote_delta` to the client emote view
  - _Requirements: 6.7, 6.8, 6.9, 6.12, 6.13_ · _Design: §2, §5_

Demo: browse the directory, open a channel, watch HLS video, and see virtualized chat rendering
custom emote images inline with a stable message buffer over a long session.

---

## Milestone 7 — Hardening, Observability & Tests

- [x] 7.1 Add metrics and correlation across services
  - Register the §10.1 metrics (streams_active, listeners, reaped, chat in/out, tokenize histogram, cache hit/miss, upstream result, asset jobs)
  - Propagate a correlation id through logs across service hops; log spawn/reap and integrity/schema errors at alertable severity
  - _Requirements: 10.1, 10.3, 10.4, 10.5, 8.3_ · _Design: §10.1, §10.3_

- [x] 7.2 Add rate limiting and input validation middleware
  - Per-IP token bucket on `stream/start`, `search`, and WS connect
  - Centralized validation for channel names, pagination, search length, upload type/size
  - _Requirements: 12.3, 12.4_ · _Design: §10.2_

- [x] 7.3 Add timeouts, retries, and circuit breakers
  - Context deadlines on all upstream/inter-service calls; backoff + circuit breaker per upstream
  - Open breaker on metadata serves `:stale`; verify per-channel failure isolation
  - _Requirements: 13.1, 13.2, 13.3_ · _Design: §10.3_

- [x] 7.4 Write the unit and contract test suites
  - Unit: tokenizer round-trip, reaper rule, header rotation, cache stale fallback, IRC parser, usher parser, scale count
  - Contract: replay recorded GQL/Usher/IRC/7TV fixtures against parsers
  - _Requirements: 15.4_ · _Design: §10.4_

- [ ] 7.5 Write integration and load tests
  - Testcontainers (PostgreSQL/Redis/MinIO): upload→active and set-change→delta
  - Load: synthetic high-velocity chat asserting bounded memory and batch latency under cap
  - _Requirements: 11.1, 11.3, 15.4_ · _Design: §10.4_

Demo: `make test` runs green; metrics scrape exposes live counters; a load run sustains a
high-velocity channel within the latency and memory bounds.

---

## Milestone 8 — Documentation & Legal

- [x] 8.1 Write the README and operator docs
  - Setup/run via Docker Compose, env var reference, the anonymous (auth-less viewer) design note, and curator-auth setup
  - Prominent legal/ToS disclaimer: educational/personal self-hosting; operator is responsible for compliance (C-1)
  - _Requirements: 12.5, 16 (C-1)_ · _Design: §10.2, §11_

Demo: a new operator can clone, configure `.env`, `make up`, seed emotes, and watch a channel by
following the README; the legal disclaimer is clearly surfaced.
