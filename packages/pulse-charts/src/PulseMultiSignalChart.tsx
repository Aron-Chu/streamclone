import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { parseEmoteKey } from '@streamclone/pulse-core'
import type { ChartGameSegment, ChartMinuteRollup, ChartPlayhead } from './types.ts'
import { computeChartCursorSync } from './chartCursorSync.ts'
import { lerpActivityLayout } from './emotePlotSelection.ts'
import { useSmoothedScalar } from './useSmoothedScalar.ts'
import { GameSegmentOverlay } from './GameSegmentOverlay.tsx'
import { normalizeGameSegments } from './gameSegments.ts'
import { CHART_THEME, emoteChartColor, hexToRgba, legendDotStyle } from './chartTheme.ts'
import {
  analyzeViewerCoverage,
  chartViewerValue,
  count,
  decimateSeriesForRender,
  formatVodClock,
  vodClock,
  minuteEmoteTotal,
  rollupHasMinuteData,
  rollupsHaveViewerData,
  rollingMedianWindow,
  viewerChartSmoothWindow,
  viewerSourceLabel,
  seriesMax,
  viewerValue,
} from './chartRollupUtils.ts'

import { buildChartSeries, type ChartSeries } from './chartSeries.ts'

const ChartHoverReadout = memo(function ChartHoverReadout({
  minuteTs,
  streamStartedAt,
  viewers,
  chatCount,
  emoteTotal,
}: {
  minuteTs?: string
  streamStartedAt?: string
  viewers: number | null
  chatCount?: number | null
  emoteTotal: number | null
}) {
  return (
    <p
      className="min-w-0 truncate text-xs font-bold tabular-nums text-zinc-500"
      title="Values at the hovered minute on the chart"
    >
      {vodClock(minuteTs, streamStartedAt)} · viewers {count(viewers)} · chat {count(chatCount)}/min · emotes {count(emoteTotal)}/min
    </p>
  )
})

function buildSeries(
  rollups: ChartMinuteRollup[],
  selected: Set<string>,
  peakViewersFallback = 0,
  avgViewersFallback = 0,
  useViewerFallback = false,
): ChartSeries[] {
  return buildChartSeries(rollups, selected, peakViewersFallback, avgViewersFallback, useViewerFallback)
}

type ViewerAxis = { min: number; max: number; mode: 'peak' | 'fit' }

function viewerScaleBounds(
  values: Array<number | null>,
  streamPeak: number,
  fitToVisible: boolean,
): ViewerAxis {
  const visible = values.filter((v): v is number => v !== null && v > 0)
  const visibleMax = visible.length > 0 ? Math.max(...visible) : 0
  const visibleMin = visible.length > 0 ? Math.min(...visible) : 0
  const peakMax = Math.max(1, streamPeak, visibleMax)
  if (!fitToVisible || visibleMax <= 0) {
    return { min: 0, max: peakMax, mode: fitToVisible ? 'fit' : 'peak' }
  }
  const span = Math.max(0, visibleMax - visibleMin)
  const pad = span > 0 ? span * 0.08 : visibleMax * 0.04
  const fitMin = Math.max(0, Math.floor(visibleMin - pad))
  const fitMax = Math.max(fitMin + 1, Math.ceil(visibleMax + pad))
  return { min: fitMin, max: fitMax, mode: 'fit' }
}

const ACTIVITY_ZONE_FRACTION_DEFAULT = 0.36
/** Set at the start of each AnalyticsChart render for layout helpers below. */
let renderActivityZoneFraction = ACTIVITY_ZONE_FRACTION_DEFAULT
let renderActivityChatFraction = 0.48
let renderActivityEmoteTraceFraction = 0.18
let renderActivityEmoteBarsFraction = 0.34
const ACTIVITY_ZONE_GAP = 8
/** Keep per-emote traces off the chat/trace divider when the rail is thin. */
const ACTIVITY_TRACE_INSET = 0.12
const CHART_VIEWBOX_HEIGHT = 400

type PlotZone = 'viewer' | 'activity-chat' | 'activity-emote-trace' | 'activity-emote'

function plotBandForZone(
  height: number,
  padTop: number,
  padBottom: number,
  zone: PlotZone,
) {
  const fullPlotHeight = height - padTop - padBottom
  const activityHeight = fullPlotHeight * renderActivityZoneFraction
  const viewerHeight = fullPlotHeight - activityHeight - ACTIVITY_ZONE_GAP
  const activityTop = height - padBottom - activityHeight
  const chatSplit = activityTop + activityHeight * renderActivityChatFraction
  const traceSplit = chatSplit + activityHeight * renderActivityEmoteTraceFraction
  const layoutBase = { activityTop, activityHeight, chatSplit, traceSplit }

  switch (zone) {
    case 'viewer':
      return { bandTop: padTop, bandBottom: padTop + viewerHeight, bandHeight: viewerHeight, ...layoutBase }
    case 'activity-chat':
      return {
        bandTop: activityTop,
        bandBottom: chatSplit,
        bandHeight: activityHeight * renderActivityChatFraction,
        ...layoutBase,
      }
    case 'activity-emote-trace':
      return {
        bandTop: chatSplit,
        bandBottom: traceSplit,
        bandHeight: activityHeight * renderActivityEmoteTraceFraction,
        ...layoutBase,
      }
    case 'activity-emote':
      return {
        bandTop: traceSplit,
        bandBottom: height - padBottom,
        bandHeight: activityHeight * renderActivityEmoteBarsFraction,
        ...layoutBase,
      }
    default:
      return { bandTop: padTop, bandBottom: height - padBottom, bandHeight: fullPlotHeight, ...layoutBase }
  }
}

function plotY(
  value: number,
  max: number,
  height: number,
  padTop: number,
  padBottom: number,
  zone: PlotZone = 'viewer',
  rangeMin = 0,
) {
  let { bandTop, bandBottom, bandHeight } = plotBandForZone(height, padTop, padBottom, zone)
  if (zone === 'activity-emote-trace' && bandHeight > 0) {
    const inset = bandHeight * ACTIVITY_TRACE_INSET
    bandTop += inset
    bandBottom -= inset
    bandHeight = Math.max(1, bandBottom - bandTop)
  }
  const span = Math.max(1, max - rangeMin)
  const y = bandBottom - ((Math.max(rangeMin, value) - rangeMin) / span) * bandHeight
  return Math.max(bandTop, Math.min(bandBottom, y))
}

type ActivityAxis = { min: number; max: number; mode: 'peak' | 'fit' }

function activityAxisBounds(series: ChartSeries[], fitToVisible = true, options: { includeAggregateEmotes?: boolean } = {}): ActivityAxis {
  const includeAggregateEmotes = options.includeAggregateEmotes ?? true
  const visible: number[] = []
  for (const item of series) {
    if (item.key === 'chat') continue
    if (!includeAggregateEmotes && item.key === 'emotes') continue
    for (const value of item.values) {
      if (value !== null && value > 0) visible.push(value)
    }
  }
  if (visible.length === 0) return { min: 0, max: 1, mode: fitToVisible ? 'fit' : 'peak' }
  const visibleMin = Math.min(...visible)
  const visibleMax = Math.max(...visible)
  const peakMax = Math.max(1, visibleMax)
  if (!fitToVisible) {
    return { min: 0, max: Math.ceil(peakMax * 1.06), mode: 'peak' }
  }
  const span = Math.max(0, visibleMax - visibleMin)
  const pad = span > 0 ? span * 0.05 : Math.max(1, visibleMax * 0.08)
  const fitMin = span > 0 ? Math.max(0, Math.floor(visibleMin - pad)) : 0
  const fitMax = Math.max(fitMin + 1, Math.ceil(visibleMax + pad))
  return { min: fitMin, max: fitMax, mode: 'fit' }
}

