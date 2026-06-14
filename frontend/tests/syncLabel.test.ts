import assert from 'node:assert/strict'
import { it } from 'node:test'
import fc from 'fast-check'
import {
  SYNC_CTA_PLACEMENTS,
  SYNCING_LABEL,
  syncCtaForPlacement,
  syncCtaLabel,
  type SyncStreamState,
} from '../src/utils/syncLabel.ts'

// Feature: moment-timeline, Property 2: Sync CTA Label Consistency Across Placements
// **Validates: Requirements 4.1**
//
// For any SyncStreamState, every placement in SYNC_CTA_PLACEMENTS that renders a
// visible primary CTA MUST show the IDENTICAL label string. The only divergence
// permitted by Req 4 is the sync-complete state where the syncPanel shows the
// contextual "Re-sync" while the other placements are hidden — in that case the
// visible set has a single member, so the "all visible labels identical"
// invariant still holds.

const stateArb: fc.Arbitrary<SyncStreamState> = fc.record({
  hasViewerSamples: fc.boolean(),
  hasChatRollups: fc.boolean(),
  syncing: fc.boolean(),
})

it('Property 2: all visible CTA placements share an identical label per state', () => {
  fc.assert(
    fc.property(stateArb, (state) => {
      const visibleLabels = SYNC_CTA_PLACEMENTS.map((p) => syncCtaForPlacement(state, p))
        .filter((r) => r.visible)
        .map((r) => {
          // A visible CTA must always carry a non-null label.
          assert.notEqual(r.label, null)
          return r.label
        })

      // Every visible label across placements must be the same string.
      const unique = new Set(visibleLabels)
      assert.ok(
        unique.size <= 1,
        `expected one shared visible label, saw ${[...unique].join(' | ')}`,
      )
    }),
    { numRuns: 200 },
  )
})

// Feature: moment-timeline, Property 2: Sync CTA Label Consistency Across Placements
// **Validates: Requirements 4.1**
//
// While syncing, EVERY placement shows the identical "Syncing…" label and is
// visible (Req 4.4 — replace the label in all placements during sync).
it('Property 2: while syncing every placement shows the identical "Syncing…" label', () => {
  fc.assert(
    fc.property(fc.boolean(), fc.boolean(), (hasViewerSamples, hasChatRollups) => {
      const state: SyncStreamState = { hasViewerSamples, hasChatRollups, syncing: true }
      for (const placement of SYNC_CTA_PLACEMENTS) {
        const result = syncCtaForPlacement(state, placement)
        assert.equal(result.visible, true)
        assert.equal(result.label, SYNCING_LABEL)
        assert.equal(result.disabled, true)
      }
    }),
    { numRuns: 100 },
  )
})

// Feature: moment-timeline, Property 2: Sync CTA Label Consistency Across Placements
// **Validates: Requirements 4.1**
//
// When sync is available (not syncing, no chat rollups yet), every placement is
// visible and shows the identical label produced by syncCtaLabel(state).
it('Property 2: when sync is available all placements match syncCtaLabel(state)', () => {
  fc.assert(
    fc.property(fc.boolean(), (hasViewerSamples) => {
      const state: SyncStreamState = {
        hasViewerSamples,
        hasChatRollups: false,
        syncing: false,
      }
      const expected = syncCtaLabel(state)
      for (const placement of SYNC_CTA_PLACEMENTS) {
        const result = syncCtaForPlacement(state, placement)
        assert.equal(result.visible, true)
        assert.equal(result.label, expected)
      }
    }),
    { numRuns: 100 },
  )
})
