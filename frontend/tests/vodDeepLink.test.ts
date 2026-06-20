import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildAnalyticsMomentLink,
  buildMomentJumpLink,
  buildVodDeepLink,
  buildVodSeekTarget,
  buildVodStartRequestBody,
  parseVodAnalyticsContext,
  preferTwitchEmbedReview,
} from '../src/utils/vodDeepLink.ts'

// Feature: moment-timeline, Task 3.3: VOD deep link smoke test
// **Validates: Requirements 25.1, 25.2, 25.3, 25.4**
//
// ── Why this is a LOGIC-LEVEL smoke test, not a full render-and-click test ──
//
// Requirement 25 describes rendering an analytics moment, clicking "Play in
// Streamclone", and asserting on the resulting navigation + relay call + HLS
// seek. A faithful render-and-click test needs jsdom + React Testing Library to
// mount Analytics.tsx and Channel.tsx.
//
// This repo's frontend tests run via `node --experimental-strip-types --test`
// (see frontend/tests/README.md). That runner:
//   1. CANNOT load `.tsx` files (no JSX transform), so the React components
//      cannot be mounted; and
//   2. CANNOT load the modules behind the real relay call — `api.ts` imports
//      `config.ts`, which reads Vite's `import.meta.env.*`. Outside the Vite
//      build, `import.meta.env` is undefined and evaluating config.ts throws.
//
// So instead of a DOM render, this smoke test exercises the SAME pure helpers
// that the production surfaces are wired to:
//   • Analytics "Play in Streamclone" Link  -> buildVodDeepLink()
//   • api.ts startVodPlayback() request body -> buildVodStartRequestBody()
//   • Channel.tsx VOD start seek             -> buildVodSeekTarget()
// and reproduces the Channel.tsx start() control flow in `simulateVodStart`
// below (mirroring the throw-on-non-200 behaviour of api.ts `json()`), driven
// by a mocked `POST /v1/stream/vod/start`. This guards the trust-critical deep
// link contract end-to-end at the logic level. A full Playwright/RTL render
// test is tracked separately and is out of scope for the node runner.

interface MockRelayResponse {
  status: number
  body?: {
    hlsUrl?: string
    session_id?: string
    listeners?: number
    vod_id?: string
    offset_seconds?: number
    seek_seconds?: number
  }
}

interface VodStartOutcome {
  requestBody: ReturnType<typeof buildVodStartRequestBody>
  hlsUrl: string | null
  seekTarget: number | null
  error: string | null
}

// Mirrors the relevant part of Channel.tsx start() + api.ts startVodPlayback():
//  - shape the request body via the production helper,
//  - the mocked relay returns {status, body}; non-200 throws (as api.ts json()
//    does), so the player records an error and never loads an hlsUrl,
//  - on 200, compute the seek target and load the returned hlsUrl.
function simulateVodStart(
  vodId: string,
  offsetSeconds: number,
  mockRelay: (body: ReturnType<typeof buildVodStartRequestBody>) => MockRelayResponse,
): VodStartOutcome {
  const requestBody = buildVodStartRequestBody(vodId, offsetSeconds, 'best', 'stable')
  const res = mockRelay(requestBody)

  if (res.status !== 200 || !res.body || !res.body.hlsUrl) {
    // api.ts json() throws ApiError on non-200; Channel.tsx catch sets
    // relayState='error' and leaves hlsUrl empty (Requirement 25.4).
    return {
      requestBody,
      hlsUrl: null,
      seekTarget: null,
      error: `vod start failed (status ${res.status})`,
    }
  }

  const seekTarget = buildVodSeekTarget(
    res.body.offset_seconds ?? offsetSeconds,
    res.body.seek_seconds ?? 0,
  )
  return { requestBody, hlsUrl: res.body.hlsUrl, seekTarget, error: null }
}

// A known analytics moment (Requirement 25.1).
const MOMENT = {
  channel: 'caedrel',
  vodId: '2371095470',
  offsetSeconds: 3725, // 01:02:05 into the stream
}

test('25.2: Play in Streamclone builds /c/{login}?vod=&offset= deep link', () => {
  const url = buildVodDeepLink(MOMENT.channel, MOMENT.vodId, MOMENT.offsetSeconds)
  assert.equal(url, `/c/caedrel?vod=2371095470&offset=3725`)

  // The deep link carries the source moment's vod and offset verbatim.
  const parsed = new URL(url, 'http://localhost:8090')
  assert.equal(parsed.pathname, '/c/caedrel')
  assert.equal(parsed.searchParams.get('vod'), MOMENT.vodId)
  assert.equal(parsed.searchParams.get('offset'), String(MOMENT.offsetSeconds))
})

test('25.2: deep link URL-encodes the channel login', () => {
  const url = buildVodDeepLink('My Channel', MOMENT.vodId, 10)
  assert.match(url, /^\/c\/My%20Channel\?vod=2371095470&offset=10$/)
})

