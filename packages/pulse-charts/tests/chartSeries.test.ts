import { describe, expect, it } from 'vitest'
import { buildChartSeries } from '../src/chartSeries.ts'
import type { ChartMinuteRollup } from '../src/types.ts'

const rollups: ChartMinuteRollup[] = [
  {
    minuteTs: '2026-07-04T18:00:00.000Z',
    viewerAvg: 1000,
    viewerMax: 1000,
    viewerLatest: 1000,
    viewerSamples: 1,
    chatCount: 50,
    totalEmoteCount: 10,
    seventvEmoteCount: 8,
    emotes: { 'seventv:o7:o7': 3 },
  },
  {
    minuteTs: '2026-07-04T18:01:00.000Z',
    viewerAvg: 1100,
    viewerMax: 1100,
    viewerLatest: 1100,
    viewerSamples: 1,
    chatCount: 40,
    totalEmoteCount: 5,
    seventvEmoteCount: 4,
    emotes: {},
  },
]

describe('buildChartSeries plotted emotes', () => {
  it('uses null for minutes without emote usage so trace lanes spike instead of flatlining', () => {
    const selected = new Set(['seventv:eece963b-2e60-4957-b358-98224ffc1ece:o7'])
    const series = buildChartSeries(rollups, selected)
    const plotted = series.find((item) => item.dashed)
    expect(plotted?.values).toEqual([3, null])
  })
})
