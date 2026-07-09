import type { ChartGameSegment } from './types.ts'

export function normalizeGameSegments(
  games: ChartGameSegment[],
  durationSeconds: number,
): ChartGameSegment[] {
  if (!games.length || durationSeconds <= 0) return []

  const cleaned = games
    .filter(game =>
      Number.isFinite(game.offsetSeconds)
      && Number.isFinite(game.durationSeconds)
      && game.offsetSeconds >= 0,
    )
    .map(game => ({
      ...game,
      offsetSeconds: Math.max(0, game.offsetSeconds),
      durationSeconds: Math.max(0, game.durationSeconds),
    }))

  if (!cleaned.length) return []

  const needsRepair = cleaned.every(game => game.durationSeconds <= 0)
  if (!needsRepair) {
    return cleaned.filter(game => game.durationSeconds > 0)
  }

  if (!Number.isFinite(durationSeconds) || durationSeconds <= 0) return []
  const each = Math.max(60, Math.floor(durationSeconds / cleaned.length))
  let offset = 0
  return cleaned.map((game, index) => {
    const segmentDuration = index === cleaned.length - 1
      ? Math.max(60, durationSeconds - offset)
      : each
    const segment = { ...game, offsetSeconds: offset, durationSeconds: segmentDuration }
    offset += segmentDuration
    return segment
  })
}

/** Extension honesty: hide single full-stream category unless multi-segment or partial coverage. */
export function hasMeaningfulGameSegments(
  segments: ChartGameSegment[],
  durationSeconds: number,
): boolean {
  if (!segments.length || durationSeconds <= 0) return false
  if (segments.length > 1) return true
  const only = segments[0]!
  if (only.offsetSeconds > 0) return true
  const coverage = only.durationSeconds / durationSeconds
  return coverage < 0.9
}

export function gameSegmentKey(
  segment: Pick<ChartGameSegment, 'gameName' | 'offsetSeconds'>,
): string {
  return `${segment.gameName.trim().toLowerCase()}:${segment.offsetSeconds}`
}

function gameSegmentEndOffset(
  segment: Pick<ChartGameSegment, 'offsetSeconds' | 'durationSeconds'>,
): number {
  return segment.offsetSeconds + Math.max(0, segment.durationSeconds)
}

export function gameSegmentOverlapsOffsetRange(
  segment: Pick<ChartGameSegment, 'offsetSeconds' | 'durationSeconds'>,
  rangeStart: number,
  rangeEnd: number,
): boolean {
  if (!Number.isFinite(rangeStart) || !Number.isFinite(rangeEnd)) return false
  const start = segment.offsetSeconds
  const end = gameSegmentEndOffset(segment)
  return end > rangeStart && start <= rangeEnd
}

export function gameSegmentVisibleSecondsInRange(
  segment: Pick<ChartGameSegment, 'offsetSeconds' | 'durationSeconds'>,
  rangeStart: number,
  rangeEnd: number,
): number {
  if (!gameSegmentOverlapsOffsetRange(segment, rangeStart, rangeEnd)) return 0
  const start = segment.offsetSeconds
  const end = gameSegmentEndOffset(segment)
  const visibleStart = Math.max(start, rangeStart)
  const visibleEnd = Math.min(end, rangeEnd)
  return Math.max(0, visibleEnd - visibleStart)
}
