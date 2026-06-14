# Requirements Document

## Introduction

The Unified Moment Timeline collapses several Streamclone roadmap ideas into one product theme: "Own the moment timeline." VOD playback, chat replay, top moments, most-replayed heatmap, 7TV/Twitch/FFZ emote spikes, and clip queue all use the same playhead. The feature spans seven phases: VOD playback trust, analytics IA redesign, replay heatmap API, heatmap UI, live statistics, VOD review mode, and integrated moment workflow.

**Implementation priority (from 2026-06 audit):**

| Phase | Theme | Priority |
|-------|--------|----------|
| 1 | VOD playback trust + release verification | **P0** — regressed in deployed build at audit time |
| 2 | Analytics IA + honest empty states | **P1** |
| 3 | Replay heatmap API + score model | **P1** |
| 4 | Heatmap UI (analytics lane + player scrub) | **P1** |
| 5 | Live stats band + live heat | **P1** |
| 6 | VOD review mode + same-page playhead sync | **P2** |
| 7 | Chat replay + batch clip workflow | **P2** (chat replay blocked on Req 27) |

**Naming honesty:** Until local playback seek/replay telemetry exists, user-facing copy SHALL use **Most Reacted** (chat + emote + viewer rollups), not **Most Replayed** (YouTube-style watch replay counts). See glossary entries `Most_Reacted` and `Most_Replayed`.

**Related docs:** [docs/product-roadmap.md](../../../docs/product-roadmap.md), [docs/repo-maintenance.md](../../../docs/repo-maintenance.md).

## Glossary

- **Moment_Timeline**: The unified playback-aligned timeline combining heatmap scores, chart data, emote spikes, and clip actions into a single synchronized playhead across analytics and VOD player surfaces.
- **Heatmap_Service**: The backend component within `internal/analytics` that computes per-window replay heatmap scores from merged rollups and serves them via the replay-heatmap endpoint.
- **Score_Model**: The deterministic algorithm that produces a 0-100 score per time window using viewer momentum, chat rate, emote rate, provider spike signals, top emote dominance, and novelty.
- **Heatmap_Lane**: The thin visual heat band rendered below the analytics chart timeline and inside the VOD player progress bar showing score intensity per time window.
- **VOD_Mode**: The channel workspace state activated by a `?vod={id}&offset={seconds}` deep link, where the player relays an archived Twitch broadcast via the existing VOD relay API.
- **Analytics_Service**: The Go backend service (`cmd/analytics`, `internal/analytics`) that collects per-minute viewer, chat, and emote rollups for tracked channels.
- **Channel_Workspace**: The frontend component (`Channel.tsx`) hosting the player, chat panel, controls, and overlay surfaces for live and VOD playback.
- **Rollup**: A per-minute analytics data point containing viewer count, chat message count, total emote count, 7TV emote count, and per-emote frequency maps.
- **Non_Max_Suppression**: An algorithm that suppresses adjacent peaks below a local maximum so that one sustained event produces one readable peak rather than many identical adjacent peaks.
- **Confidence**: A 0-1 multiplier applied to raw heatmap scores that drops when chat coverage is missing, viewer data is flat or partial, rollups are sparse, or the emote dictionary is not loaded.
- **Reason_Label**: A classification tag attached to each heatmap peak indicating the dominant signal (chat_spike, seventv_spike, twitch_emote_spike, ffz_spike, viewer_spike, game_change, manual).
- **Live_Stats_Band**: A compact UI strip on the live analytics page showing current viewers, chat rate, emote rate, top emote, and data confidence in real time.
- **Decimation**: The process of reducing heatmap data points to match visible pixel columns so that long VODs do not produce more DOM or canvas elements than the viewport can display.
- **Right_Rail**: The tabbed panel to the right of the analytics chart containing Moments, Emotes, Clips, and Sync tabs.
- **Deep_Link**: The URL pattern `/c/{login}?vod={id}&offset={seconds}` that launches VOD mode from analytics or external sources.
- **ReplayHeatmapPoint**: The canonical data contract for a single scored point in the heatmap response, containing offsetSeconds, durationSeconds, score, confidence, reason, components (per-signal breakdown with individual confidence), topEmotes, vodId, streamId, and minuteTs.
- **Scoring_Version**: A versioned configuration bundle (identifier string, weights, smoothing parameters, suppression thresholds) that produces deterministic scores; changing any parameter produces a new version.
- **Tier_Indicator**: A UI badge or label communicating whether the analytics profile is Core (Helix summary only, no minute charts) or Analytics Active (scraper profile running, minute-level data available).

## Reference Fixture

The primary visual reference for populated analytics UI is the synced Caedrel historical page at `http://localhost:8090/analytics/caedrel/2026-06-11`. This is the only fully synced stream in the local stack and should be used as the baseline for Playwright visual acceptance tests, heatmap density checks, and moment scoring fixture validation.
- **Most_Reacted**: The v1 heatmap product label for reaction-density scores derived from minute rollups (chat, emotes, viewers). Used for live ("Most Reacted So Far") and historical streams until replay telemetry ships.
- **Most_Replayed**: The future label reserved for scores that blend rollup reaction signals with **local-only** playback seek/replay telemetry. SHALL NOT be shown until that telemetry is implemented and disclosed in UI copy.
- **Provider_Spike_Signal**: Per-provider emote velocity derived from rollup fields (`seventvEmoteCount`, top-emote provider for Twitch/FFZ) used by the Score_Model and `Reason_Label` selection alongside generic emote rate.
- **VOD_Identifier**: A Twitch video id matching `^\d{5,20}$` after client-side normalization (strip whitespace, reject `videos/` URL prefixes, reject empty values before API calls).

## Requirements

### Requirement 1: VOD Deep Link Landing

**User Story:** As an analytics user, I want the "Play in Streamclone" deep link to always land in an obvious VOD mode, so that I trust the player is showing the correct archived broadcast at the correct offset.

#### Acceptance Criteria

