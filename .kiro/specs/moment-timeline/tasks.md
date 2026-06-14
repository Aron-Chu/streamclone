# Implementation Plan: Moment Timeline

## Overview

This plan converts the Moment Timeline design into incremental coding tasks across the existing Streamclone stack: a new Go `internal/analytics/heatmap` package (deterministic score engine), an HTTP handler in package `analytics` that bridges rollup consolidation into the pure heatmap engine, Redis cache, frontend React/TypeScript components (heatmap lanes, right rail, live stats band, VOD mode controls), and a P2 `internal/analytics/chatreplay` storage layer with a GQL-sync replay sink.

Tasks honor **P0 VOD playback trust first**. The phase order is: **setup → P0 VOD trust → P1 (analytics IA → heatmap package/scoring engine → heatmap API → heatmap UI → live stats) → P2 (VOD review mode → chat replay) → visual regression → final checkpoint**. Each step builds on prior steps and ends by wiring new code into existing services (`cmd/analytics`, `frontend/src/components/Analytics.tsx`, `Channel.tsx`).

Backend property tests use Go `pgregory.net/rapid`; frontend property tests use `fast-check` via the runner's `fc.assert(fc.property(...))` form (there is no `test.prop` helper in this repo). Each property test runs ≥100 iterations and carries the tag `// Feature: moment-timeline, Property {N}: {title}`. Test sub-tasks are tagged `_(test, optional)_` and may be skipped for a faster MVP, except trust/correctness-critical tests (P0 VOD smoke, backend VOD error coverage, release bundle verification, and score/fixture determinism), which are required.

## Tasks

- [x] 1. Set up property-based test tooling and dependencies
  - [x] 1.1 Add backend `rapid` dependency
    - Add `pgregory.net/rapid` to `go.mod` via `go get pgregory.net/rapid` (not currently a dependency); confirm `go mod tidy` resolves it for `internal/analytics/...`
    - _Requirements: 9.6, 9.10_

  - [x] 1.2 Add frontend `fast-check` dependency and document test form
    - Add `fast-check` to `frontend/package.json` devDependencies (`npm i -D fast-check`)
    - Document in the test setup that frontend property tests use the runner's standard `it(...)` block wrapping `fc.assert(fc.property(...))` (or `fc.assert(fc.asyncProperty(...))` for async); there is NO `test.prop([...], cb)` macro in this repo
    - _Requirements: 1.3, 8.4_

- [x] 2. Implement P0 VOD deep link landing and error states (frontend)
  - [x] 2.1 Implement VOD identifier normalization utility
    - Add `frontend/src/utils/vodId.ts`: strip whitespace, reject `videos/` URL prefixes and empties, output `^\d{5,20}$` or reject; idempotent
    - _Requirements: 1.3, 1.6_

  - [x] 2.2 Write property test for VOD ID normalization round-trip _(test, optional)_
    - **Property 1: VOD Identifier Normalization Round-Trip** — valid output matches `^\d{5,20}$` and is idempotent
    - **Validates: Requirements 1.3, 1.6**

  - [x] 2.3 Implement VOD mode landing in Channel workspace
    - Parse `?vod=&offset=` (offset optional, default 0), normalize id before relay, `POST /v1/stream/vod/start` with `vod_id` + `offset_seconds`; on 200 begin playback and seek to `Math.max(0, offset_seconds - seek_seconds)` (±2s); reuse the existing `VodStartResponse` from `frontend/src/api.ts`; reuse same Deep_Link for VODs-tab "Play VOD" and analytics "Play in Streamclone"
    - _Requirements: 1.1, 1.3, 1.4, 1.5, 1.6_

  - [x] 2.4 Implement VOD error state component
    - Add `frontend/src/components/channel/VodErrorState.tsx`: map error codes to copy/actions per Req 2 (invalid_vod_id, vod_unavailable, upstream_token_failed, capacity_reached, hls_not_ready, vod_start_failed, HLS 401 proxy auth); retry for retryable codes, max 2 auto-retries for hls_not_ready
    - _Requirements: 1.7, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8_

  - [x] 2.5 Write property test for retryable classification _(test, optional)_
    - **Property 29: VOD ID Retryable Classification** — retry action presence matches `retryable` flag per error code
    - **Validates: Requirements 2.7**

  - [x] 2.6 Write unit tests for VOD error code → UI copy mapping _(test, optional)_
    - Cover all 6 error codes plus HLS 401 proxy-auth guidance
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.8_

