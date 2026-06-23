import assert from 'node:assert/strict'
import { it } from 'node:test'

import {
  TREND_STABLE_RATIO,
  SYNCED_ROLLUP_THRESHOLD,
  computeTrend,
  liveConfidenceState,
  splitEmoteProviderRates,
  buildSparkline,
  deriveLiveStats,
  trendArrowGlyph,
  type LiveStatsInput,
  type LiveStatsRollup,
} from '@streamclone/pulse-core'

// Unit coverage for the Live_Stats_Band pure helpers (Requirements 18.1, 18.5).
//
// The React component (LiveStatsBand.tsx) cannot be rendered under the Node
// test runner (no jsdom, cannot load .tsx), so these tests exercise the pure
// derivation helpers directly. Render-level stale-indicator behavior is
// verified structurally via `npm run build`.

const MINUTE_MS = 60_000

/** Build N data-bearing rollups from a fixed base time, with per-index overrides. */
function makeRollups(
  count: number,
  shape: (i: number) => Partial<LiveStatsRollup> = () => ({}),
): LiveStatsRollup[] {
  const base = Date.parse('2026-06-11T12:00:00.000Z')
  return Array.from({ length: count }, (_, i) => ({
    minuteTs: new Date(base + i * MINUTE_MS).toISOString(),
    viewerSamples: 1,
    viewerLatest: 100,
    chatCount: 10,
    totalEmoteCount: 4,
    seventvEmoteCount: 2,
    emotes: {},
    ...shape(i),
  }))
}

// ---------------------------------------------------------------------------
// computeTrend — trend arrow thresholds around the 10% tolerance (Req 18.1)
// ---------------------------------------------------------------------------

it('computeTrend reports stable when the 1-minute rate is within 10% of the 5-minute average', () => {
  // 5% above and below the average -> within the default tolerance.
  assert.equal(computeTrend(105, 100), 'stable')
  assert.equal(computeTrend(95, 100), 'stable')
  // Exactly equal is trivially stable.
  assert.equal(computeTrend(100, 100), 'stable')
})

it('computeTrend treats the exact +/-10% boundary as stable (inclusive tolerance)', () => {
  // diff == tolerance (10) -> |diff| <= tolerance -> stable.
  assert.equal(computeTrend(110, 100), 'stable')
  assert.equal(computeTrend(90, 100), 'stable')
})

it('computeTrend reports up/down past the 10% tolerance', () => {
  assert.equal(computeTrend(120, 100), 'up')
  assert.equal(computeTrend(80, 100), 'down')
  // Just past the boundary.
  assert.equal(computeTrend(110.01, 100), 'up')
  assert.equal(computeTrend(89.99, 100), 'down')
})

it('computeTrend handles the five<=0 edge case', () => {
  // No 5-minute baseline: any positive 1-minute rate trends up, otherwise stable.
  assert.equal(computeTrend(5, 0), 'up')
  assert.equal(computeTrend(0, 0), 'stable')
  assert.equal(computeTrend(5, -3), 'up')
  assert.equal(computeTrend(0, -3), 'stable')
})

it('computeTrend honors a custom tolerance ratio', () => {
  // With a 50% tolerance, 140 vs 100 is stable; with default 10% it would be up.
  assert.equal(computeTrend(140, 100, 0.5), 'stable')
  assert.equal(computeTrend(140, 100, TREND_STABLE_RATIO), 'up')
})

it('computeTrend coerces non-finite inputs to zero deterministically', () => {
  assert.equal(computeTrend(Number.NaN, 100), 'down')
  assert.equal(computeTrend(50, Number.NaN), 'up')
  assert.equal(computeTrend(Number.NaN, Number.NaN), 'stable')
})

// ---------------------------------------------------------------------------
// liveConfidenceState — confidence states (Req 18.1)
// ---------------------------------------------------------------------------

it('liveConfidenceState returns "Waiting for first minute" for a live stream with no completed rollups', () => {
  const input: LiveStatsInput = { state: 'live', rollups: [] }
  assert.equal(liveConfidenceState(input), 'Waiting for first minute')
})

it('liveConfidenceState returns "Stats only" for a historical stream with only tracker averages', () => {
  const input: LiveStatsInput = {
    state: 'historical',
    rollups: [],
    avgViewers: 1200,
    peakViewers: 3400,
  }
  assert.equal(liveConfidenceState(input), 'Stats only')
})

it('liveConfidenceState returns "Waiting for first minute" for a non-live stream with no data and no averages', () => {
  const input: LiveStatsInput = { state: 'not_collected', rollups: [] }
  assert.equal(liveConfidenceState(input), 'Waiting for first minute')
})

it('liveConfidenceState returns "Collecting" for a live stream warming up below the synced threshold', () => {
  const input: LiveStatsInput = { state: 'live', rollups: makeRollups(3) }
  assert.equal(liveConfidenceState(input), 'Collecting')
})

it('liveConfidenceState returns "Synced" once enough chat-bearing minutes exist', () => {
  const input: LiveStatsInput = { state: 'live', rollups: makeRollups(SYNCED_ROLLUP_THRESHOLD) }
  assert.equal(liveConfidenceState(input), 'Synced')
})

it('liveConfidenceState returns "Synced" for a historical stream with some chat-bearing minutes', () => {
  // Below the threshold but not live and has chat -> trustworthy synced data.
  const input: LiveStatsInput = { state: 'historical', rollups: makeRollups(2) }
  assert.equal(liveConfidenceState(input), 'Synced')
})