1. WHEN the Channel_Workspace receives a URL with `vod` and `offset` query parameters, THE Channel_Workspace SHALL display a VOD mode banner showing the VOD identifier (numeric, 5–20 digits), the current playback offset formatted as HH:MM:SS, a "Back to live" action that removes VOD parameters and starts live relay for the channel, and a "Back to Analytics" action that navigates to the Analytics page for the same channel.
2. WHEN the VOD mode banner is visible, THE Channel_Workspace SHALL hide the "Jump to Live" control that is only relevant during live playback.
3. WHEN the Channel_Workspace receives a URL with `vod` and `offset` query parameters, THE Channel_Workspace SHALL normalize the `vod` value to a VOD_Identifier before calling the relay API and SHALL send `POST /v1/stream/vod/start` with JSON field `vod_id` (not `vodId` or query-only passthrough), the normalized identifier, and `offset_seconds` as a whole number ≥ 0.
4. WHEN the user activates "Play VOD" from the channel workspace VODs tab or "Play in Streamclone" from analytics, THE Channel_Workspace SHALL use the same Deep_Link pattern and VOD_Mode behavior as analytics deep links.
5. WHEN the VOD relay returns a valid HLS URL (HTTP 200 with a non-empty `hlsUrl` field containing `hlsUrl`, `offset_seconds`, and `seek_seconds`), THE Channel_Workspace SHALL begin playback within 15 seconds of the relay response and seek to `Math.max(0, offset_seconds - seek_seconds)` within a tolerance of ±2 seconds, so the user lands at the requested VOD moment after accounting for relay preroll.
6. IF the `vod` query parameter is missing, empty, or does not normalize to a valid VOD_Identifier, THEN THE Channel_Workspace SHALL display an error message indicating the VOD identifier is invalid and SHALL NOT request VOD relay startup.
7. IF the VOD relay request fails with a structured API error code, THEN THE Channel_Workspace SHALL follow Requirement 2 for user-visible copy and actions; THE Channel_Workspace SHALL NOT map all failure codes to a single generic "deleted from Twitch" message.

### Requirement 2: VOD Start Failure States

**User Story:** As a user attempting VOD playback, I want clear actionable error states when VOD relay fails, so that I understand whether the problem is fixable and what to do next.

#### Acceptance Criteria

1. IF the VOD relay returns an `invalid_vod_id` error (HTTP 400), THEN THE Channel_Workspace SHALL display a message indicating the VOD link is invalid or the analytics data is stale, with an action to return to analytics.
2. IF the VOD relay returns a `vod_unavailable` error (HTTP 404), THEN THE Channel_Workspace SHALL display a message indicating the VOD is deleted, subscriber-only, or unpublished, with an action to open the stream on Twitch.
3. IF the VOD relay returns an `upstream_token_failed` error (HTTP 502), THEN THE Channel_Workspace SHALL display a message indicating an upstream authentication issue and suggest checking token configuration.
4. IF the VOD relay returns a `capacity_reached` error (HTTP 503), THEN THE Channel_Workspace SHALL display a message indicating relay capacity is reached, suggest trying again later, and offer a retry action that re-sends the VOD start request.
5. IF the VOD relay returns an `hls_not_ready` error (HTTP 504) after worker spawn, THEN THE Channel_Workspace SHALL display a message indicating the relay started but MediaMTX did not publish in time, with a retry action that re-sends the VOD start request up to 2 additional attempts.
6. IF the VOD relay returns a `vod_start_failed` error (HTTP 502) not matching any specific error code above, THEN THE Channel_Workspace SHALL display a generic message indicating VOD playback failed with the server-provided error detail, and offer a retry action.
7. FOR ALL retryable error states (`capacity_reached`, `hls_not_ready`, `vod_start_failed` with retryable flag), THE Channel_Workspace SHALL display a retry action; for non-retryable errors (`invalid_vod_id`, `vod_unavailable`), THE Channel_Workspace SHALL NOT display a retry action.
8. IF HLS playback fails with repeated HTTP 401 on variant playlists (`main_stream.m3u8`) through the Caddy proxy while the relay start succeeded, THEN THE Channel_Workspace SHALL display guidance that local HLS proxy auth may be misconfigured (MediaMTX `hlsCDNSecret` / Caddy Bearer mismatch) and suggest a hard refresh after stack restart, rather than attributing the failure to Twitch VOD removal.

### Requirement 3: Analytics Right Rail Redesign

**User Story:** As an analytics user, I want the right rail to default to Moments, so that I immediately see the strongest moments when opening a stream's analytics.

#### Acceptance Criteria

1. THE Right_Rail SHALL include tabs labeled Moments, Emotes, Clips, and Sync in that order; additional tabs may be appended after Sync in future iterations.
2. WHEN the analytics page loads or the user navigates to a different stream within the analytics view, THE Right_Rail SHALL display the Moments tab as the active panel.
3. WHEN the user selects a different tab, THE Right_Rail SHALL display the corresponding panel content and retain the selection until the user navigates to a different stream or performs a full page reload.
4. IF the Moments tab is active and no minute-level rollup data is available for the current stream, THEN THE Right_Rail SHALL display an empty state message indicating that chat sync is needed to surface moments.

### Requirement 4: Unified Sync CTA

**User Story:** As an analytics user, I want a single consistent sync action label, so that I am not confused by multiple competing sync buttons with different wording.

#### Sync CTA Label Table

| Stream State | CTA Label | Disabled? | Shown In |
|---|---|---|---|
| No viewer samples, no chat rollups | "Sync chat & viewers" | No | Header, chart empty, right rail, sync panel |
| Viewer samples exist, no chat rollups | "Sync chat & emotes" | No | Header, chart empty, right rail, sync panel |
| Sync in progress | "Syncing…" | Yes (all placements) | Header, chart empty, right rail, sync panel |
| Both viewer samples and chat rollups exist | Hidden (or "Re-sync" contextual) | — | Sync panel only |

#### Acceptance Criteria

1. WHEN a historical stream is selected and sync is available, THE Analytics_Service frontend SHALL display the sync CTA label from the table above in every placement where it appears (header bar, chart empty state, right rail sync area, and sync panel) for that stream; all placements SHALL use the identical label string for the current stream state.
2. WHEN a stream has no chat rollups and no viewer samples, THE Analytics_Service frontend SHALL show the sync CTA with the label "Sync chat & viewers".
3. WHEN a stream has viewer samples but no chat rollups, THE Analytics_Service frontend SHALL show the sync CTA with the label "Sync chat & emotes".
4. WHILE a sync operation is in progress for the selected stream, THE Analytics_Service frontend SHALL replace the sync CTA label with "Syncing…" in all placements, and SHALL disable the sync action to prevent duplicate triggers.
5. WHEN a stream already has both viewer samples and chat rollups covering the stream timeline, THE Analytics_Service frontend SHALL hide the primary sync CTA from header, chart empty, and right rail placements, and MAY show a "Re-sync" option in the sync panel only.

### Requirement 5: Mobile Chart Priority

**User Story:** As a mobile analytics user, I want to see the chart and moment actions before scrolling through the stream archive, so that I can quickly reach the primary analytics content.

#### Acceptance Criteria

1. WHILE the viewport width is below 1024 px (Tailwind `lg` breakpoint), THE Analytics_Service frontend SHALL render the chart and moment controls above the stream archive list in DOM and visual order.
2. WHILE the viewport width is below 1024 px, THE Analytics_Service frontend SHALL render the stream archive in a collapsed state that shows at most 2 stream rows by default, with a visible toggle control that expands the full list on activation.
3. WHEN a user activates the stream archive expand toggle while the viewport width is below 1024 px, THE Analytics_Service frontend SHALL reveal the complete stream list and change the toggle label to indicate the list can be collapsed again.

### Requirement 6: Stats-Only Card Honesty

**User Story:** As an analytics user, I want stats cards to show source-appropriate placeholders instead of misleading zeroes, so that I understand the data coverage without confusion.

