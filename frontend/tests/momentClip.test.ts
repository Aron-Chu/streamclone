import assert from 'node:assert/strict'
import { it } from 'node:test'
import fc from 'fast-check'
import { selectBatchClipCandidates } from '../src/utils/momentClip.ts'
import type { ReplayHeatmapPoint } from '../src/types/heatmap.ts'

// Feature: moment-timeline, Property 27: Batch Clip Queue Correctness
// **Validates: Requirements 23.3**

function arbPoint(overrides?: Partial<ReplayHeatmapPoint>): fc.Arbitrary<ReplayHeatmapPoint> {
  return fc.record({
    offsetSeconds: fc.integer({ min: 0, max: 36000 }),
    durationSeconds: fc.constant(60),
    score: fc.integer({ min: 1, max: 100 }),
    confidence: fc.double({ min: 0, max: 1, noNaN: true }),
    reason: fc.constantFrom('chat_spike', 'seventv_spike', 'viewer_spike'),
    topEmotes: fc.constant([]),
    vodId: fc.constant(null),
    streamId: fc.constant('stream-1'),
    minuteTs: fc.constant('2026-06-01T00:00:00Z'),
  }).map(p => ({ ...p, ...overrides }))
}

// Property 27.1: Result always has length ≤ 5
it('Property 27: result length is at most 5', () => {
  fc.assert(
    fc.property(
      fc.array(arbPoint(), { minLength: 0, maxLength: 50 }),
      fc.integer({ min: 1, max: 10000 }),
      fc.integer({ min: 10, max: 600 }),
      (points, chatCount, streamDurationMin) => {
        const result = selectBatchClipCandidates(
          points,
          new Set<number>(),
          chatCount,
          streamDurationMin,
        )
        assert.ok(result.length <= 5, `Expected length ≤ 5, got ${result.length}`)
      },
    ),
    { numRuns: 100 },
  )
})

// Property 27.2: Result is in score-descending order
it('Property 27: result is sorted by score descending', () => {
  fc.assert(
    fc.property(
      fc.array(arbPoint(), { minLength: 0, maxLength: 50 }),
      fc.integer({ min: 1, max: 10000 }),
      fc.integer({ min: 10, max: 600 }),
      (points, chatCount, streamDurationMin) => {
        const result = selectBatchClipCandidates(
          points,
          new Set<number>(),
          chatCount,
          streamDurationMin,
        )
        for (let i = 1; i < result.length; i++) {
          assert.ok(
            result[i - 1].score >= result[i].score,
            `Score at index ${i - 1} (${result[i - 1].score}) < score at index ${i} (${result[i].score})`,
          )
        }
      },
    ),
    { numRuns: 100 },
  )
})

// Property 27.3: No point in result has its minute-offset in the existingJobOffsets set
it('Property 27: no result point has minute-offset in existingJobOffsets', () => {
  fc.assert(
    fc.property(
      fc.array(arbPoint(), { minLength: 1, maxLength: 30 }),
      fc.uniqueArray(fc.integer({ min: 0, max: 600 }), { minLength: 0, maxLength: 10 }),
      fc.integer({ min: 1, max: 10000 }),
      fc.integer({ min: 10, max: 600 }),
      (points, existingOffsets, chatCount, streamDurationMin) => {
        const existing = new Set(existingOffsets)
        const result = selectBatchClipCandidates(
          points,
          existing,
          chatCount,
          streamDurationMin,
        )
        for (const p of result) {
          const minute = Math.floor(p.offsetSeconds / 60)
          assert.ok(
            !existing.has(minute),
            `Result contains point at minute ${minute} which is in existingJobOffsets`,
          )
        }
      },
    ),
    { numRuns: 100 },
  )
})

// Property 27.4: When chatCount <= 0 or streamDurationMin < 10, result is empty
it('Property 27: returns empty when chatCount <= 0 or streamDurationMin < minChatMinutes', () => {
  fc.assert(
    fc.property(
      fc.array(arbPoint(), { minLength: 0, maxLength: 20 }),
      fc.oneof(
        fc.record({
          chatCount: fc.integer({ min: -1000, max: 0 }),
          streamDurationMin: fc.integer({ min: 10, max: 600 }),
        }),
        fc.record({
          chatCount: fc.integer({ min: 1, max: 10000 }),
          streamDurationMin: fc.integer({ min: 0, max: 9 }),
        }),
        fc.record({
          chatCount: fc.integer({ min: -1000, max: 0 }),
          streamDurationMin: fc.integer({ min: 0, max: 9 }),
        }),
      ),
      (points, { chatCount, streamDurationMin }) => {
        const result = selectBatchClipCandidates(
          points,
          new Set<number>(),
          chatCount,
          streamDurationMin,
        )
        assert.equal(result.length, 0, `Expected empty, got ${result.length} results`)
      },
    ),
    { numRuns: 100 },
  )
})

// Property 27.5: Result contains the highest-scoring eligible points (no higher-scoring eligible point was excluded)
it('Property 27: result contains highest-scoring eligible points', () => {
  fc.assert(
    fc.property(
      fc.array(arbPoint(), { minLength: 1, maxLength: 50 }),
      fc.uniqueArray(fc.integer({ min: 0, max: 600 }), { minLength: 0, maxLength: 10 }),
      fc.integer({ min: 1, max: 10000 }),
      fc.integer({ min: 10, max: 600 }),
      (points, existingOffsets, chatCount, streamDurationMin) => {
        const existing = new Set(existingOffsets)
        const result = selectBatchClipCandidates(
          points,
          existing,
          chatCount,
          streamDurationMin,
        )
        // Compute the eligible set independently
        const eligible = points.filter(
          p => !existing.has(Math.floor(p.offsetSeconds / 60)),
        )
        const sortedEligible = [...eligible].sort((a, b) => b.score - a.score)
        const expectedTop5 = sortedEligible.slice(0, 5)

        // The result should have the same length as expectedTop5
        assert.equal(result.length, expectedTop5.length)

        // Every point NOT in the result that is eligible must have score ≤ min score in result
        if (result.length > 0) {
          const minResultScore = result[result.length - 1].score
          for (const ep of eligible) {
            const inResult = result.some(
              r => r.offsetSeconds === ep.offsetSeconds && r.score === ep.score,
            )
            if (!inResult) {
              assert.ok(
                ep.score <= minResultScore,
                `Eligible point (score ${ep.score}) excluded but has higher score than result min (${minResultScore})`,
              )
            }
          }
        }
      },
    ),
    { numRuns: 100 },
  )
})
