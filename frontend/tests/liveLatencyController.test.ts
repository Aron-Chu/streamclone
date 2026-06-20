import test from 'node:test'
import assert from 'node:assert/strict'
import { createLiveLatencyController } from '../src/liveLatencyController.ts'

test('jumps when far behind live edge', () => {
  const controller = createLiveLatencyController('fast')
  const out = controller.tick({
    behindLiveSec: 12,
    targetLatencySec: 2,
    bufferSizeSec: 3,
    stalls: 0,
    fetchRatio: null,
    userMode: 'fast',
    effectiveMode: 'fast',
    levelCount: 3,
    currentLevel: 2,
  })
  assert.equal(out.shouldJumpLive, true)
  assert.equal(out.playbackRate, 1)
})

test('raises catch-up rate when moderately behind', () => {
  const controller = createLiveLatencyController('fast')
  const out = controller.tick({
    behindLiveSec: 7,
    targetLatencySec: 2,
    bufferSizeSec: 4,
    stalls: 0,
    fetchRatio: null,
    userMode: 'fast',
    effectiveMode: 'fast',
    levelCount: 3,
    currentLevel: 2,
  })
  assert.equal(out.shouldJumpLive, false)
  assert.ok(out.playbackRate > 1)
  assert.equal(out.maxLiveSyncPlaybackRate, 1.2)
})

test('downgrades mode after repeated stalls', () => {
  const controller = createLiveLatencyController('instant')
  const out = controller.tick({
    behindLiveSec: 3,
    targetLatencySec: 1,
    bufferSizeSec: 2,
    stalls: 3,
    fetchRatio: null,
    userMode: 'instant',
    effectiveMode: 'instant',
    levelCount: 2,
    currentLevel: 1,
  })
  assert.equal(out.effectiveMode, 'fast')
})

test('caps level when fetch ratio is high', () => {
  const controller = createLiveLatencyController('stable')
  const out = controller.tick({
    behindLiveSec: 2,
    targetLatencySec: 4,
    bufferSizeSec: 5,
    stalls: 0,
    fetchRatio: 1.5,
    userMode: 'stable',
    effectiveMode: 'stable',
    levelCount: 4,
    currentLevel: 3,
  })
  assert.equal(out.levelCap, 2)
})
