// Feature: moment-timeline, Tests: Chat Replay UI States
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import fc from 'fast-check'
import {
  computeBucketStart,
  computeBucketEnd,
  classifyReplayState,
  type ReplayData,
} from '../src/utils/vodChatReplay.ts'

// --- Bucket crossing tests ---

describe('computeBucketStart – bucket crossing', () => {
  it('offsets 0-59 map to bucket 0', () => {
    for (let i = 0; i < 60; i++) {
      assert.equal(computeBucketStart(i), 0)
    }
  })

  it('offsets 60-119 map to bucket 60', () => {
    for (let i = 60; i < 120; i++) {
      assert.equal(computeBucketStart(i), 60)
    }
  })

  it('offsets 120-179 map to bucket 120', () => {
    for (let i = 120; i < 180; i++) {
      assert.equal(computeBucketStart(i), 120)
    }
  })

  it('property: bucketStart is always a multiple of 60', () => {
    fc.assert(
      fc.property(fc.nat({ max: 86400 }), (offset) => {
        const bucket = computeBucketStart(offset)
        assert.equal(bucket % 60, 0)
      }),
      { numRuns: 100 },
    )
  })

  it('property: offset is always within [bucketStart, bucketStart+59]', () => {
    fc.assert(
      fc.property(fc.nat({ max: 86400 }), (offset) => {
        const bucket = computeBucketStart(offset)
        assert.ok(offset >= bucket, `offset ${offset} < bucket ${bucket}`)
        assert.ok(offset <= bucket + 59, `offset ${offset} > bucket+59 ${bucket + 59}`)
      }),
      { numRuns: 100 },
    )
  })

  it('property: consecutive offsets in same minute produce same bucket', () => {
    fc.assert(
      fc.property(fc.nat({ max: 86340 }), (offset) => {
        const bucket = computeBucketStart(offset)
        // All offsets in the same minute bucket should produce the same value
        for (let i = 0; i < 60 && offset + i < 86400; i++) {
          if (Math.floor((offset + i) / 60) === Math.floor(offset / 60)) {
            assert.equal(computeBucketStart(offset + i), bucket)
          }
        }
      }),
      { numRuns: 100 },
    )
  })
})

describe('computeBucketEnd', () => {
  it('returns bucketStart + 59', () => {
    assert.equal(computeBucketEnd(0), 59)
    assert.equal(computeBucketEnd(60), 119)
    assert.equal(computeBucketEnd(120), 179)
  })

  it('property: bucketEnd is always bucketStart + 59', () => {
    fc.assert(
      fc.property(fc.nat({ max: 1440 }), (minuteIndex) => {
        const bucketStart = minuteIndex * 60
        assert.equal(computeBucketEnd(bucketStart), bucketStart + 59)
      }),
      { numRuns: 100 },
    )
  })
})

// --- classifyReplayState tests: empty-minute vs no-data ---

describe('classifyReplayState – empty-minute vs no-data', () => {
  it('messages=[], unavailable=false → empty_minute', () => {
    const data: ReplayData = { messages: [], unavailable: false }
    const state = classifyReplayState(data, false, false, false)
    assert.equal(state, 'empty_minute')
  })

  it('unavailable=true → unavailable (sync CTA)', () => {
    const data: ReplayData = { messages: [], unavailable: true }
    const state = classifyReplayState(data, false, false, false)
    assert.equal(state, 'unavailable')
  })

  it('undefined data while loading → loading', () => {
    const state = classifyReplayState(undefined, true, false, false)
    assert.equal(state, 'loading')
  })

  it('undefined data not loading → loading (fallback)', () => {
    const state = classifyReplayState(undefined, false, false, false)
    assert.equal(state, 'loading')
  })

  it('error state → error', () => {
    const state = classifyReplayState(undefined, false, true, false)
    assert.equal(state, 'error')
  })

  it('has messages → has_messages', () => {
    const data: ReplayData = { messages: [{ text: 'hello' }], unavailable: false }
    const state = classifyReplayState(data, false, false, false)
    assert.equal(state, 'has_messages')
  })
})

// --- Sync CTA: unavailable means no persisted messages ---

describe('classifyReplayState – sync CTA', () => {
  it('unavailable=true regardless of syncing → unavailable takes priority', () => {
    const data: ReplayData = { messages: [], unavailable: true }
    // unavailable takes priority over isSyncing because there's no data to show
    const state = classifyReplayState(data, false, false, true)
    assert.equal(state, 'unavailable')
  })

  it('unavailable=true means sync needed even when not actively syncing', () => {
    const data: ReplayData = { messages: [], unavailable: true }
    const state = classifyReplayState(data, false, false, false)
    assert.equal(state, 'unavailable')
  })
})

// --- Partial sync progress ---

describe('classifyReplayState – partial sync progress', () => {
  it('isSyncing=true + has messages → syncing_with_messages', () => {
    const data: ReplayData = { messages: [{ text: 'hi' }], unavailable: false }
    const state = classifyReplayState(data, false, false, true)
    assert.equal(state, 'syncing_with_messages')
  })

  it('isSyncing=true + empty messages → syncing_empty', () => {
    const data: ReplayData = { messages: [], unavailable: false }
    const state = classifyReplayState(data, false, false, true)
    assert.equal(state, 'syncing_empty')
  })

  it('property: syncing with available data always classifies as syncing_*', () => {
    fc.assert(
      fc.property(
        fc.array(fc.record({ text: fc.string() }), { minLength: 0, maxLength: 5 }),
        (messages) => {
          const data: ReplayData = { messages, unavailable: false }
          const state = classifyReplayState(data, false, false, true)
          if (messages.length > 0) {
            assert.equal(state, 'syncing_with_messages')
          } else {
            assert.equal(state, 'syncing_empty')
          }
        },
      ),
      { numRuns: 100 },
    )
  })
})

// --- Priority ordering: loading > error > unavailable > syncing > empty/messages ---

describe('classifyReplayState – priority ordering', () => {
  it('loading takes priority over everything else', () => {
    const data: ReplayData = { messages: [{ text: 'x' }], unavailable: true }
    assert.equal(classifyReplayState(data, true, false, true), 'loading')
  })

  it('error takes priority over data and syncing', () => {
    const data: ReplayData = { messages: [{ text: 'x' }], unavailable: false }
    assert.equal(classifyReplayState(data, false, true, true), 'error')
  })
})
