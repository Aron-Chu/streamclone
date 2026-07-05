import type { AnalyticsMinuteRollup } from '../../api.ts'
import { formatHeatOffset } from '@streamclone/pulse-core'

export function count(value: number | null | undefined) {
  if (value === null || value === undefined) return '-'
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return value.toLocaleString()
}

export function clock(value?: string) {
  if (!value) return '-'
  const ts = Date.parse(value)
  if (!Number.isFinite(ts)) return '-'
  return new Date(ts).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
}

/** VOD offset from stream start (00:00:00 at startedAt). Falls back to local clock when start is unknown. */
export function vodClock(minuteTs?: string, startedAt?: string): string {
  if (!minuteTs) return '-'
  if (!startedAt) return clock(minuteTs)
  const startMs = new Date(startedAt).getTime()
  const minuteMs = new Date(minuteTs).getTime()
  if (!Number.isFinite(startMs) || !Number.isFinite(minuteMs)) return clock(minuteTs)
  const offsetSeconds = Math.max(0, Math.floor((minuteMs - startMs) / 1000))
  return formatHeatOffset(offsetSeconds)
}

export function formatVodClock(sec?: number) {
  if (sec == null || sec < 0) return '0s'
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  if (h > 0) return `${h}h ${String(m).padStart(2, '0')}m`
  if (m > 0) return `${m}m ${String(s).padStart(2, '0')}s`
  return `${s}s`
}

export function seriesMax(values: Array<number | null>) {
  return Math.max(0, ...values.map(value => value ?? 0))
}

export function viewerValue(point: AnalyticsMinuteRollup) {
  return point.viewerLatest || point.viewerAvg || point.viewerMax || 0
}

/** Stable viewer sample for chart display — prefers minute average when sampled. */
export function chartViewerValue(point: AnalyticsMinuteRollup) {
  if ((point.viewerSamples ?? 0) > 0 && (point.viewerAvg ?? 0) > 0) {
    return point.viewerAvg!
  }
  return viewerValue(point)
}

/** Pixel-bucket decimation for chart render paths (keeps peak per bucket). */
export function decimateSeriesForRender(
  values: Array<number | null>,
  maxPoints: number,
): Array<number | null> {
  const n = values.length
  if (n === 0 || maxPoints <= 0 || n <= maxPoints) return values
  const bucketSize = n / maxPoints
  const out: Array<number | null> = []
  for (let bucket = 0; bucket < maxPoints; bucket++) {
    const start = Math.floor(bucket * bucketSize)
    const end = Math.min(n, Math.floor((bucket + 1) * bucketSize))
    if (end <= start) continue
    let peak: number | null = null
    for (let i = start; i < end; i++) {
      const value = values[i]
      if (value !== null && value > 0 && (peak === null || value > peak)) {
        peak = value
      }
    }
    out.push(peak)
  }
  return out
}

/** Light rolling median for display smoothing (viewers only). */
export function rollingMedianWindow(values: Array<number | null>, window: number): Array<number | null> {
  if (window <= 1) return values
  const radius = Math.floor(window / 2)
  return values.map((value, index) => {
    if (value === null) return null
    const samples: number[] = []
    for (let j = Math.max(0, index - radius); j <= Math.min(values.length - 1, index + radius); j++) {
      const sample = values[j]
      if (sample !== null && sample > 0) samples.push(sample)
    }
    if (!samples.length) return value
    samples.sort((a, b) => a - b)
    return samples[Math.floor(samples.length / 2)]
  })
}

/** @deprecated use rollingMedianWindow(values, 3) */
export function rollingMedian3(values: Array<number | null>): Array<number | null> {
  return rollingMedianWindow(values, 3)
}

/** Pick viewer smooth window: wider for dense live samples, narrow for TT backfill. */
export function viewerChartSmoothWindow(rollups: AnalyticsMinuteRollup[], viewerSource?: string) {
  if (viewerSource === 'live') return 3
  if (viewerSource === 'tt' || viewerSource === 'twitchtracker') return 5
  if (viewerSource === 'merged') return 4
  const sampleMinutes = rollups.filter(point => !point.missing && (point.viewerSamples ?? 0) >= 2).length
  if (sampleMinutes >= rollups.length * 0.4) return 3
  return 5
}

export function viewerSourceLabel(source?: string) {
  switch (source) {
    case 'live':
      return 'Live samples'
    case 'tt':
    case 'twitchtracker':
      return 'TwitchTracker'
    case 'merged':
      return 'Live + TT gaps'
    case 'partial':
      return 'Partial viewers'
    default:
      return ''
  }
}

export function analyzeViewerCoverage(rollups: AnalyticsMinuteRollup[]) {
  const indexed = rollups
    .map((point, idx) => ({ idx, value: !point.missing ? viewerValue(point) : 0 }))
    .filter(point => point.value > 0)
  if (indexed.length < 3) {
    return {
      hasViewerRollups: false,
      hasFlatViewerLine: false,
      hasPartialTail: false,
      hasShortSpan: false,
    }
  }
  const values = indexed.map(point => point.value)
  const min = Math.min(...values)
  const max = Math.max(...values)
  const hasFlatViewerLine = min === max
  const tailCount = Math.max(4, Math.floor(indexed.length * 0.4))
  const headCount = Math.max(4, Math.floor(indexed.length * 0.2))
  const tailValues = values.slice(-tailCount)
  const headValues = values.slice(0, headCount)
  const tailFlat = tailValues.length >= 4 && Math.min(...tailValues) === Math.max(...tailValues)
  const headVaried = headValues.length >= 4 && Math.min(...headValues) !== Math.max(...headValues)
  const hasPartialTail = indexed.length >= 12 && tailFlat && headVaried
  const spanMinutes = indexed[indexed.length - 1].idx - indexed[0].idx + 1
  const hasShortSpan = rollups.length >= 10 && (spanMinutes / rollups.length) < 0.7
  return {
    hasViewerRollups: true,
    hasFlatViewerLine,
    hasPartialTail,
    hasShortSpan,
  }
}

export function minuteEmoteTotal(point: AnalyticsMinuteRollup) {
  const total = point.totalEmoteCount ?? 0
  if (total > 0) return total
  if (!point.emotes) return 0
  return Object.values(point.emotes).reduce((sum, count) => sum + count, 0)
}

export function rollupHasMinuteData(point: AnalyticsMinuteRollup) {
  return !point.missing && (
    (point.viewerSamples ?? 0) > 0
    || viewerValue(point) > 0
    || (point.chatCount ?? 0) > 0
    || minuteEmoteTotal(point) > 0
  )
}

export function rollupsHaveViewerData(rollups: AnalyticsMinuteRollup[]) {
  return rollups.some(point => !point.missing && viewerValue(point) > 0)
}

export function computeRollupViewerStats(rollups: AnalyticsMinuteRollup[]) {
  const values = rollups
    .filter(point => !point.missing && viewerValue(point) > 0)
    .map(point => viewerValue(point))
  if (!values.length) return null
  return {
    current: values[values.length - 1],
    peak: Math.max(...values),
    avg: Math.round(values.reduce((sum, value) => sum + value, 0) / values.length),
  }
}

export function computeRollupChatStats(rollups: AnalyticsMinuteRollup[]) {
  let chat = 0
  let emotes = 0
  for (const point of rollups) {
    if (point.missing) continue
    chat += point.chatCount ?? 0
    emotes += minuteEmoteTotal(point)
  }
  return { chat, emotes }
}
