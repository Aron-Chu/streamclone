import assert from 'node:assert/strict'
import test from 'node:test'
import { mergeRecapMoments } from '../src/recapMoments.ts'

test('mergeRecapMoments dedupes nearby offsets and keeps higher score', () => {
  const merged = mergeRecapMoments(
    {
      topMoments: [{ offsetSeconds: 120, score: 80, chatCount: 10 }],
      clipCandidates: [{ offsetSeconds: 125, score: 95, chatCount: 12 }],
    },
    undefined,
    10,
    true,
  )
  assert.equal(merged.length, 1)
  assert.equal(merged[0]?.score, 95)
})

test('mergeRecapMoments respects limit', () => {
  const merged = mergeRecapMoments(
    {
      topMoments: [
        { offsetSeconds: 0, score: 90 },
        { offsetSeconds: 300, score: 80 },
        { offsetSeconds: 600, score: 70 },
      ],
    },
    undefined,
    2,
    false,
  )
  assert.equal(merged.length, 2)
})
