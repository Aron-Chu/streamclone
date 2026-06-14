// Feature: moment-timeline, Property 23: Heatmap Lane Pixel-Column Bound
// **Validates: Requirements 14.3, 24.1**
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import fc from 'fast-check'
import { decimateToPixels } from '../src/utils/heatmapDecimate.ts'
import type { ReplayHeatmapPoint } from '../src/types/heatmap.ts'

const arbPoint = fc.record({
  offsetSeconds: fc.nat({ max: 36000 }),
  durationSeconds: fc.constant(60),
  score: fc.integer({ min: 0, max: 100 }),
  confidence: fc.double({ min: 0, max: 1 }),
  reason: fc.constantFrom('chat_spike', 'viewer_spike', 'seventv_spike'),
  streamId: fc.constant('test'),
  minuteTs: fc.constant('2024-01-01T00:00:00Z'),
  topEmotes: fc.constant([]),
  vodId: fc.constant(null),
})

describe('Property 23: Heatmap Lane Pixel-Column Bound', () => {
  // Property 1: Output length is always ≤ widthPx for any valid inputs
  it('output length is always ≤ widthPx', () => {
    fc.assert(
      fc.property(
        fc.array(arbPoint, { minLength: 1, maxLength: 200 }),
        fc.integer({ min: 1, max: 2000 }),
        fc.integer({ min: 1, max: 36000 }),
        (points, widthPx, totalDurationSec) => {
          const columns = decimateToPixels(points as ReplayHeatmapPoint[], widthPx, totalDurationSec)
          assert.ok(
            columns.length <= widthPx,
            `Expected ≤${widthPx} columns, got ${columns.length}`,
          )
        },
      ),
      { numRuns: 100 },
    )
  })

  // Property 2: Each column has the max score from its pixel-width time range
  it('each column holds the max score for its pixel bucket', () => {
    fc.assert(
      fc.property(
        fc.array(arbPoint, { minLength: 2, maxLength: 100 }),
        fc.integer({ min: 1, max: 500 }),
        fc.integer({ min: 1, max: 36000 }),
        (points, widthPx, totalDurationSec) => {
          const typedPoints = points as ReplayHeatmapPoint[]
          const columns = decimateToPixels(typedPoints, widthPx, totalDurationSec)
          const secsPerColumn = totalDurationSec / widthPx

          for (const col of columns) {
            const colIdx = Math.floor(col.offsetSeconds / secsPerColumn)
            // Find all points in same bucket
            const bucketPoints = typedPoints.filter((p) => {
              const px = Math.floor(p.offsetSeconds / secsPerColumn)
              return px === colIdx && px >= 0 && px < widthPx
            })
            const maxScore = Math.max(...bucketPoints.map((p) => p.score))
            assert.equal(
              col.score,
              maxScore,
              `Column at offset ${col.offsetSeconds} has score ${col.score}, expected max ${maxScore}`,
            )
          }
        },
      ),
      { numRuns: 100 },
    )
  })

  // Property 3: Columns are ordered by offsetSeconds
  it('columns are ordered by offsetSeconds', () => {
    fc.assert(
      fc.property(
        fc.array(arbPoint, { minLength: 2, maxLength: 100 }),
        fc.integer({ min: 1, max: 500 }),
        fc.integer({ min: 1, max: 36000 }),
        (points, widthPx, totalDurationSec) => {
          const columns = decimateToPixels(points as ReplayHeatmapPoint[], widthPx, totalDurationSec)
          for (let i = 1; i < columns.length; i++) {
            assert.ok(
              columns[i].offsetSeconds >= columns[i - 1].offsetSeconds,
              `Column ${i} offset ${columns[i].offsetSeconds} < column ${i - 1} offset ${columns[i - 1].offsetSeconds}`,
            )
          }
        },
      ),
      { numRuns: 100 },
    )
  })

  // Property 4: Empty inputs → empty output
  it('empty points array returns empty output', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 1, max: 2000 }),
        fc.integer({ min: 1, max: 36000 }),
        (widthPx, totalDurationSec) => {
          const columns = decimateToPixels([], widthPx, totalDurationSec)
          assert.equal(columns.length, 0)
        },
      ),
      { numRuns: 100 },
    )
  })

  // Property 5: widthPx ≤ 0 or totalDurationSec ≤ 0 → empty output
  it('invalid widthPx or totalDurationSec returns empty output', () => {
    fc.assert(
      fc.property(
        fc.array(arbPoint, { minLength: 0, maxLength: 50 }),
        fc.oneof(
          // Invalid widthPx with valid duration
          fc.tuple(
            fc.integer({ min: -100, max: 0 }),
            fc.integer({ min: 1, max: 36000 }),
          ),
          // Valid widthPx with invalid duration
          fc.tuple(
            fc.integer({ min: 1, max: 2000 }),
            fc.integer({ min: -100, max: 0 }),
          ),
          // Both invalid
          fc.tuple(
            fc.integer({ min: -100, max: 0 }),
            fc.integer({ min: -100, max: 0 }),
          ),
        ),
        (points, [widthPx, totalDurationSec]) => {
          const columns = decimateToPixels(points as ReplayHeatmapPoint[], widthPx, totalDurationSec)
          assert.equal(columns.length, 0)
        },
      ),
      { numRuns: 100 },
    )
  })

  it('non-finite dimensions and point values return finite columns only', () => {
    const point = {
      offsetSeconds: 10,
      durationSeconds: 60,
      score: 55,
      confidence: 1,
      reason: 'chat_spike',
      streamId: 'test',
      minuteTs: '2026-06-13T00:00:00Z',
      topEmotes: [],
      vodId: null,
    } satisfies ReplayHeatmapPoint

    assert.deepEqual(decimateToPixels([point], Number.NaN, 60), [])
    assert.deepEqual(decimateToPixels([point], 120, Number.POSITIVE_INFINITY), [])
    assert.deepEqual(decimateToPixels([{ ...point, offsetSeconds: Number.NaN }], 120, 60), [])
  })
})
