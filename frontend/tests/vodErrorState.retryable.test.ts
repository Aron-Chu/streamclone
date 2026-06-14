import assert from 'node:assert/strict'
import { it } from 'node:test'
import fc from 'fast-check'
import {
  describeVodError,
  isVodErrorRetryable,
  type VodErrorInput,
} from '../src/components/channel/vodError.ts'

// Known VOD relay error codes surfaced by POST /v1/stream/vod/start plus the
// client-detected HLS proxy auth condition. Mirrors VodErrorCode.
const KNOWN_CODES = [
  'invalid_vod_id',
  'vod_unavailable',
  'upstream_token_failed',
  'capacity_reached',
  'hls_not_ready',
  'vod_start_failed',
  'hls_proxy_auth',
] as const

// Codes that never expose a retry action regardless of the API flag (Req 2.7).
const NEVER_RETRYABLE = new Set(['invalid_vod_id', 'vod_unavailable'])
// Codes that always expose a retry action regardless of the API flag (Req 2.4, 2.5).
const ALWAYS_RETRYABLE = new Set(['capacity_reached', 'hls_not_ready', 'hls_proxy_auth'])

function hasRetryAction(error: VodErrorInput): boolean {
  return describeVodError(error, {}).actions.some(action => action.kind === 'retry')
}

// Generator over the known error codes plus arbitrary/unknown code strings so
// the property also covers the unknown fallback branch.
const codeArb = fc.oneof(
  fc.constantFrom(...KNOWN_CODES),
  fc.constant(undefined),
  fc.constant(null),
  fc.string(),
)

// Generator over the API retryable flag including null/undefined.
const retryableArb = fc.oneof(
  fc.boolean(),
  fc.constant(undefined),
  fc.constant(null),
)

// Feature: moment-timeline, Property 29: VOD ID Retryable Classification
// **Validates: Requirements 2.7**
it('retry action presence matches isVodErrorRetryable per error code', () => {
  fc.assert(
    fc.property(codeArb, retryableArb, (code, retryable) => {
      const error: VodErrorInput = { code, retryable }
      const classified = isVodErrorRetryable(error)

      // The retry action is present iff the classifier says retryable.
      assert.equal(hasRetryAction(error), classified)

      // The descriptor's own retryable flag agrees with the classifier.
      assert.equal(describeVodError(error, {}).retryable, classified)

      // Spell out the per-code rules from Requirement 2.7.
      if (typeof code === 'string' && NEVER_RETRYABLE.has(code)) {
        assert.equal(classified, false)
      } else if (typeof code === 'string' && ALWAYS_RETRYABLE.has(code)) {
        assert.equal(classified, true)
      } else {
        // vod_start_failed, upstream_token_failed, unknown -> follow the flag.
        assert.equal(classified, retryable === true)
      }
    }),
    { numRuns: 300 },
  )
})
