# Requirements Specification: Open-Source Streaming & Chat Platform Clone

## Introduction

This document specifies the requirements for a full-scale, open-source streaming and chat
platform that reconstructs the core viewing experience of Twitch and the custom-emote
experience of 7TV. Rather than depending on restrictive public developer APIs, the system
taps directly into the publicly observable internal application loops that the upstream web
clients themselves use: the internal GraphQL endpoint for metadata and playback tokens, the
anonymous IRC-over-WebSocket interface for chat, and the public 7TV v3 API for emote data.
These data sources are matched against a custom-built, self-hosted emote database ecosystem
that mirrors 7TV's emote-set model.

The platform is a decoupled microservices system split into three layer zones:

- **Edge / Streaming** — stateless video ingestion and HLS delivery (MediaMTX + stream worker pool).
- **State / Ingestion** — metadata aggregation, chat parsing, emote tokenization, and the 7TV
  database layer (Metadata Service, Chat Gateway, Redis, PostgreSQL, asset processor).
- **Frontend** — a browser client rendering the channel directory, HLS video, and a virtualized,
  emote-enriched chat.

This document defines **what** the system must do and the constraints it must satisfy. It does
not prescribe **how** each requirement is implemented; design decisions are deferred to the
design phase. Implementation language choices are pinned as project conventions (Go for backend
services; a React-based frontend) per PC-2.

### Scope

In scope:

- On-demand restreaming of a single upstream channel triggered per viewer request.
- Live, read-only chat aggregation and emote enrichment for a requested channel.
- A self-hosted 7TV-style emote database, asset pipeline, and CDN-style delivery.
- A web frontend for browsing the directory and watching a stream with enriched chat.
- Local-first deployment via containers, with a path to cloud object storage.

Out of scope (see Section 16):

- Broadcasting or ingesting first-party user streams (the platform restreams existing channels).
- Sending chat messages upstream, moderation actions, or any authenticated write to upstream platforms.
- Monetization, subscriptions, payments, or first-party user accounts.

### Project Conventions

- **PC-1 (No code comments).** All source code produced for this project SHALL contain no
  inline or block comments. Code SHALL be self-documenting through naming and structure;
  explanatory material belongs in design docs and READMEs, not in code.
- **PC-2.** Backend services SHALL be implemented in Go; the frontend SHALL be a
  React-based application (Next.js or Vite) with TailwindCSS.
- **PC-3.** All services SHALL be containerized and orchestratable via a single Docker Compose
  definition for local development.

## Glossary

| Term | Definition |
|------|------------|
| **Upstream** | The third-party platform whose internal loops the system reads from (Twitch for metadata/video/chat; 7TV for seed emote data). |
| **Internal GQL endpoint** | The upstream private GraphQL endpoint used by its own web client, authenticated with a public static client identifier rather than a developer API key. |
| **Playback Access Token** | The signed `value` + `signature` pair returned by the internal GQL `PlaybackAccessToken` operation, required to fetch a stream's HLS playlist. |
| **Usher** | The upstream edge service that returns the master `.m3u8` HLS playlist given a valid playback token. |
| **Metadata Service** | Backend service that fetches and caches directory, category, search, and stream metadata. |
| **Stream Worker** | A managed OS subprocess (Streamlink piped to FFmpeg) that pulls upstream HLS chunks and pushes them to the media server. |
| **Stream Worker Pool** | The orchestrator daemon that spawns, tracks, and reaps Stream Workers. |
| **Media Server** | MediaMTX instance that ingests worker output (RTMP) and republishes it as local HLS/WebRTC. |
| **Reaper** | The background routine that terminates Stream Workers with no active listeners. |
| **Chat Gateway** | Service that connects anonymously to upstream IRC, parses messages, enriches them with emotes, and broadcasts them downstream. |
| **Emote** | A named image asset (static or animated) renderable inline in chat. |
| **Emote Set** | A named collection of emotes; a channel activates exactly one set at a time. |
| **Emote Dictionary** | The flattened `name -> asset URL` map for a channel's active set, cached in Redis for fast tokenization. |
| **Trie** | The in-memory prefix-tree data structure used to tokenize chat messages against emote names in linear time. |
| **Fragment** | A typed segment of a parsed chat message — either `text` or `emote` — used by the frontend to render mixed text/image lines. |
| **Zero-width emote** | An emote flagged to render overlapping the preceding emote rather than beside it. |
| **CDN structure** | The object-storage key layout (`/emotes/{id}/{scale}.{ext}`) used to serve emote assets. |