test('25.3: relay request body uses snake_case vod_id and offset_seconds', () => {
  const outcome = simulateVodStart(MOMENT.vodId, MOMENT.offsetSeconds, () => ({
    status: 200,
    body: {
      hlsUrl: 'http://localhost:8090/live/vod_2371095470/index.m3u8',
      session_id: 'sess-1',
      vod_id: MOMENT.vodId,
      offset_seconds: MOMENT.offsetSeconds,
      seek_seconds: 12,
    },
  }))

  // Request body shape: vod_id (NOT vodId), non-empty, matching the moment.
  assert.equal(outcome.requestBody.vod_id, MOMENT.vodId)
  assert.ok(outcome.requestBody.vod_id.length > 0)
  assert.equal(outcome.requestBody.offset_seconds, MOMENT.offsetSeconds)
  assert.ok(!('vodId' in outcome.requestBody))
  // Serialized wire payload must carry snake_case keys.
  const wire = JSON.parse(JSON.stringify(outcome.requestBody))
  assert.ok(Object.prototype.hasOwnProperty.call(wire, 'vod_id'))
  assert.ok(Object.prototype.hasOwnProperty.call(wire, 'offset_seconds'))
})

test('25.3: on 200 the player loads hlsUrl and seeks to max(0, offset - seek)', () => {
  const seekSeconds = 12
  const outcome = simulateVodStart(MOMENT.vodId, MOMENT.offsetSeconds, () => ({
    status: 200,
    body: {
      hlsUrl: 'http://localhost:8090/live/vod_2371095470/index.m3u8',
      session_id: 'sess-1',
      vod_id: MOMENT.vodId,
      offset_seconds: MOMENT.offsetSeconds,
      seek_seconds: seekSeconds,
    },
  }))

  assert.equal(outcome.error, null)
  assert.equal(outcome.hlsUrl, 'http://localhost:8090/live/vod_2371095470/index.m3u8')
  assert.equal(outcome.seekTarget, Math.max(0, MOMENT.offsetSeconds - seekSeconds))
  assert.equal(outcome.seekTarget, 3713)
})

test('25.3: seek target clamps to 0 when seek_seconds exceeds offset', () => {
  // Relay preroll larger than the requested offset must not seek negative.
  assert.equal(buildVodSeekTarget(5, 30), 0)
  assert.equal(buildVodSeekTarget(0, 0), 0)
  assert.equal(buildVodSeekTarget(100, 12), 88)
})

test('25.4: non-200 relay response surfaces an error and does NOT start HLS', () => {
  for (const status of [400, 404, 502, 503, 504]) {
    const outcome = simulateVodStart(MOMENT.vodId, MOMENT.offsetSeconds, () => ({
      status,
      body: { hlsUrl: '' }, // server would not return a usable manifest
    }))
    assert.equal(outcome.hlsUrl, null, `status ${status} must not load HLS`)
    assert.equal(outcome.seekTarget, null, `status ${status} must not seek`)
    assert.ok(outcome.error, `status ${status} must surface an error`)
    // The request was still well-formed (snake_case) before failing.
    assert.equal(outcome.requestBody.vod_id, MOMENT.vodId)
  }
})

test('25.4: a 200 with an empty hlsUrl is treated as a failure (no HLS)', () => {
  const outcome = simulateVodStart(MOMENT.vodId, MOMENT.offsetSeconds, () => ({
    status: 200,
    body: { hlsUrl: '', session_id: 'sess-1' },
  }))
  assert.equal(outcome.hlsUrl, null)
  assert.ok(outcome.error)
})

test('20.5/34.3: parseVodAnalyticsContext resolves sid and from=analytics deep links', () => {
  const withSid = parseVodAnalyticsContext(
    new URLSearchParams('vod=123&offset=0&from=analytics&sid=316955094498'),
    'caedrel',
    true,
  )
  assert.equal(withSid.fromAnalytics, true)
  assert.equal(withSid.streamId, '316955094498')
  assert.equal(withSid.analyticsHref, '/analytics/caedrel/316955094498')

  const fromOnly = parseVodAnalyticsContext(
    new URLSearchParams('vod=123&from=analytics'),
    'caedrel',
    true,
  )
  assert.equal(fromOnly.fromAnalytics, true)
  assert.equal(fromOnly.streamId, '')
  assert.equal(fromOnly.analyticsHref, '/analytics/caedrel')

  const noContext = parseVodAnalyticsContext(new URLSearchParams('vod=123'), 'caedrel', true)
  assert.equal(noContext.fromAnalytics, false)
  assert.equal(noContext.analyticsHref, null)
})

test('analytics VOD review prefers Twitch embed when sid is present', () => {
  assert.equal(preferTwitchEmbedReview(true, true, '316955094498'), true)
  assert.equal(preferTwitchEmbedReview(true, true, ''), false)
  assert.equal(preferTwitchEmbedReview(true, false, '316955094498'), false)
  assert.equal(preferTwitchEmbedReview(false, true, '316955094498'), false)
})

test('buildAnalyticsMomentLink routes to stream analytics with optional offset', () => {
  assert.equal(
    buildAnalyticsMomentLink('xqc', 3725, '316955094498'),
    '/analytics/xqc/316955094498?offset=3725',
  )
  assert.equal(buildAnalyticsMomentLink('xqc', 0), '/analytics/xqc')
})

test('buildMomentJumpLink prefers VOD playback when vod id is known', () => {
  assert.equal(
    buildMomentJumpLink('xqc', 120, { vodId: '2371095470', analyticsStreamId: '316955094498' }),
    '/c/xqc?vod=2371095470&offset=120&from=analytics&sid=316955094498',
  )
  assert.equal(
    buildMomentJumpLink('xqc', 120, { analyticsStreamId: '316955094498' }),
    '/analytics/xqc/316955094498?offset=120',
  )
})
