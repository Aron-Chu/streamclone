import assert from 'node:assert/strict'
import { it } from 'node:test'
import fc from 'fast-check'
import { normalizeVodId, isVodId } from '../src/utils/vodId.ts'

// Feature: moment-timeline, Property 1: VOD Identifier Normalization Round-Trip
// **Validates: Requirements 1.3, 1.6**
//
// For any raw input whose normalized output is non-null, that output MUST match
// ^\d{5,20}$ and normalization MUST be idempotent (normalizing the output again
// returns the same value).

it('Property 1: valid normalized vod id matches ^\\d{5,20}$ and is idempotent', () => {
  fc.assert(
    fc.property(fc.stringMatching(/^\d{5,20}$/), (raw) => {
      const once = normalizeVodId(raw)
      // A bare 5-20 digit string is a valid VOD identifier.
      assert.notEqual(once, null)
      assert.match(once!, /^\d{5,20}$/)
      assert.equal(isVodId(once!), true)
      // Idempotent: normalizing the output again returns the same value.
      assert.equal(normalizeVodId(once!), once)
    }),
    { numRuns: 100 },
  )
})

// Feature: moment-timeline, Property 1: VOD Identifier Normalization Round-Trip
// **Validates: Requirements 1.3, 1.6**
//
// Surrounding/internal whitespace is stripped; the digits-only core still
// normalizes to a valid identifier and stays idempotent.
it('Property 1: whitespace is stripped before validation', () => {
  fc.assert(
    fc.property(
      fc.stringMatching(/^\d{5,20}$/),
      fc.stringMatching(/^[ \t\n\r]*$/),
      fc.stringMatching(/^[ \t\n\r]*$/),
      (digits, lead, trail) => {
        const once = normalizeVodId(`${lead}${digits}${trail}`)
        assert.equal(once, digits)
        assert.match(once!, /^\d{5,20}$/)
        assert.equal(normalizeVodId(once!), once)
      },
    ),
    { numRuns: 100 },
  )
})

// Feature: moment-timeline, Property 1: VOD Identifier Normalization Round-Trip
// **Validates: Requirements 1.3, 1.6**
//
// The output of normalizeVodId is always a fixed point: applying it to ANY raw
// input and then re-applying it yields the same result.
it('Property 1: normalizeVodId is idempotent for arbitrary input', () => {
  fc.assert(
    fc.property(fc.string(), (raw) => {
      const once = normalizeVodId(raw)
      const twice = normalizeVodId(once)
      assert.equal(twice, once)
      if (once !== null) {
        assert.match(once, /^\d{5,20}$/)
      }
    }),
    { numRuns: 100 },
  )
})

// Rejection cases (Requirement 1.6): empty, whitespace-only, videos/ URL prefix,
// and out-of-range digit lengths must normalize to null.
it('rejects empty and whitespace-only input', () => {
  assert.equal(normalizeVodId(''), null)
  assert.equal(normalizeVodId('   '), null)
  assert.equal(normalizeVodId('\t\n '), null)
  assert.equal(normalizeVodId(null), null)
  assert.equal(normalizeVodId(undefined), null)
})

it('rejects videos/ URL prefixes', () => {
  assert.equal(normalizeVodId('videos/123456789'), null)
  assert.equal(normalizeVodId('https://www.twitch.tv/videos/123456789'), null)
  assert.equal(normalizeVodId('VIDEOS/123456789'), null)
})

it('rejects digit strings outside the 5-20 length range', () => {
  fc.assert(
    fc.property(
      fc.oneof(fc.stringMatching(/^\d{1,4}$/), fc.stringMatching(/^\d{21,30}$/)),
      (raw) => {
        assert.equal(normalizeVodId(raw), null)
      },
    ),
    { numRuns: 100 },
  )
})

it('rejects non-numeric content', () => {
  assert.equal(normalizeVodId('abcde'), null)
  assert.equal(normalizeVodId('12a45'), null)
})