## Actors / Personas

- **Viewer** — an end user who browses the directory and watches a channel with live chat. Unauthenticated.
- **Curator / Admin** — an operator who manages the emote database: uploads/imports emotes,
  composes emote sets, and maps channels to sets.
- **Operator** — the person deploying and running the platform; concerned with configuration,
  resource usage, observability, and resilience.
- **Upstream systems** — external, non-human actors (internal GQL endpoint, Usher, IRC, 7TV API,
  asset CDNs) that the system integrates with and must treat as untrusted, rate-limited, and
  subject to change.

---

## 1. Metadata Service

**User Story:** As a viewer, I want to browse live channels, categories, and search results with
fresh thumbnails and viewer counts, so that I can discover and choose what to watch without the
platform constantly hammering upstream APIs.

### Acceptance Criteria

1. THE Metadata Service SHALL expose an HTTP API that returns the current top live channels,
   including channel login, display name, title, category, viewer count, and a resolvable
   thumbnail URL.
2. WHEN a thumbnail URL is returned to a client, THE Metadata Service SHALL provide a template or
   resolved form in which width and height placeholders (e.g. `{width}x{height}`) are substituted
   with concrete dimensions.
3. THE Metadata Service SHALL expose endpoints to list categories/games and to list the live
   channels within a given category, with pagination.
4. THE Metadata Service SHALL expose a search endpoint that returns matching channels and
   categories for a free-text query.
5. THE Metadata Service SHALL retrieve metadata by issuing requests to the upstream internal GQL
   endpoint, sending the public static `Client-ID` header and a simulated browser user-agent,
   without requiring a personal developer API key.
6. WHEN a directory, category, or search response is successfully retrieved from upstream, THE
   Metadata Service SHALL cache it in Redis with a short configurable TTL (default 60 seconds).
7. WHILE a cached entry is valid, THE Metadata Service SHALL serve requests from the cache without
   contacting upstream.
8. IF an upstream request returns `403 Forbidden` or otherwise indicates an integrity/anti-bot
   challenge (e.g. missing `Client-Integrity` or `X-Device-Id`), THEN THE Metadata Service SHALL
   attempt to rotate/refresh the required headers or device identifiers and retry before failing.
9. IF an upstream request fails after configured retries, THEN THE Metadata Service SHALL serve the
   most recent cached value when one exists and SHALL annotate the response as stale.
10. IF an upstream request fails and no cached value exists, THEN THE Metadata Service SHALL return
    a structured error response with an appropriate HTTP status and SHALL NOT crash the service.
11. THE Metadata Service SHALL apply outbound rate limiting and request coalescing so that
    concurrent identical client requests result in at most one in-flight upstream request per key.
12. THE Metadata Service SHALL expose a mechanism to resolve an upstream channel login to its
    upstream user identifier, so that downstream services (e.g. emote mapping) can key on a stable id.

---

## 2. On-Demand Video Pipeline

**User Story:** As a viewer, I want a channel's live video to start playing shortly after I open it
and stop consuming server resources after I leave, so that streams are available on demand without
the operator paying for idle bandwidth.

### Acceptance Criteria

1. WHEN a client requests playback for a channel (e.g. `POST /stream/start?channel={login}`), THE
   Video Pipeline SHALL obtain a Playback Access Token by issuing the appropriate playback-token
   operation to the internal GQL endpoint.
2. WHEN a Playback Access Token (`value` + `signature`) is obtained, THE Video Pipeline SHALL
   request the master HLS playlist from Usher using that token and signature and the channel name.
3. WHEN a master playlist is obtained, THE Video Pipeline SHALL start a Stream Worker that pulls the
   upstream HLS stream and publishes it into the Media Server under a per-channel path.
4. THE Video Pipeline SHALL expose the resulting local stream to clients as an HLS manifest URL
   served by the Media Server (e.g. `/live/{channel}/index.m3u8`).
