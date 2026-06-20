import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  formatChannelMatchReason,
  isSpreadBackfillWarming,
  matchConfidenceLabel,
  normalizeChannelSpreadResponse,
} from '../src/utils/pulseWireSpread.ts'
import type {
  PulseWireChannelSpreadResponse,
  PulseWireMatchExplanation,
} from '../src/pulseWireApi.ts'
import { pulseWireStoryFixture } from './fixtures/pulseWireFixtures.ts'

const spreadFixture: PulseWireChannelSpreadResponse = {
  login: 'xqc',
  items: [
    pulseWireStoryFixture({
      story: { id: 101, title: 'Confirmed spread story' },
      entity: { login: 'xqc', displayName: 'xQc' },
      matchExplanation: [
        {
          sourceType: 'reddit_thread',
          matchedBy: 'flair',
          confidence: 0.86,
          factors: ['Reddit flair', 'clip broadcaster'],
        },
      ],
    }),
  ],
  probableItems: [
    pulseWireStoryFixture({
      story: { id: 102, title: 'Probable title match' },
      matchTier: 'probable',
      matchExplanation: [
        { sourceType: 'reddit_thread', matchedBy: 'title', confidence: 0.42, factors: ['title mention'] },
      ],
    }),
  ],
  meta: {
    entityKnown: true,
    aliases: ['xQc', 'Felix'],
    lastIngestAt: '2026-06-19T12:00:00Z',
    unresolvedMentionCount: 2,
    backfill: { state: 'warming', requestedAt: '2026-06-19T12:01:00Z' },
  },
}

describe('channel spread API helpers', () => {
  it('normalizes missing spread arrays and meta', () => {
    const normalized = normalizeChannelSpreadResponse({
      login: 'caseoh',
      items: undefined as unknown as PulseWireChannelSpreadResponse['items'],
    })

    assert.deepEqual(normalized.items, [])
    assert.deepEqual(normalized.probableItems, [])
    assert.deepEqual(normalized.meta, {})
  })

  it('preserves spread meta and backfill state from fixture', () => {
    const normalized = normalizeChannelSpreadResponse(spreadFixture)

    assert.equal(normalized.login, 'xqc')
    assert.equal(normalized.items.length, 1)
    assert.equal(normalized.probableItems?.length, 1)
    assert.equal(normalized.meta?.entityKnown, true)
    assert.deepEqual(normalized.meta?.aliases, ['xQc', 'Felix'])
    assert.equal(normalized.meta?.unresolvedMentionCount, 2)
    assert.equal(normalized.meta?.backfill?.state, 'warming')
    assert.equal(normalized.meta?.backfill?.requestedAt, '2026-06-19T12:01:00Z')
    assert.equal(normalized.probableItems?.[0]?.matchTier, 'probable')
  })

  it('detects warming backfill state', () => {
    assert.equal(isSpreadBackfillWarming({ backfill: { state: 'warming' } }), true)
    assert.equal(isSpreadBackfillWarming({ backfill: { state: 'idle' } }), false)
    assert.equal(isSpreadBackfillWarming({ backfill: { state: 'ready' } }), false)
    assert.equal(isSpreadBackfillWarming(undefined), false)
  })
})

describe('formatChannelMatchReason', () => {
  it('joins match factors when present', () => {
    const match: PulseWireMatchExplanation = {
      sourceType: 'reddit_thread',
      matchedBy: 'flair',
      confidence: 0.9,
      factors: ['Reddit flair', 'clip broadcaster'],
    }
    assert.equal(formatChannelMatchReason(match), 'Matched: Reddit flair · clip broadcaster')
  })

  it('falls back to source type and matchedBy labels', () => {
    const match: PulseWireMatchExplanation = {
      sourceType: 'twitch_clip',
      matchedBy: 'broadcaster',
      confidence: 0.7,
    }
    assert.equal(formatChannelMatchReason(match), 'Matched: twitch clip · broadcaster')
  })
})

describe('matchConfidenceLabel', () => {
  it('maps confidence thresholds to labels', () => {
    assert.equal(matchConfidenceLabel(0.9), 'High')
    assert.equal(matchConfidenceLabel(0.65), 'Medium')
    assert.equal(matchConfidenceLabel(0.3), 'Low')
    assert.equal(matchConfidenceLabel(undefined), undefined)
  })
})
