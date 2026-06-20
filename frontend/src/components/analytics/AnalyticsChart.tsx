import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'

import type { AnalyticsMinuteRollup, AnalyticsStreamDetail, GameSegment } from '../../api.ts'
import { classifyLiveEmptyState } from '../../utils/liveEmptyState.ts'
import { computeChartCursorSync } from '../../utils/chartCursorSync.ts'
import { resolveEmoteImageUrl } from '../../utils/emoteImageUrl.ts'
import { usePlayheadStore } from '../../stores/playheadStore.ts'
import { CoreMinuteChartsNotice } from '../OptionalServicesPanel.tsx'
import LiveCollectionWarmup from './LiveCollectionWarmup.tsx'
import { CHART_THEME, hexToRgba, legendDotStyle } from './chartTheme.ts'
import {
  analyzeViewerCoverage,
  chartViewerValue,
  clock,
  count,
  decimateSeriesForRender,
  formatVodClock,
  minuteEmoteTotal,
  rollupHasMinuteData,
  rollupsHaveViewerData,
  rollingMedianWindow,
  viewerChartSmoothWindow,
  viewerSourceLabel,
  seriesMax,
  viewerValue,
} from './chartRollupUtils.ts'

function getEmoteImageUrl(emote: { provider?: string; id?: string; imageUrl?: string }) {
  const url = resolveEmoteImageUrl({
    provider: emote.provider,
    id: emote.id,
    imageUrl: emote.imageUrl,
    scale: '1x',
  })
  return url || undefined
}

type Series = {
  key: string
  label: string
  color: string
  values: Array<number | null>
  max: number
  dashed?: boolean
}

export type AnalyticsViewMode = 'overview' | 'emotes' | 'spikes'
export type RightPanelTab = 'moments' | 'emotes' | 'clips' | 'sync'

const analyticsViewModes: Array<{ id: AnalyticsViewMode; label: string }> = [
  { id: 'overview', label: 'Overview' },
  { id: 'emotes', label: 'Emotes' },
  { id: 'spikes', label: 'Spikes' },
]

function normalizeGameSegments(games: GameSegment[], rollupCount: number): GameSegment[] {
  if (!games.length || rollupCount <= 0) return []

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

  const totalDurationSec = rollupCount * 60
  if (!Number.isFinite(totalDurationSec) || totalDurationSec <= 0) return []
  const each = Math.max(60, Math.floor(totalDurationSec / cleaned.length))
  let offset = 0
  return cleaned.map((game, index) => {
    const durationSeconds = index === cleaned.length - 1
      ? Math.max(60, totalDurationSec - offset)
      : each
    const segment = { ...game, offsetSeconds: offset, durationSeconds }
    offset += durationSeconds
    return segment
  })
}

function rollupsForChart(rollups: AnalyticsMinuteRollup[], isLive: boolean) {
  if (!rollups.length) return rollups
  const dataIndices = rollups
    .map((point, index) => rollupHasMinuteData(point) ? index : -1)
    .filter(index => index >= 0)
  if (!dataIndices.length) return rollups

  const first = dataIndices[0]
  const last = isLive ? rollups.length - 1 : dataIndices[dataIndices.length - 1]
  const pad = isLive ? 5 : 3
  const start = Math.max(0, first - pad)
  const end = Math.min(rollups.length, last + pad + 1)
  return rollups.slice(start, end)
}