5. IF a Stream Worker for the requested channel is already running, THEN THE Video Pipeline SHALL
   attach the new client to the existing worker rather than spawning a duplicate process.
6. THE Video Pipeline SHALL support selecting among the quality renditions advertised in the master
   playlist (e.g. source, 720p, 480p) and SHALL default to a configurable rendition.
7. THE Video Pipeline SHALL track the number of active listeners per channel based on requests for
   that channel's local manifest and/or an explicit keep-alive signal.
8. WHILE a channel has zero active listeners for longer than a configurable timeout (default 30
   seconds, max 5 minutes), THE Reaper SHALL terminate that channel's Stream Worker.
9. WHEN the Reaper terminates a Stream Worker, THE Video Pipeline SHALL issue a kill to the entire
   process tree (Streamlink and any child FFmpeg process) so that no orphaned child process
   continues pulling upstream chunks.
10. WHEN a client disconnects, refreshes, or loses connectivity, THE Video Pipeline SHALL detect the
    absence of listeners via the keep-alive mechanism and SHALL apply the reaping rule in AC-8.
11. IF a Stream Worker exits unexpectedly while listeners remain, THEN THE Video Pipeline SHALL
    mark the stream as failed and SHALL allow a bounded number of automatic restarts before
    surfacing an error to clients.
12. IF token acquisition or Usher retrieval fails (including the integrity-challenge case from
    Requirement 1, AC-8), THEN THE Video Pipeline SHALL return a structured error and SHALL NOT
    spawn a Stream Worker.
13. THE Video Pipeline SHALL enforce a configurable maximum number of concurrent Stream Workers and
    SHALL reject or queue new start requests when the limit is reached.
14. THE Video Pipeline SHALL maintain an internal registry mapping channel to worker PID, start
    time, listener count, and last-activity timestamp, queryable for observability and reaping.
15. THE Media Server SHALL be configured to retain only a bounded ring buffer of live HLS segments
    per channel (default 3–5 segments) and SHALL immediately purge older segment files, so that
    long-running streams cannot exhaust the container's disk space.

---

## 3. Chat Gateway

**User Story:** As a viewer, I want to see a channel's live chat without my browser connecting
directly to the upstream chat server, so that chat is fast, parsed, and emote-enriched before it
reaches me.

### Acceptance Criteria

1. WHEN at least one client requests chat for a channel, THE Chat Gateway SHALL open (or reuse) an
   anonymous connection to the upstream IRC-over-WebSocket endpoint and join that channel's room.
2. THE Chat Gateway SHALL authenticate to upstream IRC anonymously using the read-only `justinfan`
   login convention (anonymous `PASS`/`NICK`) and SHALL NOT require any per-user OAuth credential.
3. THE Chat Gateway SHALL respond to upstream keepalive `PING` frames with `PONG` so that the
   connection is not dropped.
4. WHEN a chat message (IRC `PRIVMSG`) is received, THE Chat Gateway SHALL parse it into a
   structured object containing at minimum: sender username, display color, timestamp, the raw
   message text, and any upstream-provided metadata tags (e.g. badges, emote ranges) that are
   available.
5. THE Chat Gateway SHALL enrich each parsed message with custom emote data per Requirement 6
   before broadcasting it downstream.
6. THE Chat Gateway SHALL accept one persistent local WebSocket per client session (not one per
   channel), decoupled from the upstream connection (clients never connect to upstream directly),
   and SHALL process lightweight control frames (`SUBSCRIBE {channel}` / `UNSUBSCRIBE {channel}`)
   over it, maintaining a per-connection set of active subscriptions and delivering each channel's
   enriched messages only to connections currently subscribed to that channel.
7. WHERE multiple clients view the same channel, THE Chat Gateway SHALL maintain a single upstream
   room subscription and fan messages out to all local subscribers.
8. WHEN the last subscriber to a channel unsubscribes or disconnects, THE Chat Gateway SHALL part
   the upstream room and release associated resources after a configurable grace period.
9. IF the upstream IRC connection drops, THEN THE Chat Gateway SHALL automatically reconnect using
   exponential backoff with jitter and SHALL rejoin all rooms that still have active subscribers.
