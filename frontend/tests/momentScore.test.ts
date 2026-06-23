import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import { buildMomentScoreModel } from '@streamclone/pulse-core'
import type { ReplayHeatmapDetailPoint, ReplayHeatmapPoint } from '@streamclone/pulse-core'

const backendPoint: ReplayHeatmapPoint = {
  offsetSeconds: 120,
  durationSeconds: 60,
  score: 87,
  confidence: 0.91,
  reason: 'chat_spike',
  topEmotes: [{ id: 'lol', name: 'LOL', imageUrl: '', count: 42, provider: 'seventv' }],
  vodId: '123',
  streamId: 'stream-1',
  minuteTs: '2026-06-13T20:00:00Z',
}

describe('moment score model', () => {
  it('uses backend heatmap score, reason, confidence, and emotes when available', () => {
    const model = buildMomentScoreModel({
      heatmapPoint: backendPoint,
      fallbackScore100: 35,
      fallbackReason: 'viewer_spike',
      fallbackTopEmotes: [],
    })

    assert.equal(model.score, 87)
    assert.equal(model.label, '87/100')
    assert.equal(model.reason, 'chat_spike')
    assert.equal(model.reasonLabel, 'Chat spike')
    assert.equal(model.confidence, 0.91)
    assert.equal(model.estimated, false)
    assert.equal(model.topEmotes[0].name, 'LOL')
  })

  it('marks local fallback scores as approximate', () => {
    const model = buildMomentScoreModel({
      fallbackScore100: 64.4,
      fallbackReason: 'seventv_spike',
      fallbackTopEmotes: [{ id: 'lo', name: 'LO', imageUrl: '', count: 11, provider: 'seventv' }],
    })

    assert.equal(model.score, 64.4)
    assert.equal(model.label, '~64/100')
    assert.equal(model.reasonLabel, '7TV emote spike')
    assert.equal(model.estimated, true)
    assert.equal(model.topEmotes[0].name, 'LO')
  })

  it('uses detail components for the selected moment breakdown', () => {
    const detail: ReplayHeatmapDetailPoint = {
      ...backendPoint,
      score: 93,
      components: {
        chat_rate: { rawScore: 90, weightedScore: 45, confidence: 1 },
        viewer_momentum: { rawScore: 50, weightedScore: 12, confidence: 0.8 },
      },
    }
    const model = buildMomentScoreModel({
      heatmapPoint: backendPoint,
      heatmapDetail: detail,
      fallbackScore100: 10,
      fallbackReason: 'manual',
    })

    assert.equal(model.label, '93/100')
    assert.deepEqual(model.detailComponents.map(component => component.key), ['chat_rate', 'viewer_momentum'])
  })
})