- [x] 3. Implement P0 stale VOD id handling and release/backend smoke coverage
  - [x] 3.1 Implement Play-in-Streamclone enablement guard
    - Enable "Play in Streamclone" only when stream has non-empty `vodId` and `vodSource != "unknown"`; otherwise hide/disable with "VOD id not yet resolved" copy; on `vod_unavailable` for recently synced stream offer re-sync / open-on-Twitch / back-to-analytics
    - _Requirements: 34.1, 34.2, 34.3_

  - [x] 3.2 Write property test for Play-in-Streamclone enablement _(test, optional)_
    - **Property 30: Play-in-Streamclone Action Enablement** — enabled iff non-empty vodId and vodSource ≠ "unknown"
    - **Validates: Requirements 34.1, 34.2**

  - [x] 3.3 Write VOD deep link smoke test
    - Render analytics moment with known vod_id/offset/channel, simulate "Play in Streamclone", assert URL `/c/{login}?vod=&offset=`, mock `POST /v1/stream/vod/start`, assert request `vod_id` and seek to `Math.max(0, offset_seconds - seek_seconds)`; non-200 → error, no HLS
    - _Requirements: 25.1, 25.2, 25.3, 25.4_

  - [x] 3.4 Write backend VOD error coverage tests
    - `hls_not_ready` 504 retryable, `invalid_vod_id` 400, `vod_unavailable` 404, `capacity_reached` 503 retryable, `upstream_token_failed` 502 retryable (extend `orchestrator_test.go` patterns)
    - _Requirements: 26.1, 26.2, 26.3, 26.4, 26.5_

  - [x] 3.5 Write release bundle verification smoke check
    - Record served entry-script hash vs CI artifact; VOD smoke `POST /v1/stream/vod/start` with valid id fails on 400 `invalid_vod_id`; classify live-pass/VOD-fail as deploy mismatch
    - _Requirements: 33.1, 33.2, 33.3_

- [x] 4. Checkpoint - P0 VOD trust
  - Ensure all tests pass, ask the user if questions arise.

- [x] 5. Implement P1 analytics IA and honest empty states
  - [x] 5.1 Implement sync CTA label utility
    - Add `frontend/src/utils/syncLabel.ts`: pure function returning identical label per stream state from the CTA label table ("Sync chat & viewers", "Sync chat & emotes", "Syncing…", hidden/Re-sync)
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5_

  - [x] 5.2 Write property test for sync CTA label consistency _(test, optional)_
    - **Property 2: Sync CTA Label Consistency Across Placements** — all placements share identical label per state
    - **Validates: Requirements 4.1**

  - [x] 5.3 Implement stat-card placeholder classification utility
    - Add `frontend/src/utils/statCards.ts`: classify "Stats only" / "Needs sync" / "Collecting" / numeric per Req 6; muted style flag for placeholders
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5_

  - [x] 5.4 Write property test for stat-card placeholder classification _(test, optional)_
    - **Property 3: Stat Card Placeholder Classification** — Stats only / Needs sync / Collecting rules hold
    - **Validates: Requirements 6.1, 6.2, 6.3**

  - [x] 5.5 Implement live empty-state consistency logic
    - Suppress "No recent data" when stream badge is "Collecting now"; show "Collecting first minutes" with activity indicator when <2 rollups live; swap to chart on ≥2 rollups
    - _Requirements: 7.1, 7.2, 7.3_

  - [x] 5.6 Write property test for empty-state consistency _(test, optional)_
    - **Property 4: No Contradictory Empty State During Active Collection** — "Collecting now" + <2 rollups → not "No recent data"
    - **Validates: Requirements 7.1**

  - [x] 5.7 Implement Right Rail tabbed container
    - Add `frontend/src/components/analytics/RightRail.tsx`: tabs Moments | Emotes | Clips | Sync, default Moments on load/stream change, retain selection until stream change/reload, Moments empty-state when no rollup data
    - _Requirements: 3.1, 3.2, 3.3, 3.4_

  - [x] 5.8 Write unit tests for right rail tab order and default selection _(test, optional)_
    - Tab order, default Moments, selection retention, empty state
    - _Requirements: 3.1, 3.2, 3.3, 3.4_

  - [x] 5.9 Implement Clips tab empty-state honesty
    - Empty state instructing sync-first when zero chat/emote rollups; reference Moments/heatmap peaks (not "click the graph") when rollups exist but no clip jobs
    - _Requirements: 35.1, 35.2_

  - [x] 5.10 Implement mobile chart priority and tier indicator
    - Below 1024px: chart + moment controls above archive in DOM/visual order, archive collapsed to ≤2 rows with expand/collapse toggle; add Tier_Indicator badge in header derived from `useSystemHealth`/`useOptionalServices` (Core vs Analytics active) with empty-state guidance
    - _Requirements: 5.1, 5.2, 5.3, 38.1, 38.2, 38.3, 38.4, 38.5_

  - [x] 5.11 Wire IA utilities into Analytics page
    - Connect `syncLabel`, `statCards`, empty-state logic, RightRail, mobile layout, and Tier_Indicator into `Analytics.tsx`
    - _Requirements: 3.2, 4.1, 5.1, 6.5, 7.1, 38.4_

