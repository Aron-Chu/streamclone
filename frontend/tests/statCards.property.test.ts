// Feature: moment-timeline, Property 3: Stat Card Placeholder Classification
//
// Property-based coverage for `classifyStatCards` (frontend/src/utils/statCards.ts).
// Frontend property tests use fast-check via the runner's standard `it(...)` block
// wrapping `fc.assert(fc.property(...))` — there is NO `test.prop([...], cb)` macro
// in this repo.
//
// **Validates: Requirements 6.1, 6.2, 6.3**

import assert from 'node:assert/strict'
import { it } from 'node:test'
import fc from 'fast-check'
import {
  classifyStatCards,
  type StatCardInput,
  type StatCardRollup,
  type StreamCollectionState,
} from '../src/utils/statCards.ts'

const RUNS = { numRuns: 200 }

// A rollup that counts as a "non-missing viewer-sample" rollup (viewerSamples > 0).
const sampledRollup: fc.Arbitrary<StatCardRollup> = fc.record({
  viewerSamples: fc.integer({ min: 1, max: 100_000 }),
  chatCount: fc.nat({ max: 10_000 }),
  totalEmoteCount: fc.nat({ max: 10_000 }),
  missing: fc.constant(false),
})

// A rollup that does NOT count toward the live viewer-sample count: either missing,
// or non-missing with zero viewer samples (chat/emote may still be present).
const nonSampledRollup: fc.Arbitrary<StatCardRollup> = fc.oneof(
  fc.record({
    missing: fc.constant(true),
    viewerSamples: fc.nat({ max: 100_000 }),
    chatCount: fc.nat({ max: 10_000 }),
    totalEmoteCount: fc.nat({ max: 10_000 }),
  }),
  fc.record({
    missing: fc.constant(false),
    viewerSamples: fc.constant(0),
    chatCount: fc.nat({ max: 10_000 }),
    totalEmoteCount: fc.nat({ max: 10_000 }),
  }),
)

// A rollup carrying no usable data at all: missing, or non-missing all-zero.
const emptyRollup: fc.Arbitrary<StatCardRollup> = fc.oneof(
  fc.record({
    missing: fc.constant(true),
    viewerSamples: fc.nat({ max: 100_000 }),
    chatCount: fc.nat({ max: 10_000 }),
    totalEmoteCount: fc.nat({ max: 10_000 }),
  }),
  fc.record({
    missing: fc.constant(false),
    viewerSamples: fc.constant(0),
    chatCount: fc.constant(0),
    totalEmoteCount: fc.constant(0),
  }),
)

// A rollup with no viewer/chat signal (viewerSamples == 0 && chatCount == 0) but
// possibly some emote count — used for the "Stats only" precondition.
const noViewerChatRollup: fc.Arbitrary<StatCardRollup> = fc.oneof(
  fc.record({
    missing: fc.constant(true),
    viewerSamples: fc.nat({ max: 100_000 }),
    chatCount: fc.nat({ max: 10_000 }),
    totalEmoteCount: fc.nat({ max: 10_000 }),
  }),
  fc.record({
    missing: fc.constant(false),
    viewerSamples: fc.constant(0),
    chatCount: fc.constant(0),
    totalEmoteCount: fc.nat({ max: 10_000 }),
  }),
)

// Req 6.3 — Collecting: a "live" stream with fewer than 2 non-missing rollups that
// have viewer samples MUST classify Chat and Emote Uses as "Collecting" (muted),
// regardless of tracker averages. This rule has highest precedence.
it('Property 3 (6.3): live stream with <2 sampled rollups => Chat/Emote Uses Collecting', () => {
  fc.assert(
    fc.property(
      fc.array(nonSampledRollup, { maxLength: 20 }),
      fc.array(sampledRollup, { minLength: 0, maxLength: 1 }), // 0 or 1 sampled => still < 2
      fc.nat({ max: 100_000 }),
      fc.nat({ max: 100_000 }),
      (nonSampled, sampled, avgViewers, peakViewers) => {
        const rollups = [...nonSampled, ...sampled]
        const input: StatCardInput = { state: 'live', avgViewers, peakViewers, rollups }
        const cards = classifyStatCards(input)

        assert.equal(cards.chat.placeholder, 'Collecting')
        assert.equal(cards.emoteUses.placeholder, 'Collecting')
        assert.equal(cards.chat.muted, true)
        assert.equal(cards.emoteUses.muted, true)
      },
    ),
    RUNS,
  )
})

// Req 6.2 — Needs sync: state "not_collected" with no tracker averages and no rollup
// data MUST classify Current, Average, Peak, Chat, and Emote Uses as "Needs sync".
it('Property 3 (6.2): not_collected with no averages and no rollup data => Needs sync', () => {
  fc.assert(
    fc.property(
      fc.array(emptyRollup, { maxLength: 20 }),
      (rollups) => {
        const input: StatCardInput = {
          state: 'not_collected',
          avgViewers: 0,
          peakViewers: 0,
          rollups,
        }
        const cards = classifyStatCards(input)

        for (const key of ['current', 'average', 'peak', 'chat', 'emoteUses'] as const) {
          assert.equal(cards[key].placeholder, 'Needs sync')
          assert.equal(cards[key].muted, true)
        }
      },
    ),
    RUNS,
  )
})

// Req 6.1 — Stats only: tracker averages present but no rollup with viewer samples
// or chat counts MUST classify Chat and Emote Uses as "Stats only" while Average,
// Peak, and Duration stay numeric. State excludes "live" so the higher-precedence
// Collecting rule does not pre-empt this case.
it('Property 3 (6.1): tracker averages but no viewer/chat rollups => Stats only', () => {
  const nonLiveState: fc.Arbitrary<StreamCollectionState> = fc.constantFrom(
    'historical',
    'not_collected',
    'syncing',
  )
  // At least one of avgViewers / peakViewers must be > 0.
  const trackerAverages = fc.oneof(
    fc.record({
      avgViewers: fc.integer({ min: 1, max: 100_000 }),
      peakViewers: fc.nat({ max: 100_000 }),
    }),
    fc.record({
      avgViewers: fc.nat({ max: 100_000 }),
      peakViewers: fc.integer({ min: 1, max: 100_000 }),
    }),
  )

  fc.assert(
    fc.property(
      nonLiveState,
      trackerAverages,
      fc.array(noViewerChatRollup, { maxLength: 20 }),
      (state, averages, rollups) => {
        const input: StatCardInput = {
          state,
          avgViewers: averages.avgViewers,
          peakViewers: averages.peakViewers,
          rollups,
        }
        const cards = classifyStatCards(input)

        assert.equal(cards.chat.placeholder, 'Stats only')
        assert.equal(cards.emoteUses.placeholder, 'Stats only')
        assert.equal(cards.chat.muted, true)
        assert.equal(cards.emoteUses.muted, true)
        // Tracker-sourced cards stay numeric.
        assert.equal(cards.average.placeholder, null)
        assert.equal(cards.peak.placeholder, null)
        assert.equal(cards.duration.placeholder, null)
      },
    ),
    RUNS,
  )
})
