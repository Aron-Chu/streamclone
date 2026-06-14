import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import fc from 'fast-check'
import { buildHeatmapPeakAriaLabel } from '../src/utils/heatmapAriaLabel.ts'

// Feature: moment-timeline, Property 31: ARIA Labels on Heatmap Peaks
// **Validates: Requirements 17.4**

const VALID_REASONS = [
  'chat_spike',
  'seventv_spike',
  'twitch_emote_spike',
  'ffz_spike',
  'viewer_spike',
  'game_change',
  'manual',
]

const reasonArb = fc.oneof(
  fc.constantFrom(...VALID_REASONS),
  fc.string({ minLength: 1, maxLength: 30 }).filter(s => s.trim().length > 0),
)

describe('Property 31: ARIA Labels on Heatmap Peaks', () => {
  it('output always contains "Moment at"', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 0, max: 86400 }),
        fc.integer({ min: 0, max: 100 }),
        reasonArb,
        (offset, score, reason) => {
          const label = buildHeatmapPeakAriaLabel(offset, score, reason)
          assert.ok(label.includes('Moment at'), `Label missing "Moment at": ${label}`)
        },
      ),
      { numRuns: 100 },
    )
  })

  it('output contains a valid HH:MM:SS offset', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 0, max: 86400 }),
        fc.integer({ min: 0, max: 100 }),
        reasonArb,
        (offset, score, reason) => {
          const label = buildHeatmapPeakAriaLabel(offset, score, reason)
          const match = label.match(/\d{2,}:\d{2}:\d{2}/)
          assert.ok(match, `Label missing HH:MM:SS pattern: ${label}`)
          const parts = match[0].split(':')
          const mm = parseInt(parts[1], 10)
          const ss = parseInt(parts[2], 10)
          assert.ok(mm >= 0 && mm <= 59, `MM=${mm} out of range`)
          assert.ok(ss >= 0 && ss <= 59, `SS=${ss} out of range`)
        },
      ),
      { numRuns: 100 },
    )
  })

  it('output contains the score as a number', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 0, max: 86400 }),
        fc.integer({ min: 0, max: 100 }),
        reasonArb,
        (offset, score, reason) => {
          const label = buildHeatmapPeakAriaLabel(offset, score, reason)
          assert.ok(
            label.includes(`score ${score}`),
            `Label missing "score ${score}": ${label}`,
          )
        },
      ),
      { numRuns: 100 },
    )
  })

  it('output contains the reason label (human readable)', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 0, max: 86400 }),
        fc.integer({ min: 0, max: 100 }),
        fc.constantFrom(...VALID_REASONS),
        (offset, score, reason) => {
          const label = buildHeatmapPeakAriaLabel(offset, score, reason)
          // Known reasons are mapped to human-readable labels
          const expectedLabels: Record<string, string> = {
            chat_spike: 'chat spike',
            seventv_spike: '7TV spike',
            twitch_emote_spike: 'Twitch emote spike',
            ffz_spike: 'FFZ spike',
            viewer_spike: 'viewer spike',
            game_change: 'game change',
            manual: 'manual',
          }
          const expected = expectedLabels[reason]
          assert.ok(
            label.includes(expected),
            `Label missing reason "${expected}": ${label}`,
          )
        },
      ),
      { numRuns: 100 },
    )
  })

  it('output matches expected format for any combination of valid inputs', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 0, max: 86400 }),
        fc.integer({ min: 0, max: 100 }),
        reasonArb,
        (offset, score, reason) => {
          const label = buildHeatmapPeakAriaLabel(offset, score, reason)
          // Format: "Moment at HH:MM:SS, score N, <reason>"
          const pattern = /^Moment at \d{2,}:\d{2}:\d{2}, score \d+, .+$/
          assert.match(label, pattern, `Label format mismatch: ${label}`)
        },
      ),
      { numRuns: 100 },
    )
  })
})
