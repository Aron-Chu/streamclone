export type GameSegmentPlotInput = {
  offsetSeconds: number
  durationSeconds: number
}

export type RollupMinuteTs = {
  minuteTs: string
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