- [x] 6. Checkpoint - analytics IA
  - Ensure all tests pass, ask the user if questions arise.

- [x] 7. Establish heatmap package skeleton and shared contracts
  - [x] 7.1 Create heatmap package structure and core types
    - Create `internal/analytics/heatmap/` package with `config.go`, `score.go`, `normalize.go`, `suppress.go`, `confidence.go`, `decimate.go`, `reason.go`, `reason` constants
    - Define `ReplayHeatmapPoint`, `ReplayHeatmapDetailPoint`, `SignalComponent`, `HeatmapEmote`, `HeatmapResponse`, `HeatmapDetailResponse` structs per design Data Models
    - Define the valid Reason_Label constant set (chat_spike, seventv_spike, twitch_emote_spike, ffz_spike, viewer_spike, game_change, manual)
    - _Requirements: 8.1, 10.1, 28.1, 28.4_

  - [x] 7.2 Implement ScoringConfig and config loading
    - Implement `SignalWeights`, `ScoringConfig` (including `DensityConfidenceWeight`), `DefaultScoringConfig()` (v1 defaults), and `LoadScoringConfig()` reading `HEATMAP_SCORING_CONFIG_PATH`
    - Validate score weights sum to 1.0 (±0.001) and reject invalid configs; `DensityConfidenceWeight` is a confidence-only weight excluded from the sum
    - _Requirements: 9.1, 9.8_

  - [x] 7.3 Create frontend shared TypeScript types
    - Add `frontend/src/types/heatmap.ts` (`SignalComponent`, `HeatmapEmote`, `ReplayHeatmapPoint`, `ReplayHeatmapDetailPoint`, `HeatmapResponse`, `HeatmapDetailResponse`)
    - Add `frontend/src/types/vodMode.ts` — do NOT redeclare `VodStartResponse`; re-export the existing one with `export type { VodStartResponse } from '../api';` and add only NEW VOD-mode types (e.g. `VodStartError`)
    - _Requirements: 28.1, 28.2, 28.4_

- [x] 8. Implement the deterministic score engine (Score_Model)
  - [x] 8.1 Implement signal extraction and log/z-score normalization
    - In `normalize.go`: `ln(value+1)` log transform, per-stream z-score normalization (divide-by-zero → 0), provider spike derivation from rollup fields (`seventvEmoteCount`, twitch/ffz provider entries), top emote dominance, novelty
    - Extract signals per scoring window aligned to rollup minute boundaries
    - _Requirements: 9.2, 9.3_

  - [x] 8.2 Write property test for log transform safety _(test, optional)_
    - **Property 7: Log Transform Safety** — `ln(count+1)` finite, ≥0, `ln(1)=0`, order-preserving
    - **Validates: Requirements 9.3**

  - [x] 8.3 Write property test for per-stream z-score normalization _(test, optional)_
    - **Property 6: Per-Stream Z-Score Normalization** — mean ≈ 0, stddev ≈ 1 for ≥2 distinct values
    - **Validates: Requirements 9.2**

  - [x] 8.4 Implement EWMA smoothing and non-max suppression
    - In `normalize.go`: forward-only `ewmaSmooth(scores, span, alpha)` preserving causality
    - In `suppress.go`: `suppressPeaks(scores, threshold, radius)` retaining only local maxima within radius
    - _Requirements: 9.4, 9.5_

  - [x] 8.5 Write property test for EWMA forward-only causality _(test, optional)_
    - **Property 8: EWMA Forward-Only Causality** — changing score at index k does not alter smoothed values at j < k
    - **Validates: Requirements 9.4**

  - [x] 8.6 Write property test for non-max suppression locality _(test, optional)_
    - **Property 9: Non-Max Suppression Locality** — no two scores ≥ T remain within radius R after suppression
    - **Validates: Requirements 9.5**

  - [x] 8.7 Implement composite score and missing-window handling
    - In `score.go`: `compositeScore` with positive-surprise clamping (`max(0,z)`), `*30` scaling clamped to [0,100], early `return 0` for all-missing windows
    - Implement `ComputeHeatmap(rollups, config) → []ReplayHeatmapPoint` wiring extraction → normalization → smoothing → suppression → composite; the function is pure and receives an already-consolidated, offset-sorted rollup slice (no DB or `analytics`-package access)
    - _Requirements: 9.1, 9.6, 9.7_

  - [x] 8.8 Write property test for score output range _(test, optional)_
    - **Property 5: Score Output Range and Weight Validity** — all scores are integers in [0,100] for any valid config
    - **Validates: Requirements 9.1**

  - [x] 8.9 Write property test for missing-window zero score _(test, optional)_
    - **Property 11: Missing Window Scores Zero** — all-null signal window → score 0, no interpolation
    - **Validates: Requirements 9.7**

  - [x] 8.10 Write property test for score determinism
    - **Property 10: Score Determinism** — computing twice yields bit-for-bit identical scores, order, reasons
    - **Validates: Requirements 9.6**

  - [x] 8.11 Write fixture-based determinism test suite
    - Known rollup inputs → expected score outputs per scoring version, asserting stable output across builds
    - _Requirements: 9.10_

