import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildVodDeepLink,
  buildVodSeekTarget,
  buildVodStartRequestBody,
  parseVodAnalyticsContext,
  preferTwitchEmbedReview,
} from '../src/utils/vodLink.ts'

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

function simulateVodStart(
  vodId: string,
  offsetSeconds: number,
  mockRelay: (body: ReturnType<typeof buildVodStartRequestBody>) => MockRelayResponse,
): VodStartOutcome {
  const requestBody = buildVodStartRequestBody(vodId, offsetSeconds, 'best', 'stable')
  const res = mockRelay(requestBody)

  if (res.status !== 200 || !res.body || !res.body.hlsUrl) {
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

const MOMENT = {
  channel: 'caedrel',
  vodId: '2371095470',
  offsetSeconds: 3725,
}

test('25.2: Play in Streamclone builds /c/{login}?vod=&offset= deep link', () => {
  const url = buildVodDeepLink(MOMENT.channel, MOMENT.vodId, MOMENT.offsetSeconds)
  assert.equal(url, `/c/caedrel?vod=2371095470&offset=3725`)

  const parsed = new URL(url, 'http://localhost:8090')
  assert.equal(parsed.pathname, '/c/caedrel')
  assert.equal(parsed.searchParams.get('vod'), MOMENT.vodId)
  assert.equal(parsed.searchParams.get('offset'), String(MOMENT.offsetSeconds))
})

test('25.2: deep link URL-encodes the channel login', () => {
  const url = buildVodDeepLink('My Channel', MOMENT.vodId, 10)
  assert.match(url, /^\/c\/My%20Channel\?vod=2371095470&offset=10$/)
})

test('25.2: optional stream id adds analytics sid/from markers', () => {
  const url = buildVodDeepLink(MOMENT.channel, MOMENT.vodId, 0, '316955094498')
  const parsed = new URL(url, 'http://localhost:8090')
  assert.equal(parsed.searchParams.get('sid'), '316955094498')
  assert.equal(parsed.searchParams.get('from'), 'analytics')
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

  assert.equal(outcome.requestBody.vod_id, MOMENT.vodId)
  assert.ok(outcome.requestBody.vod_id.length > 0)
  assert.equal(outcome.requestBody.offset_seconds, MOMENT.offsetSeconds)
  assert.ok(!('vodId' in outcome.requestBody))
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
  assert.equal(buildVodSeekTarget(5, 30), 0)
  assert.equal(buildVodSeekTarget(0, 0), 0)
  assert.equal(buildVodSeekTarget(100, 12), 88)
})

test('25.4: non-200 relay response surfaces an error and does NOT start HLS', () => {
  for (const status of [400, 404, 502, 503, 504]) {
    const outcome = simulateVodStart(MOMENT.vodId, MOMENT.offsetSeconds, () => ({
      status,
      body: { hlsUrl: '' },
    }))
    assert.equal(outcome.hlsUrl, null, `status ${status} must not load HLS`)
    assert.equal(outcome.seekTarget, null, `status ${status} must not seek`)
    assert.ok(outcome.error, `status ${status} must surface an error`)
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

test('20.5/34.3: parseVodAnalyticsContext resolves sid and from=analytics markers', () => {
  const withSid = parseVodAnalyticsContext(
    new URLSearchParams('vod=123&offset=0&from=analytics&sid=316955094498'),
    'caedrel',
    true,
  )
  assert.equal(withSid.fromAnalytics, true)
  assert.equal(withSid.streamId, '316955094498')

  const fromOnly = parseVodAnalyticsContext(
    new URLSearchParams('vod=123&from=analytics'),
    'caedrel',
    true,
  )
  assert.equal(fromOnly.fromAnalytics, true)
  assert.equal(fromOnly.streamId, '')

  const noContext = parseVodAnalyticsContext(new URLSearchParams('vod=123'), 'caedrel', true)
  assert.equal(noContext.fromAnalytics, false)
  assert.equal(noContext.streamId, '')
})

test('analytics VOD review prefers Twitch embed when sid is present', () => {
  assert.equal(preferTwitchEmbedReview(true, true, '316955094498'), true)
  assert.equal(preferTwitchEmbedReview(true, true, ''), false)
  assert.equal(preferTwitchEmbedReview(true, false, '316955094498'), false)
  assert.equal(preferTwitchEmbedReview(false, true, '316955094498'), false)
})
