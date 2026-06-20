import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  rollingMedian3,
  rollingMedianWindow,
  viewerChartSmoothWindow,
  viewerSourceLabel,
} from '../src/components/analytics/chartRollupUtils.ts'
import type { AnalyticsMinuteRollup } from '../src/api.ts'

describe('chartRollupUtils viewer smoothing', () => {
  it('rollingMedianWindow preserves length', () => {
    const values = [100, 5000, 120, 110, null, 200]
    const out = rollingMedianWindow(values, 5)
    assert.equal(out.length, values.length)
  })

  it('rollingMedian3 matches window-3 median', () => {
    const values = [10, 100, 20, 30]
    assert.deepEqual(rollingMedian3(values), rollingMedianWindow(values, 3))
  })

  it('viewerChartSmoothWindow prefers wider smooth for TT', () => {
    assert.equal(viewerChartSmoothWindow([], 'tt'), 5)
    assert.equal(viewerChartSmoothWindow([], 'live'), 3)
    assert.equal(viewerChartSmoothWindow([], 'merged'), 4)
  })

  it('viewerChartSmoothWindow detects dense live rollups', () => {
    const rollups: AnalyticsMinuteRollup[] = Array.from({ length: 10 }, (_, i) => ({
      minuteTs: new Date(Date.UTC(2026, 0, 1, 0, i)).toISOString(),
      viewerAvg: 1000 + i * 10,
      viewerMax: 1000 + i * 10,
      viewerLatest: 1000 + i * 10,
      viewerSamples: 4,
      chatCount: 0,
      totalEmoteCount: 0,
      seventvEmoteCount: 0,
      emotes: {},
    }))
    assert.equal(viewerChartSmoothWindow(rollups), 3)
  })

  it('viewerSourceLabel maps API values', () => {
    assert.equal(viewerSourceLabel('live'), 'Live samples')
    assert.equal(viewerSourceLabel('tt'), 'TwitchTracker')
    assert.equal(viewerSourceLabel('merged'), 'Live + TT gaps')
    assert.equal(viewerSourceLabel('partial'), 'Partial viewers')
  })
})