10. WHILE inbound message velocity exceeds downstream delivery capacity, THE Chat Gateway SHALL
    apply backpressure handling (per Requirement 6, micro-batching) and SHALL bound its internal
    queues, dropping or coalescing the oldest messages rather than growing memory without limit.
11. THE Chat Gateway SHALL handle non-message IRC events it relies upon (e.g. `ROOMSTATE`,
    membership/`JOIN` where applicable) without failing on event types it does not handle.
12. THE Chat Gateway SHALL treat all upstream message content as untrusted input and SHALL sanitize
    or safely encode it so that downstream payloads cannot inject markup or control characters.

---

## 4. 7TV Emote Database & Asset Pipeline

**User Story:** As a curator, I want a self-hosted emote database and an upload/import pipeline that
optimizes images into multiple scales, so that channels can have custom emote sets served from our
own CDN-style storage.

### Acceptance Criteria

1. THE system SHALL persist emote data in PostgreSQL using a relational model that includes, at
   minimum: emotes, emote sets, the many-to-many membership of emotes within sets (with optional
   per-set alias and status), and channel-to-active-set mappings.
2. THE emote model SHALL record per emote: a unique identifier, name, owner identifier, a global
   flag, visibility/behavior flags (e.g. zero-width, animated, hidden), MIME type, and creation time.
3. THE emote-set membership SHALL support overriding an emote's displayed name via a per-set alias
   without altering the underlying emote.
4. THE channel mapping SHALL associate an upstream channel identifier and login with exactly one
   active emote set at a time.
5. WHEN an emote is uploaded by a curator, THE asset pipeline SHALL accept the source image,
   validate its type and size against configurable limits, and reject unsupported or oversized files
   with a clear error.
6. WHEN a valid emote image is ingested, THE asset pipeline SHALL convert it to WebP, preserving
   transparency and animation where present, using a high-throughput image library (libvips-class),
   not a general-purpose tool such as ImageMagick. WebP is the sole emote output format for V1;
   AVIF is deferred post-V1 (see Section 16) because animated AVIF encoding is CPU-prohibitive
   under high-throughput seeding and concurrent curator uploads.
7. WHEN converting an emote, THE asset pipeline SHALL generate four WebP scale variants — 1x (~32px),
   2x (~64px), 3x (~96px), and 4x (~128px).
8. WHEN scale variants are generated, THE asset pipeline SHALL write them to object storage under a
   deterministic key layout `/emotes/{id}/{scale}.{ext}` (e.g. `/emotes/{id}/1x.webp`).
9. THE system SHALL support MinIO for local/self-hosted deployments and an S3-compatible store
   (AWS S3 / Cloudflare R2) for production, selectable by configuration without code changes.
10. THE system SHALL provide a channel-level provider loading capability that imports emote data
   from selected providers, including 7TV v3 (`/users/twitch/{id}`) and FrankerFaceZ v1
   (`/room/id/{id}` or `/room/{login}`), and persists them into the local schema.
11. WHEN seeding global emotes, THE system SHALL mark them with the global flag so they apply across
    all channels.
12. IF asset processing fails for an emote, THEN THE pipeline SHALL not leave partial/orphaned
    objects in storage and SHALL record the emote as failed/pending rather than active.
13. THE asset pipeline SHALL process emotes asynchronously via a worker so that uploads/imports do
    not block the request thread, and SHALL be idempotent for re-processing the same source.
14. THE system SHALL expose curator APIs to create/update emote sets, add/remove emotes (with
    alias), and assign an active set to a channel.
15. WHEN a viewer enters a channel, THE frontend SHALL offer controls to load 7TV, FFZ, or both for
   that channel without blocking video playback or chat connection.

---

## 5. Emote Tokenization & Chat Integration

**User Story:** As a viewer, I want chat messages to display custom emotes inline as images, so that
the chat experience matches a real 7TV-enabled stream, even in high-velocity rooms.

### Acceptance Criteria

1. WHEN a channel's chat is first requested, THE system SHALL load that channel's active Emote
   Dictionary (`name -> asset URL`, including applicable global emotes) into Redis as a hash keyed
   per channel (e.g. `channel:emotes:{login}`).
2. IF the Emote Dictionary for a channel is absent from Redis, THEN THE system SHALL build it by
   querying PostgreSQL and SHALL populate Redis before tokenizing messages for that channel.