function emoteSpikeIndices(values: Array<number | null>, minFraction = 0.32, maxSpikes = 0) {
  if (maxSpikes <= 0) return []
  const positives = values.filter((v): v is number => v !== null && v > 0)
  if (positives.length === 0) return []
  const max = Math.max(...positives)
  const sorted = [...positives].sort((a, b) => a - b)
  const median = sorted[Math.floor(sorted.length / 2)] ?? 0
  const threshold = Math.max(max * minFraction, median * 1.35, 1)
  const spikes: number[] = []
  for (let i = 0; i < values.length; i++) {
    const value = values[i]
    if (value === null || value < threshold) continue
    const prev = i > 0 ? (values[i - 1] ?? 0) : 0
    const next = i < values.length - 1 ? (values[i + 1] ?? 0) : 0
    if (value >= prev && value >= next) spikes.push(i)
  }
  if (spikes.length <= maxSpikes) return spikes
  return spikes
    .sort((a, b) => (values[b] ?? 0) - (values[a] ?? 0))
    .slice(0, maxSpikes)
    .sort((a, b) => a - b)
}

function smoothDisplayValues(values: Array<number | null>, window = 3): Array<number | null> {
  if (window <= 1 || values.length < 3) return values
  const radius = Math.floor(window / 2)
  return values.map((value, index) => {
    if (value === null) return null
    let sum = 0
    let count = 0
    for (let offset = -radius; offset <= radius; offset++) {
      const sample = values[index + offset]
      if (sample === null || sample === undefined) continue
      sum += sample
      count += 1
    }
    return count > 0 ? sum / count : value
  })
}

function activityBarsMaxForLength(length: number, plotWidthPx = 876, pxPerBar = 1.05) {
  if (length <= 0) return 0
  const target = Math.floor(plotWidthPx / pxPerBar)
  return Math.min(length, Math.max(target, 64))
}

type ActivityBarRect = {
  key: string
  x: number
  y: number
  width: number
  height: number
  hasValue: boolean
  isSpike?: boolean
}

function activityBarRects(
  values: Array<number | null>,
  max: number,
  rangeMin: number,
  zone: PlotZone,
  width: number,
  height: number,
  padLeft: number,
  padRight: number,
  padTop: number,
  padBottom: number,
  density: { pxPerBar: number; minWidth: number; maxWidth: number },
  spikeThreshold = 0,
): ActivityBarRect[] {
  const n = values.length
  if (n === 0) return []
  const plotWidth = width - padLeft - padRight
  const barSeries = chatBarsForChart(values, activityBarsMaxForLength(n, plotWidth, density.pxPerBar))
  const barCount = barSeries.length
  const slotWidth = barCount <= 1 ? plotWidth : plotWidth / Math.max(1, barCount - 1)
  const barWidth = Math.min(density.maxWidth, Math.max(density.minWidth, slotWidth * 0.98))
  const { bandBottom } = plotBandForZone(height, padTop, padBottom, zone)

  return barSeries.map(({ index, value }, barIdx) => {
    const cx = barCount === 1
      ? padLeft
      : padLeft + (barIdx / Math.max(1, barCount - 1)) * plotWidth
    const cy = plotY(value, max, height, padTop, padBottom, zone, rangeMin)
    const barHeight = value > 0 ? Math.max(1, bandBottom - cy) : 1
    const y = value > 0 ? cy : bandBottom - 1
    return {
      key: `bar-${index}-${barIdx}`,
      x: cx - barWidth / 2,
      y,
      width: barWidth,
      height: barHeight,
      hasValue: value > 0,
      isSpike: spikeThreshold > 0 && value > spikeThreshold,
    }
  })
}

function chatBarsForChart(values: Array<number | null>, maxBars = 360) {
  const n = values.length
  if (n === 0) return [] as Array<{ index: number; value: number }>
  if (n <= maxBars) {
    return values.map((value, index) => ({ index, value: value ?? 0 }))
  }
  const bucketSize = n / maxBars
  const bars: Array<{ index: number; value: number }> = []
  for (let bucket = 0; bucket < maxBars; bucket++) {
    const start = Math.floor(bucket * bucketSize)
    const end = Math.min(n, Math.floor((bucket + 1) * bucketSize))
    if (end <= start) continue
    let peak = 0
    for (let i = start; i < end; i++) {
      const value = values[i] ?? 0
      if (value > peak) peak = value
    }
    const centerIndex = Math.min(n - 1, Math.floor((start + end - 1) / 2))
    bars.push({ index: centerIndex, value: peak })
  }
  return bars
}

function findEstimatedViewerTailStart(values: Array<number | null>) {
  const n = values.length
  if (n < 12) return -1
  const startSearch = Math.floor(n * 0.12)
  for (let i = startSearch; i < n - 8; i++) {
    const value = values[i]
    if (value === null || value <= 0) continue
    let flat = true
    for (let j = i + 1; j < Math.min(n, i + 10); j++) {
      if (values[j] !== value) {
        flat = false
        break
      }
    }
    if (!flat) continue
    const head = values.slice(startSearch, i).filter((point): point is number => point !== null && point > 0)
    if (head.length >= 3 && Math.min(...head) !== Math.max(...head)) return i
  }
  return -1
}

function linePath(
  values: Array<number | null>,
  max: number,
  width: number,
  height: number,
  padLeft: number,
  padRight: number,
  padTop: number,
  padBottom: number,
  linear = false,
  zone: PlotZone = 'viewer',
  rangeMin = 0,
) {
  const n = values.length
  if (!n) return ''
  const { bandTop, bandBottom } = plotBandForZone(height, padTop, padBottom, zone)

  const points: Array<{ x: number; y: number } | null> = values.map((value, index) => {
    if (value === null) return null
    const x = n === 1 ? padLeft : padLeft + (index / (n - 1)) * (width - padLeft - padRight)
    const y = plotY(value, max, height, padTop, padBottom, zone, rangeMin)
    return { x, y }
  })

  let path = ''
  let segment: Array<{ x: number; y: number }> = []

  const drawSegment = (seg: Array<{ x: number; y: number }>, linearSeg = false) => {
    if (seg.length === 0) return ''
    if (seg.length === 1) return `M${seg[0].x.toFixed(1)} ${seg[0].y.toFixed(1)}`
    if (seg.length === 2 || linearSeg) {
      let d = `M${seg[0].x.toFixed(1)} ${seg[0].y.toFixed(1)}`
      for (let i = 1; i < seg.length; i++) {
        d += ` L${seg[i].x.toFixed(1)} ${seg[i].y.toFixed(1)}`
      }
      return d
    }

    let d = `M${seg[0].x.toFixed(1)} ${seg[0].y.toFixed(1)}`

    // Compute slopes at each point for smooth tangent matching
    const slopes: number[] = new Array(seg.length)
    for (let i = 0; i < seg.length; i++) {
      if (i === 0) {
        slopes[i] = (seg[1].y - seg[0].y) / (seg[1].x - seg[0].x)
      } else if (i === seg.length - 1) {
        slopes[i] = (seg[i].y - seg[i - 1].y) / (seg[i].x - seg[i - 1].x)
      } else {
        const dx1 = seg[i].x - seg[i - 1].x
        const dy1 = seg[i].y - seg[i - 1].y
        const dx2 = seg[i + 1].x - seg[i].x
        const dy2 = seg[i + 1].y - seg[i].y
        slopes[i] = (dy1 / dx1 + dy2 / dx2) / 2
      }
    }

    for (let i = 0; i < seg.length - 1; i++) {
      const p1 = seg[i]
      const p2 = seg[i + 1]
      const dx = p2.x - p1.x

      const cp1x = p1.x + dx * 0.35
      const cp1y = Math.max(bandTop, Math.min(bandBottom, p1.y + slopes[i] * dx * 0.35))
      const cp2x = p2.x - dx * 0.35
      const cp2y = Math.max(bandTop, Math.min(bandBottom, p2.y - slopes[i + 1] * dx * 0.35))

      d += ` C ${cp1x.toFixed(1)} ${cp1y.toFixed(1)}, ${cp2x.toFixed(1)} ${cp2y.toFixed(1)}, ${p2.x.toFixed(1)} ${p2.y.toFixed(1)}`
    }
    return d
  }

  for (let i = 0; i < points.length; i++) {
    const pt = points[i]
    if (pt === null) {
      if (segment.length > 0) {
        path += (path ? ' ' : '') + drawSegment(segment, linear)
        segment = []
      }
    } else {
      segment.push(pt)
    }
  }
  if (segment.length > 0) {
    path += (path ? ' ' : '') + drawSegment(segment, linear)
  }

  return path
}

