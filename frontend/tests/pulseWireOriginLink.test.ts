import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import type { PulseWireStory } from '../src/pulseWireApi.ts'
import { buildPulseWireOriginHref } from '../src/utils/pulseWireOriginLink.ts'

function story(partial: Partial<PulseWireStory>): PulseWireStory {
  return {
    story: { id: 1, state: 'published', ...partial.story },
    entity: partial.entity,
    origin: partial.origin,
    scores: { trend: null, volatility: null, confidence: null, sentiment: null },
  }
}

describe('buildPulseWireOriginHref', () => {
  it('links Pulse Wire origin data to the VOD review moment when the payload includes a VOD id', () => {
    const href = buildPulseWireOriginHref(story({
      entity: { login: 'caseoh' },
      origin: { streamId: '316955094498', vodId: '2371095470', vodOffsetS: 420 },
    }))

    assert.equal(href, '/c/caseoh?vod=2371095470&offset=420&from=analytics&sid=316955094498')
  })

  it('does not invent a link without streamer login, stream id, and VOD id', () => {
    assert.equal(buildPulseWireOriginHref(story({
      entity: { login: 'caseoh' },
    })), undefined)
    assert.equal(buildPulseWireOriginHref(story({
      origin: { streamId: '316955094498', vodOffsetS: 420 },
    })), undefined)
    assert.equal(buildPulseWireOriginHref(story({
      entity: { login: 'caseoh' },
      origin: { streamId: '316955094498', vodOffsetS: 420 },
    })), undefined)
  })
})
