// Feature: moment-timeline, Property: Cursor Sync Guards
//
// Unit and property tests for chart ↔ VOD player cursor sync guard conditions.
// Validates: Requirements 22.3

import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import fc from 'fast-check'
import {
  isPlayerLinkedToChart,
  computeChartCursorSync,
  resolveChartClickSeek,
  type ChartCursorSyncInput,
  type PlayheadSnapshot,
} from '../src/utils/chartCursorSync.ts'

// ---------------------------------------------------------------------------
// Unit tests: isPlayerLinkedToChart
// ---------------------------------------------------------------------------

describe('isPlayerLinkedToChart', () => {
  it('returns true when chartStreamId and playhead.streamId match and are non-null', () => {
    const input: ChartCursorSyncInput = {
      chartStreamId: 'stream-123',
      playhead: { streamId: 'stream-123', isPlaying: true, offsetSeconds: 42 },
    }
    assert.equal(isPlayerLinkedToChart(input), true)
  })

  it('returns false when playhead is null (no player)', () => {
    const input: ChartCursorSyncInput = { chartStreamId: 'stream-123', playhead: null }
    assert.equal(isPlayerLinkedToChart(input), false)
  })

  it('returns false when chartStreamId is null', () => {
    const input: ChartCursorSyncInput = {
      chartStreamId: null,
      playhead: { streamId: 'stream-123', isPlaying: true, offsetSeconds: 10 },
    }
    assert.equal(isPlayerLinkedToChart(input), false)
  })

  it('returns false when playhead.streamId is null', () => {
    const input: ChartCursorSyncInput = {
      chartStreamId: 'stream-123',
      playhead: { streamId: null, isPlaying: true, offsetSeconds: 10 },
    }
    assert.equal(isPlayerLinkedToChart(input), false)
  })

  it('returns false when stream IDs do not match', () => {
    const input: ChartCursorSyncInput = {
      chartStreamId: 'stream-123',
      playhead: { streamId: 'stream-456', isPlaying: true, offsetSeconds: 10 },
    }
    assert.equal(isPlayerLinkedToChart(input), false)
  })

  it('returns true even when player is not playing (link check ignores isPlaying)', () => {
    const input: ChartCursorSyncInput = {
      chartStreamId: 'stream-123',
      playhead: { streamId: 'stream-123', isPlaying: false, offsetSeconds: 0 },
    }
    assert.equal(isPlayerLinkedToChart(input), true)
  })
})

// ---------------------------------------------------------------------------
// Unit tests: computeChartCursorSync
// ---------------------------------------------------------------------------

describe('computeChartCursorSync', () => {
  it('synced when same stream and player is playing', () => {
    const input: ChartCursorSyncInput = {
      chartStreamId: 'abc',
      playhead: { streamId: 'abc', isPlaying: true, offsetSeconds: 99.5 },
    }
    const result = computeChartCursorSync(input)
    assert.equal(result.synced, true)
    assert.equal(result.cursorOffsetSeconds, 99.5)
  })

  it('not synced when player is paused (inactive)', () => {
    const input: ChartCursorSyncInput = {
      chartStreamId: 'abc',
      playhead: { streamId: 'abc', isPlaying: false, offsetSeconds: 50 },
    }
    const result = computeChartCursorSync(input)
    assert.equal(result.synced, false)
    assert.equal(result.cursorOffsetSeconds, null)
  })

  it('not synced when playhead is null', () => {
    const input: ChartCursorSyncInput = { chartStreamId: 'abc', playhead: null }
    const result = computeChartCursorSync(input)
    assert.equal(result.synced, false)
    assert.equal(result.cursorOffsetSeconds, null)
  })

  it('not synced when stream IDs mismatch', () => {
    const input: ChartCursorSyncInput = {
      chartStreamId: 'abc',
      playhead: { streamId: 'def', isPlaying: true, offsetSeconds: 30 },
    }
    const result = computeChartCursorSync(input)
    assert.equal(result.synced, false)
    assert.equal(result.cursorOffsetSeconds, null)
  })

  it('offset clamped to 0 for negative values', () => {
    const input: ChartCursorSyncInput = {
      chartStreamId: 'x',
      playhead: { streamId: 'x', isPlaying: true, offsetSeconds: -5 },
    }
    const result = computeChartCursorSync(input)
    assert.equal(result.synced, true)
    assert.equal(result.cursorOffsetSeconds, 0)
  })

  it('offset clamped to 0 for NaN', () => {
    const input: ChartCursorSyncInput = {
      chartStreamId: 'x',
      playhead: { streamId: 'x', isPlaying: true, offsetSeconds: NaN },
    }
    const result = computeChartCursorSync(input)
    assert.equal(result.synced, true)
    assert.equal(result.cursorOffsetSeconds, 0)
  })

  it('offset clamped to 0 for Infinity', () => {
    const input: ChartCursorSyncInput = {
      chartStreamId: 'x',
      playhead: { streamId: 'x', isPlaying: true, offsetSeconds: Infinity },
    }
    const result = computeChartCursorSync(input)
    assert.equal(result.synced, true)
    assert.equal(result.cursorOffsetSeconds, 0)
  })
})

