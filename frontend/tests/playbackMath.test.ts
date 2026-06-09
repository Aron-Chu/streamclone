import assert from 'node:assert/strict'
import test from 'node:test'
import { calculateLiveEdge } from '../src/playbackMath.ts'

test('prefers hls live sync position for jump live', () => {
  const result = calculateLiveEdge({
    currentTimeSec: 100,
    liveSyncPositionSec: 116.5,
    seekableEndSec: 120,
    targetLatencySec: 5,
  })
  assert.equal(result.behindLiveSec, 16.5)
  assert.equal(result.canJumpLive, true)
  assert.equal(result.jumpTargetSec, 116.5)
})

test('falls back to seekable end minus target latency', () => {
  const result = calculateLiveEdge({
    currentTimeSec: 111,
    liveSyncPositionSec: null,
    seekableEndSec: 120,
    targetLatencySec: 5,
  })
  assert.equal(result.behindLiveSec, 9)
  assert.equal(result.canJumpLive, false)
  assert.equal(result.jumpTargetSec, 115)
})

test('uses ten second safe jump threshold minimum', () => {
  const result = calculateLiveEdge({
    currentTimeSec: 100,
    liveSyncPositionSec: null,
    seekableEndSec: 111,
    targetLatencySec: 1,
  })
  assert.equal(result.behindLiveSec, 11)
  assert.equal(result.canJumpLive, true)
  assert.equal(result.jumpTargetSec, 110)
})
