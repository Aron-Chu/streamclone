// Same-page analytics chart cursor <-> VOD player sync (Requirement 22).
//
// Pure, dependency-free helpers that decide whether the analytics chart cursor
// should follow VOD playback, and where a chart click should seek the player.
// The actual playback position is carried through shared React state (the
// Zustand `usePlayheadStore`), NOT BroadcastChannel, localStorage, or backend
// session state (Req 22.1). These helpers only operate on plain data so they
// can be unit-tested without rendering (task 17.4) and reused by both the
// analytics chart and the channel workspace.
//
// Guard behaviour (Req 22.3): the cursor only tracks playback when the chart
// and the VOD player are on the same page for the SAME stream AND the player
// is active. When the player is absent, inactive, or the stream identifiers do
// not match, sync is disabled and the caller falls back to the standard
// hover/click cursor.

/**
 * Minimum playback-position update rate the player must publish to the shared
 * store so the chart cursor tracks at >= 1 Hz (Req 22.1). The player writes the
 * store at least once per `PLAYHEAD_SYNC_INTERVAL_MS`.
 */
export const PLAYHEAD_SYNC_MIN_HZ = 1
export const PLAYHEAD_SYNC_INTERVAL_MS = 1000

/**
 * Tolerance, in seconds, within which a chart click must seek the VOD player
 * (Req 22.2). Returning the exact clicked offset lands well inside this bound.
 */
export const CHART_SEEK_TOLERANCE_SECONDS = 1

/** Minimal playhead snapshot consumed by the sync helpers (subset of PlayheadState). */
export interface PlayheadSnapshot {
  streamId: string | null
  isPlaying: boolean
  offsetSeconds: number
}

export interface ChartCursorSyncInput {
  /** Analytics stream id of the chart currently rendered, or null when unknown. */
  chartStreamId: string | null
  /**
   * Current shared playhead snapshot published by a same-page VOD player, or
   * null when no player is mounted on the page.
   */
  playhead: PlayheadSnapshot | null
}

export interface ChartCursorSyncResult {
  /**
   * True when the chart cursor should track playback (same page, same stream,
   * active player). False means the caller uses the standard cursor (Req 22.3).
   */
  synced: boolean
  /**
   * Offset in seconds the synced cursor should display, or null when not synced.
   */
  cursorOffsetSeconds: number | null
}

/** Normalize a raw offset to a finite, non-negative whole-or-fractional second. */
function normalizeOffset(offset: number): number {
  if (!Number.isFinite(offset) || offset <= 0) return 0
  return offset
}

/**
 * True when a same-page VOD player is active for the same stream as the chart.
 * This is the shared guard for both cursor tracking (Req 22.1) and chart-click
 * seeking (Req 22.2); when false the chart behaves as a standard cursor
 * (Req 22.3).
 */
export function isPlayerLinkedToChart(input: ChartCursorSyncInput): boolean {
  const { chartStreamId, playhead } = input
  // No player on the page → not linked (Req 22.3).
  if (!playhead) return false
  // Stream identifiers must both be present and match exactly (Req 22.3).
  if (!chartStreamId || !playhead.streamId) return false
  if (chartStreamId !== playhead.streamId) return false
  return true
}

/**
 * Decide whether the analytics chart cursor should sync to VOD playback, and
 * what offset it should show (Req 22.1, 22.3).
 *
 * Sync is active only when a same-page player is linked to the chart's stream
 * AND the player is currently active (playing). An inactive (paused/idle)
 * player counts as inactive per Req 22.3, so the cursor falls back to the
 * standard hover/click behaviour.
 */
export function computeChartCursorSync(input: ChartCursorSyncInput): ChartCursorSyncResult {
  const linked = isPlayerLinkedToChart(input)
  if (!linked || !input.playhead || !input.playhead.isPlaying) {
    return { synced: false, cursorOffsetSeconds: null }
  }
  return { synced: true, cursorOffsetSeconds: normalizeOffset(input.playhead.offsetSeconds) }
}

export interface ChartClickSeekResult {
  /** Whether the click should drive a player seek (Req 22.2) vs standard click. */
  shouldSeek: boolean
  /** Whole-second-tolerant seek target, or null when seeking is not applicable. */
  seekOffsetSeconds: number | null
}

/**
 * Resolve a chart click into a VOD player seek target (Req 22.2). The player is
 * only seeked when it is linked to the chart's stream on the same page;
 * otherwise the click is a standard chart interaction (Req 22.3). The returned
 * offset equals the clicked offset (clamped to >= 0), which lands within the
 * ±1s tolerance the player must honour.
 */
export function resolveChartClickSeek(
  input: ChartCursorSyncInput,
  clickedOffsetSeconds: number,
): ChartClickSeekResult {
  if (!isPlayerLinkedToChart(input)) {
    return { shouldSeek: false, seekOffsetSeconds: null }
  }
  return { shouldSeek: true, seekOffsetSeconds: normalizeOffset(clickedOffsetSeconds) }
}