function buildSeries(
  rollups: AnalyticsMinuteRollup[],
  selected: Set<string>,
  peakViewersFallback = 0,
  avgViewersFallback = 0,
  useViewerFallback = false,
): Series[] {
  const viewerBaseline = avgViewersFallback > 0 ? avgViewersFallback : peakViewersFallback
  const viewers = rollups.map(point => {
    if (point.missing) return null
    const value = chartViewerValue(point)
    if (value > 0) return value
    return useViewerFallback && viewerBaseline > 0 ? viewerBaseline : 0
  })
  const chat = rollups.map(point => point.missing ? null : point.chatCount)

  // Raw per-minute emote counts (no smoothing — preserves spikes).
  const emotesRaw = rollups.map(point => point.missing ? null : minuteEmoteTotal(point))
  const emotesMax = seriesMax(emotesRaw)

  const viewersMax = seriesMax(viewers)
  const out: Series[] = [
    { key: 'viewers', label: 'Viewers', color: '#22d3ee', values: viewers, max: viewersMax > 0 ? viewersMax : Math.max(0, peakViewersFallback) },
    { key: 'chat', label: 'Chat/min', color: '#a78bfa', values: chat, max: seriesMax(chat) },
    { key: 'emotes', label: 'Emotes/min', color: '#34d399', values: emotesRaw, max: emotesMax },
  ]
  const palette = [...CHART_THEME.perEmotePalette]
  Array.from(selected).slice(0, 5).forEach((key, index) => {
    const rawValues = rollups.map(point => point.missing ? null : point.emotes?.[key] ?? 0)
    const maxVal = seriesMax(rawValues)
    out.push({ key, label: emoteLabel(key), color: palette[index % palette.length], values: rawValues, max: maxVal, dashed: true })
  })
  return out
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

function emoteLabel(key: string) {
  const parts = key.split(':')
  if (parts.length >= 3) return parts.slice(2).join(':')
  return key
}

const ACTIVITY_ZONE_FRACTION = 0.36
const ACTIVITY_ZONE_GAP = 8
const ACTIVITY_CHAT_LINE_FRACTION = 0.42
const ACTIVITY_EMOTE_BARS_FRACTION = 0.58
const CHART_VIEWBOX_HEIGHT = 400

type PlotZone = 'viewer' | 'activity-chat' | 'activity-emote'

function plotBandForZone(
  height: number,
  padTop: number,
  padBottom: number,
  zone: PlotZone,
) {
  const fullPlotHeight = height - padTop - padBottom
  const activityHeight = fullPlotHeight * ACTIVITY_ZONE_FRACTION
  const viewerHeight = fullPlotHeight - activityHeight - ACTIVITY_ZONE_GAP
  const activityTop = height - padBottom - activityHeight
  const chatSplit = activityTop + activityHeight * ACTIVITY_CHAT_LINE_FRACTION

  switch (zone) {
    case 'viewer':
      return { bandTop: padTop, bandBottom: padTop + viewerHeight, bandHeight: viewerHeight, activityTop, activityHeight, chatSplit }
    case 'activity-chat':
      return {
        bandTop: activityTop,
        bandBottom: chatSplit,
        bandHeight: activityHeight * ACTIVITY_CHAT_LINE_FRACTION,
        activityTop,
        activityHeight,
        chatSplit,
      }
    case 'activity-emote':
      return {
        bandTop: chatSplit,
        bandBottom: height - padBottom,
        bandHeight: activityHeight * ACTIVITY_EMOTE_BARS_FRACTION,
        activityTop,
        activityHeight,
        chatSplit,
      }
    default:
      return { bandTop: padTop, bandBottom: height - padBottom, bandHeight: fullPlotHeight, activityTop, activityHeight, chatSplit }
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
  const { bandTop, bandBottom, bandHeight } = plotBandForZone(height, padTop, padBottom, zone)
  const span = Math.max(1, max - rangeMin)
  const y = bandBottom - ((Math.max(rangeMin, value) - rangeMin) / span) * bandHeight
  return Math.max(bandTop, Math.min(bandBottom, y))
}

type ActivityAxis = { min: number; max: number; mode: 'peak' | 'fit' }

function activityAxisBounds(series: Series[], fitToVisible = true, options: { includeAggregateEmotes?: boolean } = {}): ActivityAxis {
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

function AnalyticsChart({
  detail,
  selectedEmotes,
  onSelectEmote,
  selectedRollup,
  onSelectRollup,
  syncing = false,
  syncError = null,
  syncNotice = null,
  onSync = () => {},
  onRefresh = () => {},
  refreshing = false,
  loading = false,
  games = [],
  canSync = false,
  isLive = false,
  notInAnalyticsDb = false,
  coreMinuteChartsBlocked = false,
  liveHasRichHistory = false,
  chatOnlySyncAvailable = false,
  onChatOnlySync,
  syncCtaLabel: syncCtaLabelText,
  syncViewerStatus,
  viewMode,
  onViewModeChange,
}: {
  detail?: AnalyticsStreamDetail;
  selectedEmotes: Set<string>;
  onSelectEmote: (key: string) => void;
  selectedRollup: AnalyticsMinuteRollup | null;
  onSelectRollup: (rollup: AnalyticsMinuteRollup | null) => void;
  syncing?: boolean;
  syncError?: string | null;
  syncNotice?: string | null;
  onSync?: () => void;
  onRefresh?: () => void;
  refreshing?: boolean;
  loading?: boolean;
  games?: GameSegment[];
  canSync?: boolean;
  isLive?: boolean;
  notInAnalyticsDb?: boolean;
  coreMinuteChartsBlocked?: boolean;
  liveHasRichHistory?: boolean;
  chatOnlySyncAvailable?: boolean;
  onChatOnlySync?: () => void;
  /** Canonical sync CTA label (Req 4.1) shared with header/right-rail placements. */
  syncCtaLabel?: string;
  syncViewerStatus?: string;
  viewMode: AnalyticsViewMode;
  onViewModeChange: (mode: AnalyticsViewMode) => void;
}) {
  const [hover, setHover] = useState<number | null>(null)
  const hoverIndexRef = useRef<number | null>(null)
  const hoverRafRef = useRef<number | null>(null)
  const [expandedScale, setExpandedScale] = useState(true)
  const [showDots, setShowDots] = useState(false)
  // Same-page playhead sync (Req 22.1, 22.3): when a VOD player is mounted on
  // the same page for THIS stream and is actively playing, the chart shows a
  // playback cursor tracking the shared playhead. When no player is present,
  // the stream id does not match, or the player is inactive, sync is disabled
  // and the chart uses its standard hover cursor.
  const playheadStreamId = usePlayheadStore(s => s.streamId)
  const playheadOffsetSeconds = usePlayheadStore(s => s.offsetSeconds)
  const playheadPlaying = usePlayheadStore(s => s.isPlaying)
  const cursorSync = computeChartCursorSync({
    chartStreamId: detail?.stream?.streamId ?? null,
    playhead: { streamId: playheadStreamId, isPlaying: playheadPlaying, offsetSeconds: playheadOffsetSeconds },
  })
  const [showSpikes, setShowSpikes] = useState(false)
  const [focusedSeriesKey, setFocusedSeriesKey] = useState<string | null>(null)
  useEffect(() => {
    setShowSpikes(viewMode === 'spikes')
    if (viewMode === 'overview') setFocusedSeriesKey(null)
  }, [viewMode])
  const seriesFocusOpacity = useCallback((seriesKey: string, base: number) => {
    if (!focusedSeriesKey) return base
    return seriesKey === focusedSeriesKey ? base : base * 0.14
  }, [focusedSeriesKey])
  const toggleSeriesFocus = useCallback((seriesKey: string) => {
    setFocusedSeriesKey(current => current === seriesKey ? null : seriesKey)
  }, [])
  const allRollups = detail?.rollups ?? []
  const rollups = useMemo(() => rollupsForChart(allRollups, isLive), [allRollups, isLive])
  const isLongChart = rollups.length >= 360
  const commitHover = useCallback((index: number | null) => {
    hoverIndexRef.current = index
    if (!isLongChart) {
      setHover(index)
      return
    }
    if (hoverRafRef.current != null) return
    hoverRafRef.current = requestAnimationFrame(() => {
      hoverRafRef.current = null
      setHover(hoverIndexRef.current)
    })
  }, [isLongChart])
  useEffect(() => () => {
    if (hoverRafRef.current != null) {
      cancelAnimationFrame(hoverRafRef.current)
    }
  }, [])
  const peakViewersFallback = detail?.stream?.peakViewers ?? 0
  const avgViewersFallback = detail?.stream?.avgViewers ?? 0
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
    () => allRollups.some(rollupHasMinuteData),
    [allRollups],
  )
  const hasViewerChartData = useMemo(
    () => rollupsHaveViewerData(allRollups),
    [allRollups],
  )
  const canRenderChart = hasChartData || hasViewerChartData
  const viewerBackfillPending = syncing && (syncViewerStatus === 'pending_backfill' || syncViewerStatus === 'backfilling')
  const partialChatCoverage = !isLive && !syncing && Boolean(detail?.chatCoverage?.partial)
  const width = 1000
  const height = CHART_VIEWBOX_HEIGHT
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
    const smoothWindow = viewerChartSmoothWindow(allRollups, detail?.viewerSource)
    return rollingMedianWindow(prepared, smoothWindow)
  }, [plotWidthPx, allRollups, detail?.viewerSource])

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
  const activityAxisSeries = useMemo(
    () => series.filter(s => s.key !== 'viewers' && s.key !== 'chat'),
    [series],
  )
  const activityAxis = useMemo(
    () => activityAxisBounds(activityAxisSeries, expandedScale),
    [activityAxisSeries, expandedScale],
  )
  const activityScaleMax = activityAxis.max
  const activityScaleMin = activityAxis.min
  const selectedEmoteAxis = useMemo(
    () => activityAxisBounds(perEmoteSeries, expandedScale, { includeAggregateEmotes: false }),
    [perEmoteSeries, expandedScale],
  )
  const selectedEmoteScaleMax = selectedEmoteAxis.max
  const selectedEmoteScaleMin = selectedEmoteAxis.min
  const activityLayout = useMemo(
    () => plotBandForZone(height, padTop, padBottom, 'viewer'),
    [height, padTop, padBottom],
  )
  const emoteBandMaxY = useMemo(
    () => plotY(activityAxis.max, activityAxis.max, height, padTop, padBottom, 'activity-emote', activityAxis.min),
    [activityAxis, height, padTop, padBottom],
  )
  const viewerAxis = useMemo(
    () => viewerScaleBounds(viewersItem?.values ?? [], peakViewersFallback, expandedScale),
    [viewersItem, peakViewersFallback, expandedScale],
  )
  const viewerPeakAxis = useMemo(
    () => viewerScaleBounds(viewersItem?.values ?? [], peakViewersFallback, false),
    [viewersItem, peakViewersFallback],
  )
  const scaleForSeries = useCallback((item: Series) => {
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
  }, [chatItem, height, padTop, padBottom])
  const emotesDisplayValues = useMemo(
    () => (emotesItem ? smoothDisplayValues(emotesItem.values, expandedScale ? 3 : 5) : []),
    [emotesItem, expandedScale],
  )
  const chatDisplayValues = useMemo(
    () => (chatItem ? smoothDisplayValues(chatItem.values, expandedScale ? 3 : 5) : []),
    [chatItem, expandedScale],
  )
  const emoteBarRects = useMemo(() => {
    if (!emotesItem) return []
    const spikeThreshold = activityAxis.max * 0.72
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
  }, [emotesItem, activityAxis, width, height, padLeft, padRight, padTop, padBottom])
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
  }, [emotesItem, emotesDisplayValues, activityAxis, width, height, padLeft, padRight, padTop, padBottom])
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
  }, [chatItem, chatDisplayValues, scaleForSeries, width, height, padLeft, padRight, padTop, padBottom])
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
  }, [chatItem, scaleForSeries, width, height, padLeft, padRight, padTop, padBottom])
  const emoteSpikeIdxs = useMemo(() => {
    if (!showSpikes || !emotesItem) return []
    return emoteSpikeIndices(emotesDisplayValues, 0.3, 28)
  }, [showSpikes, emotesItem, emotesDisplayValues])
  const chatSpikeIdxs = useMemo(() => {
    if (!showSpikes || !chatItem) return []
    return emoteSpikeIndices(chatDisplayValues, 0.38, 12)
  }, [showSpikes, chatItem, chatDisplayValues])
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
  }, [height, padTop, padBottom])
  const perEmoteOverlays = useMemo(() => perEmoteSeries.map(item => ({
    key: item.key,
    color: item.color,
    areaPathD: areaPath(
      item.values,
      selectedEmoteAxis.max,
      width,
      height,
      padLeft,
      padRight,
      padTop,
      padBottom,
      'viewer',
      selectedEmoteAxis.min,
    ),
    linePathD: linePath(
      item.values,
      selectedEmoteAxis.max,
      width,
      height,
      padLeft,
      padRight,
      padTop,
      padBottom,
      true,
      'viewer',
      selectedEmoteAxis.min,
    ),
  })), [perEmoteSeries, selectedEmoteAxis, width, height, padLeft, padRight, padTop, padBottom])
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
  const chartGames = normalizeGameSegments(games, rollups.length)

  if (loading && !detail) {
    return (
      <div className="grid min-h-80 place-items-center rounded border border-white/10 bg-[#0d0d12]/50 px-4 text-center">
        <div className="text-sm font-bold text-zinc-500">Loading chart data…</div>
      </div>
    )
  }

  if (!canRenderChart && (detail?.state === 'syncing' || syncing)) {
    return (
      <div className="grid min-h-80 place-items-center rounded border border-white/10 bg-[#0d0d12]/50 backdrop-blur-md px-4 text-center">
        <div>
          <div className="text-base font-black text-zinc-100">Syncing chart data…</div>
          <div className="mt-1 text-sm font-semibold text-zinc-500 max-w-md">
            Viewer minutes appear as soon as TwitchTracker finishes. Chat and emotes fill in segment by segment.
          </div>
          <div className="mt-2 text-xs font-semibold text-zinc-600">
            Step-by-step progress is in the Sync tab on the right.
          </div>
          {syncNotice ? <div className="mt-2 text-xs font-bold text-amber-300">{syncNotice}</div> : null}
          {syncError ? <div className="mt-2 text-xs font-bold text-red-400">{syncError}</div> : null}
        </div>
      </div>
    )
  }

  if (!canRenderChart) {
    if (coreMinuteChartsBlocked) {
      return (
        <div className="grid min-h-80 place-items-center rounded border border-white/10 bg-[#0d0d12]/50 backdrop-blur-md px-4 text-center">
          <CoreMinuteChartsNotice />
        </div>
      )
    }
    if (isLive && liveHasRichHistory) {
      return (
        <div className="grid min-h-80 place-items-center rounded border border-white/10 bg-[#0d0d12]/50 backdrop-blur-md px-4 text-center">
          <div>
            <div className="text-base font-black text-zinc-100">Live collector has no minute rollups yet</div>
            <div className="mt-1 text-sm font-semibold text-zinc-500 max-w-md">
              The IRC collector is running but has not written chart minutes for this session. Past synced streams are in the left rail — pick one for full charts, or wait and refresh.
            </div>
            <button
              type="button"
              onClick={onRefresh}
              disabled={refreshing}
              className="mt-5 rounded-lg border border-white/10 bg-white/[0.05] px-5 py-2.5 text-xs font-black uppercase tracking-wider text-zinc-200 transition hover:bg-white/10 disabled:opacity-50"
            >
              {refreshing ? 'Refreshing…' : 'Refresh data'}
            </button>
          </div>
        </div>
      )
    }
    const isTwitchTracker = detail?.sources?.some(s => s.source === 'twitchtracker')
    const canShowSync = canSync || detail?.state === 'historical' || isTwitchTracker
    // Req 7.1/7.2: never contradict a "Collecting now" rail with "No recent data".
    // While the stream is live and fewer than two rollup minutes exist, show a
    // "Collecting first minutes" state with an activity indicator instead.
    const liveEmpty = classifyLiveEmptyState({
      collectingNow: isLive,
      rollupCount: rollups.filter(point => !point.missing).length,
    })
    if (liveEmpty.kind === 'collecting-first-minutes') {
      const minuteDataCount = rollups.filter(point => rollupHasMinuteData(point)).length
      return (
        <div className="grid min-h-80 place-items-center rounded border border-white/10 bg-[#0d0d12]/50 backdrop-blur-md px-4 py-8 text-center">
          <LiveCollectionWarmup
            rollupMinuteCount={minuteDataCount}
            viewerSamples={detail?.stream?.viewerSamples}
            chatMessages={detail?.stream?.chatMessages}
          />
        </div>
      )
    }
    return (
      <div className="grid min-h-80 place-items-center rounded border border-white/10 bg-[#0d0d12]/50 backdrop-blur-md px-4 text-center">
        <div>
          <div className="text-base font-black text-zinc-100">{(isTwitchTracker || canSync) ? 'Chat & Emotes Offline' : 'No recent data'}</div>
          <div className="mt-1 text-sm font-semibold text-zinc-500 max-w-md">
            {(isTwitchTracker || canSync)
              ? 'This stream has TwitchTracker averages only. Sync pulls minute-level viewers, chat, and 7TV data (large VODs can take a few minutes).'
              : 'Analytics start collecting when this channel is viewed in Streamclone.'}
          </div>
          {notInAnalyticsDb ? (
            <div className="mt-2 text-[11px] font-semibold text-zinc-600">
              Stream not in analytics DB yet — sync will create it.
            </div>
          ) : null}
          {canShowSync && (
            <div className="mt-5 flex w-full flex-col items-center gap-2">
              {chatOnlySyncAvailable && onChatOnlySync ? (
                <div className="max-w-md text-[11px] font-semibold text-zinc-500">
                  Viewer minutes are already synced — this fetches VOD chat via Twitch GQL only (no scraper profile needed).
                </div>
              ) : null}
              <button
                onClick={onSync}
                disabled={syncing}
                className="rounded-lg bg-violet-600 px-5 py-2.5 text-xs font-black uppercase tracking-wider text-white transition hover:bg-violet-500 active:scale-95 disabled:pointer-events-none disabled:opacity-50"
              >
                {syncing ? (syncCtaLabelText ?? 'Sync in progress…') : (syncCtaLabelText ?? (chatOnlySyncAvailable ? 'Sync VOD chat' : 'Sync Historical Data'))}
              </button>
              {chatOnlySyncAvailable && onChatOnlySync ? (
                <button
                  type="button"
                  onClick={onChatOnlySync}
                  disabled={syncing}
                  className="rounded-lg border border-cyan-400/30 bg-cyan-500/10 px-5 py-2 text-[10px] font-black uppercase tracking-wider text-cyan-100 transition hover:bg-cyan-500/20 disabled:opacity-50"
                >
                  Re-sync chat only
                </button>
              ) : null}
              {syncNotice && <div className="mt-2 text-xs font-bold text-amber-300">{syncNotice}</div>}
              {syncError && <div className="mt-2 text-xs font-bold text-red-400">{syncError}</div>}
            </div>
          )}
        </div>
      </div>
    )
  }

  const viewersItemForRender = viewersItem
  const viewerValues = viewersItemForRender?.values.filter((v): v is number => v !== null && v > 0) ?? []
  const avgViewers = viewerValues.length > 0
    ? Math.round(viewerValues.reduce((a, b) => a + b, 0) / viewerValues.length)
    : (detail?.stream?.avgViewers ?? 0)
  const activeViewerAxis = viewersItemForRender ? viewerAxis : viewerPeakAxis
  const viewerScale = activeViewerAxis.max
  const viewerScaleMin = activeViewerAxis.min
  const viewerScaleSpan = Math.max(1, viewerScale - viewerScaleMin)
  const yMax = padTop
  const viewerBand = plotBandForZone(height, padTop, padBottom, 'viewer')
  const yAvg = viewerBand.bandBottom - ((avgViewers - viewerScaleMin) / viewerScaleSpan) * viewerBand.bandHeight
  const showAvgLabel = (yAvg - yMax) > 22 && (viewerBand.bandBottom - yAvg) > 22

  const hoverIndex = rollups.length === 0
    ? 0
    : Math.max(0, Math.min(rollups.length - 1, hover === null ? rollups.length - 1 : hover))
  const hoverPoint = rollups[hoverIndex]
  const hoverX = rollups.length <= 1
    ? padLeft
    : padLeft + (hoverIndex / (rollups.length - 1)) * (width - padLeft - padRight)

  return (
    <div className="rounded border border-white/10 bg-[#0d0d12] p-3">
      {needsViewerResync ? (
        <div className="mb-3 rounded border border-amber-400/25 bg-amber-400/10 px-3 py-2 text-xs font-semibold text-amber-100">
          Viewer timeline is incomplete for this sync. Click <span className="font-black">Re-sync viewers</span> to pull the TwitchTracker viewer chart (fast — chat/7TV stay as-is).
        </div>
      ) : null}
      {partialChatCoverage ? (
        <div className="mb-3 rounded border border-amber-400/25 bg-amber-400/10 px-3 py-2 text-xs font-semibold text-amber-100">
          Chat only covers the first {formatVodClock((detail?.chatCoverage?.chatSpanMinutes ?? 0) * 60)} of this{' '}
          {formatVodClock((detail?.chatCoverage?.streamSpanMinutes ?? 0) * 60)} stream
          {detail?.vodId ? ` (VOD ${detail.vodId})` : ''}. Twitch may still be processing the archive — re-sync later.
        </div>
      ) : null}
      {viewerBackfillPending && hasViewerChartData ? (
        <div className="mb-3 rounded border border-cyan-400/25 bg-cyan-500/10 px-3 py-2 text-xs font-semibold text-cyan-100">
          Viewer line from {detail?.viewerSource === 'live' ? 'live collection' : viewerSourceLabel(detail?.viewerSource) || 'existing rollups'}; TwitchTracker backfill{' '}
          {syncViewerStatus === 'backfilling' ? 'running in background' : 'pending'}.
        </div>
      ) : null}
      {syncNotice ? (
        <div className="mb-3 rounded border border-amber-400/25 bg-amber-400/10 px-3 py-2 text-xs font-bold text-amber-200">{syncNotice}</div>
      ) : null}
      {syncError ? (
        <div className="mb-3 rounded border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs font-bold text-red-300">{syncError}</div>
      ) : null}
      <div className="mb-3 flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
        <div className="min-w-0 space-y-2">
          {detail?.viewerSource ? (
            <div className="inline-flex items-center rounded border border-white/10 bg-white/[0.04] px-2 py-1 text-[10px] font-bold uppercase tracking-wide text-zinc-400">
              Viewers: {viewerSourceLabel(detail.viewerSource) || detail.viewerSource}
            </div>
          ) : null}
          <div className="inline-flex rounded border border-white/10 bg-white/[0.035] p-1 text-[10px] font-black uppercase">
            {analyticsViewModes.map(mode => (
              <button
                key={mode.id}
                type="button"
                onClick={() => onViewModeChange(mode.id)}
                className={`rounded px-3 py-1.5 transition ${
                  viewMode === mode.id
                    ? 'bg-white text-zinc-950'
                    : 'text-zinc-500 hover:bg-white/10 hover:text-zinc-200'
                }`}
              >
                {mode.label}
              </button>
            ))}
          </div>
          <div className="flex max-h-24 flex-wrap gap-2 overflow-y-auto pr-1 sm:max-h-none">
          {series.map(item => {
            const parts = item.key.split(':')
            const isEmote = parts.length >= 2
            let imageUrl: string | undefined
            if (isEmote) {
              imageUrl = getEmoteImageUrl({ provider: parts[0], id: parts[1] })
            }
            const isFocused = focusedSeriesKey === item.key
            const isDimmed = focusedSeriesKey != null && !isFocused
            return (
              <button
                key={item.key}
                type="button"
                onClick={() => toggleSeriesFocus(item.key)}
                title={isFocused ? 'Click to show all series' : `Highlight ${item.label}`}
                className={`flex items-center gap-1.5 rounded border px-2 py-1 text-[11px] font-black uppercase transition ${
                  isFocused
                    ? 'border-white/35 bg-white/[0.12] text-zinc-100 ring-1 ring-white/20'
                    : isDimmed
                      ? 'border-white/5 bg-white/[0.02] text-zinc-600 opacity-45'
                      : 'border-white/10 bg-white/[0.03] text-zinc-400 hover:bg-white/[0.07] hover:text-zinc-200'
                }`}
              >
                <span className="inline-block h-2 w-2 rounded-full" style={legendDotStyle(item.color)} />
                {imageUrl && (
                  <img src={imageUrl} alt={item.label} className="h-3.5 w-3.5 object-contain inline-block align-middle" loading="lazy" />
                )}
                <span>
                  {item.label} max {count(item.max)}
                  {syncing && item.key === 'chat' && (item.max ?? 0) <= 0 ? ' · syncing' : ''}
                  {syncing && item.key === 'emotes' && (item.max ?? 0) <= 0 ? ' · syncing' : ''}
                  {item.dashed && selectedEmoteScaleMax > selectedEmoteScaleMin && item.max > 0
                    ? ` · ${Math.round(((item.max - selectedEmoteScaleMin) / (selectedEmoteScaleMax - selectedEmoteScaleMin)) * 100)}% focus`
                    : ''}
                </span>
              </button>
            )
          })}
          </div>
        </div>
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center xl:justify-end">
          <div className="text-xs font-bold text-zinc-500">{clock(hoverPoint?.minuteTs)} · viewers {count(hoverPoint ? viewerValue(hoverPoint) : null)} · chat {count(hoverPoint?.chatCount)} · emotes {count(hoverPoint ? minuteEmoteTotal(hoverPoint) : null)}</div>
          {canSync && !coreMinuteChartsBlocked && (!hasChatData || needsViewerResync) ? (
            <button
              type="button"
              onClick={onSync}
              disabled={syncing}
              className="rounded border border-violet-400/30 bg-violet-500/10 px-2.5 py-1 text-[10px] font-black uppercase text-violet-200 transition hover:bg-violet-500/20 disabled:opacity-50"
            >
              {syncing ? 'Syncing…' : needsViewerResync ? 'Re-sync viewers' : 'Sync chat/emotes'}
            </button>
          ) : null}
          <div className="flex flex-wrap items-center gap-1.5">
            <button
              type="button"
              onClick={onRefresh}
              disabled={refreshing}
              title="Reload chart and stats from server"
              className="rounded border border-white/10 bg-white/[0.04] px-2 py-1 text-[10px] font-black uppercase text-zinc-500 transition hover:text-zinc-300 disabled:opacity-50"
            >
              {refreshing ? '…' : '↻'}
            </button>
            <button
              type="button"
              onClick={() => onViewModeChange(showSpikes ? 'overview' : 'spikes')}
              title={showSpikes ? 'Hide spike markers' : 'Show spike markers'}
              className={`rounded border px-2 py-1 text-[10px] font-black uppercase transition ${showSpikes ? 'border-emerald-400/30 bg-emerald-400/10 text-emerald-200' : 'border-white/10 bg-white/[0.04] text-zinc-500 hover:text-zinc-300'}`}
            >
              Spikes
            </button>
            <button
              type="button"
              onClick={() => setShowDots(v => !v)}
              title={showDots ? 'Hide viewer dots' : 'Show viewer dots'}
              className={`rounded border px-2 py-1 text-[10px] font-black uppercase transition ${showDots ? 'border-cyan-400/30 bg-cyan-400/10 text-cyan-200' : 'border-white/10 bg-white/[0.04] text-zinc-500 hover:text-zinc-300'}`}
            >
              Dots
            </button>
            <button
              type="button"
              onClick={() => setExpandedScale(v => !v)}
              title={expandedScale
                ? `Zoomed scale: viewers ${count(viewerAxis.min)}–${count(viewerAxis.max)}, total emotes ${count(activityScaleMin)}–${count(activityScaleMax)}, selected emotes ${count(selectedEmoteScaleMin)}–${count(selectedEmoteScaleMax)}. Click for full zero-based scale.`
                : `Full scale: viewers 0–${count(viewerPeakAxis.max)}, emotes 0–${count(activityScaleMax)}. Click to zoom into the visible min–max range.`}
              aria-pressed={expandedScale}
              className={`rounded border px-2 py-1 text-[10px] font-black uppercase transition ${expandedScale ? 'border-violet-400/30 bg-violet-400/10 text-violet-200' : 'border-white/10 bg-white/[0.04] text-zinc-500 hover:text-zinc-300'}`}
            >
              {expandedScale ? 'Zoom' : 'Full'}
            </button>
          </div>
        </div>
      </div>
      <div className="overflow-hidden rounded">
      <svg
        viewBox={`0 0 ${width} ${height}`}
        role="img"
        aria-label="Analytics timeline chart"
        className="h-[360px] min-h-[320px] w-full cursor-crosshair select-none sm:h-[min(420px,52vh)]"
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

        <g clipPath="url(#analyticsPlotClip)">
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

        {/* Viewer area fill */}
        {viewerAreaPathD ? (
          <path
            d={viewerAreaPathD}
            fill="url(#viewerAreaGradient)"
            opacity={seriesFocusOpacity('viewers', 1)}
          />
        ) : null}

        {/* Per-emote overlays in viewer zone */}
        {perEmoteOverlays.map(overlay => (
          <g key={overlay.key}>
            {overlay.areaPathD ? (
              <path
                d={overlay.areaPathD}
                fill={overlay.color}
                opacity={seriesFocusOpacity(overlay.key, CHART_THEME.emoteOverlay)}
              />
            ) : null}
            {overlay.linePathD ? (
              <path
                d={overlay.linePathD}
                fill="none"
                stroke={overlay.color}
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="2"
                strokeDasharray="4 4"
                opacity={seriesFocusOpacity(overlay.key, 0.92)}
              />
            ) : null}
          </g>
        ))}

        {/* Viewer line (split when tail is estimated/incomplete) */}
        {viewersItem && viewerLineSegments.map((segment, segmentIndex) => {
          const pathD = linePath(segment.values, viewerAxis.max, width, height, padLeft, padRight, padTop, padBottom, true, 'viewer', viewerAxis.min)
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
                bar.isSpike
                  ? CHART_THEME.emote.barSpike
                  : bar.hasValue
                    ? CHART_THEME.emote.bar
                    : CHART_THEME.emote.barBaseline,
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
            strokeWidth="1"
            opacity={seriesFocusOpacity('emotes', CHART_THEME.emote.line)}
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
            strokeWidth="1.5"
            opacity={seriesFocusOpacity('chat', CHART_THEME.chat.lineOpacity)}
          />
        ) : null}

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

        {/* Emote spike markers */}
        {emoteSpikeIdxs.map(idx => {
          const value = emotesDisplayValues[idx]
          if (value === null || value <= 0) return null
          const n = rollups.length
          const cx = n === 1 ? padLeft : padLeft + (idx / (n - 1)) * (width - padLeft - padRight)
          const cy = plotY(value, activityAxis.max, height, padTop, padBottom, 'activity-emote', activityAxis.min)
          return (
            <circle
              key={`emote-spike-${idx}`}
              cx={cx}
              cy={cy}
              r={CHART_THEME.spike.dotRadius}
              fill={CHART_THEME.spike.color}
              stroke={CHART_THEME.background}
              strokeWidth="1"
              opacity={seriesFocusOpacity('emotes', CHART_THEME.spike.opacity)}
            />
          )
        })}

        {/* Chat spike markers */}
        {chatSpikeIdxs.map(idx => {
          const value = chatDisplayValues[idx]
          if (value === null || value <= 0 || !chatItem) return null
          const n = rollups.length
          const chatMax = scaleForSeries(chatItem)
          const cx = n === 1 ? padLeft : padLeft + (idx / (n - 1)) * (width - padLeft - padRight)
          const cy = plotY(value, chatMax, height, padTop, padBottom, 'activity-chat', 0)
          return (
            <circle
              key={`chat-spike-${idx}`}
              cx={cx}
              cy={cy}
              r={CHART_THEME.spike.dotRadius}
              fill={CHART_THEME.spike.color}
              stroke={CHART_THEME.background}
              strokeWidth="1"
              opacity={seriesFocusOpacity('chat', CHART_THEME.spike.opacity)}
            />
          )
        })}

        {/* Viewer data point dots */}
        {showDots && viewersItem && viewersItem.values.map((val, idx) => {
          if (val === null) return null
          const n = rollups.length
          const cx = n === 1 ? padLeft : padLeft + (idx / (n - 1)) * (width - padLeft - padRight)
          const cy = plotY(val, viewerAxis.max, height, padTop, padBottom, 'viewer', viewerAxis.min)
          const step = Math.max(1, Math.floor(n / 60))
          if (idx % step !== 0 && idx !== n - 1 && idx !== 0) return null
          const estimated = viewerTailStart > 0 && idx >= viewerTailStart
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
                y={padTop + 26}
                textAnchor="start"
                className="fill-amber-200/80 text-[8px] font-black uppercase"
              >
                Selected max {count(selectedEmoteScaleMax)}
              </text>
            ) : null}
            <text
              x={width - padRight + 2}
              y={activityLayout.activityTop + activityLayout.activityHeight * 0.21}
              textAnchor="start"
              className="fill-violet-300/80 text-[8px] font-black uppercase"
            >
              Chat (syncing)
            </text>
            <text
              x={width - padRight + 2}
              y={activityLayout.activityTop + activityLayout.activityHeight * 0.71}
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
              {expandedScale ? 'Emote max' : 'Emote peak'} {count(activityScaleMax)}
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
              y={activityLayout.activityTop + activityLayout.activityHeight * 0.71}
              className="fill-emerald-400/50 text-[8px] font-black uppercase"
            >
              Emotes
            </text>
            {perEmoteSeries.length > 0 ? (
              <text
                x={width - padRight + 2}
                y={padTop + 12}
                textAnchor="start"
                className="fill-amber-200/80 text-[8px] font-black uppercase"
              >
                Selected max {count(selectedEmoteScaleMax)}
              </text>
            ) : null}
            <text
              x={width - padRight + 2}
              y={activityLayout.activityTop + activityLayout.activityHeight * 0.21}
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
                <text x={x} y={height - padBottom + 20} textAnchor="middle" className="fill-zinc-500 text-[10px] font-black">{clock(item.minuteTs)}</text>
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
              stroke="#f59e0b"
              strokeWidth="2.5"
              strokeDasharray="4 3"
            />
          )
        })()}

        {/* Same-page playback cursor (Req 22.1): tracks VOD playhead at >= 1Hz
            when a co-located player is active for this stream. */}
        {cursorSync.synced && cursorSync.cursorOffsetSeconds !== null && rollups.length > 1 && (() => {
          const firstMs = new Date(rollups[0].minuteTs).getTime()
          const lastMs = new Date(rollups[rollups.length - 1].minuteTs).getTime()
          const span = lastMs - firstMs
          if (!Number.isFinite(span) || span <= 0) return null
          const startedAt = detail?.stream?.startedAt
          const startMs = startedAt ? new Date(startedAt).getTime() : firstMs
          const targetMs = startMs + cursorSync.cursorOffsetSeconds * 1000
          const pct = Math.min(1, Math.max(0, (targetMs - firstMs) / span))
          const playX = padLeft + pct * (width - padLeft - padRight)
          return (
            <line
              x1={playX}
              x2={playX}
              y1={padTop}
              y2={height - padBottom}
              stroke="#34d399"
              strokeWidth="2"
            />
          )
        })()}

        {/* Draw game dividers and labels */}
        {chartGames.map((segment, index) => {
          const n = rollups.length
          if (n === 0) return null
          const totalDurationSec = n * 60
          if (!Number.isFinite(totalDurationSec) || totalDurationSec <= 0) return null
          if (!Number.isFinite(segment.offsetSeconds) || !Number.isFinite(segment.durationSeconds) || segment.durationSeconds <= 0) return null

          const startX = padLeft + (segment.offsetSeconds / totalDurationSec) * (width - padLeft - padRight)
          const durationFraction = segment.durationSeconds / totalDurationSec
          const endX = startX + durationFraction * (width - padLeft - padRight)
          if (!Number.isFinite(startX) || !Number.isFinite(endX)) return null
          const centerX = (startX + endX) / 2
          const textWidth = endX - startX
          const maxChars = Math.floor(textWidth / 8)
          const displayTitle = segment.gameName.length > maxChars ? segment.gameName.slice(0, Math.max(0, maxChars - 3)) + '...' : segment.gameName

          return (
            <g key={segment.id || index}>
              {segment.offsetSeconds > 0 && (
                <line
                  x1={startX}
                  y1={padTop}
                  x2={startX}
                  y2={height - padBottom}
                  stroke="#f97316"
                  strokeWidth="1.5"
                  strokeDasharray="4 4"
                  opacity="0.6"
                />
              )}
              {textWidth > 30 && (
                <g>
                  <rect
                    x={startX + 4}
                    y={padTop - 24}
                    width={textWidth - 8}
                    height={18}
                    rx={4}
                    fill="#f97316"
                    opacity="0.12"
                  />
                  <text
                    x={centerX}
                    y={padTop - 11}
                    fill="#f97316"
                    fontSize="9.5"
                    fontWeight="900"
                    textAnchor="middle"
                    opacity="0.95"
                  >
                    {displayTitle}
                  </text>
                </g>
              )}
            </g>
          )
        })}

        <line x1={hoverX} x2={hoverX} y1={padTop} y2={height - padBottom} stroke="rgba(255,255,255,.28)" strokeWidth="1" />

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
          onClick={event => {
            if (rollups.length === 0) return
            const rect = event.currentTarget.getBoundingClientRect()
            if (rect.width <= 0) return
            const clientXRelative = event.clientX - rect.left
            const pct = Math.min(1, Math.max(0, clientXRelative / rect.width))
            const idx = Math.round(pct * (rollups.length - 1))
            if (rollups[idx]) {
              onSelectRollup(rollups[idx])
            }
          }}
        />
      </svg>
      </div>
      {viewMode === 'emotes' ? (
      <div className="mt-3 flex max-h-32 flex-wrap gap-1.5 overflow-y-auto pr-1 sm:max-h-none">
        {(detail?.topEmotes ?? []).slice(0, 16).map(emote => {
          const imageUrl = getEmoteImageUrl(emote)
          return (
            <button
              key={emote.key}
              type="button"
              onClick={() => onSelectEmote(emote.key)}
              className={`inline-flex min-w-0 items-center gap-1 rounded border px-1.5 py-0.5 text-[10px] font-black transition ${selectedEmotes.has(emote.key) ? 'border-amber-200/60 bg-amber-300/20 text-amber-100' : 'border-white/10 bg-white/[0.045] text-zinc-300 hover:bg-white/[0.08]'}`}
            >
              {imageUrl && (
                <img src={imageUrl} alt={emote.name} className="h-4 w-4 object-contain inline-block align-middle" loading="lazy" />
              )}
              <span>{emote.name}</span>
              <span className="text-zinc-500 font-bold">{count(emote.count)}</span>
            </button>
          )
        })}
      </div>
      ) : null}
    </div>
  )
}


export default memo(AnalyticsChart)
