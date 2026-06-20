import assert from 'node:assert/strict'
import { it } from 'node:test'

import {
  ACTIVITY_WAVEFORM_LAYER_ORDER,
  ACTIVITY_WAVEFORM_MAX_PEAKS,
  ACTIVITY_WAVEFORM_PEAK_MIN_SCORE,
  activityWaveformOffsetToX,
  defaultLayerVisibility,
  deriveActivityWaveform,
  detectActivityPeaks,
  layerValueForRollup,
  normalizeLayerValue,
  peakLayerToReason,
  type ActivityWaveformRollup,
} from '../src/utils/activityWaveform.ts'

const MINUTE_MS = 60_000

function makeRollups(
  count: number,
  shape: (i: number) => Partial<ActivityWaveformRollup> = () => ({}),
): ActivityWaveformRollup[] {
  const base = Date.parse('2026-06-11T12:00:00.000Z')
  return Array.from({ length: count }, (_, i) => ({
    minuteTs: new Date(base + i * MINUTE_MS).toISOString(),
    chatCount: 10,
    totalEmoteCount: 4,
    seventvEmoteCount: 2,
    emotes: {
      'seventv:abc:KEKW': 2,
      'twitch:123:PogChamp': 1,
      'ffz:456:pepeLaugh': 1,
    },
    ...shape(i),
  }))
}

it('returns empty state when rollups have no activity-bearing minutes', () => {
  const rollups = makeRollups(5, () => ({ chatCount: 0, totalEmoteCount: 0, seventvEmoteCount: 0, emotes: {}, missing: true }))
  const result = deriveActivityWaveform(rollups)
  assert.equal(result.hasData, false)
  assert.equal(result.points.length, 0)
  assert.ok(result.emptyReason)
})

it('extracts per-layer raw values from rollup shape', () => {
  const rollup = makeRollups(1)[0]
  assert.equal(layerValueForRollup(rollup, 'chat'), 10)
  assert.equal(layerValueForRollup(rollup, 'seventv'), 2)
  assert.equal(layerValueForRollup(rollup, 'twitch'), 1)
  assert.equal(layerValueForRollup(rollup, 'ffz'), 1)
  assert.equal(layerValueForRollup(rollup, 'total_emotes'), 4)
})

it('normalizes each layer against the stream max independently', () => {
  const rollups = makeRollups(4, i => ({
    chatCount: i === 2 ? 100 : 10,
    totalEmoteCount: i === 1 ? 50 : 5,
  }))
  const result = deriveActivityWaveform(rollups)
  assert.equal(result.hasData, true)
  assert.equal(result.layerMax.chat, 100)
  assert.equal(result.layerMax.total_emotes, 50)
  assert.equal(result.points[2].normalized.chat, 1)
  assert.equal(result.points[1].normalized.total_emotes, 1)
  assert.equal(normalizeLayerValue(50, 100), 0.5)
})

it('detects local peaks above the minimum combined score', () => {
  const rollups = makeRollups(9, i => ({
    chatCount: i === 4 ? 200 : 5,
    totalEmoteCount: i === 4 ? 80 : 2,
  }))
  const result = deriveActivityWaveform(rollups)
  assert.ok(result.peaks.length >= 1)
  const centerPeak = result.peaks.find(peak => peak.index === 4)
  assert.ok(centerPeak)
  assert.ok(centerPeak.score >= ACTIVITY_WAVEFORM_PEAK_MIN_SCORE * 100)
  assert.ok(result.peaks.length <= ACTIVITY_WAVEFORM_MAX_PEAKS)
})

it('ranks peaks by score and preserves chronological order in output', () => {
  const rollups = makeRollups(11, i => ({
    chatCount: i === 3 ? 120 : i === 7 ? 140 : 8,
    totalEmoteCount: i === 3 ? 40 : i === 7 ? 60 : 2,
  }))
  const result = deriveActivityWaveform(rollups)
  for (let i = 1; i < result.peaks.length; i++) {
    assert.ok(result.peaks[i].offsetSeconds >= result.peaks[i - 1].offsetSeconds)
  }
})

it('respects layer visibility when scoring peaks', () => {
  const rollups = makeRollups(7, i => ({
    chatCount: i === 3 ? 0 : 5,
    totalEmoteCount: 0,
    seventvEmoteCount: i === 3 ? 90 : 1,
    emotes: i === 3 ? { 'seventv:abc:KEKW': 90 } : { 'seventv:abc:KEKW': 1 },
  }))
  const chatOnly = deriveActivityWaveform(rollups, { chat: true, seventv: false, twitch: false, ffz: false, total_emotes: false })
  const seventvOnly = deriveActivityWaveform(rollups, { chat: false, seventv: true, twitch: false, ffz: false, total_emotes: false })
  assert.ok(chatOnly.peaks.length === 0 || chatOnly.peaks.every(p => p.dominantLayer === 'chat'))
  assert.ok(seventvOnly.peaks.some(p => p.dominantLayer === 'seventv'))
})

it('maps dominant layers to heatmap-style reason strings', () => {
  assert.equal(peakLayerToReason('chat'), 'chat_spike')
  assert.equal(peakLayerToReason('seventv'), 'seventv_spike')
  assert.equal(peakLayerToReason('twitch'), 'twitch_emote_spike')
  assert.equal(peakLayerToReason('ffz'), 'ffz_spike')
})

it('builds one point per rollup with minute offsets', () => {
  const rollups = makeRollups(6)
  const result = deriveActivityWaveform(rollups)
  assert.equal(result.points.length, 6)
  assert.equal(result.points[0].offsetSeconds, 0)
  assert.equal(result.points[5].offsetSeconds, 300)
  assert.equal(result.totalDurationSec, 360)
  for (const layer of ACTIVITY_WAVEFORM_LAYER_ORDER) {
    assert.ok(Number.isFinite(result.points[0].normalized[layer]))
  }
})

it('detectActivityPeaks returns empty for very short series', () => {
  const rollups = makeRollups(2)
  const result = deriveActivityWaveform(rollups)
  assert.deepEqual(detectActivityPeaks(result.points), [])
})

it('maps VOD offsets to waveform x coordinates for playhead markers', () => {
  assert.equal(activityWaveformOffsetToX(0, 3600, 400), 0)
  assert.equal(activityWaveformOffsetToX(1800, 3600, 400), 200)
  assert.equal(activityWaveformOffsetToX(3600, 3600, 400), 400)
  assert.equal(activityWaveformOffsetToX(5000, 3600, 400), 400)
  assert.equal(activityWaveformOffsetToX(-10, 3600, 400), 0)
})
