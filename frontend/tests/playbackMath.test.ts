import assert from 'node:assert/strict'
import test from 'node:test'
import {
  calculateLiveEdge,
  computeEndToEndLiveDelaySec,
  parseHlsTargetDuration,
} from '../src/playbackMath.ts'

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

test('parseHlsTargetDuration defaults invalid values to 2', () => {
  assert.equal(parseHlsTargetDuration(undefined), 2)
  assert.equal(parseHlsTargetDuration(''), 2)
  assert.equal(parseHlsTargetDuration('abc'), 2)
  assert.equal(parseHlsTargetDuration('6'), 6)
})

test('computeEndToEndLiveDelaySec prefers hls.js latency when >= 0.5s', () => {
  const result = computeEndToEndLiveDelaySec(
    { latencyToLiveSec: 8.2, targetLatencySec: 5, behindLiveSec: 0.1 },
    { liveEdge: 3, hlsProbe: { targetDuration: '2' } },
  )
  assert.equal(result.displayDelaySec, 8.2)
  assert.equal(result.relayDelaySec, 6)
})

test('computeEndToEndLiveDelaySec composes relay segments and target buffer', () => {
  const result = computeEndToEndLiveDelaySec(
    { latencyToLiveSec: 0.1, targetLatencySec: 5, behindLiveSec: 0.1 },
    { liveEdge: 3, hlsProbe: { targetDuration: '2' } },
  )
  assert.equal(result.relayDelaySec, 6)
  assert.equal(result.displayDelaySec, 11)
  assert.match(result.tooltip, /Relay ~6(\.0)?s/)
  assert.match(result.tooltip, /buffer ~5(\.0)?s/)
})

test('computeEndToEndLiveDelaySec falls back to behindLiveSec without relay', () => {
  const result = computeEndToEndLiveDelaySec(
    { latencyToLiveSec: null, targetLatencySec: 5, behindLiveSec: 4.5 },
    null,
  )
  assert.equal(result.displayDelaySec, 4.5)
})