#### Acceptance Criteria

1. WHEN a stream has TwitchTracker averages (avgViewers or peakViewers greater than zero) but zero rollup rows where viewerSamples or chatCount is greater than zero, THE Analytics_Service frontend SHALL display the text "Stats only" in place of the numeric value for the Chat and Emote Uses stat cards, and SHALL display the TwitchTracker-sourced values for Average, Peak, and Duration stat cards.
2. WHEN a stream detail returns state "not_collected" and the stream has no avgViewers, no peakViewers, and no rollup data, THE Analytics_Service frontend SHALL display the text "Needs sync" in place of the numeric value for the Current, Average, Peak, Chat, and Emote Uses stat cards.
3. WHILE a stream detail state is "live" and the total count of non-missing rollup rows with viewerSamples greater than zero is fewer than 2, THE Analytics_Service frontend SHALL display the text "Collecting" in place of the numeric value for the Chat and Emote Uses stat cards, and SHALL display any available live viewer counts for Current, Average, and Peak stat cards.
4. IF the stream transitions from fewer than 2 non-missing rollups to 2 or more non-missing rollups while in "live" state, THEN THE Analytics_Service frontend SHALL replace the "Collecting" placeholder with the computed numeric values for Chat and Emote Uses stat cards within the next data refresh cycle.
5. WHEN a stat card displays a placeholder text ("Stats only", "Needs sync", or "Collecting"), THE Analytics_Service frontend SHALL render the placeholder in a visually distinct muted style differentiable from numeric stat values.

### Requirement 7: Live Empty State Consistency

**User Story:** As a user viewing a live analytics page, I want the chart empty state to not contradict the stream rail that shows active collection, so that the page feels coherent.

#### Acceptance Criteria

1. WHILE the stream rail shows a stream with a "Collecting now" badge and that stream is selected, THE Analytics_Service frontend SHALL NOT display "No recent data" as the chart empty message.
2. WHILE live collection is active for the selected channel but fewer than two rollup minutes exist, THE Analytics_Service frontend SHALL display a "Collecting first minutes" message with an animated activity indicator.
3. WHEN the rollup count for the selected live stream transitions from fewer than two to two or more, THE Analytics_Service frontend SHALL replace the "Collecting first minutes" message with the chart visualization within the next data refresh cycle.

### Requirement 8: Replay Heatmap Endpoint

**User Story:** As a frontend developer, I want a dedicated API endpoint that returns deterministic heatmap scores from merged rollups, so that I can render the heatmap without computing scores client-side.

#### Acceptance Criteria

1. WHEN `GET /v1/analytics/streams/{streamId}/replay-heatmap?window={seconds}` is called with a valid stream identifier that has rollup data, THE Heatmap_Service SHALL return a JSON response containing the stream identifier, window size in seconds, an overall confidence value between 0 and 1, an array of scored points (each with offset in seconds, score 0-100, reason label, per-signal component breakdown, per-signal confidence values, and top emotes), a scoring version identifier, and an update timestamp in milliseconds.
2. THE Heatmap_Service SHALL compute scores from rollups produced by `consolidateRollupsByMinute` to ensure merged and deduplicated input data.
3. WHEN the `window` query parameter is omitted, THE Heatmap_Service SHALL default to a 60-second window size.
4. IF the `window` query parameter is present but is not an integer between 10 and 600 inclusive, THEN THE Heatmap_Service SHALL return a 400 response with an error message indicating the valid window range.
5. WHEN the stream has no rollups, THE Heatmap_Service SHALL return a 200 response with an empty points array and confidence of zero.
6. IF the `streamId` path parameter does not match an existing stream, THEN THE Heatmap_Service SHALL return a 404 response with an error message indicating the stream was not found.
7. WHEN the `channel` query parameter is provided and non-empty, THE Heatmap_Service SHALL use it as an optimization hint for channel context resolution; WHEN the `channel` query parameter is omitted or empty, THE Heatmap_Service SHALL resolve the channel from the stream identifier.

### Requirement 9: Heatmap Score Computation

**User Story:** As a product owner, I want heatmap scores to be deterministic, versioned, and tunable, so that the same rollup data always produces the same heatmap output for a given scoring version, and weights can be adjusted as fixture data proves what feels right.

#### Acceptance Criteria

1. THE Score_Model SHALL produce a score between 0 and 100 inclusive for each time window by combining weighted signal components: chat rate, emote rate, viewer momentum, **Provider_Spike_Signal** (max of normalized 7TV/Twitch/FFZ provider rates for the window), top emote dominance, and novelty; the default weights SHALL be configurable per scoring version (initial defaults: chat rate 0.25, emote rate 0.20, viewer momentum 0.20, provider spike 0.15, top emote dominance 0.10, novelty 0.10 — weights MUST sum to 1.0 per version) and SHALL NOT be hardcoded in application source.
2. THE Score_Model SHALL normalize signals within each stream independently by computing z-scores from the stream's own per-window distribution, not against a cross-stream global baseline.
3. THE Score_Model SHALL apply a natural-log transformation as `ln(value + 1)` to chat count and emote count before computing z-scores, so that zero counts map to zero and no undefined values are produced.
4. THE Score_Model SHALL apply smoothing across neighboring windows using an exponentially weighted moving average with a configurable span (default 3 windows, alpha = 0.5), applied as a forward pass only to preserve causality; the smoothing parameters SHALL be part of the scoring version configuration.
5. THE Score_Model SHALL apply Non_Max_Suppression to windows scoring above a configurable threshold (default 20) so that one sustained event produces at most one peak within a configurable suppression radius that defaults to 3 windows and accepts values between 1 and 10 windows.
6. THE Score_Model SHALL produce identical score outputs for all identical rollup inputs with the same scoring version, using no random seeds, non-deterministic sort orders, or floating-point order-dependent accumulations.
7. IF a time window contains no rollup data (all signal values are null or missing), THEN THE Score_Model SHALL assign a score of 0 for that window rather than interpolating from neighbors.
8. THE Score_Model SHALL carry a version identifier (string, e.g. "v1") that is returned in the heatmap response; changing weights, smoothing, or threshold parameters SHALL produce a new version identifier.
9. THE Score_Model SHALL supersede the existing frontend `computeMomentScore` function in Analytics.tsx once the heatmap endpoint ships; until then, THE Analytics_Service frontend MAY continue to use `computeMomentScore` for the Moments list but SHALL label those scores as estimated and SHALL replace them with server-computed scores when heatmap data is available.
10. THE Score_Model SHALL be validated against a fixture test suite containing known rollup inputs and expected score outputs for each scoring version, ensuring determinism across builds.

### Requirement 10: Heatmap Reason Labels

**User Story:** As an analytics user, I want each heatmap peak to show why it scored high, so that I can understand the nature of the moment at a glance.

#### Acceptance Criteria

