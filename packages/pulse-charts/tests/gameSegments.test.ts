import { describe, expect, it } from 'vitest'
import { gameSegmentPlotBounds } from '../src/gameSegmentChart.ts'
import { hasMeaningfulGameSegments, normalizeGameSegments } from '../src/gameSegments.ts'
import type { ChartGameSegment } from '../src/types.ts'

describe('gameSegmentPlotBounds', () => {
  it('maps segment onto visible rollup window', () => {
    const rollups = [
      { minuteTs: '2026-01-01T01:00:00.000Z' },
      { minuteTs: '2026-01-01T01:30:00.000Z' },
      { minuteTs: '2026-01-01T02:00:00.000Z' },
    ]
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

  it('maps multiple segments onto partial rollup window (late coverage start)', () => {
    const streamStartedAt = '2026-01-01T00:00:00.000Z'
    const rollups = [
      { minuteTs: '2026-01-01T00:11:00.000Z' },
      { minuteTs: '2026-01-01T00:20:00.000Z' },
      { minuteTs: '2026-01-01T00:30:00.000Z' },
      { minuteTs: '2026-01-01T00:40:00.000Z' },
      { minuteTs: '2026-01-01T00:50:00.000Z' },
      { minuteTs: '2026-01-01T01:01:00.000Z' },
    ]
    const segments: ChartGameSegment[] = [
      { gameName: 'Game A', offsetSeconds: 660, durationSeconds: 540 },
      { gameName: 'Game B', offsetSeconds: 1200, durationSeconds: 600 },
      { gameName: 'Game C', offsetSeconds: 1800, durationSeconds: 600 },
      { gameName: 'Game D', offsetSeconds: 2400, durationSeconds: 600 },
      { gameName: 'Game E', offsetSeconds: 3000, durationSeconds: 600 },
    ]
    const bounds = segments.map(segment =>
      gameSegmentPlotBounds(segment, rollups, streamStartedAt, 90, 820),
    )
    expect(bounds.filter(Boolean)).toHaveLength(5)
    for (const bound of bounds) {
      expect(bound!.startX).toBeGreaterThanOrEqual(90)
      expect(bound!.endX).toBeLessThanOrEqual(910)
    }
  })

  it('returns null when segment ends before visible rollup window', () => {
    const rollups = [{ minuteTs: '2026-01-01T00:11:00.000Z' }]
    const segment: ChartGameSegment = {
      gameName: 'Early only',
      offsetSeconds: 0,
      durationSeconds: 300,
    }
    expect(
      gameSegmentPlotBounds(segment, rollups, '2026-01-01T00:00:00.000Z', 90, 820),
    ).toBeNull()
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