- [x] 9. Implement reason labels, confidence, and decimation
  - [x] 9.1 Implement reason label selection
    - In `reason.go`: select label = signal with highest individual z-score when > 1.0, else `chat_spike` fallback; attach exactly one label
    - _Requirements: 10.1, 10.2_

  - [x] 9.2 Implement top emotes attachment
    - Attach top 1–3 emotes ordered by per-window count descending, each with local id and `imageUrl` of form `/emotes/{id}/1x.webp`
    - _Requirements: 10.3_

  - [x] 9.3 Write property test for reason label selection _(test, optional)_
    - **Property 12: Reason Label Selection** — highest z-score label if > 1.0 else chat_spike; exactly one from valid set
    - **Validates: Requirements 10.1, 10.2**

  - [x] 9.4 Write property test for top emotes ordering and format _(test, optional)_
    - **Property 13: Top Emotes Ordering and Format** — 1–3 entries, count descending, imageUrl matches `/emotes/{id}/1x.webp`
    - **Validates: Requirements 10.3**

  - [x] 9.5 Implement per-signal and per-window confidence
    - In `confidence.go`: chat cap 0.3 (<35% coverage, no chat rollup), viewer cap 0.4 (zero samples / flat), density cap 0.5 (low density), emote 0.0 (dict not loaded); overall = **weighted average over available signals** (NOT a product), weighted by the scoring-config `SignalWeights` (chat→ChatRate, viewer→ViewerMomentum, emote→EmoteRate+ProviderSpike+TopEmoteDominance+Novelty) plus `DensityConfidenceWeight` for density, excluding 0.0-confidence signals; stream-level = median of window overall confidences
    - Populate `components` with per-signal `rawScore`/`weightedScore`/`confidence`, zeroed when a signal has no data
    - _Requirements: 11.1, 11.2, 11.3, 11.4, 11.5, 11.6, 11.7, 11.8, 28.2, 28.3_

  - [x] 9.6 Write property tests for confidence caps _(test, optional)_
    - **Property 14: Chat Confidence Cap** (Req 11.1), **Property 15: Viewer Confidence Cap** (Req 11.2), **Property 16: Density Confidence Cap** (Req 11.3), **Property 17: Emote Dictionary Absent Zeroes Emote Signals** (Req 11.4)
    - **Validates: Requirements 11.1, 11.2, 11.3, 11.4**

  - [x] 9.7 Write property tests for confidence composition _(test, optional)_
    - **Property 18: Overall Confidence Composition** — overall window confidence is the weighted average of available per-signal confidences (weighted by scoring-config weights + `DensityConfidenceWeight`), not a product (Req 11.6); **Property 19: Stream-Level Confidence Is Median** (Req 11.7)
    - **Validates: Requirements 11.6, 11.7**

  - [x] 9.8 Implement decimation to 720-point cap
    - In `decimate.go`: retain top 20% by score, uniformly sample remaining to ≤720, omit zero-score points, re-sort by offset; proportional window scaling for >12h streams
    - _Requirements: 12.1, 12.2, 12.3, 12.4_

  - [x] 9.9 Write property tests for decimation and compactness _(test, optional)_
    - **Property 21: Decimation Retains Top Percentile** (Req 12.2), **Property 22: Zero-Score Points Omitted** (Req 12.3), **Property 20: Response Size Compactness** ≤50 KB for 720 points (Req 12.1)
    - **Validates: Requirements 12.1, 12.2, 12.3**

- [x] 10. Checkpoint - score engine
  - Ensure all tests pass, ask the user if questions arise.

