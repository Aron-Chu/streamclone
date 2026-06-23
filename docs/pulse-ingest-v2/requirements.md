# Pulse Ingest v2 Requirements

Status: brainstorm / requirements draft
Owner: Streamclone analytics + Pulse surfaces
Related docs: [Analytics steering](../../.kiro/steering/analytics.md), [Archive requirements](../scraping-archive/requirements.md), [Workspace layout](../workspace.md)

## Summary

Pulse Ingest v2 should make Streamclone's live Pulse surfaces feel richer without making every channel require full raw chat retention. The product direction is:

- Keep the main chart focused on chat activity.
- Add an optional expanded panel for 7TV/emote spikes aligned to the chat graph.
- Lazy-load the full stream timeline when the user asks for it.
- Scale hosted collection by tiering channels and aggregating early, not by storing every raw chat line for every top streamer.
- Treat top 100-200 streamer coverage as viable on a VPS when the default product is minute rollups, viewer samples, and selective full chat/backfill.

The key principle: **aggregate early, fetch in segments, backfill selectively**.

## Goals

- Provide a compact Pulse chart that remains readable in the web sidebar and Chrome extension.
- Let users expand under the graph to inspect specific 7TV/emote spikes and compare them against chat activity.
- Support "load the whole livestream since start" for viewer/chat/emote minute rollups without forcing every poll to return the whole stream.
- Define a realistic hosted VPS path for tracking top 100-200 streamers from stream start.
- Keep raw chat storage optional and policy-driven, because full-message retention is expensive.
- Preserve honest coverage states when Streamclone starts tracking late or when only viewer samples are available.

## Non-Goals

- Do not mirror Twitch video files.
- Do not store raw chat for every top-200 stream by default.
- Do not run GQL VOD chat backfill for every stream. GQL backfill remains selective because upstream rate limits and pagination are the bottleneck.
- Do not make the extension fetch full-stream history on every 15-30 second refresh.
- Do not add independent IRC clients per feature. Shared IRC ingest should feed all live analytics consumers.

## Product Requirements

### R1. Main Pulse Chart

- R1.1 The default Pulse graph SHALL remain a chat-activity chart, not a combined viewer/chat/emote chart.
- R1.2 The default graph SHALL avoid noisy labels such as "CHAT" and "EMOTES" directly on the chart.
- R1.3 The chart SHALL keep the current dark Stream Pulse visual style across web and extension.
- R1.4 The graph SHALL support recent-window rendering for compact surfaces, with no full-stream fetch required for normal polling.

### R2. Expandable Emote Spike Panel

- R2.1 Users SHOULD be able to expand a small area under the chart to inspect emote spikes.
- R2.2 The expanded panel SHALL align emote spike data to the same minute timeline as chat activity.
- R2.3 The panel SHOULD show 7TV activity as one of:
  - a secondary mini sparkline,
  - thin overlay lines for selected emotes,
  - spike markers with emote names/images,
  - or a compact ranked list for the current visible window.
- R2.4 The panel SHOULD default to the top 3-5 relevant emotes for the visible range.
- R2.5 Users SHOULD be able to pin/select specific emotes when there are many candidates.
- R2.6 The panel SHALL degrade cleanly when no emote identity is available, showing aggregate 7TV counts only.
- R2.7 The extension and web sidebar SHOULD share derivation through `packages/pulse-core` where possible.

### R3. Full Livestream Timeline Loading

- R3.1 Users SHOULD be able to load the full stream timeline since `startedAt` on demand.
- R3.2 Full timeline loading SHALL be lazy, initiated by an expand action or "full stream" control.
- R3.3 Normal auto-refresh SHALL fetch only recent data or deltas, not the whole stream.
- R3.4 The API SHOULD support range and delta requests such as:
  - `fromOffsetSeconds` / `toOffsetSeconds`
  - `sinceMinuteTs`
  - `window=recent|full`
- R3.5 A long stream SHOULD be safe to render by decimating or virtualizing chart points when needed.
- R3.6 The UI SHALL disclose partial coverage when tracking began after stream start.

### R4. Hosted Top-Streamer Tracking

