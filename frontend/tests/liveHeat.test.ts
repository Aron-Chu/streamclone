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

it('spreads fallback scores instead of capping many peaks at 100', () => {
  const rollups = makeRollups(25, i => ({
    chatCount: 180 + i * 45,
    totalEmoteCount: 80 + i * 18,
  }))
  const heat = deriveLiveHeat({ state: 'live', rollups })
  const scores = heat.points.map(point => point.score)
  assert.ok(scores.length > 1)
  assert.ok(new Set(scores).size > 1, 'scores should differentiate across minutes')
  assert.ok(scores.some(score => score < 100), 'not every peak should hit the ceiling')
})

it('uses backend heatmap score when a point is available', () => {
  const rollups = makeRollups(10)
  const targetMinute = rollups[3].minuteTs as string
  const heatmapPoints = [{
    minuteTs: targetMinute,
    offsetSeconds: 180,
    durationSeconds: 60,
    score: 87,
    confidence: 0.91,
    reason: 'chat_spike',
    topEmotes: [],
    vodId: '123',
    streamId: 'stream-1',
  }]
  const heat = deriveLiveHeat({ state: 'historical', rollups, heatmapPoints })
  const match = heat.points.find(point => point.minuteTs === targetMinute)
  assert.ok(match)
  assert.equal(match?.score, 87)
  assert.equal(match?.estimated, false)
})

it('keeps the collecting minute out of ranked points while live', () => {
  const rollups = makeRollups(12, i => ({
    chatCount: 20 + i * 30,
    totalEmoteCount: 5 + i,
  }))
  const heat = deriveLiveHeat({ state: 'live', rollups })
  assert.equal(heat.completedRollupCount, 11)
  assert.ok(heat.collectingPoint)
  assert.equal(heat.collectingPoint?.collecting, true)
  assert.ok(
    heat.points.every(point => point.minuteTs !== heat.collectingPoint?.minuteTs),
    'collecting minute must not appear in ranked list',
  )
})

it('spreads scores instead of saturating every minute at 100', () => {
  const rollups = makeRollups(12, i => ({
    chatCount: 10 + i * 3,
    totalEmoteCount: 2 + i,
  }))
  const heat = deriveLiveHeat({ state: 'historical', rollups })
  const scores = heat.points.map(point => point.score)
  assert.ok(scores.length >= 2)
  assert.ok(new Set(scores).size > 1, 'scores should not all be identical')
  assert.ok(scores.every(score => score <= 100))
  assert.ok(scores.some(score => score < 100), 'not every minute should hit 100')
})

it('uses backend heatmap scores when heatmap points are provided', () => {
  const rollups = makeRollups(8)
  const minuteTs = rollups[3].minuteTs as string
  const heat = deriveLiveHeat({
    state: 'historical',
    rollups,
    heatmapPoints: [{
      offsetSeconds: 180,
      durationSeconds: 60,
      score: 42,
      confidence: 0.9,
      reason: 'chat_spike',
      topEmotes: [],
      vodId: null,
      streamId: 'stream-1',
      minuteTs,
    }],
  })
  const matched = heat.points.find(point => point.minuteTs === minuteTs)
  assert.ok(matched)
  assert.equal(matched?.score, 42)
  assert.equal(matched?.estimated, false)
})

it('still excludes the collecting minute from ranked points while live', () => {
  const rollups = makeRollups(8, i => ({
    chatCount: 20 + i * 5,
    totalEmoteCount: 4 + i,
  }))
  const heat = deriveLiveHeat({ state: 'live', rollups })
  assert.equal(heat.completedRollupCount, 7)
  assert.ok(heat.collectingPoint)
  assert.equal(heat.collectingPoint?.collecting, true)
  assert.ok(!heat.points.some(point => point.minuteTs === heat.collectingPoint?.minuteTs))
})