- [x] 11. Implement heatmap cache and HTTP handler
  - [x] 11.1 Add GetStreamUpdatedAt store method
    - Add `GetStreamUpdatedAt(ctx, streamID)` querying `analytics_streams.updated_at` for cache-key revision
    - _Requirements: 13.2, 29.2_

  - [x] 11.2 Implement Redis cache layer
    - In `cache.go`: key `heatmap:{streamId}:{version}:{updatedAtMs}:{window}`, get/set with 1h TTL, prefix-scan invalidation; on Redis failure compute fresh and log warning
    - _Requirements: 13.2, 29.1, 29.2, 29.3_

  - [x] 11.3 Write property test for cache key determinism _(test, optional)_
    - **Property 26: Cache Key Determinism** — same inputs → same key, different inputs → different key
    - **Validates: Requirements 29.1**

  - [x] 11.4 Implement rollup consolidation bridge (package `analytics`)
    - In package `analytics`: resolve the stream, read raw rollups, call the existing unexported `consolidateRollupsByMinute` in `internal/analytics/api.go`, and produce a consolidated, deduplicated, offset-sorted `[]MinuteRollup` to pass into `heatmap.ComputeHeatmap(rollups, config)` (design approach (b)); the `heatmap` package MUST NOT import `analytics`
    - _Requirements: 8.2_

  - [x] 11.5 Implement heatmap HTTP handler in package `analytics`
    - In package `analytics` (NOT the `heatmap` package): `GET /v1/analytics/streams/{streamId}/replay-heatmap?window=&channel=&detail=` — default window 60, validate window ∈ [10,600] → 400, missing stream → 404, no rollups → 200 empty points + confidence 0, `?detail=true` returns components; consume the consolidated `[]MinuteRollup` from task 11.4 and invoke `heatmap.ComputeHeatmap`; cache-check before compute
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7_

  - [x] 11.6 Write property test for window parameter validation _(test, optional)_
    - **Property 24: Window Parameter Validation** — non-integer or out-of-[10,600] → HTTP 400
    - **Validates: Requirements 8.4**

  - [x] 11.7 Write property test for response schema conformance _(test, optional)_
    - **Property 25: Heatmap Response Schema Conformance** — envelope and every point conform to ReplayHeatmapPoint contract
    - **Validates: Requirements 8.1, 28.1, 28.2, 28.3, 28.4**

  - [x] 11.8 Implement cache invalidation endpoint and register routes
    - Add `DELETE /v1/analytics/streams/{streamId}/replay-heatmap/cache` (204); register heatmap routes in `cmd/analytics` router; load scoring config at startup
    - _Requirements: 29.4_

  - [x] 11.9 Write integration tests for heatmap endpoint _(test, optional)_
    - Endpoint with pre-loaded rollups (httptest), cache hit/miss, rollup-write → invalidation flow
    - _Requirements: 8.1, 13.2, 29.2_

  - [x] 11.10 Write heatmap endpoint performance benchmarks _(test, optional)_
    - `testing.B` for p50 ≤100ms / p95 ≤200ms cached and ≤500ms p95 cold for ≤360 rollup points
    - _Requirements: 13.1, 13.3_

- [x] 12. Checkpoint - heatmap API
  - Ensure all tests pass, ask the user if questions arise.

- [x] 13. Implement P1 heatmap UI (analytics lane + player scrub)
  - [x] 13.1 Implement pixel-column decimation utility
    - Add `decimateToPixels(points, widthPx, totalDurationSec)` keeping max score per column; re-decimate on resize
    - _Requirements: 14.3, 24.1_

  - [x] 13.2 Write property test for pixel-column bound _(test, optional)_
    - **Property 23: Heatmap Lane Pixel-Column Bound** — at most W visual elements for width W
    - **Validates: Requirements 14.3, 24.1**

  - [x] 13.3 Implement HeatmapLane component
    - Add `frontend/src/components/analytics/HeatmapLane.tsx`: 12–24px canvas strip aligned to chart axis, theme-palette gradient, hover tooltip (HH:MM:SS, reason, ≤3 emotes, chat rate, Play action), muted empty state when no points / all major signals 0 confidence; lazy-load emotes on hover/select, cap 3 concurrent requests, no animation when tab hidden
    - _Requirements: 14.1, 14.2, 14.3, 14.4, 14.5, 24.1, 24.2, 24.3, 24.4_

  - [x] 13.4 Implement heatmap accessibility layer
    - Hidden peak buttons in `role="toolbar"`, roving tabindex (Arrow Left/Right, Enter/Space), visible focus, `aria-label` with offset/score/reason; reduced-motion disables transitions and renders animated emotes as static first frame; glow/pulse only when motion allowed (≤2s, ≤3 peaks)
    - _Requirements: 17.1, 17.2, 17.3, 17.4, 31.1, 31.3_

  - [x] 13.5 Write property test for ARIA labels on peaks _(test, optional)_
    - **Property 31: ARIA Labels on Heatmap Peaks** — each peak button aria-label contains HH:MM:SS offset, score, reason
    - **Validates: Requirements 17.4**

  - [x] 13.6 Implement HH:MM:SS duration formatter
    - Add shared formatter producing `^\d{2,}:\d{2}:\d{2}$` with MM/SS in [00,59]
    - _Requirements: 20.2_

  - [x] 13.7 Write property test for duration format _(test, optional)_
    - **Property 28: Duration Format** — formatter output matches `^\d{2,}:\d{2}:\d{2}$`
    - **Validates: Requirements 20.2**

  - [x] 13.8 Implement player progress-bar heatmap
    - Add `frontend/src/components/channel/PlayerHeatmap.tsx`: gradient strip in progress bar (≤1 element/column), hover tooltip on peaks (offset/reason/emotes) and offset-only on non-peak regions, click-to-seek; hide layer if heatmap errors or times out within 5s; reduced-motion immediate color changes
    - _Requirements: 15.1, 15.2, 15.3, 15.4, 15.5, 31.2_

  - [x] 13.9 Implement heatmap peak selection and moment drawer
    - On peak select: move chart cursor to offset, update selected-moment drawer (score, reason, offset, ≤4 emotes with name/image/provider/count, conditional Play/Jump/Clip/Export actions); show emotes only for selected/hovered peak; empty drawer state when no rollup at window
    - _Requirements: 16.1, 16.2, 16.3, 16.4_

  - [x] 13.10 Implement moment-to-clip workflow
    - Queue clip via clipper API with offset/streamId/vodId/score/topEmotes; batch queue top 5 by score (≥10 min chat data) skipping already-queued/completed; link to `/studio/{jobId}`; surface clipper auth failures inline (missing_scope, invalid_token, twitch_not_configured, vod_auth_failed) retaining selection
    - _Requirements: 23.1, 23.2, 23.3, 23.4, 36.1, 36.2_

  - [x] 13.11 Write property test for batch clip queue correctness _(test, optional)_
    - **Property 27: Batch Clip Queue Correctness** — exactly top 5 by score, excluding already-queued/completed for same stream+minute
    - **Validates: Requirements 23.3**

  - [x] 13.12 Wire heatmap lane and player heatmap into Analytics and Channel
    - Fetch heatmap via React Query, render HeatmapLane below chart and PlayerHeatmap in player; replace estimated `computeMomentScore` with server scores when available
    - _Requirements: 9.9, 14.1, 15.1, 16.1_