3. THE Chat Gateway SHALL tokenize each message by splitting it into whitespace-delimited words and
   matching words against the channel's active emote names using an in-memory Trie, achieving
   parse time that scales linearly with message length rather than with emote-set size.
4. THE tokenizer SHALL NOT use repeated global regular expressions over the full emote set for
   per-message matching.
5. WHEN a message is tokenized, THE system SHALL produce an ordered `fragments` array where each
   fragment is typed `text` (with `content`) or `emote` (with `content` name and resolved `url`),
   such that concatenating fragment contents reproduces the original message text and spacing.
6. WHERE an emote is flagged zero-width, THE fragment for that emote SHALL carry the zero-width flag
   so the frontend can render it overlapping the preceding emote.
7. WHEN enriched messages are ready, THE Chat Gateway SHALL publish them to a Redis pub/sub channel,
   and the downstream WebSocket fan-out SHALL deliver them to the correct channel's subscribers.
8. THE downstream delivery SHALL micro-batch messages, collecting them for a configurable window
   (50–100 ms) and flushing them to each client as a single array frame to minimize client re-renders.
9. WHEN a curator changes a channel's active emote set or its contents, THE system SHALL update the
   affected Redis Emote Dictionary and SHALL propagate a live delta update (via SSE or WebSocket)
   so that connected clients reflect the change without re-pulling the entire database.
10. THE Emote Dictionary lookup used during tokenization SHALL complete in sub-millisecond time for
    a cached channel under normal load.
11. WHERE a word matches no emote, THE system SHALL emit it as part of a `text` fragment.
12. WHEN a live delta update changes a channel's emote dictionary (per AC-9), THE Chat Gateway SHALL
    build the updated dictionary and Trie off the hot path and SHALL install it via an atomic pointer
    swap of the channel's active Trie, so that in-flight tokenization is never blocked and no data
    race occurs against the high-velocity message loop.

---

## 6. Frontend Application

**User Story:** As a viewer, I want a responsive web app that shows the channel directory, plays the
selected stream, and renders fast emote-rich chat, so that I have a complete viewing experience.

### Acceptance Criteria

1. THE frontend SHALL present a directory view of live channels and categories sourced from the
   Metadata Service, displaying thumbnails, titles, categories, and viewer counts.
2. THE frontend SHALL provide search and category navigation backed by the Metadata Service endpoints.
3. WHEN a viewer selects a channel, THE frontend SHALL request stream start and SHALL play the local
   HLS manifest in an HTML5 video player using an HLS-capable library (e.g. hls.js or Video.js).
4. WHILE a stream is playing, THE frontend SHALL periodically emit the keep-alive signal expected by
   the Video Pipeline so the stream is not reaped while actively watched.
5. WHEN the viewer navigates away from or closes a channel, THE frontend SHALL stop the player,
   cease keep-alive, and send `UNSUBSCRIBE {channel}` over the persistent chat socket, so the
   backend can reap the stream and release the chat room.
6. THE frontend SHALL maintain a single persistent WebSocket per client session to the Chat Gateway
   and SHALL switch channels by sending `SUBSCRIBE`/`UNSUBSCRIBE` control frames over that existing
   connection rather than opening a new socket per channel, and SHALL render incoming batched messages.
7. THE frontend SHALL render the chat list using virtualization (e.g. react-window / TanStack
   Virtual) so that only the visible subset of messages is mounted in the DOM regardless of total
   message volume.
8. WHEN rendering a message, THE frontend SHALL iterate the `fragments` array, rendering `text`
   fragments as text elements and `emote` fragments as image elements pointing at the emote asset URL.
9. WHERE a fragment is flagged zero-width, THE frontend SHALL render it overlapping the preceding
   emote rather than occupying new horizontal space.
10. IF the persistent chat WebSocket disconnects, THEN THE frontend SHALL automatically attempt to
    reconnect with backoff, SHALL re-send `SUBSCRIBE` for the currently viewed channel upon
    reconnect, and SHALL visually indicate the disconnected/reconnecting state.
11. IF the video stream fails to start or stalls, THEN THE frontend SHALL surface a clear error or
    retry affordance rather than failing silently.
