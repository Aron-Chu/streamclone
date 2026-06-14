import assert from 'node:assert/strict'
import { it } from 'node:test'

import {
  LIVE_HEAT_MAX_POINTS,
  LIVE_HEAT_MIN_COMPLETED_ROLLUPS,
  LIVE_HEAT_REFRESH_MS,
  LIVE_HEAT_SUBTITLE,
  deriveLiveHeat,
  formatHeatOffset,
  type LiveHeatInput,
  type LiveHeatRollup,
} from '../src/utils/liveHeat.ts'

// Unit coverage for the "Most Reacted So Far" live heat derivation
// (Requirements 19.1, 19.2).

const MINUTE_MS = 60_000

/** Build N data-bearing rollups starting at a fixed base time, plus options. */
function makeRollups(
  count: number,
  shape: (i: number) => Partial<LiveHeatRollup> = () => ({}),
): LiveHeatRollup[] {
  const base = Date.parse('2026-06-11T12:00:00.000Z')
  return Array.from({ length: count }, (_, i) => ({
    minuteTs: new Date(base + i * MINUTE_MS).toISOString(),
    viewerSamples: 1,
    chatCount: 10,
    totalEmoteCount: 4,
    emotes: {},
    ...shape(i),
  }))
}

it('hides the section until 5 completed rollups exist (live trailing minute excluded)', () => {
  // 5 data rollups while live -> trailing minute is "collecting", leaving 4
  // completed, which is below the threshold.
  const input: LiveHeatInput = { state: 'live', rollups: makeRollups(5) }
  const heat = deriveLiveHeat(input)
  assert.equal(heat.completedRollupCount, 4)
  assert.equal(heat.visible, false)
  assert.equal(heat.points.length, 0)
  assert.ok(heat.collectingPoint)
  assert.equal(heat.collectingPoint?.collecting, true)
})

it('shows the section once 5 completed rollups exist', () => {
  // 6 data rollups while live -> 5 completed + 1 collecting.
  const input: LiveHeatInput = { state: 'live', rollups: makeRollups(6) }
  const heat = deriveLiveHeat(input)
  assert.equal(heat.completedRollupCount, LIVE_HEAT_MIN_COMPLETED_ROLLUPS)
  assert.equal(heat.visible, true)
  assert.equal(heat.subtitle, LIVE_HEAT_SUBTITLE)
  assert.ok(heat.collectingPoint?.collecting)
})

it('caps ranked points at 10 and ranks by score descending', () => {
  // 24 completed (+1 collecting). One clearly hottest minute.
  const rollups = makeRollups(25, i => ({
    chatCount: i === 3 ? 500 : 10 + i,
    totalEmoteCount: i === 3 ? 200 : 2,
  }))
  const heat = deriveLiveHeat({ state: 'live', rollups })
  assert.equal(heat.visible, true)
  assert.ok(heat.points.length <= LIVE_HEAT_MAX_POINTS)
  // Sorted by score descending.
  for (let i = 1; i < heat.points.length; i++) {
    assert.ok(heat.points[i - 1].score >= heat.points[i].score)
  }
  // The hottest minute (index 3) should be ranked first.
  assert.equal(heat.points[0].offsetSeconds, 3 * 60)
})

it('omits zero-score points and marks no collecting minute when historical', () => {
  const rollups = makeRollups(8, i => ({
    // Half the minutes have no activity at all.
    chatCount: i % 2 === 0 ? 0 : 40,
    totalEmoteCount: 0,
    viewerSamples: i % 2 === 0 ? 0 : 1,
  }))
  // Historical: every data rollup is "completed"; no trailing collecting minute.
  const heat = deriveLiveHeat({ state: 'historical', rollups })
  assert.equal(heat.collectingPoint, null)
  assert.ok(heat.points.every(p => p.score > 0))
})

it('is deterministic for identical input', () => {
  const rollups = makeRollups(12, i => ({ chatCount: 5 * i, totalEmoteCount: i }))
  const a = deriveLiveHeat({ state: 'live', rollups })
  const b = deriveLiveHeat({ state: 'live', rollups })
  assert.deepEqual(a, b)
})

it('exposes a 30s refresh cadence and HH:MM:SS offset formatter', () => {
  assert.equal(LIVE_HEAT_REFRESH_MS, 30000)
  assert.equal(formatHeatOffset(0), '00:00:00')
  assert.equal(formatHeatOffset(65), '00:01:05')
  assert.equal(formatHeatOffset(3661), '01:01:01')
})