- R4.1 A hosted deployment SHOULD support top 100-200 streamer session and viewer coverage using Tier-0 roster sampling.
- R4.2 Tier-0 viewer sampling SHOULD open/update stream sessions from Helix even when no user is watching the channel.
- R4.3 Full IRC chat ingest SHOULD be tiered rather than enabled blindly for all top-200 streams.
- R4.4 Always-tracked and user-requested channels SHALL have higher priority than global top-N channels.
- R4.5 The system SHOULD support policy tiers:
  - P0: always-tracked and actively watched channels, full live IRC rollups.
  - P1: top 25 by viewers, full live IRC rollups.
  - P2: remaining top 100-200, viewer samples and stream sessions by default.
  - P3: selective post-stream GQL backfill for high-value streams or detected gaps.
- R4.6 The tracking policy SHOULD be configurable by environment variables and future admin settings.

### R5. VPS Scaling Target

- R5.1 A 4 core / 8 GB VPS SHOULD be considered enough for:
  - Tier-0 viewer/session sampling for top 100-200.
  - full IRC rollups for a smaller hot set such as top 25-50, depending on peak chat rate.
- R5.2 Top-200 full IRC rollups MAY require either:
  - a larger single VPS, around 8 core / 16 GB, or
  - multiple smaller ingest workers sharing Postgres.
- R5.3 Horizontal sharding SHOULD be preferred over a single very large VPS when Twitch IRC socket/channel limits or join behavior become bottlenecks.
- R5.4 Raw chat retention for all top-200 streams SHALL NOT be assumed viable on a 4 core / 8 GB VPS.
- R5.5 The initial production target SHOULD be measured with rollup ingest first, before enabling raw-message retention.

### R6. Emote Metadata Readiness (Live Pulse)

- R6.1 When live tracking starts (`POST /watch`), the backend SHALL kick off async 7TV/emote ensure without blocking IRC join or the watch response.
- R6.2 The live collector SHALL use any existing Redis emote dictionary immediately (stale-while-revalidate).
- R6.3 If emote metadata is missing or syncing, rollups SHALL still record chat counts and aggregate emote totals; provider-specific identity MAY degrade to aggregate-only until ensure completes.
- R6.4 The extension BFF SHALL expose `emoteSync` state (`ready`, `syncing`, `stale`, `unavailable`, `aggregate_only`) on pulse payloads — not full emote sets on every poll.
- R6.5 Gold VOD replay SHALL continue to block tokenization on emote ensure before GQL chat fetch (accuracy over speed for historical backfill).
- R6.6 The MVP SHALL NOT render Twitch chat emotes in the extension; the official 7TV extension remains responsible for chat rendering.

## Data Requirements

### D1. Rollup-First Storage

- D1.1 The canonical live analytics unit SHALL remain a minute rollup.
- D1.2 Minute rollups SHALL include viewer samples, chat count, total emote count, 7TV count, and bounded top-emote maps.
- D1.3 The system SHOULD avoid per-message database writes in the live path.
- D1.4 Top emotes per minute SHOULD remain bounded, with the default top-N treated as a storage and CPU tuning knob.
- D1.5 Rollups SHOULD be exportable to object storage as compressed JSONL or a later columnar format.

### D2. Raw Chat Retention

- D2.1 Raw chat SHALL be optional.
- D2.2 Raw chat SHOULD be retained only for selected streams:
  - always-tracked channels,
  - gold/high-peak streams,
  - user-requested captures,
  - or streams selected for corpus analysis.
- D2.3 Raw chat, when retained, SHOULD be written to compressed append-only objects rather than hot Postgres rows whenever possible.
- D2.4 Raw chat retention SHALL have explicit retention and archive policies.

### D3. Emote Spike Compression

- D3.1 For UI spike analysis, Streamclone SHOULD prefer heavy-hitter summaries over full per-emote maps for every minute.
- D3.2 Candidate techniques MAY include:
  - bounded top-K maps per minute,
  - Count-Min Sketch for tail estimates,
  - Space-Saving / Misra-Gries heavy hitters,
  - or provider-specific top lists.
- D3.3 Any approximate algorithm SHALL mark confidence or explain when counts are estimated.
- D3.4 Exact top-K rollups SHOULD remain the default until top-200 scale proves them too large.

## API Requirements

