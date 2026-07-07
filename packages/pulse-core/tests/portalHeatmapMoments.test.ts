import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  computeStreamBaselines,
  detectPickReason as detectPickReasonFromScoring,
} from '../src/momentScoring.ts'
import {
  findRollupForMomentCandidate,
  heatmapPointsToMomentCandidates,
  honestMomentReasonLabel,
  normalizeMinuteBucket,
  rollupFallbackMomentCandidates,
} from '../src/portalHeatmapMoments.ts'

const STARTED_AT = '2026-07-04T12:00:00.000Z'

describe('normalizeMinuteBucket', () => {
  it('maps ISO timestamps to the same minute bucket', () => {
    assert.equal(
      normalizeMinuteBucket('2026-07-04T12:05:00.000Z'),
      normalizeMinuteBucket('2026-07-04T12:05:59.999Z'),
    )
  })
})

describe('heatmapPointsToMomentCandidates', () => {
  it('ranks backend heatmap scores desc with emote identity', () => {
    const candidates = heatmapPointsToMomentCandidates(
      [
        {
          offsetSeconds: 300,
          durationSeconds: 60,
          score: 27,
          confidence: 0.8,
          reason: 'twitch_emote_spike',
          topEmotes: [{
            id: '123',
            name: 'PJSugar',
            imageUrl: 'https://static-cdn.jtvnw.net/emoticons/v2/123/default/dark/2.0',
            count: 12,
            provider: 'twitch',
          }],
          vodId: null,
          streamId: 's1',
          minuteTs: '2026-07-04T12:05:00.000Z',
        },
        {
          offsetSeconds: 1920,
          durationSeconds: 60,
          score: 80,
          confidence: 0.5,
          reason: 'viewer_spike',
          topEmotes: [],
          vodId: null,
          streamId: 's1',
          minuteTs: '2026-07-04T12:32:00.000Z',
        },
      ],
      STARTED_AT,
    )

    assert.equal(candidates.length, 2)
    assert.equal(candidates[0]?.score, 80)
    assert.equal(candidates[0]?.estimated, false)
    assert.equal(candidates[0]?.scoreLabel, '80/100')
    assert.equal(candidates[1]?.score, 27)
    assert.equal(candidates[1]?.topEmote?.name, 'PJSugar')
    assert.equal(candidates[1]?.scoreLabel, '27/100')
    assert.equal(/~/.test(candidates[1]?.scoreLabel ?? ''), false)
  })

  it('never labels emote-family reason without topEmote', () => {
    const label = honestMomentReasonLabel('seventv_spike', undefined, {
      chatCount: 40,
      minuteTs: '2026-07-04T12:05:00.000Z',
    })
    assert.equal(label, 'Chat spike')
  })
})

describe('rollupFallbackMomentCandidates', () => {
  it('does not emit generic emote spike without per-emote breakdown', () => {
    const rollups = [
      {
        minuteTs: '2026-07-04T12:32:00.000Z',
        chatCount: 10,
        totalEmoteCount: 200,
        emotes: {},
        viewerLatest: 1000,
        viewerAvg: 1000,
        viewerMax: 1000,
        viewerSamples: 1,
        seventvEmoteCount: 200,
      },
      {
        minuteTs: '2026-07-04T12:05:00.000Z',
        chatCount: 50,
        totalEmoteCount: 12,
        emotes: { 'twitch:PJSugar:PJSugar': 12 },
        viewerLatest: 800,
        viewerAvg: 800,
        viewerMax: 800,
        viewerSamples: 1,
        seventvEmoteCount: 0,
      },
    ]
    const baselines = computeStreamBaselines(rollups)
    const reason = detectPickReasonFromScoring(rollups[0]!, baselines)
    assert.notEqual(reason, 'emote_spike')

    const candidates = rollupFallbackMomentCandidates(rollups, undefined, STARTED_AT)
    for (const candidate of candidates) {
      if (candidate.reasonLabel.toLowerCase().includes('emote spike')) {
        assert.ok(candidate.topEmote?.name)
      }
    }
  })
})

describe('findRollupForMomentCandidate', () => {
  it('joins heatmap candidate to rollup by minute bucket', () => {
    const rollup = {
      minuteTs: '2026-07-04T12:05:00.000Z',
      chatCount: 12,
      totalEmoteCount: 12,
      emotes: {},
      viewerLatest: 0,
      viewerAvg: 0,
      viewerMax: 0,
      viewerSamples: 0,
      seventvEmoteCount: 0,
    }
    const candidate = heatmapPointsToMomentCandidates(
      [{
        offsetSeconds: 300,
        durationSeconds: 60,
        score: 27,
        confidence: 0.8,
        reason: 'twitch_emote_spike',
        topEmotes: [{
          id: '123',
          name: 'PJSugar',
          imageUrl: 'https://example.test/pj.png',
          count: 12,
          provider: 'twitch',
        }],
        vodId: null,
        streamId: 's1',
        minuteTs: '2026-07-04T12:05:00Z',
      }],
      STARTED_AT,
    )[0]!
    const joined = findRollupForMomentCandidate([rollup], candidate)
    assert.equal(joined?.minuteTs, rollup.minuteTs)
  })
})
