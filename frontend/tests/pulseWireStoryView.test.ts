import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import type { PulseWireSourceHealth, PulseWireStory } from '../src/pulseWireApi.ts'
import { isCrossPlatformStory, toWireStoryView } from '../src/utils/pulseWireStoryView.ts'
import {
  expectedWireStoryViews,
  pulseWireStoryFixtures,
  pulseWireStoryFixture,
  sourceHealthDegraded,
  sourceHealthLinkOnly,
  wireEmptyCorrob,
  wirePulseMatched,
  wireRedditYoutubeCorrelated,
  wireSettled,
  wireTwitchOnlyDeveloping,
  wireUnverified,
} from './fixtures/pulseWireFixtures.ts'

function story(partial: Partial<PulseWireStory>): PulseWireStory {
  return pulseWireStoryFixture(partial)
}

describe('toWireStoryView', () => {
  it('keeps named story fixtures aligned with expected WireStoryView summaries', () => {
    for (const [name, fixture] of Object.entries(pulseWireStoryFixtures)) {
      const expected = expectedWireStoryViews[name as keyof typeof expectedWireStoryViews]
      const view = toWireStoryView(fixture, undefined, { analystMode: true })

      assert.equal(view.readerStatus, expected.readerStatus, name)
      assert.equal(view.confidenceLabel, expected.confidenceLabel, name)
      assert.equal(view.entityLabel, expected.entityLabel, name)
      assert.equal(view.hasPulseOrigin, expected.hasPulseOrigin, name)
      assert.ok(Array.isArray(fixture.windowReceipts), `${name} has windowReceipts`)
      assert.ok(Array.isArray(fixture.windowTimeline), `${name} has windowTimeline`)
      assert.ok(Array.isArray(fixture.evidenceGallery), `${name} has evidenceGallery`)

      for (const [platform, state] of Object.entries(expected.platformStates)) {
        assert.equal(view.platformPresence[platform as keyof typeof view.platformPresence].state, state, `${name}:${platform}`)
      }
      for (const missing of expected.missingEvidence ?? []) {
        assert.ok(view.missingEvidence.includes(missing), `${name}:${missing}`)
      }
    }
  })

  it('marks a single-source published story as Developing instead of Breaking', () => {
    const view = toWireStoryView(story({
      windowReceipts: [{ sourceType: 'reddit_thread', pct: 78, label: 'LSF thread' }],
      windowTimeline: [],
      evidenceGallery: [],
      windowScores: { sourceCount: 1, evidenceCount: 1 },
    }))

    assert.equal(view.readerStatus, 'developing')
    assert.equal(view.confidenceLabel, 'Low')
    assert.equal(view.platformPresence.reddit.state, 'linked')
    assert.equal(view.missingEvidence.length, 0)
  })

  it('analyst mode keeps the full missing-evidence checklist for single-source stories', () => {
    const analyst = toWireStoryView(story({
      windowReceipts: [{ sourceType: 'reddit_thread', pct: 78, label: 'LSF thread' }],
      windowTimeline: [],
      evidenceGallery: [],
      windowScores: { sourceCount: 1, evidenceCount: 1 },
    }), undefined, { analystMode: true })
    assert.ok(analyst.missingEvidence.length > 0)
  })

  it('marks multi-source spread without Pulse or Twitch origin as Needs origin', () => {
    const view = toWireStoryView(wireRedditYoutubeCorrelated)
    const analyst = toWireStoryView(wireRedditYoutubeCorrelated, undefined, { analystMode: true })

    assert.equal(view.readerStatus, 'needs_origin')
    assert.equal(view.confidenceLabel, 'Medium')
    assert.deepEqual(view.missingEvidence.slice(0, 1), ['No Pulse or Twitch origin matched'])
    assert.ok(analyst.missingEvidence.length >= view.missingEvidence.length)
    assert.equal(view.platformPresence.youtube.state, 'linked')
  })

  it('separates pending origin from searched-and-missing origin state', () => {
    const view = toWireStoryView(story({
      ...wireRedditYoutubeCorrelated,
      story: {
        ...wireRedditYoutubeCorrelated.story,
        originSearchStatus: 'searched_missing',
        originCheckedAt: '2026-06-17T20:05:00Z',
      },
    }))

    assert.equal(view.readerStatus, 'needs_origin')
    assert.deepEqual(view.missingEvidence.slice(0, 1), ['No Twitch origin found'])
    assert.ok(view.displayReasonBullets.includes('Origin search ran; no Twitch origin was found.'))
  })

  it('classifies YouTube as context by default and amplification for repost or reaction evidence', () => {
    const contextView = toWireStoryView(story({
      windowReceipts: [
        { sourceType: 'youtube_video', pct: 55, label: 'YouTube explainer', url: 'https://youtube.com/watch?v=context123' },
      ],
      evidenceGallery: [],
      windowScores: { sourceCount: 1, evidenceCount: 1 },
    }))
    assert.equal(contextView.platformPresence.youtube.role, 'context')

    const repostView = toWireStoryView(story({
      windowReceipts: [
        { sourceType: 'youtube_video', pct: 70, label: 'YouTube Shorts repost', url: 'https://youtube.com/shorts/repost12345' },
      ],
      evidenceGallery: [],
      windowScores: { sourceCount: 1, evidenceCount: 1 },
    }))
    assert.equal(repostView.platformPresence.youtube.role, 'amplification')

    const previewOnlyView = toWireStoryView(story({
      windowReceipts: [],
      evidenceGallery: [
        {
          canonicalUrl: 'https://youtube.com/watch?v=react123456',
          platform: 'youtube',
          providerName: 'YouTube',
          title: 'Creator reacts to LSF clip',
          previewStatus: 'ready',
          matchKind: 'url',
        },
      ],
      windowScores: { sourceCount: 1, evidenceCount: 1 },
    }))
    assert.equal(previewOnlyView.platformPresence.youtube.role, 'amplification')
  })

  it('recognizes Pulse and Twitch origin evidence', () => {
    const view = toWireStoryView(story({
      ...wirePulseMatched,
      windowReceipts: [
        ...(wirePulseMatched.windowReceipts ?? []),
        { sourceType: 'twitch_clip', pct: 90, label: 'Twitch clip' },
      ],
    }))

    assert.equal(view.hasPulseOrigin, true)
    assert.equal(view.platformPresence.pulse.state, 'matched')
    assert.equal(view.platformPresence.twitch.state, 'matched')
    assert.equal(view.readerStatus, 'corroborated')
  })

  it('only enables clip creation for VOD-backed origin timestamps', () => {
    const matchedButNoVod = toWireStoryView(story({
      ...wirePulseMatched,
      origin: {
        streamId: '316955094498',
        vodOffsetS: 420,
        quotes: ['chat spike matched the story origin'],
      },
    }))

    assert.equal(matchedButNoVod.hasPulseOrigin, true)
    assert.equal(matchedButNoVod.canCreateClip, false)
    assert.equal(matchedButNoVod.platformPresence.pulse.state, 'matched')

    const clipReady = toWireStoryView(wirePulseMatched)

    assert.equal(clipReady.hasPulseOrigin, true)
    assert.equal(clipReady.canCreateClip, true)
  })

  it('treats a Twitch-only clip fixture as Developing origin evidence', () => {
    const view = toWireStoryView(wireTwitchOnlyDeveloping)

    assert.equal(view.readerStatus, 'developing')
    assert.equal(view.platformPresence.twitch.state, 'linked')
    assert.equal(view.platformPresence.twitch.role, 'origin')
    assert.equal(view.confidenceLabel, 'Low')
    assert.equal(view.hasPulseOrigin, false)
  })

  it('uses source health for disabled and degraded platform states', () => {
    const health: PulseWireSourceHealth = {
      ...sourceHealthDegraded,
      ...sourceHealthLinkOnly,
      youtube: { mode: 'error', last_error: 'scraper unhealthy' },
      x: { mode: 'off' },
    }
    const view = toWireStoryView(wireEmptyCorrob, health)

    assert.equal(view.platformPresence.reddit.state, 'degraded')
    assert.equal(view.platformPresence.youtube.state, 'degraded')
    assert.equal(view.platformPresence.x.state, 'disabled')
    assert.equal(view.platformPresence.tiktok.state, 'missing')
    assert.equal(view.readerStatus, 'insufficient_data')
  })

  it('shows X and TikTok source types as linked amplification evidence', () => {
    const view = toWireStoryView(story({
      scores: { trend: null, volatility: null, confidence: 'corroborated', sentiment: null },
      windowScores: { sourceCount: 2, evidenceCount: 2 },
      windowReceipts: [
        { sourceType: 'x_post', pct: 61, label: 'X post', url: 'https://x.com/creator/status/123' },
        { sourceType: 'tiktok_video', pct: 59, label: 'TikTok video', url: 'https://www.tiktok.com/@creator/video/999' },
      ],
    }))

    assert.equal(view.platformPresence.x.state, 'linked')
    assert.equal(view.platformPresence.x.role, 'amplification')
    assert.equal(view.platformPresence.tiktok.state, 'linked')
    assert.equal(view.platformPresence.tiktok.role, 'amplification')
    assert.ok(!view.missingEvidence.includes('No X link found'))
    assert.ok(!view.missingEvidence.includes('No TikTok link found'))
  })

  it('flags a missing official response only until manual or creator statement evidence exists', () => {
    const withoutOfficial = toWireStoryView(wireRedditYoutubeCorrelated)
    assert.ok(withoutOfficial.missingEvidence.includes('No official response found'))

    const withOfficial = toWireStoryView(story({
      ...wireRedditYoutubeCorrelated,
      windowReceipts: [
        ...(wireRedditYoutubeCorrelated.windowReceipts ?? []),
        { sourceType: 'manual_curation', pct: 55, label: 'Official statement', url: 'https://creator.example/statement' },
      ],
      evidenceGallery: [
        {
          canonicalUrl: 'https://creator.example/statement',
          platform: 'web',
          providerName: 'Creator site',
          title: 'Official statement',
          previewStatus: 'fallback',
          matchKind: 'manual_curation',
        },
      ],
    }))

    assert.ok(!withOfficial.missingEvidence.includes('No official response found'))

    const withCreatorPreview = toWireStoryView(story({
      ...wireRedditYoutubeCorrelated,
      evidenceGallery: [
        {
          canonicalUrl: 'https://creator.example/response',
          platform: 'web',
          providerName: 'Creator site',
          title: 'Response from creator',
          previewStatus: 'fallback',
          matchKind: 'manual_curation',
        },
      ],
    }))

    assert.ok(!withCreatorPreview.missingEvidence.includes('No official response found'))
  })

  it('preserves explicit backend state for an unverified story with no entity', () => {
    const view = toWireStoryView(wireUnverified)

    assert.equal(view.readerStatus, 'unverified')
    assert.equal(view.entityLabel, 'Unconfirmed streamer rumor')
    assert.equal(view.entitySublabel, 'Streamer not matched yet')
  })

  it('preserves explicit backend state for a settled story fixture', () => {
    const view = toWireStoryView(wireSettled)

    assert.equal(view.readerStatus, 'settled')
    assert.equal(view.confidenceLabel, 'High')
  })

  it('flags isCrossPlatformStory when windowScores sourceCount is at least two', () => {
    assert.equal(isCrossPlatformStory(wireRedditYoutubeCorrelated), true)
    assert.equal(isCrossPlatformStory(wireTwitchOnlyDeveloping), false)
  })
})