- A1. Pulse APIs SHOULD expose recent-window rollups for lightweight polling.
- A2. Pulse APIs SHOULD expose range/delta rollup endpoints for expanded full-stream views.
- A3. API responses SHOULD include coverage metadata:
  - `startedAt`
  - `coverageStartOffsetSeconds`
  - `currentOffsetSeconds`
  - rollup window start/end
  - whether data is recent, partial, or backfilled.
- A4. The extension BFF SHOULD avoid returning full-stream payloads unless explicitly requested.
- A5. Web and extension clients SHOULD share adapters in `packages/pulse-core` to reduce drift.

## Ingest Architecture Requirements

- I1. IRC ingest SHOULD be shared across Streamclone live features.
- I2. IRC channels SHOULD be sharded across sockets and, later, across workers.
- I3. The collector SHALL aggregate in memory by stream/minute and flush rollups on a bounded cadence.
- I4. Worker queues SHALL have priority ordering: user watch and always-tracked first, then top-N policy tiers.
- I5. Backpressure SHALL prefer reducing lower-priority chat coverage over losing P0/P1 channels.
- I6. On worker restart, the system SHOULD lose at most the current open minute of live chat aggregation.
- I7. Helix viewer/session sampling SHALL continue even when IRC capacity is saturated.

## Performance Budgets

Initial targets to validate with measurements:

- Recent Pulse payload: under 100 KB for normal extension polling.
- Full stream rollup payload: acceptable up to multi-hour streams when requested explicitly.
- Normal polling interval: 15-30 seconds.
- Full-stream reload: manual or infrequent, not every poll.
- Ingest memory: proportional to active channels and current-minute emote maps, not total messages.
- Database live writes: approximately active streams multiplied by one rollup per minute, plus viewer sample updates.

## Suggested Implementation Phases

### Phase 1: UI and API Efficiency

- Add expanded emote spike panel under the Pulse graph.
- Add range/delta request support for rollups.
- Lazy-load full-stream timeline only on user action.
- Keep recent-window payload as the default extension behavior.

### Phase 2: Hosted Tier-0 Viewer Coverage

- Enable and harden top-N roster/session/viewer sampling for top 100-200.
- Measure storage, API quotas, and Postgres load for one week.
- Add operator dashboards for roster size, live streams, sample lag, and rollup write rate.

### Phase 3: Tiered IRC Chat Coverage

- Raise live IRC rollup coverage from default watched channels toward top 25-50.
- Add shard assignment for ingest workers.
- Add priority rules so always-tracked and user-watch channels win under load.
- Measure message throughput, CPU, and dropped/lost minute windows.

### Phase 4: Selective Gold Backfill and Archive

- Queue GQL VOD chat backfill only for gold streams, gaps, and user-selected streams.
- Export rollups and selected raw chat to object storage.
- Add decimation and retention policies for old streams.

## Open Questions

- What should be the first hosted target: top 25 full chat, top 50 full chat, or top 200 viewer-only?
- Should "top 200" be global live rank, a fixed streamer roster, or a hybrid with always-tracked overrides?
- How long should exact 1-minute rollups remain hot in Postgres before decimation/export?
- Which emote spike visualization is easiest to understand in the extension sidebar?
- Should raw chat ever be visible in the product, or only retained for re-parsing and future analytics?
- What monthly infrastructure budget is acceptable for the hosted tracker?
- How should Streamclone disclose estimated vs exact emote spike counts if approximate heavy-hitter algorithms are introduced?

## Current Recommendation

Build this in two separate but compatible tracks:

1. **Product track:** add the expandable emote spike panel and lazy full-stream timeline loading using existing rollups. This is high user value and low infrastructure risk.
2. **Infrastructure track:** scale hosted coverage as Tier-0 first, then gradually add IRC rollup coverage for prioritized channels. Do not jump directly to raw chat for top 200.

A 4 core / 8 GB VPS is a reasonable starting point for Tier-0 viewer/session sampling and a smaller hot IRC set. For reliable top-200 full IRC rollups, plan for sharded ingest workers or a larger node after measurement. The better "compression algorithm" is not downloading chat faster like IDM; it is converting messages into bounded minute rollups immediately, then fetching those rollups in ranges and deltas.