- [x] 14. Checkpoint - heatmap UI
  - Ensure all tests pass, ask the user if questions arise.

- [x] 15. Implement P1 live stats band and live heat
  - [x] 15.1 Implement LiveStatsBand component
    - Add `frontend/src/components/analytics/LiveStatsBand.tsx`: 15s refresh, current viewers + 5min delta, chat/min (1min & 5min avg + trend arrow), emotes/min by provider, top emote (≤3 images), confidence state; number animation 200–300ms no layout shift; 60-point sparkline canvas; reduced-motion disables animation; on error/timeout retain last values + stale indicator and retry next cycle
    - _Requirements: 18.1, 18.2, 18.3, 18.4, 18.5_

  - [x] 15.2 Write unit tests for live stats band rendering _(test, optional)_
    - Trend arrow thresholds, confidence states, stale-data fallback
    - _Requirements: 18.1, 18.5_

  - [x] 15.3 Implement Most Reacted So Far live heat section
    - Show up to 10 live moment points when ≥5 completed rollups, refresh 30s, "based on chat and emote activity" subtitle; mute last incomplete minute labeled "Collecting"
    - _Requirements: 19.1, 19.2_

  - [x] 15.4 Implement live-to-historical heatmap stitching (backend)
    - On stream close, resolve VOD id via Helix within 5 min and stitch live points to historical record using same minute-bucket offsets; otherwise retain under live id marked "unlinked" until sync/manual trigger
    - _Requirements: 19.3, 19.4_

  - [x] 15.5 Write integration test for live-to-historical stitching _(test, optional)_
    - VOD resolves within 5 min → stitched; no resolution → unlinked retained
    - _Requirements: 19.3, 19.4_

- [x] 16. Checkpoint - live stats
  - Ensure all tests pass, ask the user if questions arise.

- [x] 17. Implement P2 VOD review mode and same-page sync
  - [x] 17.1 Implement playhead sync store
    - Add `frontend/src/stores/playheadStore.ts`: Zustand `{ streamId, offsetSeconds, isPlaying, vodId, setPlayhead, setPlaying, reset }`, updated by VOD player at 1Hz
    - _Requirements: 22.1_

  - [x] 17.2 Implement VOD mode controls
    - Add `frontend/src/components/channel/VodModeControls.tsx`: banner with VOD id + offset HH:MM:SS, hide "Jump to Live" (control bar + diagnostics), show current/total duration (placeholder when unknown), "Back to live channel" → `/c/{login}`, conditional "Back to Analytics"
    - _Requirements: 1.1, 1.2, 20.1, 20.2, 20.3, 20.4, 20.5_

  - [x] 17.3 Implement same-page chart cursor sync
    - Chart cursor tracks playback via shared React state (≥1Hz) only when chart + player on same page for same stream; chart click seeks within ±1s; guard when player absent/inactive or stream mismatch → standard cursor
    - _Requirements: 22.1, 22.2, 22.3_

  - [x] 17.4 Write unit tests for cursor sync guard conditions _(test, optional)_
    - Same-page match syncs; mismatch/inactive falls back to standard cursor
    - _Requirements: 22.3_