12. THE frontend SHALL bound the number of retained chat messages (a rolling buffer) to prevent
    unbounded memory growth during long sessions.
13. THE frontend SHALL be styled with TailwindCSS and SHALL meet baseline accessibility
    expectations (keyboard navigation, alt text on emote images using the emote name, sufficient
    contrast).

---

## 7. Caching, Pub/Sub & Data Synchronization

**User Story:** As an operator, I want Redis to act as the central cache and message bus, so that
parsing stays fast and services stay decoupled.

### Acceptance Criteria

1. THE system SHALL use Redis as the cache for metadata responses and per-channel Emote Dictionaries.
2. THE system SHALL use Redis pub/sub (or an equivalent broker capability) to broadcast enriched
   chat messages and live emote-set delta updates between services.
3. THE system SHALL define explicit, namespaced key conventions for all cached data (e.g.
   `channel:emotes:{login}`, metadata keys) documented in the design phase.
4. WHEN a cached key's TTL expires, THE system SHALL repopulate it on next demand without serving
   corrupt or partially written values.
5. IF Redis is unavailable, THEN dependent services SHALL degrade gracefully (e.g. fall back to
   direct PostgreSQL reads for emotes) where feasible and SHALL report the degraded state rather
   than crashing.

---

## 8. Reverse-Engineering Resilience & Upstream Contract Management

**User Story:** As an operator, I want the system to absorb upstream changes and anti-bot defenses,
so that breakage is contained and diagnosable rather than catastrophic.

### Acceptance Criteria

1. THE system SHALL centralize upstream request construction (endpoints, client identifiers,
   headers, GQL operation payloads) in configuration or a single module so contracts can be updated
   without scattering changes across the codebase.
2. IF upstream responds with integrity challenges or `403`s, THEN the affected service SHALL apply
   the header/device-id rotation strategy and SHALL expose a hook for an optional headless-browser
   worker to supply fresh integrity tokens.
3. THE system SHALL detect and log when an upstream response shape diverges from the expected schema
   (parse failure) and SHALL surface this as a distinct, alertable error class.
4. THE system SHALL make all upstream identifiers and endpoints (internal GQL endpoint, public
   Client-ID, Usher base, IRC endpoint, 7TV API base, CDN base) configurable via environment.
5. THE system SHALL apply per-upstream rate limiting and backoff to avoid tripping anti-abuse
   defenses.

---

## 9. Configuration & Deployment

**User Story:** As an operator, I want to run the entire stack locally with one command and point it
at production storage via configuration, so that development mirrors production.

### Acceptance Criteria

1. THE system SHALL provide a single Docker Compose definition that brings up MediaMTX, Redis,
   PostgreSQL, the object store (MinIO), and all custom services.
2. THE system SHALL read all environment-specific settings (endpoints, credentials, TTLs, timeouts,
   concurrency limits, storage backend selection) from environment variables or config files, with
   no secrets hardcoded in source.
3. THE system SHALL provide database schema migrations that initialize the PostgreSQL schema on a
   fresh deployment.
4. THE system SHALL support switching object storage between MinIO and an S3-compatible production
   store purely via configuration.
5. WHERE optional official upstream credentials are configured (e.g. a developer app for live/offline
   event notifications), THE system SHALL use them to enhance functionality but SHALL NOT require
   them for core operation.
6. THE system SHALL start each service independently so that failure of one service (e.g. asset
   processor) does not prevent unrelated services (e.g. chat) from operating.

---

## 10. Observability & Operations

**User Story:** As an operator, I want logs, metrics, and health checks, so that I can monitor
stream workers, chat throughput, cache hit rates, and upstream failures.

### Acceptance Criteria

1. EACH service SHALL emit structured logs with severity levels and correlation identifiers for
   request tracing across services.
2. EACH service SHALL expose a health/readiness endpoint suitable for container orchestration probes.
3. THE system SHALL expose metrics for at least: active Stream Workers, per-channel listener counts,
   reaped workers, chat messages per second (in/out), tokenization latency, cache hit/miss rates,
   upstream request success/failure counts, and asset-processing throughput.
4. WHEN a Stream Worker is spawned or reaped, THE system SHALL log the event with channel, PID, and
   reason.
