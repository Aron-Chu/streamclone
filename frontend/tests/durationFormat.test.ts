import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import fc from 'fast-check'
import { formatDuration } from '../src/utils/durationFormat.ts'

// Feature: moment-timeline, Property 28: Duration Format
// **Validates: Requirements 20.2**

describe('Property 28: Duration Format', () => {
  it('output always matches ^\\d{2,}:\\d{2}:\\d{2}$', () => {
    fc.assert(
      fc.property(fc.integer({ min: 0, max: 999999 }), (n) => {
        const result = formatDuration(n)
        assert.match(result, /^\d{2,}:\d{2}:\d{2}$/)
      }),
      { numRuns: 100 },
    )
  })

  it('MM and SS components are in [00,59] for any non-negative integer', () => {
    fc.assert(
      fc.property(fc.integer({ min: 0, max: 999999 }), (n) => {
        const result = formatDuration(n)
        const parts = result.split(':')
        const mm = parseInt(parts[1], 10)
        const ss = parseInt(parts[2], 10)
        assert.ok(mm >= 0 && mm <= 59, `MM=${mm} out of range for input ${n}`)
        assert.ok(ss >= 0 && ss <= 59, `SS=${ss} out of range for input ${n}`)
      }),
      { numRuns: 100 },
    )
  })

  it('formatDuration(0) → "00:00:00"', () => {
    assert.equal(formatDuration(0), '00:00:00')
  })

  it('formatDuration(3661) → "01:01:01"', () => {
    assert.equal(formatDuration(3661), '01:01:01')
  })

  it('negative inputs → "00:00:00"', () => {
    fc.assert(
      fc.property(fc.integer({ min: -1_000_000, max: -1 }), (n) => {
        assert.equal(formatDuration(n), '00:00:00')
      }),
      { numRuns: 100 },
    )
  })

  it('NaN and Infinity → "00:00:00"', () => {
    assert.equal(formatDuration(NaN), '00:00:00')
    assert.equal(formatDuration(Infinity), '00:00:00')
    assert.equal(formatDuration(-Infinity), '00:00:00')
  })

  it('large n (>= 360000) → HH has 3+ digits', () => {
    fc.assert(
      fc.property(fc.integer({ min: 360000, max: 9_999_999 }), (n) => {
        const result = formatDuration(n)
        const hh = result.split(':')[0]
        assert.ok(hh.length >= 3, `HH="${hh}" should be 3+ digits for input ${n}`)
      }),
      { numRuns: 100 },
    )
  })
})
