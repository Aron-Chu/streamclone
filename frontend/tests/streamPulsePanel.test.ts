import assert from 'node:assert/strict'
import test from 'node:test'
import {
  PULSE_ROLLUP_WINDOW,
  slicePulseRollups,
  summarizeLsfEmptyState,
} from '../src/utils/pulseEmptyState.ts'

// Stream Pulse panel render depends on JSX (no jsdom in this repo). These tests
// lock the pure panel contract: LSF empty states and the 15-minute rollup window
// used for the chat spike ActivityWaveform.

test('panel LSF contract: disabled reddit yields retry copy', () => {
  const empty = summarizeLsfEmptyState(
    [{ source: 'reddit_lsf', provider: 'off', state: 'unavailable', message: 'reddit provider disabled' }],
    { scraperOffline: true },
  )
  assert.equal(empty.showScraperAction, false)
  assert.match(empty.body, /Reddit/i)
})

test('panel rollup window is 15 minutes for chat spike section', () => {
  assert.equal(PULSE_ROLLUP_WINDOW, 15)
  const rollups = Array.from({ length: 30 }, (_, i) => ({ chatCount: i }))
  assert.equal(slicePulseRollups(rollups).length, 15)
  assert.equal(slicePulseRollups(rollups)[14]?.chatCount, 29)
})

test('panel LSF contract: no sources falls back to loading copy', () => {
  const empty = summarizeLsfEmptyState(undefined)
  assert.equal(empty.title, 'LSF threads loading')
})
