import assert from 'node:assert/strict'
import { it } from 'node:test'
import {
  liveChartWarmupProgress,
  liveWarmupHintLine,
  liveWarmupStatusLine,
} from '../src/utils/liveCollectionWarmup.ts'

it('liveChartWarmupProgress tracks minute bucket threshold', () => {
  assert.deepEqual(liveChartWarmupProgress(0), {
    readyMinutes: 0,
    targetMinutes: 2,
    percent: 0,
    chartReady: false,
  })
  assert.deepEqual(liveChartWarmupProgress(1), {
    readyMinutes: 1,
    targetMinutes: 2,
    percent: 50,
    chartReady: false,
  })
  assert.equal(liveChartWarmupProgress(2).chartReady, true)
})

it('liveWarmupStatusLine explains bucket progress', () => {
  assert.match(liveWarmupStatusLine(liveChartWarmupProgress(0)), /first completed minute/i)
  assert.match(liveWarmupStatusLine(liveChartWarmupProgress(1)), /1 of 2/)
})

it('liveWarmupHintLine surfaces aggregate collector activity', () => {
  assert.equal(liveWarmupHintLine({ viewerSamples: 0, chatMessages: 0 }), null)
  assert.match(
    liveWarmupHintLine({ viewerSamples: 12, chatMessages: 40 }) ?? '',
    /viewer samples/i,
  )
})
