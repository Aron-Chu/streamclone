// Feature: auto-clipper-replayforge-productization, Property 15: `/studio`
// redirect resolves to the job's Clip Studio URL.
//
// Covers RF-P5-005 (Recent Clips listing with `/studio` links) — the pure
// studio-link builders used by the Recent Clips listing, the retry redirect,
// and the `/studio` redirect component.
//
// **Validates: Requirements 6.3**

import assert from 'node:assert/strict'
import test, { it } from 'node:test'
import fc from 'fast-check'
import { studioPath, replayforgeStudioUrl } from '../src/utils/studioLink.ts'

// Token shapes that MUST never appear in a studio link — Streamclone owns the
// redirect and never carries clip/access/refresh/auth tokens in the URL.
const TOKEN_MARKERS = [
  'token',
  'bearer',
  'access_token',
  'refresh_token',
  'authorization',
  'clips:edit',
]

function assertTokenFree(link: string) {
  const lower = link.toLowerCase()
  for (const marker of TOKEN_MARKERS) {
    assert.ok(!lower.includes(marker), `link must not contain token marker ${marker}: ${link}`)
  }
}

// ── Unit examples ──────────────────────────────────────────────────────────

test('studioPath builds the in-app route from the job id', () => {
  assert.equal(studioPath('rf_abc123'), '/studio/rf_abc123')
})

test('studioPath resolves blank/whitespace/nullish ids to the archive root', () => {
  assert.equal(studioPath(''), '/studio')
  assert.equal(studioPath('   '), '/studio')
  assert.equal(studioPath(null), '/studio')
  assert.equal(studioPath(undefined), '/studio')
})

test('studioPath trims and encodes opaque ids into a single safe segment', () => {
  assert.equal(studioPath('  rf 1/2  '), `/studio/${encodeURIComponent('rf 1/2')}`)
})

test('replayforgeStudioUrl composes the ReplayForge base with the job segment', () => {
  assert.equal(
    replayforgeStudioUrl('http://localhost:8096', 'rf_abc123'),
    'http://localhost:8096/studio/rf_abc123',
  )
})

test('replayforgeStudioUrl normalizes a trailing slash on the base', () => {
  assert.equal(
    replayforgeStudioUrl('http://localhost:8096/', 'rf_abc123'),
    'http://localhost:8096/studio/rf_abc123',
  )
  assert.equal(replayforgeStudioUrl('http://localhost:8096///', ''), 'http://localhost:8096/studio')
})

// ── Property 15 (seeded loop via fast-check) ─────────────────────────────────

// A ReplayForge job id is an opaque identifier. Generate arbitrary non-blank
// strings (the id space is not restricted to a known charset).
const jobIdArb = fc.string({ minLength: 1, maxLength: 64 }).filter((s) => s.trim().length > 0)

it('Property 15: studioPath round-trips the job id and never leaks a token', () => {
  fc.assert(
    fc.property(jobIdArb, (jobId) => {
      const path = studioPath(jobId)
      // Always rooted at the `/studio/` prefix with exactly one job segment.
      assert.ok(path.startsWith('/studio/'))
      const segment = path.slice('/studio/'.length)
      // The single dynamic segment decodes back to the trimmed job id — the
      // link is derived ONLY from the job id, nothing else.
      assert.equal(decodeURIComponent(segment), jobId.trim())
      assert.ok(!segment.includes('/'), 'job id must be a single encoded segment')
      assertTokenFree(path)
    }),
    { numRuns: 200 },
  )
})

it('Property 15: replayforgeStudioUrl resolves to base + studioPath, token-free', () => {
  const baseArb = fc.constantFrom(
    'http://localhost:8096',
    'http://localhost:8096/',
    'https://studio.example.test',
    'https://studio.example.test/',
  )
  fc.assert(
    fc.property(baseArb, jobIdArb, (base, jobId) => {
      const url = replayforgeStudioUrl(base, jobId)
      const expectedBase = base.replace(/\/+$/, '')
      assert.equal(url, `${expectedBase}${studioPath(jobId)}`)
      // The job id is recoverable from the final segment; no token in the URL.
      const segment = url.slice(url.indexOf('/studio/') + '/studio/'.length)
      assert.equal(decodeURIComponent(segment), jobId.trim())
      assertTokenFree(url)
    }),
    { numRuns: 200 },
  )
})

it('Property 15: a blank job id resolves to the archive root on both builders', () => {
  fc.assert(
    fc.property(fc.stringMatching(/^[ \t\n\r]*$/), (blank) => {
      assert.equal(studioPath(blank), '/studio')
      assert.equal(replayforgeStudioUrl('http://localhost:8096', blank), 'http://localhost:8096/studio')
    }),
    { numRuns: 50 },
  )
})
