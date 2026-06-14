import assert from 'node:assert/strict'
import test from 'node:test'
import type { SourceStatus } from '../src/api.ts'
import {
  analyticsNotTrackedMessage,
  formatPeakOffset,
  pickTopPeakOffsetSeconds,
  PULSE_ROLLUP_WINDOW,
  rollupsWarmingMessage,
  slicePulseRollups,
  summarizeLsfEmptyState,
} from '../src/utils/pulseEmptyState.ts'

const disabledLsf: SourceStatus = {
  source: 'reddit_lsf',
  provider: 'off',
  state: 'unavailable',
  message: 'reddit provider disabled',
}

const readyLsf: SourceStatus = {
  source: 'reddit_lsf',
  provider: 'public_json',
  state: 'ready',
}

test('summarizeLsfEmptyState: reddit provider off hints retry', () => {
  const content = summarizeLsfEmptyState([disabledLsf], { scraperOffline: true })
  assert.equal(content.title, 'LSF threads unavailable')
  assert.match(content.body, /Reddit/i)
  assert.equal(content.showScraperAction, false)
})

test('summarizeLsfEmptyState: ready source with no posts is a soft empty', () => {
  const content = summarizeLsfEmptyState([readyLsf])
  assert.equal(content.title, 'No LSF threads matched')
  assert.equal(content.showScraperAction, false)
})

test('summarizeLsfEmptyState: blocked source surfaces provider message', () => {
  const content = summarizeLsfEmptyState([
    { source: 'reddit_lsf', provider: 'public_json', state: 'blocked', message: 'status 429' },
  ])
  assert.equal(content.title, 'Reddit temporarily blocked')
  assert.match(content.body, /429/)
})

test('summarizeLsfEmptyState: backoff message is user friendly', () => {
  const content = summarizeLsfEmptyState([
    { source: 'reddit_lsf', provider: 'public_json', state: 'blocked', message: 'provider in backoff' },
  ])
  assert.match(content.body, /cooling down/i)
  assert.doesNotMatch(content.body, /provider in backoff/i)
})

test('summarizeLsfEmptyState: interrupted fetch shows loading copy', () => {
  const content = summarizeLsfEmptyState([
    {
      source: 'reddit_lsf',
      provider: 'scraper',
      state: 'unavailable',
      message: 'fetch interrupted before Reddit responded; retry shortly',
    },
  ])
  assert.equal(content.title, 'LSF threads loading')
  assert.match(content.body, /Still fetching|first load can take/i)
})

test('summarizeLsfEmptyState: warming fetch shows scraper timing copy', () => {
  const content = summarizeLsfEmptyState([
    {
      source: 'reddit_lsf',
      provider: 'warmup',
      state: 'unavailable',
      message: 'fetching from Reddit; first load may take a couple of minutes',
    },
  ])
  assert.equal(content.title, 'LSF threads loading')
  assert.match(content.body, /under a minute/i)
})

test('summarizeLsfEmptyState: pending state explains manual load', () => {
  const content = summarizeLsfEmptyState([
    {
      source: 'reddit_lsf',
      provider: 'pending',
      state: 'unavailable',
      message: 'ready to search Reddit when Analytics is idle',
    },
  ])
  assert.equal(content.title, 'LSF threads')
  assert.match(content.body, /will not start automatically/i)
})

test('slicePulseRollups keeps last window and drops missing rows', () => {
  const rollups = Array.from({ length: 20 }, (_, i) => ({
    minuteTs: `2024-01-01T00:${String(i).padStart(2, '0')}:00Z`,
    chatCount: i,
    missing: i % 5 === 0,
  }))
  const sliced = slicePulseRollups(rollups)
  assert.equal(sliced.length, PULSE_ROLLUP_WINDOW)
  assert.ok(sliced.every(r => !r.missing))
  assert.equal(sliced[0]?.chatCount, 2)
  assert.equal(sliced[sliced.length - 1]?.chatCount, 19)
})

test('pickTopPeakOffsetSeconds returns minute index of highest activity', () => {
  const rollups = [
    { chatCount: 1, totalEmoteCount: 0 },
    { chatCount: 40, totalEmoteCount: 10 },
    { chatCount: 5, totalEmoteCount: 1 },
  ]
  assert.equal(pickTopPeakOffsetSeconds(rollups), 60)
})

test('formatPeakOffset renders mm:ss', () => {
  assert.equal(formatPeakOffset(125), '2:05')
})

test('analytics helper copy mentions tracking', () => {
  assert.match(analyticsNotTrackedMessage(), /track analytics/i)
})

test('rollups warming copy matches live chart placeholder', () => {
  assert.match(rollupsWarmingMessage(), /first minute/i)
})
