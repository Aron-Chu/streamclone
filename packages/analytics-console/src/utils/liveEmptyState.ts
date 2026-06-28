// Live empty-state consistency logic for the Analytics chart.
//
// Pure, dependency-free classification of what the chart area should render
// while a live stream is still warming up, per Requirement 7 of the
// moment-timeline spec. Kept pure so it can be property-tested (task 5.6) and
// wired into Analytics.tsx (task 5.11) without duplicating the decision logic.
//
// The goal is coherence between the stream rail and the chart empty state:
// when the rail shows a stream with a "Collecting now" badge, the chart must
// not contradict it with a "No recent data" message (Req 7.1). Instead it
// shows a "Collecting first minutes" message with an activity indicator until
// at least two rollup minutes exist, then swaps to the chart (Req 7.2, 7.3).

/** Minimum non-missing rollup minutes required before the chart renders (Req 7.2, 7.3). */
export const MIN_LIVE_ROLLUPS_FOR_CHART = 2

/** Message shown while a live stream has fewer than two rollup minutes (Req 7.2). */
export const COLLECTING_FIRST_MINUTES_MESSAGE = 'Collecting first minutes'

/** The empty message that must never appear during active collection (Req 7.1). */
export const NO_RECENT_DATA_MESSAGE = 'No recent data'

export type LiveEmptyStateKind =
  /** Two or more rollup minutes exist: render the chart visualization (Req 7.3). */
  | 'chart'
  /** Live collection active with fewer than two rollups: warming-up message (Req 7.2). */
  | 'collecting-first-minutes'
  /** Not collecting and no usable data: standard empty message. */
  | 'no-recent-data'

export interface LiveEmptyStateInput {
  /**
   * True when the selected stream shows the "Collecting now" badge, i.e. live
   * collection is active for the selected channel/stream (Req 7.1, 7.2).
   */
  collectingNow: boolean
  /** Number of non-missing rollup minutes available for the selected stream. */
  rollupCount: number
}

export interface LiveEmptyStateDecision {
  kind: LiveEmptyStateKind
  /** True when the chart visualization should render instead of an empty message. */
  showChart: boolean
  /** True when an animated activity indicator should accompany the message (Req 7.2). */
  showActivityIndicator: boolean
  /** True when the "No recent data" empty message must be suppressed (Req 7.1). */
  suppressNoRecentData: boolean
  /** Empty-state message copy, or null when the chart should render. */
  message: string | null
}

/**
 * Decide what the analytics chart area should render for the selected stream.
 *
 * Precedence (only one rule applies per call):
 *  - 7.3 chart: two or more rollup minutes exist → render the chart. Recomputing
 *        once the count crosses the threshold naturally replaces a
 *        "Collecting first minutes" message with the chart on the next cycle.
 *  - 7.2 collecting-first-minutes: live collection is active ("Collecting now")
 *        but fewer than two rollup minutes exist → show the warming-up message
 *        with an activity indicator. This also satisfies 7.1 because the kind is
 *        never "no-recent-data" while collection is active.
 *  - no-recent-data: not collecting and not enough data → standard empty message.
 *
 * Requirement 7.1 invariant: whenever `collectingNow` is true, the returned
 * `kind` is never "no-recent-data" and `suppressNoRecentData` is true.
 */
export function classifyLiveEmptyState(input: LiveEmptyStateInput): LiveEmptyStateDecision {
  const rollupCount = Math.max(0, Math.floor(input.rollupCount ?? 0))
  const collectingNow = input.collectingNow === true

  if (rollupCount >= MIN_LIVE_ROLLUPS_FOR_CHART) {
    return {
      kind: 'chart',
      showChart: true,
      showActivityIndicator: false,
      suppressNoRecentData: collectingNow,
      message: null,
    }
  }

  if (collectingNow) {
    return {
      kind: 'collecting-first-minutes',
      showChart: false,
      showActivityIndicator: true,
      suppressNoRecentData: true,
      message: COLLECTING_FIRST_MINUTES_MESSAGE,
    }
  }

  return {
    kind: 'no-recent-data',
    showChart: false,
    showActivityIndicator: false,
    suppressNoRecentData: false,
    message: NO_RECENT_DATA_MESSAGE,
  }
}

/** Convenience guard: should the "Collecting first minutes" message render? (Req 7.2) */
export function isCollectingFirstMinutes(decision: LiveEmptyStateDecision): boolean {
  return decision.kind === 'collecting-first-minutes'
}