5. WHEN an upstream integrity challenge or schema-divergence error occurs, THE system SHALL log it at
   a severity that supports alerting.

---

## 11. Performance & Scalability

**User Story:** As a viewer in a large channel, I want chat and video to stay responsive, so that
high message velocity does not degrade the experience.

### Acceptance Criteria

1. THE Chat Gateway SHALL sustain a high-velocity room (target: thousands of messages per second for
   a single channel) without unbounded memory growth, using the Trie tokenizer and micro-batching.
2. THE tokenization of a single chat message SHALL complete in time proportional to the message
   length, independent of the total emote-set size.
3. THE micro-batching window SHALL reduce frontend render triggers substantially versus per-message
   delivery (design target: ~90% reduction) while keeping perceived latency within the configured
   window (≤100 ms).
4. THE Metadata Service SHALL serve cached directory/category requests without per-request upstream
   calls while a cache entry is valid.
5. THE system SHALL support multiple concurrently active channels (video + chat) bounded only by
   configured resource limits, and SHALL document the per-channel resource cost.

---

## 12. Security & Privacy

**User Story:** As an operator, I want the platform's own surfaces to be safe by default, so that
exposing it does not create injection, abuse, or data-leak risks.

### Acceptance Criteria

1. THE system SHALL treat all data received from upstream (metadata, chat text, emote payloads) as
   untrusted and SHALL validate/sanitize it before storage or broadcast.
2. THE frontend SHALL render chat fragments without executing or interpreting message content as
   markup, preventing cross-site scripting via chat text or emote names.
3. THE system SHALL validate and bound all client-supplied inputs (channel names, search queries,
   pagination, uploads) and SHALL reject malformed or oversized inputs.
4. THE system SHALL apply rate limiting to its own public-facing endpoints (stream start, search,
   chat subscribe) to mitigate abuse and accidental self-inflicted load.
5. THE curator/admin APIs (emote upload, set management, channel mapping) SHALL be access-controlled;
   they SHALL NOT be exposed without authentication. **Security note:** the viewer-facing read APIs
   are intentionally unauthenticated for an anonymous viewing experience — this is an explicit design
   choice and the absence of viewer auth SHALL be documented; any deployment exposing write/admin
   surfaces without authentication is a defect.
6. THE system SHALL store all secrets (object-storage keys, optional upstream credentials) outside of
   source control and SHALL load them from the environment or a secrets store.
7. THE system SHALL construct subprocess invocations (Streamlink/FFmpeg) without unsafe interpolation
   of unvalidated channel names, preventing command injection.
8. THE system SHALL not transmit project code, secrets, or collected data to any third party beyond
   the upstream endpoints required for its stated function.

---

## 13. Resilience & Fault Tolerance

**User Story:** As an operator, I want partial failures to be contained, so that one bad channel,
process, or dependency does not take down the platform.

### Acceptance Criteria

1. IF a single Stream Worker crashes, THEN only that channel's playback SHALL be affected, and the
   pool SHALL continue serving other channels.
2. IF the upstream IRC connection for one channel fails, THEN other channels' chat SHALL continue
   uninterrupted while the failed connection reconnects.
3. THE system SHALL apply timeouts to all upstream and inter-service calls so that no request hangs
   indefinitely.
4. THE system SHALL guarantee no zombie/orphaned Streamlink or FFmpeg processes persist after a
   stream ends, a worker crashes, or the orchestrator restarts (process-tree cleanup on shutdown).
5. WHEN the orchestrator restarts, THE system SHALL reconcile its worker registry with actual running
   processes and SHALL clean up any untracked stream processes.

---

## 14. Data Integrity & Lifecycle

**User Story:** As a curator, I want emote data and assets to stay consistent, so that the database
and object store never drift apart.

### Acceptance Criteria

1. WHEN an emote is deleted, THE system SHALL remove or mark its database records and SHALL remove or
   orphan-collect its associated objects in storage according to a defined policy.
2. THE database SHALL enforce referential integrity between emotes, sets, set membership, and channel
   mappings (cascading where appropriate).
3. WHEN re-seeding from the upstream 7TV API, THE system SHALL upsert idempotently so repeated seeds
   do not create duplicates.
