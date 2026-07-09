import { describe, expect, it } from 'vitest'
import type { ChartMinuteRollup } from '../src/types.ts'
import {
  findRollupIndexInSeries,
  holdForwardViewerGaps,
  rollupMinuteBucketsEqual,
  smoothDisplayValues,
  viewerChartSmoothWindow,
} from '../src/chartRollupUtils.ts'

function minute(partial: Partial<ChartMinuteRollup> & Pick<ChartMinuteRollup, 'minuteTs'>): ChartMinuteRollup {
  return {
    viewerAvg: 0,
    viewerMax: 0,
    viewerLatest: 0,
    viewerSamples: 0,
    chatCount: 0,
    totalEmoteCount: 0,
    seventvEmoteCount: 0,
    emotes: {},
    ...partial,
  }
}

describe('viewerChartSmoothWindow', () => {
  it('uses a wider window for live collector charts', () => {
    expect(viewerChartSmoothWindow([], 'live', true)).toBe(7)
    expect(viewerChartSmoothWindow([], 'tt')).toBe(5)
  })
})

describe('holdForwardViewerGaps', () => {
  it('carries the last sample across short chat gaps', () => {
    const rollups = [
      minute({ minuteTs: '2026-07-06T18:00:00.000Z', viewerLatest: 20_000, viewerSamples: 1, chatCount: 400 }),
      minute({ minuteTs: '2026-07-06T18:01:00.000Z', chatCount: 380 }),
      minute({ minuteTs: '2026-07-06T18:02:00.000Z', chatCount: 360 }),
      minute({ minuteTs: '2026-07-06T18:03:00.000Z', viewerLatest: 19_500, viewerSamples: 1, chatCount: 350 }),
    ]
    const values = [20_000, 0, 0, 19_500]
    expect(holdForwardViewerGaps(rollups, values)).toEqual([20_000, 20_000, 20_000, 19_500])
  })
})

describe('smoothDisplayValues', () => {
  it('averages interior spikes', () => {
    const smoothed = smoothDisplayValues([10_000, 20_000, 10_000], 3)
    expect(smoothed[1]).toBeGreaterThan(10_000)
    expect(smoothed[1]).toBeLessThan(20_000)
  })
})

describe('findRollupIndexInSeries', () => {
  const rollups = [
    minute({ minuteTs: '2026-07-04T12:04:00.000Z', chatCount: 5 }),
    minute({ minuteTs: '2026-07-04T12:05:00.000Z', chatCount: 12 }),
    minute({ minuteTs: '2026-07-04T12:06:00.000Z', chatCount: 8 }),
  ]

  it('matches by normalized minute bucket across timestamp formats', () => {
    expect(findRollupIndexInSeries(rollups, '2026-07-04T12:05:00Z')).toBe(1)
    expect(rollupMinuteBucketsEqual('2026-07-04T12:05:00Z', '2026-07-04T12:05:00.000Z')).toBe(true)
  })

  it('falls back to nearest rollup within 60 seconds', () => {
    expect(findRollupIndexInSeries(rollups, '2026-07-04T12:05:30.000Z')).toBe(1)
  })

  it('returns -1 when target is outside the series window', () => {
    expect(findRollupIndexInSeries(rollups, '2026-07-04T12:10:00.000Z')).toBe(-1)
  })
})
