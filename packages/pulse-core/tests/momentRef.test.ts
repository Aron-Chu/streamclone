import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  bookmarkToMomentRef,
  buildMomentRef,
  isValidMomentRef,
  momentRefToPortalChannelPath,
  momentRefToPortalStreamPath,
  momentRefToTwitchVodUrl,
  normalizeCreateBookmarkInput,
} from '../src/momentRef.ts'

describe('momentRef', () => {
  it('builds a valid ref with normalized login', () => {
    const ref = buildMomentRef({
      login: ' XQC ',
      streamId: '12345',
      offsetSeconds: 90,
      vodId: '987',
      label: 'Peak',
      source: 'web',
    })
    assert.ok(ref)
    assert.equal(ref.login, 'xqc')
    assert.equal(ref.streamId, '12345')
    assert.equal(ref.offsetSeconds, 90)
    assert.equal(ref.vodId, '987')
  })

  it('rejects invalid login, stream id, or offset', () => {
    assert.equal(buildMomentRef({ login: '', streamId: '1', offsetSeconds: 0 }), null)
    assert.equal(buildMomentRef({ login: 'bad login', streamId: '1', offsetSeconds: 0 }), null)
    assert.equal(buildMomentRef({ login: 'xqc', streamId: '', offsetSeconds: 0 }), null)
    assert.equal(buildMomentRef({ login: 'xqc', streamId: '1', offsetSeconds: -1 }), null)
    assert.equal(buildMomentRef({ login: 'xqc', streamId: '1', offsetSeconds: Number.NaN }), null)
  })

  it('bookmarkToMomentRef mirrors bookmark fields', () => {
    const ref = bookmarkToMomentRef({
      login: 'shroud',
      streamId: 'abc',
      offsetSeconds: 42,
      vodId: 'vod1',
      source: 'extension',
      score: 88,
    })
    assert.ok(ref)
    assert.equal(ref?.source, 'extension')
    assert.equal(ref?.score, 88)
  })

  it('isValidMomentRef guards shape', () => {
    const ref = buildMomentRef({ login: 'xqc', streamId: '1', offsetSeconds: 0 })
    assert.ok(ref)
    assert.equal(isValidMomentRef(ref), true)
    assert.equal(isValidMomentRef({ login: 'xqc', streamId: '', offsetSeconds: 0 }), false)
  })

  it('portal channel path encodes login', () => {
    const ref = buildMomentRef({ login: 'xqc', streamId: '1', offsetSeconds: 0 })
    assert.ok(ref)
    assert.equal(momentRefToPortalChannelPath(ref), '/analytics/xqc')
  })

  it('portal stream path includes offset query when present', () => {
    const ref = buildMomentRef({ login: 'xqc', streamId: 'stream-1', offsetSeconds: 120 })
    assert.ok(ref)
    assert.equal(
      momentRefToPortalStreamPath(ref),
      '/analytics/xqc/s/stream-1?t=120',
    )
  })

  it('withholds stream path when route unavailable', () => {
    const ref = buildMomentRef({ login: 'xqc', streamId: 'stream-1', offsetSeconds: 0 })
    assert.ok(ref)
    assert.equal(
      momentRefToPortalStreamPath(ref, { streamAnalyticsAvailable: false }),
      null,
    )
  })

  it('twitch vod url only when vodId present', () => {
    const withVod = buildMomentRef({
      login: 'xqc',
      streamId: '1',
      offsetSeconds: 30,
      vodId: '123456789',
    })
    const withoutVod = buildMomentRef({ login: 'xqc', streamId: '1', offsetSeconds: 30 })
    assert.ok(withVod && withoutVod)
    assert.equal(
      momentRefToTwitchVodUrl(withVod),
      'https://www.twitch.tv/videos/123456789?t=30s',
    )
    assert.equal(momentRefToTwitchVodUrl(withoutVod), null)
  })

  it('normalizeCreateBookmarkInput rejects estimated moments', () => {
    const result = normalizeCreateBookmarkInput({
      login: 'xqc',
      streamId: '1',
      offsetSeconds: 10,
      source: 'extension',
      estimated: true,
    })
    assert.equal(result.ok, false)
    if (!result.ok) assert.equal(result.code, 'estimated_moment')
  })

  it('normalizeCreateBookmarkInput strips web score and keeps extension score', () => {
    const web = normalizeCreateBookmarkInput({
      login: 'xqc',
      streamId: '1',
      offsetSeconds: 5,
      source: 'web',
      score: 99,
    })
    assert.equal(web.ok, true)
    if (web.ok) assert.equal(web.value.score, undefined)

    const ext = normalizeCreateBookmarkInput({
      login: 'xqc',
      streamId: '1',
      offsetSeconds: 5,
      source: 'extension',
      score: 77,
    })
    assert.equal(ext.ok, true)
    if (ext.ok) assert.equal(ext.value.score, 77)
  })
})