- [x] 18. Implement P2 VOD chat message storage and replay
  - [x] 18.1 Create VOD chat storage schema and migration
    - Add migration for `analytics_vod_chat_messages` table (with `UNIQUE (stream_id, message_id)`) and indexes per design Database Schema
    - _Requirements: 27.1_

  - [x] 18.2 Implement chatreplay model, store, and sink interface
    - Add `internal/analytics/chatreplay/model.go` (`VODChatMessage`, `EmoteFrag`), `store.go` (CRUD + paginated query ordered by offset asc; `ON CONFLICT DO NOTHING` upsert on `(stream_id, message_id)`), and `sink.go` (`Sink` interface: `Add`, `FlushSegment`, `Flush`; `nil` sink = no-op)
    - _Requirements: 27.1, 27.4, 27.5_

  - [x] 18.3 Implement message sanitization and privacy persistence
    - Strip control chars, truncate to configurable max (default 500), remove URLs/identifiable metadata; persist only display name + HMAC-hashed sender (server-side salt; no raw user id/IP/token); drop URL-only/spam/bot messages
    - _Requirements: 27.2, 30.1, 30.5_

  - [x] 18.4 Persist sanitized individual chat messages during VOD GQL sync
    - Modify BOTH the serial (`fetchVODCommentsSerial`) and parallel (`fetchVODCommentsParallel`) GQL fetch paths in `internal/analytics/sync_gql_parallel.go` to write each sanitized message into the chatreplay `Sink` (`sink.Add`) at the same per-edge point that appends text to `commentsMap`, leaving rollup aggregation untouched
    - Reuse the existing `gqlCommentDeduper` for comment-id dedupe so a message is buffered only on first sighting of `edge.Node.ID`; rely on the `UNIQUE (stream_id, message_id)` upsert for idempotency
    - Checkpoint-resume compatibility: parallel path calls `sink.FlushSegment(startMinute, endMinute)` from the existing per-segment `onSegmentDone`/`patchChatRollupsForSegment` hook tied to `analytics_sync_checkpoints`; serial path flushes at cursor/offset checkpoint boundaries with a final `Flush`; dedupe spans the parallel→serial fallback via the shared deduper
    - _Requirements: 27.1, 27.2, 30.1_

  - [x] 18.5 Write property tests for sanitization and privacy _(test, optional)_
    - **Property 33: VOD Chat Message Sanitization** (Req 27.2), **Property 34: Privacy — No Raw User IDs in Storage** (Req 30.1)
    - **Validates: Requirements 27.2, 30.1**

  - [x] 18.6 Implement chat replay endpoint
    - `GET /v1/analytics/streams/{streamId}/chat-replay?offsetStart=&offsetEnd=&limit=&cursor=` ordered by offset asc, ≤200 default / ≤500 max per page, cursor token; empty → 200 empty array + unavailable flag
    - _Requirements: 27.4, 27.5, 27.6_

  - [x] 18.7 Write property test for chat replay pagination _(test, optional)_
    - **Property 32: VOD Chat Message Pagination** — offsets within range, ascending, page size ≤ min(limit,500) default 200
    - **Validates: Requirements 27.4, 27.5**

  - [x] 18.8 Implement retention cleanup and admin purge
    - `retention.go` scheduled purge (≥1/24h) using `ANALYTICS_VOD_CHAT_RETENTION_DAYS` (default 90), log stream id + purged count (no content); `DELETE /v1/analytics/streams/{streamId}/chat-messages` (204)
    - _Requirements: 27.3, 30.2, 30.3, 30.4_

  - [x] 18.9 Implement VOD chat replay panel (frontend)
    - Show messages for the minute bucket at current offset, fetch next page on bucket crossing within 500ms, empty-minute indicator distinct from no-data, sync CTA when no persisted messages, partial-progress indicator during in-progress sync
    - _Requirements: 21.1, 21.2, 21.3, 21.4, 21.5_

  - [x] 18.10 Write unit tests for chat replay UI states _(test, optional)_
    - Bucket crossing update, empty-minute vs no-data, sync CTA, partial sync progress
    - _Requirements: 21.1, 21.3, 21.4, 21.5_

- [x] 19. Checkpoint - P2 VOD review and chat replay
  - Ensure all tests pass, ask the user if questions arise.

