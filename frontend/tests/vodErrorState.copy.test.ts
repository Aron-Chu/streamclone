import assert from 'node:assert/strict'
import test from 'node:test'
import {
  describeVodError,
  type VodErrorActionKind,
} from '../src/components/channel/vodError.ts'

// Feature: moment-timeline
// Unit coverage for the VOD error code -> UI copy/action mapping in
// describeVodError. Covers all 6 backend error codes plus the client-detected
// HLS 401 proxy-auth guidance.
// Validates: Requirements 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.8

const ctx = { channelLogin: 'caedrel', vodId: '123456789' }

function actionKinds(actions: { kind: VodErrorActionKind }[]): VodErrorActionKind[] {
  return actions.map(a => a.kind)
}

test('2.1 invalid_vod_id: stale/invalid copy + open-on-Twitch, no retry', () => {
  const d = describeVodError({ code: 'invalid_vod_id' }, ctx)
  assert.equal(d.code, 'invalid_vod_id')
  assert.match(d.title, /invalid/i)
  assert.match(d.description, /invalid/i)
  assert.equal(d.retryable, false)
  const kinds = actionKinds(d.actions)
  assert.ok(kinds.includes('twitch'), 'offers open-on-Twitch')
  assert.ok(!kinds.includes('retry'), 'non-retryable: no retry action')
})

test('2.2 vod_unavailable: deleted/sub-only copy + open-on-Twitch, no retry', () => {
  const d = describeVodError({ code: 'vod_unavailable' }, ctx)
  assert.equal(d.code, 'vod_unavailable')
  assert.match(d.title, /unavailable/i)
  assert.match(d.description, /deleted/i)
  assert.match(d.description, /subscriber-only/i)
  assert.equal(d.retryable, false)
  const kinds = actionKinds(d.actions)
  assert.ok(kinds.includes('twitch'), 'offers open-on-Twitch')
  assert.ok(!kinds.includes('retry'), 'non-retryable: no retry action')
  const twitch = d.actions.find(a => a.kind === 'twitch')
  assert.ok(twitch?.href?.includes('twitch.tv'), 'twitch action links to Twitch')
  assert.equal(twitch?.external, true)
})

test('34.3 vod_unavailable from analytics deep link: honest copy + open-on-Twitch only', () => {
  const analyticsCtx = {
    ...ctx,
    fromAnalytics: true,
  }
  const d = describeVodError({ code: 'vod_unavailable' }, analyticsCtx)
  assert.match(d.title, /won.t play in Streamclone/i)
  assert.match(d.description, /stale/i)
  assert.doesNotMatch(d.description, /^this vod is deleted from twitch/i)
  const kinds = actionKinds(d.actions)
  assert.ok(kinds.includes('twitch'))
  assert.ok(!kinds.includes('retry'))
})

test('2.3 upstream_token_failed: auth-issue copy, retry follows flag', () => {
  const base = describeVodError({ code: 'upstream_token_failed' }, ctx)
  assert.equal(base.code, 'upstream_token_failed')
  assert.match(base.title, /authentication/i)
  assert.match(base.description, /token/i)
  // Default (no retryable flag): no retry action.
  assert.equal(base.retryable, false)
  assert.ok(!actionKinds(base.actions).includes('retry'))

  // With retryable flag from the API: retry action appears.
  const retryable = describeVodError({ code: 'upstream_token_failed', retryable: true }, ctx)
  assert.equal(retryable.retryable, true)
  assert.ok(actionKinds(retryable.actions).includes('retry'))
})

test('2.4 capacity_reached: retry copy + retry action, always retryable', () => {
  const d = describeVodError({ code: 'capacity_reached' }, ctx)
  assert.equal(d.code, 'capacity_reached')
  assert.match(d.title, /capacity/i)
  assert.equal(d.retryable, true)
  const kinds = actionKinds(d.actions)
  assert.ok(kinds.includes('retry'), 'offers retry action')
})

test('2.5 hls_not_ready: relay-warming copy + retry action, always retryable', () => {
  const d = describeVodError({ code: 'hls_not_ready' }, ctx)
  assert.equal(d.code, 'hls_not_ready')
  assert.match(d.description, /publish/i)
  assert.equal(d.retryable, true)
  assert.ok(actionKinds(d.actions).includes('retry'), 'offers retry action')
})

test('2.6 vod_start_failed: generic copy includes server detail + retry', () => {
  const withDetail = describeVodError(
    { code: 'vod_start_failed', message: 'streamlink exited 1', retryable: true },
    ctx,
  )
  assert.equal(withDetail.code, 'vod_start_failed')
  assert.match(withDetail.description, /streamlink exited 1/, 'surfaces server-provided detail')
  assert.equal(withDetail.retryable, true)
  assert.ok(actionKinds(withDetail.actions).includes('retry'), 'offers retry action')

  // No detail provided: still a generic VOD playback failure message.
  const noDetail = describeVodError({ code: 'vod_start_failed' }, ctx)
  assert.match(noDetail.title, /failed/i)
  assert.match(noDetail.description, /failed/i)
})

test('2.8 hls_proxy_auth: proxy-auth guidance, NOT "removed from Twitch" blame', () => {
  const d = describeVodError({ code: 'hls_proxy_auth' }, ctx)
  assert.equal(d.code, 'hls_proxy_auth')
  assert.match(d.description, /hlsCDNSecret/i, 'mentions MediaMTX hlsCDNSecret')
  assert.match(d.description, /Caddy/i, 'mentions Caddy Bearer mismatch')
  assert.match(d.description, /401/, 'references the repeated 401 condition')
  // Must not attribute the failure to Twitch VOD removal (Req 2.8).
  assert.doesNotMatch(
    d.description.replace(/rather than assuming the VOD was removed from Twitch\.?/i, ''),
    /removed from Twitch|deleted from Twitch/i,
    'does not blame Twitch VOD removal',
  )
  assert.equal(d.retryable, true)
  const kinds = actionKinds(d.actions)
  assert.ok(kinds.includes('retry'), 'offers retry action')
  assert.ok(kinds.includes('hard-refresh'), 'offers hard-refresh guidance')
})

test('unknown code falls back to generic failure copy', () => {
  const d = describeVodError({ code: 'something_unexpected' }, ctx)
  assert.equal(d.code, 'unknown')
  assert.match(d.title, /failed/i)
})
