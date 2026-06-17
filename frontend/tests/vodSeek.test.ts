import assert from 'node:assert/strict'
import test from 'node:test'

import {
  isRelativeSeekInWindow,
  nearestMomentIndex,
  needsVodRelayRestart,
  vodRelativeSeekSeconds,
} from '../src/utils/vodSeek.ts'

test('vodRelativeSeekSeconds maps absolute VOD time to relay timeline', () => {
  assert.equal(vodRelativeSeekSeconds(4541, 4511), 30)
  assert.equal(vodRelativeSeekSeconds(100, 4511), 0)
})

test('isRelativeSeekInWindow checks seekable ranges', () => {
  assert.equal(isRelativeSeekInWindow(15, [{ start: 0, end: 120 }]), true)
  assert.equal(isRelativeSeekInWindow(200, [{ start: 0, end: 120 }]), false)
  assert.equal(isRelativeSeekInWindow(90, [], 120), true)
})

test('nearestMomentIndex picks closest offset', () => {
  assert.equal(nearestMomentIndex([100, 500, 900], 480), 1)
  assert.equal(nearestMomentIndex([100, 500, 900], 50), 0)
  assert.equal(nearestMomentIndex([], 100), -1)
})

test('needsVodRelayRestart is false for in-window relative time', () => {
  assert.equal(needsVodRelayRestart(4541, 4511, [{ start: 0, end: 120 }]), false)
  assert.equal(needsVodRelayRestart(4530, 4511, [{ start: 0, end: 120 }]), false)
})

test('needsVodRelayRestart is true for far absolute offset', () => {
  assert.equal(needsVodRelayRestart(5000, 4511, [{ start: 0, end: 120 }]), true)
  assert.equal(needsVodRelayRestart(4700, 4511, [{ start: 0, end: 120 }]), true)
})

test('needsVodRelayRestart without seekable ranges uses padSeconds', () => {
  assert.equal(needsVodRelayRestart(4520, 4511, [], 30), false)
  assert.equal(needsVodRelayRestart(4600, 4511, [], 30), true)
})