it('liveConfidenceState returns "Stats only" for a historical stream with completed but no chat-bearing minutes', () => {
  // Viewer-only rollups (no chat) plus tracker averages.
  const rollups = makeRollups(3, () => ({ chatCount: 0, totalEmoteCount: 0 }))
  const input: LiveStatsInput = {
    state: 'historical',
    rollups,
    avgViewers: 800,
  }
  assert.equal(liveConfidenceState(input), 'Stats only')
})

it('liveConfidenceState returns "Collecting" for a historical stream with no chat and no averages', () => {
  const rollups = makeRollups(3, () => ({ chatCount: 0, totalEmoteCount: 0 }))
  const input: LiveStatsInput = { state: 'historical', rollups }
  assert.equal(liveConfidenceState(input), 'Collecting')
})

// ---------------------------------------------------------------------------
// trendArrowGlyph — glyph mapping (Req 18.1)
// ---------------------------------------------------------------------------

it('trendArrowGlyph maps each direction to its compact glyph', () => {
  assert.equal(trendArrowGlyph('up'), '▲')
  assert.equal(trendArrowGlyph('down'), '▼')
  assert.equal(trendArrowGlyph('stable'), '▬')
})

// ---------------------------------------------------------------------------
// splitEmoteProviderRates — provider lanes (Req 18.1)
// ---------------------------------------------------------------------------

it('splitEmoteProviderRates surfaces 7TV and Other lanes only when each has activity', () => {
  assert.deepEqual(splitEmoteProviderRates({ totalEmoteCount: 10, seventvEmoteCount: 4 }), [
    { provider: '7TV', perMinute: 4 },
    { provider: 'Other', perMinute: 6 },
  ])
  // All 7TV -> no Other lane.
  assert.deepEqual(splitEmoteProviderRates({ totalEmoteCount: 5, seventvEmoteCount: 5 }), [
    { provider: '7TV', perMinute: 5 },
  ])
  // No emotes at all -> no lanes.
  assert.deepEqual(splitEmoteProviderRates({ totalEmoteCount: 0, seventvEmoteCount: 0 }), [])
  // Undefined rollup -> no lanes.
  assert.deepEqual(splitEmoteProviderRates(undefined), [])
})

// ---------------------------------------------------------------------------
// buildSparkline — bounded chat-per-minute series (Req 18.3 / 18.1)
// ---------------------------------------------------------------------------

it('buildSparkline returns oldest-first chat counts capped at the requested point count', () => {
  const rollups = makeRollups(8, i => ({ chatCount: i }))
  // i === 0 is not a completed rollup (chat 0, but viewerSamples 1 keeps it completed).
  const series = buildSparkline(rollups, 3)
  assert.deepEqual(series, [5, 6, 7])
})

it('buildSparkline returns an empty series when the cap is zero', () => {
  assert.deepEqual(buildSparkline(makeRollups(4), 0), [])
})

// ---------------------------------------------------------------------------
// deriveLiveStats — deterministic stale-data fallback semantics (Req 18.5)
// ---------------------------------------------------------------------------

it('deriveLiveStats is pure and deterministic for identical input', () => {
  const rollups = makeRollups(12, i => ({ chatCount: 5 * i, viewerLatest: 100 + i }))
  const input: LiveStatsInput = { state: 'live', rollups, avgViewers: 500 }
  const a = deriveLiveStats(input)
  const b = deriveLiveStats(input)
  assert.deepEqual(a, b)
})

it('deriveLiveStats stays correct with partial rollup data (missing viewer/emote fields)', () => {
  // Chat-only rollups: completed because chatCount > 0, but no viewer data.
  const rollups: LiveStatsRollup[] = [
    { chatCount: 20 },
    { chatCount: 40 },
  ]
  const stats = deriveLiveStats({ state: 'live', rollups })
  assert.equal(stats.currentViewers, 0)
  assert.equal(stats.viewerDelta5m, null)
  assert.equal(stats.chatPerMin1m, 40)
  assert.equal(stats.chatPerMin5m, 30)
  assert.equal(stats.totalEmotePerMin, 0)
  assert.deepEqual(stats.emoteProviderRates, [])
  assert.equal(stats.hasProviderSplit, false)
  // Two chat-bearing minutes while live is still below the synced threshold.
  assert.equal(stats.confidence, 'Collecting')
  assert.equal(stats.completedRollupCount, 2)
})

it('deriveLiveStats produces a 5-minute viewer delta only after enough completed minutes', () => {
  const rollups = makeRollups(7, i => ({ viewerLatest: 100 + i * 10 }))
  const stats = deriveLiveStats({ state: 'live', rollups })
  // 7 completed minutes: current is index 6 (160), prior is index 1 (110).
  assert.equal(stats.currentViewers, 160)
  assert.equal(stats.viewerDelta5m, 50)
})

it('deriveLiveStats falls back to safe zeros when there is no data (stale-first-load semantics)', () => {
  const stats = deriveLiveStats({ state: 'live', rollups: [] })
  assert.equal(stats.currentViewers, 0)
  assert.equal(stats.viewerDelta5m, null)
  assert.equal(stats.chatPerMin1m, 0)
  assert.equal(stats.chatPerMin5m, 0)
  assert.equal(stats.chatTrend, 'stable')
  assert.deepEqual(stats.sparkline, [])
  assert.equal(stats.confidence, 'Waiting for first minute')
})