4. THE asset key layout SHALL be deterministic from the emote identifier so assets are always
   locatable from database records.

---

## 15. Non-Functional Requirements Summary

1. **Maintainability.** Code SHALL be modular per service, SHALL contain no comments (PC-1), and
   SHALL rely on clear naming and design documentation.
2. **Portability.** The full stack SHALL run on a single developer machine via containers and SHALL
   deploy to a cloud host without code changes (configuration-only).
3. **Extensibility.** Upstream contracts SHALL be isolated so that adapting to upstream changes is
   localized.
4. **Testability.** Core logic (tokenizer, fragment generation, reaper rules, cache fallbacks) SHALL
   be unit-testable independent of upstream and SHALL ship with automated tests.
5. **Observability.** Per Section 10.
6. **Resource discipline.** The system SHALL not leak processes, memory, or storage objects under
   normal operation or partial failure.

---

## 16. Constraints, Assumptions & Out of Scope

### Constraints

- **C-1 (Legal / Terms of Service).** This platform reads from third-party internal endpoints and
  restreams third-party content. Operating it may violate the upstream platforms' Terms of Service
  and/or applicable law. The project is intended for educational, research, and personal
  self-hosting use. Operators are solely responsible for compliance; the specification does not
  authorize infringing or unauthorized commercial use. This constraint SHALL be surfaced in project
  documentation.
- **C-2.** The system depends on undocumented upstream behavior (internal GQL operations, Usher,
  anonymous IRC, public client identifiers). These can change without notice; the system is designed
  to degrade and adapt (Section 8) but cannot guarantee continuous availability.
- **C-3.** Anonymous IRC access is read-only; the platform cannot and SHALL not send chat upstream.
- **C-4 (No code comments).** Per PC-1.
- **C-5.** Backend in Go; frontend in React (Next.js/Vite) + TailwindCSS; MediaMTX as media
  server; Redis and PostgreSQL as datastores; libvips-class image processing producing WebP.

### Assumptions

- **A-1.** Streamlink, FFmpeg, MediaMTX, Redis, PostgreSQL, and an object store are available in the
  deployment environment (provided via containers).
- **A-2.** The public static upstream Client-ID and anonymous IRC conventions remain usable; if
  revoked, configuration allows substitution.
- **A-3.** Viewers are anonymous; no first-party viewer accounts are required for the core experience.
- **A-4.** Network egress to upstream endpoints and CDNs is permitted from the deployment host.

### Out of Scope

- First-party broadcasting/ingest of original streams.
- Sending messages, moderation, subscriptions, bits, or any authenticated upstream write.
- First-party viewer authentication, profiles, follows, or notifications (beyond optional EventSub
  live/offline signals in Requirement 9, AC-5).
- Payments, monetization, and DRM-protected content handling.
- Mobile native applications (web frontend only for this phase).
- VOD/clip archival and playback (live restreaming only for this phase).
- AVIF emote output (WebP only for V1; AVIF deferred post-V1 due to animated-encoding CPU cost).

---

## 17. Traceability to Source Plans

| Plan source | Captured in |
|-------------|-------------|
| Metadata engine, GQL, Redis cache, Client-Integrity rotation | Req. 1, 8 |
| MediaMTX, Streamlink orchestration, reaper, zombie prevention | Req. 2, 13 |
| Playback token + Usher handshake | Req. 2 |
| Chat Gateway, anonymous IRC (`justinfan`), JSON parsing, fan-out | Req. 3 |
| 7TV schema, libvips pipeline, WebP (AVIF deferred), 1x–4x scales, MinIO/S3, 7TV v3 seeding | Req. 4, 14 |
| Redis emote dictionary, Trie tokenizer, fragments payload, micro-batching, live deltas | Req. 5, 7 |
| Frontend: directory, hls.js/Video.js, virtualized chat, fragment rendering | Req. 6 |
| Redis pub/sub data synchronization | Req. 7 |
| Backpressure, Trie, micro-batching trade-offs | Req. 5, 11 |
| Docker Compose, language choices, libvips, config | Req. 9, PC-1..3 |
| Self-review additions: security, resilience, observability, data integrity, legal | Req. 8, 10, 12, 13, 14, 16 |
