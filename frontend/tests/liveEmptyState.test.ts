import assert from 'node:assert/strict'
import { it } from 'node:test'
import fc from 'fast-check'
import {
  classifyLiveEmptyState,
  MIN_LIVE_ROLLUPS_FOR_CHART,
  COLLECTING_FIRST_MINUTES_MESSAGE,
  NO_RECENT_DATA_MESSAGE,
} from '../src/utils/liveEmptyState.ts'

// Feature: moment-timeline, Property 4: No Contradictory Empty State During Active Collection
// **Validates: Requirements 7.1**
//
// Whenever collection is active ("Collecting now"), the chart must never
// contradict the stream rail with a "No recent data" empty message. For any
// random rollup count, classifyLiveEmptyState with collectingNow=true MUST
// never return kind 'no-recent-data', MUST set suppressNoRecentData=true, and
// MUST never emit the NO_RECENT_DATA_MESSAGE copy.

// Smart generator: rollup counts spanning the meaningful boundaries — 0, 1, the
// threshold itself, and large values — plus negatives and fractional inputs the
// classifier is expected to floor/clamp to >= 0.
const rollupCount = fc.oneof(
  fc.constant(0),
  fc.constant(1),
  fc.constant(MIN_LIVE_ROLLUPS_FOR_CHART),
  fc.integer({ min: -5, max: 5 }),
  fc.integer({ min: 0, max: 100_000 }),
  fc.double({ min: -10, max: 10, noNaN: true }),
)

it('Property 4: collectingNow never yields no-recent-data and always suppresses it', () => {
  fc.assert(
    fc.property(rollupCount, (count) => {
      const decision = classifyLiveEmptyState({ collectingNow: true, rollupCount: count })
      assert.notEqual(decision.kind, 'no-recent-data')
      assert.equal(decision.suppressNoRecentData, true)
      assert.notEqual(decision.message, NO_RECENT_DATA_MESSAGE)
    }),
    { numRuns: 200 },
  )
})

// Feature: moment-timeline, Property 4: No Contradictory Empty State During Active Collection
// **Validates: Requirements 7.1**
//
// Specific shape (Req 7.1/7.2): collectingNow + fewer than two rollups ->
// 'collecting-first-minutes' with the warming-up copy and an activity indicator.
it('Property 4: collectingNow with <2 rollups shows collecting-first-minutes', () => {
  fc.assert(
    fc.property(
      fc.oneof(fc.constant(0), fc.constant(1), fc.double({ min: 0, max: 1.99, noNaN: true })),
      (count) => {
        const decision = classifyLiveEmptyState({ collectingNow: true, rollupCount: count })
        assert.equal(decision.kind, 'collecting-first-minutes')
        assert.equal(decision.showChart, false)
        assert.equal(decision.showActivityIndicator, true)
        assert.equal(decision.suppressNoRecentData, true)
        assert.equal(decision.message, COLLECTING_FIRST_MINUTES_MESSAGE)
      },
    ),
    { numRuns: 100 },
  )
})

// Feature: moment-timeline, Property 4: No Contradictory Empty State During Active Collection
// **Validates: Requirements 7.1**
//
// Specific shape (Req 7.3): two or more rollups -> chart, regardless of the
// collecting badge. The chart replaces the warming-up message once the count
// crosses the threshold.
it('Property 4: rollupCount >= 2 always yields the chart', () => {
  fc.assert(
    fc.property(
      fc.integer({ min: MIN_LIVE_ROLLUPS_FOR_CHART, max: 100_000 }),
      fc.boolean(),
      (count, collectingNow) => {
        const decision = classifyLiveEmptyState({ collectingNow, rollupCount: count })
        assert.equal(decision.kind, 'chart')
        assert.equal(decision.showChart, true)
        assert.equal(decision.message, null)
        assert.notEqual(decision.message, NO_RECENT_DATA_MESSAGE)
      },
    ),
    { numRuns: 100 },
  )
})

// Boundary unit checks anchoring the property: exactly at and just below the
// chart threshold, and the not-collecting fallback.
it('classifies the rollup threshold boundary and not-collecting fallback', () => {
  assert.equal(
    classifyLiveEmptyState({ collectingNow: true, rollupCount: 1 }).kind,
    'collecting-first-minutes',
  )
  assert.equal(
    classifyLiveEmptyState({ collectingNow: true, rollupCount: MIN_LIVE_ROLLUPS_FOR_CHART }).kind,
    'chart',
  )

  // Not collecting + insufficient data: the only path that surfaces "No recent data".
  const idle = classifyLiveEmptyState({ collectingNow: false, rollupCount: 0 })
  assert.equal(idle.kind, 'no-recent-data')
  assert.equal(idle.suppressNoRecentData, false)
  assert.equal(idle.message, NO_RECENT_DATA_MESSAGE)
})
