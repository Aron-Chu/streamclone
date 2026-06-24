import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import { deriveLiveHeat, type LiveHeatInput, type LiveHeatRollup } from '../src/liveHeat.ts'
import { toLiveHeatInputFromExtension, type ExtensionPulseLike } from '../src/extensionAdapters.ts'

const STARTED_AT = '2026-06-11T12:00:00.000Z'
const MINUTE_MS = 60_000

function webRollups(count: number, shape: (i: number) => Partial<LiveHeatRollup> = () => ({})): LiveHeatRollup[] {
  const base = Date.parse(STARTED_AT)
  return Array.from({ length: count }, (_, i) => ({
    minuteTs: new Date(base + i * MINUTE_MS).toISOString(),
    viewerSamples: 1,
    chatCount: 10 + i,
    totalEmoteCount: 4 + i,
    seventvEmoteCount: 2,
    emotes: {},
    ...shape(i),
  }))
}

function extensionPayloadFromWebRollups(rollups: LiveHeatRollup[], isLive = true): ExtensionPulseLike {
  const startedMs = Date.parse(STARTED_AT)
  return {
    isLive,
    startedAt: STARTED_AT,
    rollups: rollups.map(r => ({
      offsetSeconds: Math.round((Date.parse(r.minuteTs!) - startedMs) / 1000),
      chatCount: r.chatCount,
      totalEmoteCount: r.totalEmoteCount,
      sevenTvEmoteCount: r.seventvEmoteCount,
      viewerCount: (r.viewerSamples ?? 0) > 0 ? 100 : 0,
    })),
  }
}

describe('deriveLiveHeat parity: web input vs extension adapter', () => {
  it('matches visibility and completed rollup count for live fixture', () => {
    const rollups = webRollups(8, i => ({
      chatCount: i === 5 ? 400 : 10 + i,
      totalEmoteCount: i === 5 ? 120 : 4,
    }))
    const webInput: LiveHeatInput = { state: 'live', rollups, streamStartedAt: STARTED_AT }
    const extInput = toLiveHeatInputFromExtension(extensionPayloadFromWebRollups(rollups))

    const webHeat = deriveLiveHeat(webInput)
    const extHeat = deriveLiveHeat(extInput)

    assert.equal(extHeat.visible, webHeat.visible)
    assert.equal(extHeat.completedRollupCount, webHeat.completedRollupCount)
    assert.equal(extHeat.points.length, webHeat.points.length)

    for (let i = 0; i < webHeat.points.length; i++) {
      assert.equal(extHeat.points[i].offsetSeconds, webHeat.points[i].offsetSeconds)
      assert.equal(extHeat.points[i].score, webHeat.points[i].score)
      assert.equal(extHeat.points[i].estimated, webHeat.points[i].estimated)
    }
  })

  it('matches collecting point offset for live trailing minute', () => {
    const rollups = webRollups(6)
    const webHeat = deriveLiveHeat({ state: 'live', rollups, streamStartedAt: STARTED_AT })
    const extHeat = deriveLiveHeat(toLiveHeatInputFromExtension(extensionPayloadFromWebRollups(rollups)))

    assert.equal(extHeat.collectingPoint?.offsetSeconds, webHeat.collectingPoint?.offsetSeconds)
    assert.equal(extHeat.collectingPoint?.collecting, true)
  })
})