1. THE Heatmap_Service SHALL attach exactly one primary Reason_Label to each scored point from the set: chat_spike, seventv_spike, twitch_emote_spike, ffz_spike, viewer_spike, game_change, manual.
2. THE Heatmap_Service SHALL select the Reason_Label by choosing the signal component with the highest individual z-score for that window; if no signal exceeds a z-score of 1.0, THE Heatmap_Service SHALL assign the label `chat_spike` as the default fallback.
3. THE Heatmap_Service SHALL include the top 1-3 emotes for each scored point when emote data is available, ordered by per-window count descending, each containing the local emote identifier and image URL in the format `/emotes/{id}/1x.webp`.

### Requirement 11: Heatmap Confidence

**User Story:** As a user, I want the heatmap to communicate when its data is unreliable and show which signals are missing, so that I do not mistake low-confidence scores for genuinely uninteresting segments and useful moments with partial data are not hidden.

#### Acceptance Criteria

1. WHEN the stream has chat coverage below 35% of the stream time span, THE Heatmap_Service SHALL cap the chat-signal confidence for windows without chat rollups at 0.3.
2. WHEN viewer data for a window has zero viewer samples or all viewer values are identical across the stream (mock/stats-only), THE Heatmap_Service SHALL cap the viewer-signal confidence for affected windows at 0.4.
3. WHEN rollup density is below one rollup point per two scoring windows, THE Heatmap_Service SHALL cap the density confidence for affected windows at 0.5.
4. WHEN the emote dictionary is not loaded for the channel, THE Heatmap_Service SHALL multiply the emote-related signal components by 0.0 and set the emote-signal confidence to 0.0.
5. THE Heatmap_Service SHALL include per-signal confidence values (chat, viewer, emote, density) in each scored point, in addition to the overall window confidence, so the frontend can show which signals are absent or degraded.
6. THE Heatmap_Service SHALL compute the per-window overall confidence as the weighted average of the per-signal confidence values that have data (each signal capped per criteria 1-4: chat 0.3, viewer 0.4, density 0.5, emote 0.0 when the dictionary is absent), weighted by the signal weights from the scoring configuration, excluding signals whose confidence is 0.0 from the average, and clamped to the range 0.0 to 1.0.
7. THE Heatmap_Service SHALL include an overall stream-level confidence value in the response, computed as the median of all per-window overall confidence values, between 0.0 and 1.0.
8. WHEN no degradation conditions apply, THE Heatmap_Service SHALL report a confidence of 1.0 for the stream and for each window and each signal.

### Requirement 12: Heatmap Response Compactness

**User Story:** As a frontend developer loading heatmaps for long VODs, I want the response payload to stay compact, so that the page does not stall on large downloads.

#### Acceptance Criteria

1. THE Heatmap_Service SHALL produce a response payload of 50 KB or less for streams up to 12 hours in duration after applying decimation to retain at most 720 scored points (one per minute at 60-second windows).
2. WHEN the number of scored windows exceeds 720 points, THE Heatmap_Service SHALL reduce points by always retaining windows with scores in the top 20% and applying uniform sampling to the remaining lower-score windows until the total does not exceed 720.
3. THE Heatmap_Service SHALL omit zero-score points from the response array.
4. FOR streams exceeding 12 hours in duration, THE Heatmap_Service SHALL apply the same decimation rules but with a proportionally increased window size to maintain the 720-point cap.

### Requirement 13: Heatmap Endpoint Performance

**User Story:** As a user navigating between streams, I want the heatmap to load quickly, so that the analytics page feels responsive.

#### Acceptance Criteria

1. THE Heatmap_Service SHALL respond within 100 milliseconds at the 50th percentile and within 200 milliseconds at the 95th percentile for streams with up to 360 rollup points (6 hours at 60-second windows) when rollup data is already in the database.
2. WHEN the same stream is requested multiple times with unchanged rollup data, THE Heatmap_Service SHALL serve cached results without recomputing scores; the cache SHALL be invalidated when new rollup rows are written for that stream.
3. WHEN the cache is empty for a requested stream, THE Heatmap_Service SHALL compute and return scores within 500 milliseconds at the 95th percentile for streams with up to 360 rollup points.

### Requirement 14: Analytics Heatmap Lane

**User Story:** As an analytics user, I want a thin heatmap band below the timeline chart, so that I can see moment intensity at a glance while examining the full viewer/chat/emote chart.

#### Acceptance Criteria

1. WHEN heatmap data is available for the selected stream, THE Analytics_Service frontend SHALL render a Heatmap_Lane below the timeline chart as a horizontal strip between 12 and 24 CSS pixels tall, aligned to the same time axis and visible width as the chart above it.
2. THE Heatmap_Lane SHALL use a color gradient from the existing chart theme palette to represent score intensity, mapping score 0 to the palette's lowest-intensity color and score 100 to the palette's highest-intensity color.
3. THE Heatmap_Lane SHALL render as one SVG path or canvas strip with no more than one visual element per visible pixel column.
4. WHEN the user hovers over a scored point with a score greater than zero in the Heatmap_Lane, THE Analytics_Service frontend SHALL display a tooltip showing the time offset formatted as HH:MM:SS, the Reason_Label, up to 3 top emote images using local emote URLs, the chat rate in messages per minute, and a "Play" action that navigates to the Channel_Workspace VOD deep link at the hovered offset.
5. IF the heatmap endpoint returns an empty points array or ALL major per-signal confidence values (chat, viewer, and emote) are 0.0 for the stream, THEN THE Analytics_Service frontend SHALL render the Heatmap_Lane in a muted empty state with no color gradient and no interactive tooltips; the lane SHALL NOT be muted when at least one major signal has non-zero confidence.

### Requirement 15: VOD Player Heatmap

**User Story:** As a user watching a VOD in Streamclone, I want to see the heatmap behind the player progress bar, so that I can skip to the most interesting moments during playback.

#### Acceptance Criteria

1. WHILE in VOD_Mode and heatmap data is available, THE Channel_Workspace SHALL render the heatmap as a color-gradient strip within the player progress bar using no more than one visual element per visible pixel column.
2. WHEN the user hovers over a heatmap peak in the player progress bar, THE Channel_Workspace SHALL display a tooltip showing the time offset, the Reason_Label, and up to 3 top emote images using local CDN URLs.
3. WHEN the user hovers over a non-peak region of the player progress bar where heatmap data exists, THE Channel_Workspace SHALL display a tooltip showing the time offset without reason or emote details.
4. WHEN the user clicks a point on the heatmap strip in the player progress bar, THE Channel_Workspace SHALL seek playback to the time offset corresponding to that position.
5. IF the replay-heatmap endpoint returns an error or the request times out within 5 seconds, THEN THE Channel_Workspace SHALL hide the heatmap layer and continue displaying the standard progress bar without interrupting playback.

### Requirement 16: Heatmap Peak Selection

**User Story:** As an analytics user, I want selecting a heatmap peak to move the chart cursor and update the selected moment drawer, so that chart and moment panels stay synchronized.

