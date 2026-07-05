import { describe, expect, it } from 'vitest'
import { gameSegmentPlotBounds } from '../src/gameSegmentChart.ts'
import { hasMeaningfulGameSegments, normalizeGameSegments } from '../src/gameSegments.ts'
import type { ChartGameSegment } from '../src/types.ts'

describe('gameSegmentPlotBounds', () => {
  const rollups = [
    { minuteTs: '2026-01-01T01:00:00.000Z' },
    { minuteTs: '2026-01-01T01:30:00.000Z' },
    { minuteTs: '2026-01-01T02:00:00.000Z' },
  ]

  it('maps segment onto visible rollup window', () => {
    const segment: ChartGameSegment = {
      gameName: 'Rocket League',
      offsetSeconds: 15 * 60,
      durationSeconds: 45 * 60,
    }
    const bounds = gameSegmentPlotBounds(segment, rollups, '2026-01-01T01:00:00.000Z', 90, 820)
    expect(bounds).not.toBeNull()
    expect(bounds!.startX).toBeGreaterThan(90)
    expect(bounds!.endX).toBeLessThanOrEqual(910)
  })
})

describe('normalizeGameSegments', () => {
  it('repairs zero-duration segments', () => {
    const games: ChartGameSegment[] = [
      { gameName: 'A', offsetSeconds: 0, durationSeconds: 0 },
      { gameName: 'B', offsetSeconds: 0, durationSeconds: 0 },
    ]
    const out = normalizeGameSegments(games, 3600)
    expect(out).toHaveLength(2)
    expect(out[0]!.durationSeconds).toBeGreaterThan(0)
  })
})

describe('hasMeaningfulGameSegments', () => {
  it('hides single full-stream segment', () => {
    const games: ChartGameSegment[] = [{ gameName: 'Just Chatting', offsetSeconds: 0, durationSeconds: 3600 }]
    expect(hasMeaningfulGameSegments(games, 3600)).toBe(false)
  })

  it('shows multi-segment streams', () => {
    const games: ChartGameSegment[] = [
      { gameName: 'A', offsetSeconds: 0, durationSeconds: 1800 },
      { gameName: 'B', offsetSeconds: 1800, durationSeconds: 1800 },
    ]
    expect(hasMeaningfulGameSegments(games, 3600)).toBe(true)
  })
})