// ---------------------------------------------------------------------------
// Unit tests: resolveChartClickSeek
// ---------------------------------------------------------------------------

describe('resolveChartClickSeek', () => {
  it('shouldSeek when player linked to chart', () => {
    const input: ChartCursorSyncInput = {
      chartStreamId: 's1',
      playhead: { streamId: 's1', isPlaying: true, offsetSeconds: 10 },
    }
    const result = resolveChartClickSeek(input, 120)
    assert.equal(result.shouldSeek, true)
    assert.equal(result.seekOffsetSeconds, 120)
  })

  it('shouldSeek even when player is paused (link check does not require isPlaying)', () => {
    const input: ChartCursorSyncInput = {
      chartStreamId: 's1',
      playhead: { streamId: 's1', isPlaying: false, offsetSeconds: 10 },
    }
    const result = resolveChartClickSeek(input, 60)
    assert.equal(result.shouldSeek, true)
    assert.equal(result.seekOffsetSeconds, 60)
  })

  it('should not seek when playhead is null', () => {
    const input: ChartCursorSyncInput = { chartStreamId: 's1', playhead: null }
    const result = resolveChartClickSeek(input, 120)
    assert.equal(result.shouldSeek, false)
    assert.equal(result.seekOffsetSeconds, null)
  })

  it('should not seek when stream IDs mismatch', () => {
    const input: ChartCursorSyncInput = {
      chartStreamId: 's1',
      playhead: { streamId: 's2', isPlaying: true, offsetSeconds: 10 },
    }
    const result = resolveChartClickSeek(input, 100)
    assert.equal(result.shouldSeek, false)
    assert.equal(result.seekOffsetSeconds, null)
  })

  it('seek offset clamped to 0 for negative click', () => {
    const input: ChartCursorSyncInput = {
      chartStreamId: 's1',
      playhead: { streamId: 's1', isPlaying: true, offsetSeconds: 10 },
    }
    const result = resolveChartClickSeek(input, -10)
    assert.equal(result.shouldSeek, true)
    assert.equal(result.seekOffsetSeconds, 0)
  })
})

// ---------------------------------------------------------------------------
// Property test: guard conditions (Req 22.3)
// ---------------------------------------------------------------------------

describe('Cursor Sync Guards - Property Tests', () => {
  const arbStreamId = fc.stringMatching(/^[a-z0-9]{3,10}$/)

  const arbPlayhead: fc.Arbitrary<PlayheadSnapshot> = fc.record({
    streamId: fc.oneof(arbStreamId, fc.constant(null)),
    isPlaying: fc.boolean(),
    offsetSeconds: fc.double({ min: -100, max: 36000, noNaN: true }),
  })

  // **Validates: Requirements 22.3**
  it('Property: not synced when stream IDs mismatch, playhead null, or not playing', () => {
    fc.assert(
      fc.property(
        fc.oneof(arbStreamId, fc.constant(null)),
        fc.oneof(arbPlayhead, fc.constant(null)),
        (chartStreamId, playhead) => {
          const input: ChartCursorSyncInput = { chartStreamId, playhead }
          const result = computeChartCursorSync(input)

          // Determine if we expect sync
          const shouldSync =
            playhead !== null &&
            chartStreamId !== null &&
            playhead.streamId !== null &&
            chartStreamId === playhead.streamId &&
            playhead.isPlaying

          if (!shouldSync) {
            assert.equal(result.synced, false, 'Expected synced=false when guard conditions not met')
            assert.equal(result.cursorOffsetSeconds, null, 'Expected null offset when not synced')
          } else {
            assert.equal(result.synced, true, 'Expected synced=true when all guards pass')
            assert.equal(typeof result.cursorOffsetSeconds, 'number')
            assert.ok(
              result.cursorOffsetSeconds! >= 0,
              'Offset must be >= 0 (clamped)',
            )
          }
        },
      ),
      { numRuns: 100 },
    )
  })

  // **Validates: Requirements 22.3**
  it('Property: chart click seek only when isPlayerLinkedToChart is true', () => {
    fc.assert(
      fc.property(
        fc.oneof(arbStreamId, fc.constant(null)),
        fc.oneof(arbPlayhead, fc.constant(null)),
        fc.double({ min: -100, max: 36000, noNaN: true }),
        (chartStreamId, playhead, clickOffset) => {
          const input: ChartCursorSyncInput = { chartStreamId, playhead }
          const linked = isPlayerLinkedToChart(input)
          const result = resolveChartClickSeek(input, clickOffset)

          if (!linked) {
            assert.equal(result.shouldSeek, false)
            assert.equal(result.seekOffsetSeconds, null)
          } else {
            assert.equal(result.shouldSeek, true)
            assert.equal(typeof result.seekOffsetSeconds, 'number')
            assert.ok(result.seekOffsetSeconds! >= 0, 'Seek offset must be >= 0')
          }
        },
      ),
      { numRuns: 100 },
    )
  })
})