#### Acceptance Criteria

1. WHEN the user selects a peak in the Heatmap_Lane, THE Analytics_Service frontend SHALL move the chart cursor vertical indicator to the time offset corresponding to that peak's time window.
2. WHEN the user selects a peak in the Heatmap_Lane, THE Analytics_Service frontend SHALL update the selected moment drawer showing the peak's score, reason label, stream offset, up to 4 top emotes with name, image, provider badge, and count, and the conditionally available actions: "Play in Streamclone" when a VOD identifier is resolved, "Jump into VOD" when a Twitch VOD URL is available, and "Clip Live Moment" or "Export Moment" depending on whether the view is live or historical.
3. WHEN heatmap data is available, THE Analytics_Service frontend SHALL display emote images only for the currently selected or hovered peak and SHALL NOT render emote images as idle animation on non-interacted peaks.
4. IF the user selects a peak whose time window has no corresponding rollup data, THEN THE Analytics_Service frontend SHALL display the selected moment drawer in an empty state indicating that detailed metrics are unavailable for this moment.

### Requirement 17: Heatmap Accessibility

**User Story:** As a user with motion sensitivity, I want heatmap animations to respect my system preference, so that the analytics page does not cause discomfort.

#### Acceptance Criteria

1. WHILE the user has `prefers-reduced-motion` enabled, THE Heatmap_Lane SHALL disable all CSS transitions and transform-based animations, applying immediate state changes only.
2. WHILE the user has `prefers-reduced-motion` enabled, THE Channel_Workspace heatmap SHALL render the first frame of animated emotes as a static image instead of playing animation sequences.
3. THE Heatmap_Lane SHALL be keyboard-navigable using Arrow Left/Right to move between peaks and Enter/Space to select, with a visible focus indicator on the active peak.
4. THE Heatmap_Lane SHALL implement a roving-tabindex pattern where each peak is a focusable button element within a `role="toolbar"` container; each peak button SHALL have `aria-label` describing the peak's score, reason label, and offset (e.g. "Peak at 01:23:45, score 82, chat spike"), and screen readers SHALL announce peak context during keyboard navigation.

### Requirement 18: Live Stats Band

**User Story:** As a user viewing a live stream's analytics page, I want a compact stats band showing real-time activity, so that I can see what is happening now without waiting for full historical sync.

#### Acceptance Criteria

1. WHILE viewing a live stream analytics page, THE Analytics_Service frontend SHALL display a Live_Stats_Band that refreshes every 15 seconds and contains: current viewers with 5-minute delta, chat messages per minute (1-minute and 5-minute average with up/down/stable trend arrow based on whether the 1-minute rate is above, below, or within 10% of the 5-minute average), emotes per minute split by provider when at least one provider has data, top emote with image and provider badge (displaying up to 3 emote images per update cycle), and a data confidence indicator showing one of the states: "Collecting", "Waiting for first minute", "Stats only", or "Synced".
2. WHEN a stat value in the Live_Stats_Band updates, THE Analytics_Service frontend SHALL animate the number change using CSS transforms with a duration between 200 and 300 milliseconds without causing layout shifts.
3. THE Live_Stats_Band SHALL include a sparkline visualization rendered on a canvas element displaying up to 60 data points at 1-point-per-minute resolution, representing the most recent 60 minutes of available rollup data or fewer points when less data is available.
4. WHILE the user has `prefers-reduced-motion` enabled, THE Live_Stats_Band SHALL disable number animations and sparkline transitions.
5. IF the live analytics endpoint returns an error or does not respond within 5 seconds, THEN THE Live_Stats_Band SHALL retain the last successfully received values, display a stale-data indicator, and retry on the next 15-second refresh cycle.

### Requirement 19: Live Heat Display

**User Story:** As a user viewing a live stream, I want to see the most reacted moments so far, so that I can jump back to exciting parts of the ongoing stream.

#### Acceptance Criteria

1. WHILE viewing a live stream analytics page where at least 5 completed minute rollups exist for the current stream, THE Analytics_Service frontend SHALL display a **Most Reacted So Far** section (not "Most Replayed") showing up to 10 scored moment points computed from live rollups, refreshing every 30 seconds, with subtitle copy indicating scores are based on chat and emote activity.
2. THE Analytics_Service frontend SHALL visually mute the last incomplete minute bucket and label it "Collecting" to indicate that its score may change once the minute closes.
3. WHEN a live stream ends and the Heatmap_Service resolves the VOD identifier via Helix within 5 minutes of stream close, THE Heatmap_Service SHALL stitch all live heatmap points to the historical stream record using the same minute-bucket offsets as the live session.
4. IF the VOD identifier does not resolve within 5 minutes of stream close, THEN THE Heatmap_Service SHALL retain the live heatmap points under the live stream identifier and mark the record as "unlinked" until a subsequent sync or manual trigger resolves the VOD association.

### Requirement 20: VOD Review Mode Controls

**User Story:** As a user watching a VOD in Streamclone, I want the player controls to reflect VOD context rather than live context, so that the interface is not confusing.

#### Acceptance Criteria

1. WHILE in VOD_Mode, THE Channel_Workspace SHALL hide the "Jump to Live" control from the primary control bar and the diagnostics panel.
2. WHILE in VOD_Mode and total VOD duration is known, THE Channel_Workspace SHALL display current playback time and total VOD duration in HH:MM:SS format in the control bar.
3. IF in VOD_Mode and total VOD duration is not available, THEN THE Channel_Workspace SHALL display current playback time in HH:MM:SS format with a placeholder indicator in place of total duration.
4. WHILE in VOD_Mode, THE Channel_Workspace SHALL display a "Back to live channel" action that navigates to the same channel's live view at `/c/{login}` without VOD query parameters.
5. WHILE in VOD_Mode, THE Channel_Workspace SHALL display a "Back to Analytics" action that navigates to the analytics stream page for the current VOD when analytics context is available from the referring deep link, and hide the action when no analytics context is available.

### Requirement 21: VOD Chat Replay (P2 — Backend Storage Required)

**User Story:** As a user watching a VOD, I want to see the chat messages that occurred at the current playback time, so that I experience the stream context alongside the video.

**Note:** This requirement is deferred to P2. The current MinuteRollup stores aggregate counts and emote frequency maps but does NOT persist individual message text. Implementing chat replay requires a dedicated backend storage layer, retention policy, and replay endpoint. See Requirement 27 (VOD Chat Message Storage) for the prerequisite backend work.

#### Acceptance Criteria

