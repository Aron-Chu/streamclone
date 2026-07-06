export type GameSegmentPlotInput = {
  offsetSeconds: number
  durationSeconds: number
}

export type RollupMinuteTs = {
  minuteTs: string
}

const PLACEHOLDER_CATEGORIES = /^(live|syncing\.{3}|syncing…)$/i

/** Span in seconds covered by minute rollup timestamps (inclusive of last bucket). */
export function minuteRollupSpanSeconds(rollups: Array<{ minuteTs: string }>): number {
  if (rollups.length === 0) return 0
  if (rollups.length === 1) return 60
  const first = Date.parse(rollups[0].minuteTs)
  const last = Date.parse(rollups[rollups.length - 1].minuteTs)
  if (!Number.isFinite(first) || !Number.isFinite(last)) return rollups.length * 60
  return Math.max(60, Math.round((last - first) / 1000) + 60)
}

type ChartGameSegment = {
  id: number
  streamId: string
  gameName: string
  boxArtUrl: string
  offsetSeconds: number
  durationSeconds: number
  createdAt: string
  source?: string
}

/** Synthesize one chart segment when the games API is empty but stream category is known. */
export function deriveChartGameSegments(
  streamId: string,
  detail: { stream?: { category?: string }; rollups?: Array<{ minuteTs: string }> } | null | undefined,
  apiSegments: ChartGameSegment[] | null | undefined,
  options?: { allowCategoryFallback?: boolean },
): ChartGameSegment[] {
  if (apiSegments?.length) return apiSegments
  if (options?.allowCategoryFallback === false) return []
  const category = detail?.stream?.category?.trim() ?? ''
  if (!category || PLACEHOLDER_CATEGORIES.test(category)) return []
  const rollups = detail?.rollups ?? []
  const durationSeconds = minuteRollupSpanSeconds(rollups)
  if (durationSeconds <= 0) return []
  return [
    {
      id: 0,
      streamId,
      gameName: category,
      boxArtUrl: '',
      offsetSeconds: 0,
      durationSeconds,
      createdAt: new Date(0).toISOString(),
      source: 'category_fallback',
    },
  ]
}

/** Map absolute stream game segments onto the visible chart rollup window. */
export function gameSegmentPlotBounds(
  segment: GameSegmentPlotInput,
  rollups: RollupMinuteTs[],
  streamStartedAt: string | undefined,
  plotLeft: number,
  plotWidth: number,
): { startX: number; endX: number; centerX: number; textWidth: number } | null {
  if (
    rollups.length < 1
    || !Number.isFinite(segment.offsetSeconds)
    || !Number.isFinite(segment.durationSeconds)
    || segment.durationSeconds <= 0
    || plotWidth <= 0
  ) {
    return null
  }

  const chartFirstMs = Date.parse(rollups[0].minuteTs)
  const chartLastMs = Date.parse(rollups[rollups.length - 1].minuteTs)
  if (!Number.isFinite(chartFirstMs) || !Number.isFinite(chartLastMs)) return null
  const chartSpanMs = chartLastMs - chartFirstMs
  if (!Number.isFinite(chartSpanMs) || chartSpanMs <= 0) return null

  const streamStartMs = streamStartedAt ? Date.parse(streamStartedAt) : chartFirstMs
  if (!Number.isFinite(streamStartMs)) return null

  const segStartMs = streamStartMs + Math.max(0, segment.offsetSeconds) * 1000
  const segEndMs = segStartMs + segment.durationSeconds * 1000
  const visibleStartMs = Math.max(chartFirstMs, segStartMs)
  const visibleEndMs = Math.min(chartLastMs, segEndMs)
  if (visibleEndMs <= visibleStartMs) return null

  const startPct = (visibleStartMs - chartFirstMs) / chartSpanMs
  const endPct = (visibleEndMs - chartFirstMs) / chartSpanMs
  const startX = plotLeft + startPct * plotWidth
  const endX = plotLeft + endPct * plotWidth
  if (!Number.isFinite(startX) || !Number.isFinite(endX)) return null

  return {
    startX,
    endX,
    centerX: (startX + endX) / 2,
    textWidth: endX - startX,
  }
}
