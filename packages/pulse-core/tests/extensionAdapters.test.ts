import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  aggregateTopEmotesFromExtensionRollups,
  extensionSupportsPeaks,
  peaksToLiveHeatPoints,
  toLiveHeatInputFromExtension,
  toLiveStatsInputFromExtension,
  type ExtensionPeakLike,
  type ExtensionPulseLike,
} from '../src/extensionAdapters.ts'
import { deriveLiveHeat, deriveLiveStats, splitEmoteProviderRates } from '../src/index.ts'

const STARTED_AT = '2026-06-11T12:00:00.000Z'

function makeExtensionPayload(rollupCount: number, overrides: Partial<ExtensionPulseLike> = {}): ExtensionPulseLike {
  return {
    isLive: true,
    startedAt: STARTED_AT,
    rollups: Array.from({ length: rollupCount }, (_, i) => ({
      offsetSeconds: i * 60,
      chatCount: i + 1,
      sevenTvEmoteCount: i,
      totalEmoteCount: i + 1,
      viewerCount: i % 3 === 0 ? 300 + i : 0,
    })),
    ...overrides,
  }
}

describe('toLiveStatsInputFromExtension', () => {
  it('returns empty input for null payload', () => {
    const input = toLiveStatsInputFromExtension(null)
    assert.equal(input.state, 'historical')
    assert.deepEqual(input.rollups, [])
  })

  it('maps extension rollups to live stats derivation', () => {
    const stats = deriveLiveStats(toLiveStatsInputFromExtension(makeExtensionPayload(10)))
    assert.equal(stats.chatPerMin1m, 10)
    assert.equal(stats.confidence, 'Synced')
    assert.equal(stats.sparkline.length, 10)
  })

  it('uses stream topEmotes when present', () => {
    const payload = makeExtensionPayload(2, {
      topEmotes: [{ name: 'KEKW', count: 50, provider: 'seventv', id: 'e1' }],
    })
    const stats = deriveLiveStats(toLiveStatsInputFromExtension(payload))
    assert.equal(stats.topEmotes[0]?.name, 'KEKW')
    assert.equal(stats.topEmotes[0]?.provider, '7TV')
  })

  it('aggregates rollup top emotes when stream list is empty', () => {
    const payload = makeExtensionPayload(2, {
      rollups: [
        {
          offsetSeconds: 0,
          chatCount: 5,
          sevenTvEmoteCount: 2,
          totalEmoteCount: 4,
          topEmotes: [{ name: 'KEKW', count: 3, provider: 'seventv', id: 'abc123' }],
        },
        {
          offsetSeconds: 60,
          chatCount: 8,
          sevenTvEmoteCount: 4,
          totalEmoteCount: 6,
          topEmotes: [{ name: 'KEKW', count: 2, provider: 'seventv', id: 'abc123' }],
        },
      ],
    })
    const merged = aggregateTopEmotesFromExtensionRollups(payload.rollups)
    assert.equal(merged[0]?.count, 5)
  })

  it('ignores trailing empty live minute', () => {
    const payload = makeExtensionPayload(3, {
      rollups: [
        { offsetSeconds: 0, chatCount: 5, sevenTvEmoteCount: 1, totalEmoteCount: 2 },
        { offsetSeconds: 60, chatCount: 10, sevenTvEmoteCount: 2, totalEmoteCount: 4 },
        { offsetSeconds: 120, chatCount: 0, sevenTvEmoteCount: 0, totalEmoteCount: 0 },
      ],
    })
    const stats = deriveLiveStats(toLiveStatsInputFromExtension(payload))
    assert.equal(stats.chatPerMin1m, 10)
  })
})

describe('splitEmoteProviderRates via extension mapping', () => {
  it('maps sevenTvEmoteCount field name', () => {
    const input = toLiveStatsInputFromExtension({
      isLive: true,
      rollups: [{ offsetSeconds: 0, chatCount: 3, sevenTvEmoteCount: 2, totalEmoteCount: 4 }],
    })
    assert.deepEqual(splitEmoteProviderRates(input.rollups[0]), [
      { provider: '7TV', perMinute: 2 },
      { provider: 'Other', perMinute: 2 },
    ])
  })

  it('infers emote counts from rollup topEmotes when totals are zero', () => {
    const input = toLiveStatsInputFromExtension({
      isLive: true,
      rollups: [
        {
          offsetSeconds: 60,
          chatCount: 12,
          sevenTvEmoteCount: 0,
          totalEmoteCount: 0,
          topEmotes: [{ name: 'KEKW', count: 9, provider: '7TV' }],
        },
      ],
    })
    assert.equal(input.rollups[0]?.totalEmoteCount, 9)
    assert.equal(input.rollups[0]?.seventvEmoteCount, 9)
    assert.deepEqual(splitEmoteProviderRates(input.rollups[0]), [{ provider: '7TV', perMinute: 9 }])
  })

  it('falls back to fullRollups when recent rollups are empty', () => {
    const payload = makeExtensionPayload(0, {
      fullRollups: Array.from({ length: 7 }, (_, i) => ({
        offsetSeconds: i * 60,
        chatCount: i + 2,
        totalEmoteCount: i + 1,
      })),
    })
    const stats = deriveLiveStats(toLiveStatsInputFromExtension(payload))
    assert.equal(stats.completedRollupCount, 7)
    assert.notEqual(stats.confidence, 'Waiting for first minute')
  })
})

describe('toLiveHeatInputFromExtension', () => {
  it('produces visible heat once enough completed minutes exist', () => {
    const heat = deriveLiveHeat(toLiveHeatInputFromExtension(makeExtensionPayload(7)))
    assert.equal(heat.visible, true)
    assert.ok(heat.collectingPoint?.collecting)
  })
})

describe('peaksToLiveHeatPoints', () => {
  const peaks: ExtensionPeakLike[] = [
    {
      offsetSeconds: 3120,
      score: 88,
      reasons: ['twitch_emote_spike'],
      reasonLabel: 'Twitch emote spike',
      chatCount: 214,
      emoteCount: 5,
      topEmotes: [
        { name: 'LUL', count: 2, provider: 'twitch', id: 'lul' },
        { name: 'TriHard', count: 1, provider: 'twitch', id: 'tri' },
      ],
    },
  ]

  it('maps backend peaks to non-estimated LiveHeatPoint rows', () => {
    const points = peaksToLiveHeatPoints(peaks, STARTED_AT)
    assert.equal(points.length, 1)
    assert.equal(points[0]?.score, 88)
    assert.equal(points[0]?.estimated, false)
    assert.equal(points[0]?.reasonLabel, 'Emote spike')
    assert.equal(points[0]?.chatCount, 214)
    assert.equal(points[0]?.topEmotes[0]?.name, 'LUL')
    assert.equal(points[0]?.topEmotes[0]?.count, 2)
  })

  it('extensionSupportsPeaks is true only when peaks field is present', () => {
    assert.equal(extensionSupportsPeaks(makeExtensionPayload(1, { peaks: [] })), true)
    assert.equal(extensionSupportsPeaks(makeExtensionPayload(1)), false)
    assert.equal(extensionSupportsPeaks(null), false)
  })
})