1. WHILE in VOD_Mode and persisted VOD chat messages exist for the current stream, THE Channel_Workspace SHALL display a chat replay panel showing messages from the minute bucket that corresponds to the current video playback offset (aligned to the stream's rollup start time).
2. WHEN the playback position crosses into a different minute bucket (via seek or normal advance), THE Channel_Workspace chat replay panel SHALL request the next page of messages from the VOD chat replay endpoint and update displayed messages within 500 milliseconds of the bucket boundary crossing.
3. WHEN the current minute bucket contains zero chat messages, THE Channel_Workspace chat replay panel SHALL display an empty-minute indicator (distinct from the no-data-available state) and retain the last-shown messages until the next non-empty bucket is reached.
4. WHEN no persisted chat messages exist for the VOD, THE Channel_Workspace SHALL display an empty state indicating chat replay is unavailable and providing a sync action that initiates the analytics VOD chat sync for the associated stream.
5. IF the VOD chat sync is in progress (partial message data available), THEN THE Channel_Workspace chat replay panel SHALL display messages for any minutes already persisted and show a progress indicator for minutes not yet available.

### Requirement 22: Analytics Chart Cursor Sync (Same Page)

**User Story:** As a user viewing the analytics page with an embedded VOD player on the same route, I want the analytics chart cursor to follow the VOD playback position, so that chart context and video are synchronized without cross-tab coordination.

#### Acceptance Criteria

1. WHILE in VOD_Mode with the analytics chart and VOD player rendered on the same page/route for the same stream, THE Analytics_Service frontend chart cursor SHALL update its position to track the current video playback time via shared React state (not BroadcastChannel, localStorage, or backend session state), polling or subscribing to playback position updates at a rate of at least once per second.
2. WHEN the user clicks a point on the analytics chart while the VOD player is active on the same page, THE Channel_Workspace SHALL seek playback to the corresponding time offset within ±1 second of the clicked chart position.
3. IF the VOD player is not rendered on the same page, the stream identifiers do not match, or the VOD player is inactive, THEN THE Analytics_Service frontend chart cursor SHALL not attempt to sync with video playback and SHALL behave as a standard hover/click cursor.

### Requirement 23: Moment-to-Clip Workflow

**User Story:** As a user reviewing stream moments, I want to queue a clip directly from a heatmap peak, so that I can export interesting moments without manual offset entry.

#### Acceptance Criteria

1. WHEN the user activates the clip action on a selected heatmap peak, THE Analytics_Service frontend SHALL queue a clip job via the clipper API with the peak's VOD offset in seconds, stream identifier, VOD identifier, moment score, and top emotes from that minute rollup.
2. IF the clipper service is unreachable or returns an authorization error when the user activates the clip action, THEN THE Analytics_Service frontend SHALL display an inline error message indicating the failure reason and retain the selected peak so the user can retry without re-selecting.
3. WHEN the user activates a "batch queue top moments" action after the stream's minute rollups contain at least 10 minutes of chat data, THE Analytics_Service frontend SHALL queue clip jobs for the top 5 peaks ordered by computed moment score descending, skipping any peak that already has a queued or completed clip job for the same stream and minute timestamp.
4. WHEN a clip job is successfully queued from a heatmap peak, THE Analytics_Service frontend SHALL display a link to the corresponding Clip Studio page at the route `/studio/{jobId}` using the job identifier returned by the clipper API.

### Requirement 24: Heatmap Rendering Performance

**User Story:** As a user on a lower-powered device, I want the heatmap to render efficiently without degrading page performance, so that analytics remains usable on modest hardware.

#### Acceptance Criteria

1. THE Heatmap_Lane SHALL render no more than one visual node per visible pixel column through Decimation, re-decimating on viewport resize to maintain this constraint, with initial render completing within 100 milliseconds and no single main-thread frame exceeding 16 milliseconds.
2. WHILE the browser tab is hidden (document.visibilityState === "hidden"), THE Heatmap_Lane SHALL not perform continuous animation or emote sprite updates.
3. THE Heatmap_Lane SHALL lazy-load emote images only when a peak is hovered or selected, reuse cached local emote URLs from `/emotes/{id}/1x.webp`, and cap concurrent image requests to 3 at a time.
4. THE Heatmap_Lane SHALL produce no long tasks exceeding 50 milliseconds during scroll or resize interactions.

### Requirement 25: VOD Playback Smoke Test

**User Story:** As a developer, I want automated smoke test coverage for the analytics deep link to VOD mode flow, so that regressions in the deep link path are caught before release.

#### Acceptance Criteria

1. THE test suite SHALL include a smoke test that renders an analytics moment with a known `vod_id`, `offset_seconds`, and `channel` login, then simulates activation of the "Play in Streamclone" deep link.
2. THE smoke test SHALL assert that the resulting navigation URL matches `/c/{login}` with query parameter `vod` equal to the source moment's `vod_id` and query parameter `offset` equal to the source moment's `offset_seconds`.
3. THE smoke test SHALL mock `POST /v1/stream/vod/start` to return a response containing `hlsUrl`, `offset_seconds`, and `seek_seconds`, assert that the request body includes non-empty `vod_id` matching the source moment, and SHALL assert that the player loads the returned `hlsUrl` and seeks to `Math.max(0, offset_seconds - seek_seconds)`.
4. IF the mocked `POST /v1/stream/vod/start` returns a non-200 status, THEN THE smoke test SHALL assert that the player displays an error indication and does not attempt HLS playback.

### Requirement 26: VOD Backend Error Coverage

**User Story:** As a developer, I want backend test coverage for VOD HLS readiness errors, so that failure paths are verified and produce correct error types.

#### Acceptance Criteria

1. THE backend test suite SHALL include a test where `waitForHLS` times out after worker spawn (HLS probe endpoint returns 404 for the VOD path) and verify the response is HTTP 504 with error code `hls_not_ready` and `retryable: true`.
2. THE backend test suite SHALL include a test for `invalid_vod_id` (non-numeric or empty VOD identifier) verifying HTTP 400 response with error code `invalid_vod_id`.
3. THE backend test suite SHALL include a test for `vod_unavailable` (usher returns ErrVodUnavailable) verifying HTTP 404 response with error code `vod_unavailable`.
4. THE backend test suite SHALL include a test for `capacity_reached` (max concurrent streams reached) verifying HTTP 503 response with error code `capacity_reached` and `retryable: true`.
5. THE backend test suite SHALL include a test for `upstream_token_failed` (token provider returns ErrPlaybackToken) verifying HTTP 502 response with error code `upstream_token_failed` and `retryable: true`.


### Requirement 27: VOD Chat Message Storage (P2 Prerequisite)

**User Story:** As a backend developer, I want a dedicated storage model for persisted VOD chat messages, so that VOD chat replay has access to actual message text rather than only aggregate rollup counts.

#### Acceptance Criteria

1. THE Analytics_Service SHALL persist individual VOD chat messages during sync in a dedicated storage table (or collection) separate from MinuteRollup aggregates, containing at minimum: stream identifier, minute timestamp (bucket), message identifier, sender display name, message text (sanitized), emote fragments, and offset within the VOD in seconds.
2. THE Analytics_Service SHALL sanitize stored chat messages by stripping control characters, truncating messages to a configurable maximum length (default 500 characters), and removing any embedded URLs or user-identifiable metadata not required for replay display.
3. THE Analytics_Service SHALL enforce a configurable retention policy for stored VOD chat messages (default 90 days from sync date), after which messages SHALL be purged automatically via a scheduled cleanup process.
4. THE Analytics_Service SHALL expose a paginated replay endpoint `GET /v1/analytics/streams/{streamId}/chat-replay?offsetStart={seconds}&offsetEnd={seconds}&limit={n}&cursor={token}` that returns messages within the requested time range ordered by offset ascending.
5. THE replay endpoint SHALL return at most 200 messages per page by default, with a maximum configurable limit of 500, and include a cursor token for fetching subsequent pages.
6. IF the stream has no persisted chat messages, THEN THE replay endpoint SHALL return a 200 response with an empty messages array and a flag indicating chat replay data is unavailable for this stream.

### Requirement 28: ReplayHeatmapPoint Data Contract

**User Story:** As a frontend developer, I want a documented canonical response shape for heatmap points, so that all consumers agree on the data contract and can validate responses.

#### Acceptance Criteria

1. THE Heatmap_Service SHALL return each scored point conforming to the ReplayHeatmapPoint schema containing: `offsetSeconds` (integer, seconds from stream start), `durationSeconds` (integer, window duration), `score` (integer, 0-100), `confidence` (float, 0.0-1.0 overall window confidence), `reason` (string, one of the defined Reason_Label values), `components` (object containing per-signal score breakdown and per-signal confidence), `topEmotes` (array of 0-3 emote objects with id, name, imageUrl, count, and provider), `vodId` (string or null, resolved VOD identifier), `streamId` (string, stream identifier), and `minuteTs` (ISO 8601 timestamp of the minute bucket).
2. THE `components` field SHALL contain entries for each signal: `chatRate`, `emoteRate`, `viewerMomentum`, `providerSpike`, `topEmoteDominance`, and `novelty`, each with a `rawScore` (float, un-clamped signal z-score), `weightedScore` (float, after applying version weight), and `confidence` (float, 0.0-1.0 per-signal confidence).
3. IF a signal has no data for a given window (e.g., emote dictionary not loaded), THEN THE corresponding component entry SHALL have `rawScore: 0`, `weightedScore: 0`, and `confidence: 0.0`.
4. THE Heatmap_Service response envelope SHALL include: `streamId`, `windowSeconds`, `confidence` (stream-level median), `scoringVersion` (string identifier), `updatedAt` (millisecond timestamp), and `points` (array of ReplayHeatmapPoint).

### Requirement 29: Heatmap Cache Invalidation

**User Story:** As a backend developer, I want deterministic cache invalidation for heatmap results, so that stale scores are never served after data changes and cache keys are unambiguous.

#### Acceptance Criteria

1. THE Heatmap_Service SHALL construct cache keys using the composite of: stream identifier, scoring version identifier, rollup `updatedAt` timestamp (milliseconds), and requested window size in seconds.
2. WHEN new rollup rows are written for a stream (rollup `updatedAt` changes), THE Heatmap_Service cache SHALL treat the previous cache entry as invalid and recompute on the next request.
3. WHEN the scoring version changes (weights, smoothing, or threshold parameters are updated), THE Heatmap_Service SHALL invalidate all cached entries for all streams since the version component of the cache key will differ.
4. THE Heatmap_Service SHALL support explicit cache invalidation via `DELETE /v1/analytics/streams/{streamId}/replay-heatmap/cache` for administrative or debugging purposes, returning 204 on success.

### Requirement 30: Privacy and Retention for VOD Chat Messages

**User Story:** As a platform operator, I want stored VOD chat messages to respect user privacy and comply with data retention best practices, so that personal data is not retained indefinitely.

#### Acceptance Criteria

1. THE Analytics_Service SHALL NOT store raw Twitch user IDs, IP addresses, or authentication tokens in the VOD chat message storage; only display names and anonymized sender identifiers (hashed or truncated) SHALL be persisted.
2. THE Analytics_Service SHALL provide a configurable retention duration for VOD chat messages via environment variable `ANALYTICS_VOD_CHAT_RETENTION_DAYS` (default 90); messages older than this threshold SHALL be purged by an automated cleanup process running at least once per 24 hours.
3. WHEN the retention policy purges messages for a stream, THE Analytics_Service SHALL log the stream identifier and count of purged messages at info level without logging message content.
4. THE Analytics_Service SHALL provide an administrative endpoint `DELETE /v1/analytics/streams/{streamId}/chat-messages` that purges all stored chat messages for a specific stream immediately, returning 204 on success.
5. THE VOD chat message storage SHALL NOT store messages containing only URLs, spam patterns matched by a configurable blocklist, or messages from known bot accounts (configurable bot username list).

### Requirement 31: Reduced-Motion for Heatmap Glow and Pulse Effects

**User Story:** As a user with motion sensitivity, I want heatmap glow and pulse effects to respect my system preference, so that animated highlights do not cause discomfort.

#### Acceptance Criteria

1. WHILE the user has `prefers-reduced-motion` enabled, THE Heatmap_Lane SHALL disable glow, pulse, and shimmer CSS animations on peak highlights and score intensity indicators, rendering them as static color fills instead.
2. WHILE the user has `prefers-reduced-motion` enabled, THE Channel_Workspace heatmap progress bar SHALL disable any animated gradient transitions or pulsing peak indicators, applying immediate color state changes only.
3. WHILE the user has `prefers-reduced-motion` disabled (motion allowed), THE Heatmap_Lane MAY apply subtle glow or pulse animations to peaks scoring above 80, with animation duration not exceeding 2 seconds per cycle and no more than 3 simultaneously animated peaks visible at any time.

### Requirement 32: Playwright Visual Acceptance Test

**User Story:** As a developer, I want Playwright visual regression coverage of the analytics heatmap at realistic data density, so that UI regressions in the primary analytics view are caught before release.

#### Acceptance Criteria

1. THE test suite SHALL include a Playwright visual acceptance test that loads the analytics page at `http://localhost:8090/analytics/caedrel/2026-06-11` (the synced Caedrel historical stream) as the primary fixture, representing populated real-data density (120+ minutes of rollup data with varied scores, emote spikes, and multiple peaks).
2. THE visual acceptance test SHALL capture a full-page screenshot of the analytics view including the heatmap lane, right rail moments tab, stat cards, and chart, at a viewport width of 1920px and height of 1080px.
3. THE visual acceptance test SHALL compare the captured screenshot against a baseline image with a configurable pixel-difference threshold (default 0.1% of total pixels) and fail if the difference exceeds the threshold.
4. THE visual acceptance test SHALL be runnable in CI with headed or headless Chromium and produce a diff image artifact on failure showing changed regions.
5. WHEN fixture data includes heatmap peaks with top emotes, THE visual acceptance test SHALL verify that emote images render within peak tooltips on hover interaction.

### Requirement 33: Release Bundle Verification (P0)

**User Story:** As a release operator, I want smoke checks that the served frontend bundle matches the built artifact, so that VOD playback fixes are not silently lost to stale deploys.

#### Acceptance Criteria

1. THE release or local smoke checklist SHALL record the hash or filename of the served frontend entry script (e.g. `index-*.js` from `index.html`) after deploy and SHALL fail the check when it does not match the CI-built artifact for the same commit or tag.
2. WHEN VOD smoke runs against `http://localhost:8090`, THE smoke test SHALL issue `POST /v1/stream/vod/start` with a known valid VOD_Identifier and SHALL fail if the response is HTTP 400 with `invalid_vod_id` while the request body contained a valid normalized id (indicating client bundle regression).
3. IF live playback smoke passes but VOD smoke fails with `invalid_vod_id` for valid ids, THEN THE release gate SHALL treat this as a frontend deploy mismatch rather than a Twitch content issue.

### Requirement 34: Stale Analytics VOD Identifier Handling

**User Story:** As an analytics user, I want invalid or stale `vod_id` values on stream rows handled before playback, so that deep links fail with actionable guidance instead of a misleading Twitch deletion message.

#### Acceptance Criteria

1. WHEN analytics presents a "Play in Streamclone" action, THE Analytics_Service frontend SHALL only enable the action when the stream record includes a non-empty VOD_Identifier resolved from Helix, DB cache, or sync (`vod_source` not unknown).
2. IF the stream row lacks a resolved `vod_id`, THEN THE Analytics_Service frontend SHALL disable or hide "Play in Streamclone" and SHALL show copy indicating VOD id is not yet resolved (sync in progress or stats-only).
3. WHEN `POST /v1/stream/vod/start` returns `vod_unavailable` for a recently synced stream, THE Analytics_Service frontend SHALL offer actions to re-sync stream metadata, open on Twitch, and return to analytics — not only a deletion message.

### Requirement 35: Clips Tab Empty State Honesty

**User Story:** As an analytics user, I want the Clips tab to tell me to sync first when no moment data exists, so that I am not directed to click an empty chart.

#### Acceptance Criteria

1. WHEN the Clips tab is active and the selected stream has zero minute-level rollup rows with chat or emote data, THE Analytics_Service frontend SHALL display an empty state instructing the user to sync chat/emotes first, then clip from ranked moments or heatmap peaks.
2. WHEN rollup data exists but no clip jobs are queued, THE Analytics_Service frontend SHALL display guidance referencing the Moments tab or heatmap peak selection rather than "click the graph" without context.

### Requirement 36: Clipper Auth Failure Surfacing (Moment Actions)

**User Story:** As a user queueing clips from moments, I want OAuth scope failures explained inline, so that I can fix Twitch auth without guessing.

#### Acceptance Criteria

1. WHEN the clipper API returns `missing_scope`, `invalid_token`, `twitch_not_configured`, or `vod_auth_failed`, THE Analytics_Service frontend SHALL display inline auth help (scope list, re-auth path) on the Clips tab and selected-moment clip action, and SHALL retain the selected moment context for retry after auth succeeds.
2. THE clip action SHALL NOT silently fail or show a generic "clip failed" message when the failure code is auth-related.

### Requirement 37: Local Replay Telemetry (Future — Most_Replayed)

**User Story:** As a product owner, I want a path to honest "Most Replayed" scores later, without retroactively mislabeling v1 reaction heatmaps.

#### Acceptance Criteria

1. THE Channel_Workspace SHALL emit local-only playback events (seek, replay segment, dwell > N seconds) to an operator-owned store; events SHALL NOT be uploaded to Twitch or third parties by default.
2. WHEN replay telemetry is enabled and coverage exceeds a configurable threshold (default 5% of stream duration with ≥10 seek events), THE Heatmap_Service MAY expose `label: most_replayed` and blend replay signal into the Score_Model under a new Scoring_Version; until then, responses and UI SHALL remain `most_reacted`.
3. THE UI SHALL disclose the data source in subtitle copy whenever the label changes from Most_Reacted to Most_Replayed.


### Requirement 38: Analytics Tier Indicator

**User Story:** As an analytics user, I want to see whether the analytics tier is Core or Analytics Active when minute charts are empty, so that I understand whether the system cannot produce charts (scraper not running) versus charts being unsynced.

#### Acceptance Criteria

1. WHEN the analytics page loads for any channel, THE Analytics_Service frontend SHALL display a Tier_Indicator badge showing the current analytics profile state.
2. IF the scraper service is not reachable or the compose profile does not include the scraper, THEN THE Tier_Indicator SHALL display "Core" and THE chart empty state SHALL explain that minute-level viewer charts require the Analytics tier (scraper profile) and provide a link or action to start it via setup-control.
3. IF the scraper service is reachable and the analytics profile is active, THEN THE Tier_Indicator SHALL display "Analytics active" and THE chart empty state SHALL follow standard sync or collecting messaging from Requirements 6 and 7.
4. THE Tier_Indicator SHALL be positioned in the analytics header alongside the stream title and source quality badges, not duplicated in stat cards or the chart area.
5. THE Tier_Indicator state SHALL be derived from the existing `useSystemHealth` or `useOptionalServices` hooks rather than a separate health probe, to avoid duplicating compose-level health checks.

### Requirement 39: Cross-Tab Playhead Sync (P3 — Optional Follow-Up)

**User Story:** As a power user with the analytics page and VOD player open on separate browser tabs or routes, I want playback position to optionally sync across tabs, so that I can use a multi-monitor setup without losing chart-to-player coordination.

**Note:** This is an optional P3 follow-up to Requirement 22 (same-page sync). It is NOT required for P1 or P2 milestones.

#### Acceptance Criteria

1. WHEN the user enables cross-tab sync via a settings toggle (default off), THE Channel_Workspace SHALL write current playback position (streamId, offsetSeconds, timestamp) to `sessionStorage` at a rate of once per second while in VOD_Mode.
2. WHEN cross-tab sync is enabled and the analytics page detects a `sessionStorage` change event with a matching stream identifier, THE Analytics_Service frontend chart cursor SHALL update to the stored playback offset.
3. WHEN the user clicks a point on the analytics chart while cross-tab sync is enabled, THE Analytics_Service frontend SHALL write a seek intent (streamId, targetOffsetSeconds) to `sessionStorage`; the Channel_Workspace on the other tab SHALL detect this event and seek playback to the target offset.
4. IF the stream identifiers do not match between the analytics page and the stored playback state, THEN THE Analytics_Service frontend SHALL ignore the cross-tab sync and behave as a standard cursor.
5. THE cross-tab sync feature SHALL use `sessionStorage` events only (not localStorage, BroadcastChannel, or backend state) to limit scope to the same browser session and avoid security or persistence concerns.
6. WHEN cross-tab sync is disabled (default), THE analytics page and VOD player SHALL have no inter-tab coordination and SHALL operate independently.
