import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

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
} from '../src/liveStats.ts'

const MINUTE_MS = 60_000

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

describe('computeTrend', () => {
  it('reports stable within 10% tolerance', () => {
    assert.equal(computeTrend(105, 100), 'stable')
    assert.equal(computeTrend(110, 100), 'stable')
    assert.equal(computeTrend(120, 100), 'up')
    assert.equal(computeTrend(80, 100), 'down')
  })

  it('honors custom tolerance ratio', () => {
    assert.equal(computeTrend(140, 100, 0.5), 'stable')
    assert.equal(computeTrend(140, 100, TREND_STABLE_RATIO), 'up')
  })
})

describe('liveConfidenceState', () => {
  it('returns Collecting for live stream below synced threshold', () => {
    assert.equal(liveConfidenceState({ state: 'live', rollups: makeRollups(3) }), 'Collecting')
  })

  it('returns Synced once enough chat-bearing minutes exist', () => {
    assert.equal(
      liveConfidenceState({ state: 'live', rollups: makeRollups(SYNCED_ROLLUP_THRESHOLD) }),
      'Synced',
    )
  })
})

describe('deriveLiveStats', () => {
  it('is pure and deterministic', () => {
    const input: LiveStatsInput = { state: 'live', rollups: makeRollups(12) }
    assert.deepEqual(deriveLiveStats(input), deriveLiveStats(input))
  })

  it('builds sparkline from completed minutes only', () => {
    const rollups = makeRollups(8, i => ({ chatCount: i }))
    assert.deepEqual(buildSparkline(rollups, 3), [5, 6, 7])
  })

  it('falls back to safe zeros when empty', () => {
    const stats = deriveLiveStats({ state: 'live', rollups: [] })
    assert.equal(stats.confidence, 'Waiting for first minute')
    assert.deepEqual(stats.sparkline, [])
  })
})

describe('trendArrowGlyph', () => {
  it('maps directions', () => {
    assert.equal(trendArrowGlyph('up'), '▲')
    assert.equal(trendArrowGlyph('down'), '▼')
    assert.equal(trendArrowGlyph('stable'), '▬')
  })
})

describe('splitEmoteProviderRates', () => {
  it('splits 7TV and Other', () => {
    assert.deepEqual(splitEmoteProviderRates({ totalEmoteCount: 10, seventvEmoteCount: 4 }), [
      { provider: '7TV', perMinute: 4 },
      { provider: 'Other', perMinute: 6 },
    ])
  })
})