function areaPath(
  values: Array<number | null>,
  max: number,
  width: number,
  height: number,
  padLeft: number,
  padRight: number,
  padTop: number,
  padBottom: number,
  zone: PlotZone = 'viewer',
  rangeMin = 0,
) {
  const n = values.length
  if (!n) return ''
  const { bandTop, bandBottom } = plotBandForZone(height, padTop, padBottom, zone)

  const points: Array<{ x: number; y: number } | null> = values.map((value, index) => {
    if (value === null) return null
    const x = n === 1 ? padLeft : padLeft + (index / (n - 1)) * (width - padLeft - padRight)
    const y = plotY(value, max, height, padTop, padBottom, zone, rangeMin)
    return { x, y }
  })

  let path = ''
  let segment: Array<{ x: number; y: number }> = []

  const drawAreaSegment = (seg: Array<{ x: number; y: number }>) => {
    if (seg.length === 0) return ''
    const bottomY = bandBottom

    let d = `M${seg[0].x.toFixed(1)} ${bottomY.toFixed(1)}`
    d += ` L${seg[0].x.toFixed(1)} ${seg[0].y.toFixed(1)}`

    if (seg.length === 2) {
      d += ` L${seg[1].x.toFixed(1)} ${seg[1].y.toFixed(1)}`
    } else if (seg.length > 2) {
      const slopes: number[] = new Array(seg.length)
      for (let i = 0; i < seg.length; i++) {
        if (i === 0) {
          slopes[i] = (seg[1].y - seg[0].y) / (seg[1].x - seg[0].x)
        } else if (i === seg.length - 1) {
          slopes[i] = (seg[i].y - seg[i - 1].y) / (seg[i].x - seg[i - 1].x)
        } else {
          const dx1 = seg[i].x - seg[i - 1].x
          const dy1 = seg[i].y - seg[i - 1].y
          const dx2 = seg[i + 1].x - seg[i].x
          const dy2 = seg[i + 1].y - seg[i].y
          slopes[i] = (dy1 / dx1 + dy2 / dx2) / 2
        }
      }

      for (let i = 0; i < seg.length - 1; i++) {
        const p1 = seg[i]
        const p2 = seg[i + 1]
        const dx = p2.x - p1.x
        const cp1x = p1.x + dx * 0.35
        const cp1y = Math.max(bandTop, Math.min(bandBottom, p1.y + slopes[i] * dx * 0.35))
        const cp2x = p2.x - dx * 0.35
        const cp2y = Math.max(bandTop, Math.min(bandBottom, p2.y - slopes[i + 1] * dx * 0.35))
        d += ` C ${cp1x.toFixed(1)} ${cp1y.toFixed(1)}, ${cp2x.toFixed(1)} ${cp2y.toFixed(1)}, ${p2.x.toFixed(1)} ${p2.y.toFixed(1)}`
      }
    }

    d += ` L${seg[seg.length - 1].x.toFixed(1)} ${bottomY.toFixed(1)}`
    d += ' Z'
    return d
  }

  for (let i = 0; i < points.length; i++) {
    const pt = points[i]
    if (pt === null) {
      if (segment.length > 0) {
        path += (path ? ' ' : '') + drawAreaSegment(segment)
        segment = []
      }
    } else {
      segment.push(pt)
    }
  }
  if (segment.length > 0) {
    path += (path ? ' ' : '') + drawAreaSegment(segment)
  }

  return path
}