- [x] 20. Visual regression coverage
  - [x] 20.1 Create seeded fixture / mocked API responses for visual regression _(test, optional)_
    - Provide deterministic CI data for the Playwright test: either seed analytics rollups for the reference stream into the test database or serve mocked `GET /v1/analytics/streams/{id}` + `replay-heatmap` responses (with peak top-emotes), so the test does not depend on a live `localhost:8090` stream
    - _Requirements: 32.1, 32.5_

  - [x] 20.2 Write Playwright visual acceptance test _(test, optional)_
    - Load the analytics view backed by the task 20.1 fixture/mocks (local acceptance target remains `http://localhost:8090/analytics/caedrel/2026-06-11`), full-page screenshot at 1920×1080 (heatmap lane, right rail Moments, stat cards, chart), compare to baseline at 0.1% threshold with diff artifact, verify emote images render in peak tooltips on hover; deterministic in CI
    - _Requirements: 32.1, 32.2, 32.3, 32.4, 32.5_

- [x] 21. Final checkpoint - full feature
  - Ensure all tests pass (`go test ./internal/analytics/...`, `cd frontend && npm run build`), ask the user if questions arise.

## Notes

- Test sub-tasks are tagged `_(test, optional)_` and rendered as real checkboxes (`- [ ]`, not `- [ ]*`); they can be skipped for a faster MVP. Core implementation tasks are never optional.
- **Required (non-optional) tests** because they guard trust/correctness: P0 VOD deep-link smoke test (3.3), backend VOD error coverage tests (3.4), release bundle verification smoke check (3.5), score determinism test (8.10), and fixture-based determinism suite (8.11).
- Phase ordering honors **P0 VOD playback trust first**: setup (task 1) → P0 VOD trust (tasks 2–4) → P1 analytics IA (5–6) → heatmap package/scoring engine (7–10) → heatmap API (11–12) → heatmap UI (13–14) → live stats (15–16) → P2 VOD review mode (17) → P2 chat replay (18–19) → visual regression (20) → final checkpoint (21).
- Property test sub-tasks each reference a specific Correctness Property (1–34) and the requirement clause it validates; backend uses Go `rapid`, frontend uses `fast-check` (via `fc.assert(fc.property(...))`, no `test.prop` helper), ≥100 iterations each, tagged `// Feature: moment-timeline, Property {N}: {title}`.
- The heatmap HTTP handler lives in package `analytics` (task 11.5) and bridges rollup consolidation (task 11.4, design approach (b)): it calls the unexported `consolidateRollupsByMinute` and passes a consolidated, deduplicated, offset-sorted `[]MinuteRollup` into the pure `heatmap.ComputeHeatmap`. The `heatmap` package never imports `analytics`.
- VOD chat replay persistence (task 18.4) is an additive side-channel `Sink` written from both GQL fetch paths; it does not change existing rollup aggregation and is idempotent under checkpoint resume via the shared `gqlCommentDeduper` plus the `UNIQUE (stream_id, message_id)` upsert.
- Frontend VOD-mode code reuses the existing `VodStartResponse` from `frontend/src/api.ts` (re-exported, not redeclared); only new VOD-mode types like `VodStartError` are added (task 7.3).
- Requirements 37 (Local Replay Telemetry) and 39 (Cross-Tab Playhead Sync) are future P3 / optional follow-ups and are intentionally excluded from this implementation plan.
- Each task builds on prior steps and ends by wiring new code into existing services (`cmd/analytics`, `Analytics.tsx`, `Channel.tsx`).

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "1.2"] },
    { "id": 1, "tasks": ["2.1", "2.4", "3.1", "5.1", "5.3", "5.5", "5.7", "5.9", "5.10", "7.1", "7.2", "7.3"] },
    { "id": 2, "tasks": ["2.2", "2.3", "2.5", "2.6", "3.2", "5.2", "5.4", "5.6", "5.8", "5.11", "8.1", "8.4", "11.1", "17.1", "18.1"] },
    { "id": 3, "tasks": ["3.3", "3.4", "3.5", "8.2", "8.3", "8.5", "8.6", "8.7", "15.1", "15.3", "15.4", "18.2", "18.3"] },
    { "id": 4, "tasks": ["8.8", "8.9", "8.10", "8.11", "9.1", "9.2", "9.5", "9.8", "15.2", "15.5", "17.2", "17.3", "18.4", "18.5", "18.6", "18.8"] },
    { "id": 5, "tasks": ["9.3", "9.4", "9.6", "9.7", "9.9", "11.2", "11.4", "17.4", "18.7", "18.9"] },
    { "id": 6, "tasks": ["11.3", "11.5", "18.10"] },
    { "id": 7, "tasks": ["11.6", "11.7", "11.8", "13.1", "13.6"] },
    { "id": 8, "tasks": ["11.9", "11.10", "13.2", "13.3", "13.7"] },
    { "id": 9, "tasks": ["13.4", "13.8", "13.9"] },
    { "id": 10, "tasks": ["13.5", "13.10", "13.12"] },
    { "id": 11, "tasks": ["13.11", "20.1"] },
    { "id": 12, "tasks": ["20.2"] }
  ]
}
```