function PulseMultiSignalChartInnerImpl({
  rollups: allRollups,
  games = [],
  streamStartedAt,
  chartStreamId = null,
  peakViewersFallback = 0,
  avgViewersFallback = 0,
  viewerSource,
  selectedEmotes = new Set<string>(),
  selectedRollup = null,
  previewRollup = null,
  onSelectRollup,
  syncing = false,
  isLive = false,
  showSpikes: showSpikesProp = false,
  showDots: showDotsProp = false,
  activityExpanded: activityExpandedProp = false,
  height: heightProp,
  playhead = null,
  variant = 'compact',
  motionEnabled = true,
  chromeless = false,
  onHoverRollupChange,
  focusedSeriesKey: focusedSeriesKeyProp,
  onFocusedSeriesKeyChange,
}: {
  rollups: ChartMinuteRollup[]
  games?: ChartGameSegment[]
  streamStartedAt?: string
  chartStreamId?: string | null
  peakViewersFallback?: number
  avgViewersFallback?: number
  viewerSource?: string
  selectedEmotes?: Set<string>
  selectedRollup?: ChartMinuteRollup | null
  previewRollup?: ChartMinuteRollup | null
  onSelectRollup?: (rollup: ChartMinuteRollup | null) => void
  syncing?: boolean
  isLive?: boolean
  showSpikes?: boolean
  showDots?: boolean
  activityExpanded?: boolean
  height?: number
  playhead?: { streamId: string; offsetSeconds: number; isPlaying: boolean } | null
  variant?: 'console' | 'compact'
  motionEnabled?: boolean
  chromeless?: boolean
  onHoverRollupChange?: (rollup: ChartMinuteRollup | null) => void
  focusedSeriesKey?: string | null
  onFocusedSeriesKeyChange?: (key: string | null) => void
}) {
  const [hover, setHover] = useState<number | null>(null)
  const hoverIndexRef = useRef<number | null>(null)
  const hoverRafRef = useRef<number | null>(null)
  const [activityExpanded, setActivityExpanded] = useState(activityExpandedProp)
  const [showDots, setShowDots] = useState(showDotsProp)
  // Same-page playhead sync (Req 22.1, 22.3): when a VOD player is mounted on
  // the same page for THIS stream and is actively playing, the chart shows a
  // playback cursor tracking the shared playhead. When no player is present,
  // the stream id does not match, or the player is inactive, sync is disabled
  // and the chart uses its standard hover cursor.
  const cursorSync = computeChartCursorSync({
    chartStreamId,
    playhead: playhead ?? { streamId: '', offsetSeconds: 0, isPlaying: false },
  })
  const [showSpikes, setShowSpikes] = useState(showSpikesProp)
  const [focusedSeriesKeyState, setFocusedSeriesKeyState] = useState<string | null>(null)
  const focusedSeriesKey = focusedSeriesKeyProp ?? focusedSeriesKeyState
  const setFocusedSeriesKey = useCallback((key: string | null) => {
    onFocusedSeriesKeyChange?.(key)
    if (focusedSeriesKeyProp === undefined) setFocusedSeriesKeyState(key)
  }, [focusedSeriesKeyProp, onFocusedSeriesKeyChange])
  const [hoveredSpikeKey, setHoveredSpikeKey] = useState<string | null>(null)
  // motionEnabled from props
  const expandProgress = useSmoothedScalar(activityExpanded ? 1 : 0, motionEnabled)
  useEffect(() => { setShowSpikes(showSpikesProp) }, [showSpikesProp])
  useEffect(() => { setShowDots(showDotsProp) }, [showDotsProp])
  useEffect(() => { setActivityExpanded(activityExpandedProp) }, [activityExpandedProp])
  const seriesFocusOpacity = useCallback((seriesKey: string, base: number) => {
    if (!focusedSeriesKey) return base
    if (seriesKey === focusedSeriesKey) return base

    const emoteFamily = seriesKey === 'emotes' || seriesKey.includes(':')
    if (focusedSeriesKey === 'emotes' && emoteFamily) return base

    return base * 0.14
  }, [focusedSeriesKey])
  const rollups = allRollups
  const commitHover = useCallback((index: number | null) => {
    hoverIndexRef.current = index
    if (onHoverRollupChange) {
      onHoverRollupChange(index != null && rollups[index] ? rollups[index]! : null)
    }
    if (hoverRafRef.current != null) return
    hoverRafRef.current = requestAnimationFrame(() => {
      hoverRafRef.current = null
      setHover(hoverIndexRef.current)
    })
  }, [onHoverRollupChange, rollups])
  useEffect(() => () => {
    if (hoverRafRef.current != null) {
      cancelAnimationFrame(hoverRafRef.current)
    }
  }, [])

  const hasSyncedChat = rollups.some(point => !point.missing && (point.chatCount ?? 0) > 0)
  const viewerCoverage = useMemo(() => analyzeViewerCoverage(rollups), [rollups])
  const hasViewerRollups = viewerCoverage.hasViewerRollups
  const hasFlatViewerLine = viewerCoverage.hasFlatViewerLine
  const useViewerFallback = !isLive
    && !hasSyncedChat
    && rollups.every(point => point.missing || viewerValue(point) === 0)
  const needsViewerResync = !isLive && hasSyncedChat && (
    !hasViewerRollups
    || hasFlatViewerLine
    || viewerCoverage.hasPartialTail
    || viewerCoverage.hasShortSpan
  )
  const hasChartData = useMemo(
    () => rollups.some(rollupHasMinuteData),
    [rollups],
  )
  const hasViewerChartData = useMemo(
    () => rollupsHaveViewerData(rollups),
    [rollups],
  )
  const canRenderChart = hasChartData || hasViewerChartData
  const width = 1000
  const height = heightProp ?? CHART_VIEWBOX_HEIGHT
  const padLeft = 90
  const padRight = 34
  const padTop = 34
  const padBottom = 34
  const plotWidthPx = width - padLeft - padRight

  const decimateViewerForRender = useCallback((values: Array<number | null>) => {
    let prepared = values
    if (prepared.length > plotWidthPx) {
      prepared = decimateSeriesForRender(prepared, plotWidthPx)
    }
    const smoothWindow = viewerChartSmoothWindow(rollups, viewerSource)
    return rollingMedianWindow(prepared, smoothWindow)
  }, [plotWidthPx, rollups, viewerSource])

  const series = useMemo(
    () => buildSeries(rollups, selectedEmotes, peakViewersFallback, avgViewersFallback, useViewerFallback),
    [rollups, selectedEmotes, peakViewersFallback, avgViewersFallback, useViewerFallback],
  )
  const viewersItem = useMemo(() => series.find(s => s.key === 'viewers'), [series])
  const viewerDisplayValues = useMemo(() => {
    if (!viewersItem) return [] as Array<number | null>
    return decimateViewerForRender(viewersItem.values)
  }, [viewersItem, decimateViewerForRender])
  const chatItem = useMemo(() => series.find(s => s.key === 'chat'), [series])
  const emotesItem = useMemo(() => series.find(s => s.key === 'emotes'), [series])
  const perEmoteSeries = useMemo(() => series.filter(s => s.dashed), [series])
  const hasPlottedEmotes = perEmoteSeries.length > 0
  const activityAxisSeries = useMemo(
    () => series.filter(s => s.key !== 'viewers' && s.key !== 'chat'),
    [series],
  )
  const activityAxis = useMemo(
    () => activityAxisBounds(activityAxisSeries, true),
    [activityAxisSeries],
  )
  const activityScaleMax = activityAxis.max
  const activityScaleMin = activityAxis.min
  const selectedEmoteAxis = useMemo(
    () => activityAxisBounds(perEmoteSeries, true, { includeAggregateEmotes: false }),
    [perEmoteSeries],
  )
  const selectedEmoteScaleMax = selectedEmoteAxis.max
  const selectedEmoteScaleMin = selectedEmoteAxis.min
  // Per-emote traces always use their own scale — aggregate emote totals dwarf individual counts.
  const perEmotePlotAxis = selectedEmoteAxis
  const activityLayout = useMemo(() => {
    const layout = lerpActivityLayout(expandProgress, hasPlottedEmotes)
    renderActivityZoneFraction = layout.zoneFraction
    renderActivityChatFraction = layout.chat
    renderActivityEmoteTraceFraction = layout.trace
    renderActivityEmoteBarsFraction = layout.bars
    return plotBandForZone(height, padTop, padBottom, 'viewer')
  }, [height, padTop, padBottom, expandProgress, hasPlottedEmotes])
  const activityLabelYs = useMemo(() => ({
    chat: (activityLayout.activityTop + activityLayout.chatSplit) / 2,
    trace: (activityLayout.chatSplit + activityLayout.traceSplit) / 2,
    bars: (activityLayout.traceSplit + height - padBottom) / 2,
  }), [activityLayout, height, padBottom])
  const emoteBandMaxY = useMemo(
    () => plotY(activityAxis.max, activityAxis.max, height, padTop, padBottom, 'activity-emote', activityAxis.min),
    [activityAxis, height, padTop, padBottom, expandProgress, hasPlottedEmotes],
  )
  const viewerAxis = useMemo(
    () => viewerScaleBounds(viewersItem?.values ?? [], peakViewersFallback, true),
    [viewersItem, peakViewersFallback],
  )
  const viewerPeakAxis = useMemo(
    () => viewerScaleBounds(viewersItem?.values ?? [], peakViewersFallback, false),
    [viewersItem, peakViewersFallback],
  )
  const scaleForSeries = useCallback((item: ChartSeries) => {
    if (item.key === 'viewers') {
      return viewerAxis.max
    }
    if (item.key === 'chat') {
      return Math.max(1, chatItem?.max ?? item.max)
    }
    return activityAxis.max
  }, [viewerAxis.max, chatItem, activityAxis.max])
  const chatBandMaxY = useMemo(() => {
    if (!chatItem) return 0
    const chatMax = Math.max(1, chatItem.max)
    return plotY(chatMax, chatMax, height, padTop, padBottom, 'activity-chat')
  }, [chatItem, height, padTop, padBottom, expandProgress, hasPlottedEmotes])
  const emotesDisplayValues = useMemo(
    () => (emotesItem ? smoothDisplayValues(emotesItem.values, expandProgress >= 0.5 ? 2 : 3) : []),
    [emotesItem, expandProgress],
  )
  const chatDisplayValues = useMemo(
    () => (chatItem ? smoothDisplayValues(chatItem.values, expandProgress >= 0.5 ? 2 : 3) : []),
    [chatItem, expandProgress],
  )
  const activityVisualBoost = 1 + expandProgress * 0.2
  const emoteBarRects = useMemo(() => {
    if (!emotesItem) return []
    const spikeThreshold = activityAxis.max * 0.55
    return activityBarRects(
      emotesItem.values,
      activityAxis.max,
      activityAxis.min,
      'activity-emote',
      width,
      height,
      padLeft,
      padRight,
      padTop,
      padBottom,
      { pxPerBar: 1.05, minWidth: 1, maxWidth: 3 },
      spikeThreshold,
    )
  }, [emotesItem, activityAxis, width, height, padLeft, padRight, padTop, padBottom, expandProgress, hasPlottedEmotes])
  const emoteGuidePathD = useMemo(() => {
    if (!emotesItem) return ''
    return linePath(
      emotesDisplayValues,
      activityAxis.max,
      width,
      height,
      padLeft,
      padRight,
      padTop,
      padBottom,
      true,
      'activity-emote',
      activityAxis.min,
    )
  }, [emotesItem, emotesDisplayValues, activityAxis, width, height, padLeft, padRight, padTop, padBottom, expandProgress, hasPlottedEmotes])
  const chatLinePathD = useMemo(() => {
    if (!chatItem) return ''
    const chatMax = scaleForSeries(chatItem)
    return linePath(
      chatDisplayValues,
      chatMax,
      width,
      height,
      padLeft,
      padRight,
      padTop,
      padBottom,
      true,
      'activity-chat',
      0,
    )
  }, [chatItem, chatDisplayValues, scaleForSeries, width, height, padLeft, padRight, padTop, padBottom, expandProgress, hasPlottedEmotes])
  const chatWhisperBarRects = useMemo(() => {
    if (!chatItem) return []
    const chatMax = scaleForSeries(chatItem)
    return activityBarRects(
      chatItem.values,
      chatMax,
      0,
      'activity-chat',
      width,
      height,
      padLeft,
      padRight,
      padTop,
      padBottom,
      { pxPerBar: 1.35, minWidth: 1, maxWidth: 2 },
    )
  }, [chatItem, scaleForSeries, width, height, padLeft, padRight, padTop, padBottom, expandProgress, hasPlottedEmotes])
  const emoteSpikeIdxs = useMemo(() => {
    if (!showSpikes || !emotesItem) return []
    return emoteSpikeIndices(emotesItem.values, 0.3, 28)
  }, [showSpikes, emotesItem])
  const chatSpikeIdxs = useMemo(() => {
    if (!showSpikes || !chatItem) return []
    return emoteSpikeIndices(chatItem.values, 0.38, 12)
  }, [showSpikes, chatItem])
  const syncChatFrontierIdx = useMemo(() => {
    if (!syncing || !rollups.length) return -1
    let last = -1
    rollups.forEach((point, idx) => {
      if (!point.missing && (point.chatCount ?? 0) > 0) last = idx
    })
    return last
  }, [syncing, rollups])
  const syncChatFrontierX = useMemo(() => {
    if (syncChatFrontierIdx < 0 || rollups.length === 0) return null
    const n = rollups.length
    return n === 1 ? padLeft : padLeft + (syncChatFrontierIdx / (n - 1)) * (width - padLeft - padRight)
  }, [syncChatFrontierIdx, rollups.length, padLeft, padRight, width])
  const syncOverlayBand = useMemo(() => {
    const viewerBand = plotBandForZone(height, padTop, padBottom, 'viewer')
    return {
      bandTop: viewerBand.activityTop,
      bandBottom: height - padBottom,
      bandHeight: viewerBand.activityHeight,
    }
  }, [height, padTop, padBottom, expandProgress, hasPlottedEmotes])
  const plottedEmoteKeys = useMemo(() => perEmoteSeries.map(item => item.key), [perEmoteSeries])
  const handleSpikeSelect = useCallback((idx: number, event: { stopPropagation: () => void }) => {
    event.stopPropagation()
    const rollup = rollups[idx]
    if (!rollup) return
    if (selectedRollup?.minuteTs === rollup.minuteTs) {
      onSelectRollup?.(null)
      return
    }
    onSelectRollup?.(rollup)
  }, [rollups, onSelectRollup, selectedRollup])
  const perEmoteOverlays = useMemo(() => perEmoteSeries.map(item => ({
    key: item.key,
    color: item.color,
    linePathD: linePath(
      item.values,
      perEmotePlotAxis.max,
      width,
      height,
      padLeft,
      padRight,
      padTop,
      padBottom,
      true,
      'activity-emote-trace',
      perEmotePlotAxis.min,
    ),
  })), [perEmoteSeries, perEmotePlotAxis, width, height, padLeft, padRight, padTop, padBottom, expandProgress, hasPlottedEmotes])
  const viewerAreaPathD = useMemo(() => {
    if (!viewersItem) return ''
    return areaPath(
      viewerDisplayValues,
      viewerAxis.max,
      width,
      height,
      padLeft,
      padRight,
      padTop,
      padBottom,
      'viewer',
      viewerAxis.min,
    )
  }, [viewersItem, viewerDisplayValues, viewerAxis, width, height, padLeft, padRight, padTop, padBottom])
  const viewerTailStart = useMemo(() => {
    if (!needsViewerResync || !viewersItem) return -1
    return findEstimatedViewerTailStart(viewersItem.values)
  }, [needsViewerResync, viewersItem])
  const viewerLineSegments = useMemo(() => {
    if (!viewersItem) return []
    if (viewerTailStart <= 0) {
      return [{ values: viewerDisplayValues, estimated: false }]
    }
    return [
      {
        values: decimateViewerForRender(
          viewersItem.values.map((value, index) => (index < viewerTailStart ? value : null)),
        ),
        estimated: false,
      },
      {
        values: decimateViewerForRender(
          viewersItem.values.map((value, index) => (index >= viewerTailStart - 1 ? value : null)),
        ),
        estimated: true,
      },
    ]
  }, [viewersItem, viewerDisplayValues, viewerTailStart, decimateViewerForRender])
  const hasChatData = useMemo(
    () => rollups.some(point => (point.chatCount ?? 0) > 0 || minuteEmoteTotal(point) > 0),
    [rollups],
  )
  const chartGames = normalizeGameSegments(games, Math.max(rollups.length * 60, 60))
  const gameBandHeight = chartGames.length > 0 ? 16 : 0
  const gameBandTop = padTop + 1

  const hoverIndex = rollups.length === 0
    ? 0
    : Math.max(0, Math.min(rollups.length - 1, hover === null ? rollups.length - 1 : hover))
  const plotSpan = width - padLeft - padRight
  const rawHoverX = rollups.length <= 1
    ? padLeft
    : padLeft + (hoverIndex / (rollups.length - 1)) * plotSpan
  const playheadTargetX = useMemo(() => {
    if (!cursorSync.synced || cursorSync.cursorOffsetSeconds === null || rollups.length <= 1) {
      return padLeft
    }
    const firstMs = new Date(rollups[0].minuteTs).getTime()
    const lastMs = new Date(rollups[rollups.length - 1].minuteTs).getTime()
    const span = lastMs - firstMs
    if (!Number.isFinite(span) || span <= 0) return padLeft
    const startedAt = streamStartedAt
    const startMs = startedAt ? new Date(startedAt).getTime() : firstMs
    const targetMs = startMs + cursorSync.cursorOffsetSeconds * 1000
    const pct = Math.min(1, Math.max(0, (targetMs - firstMs) / span))
    return padLeft + pct * plotSpan
  }, [cursorSync.synced, cursorSync.cursorOffsetSeconds, rollups, streamStartedAt, padLeft, plotSpan])
  const smoothHoverX = useSmoothedScalar(rawHoverX, motionEnabled && !cursorSync.synced && hover !== null)
  const smoothPlayheadX = useSmoothedScalar(playheadTargetX, motionEnabled && cursorSync.synced)
  const displayHoverX = motionEnabled && !cursorSync.synced && hover !== null ? smoothHoverX : rawHoverX
  const displayPlayheadX = motionEnabled && cursorSync.synced ? smoothPlayheadX : playheadTargetX

  if (!canRenderChart) {
    return (
      <div className="pulse-chart-empty" style={{ minHeight: heightProp ?? 200, display: 'grid', placeItems: 'center', color: '#71717a', fontSize: 12, padding: 12, textAlign: 'center' }}>
        Chart minutes not available yet.
      </div>
    )
  }

  const viewersItemForRender = viewersItem
  const viewerValues = viewersItemForRender?.values.filter((v): v is number => v !== null && v > 0) ?? []
  const avgViewers = viewerValues.length > 0
    ? Math.round(viewerValues.reduce((a, b) => a + b, 0) / viewerValues.length)
    : (avgViewersFallback)
  const activeViewerAxis = viewersItemForRender ? viewerAxis : viewerPeakAxis
  const viewerScale = activeViewerAxis.max
  const viewerScaleMin = activeViewerAxis.min
  const viewerScaleSpan = Math.max(1, viewerScale - viewerScaleMin)
  const yMax = padTop
  const viewerBand = plotBandForZone(height, padTop, padBottom, 'viewer')
  const yAvg = viewerBand.bandBottom - ((avgViewers - viewerScaleMin) / viewerScaleSpan) * viewerBand.bandHeight
  const showAvgLabel = (yAvg - yMax) > 22 && (viewerBand.bandBottom - yAvg) > 22
  const hoverPoint = rollups[hoverIndex]

  const chartBody = (
    <div className="overflow-hidden rounded">
      <svg
        viewBox={`0 0 ${width} ${height}`}
        role="img"
        aria-label="Analytics timeline chart"
        className={variant === 'compact' ? 'w-full cursor-crosshair select-none' : 'h-[360px] min-h-[320px] w-full cursor-crosshair select-none sm:h-[min(420px,52vh)]'}
        style={variant === 'compact' && heightProp ? { height: heightProp, minHeight: heightProp } : undefined}
      >
        <defs>
          <linearGradient id="viewerAreaGradient" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={CHART_THEME.viewer.color} stopOpacity={CHART_THEME.viewer.fillTop} />
            <stop offset="100%" stopColor={CHART_THEME.viewer.color} stopOpacity={CHART_THEME.viewer.fillBottom} />
          </linearGradient>
          <clipPath id="analyticsPlotClip">
            <rect x={padLeft} y={padTop} width={width - padLeft - padRight} height={height - padTop - padBottom} />
          </clipPath>
        </defs>

        {/* Horizontal guide lines */}
        <line x1={padLeft} x2={width - padRight} y1={padTop} y2={padTop} stroke={hexToRgba(CHART_THEME.viewer.color, CHART_THEME.viewer.guide)} strokeWidth="1" strokeDasharray="4 4" />
        {showAvgLabel && (
          <line x1={padLeft} x2={width - padRight} y1={yAvg} y2={yAvg} stroke={hexToRgba(CHART_THEME.viewer.color, CHART_THEME.viewer.guide)} strokeWidth="1" strokeDasharray="4 4" />
        )}
        <line x1={padLeft} x2={width - padRight} y1={height - padBottom} y2={height - padBottom} stroke="rgba(255,255,255,.08)" strokeWidth="1" />

        {/* Left Y-Axis labels */}
        <g>
          {/* MAX Label */}
          <text x={padLeft - 12} y={padTop - 4} textAnchor="end" className="fill-cyan-400 text-[10px] font-black uppercase">MAX</text>
          <text x={padLeft - 12} y={padTop + 10} textAnchor="end" className="fill-cyan-400 text-sm font-black">{count(viewerScale)}</text>

          {/* AVG Label */}
          {showAvgLabel && (
            <>
              <text x={padLeft - 12} y={yAvg - 4} textAnchor="end" className="fill-cyan-400/80 text-[10px] font-black uppercase">AVG</text>
              <text x={padLeft - 12} y={yAvg + 10} textAnchor="end" className="fill-cyan-400/80 text-sm font-black">{count(avgViewers)}</text>
            </>
          )}
          {viewerScaleMin > 0 && (
            <>
              <text x={padLeft - 12} y={viewerBand.bandBottom - 14} textAnchor="end" className="fill-cyan-400/70 text-[10px] font-black uppercase">MIN</text>
              <text x={padLeft - 12} y={viewerBand.bandBottom} textAnchor="end" className="fill-cyan-400/70 text-sm font-black">{count(viewerScaleMin)}</text>
            </>
          )}
        </g>

        <g className="sc-chart-plot" clipPath="url(#analyticsPlotClip)">
        {/* Activity strip background */}
        <rect
          x={padLeft}
          y={activityLayout.activityTop}
          width={width - padLeft - padRight}
          height={activityLayout.activityHeight}
          fill="rgba(255,255,255,0.025)"
        />
        <line
          x1={padLeft}
          x2={width - padRight}
          y1={activityLayout.activityTop}
          y2={activityLayout.activityTop}
          stroke="rgba(255,255,255,0.1)"
          strokeWidth="1"
        />
        <line
          x1={padLeft}
          x2={width - padRight}
          y1={activityLayout.chatSplit}
          y2={activityLayout.chatSplit}
          stroke="rgba(255,255,255,0.06)"
          strokeWidth="1"
          strokeDasharray="3 4"
        />
        <line
          x1={padLeft}
          x2={width - padRight}
          y1={activityLayout.traceSplit}
          y2={activityLayout.traceSplit}
          stroke="rgba(255,255,255,0.06)"
          strokeWidth="1"
          strokeDasharray="3 4"
        />

        {/* Thin per-emote trace rail (above aggregate bars). */}
        {perEmoteSeries.length > 0 ? (
          <rect
            x={padLeft}
            y={activityLayout.chatSplit}
            width={width - padLeft - padRight}
            height={Math.max(1, activityLayout.traceSplit - activityLayout.chatSplit)}
            fill="rgba(255,255,255,0.02)"
          />
        ) : null}

        {/* Viewer area fill */}
        {viewerAreaPathD ? (
          <path
            d={viewerAreaPathD}
            fill="url(#viewerAreaGradient)"
            opacity={seriesFocusOpacity('viewers', expandProgress >= 0.5 ? 0.55 : 1)}
          />
        ) : null}

        {/* Viewer line (split when tail is estimated/incomplete) */}
        {viewersItem && viewerLineSegments.map((segment, segmentIndex) => {
          const pathD = linePath(segment.values, viewerAxis.max, width, height, padLeft, padRight, padTop, padBottom, false, 'viewer', viewerAxis.min)
          if (!pathD) return null
          return (
            <path
              key={`viewer-${segmentIndex}`}
              d={pathD}
              fill="none"
              stroke={CHART_THEME.viewer.color}
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={segment.estimated ? 2 : 2.5}
              strokeDasharray={segment.estimated ? '7 6' : undefined}
              opacity={seriesFocusOpacity('viewers', segment.estimated ? 0.4 : CHART_THEME.viewer.line)}
            />
          )
        })}

        {/* Emote max guide */}
        <line
          x1={padLeft}
          x2={width - padRight}
          y1={emoteBandMaxY}
          y2={emoteBandMaxY}
          stroke={hexToRgba(CHART_THEME.emote.color, CHART_THEME.emote.guide)}
          strokeWidth="1"
          strokeDasharray="4 5"
        />

        {/* Dense emote bar histogram */}
        {emoteBarRects.map(bar => (
          <rect
            key={bar.key}
            x={bar.x}
            y={bar.y}
            width={bar.width}
            height={bar.height}
            rx={0}
            fill={bar.isSpike ? CHART_THEME.spike.color : CHART_THEME.emote.color}
            opacity={
              seriesFocusOpacity(
                'emotes',
                (bar.isSpike
                  ? CHART_THEME.emote.barSpike
                  : bar.hasValue
                    ? CHART_THEME.emote.bar
                    : CHART_THEME.emote.barBaseline) * activityVisualBoost,
              )
            }
          />
        ))}

        {/* Optional thin emote peak guide */}
        {emoteGuidePathD ? (
          <path
            d={emoteGuidePathD}
            fill="none"
            stroke={CHART_THEME.emote.color}
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={expandProgress >= 0.5 ? 2 : 1}
            opacity={seriesFocusOpacity('emotes', CHART_THEME.emote.line * activityVisualBoost)}
          />
        ) : null}

        {/* Chat whisper bars behind line */}
        {chatWhisperBarRects.map(bar => (
          <rect
            key={bar.key}
            x={bar.x}
            y={bar.y}
            width={bar.width}
            height={bar.height}
            rx={0}
            fill={CHART_THEME.chat.color}
            opacity={seriesFocusOpacity('chat', bar.hasValue ? CHART_THEME.chat.whisperBar : CHART_THEME.chat.whisperBar * 0.6)}
          />
        ))}

        {/* Chat max guide */}
        {chatItem ? (
          <line
            x1={padLeft}
            x2={width - padRight}
            y1={chatBandMaxY}
            y2={chatBandMaxY}
            stroke={hexToRgba(CHART_THEME.chat.color, CHART_THEME.chat.guide)}
            strokeWidth="1"
            strokeDasharray="4 5"
          />
        ) : null}

        {/* Chat line */}
        {chatLinePathD ? (
          <path
            d={chatLinePathD}
            fill="none"
            stroke={CHART_THEME.chat.line}
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={expandProgress >= 0.5 ? 2.5 : 1.5}
            opacity={seriesFocusOpacity('chat', CHART_THEME.chat.lineOpacity * activityVisualBoost)}
          />
        ) : null}

        {/* Selected emote traces — thin rail directly above aggregate bars. */}
        {perEmoteOverlays.map(overlay => (
          <g key={overlay.key}>
            {overlay.linePathD ? (
              <path
                d={overlay.linePathD}
                fill="none"
                stroke={overlay.color}
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="2.25"
                className="sc-emote-plot-line"
                opacity={seriesFocusOpacity(overlay.key, 0.95 * activityVisualBoost)}
              />
            ) : null}
          </g>
        ))}

        {syncing && syncChatFrontierX != null ? (
          <>
            <rect
              x={syncChatFrontierX}
              y={syncOverlayBand.bandTop}
              width={Math.max(0, width - padRight - syncChatFrontierX)}
              height={syncOverlayBand.bandHeight}
              fill="rgba(9,9,11,0.35)"
            />
            <line
              x1={syncChatFrontierX}
              x2={syncChatFrontierX}
              y1={syncOverlayBand.bandTop}
              y2={syncOverlayBand.bandBottom}
              stroke="rgba(34,211,238,0.85)"
              strokeWidth="1.5"
              strokeDasharray="4 3"
              className="animate-pulse"
            />
          </>
        ) : null}

        {/* Viewer data point dots */}
        {showDots && viewersItem && viewerDisplayValues.map((val, idx) => {
          if (val === null) return null
          const n = viewerDisplayValues.length
          const step = Math.max(1, Math.floor(n / 60))
          if (idx % step !== 0 && idx !== n - 1 && idx !== 0) return null
          const cx = n === 1 ? padLeft : padLeft + (idx / (n - 1)) * (width - padLeft - padRight)
          const cy = plotY(val, viewerAxis.max, height, padTop, padBottom, 'viewer', viewerAxis.min)
          const rawN = viewersItem.values.length
          const displayTailStart = viewerTailStart > 0 && rawN > 1 && n > 1
            ? Math.floor((viewerTailStart / (rawN - 1)) * (n - 1))
            : -1
          const estimated = displayTailStart > 0 && idx >= displayTailStart
          return (
            <circle
              key={`viewer-dot-${idx}`}
              cx={cx}
              cy={cy}
              r={hover === idx ? 5 : 3}
              fill={CHART_THEME.viewer.color}
              stroke={CHART_THEME.background}
              strokeWidth="1.5"
              opacity={seriesFocusOpacity('viewers', hover === idx ? CHART_THEME.viewer.line : estimated ? 0.35 : CHART_THEME.viewer.dot)}
              className="transition-all duration-100"
            />
          )
        })}
        </g>

        {syncing ? (
          <>
            <text
              x={width - padRight + 2}
              y={padTop + 12}
              textAnchor="start"
              className="fill-cyan-300/90 text-[8px] font-black uppercase"
            >
              Viewers
            </text>
            {perEmoteSeries.length > 0 ? (
              <text
                x={width - padRight + 2}
                y={activityLabelYs.trace}
                textAnchor="start"
                className="fill-zinc-300/80 text-[8px] font-black uppercase"
              >
                Selected max {count(selectedEmoteScaleMax)}
              </text>
            ) : null}
            <text
              x={width - padRight + 2}
              y={activityLabelYs.chat}
              textAnchor="start"
              className="fill-violet-300/80 text-[8px] font-black uppercase"
            >
              Chat (syncing)
            </text>
            <text
              x={width - padRight + 2}
              y={activityLabelYs.bars}
              textAnchor="start"
              className="fill-emerald-300/70 text-[8px] font-black uppercase"
            >
              Emotes (syncing)
            </text>
          </>
        ) : (
          <>
            <text
              x={width - padRight + 2}
              y={emoteBandMaxY - 3}
              textAnchor="start"
              className="fill-emerald-300/80 text-[8px] font-black uppercase"
            >
              Emote peak {count(activityScaleMax)}
            </text>
            {chatItem ? (
              <text
                x={width - padRight + 2}
                y={chatBandMaxY - 3}
                textAnchor="start"
                className="fill-violet-300/80 text-[8px] font-black uppercase"
              >
                Chat max {count(scaleForSeries(chatItem))}
              </text>
            ) : null}
            <text
              x={width - padRight + 2}
              y={activityLabelYs.bars - 3}
              className="fill-emerald-400/50 text-[8px] font-black uppercase"
            >
              Emotes
            </text>
            {perEmoteSeries.length > 0 ? (
              <text
                x={width - padRight + 2}
                y={activityLabelYs.trace - 3}
                textAnchor="start"
                className="fill-zinc-300/80 text-[8px] font-black uppercase"
              >
                Selected max {count(selectedEmoteScaleMax)}
              </text>
            ) : null}
            <text
              x={width - padRight + 2}
              y={activityLabelYs.chat - 3}
              className="fill-violet-400/50 text-[8px] font-black uppercase"
            >
              Chat
            </text>
          </>
        )}

        {/* Draw X-axis ticks and time labels */}
        {(() => {
          const numTicks = Math.min(8, rollups.length)
          if (numTicks <= 1) return null
          const tickIndices = []
          for (let i = 0; i < numTicks; i++) {
            tickIndices.push(Math.round((i / (numTicks - 1)) * (rollups.length - 1)))
          }
          return tickIndices.map(idx => {
            const item = rollups[idx]
            if (!item) return null
            const x = padLeft + (idx / (rollups.length - 1)) * (width - padLeft - padRight)
            return (
              <g key={idx} className="opacity-60">
                <line x1={x} x2={x} y1={height - padBottom} y2={height - padBottom + 6} stroke="rgba(255,255,255,.3)" strokeWidth="1" />
                <text x={x} y={height - padBottom + 20} textAnchor="middle" className="fill-zinc-500 text-[10px] font-black">{vodClock(item.minuteTs, streamStartedAt)}</text>
              </g>
            )
          })
        })()}

        {/* Draw a vertical line for the selected rollup */}
        {selectedRollup && (() => {
          const selectedIdx = rollups.findIndex(r => r.minuteTs === selectedRollup.minuteTs)
          if (selectedIdx === -1) return null
          const selX = rollups.length <= 1
            ? padLeft
            : padLeft + (selectedIdx / (rollups.length - 1)) * (width - padLeft - padRight)
          if (!Number.isFinite(selX)) return null
          return (
            <line
              x1={selX}
              x2={selX}
              y1={padTop}
              y2={height - padBottom}
              stroke={CHART_THEME.moment.selected}
              strokeWidth="2.5"
              strokeDasharray="4 3"
            />
          )
        })()}

        {/* Hover preview from moment list (lighter than click selection) */}
        {previewRollup && previewRollup.minuteTs !== selectedRollup?.minuteTs && (() => {
          const previewIdx = rollups.findIndex(r => r.minuteTs === previewRollup.minuteTs)
          if (previewIdx === -1) return null
          const previewX = rollups.length <= 1
            ? padLeft
            : padLeft + (previewIdx / (rollups.length - 1)) * (width - padLeft - padRight)
          if (!Number.isFinite(previewX)) return null
          return (
            <line
              x1={previewX}
              x2={previewX}
              y1={padTop}
              y2={height - padBottom}
              stroke={CHART_THEME.moment.preview}
              strokeWidth="1.75"
              strokeDasharray="3 5"
            />
          )
        })()}

        {cursorSync.synced && cursorSync.cursorOffsetSeconds !== null && rollups.length > 1 ? (
          <line
            x1={displayPlayheadX}
            x2={displayPlayheadX}
            y1={padTop}
            y2={height - padBottom}
            stroke="#34d399"
            strokeWidth="2"
            className="sc-playhead-line"
          />
        ) : null}

        <GameSegmentOverlay
          segments={chartGames}
          rollups={rollups}
          streamStartedAt={streamStartedAt}
          padLeft={padLeft}
          plotWidth={plotWidthPx}
          gameBandTop={gameBandTop}
          gameBandHeight={gameBandHeight}
        />

        {!cursorSync.synced ? (
          <line
            x1={displayHoverX}
            x2={displayHoverX}
            y1={padTop}
            y2={height - padBottom}
            stroke="rgba(255,255,255,.28)"
            strokeWidth="1"
            className="sc-hover-line"
          />
        ) : null}

        {/* Transparent overlay rect for reliable mouse interaction */}
        <rect
          x={padLeft}
          y={padTop}
          width={width - padLeft - padRight}
          height={height - padTop - padBottom}
          fill="transparent"
          style={{ cursor: 'crosshair' }}
          onMouseMove={event => {
            if (rollups.length === 0) return
            const rect = event.currentTarget.getBoundingClientRect()
            if (rect.width <= 0) return
            const clientXRelative = event.clientX - rect.left
            const pct = Math.min(1, Math.max(0, clientXRelative / rect.width))
            commitHover(Math.round(pct * (rollups.length - 1)))
          }}
          onMouseLeave={() => {
            if (hoverRafRef.current != null) {
              cancelAnimationFrame(hoverRafRef.current)
              hoverRafRef.current = null
            }
            hoverIndexRef.current = null
            setHover(null)
          }}
          onPointerDown={event => {
            event.stopPropagation()
          }}
          onClick={event => {
            if (rollups.length === 0) return
            const rect = event.currentTarget.getBoundingClientRect()
            if (rect.width <= 0) return
            const clientXRelative = event.clientX - rect.left
            const pct = Math.min(1, Math.max(0, clientXRelative / rect.width))
            const idx = Math.round(pct * (rollups.length - 1))
            const rollup = rollups[idx]
            if (!rollup) return
            if (selectedRollup?.minuteTs === rollup.minuteTs) {
              onSelectRollup?.(null)
              return
            }
            onSelectRollup?.(rollup)
          }}
        />

        {showSpikes ? emoteSpikeIdxs.map(idx => {
          const value = emotesItem?.values[idx] ?? null
          if (value === null || value <= 0) return null
          const spikeKey = `emote-${idx}`
          const n = rollups.length
          const cx = n === 1 ? padLeft : padLeft + (idx / (n - 1)) * (width - padLeft - padRight)
          const cy = plotY(value, activityAxis.max, height, padTop, padBottom, 'activity-emote', activityAxis.min)
          const isHovered = hoveredSpikeKey === spikeKey
          const radius = isHovered ? CHART_THEME.spike.hoverRadius : CHART_THEME.spike.dotRadius
          return (
            <circle
              key={spikeKey}
              cx={cx}
              cy={cy}
              r={radius}
              fill={CHART_THEME.spike.color}
              stroke="#fafafa"
              strokeWidth={isHovered ? 2 : 1.5}
              className="cursor-pointer"
              style={{ pointerEvents: 'all' }}
              opacity={seriesFocusOpacity('emotes', CHART_THEME.spike.opacity)}
              onMouseEnter={() => setHoveredSpikeKey(spikeKey)}
              onMouseLeave={() => setHoveredSpikeKey(current => current === spikeKey ? null : current)}
              onClick={(event) => handleSpikeSelect(idx, event)}
            />
          )
        }) : null}

        {showSpikes ? chatSpikeIdxs.map(idx => {
          const value = chatItem?.values[idx] ?? null
          if (value === null || value <= 0 || !chatItem) return null
          const spikeKey = `chat-${idx}`
          const n = rollups.length
          const chatMax = scaleForSeries(chatItem)
          const cx = n === 1 ? padLeft : padLeft + (idx / (n - 1)) * (width - padLeft - padRight)
          const cy = plotY(value, chatMax, height, padTop, padBottom, 'activity-chat', 0)
          const isHovered = hoveredSpikeKey === spikeKey
          const radius = isHovered ? CHART_THEME.spike.hoverRadius : CHART_THEME.spike.dotRadius
          return (
            <circle
              key={spikeKey}
              cx={cx}
              cy={cy}
              r={radius}
              fill={CHART_THEME.spike.color}
              stroke="#fafafa"
              strokeWidth={isHovered ? 2 : 1.5}
              className="cursor-pointer"
              style={{ pointerEvents: 'all' }}
              opacity={seriesFocusOpacity('chat', CHART_THEME.spike.opacity)}
              onMouseEnter={() => setHoveredSpikeKey(spikeKey)}
              onMouseLeave={() => setHoveredSpikeKey(current => current === spikeKey ? null : current)}
              onClick={(event) => handleSpikeSelect(idx, event)}
            />
          )
        }) : null}
      </svg>
    </div>
  )

  if (chromeless) return chartBody

  return (
    <div className="sc-chart-root rounded border border-white/10 bg-[#0d0d12] p-3" data-variant={variant}>
      {variant === 'compact' ? (
        <div className="mb-2 flex items-center justify-between gap-2">
          <ChartHoverReadout
            minuteTs={hoverPoint?.minuteTs}
            streamStartedAt={streamStartedAt}
            viewers={hoverPoint ? viewerValue(hoverPoint) : null}
            chatCount={hoverPoint?.chatCount}
            emoteTotal={hoverPoint ? minuteEmoteTotal(hoverPoint) : null}
          />
          <div className="inline-flex shrink-0 items-center gap-1 rounded border border-white/10 bg-white/[0.03] p-0.5">
            <button
              type="button"
              onClick={() => setShowSpikes(value => !value)}
              aria-pressed={showSpikes}
              className={`rounded px-2 py-1 text-[10px] font-black uppercase transition ${showSpikes ? 'bg-amber-400/10 text-amber-200' : 'text-zinc-500 hover:bg-white/[0.06] hover:text-zinc-300'}`}
            >
              Moments
            </button>
            <button
              type="button"
              onClick={() => setActivityExpanded(value => !value)}
              aria-pressed={activityExpanded}
              className={`rounded px-2 py-1 text-[10px] font-black uppercase transition ${activityExpanded ? 'bg-violet-400/10 text-violet-200' : 'text-zinc-500 hover:bg-white/[0.06] hover:text-zinc-300'}`}
            >
              {activityExpanded ? 'Reset' : 'Expand'}
            </button>
          </div>
        </div>
      ) : null}
      {chartBody}
    </div>
  )
}


export const PulseMultiSignalChartInner = memo(PulseMultiSignalChartInnerImpl)
