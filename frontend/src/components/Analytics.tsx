import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query'

import {
  ensureChannelEmotes,
  getAnalyticsLive,
  getAnalyticsStream,
  getAnalyticsStreams,
  getChannel,
  getChannelStreamHistory,
  watchAnalyticsChannel,
  getClipperJobs,
  getClipperFinalVideoUrl,
  describeClipperFailure,
  getSyncStatus,
  startHistoricalSync,
  getStreamGameSegments,
  type SyncPhase,
  type SyncStatus,
  getTwitchDayClips,
  triggerClipperManual,
  type AnalyticsStream,
  type AnalyticsStreamDetail,
  type AnalyticsTopEmote,
  type SourceStatus,
  type AnalyticsMinuteRollup,
  type ClipperJob,
  type GameSegment,
} from '../api'

function getEmoteImageUrl(emote: { provider?: string; id?: string; imageUrl?: string }) {
  if (emote.imageUrl) return emote.imageUrl
  if (!emote.id) return undefined
  return `/emotes/${emote.id}/1x.webp`
}

type Series = {
  key: string
  label: string
  color: string
  values: Array<number | null>
  max: number
  dashed?: boolean
}

function count(value: number | null | undefined) {
  if (value === null || value === undefined) return '-'
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return value.toLocaleString()
}

function relativeTime(value?: string | number) {
  if (!value) return '-'
  const ts = typeof value === 'number' ? value : Date.parse(value)
  if (!Number.isFinite(ts)) return '-'
  const diff = Date.now() - ts
  const minutes = Math.max(1, Math.round(diff / 60000))
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.round(minutes / 60)
  if (hours < 48) return `${hours}h ago`
  return `${Math.round(hours / 24)}d ago`
}

function clock(value?: string) {
  if (!value) return '-'
  const ts = Date.parse(value)
  if (!Number.isFinite(ts)) return '-'
  return new Date(ts).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
}

function formatDateTime(value?: string) {
  if (!value) return '-'
  const ts = Date.parse(value)
  if (!Number.isFinite(ts)) return '-'
  const date = new Date(ts)
  return date.toLocaleDateString([], { month: 'short', day: 'numeric' }) + ' ' + date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
}

function duration(stream?: AnalyticsStream) {
  if (!stream) return '-'
  const start = Date.parse(stream.startedAt)
  const end = stream.endedAt ? Date.parse(stream.endedAt) : Date.now()
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return '-'
  const minutes = Math.round((end - start) / 60000)
  if (minutes < 60) return `${minutes}m`
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`
}

function sourceTone(state: SourceStatus['state']) {
  if (state === 'ready') return 'border-emerald-400/20 bg-emerald-400/10 text-emerald-100'
  if (state === 'fallback') return 'border-cyan-300/20 bg-cyan-400/10 text-cyan-100'
  if (state === 'blocked' || state === 'unavailable' || state === 'limited') return 'border-amber-300/20 bg-amber-400/10 text-amber-100'
  return 'border-red-400/20 bg-red-500/10 text-red-100'
}

function SourcePills({ sources }: { sources?: SourceStatus[] }) {
  if (!sources?.length) return null
  return (
    <div className="flex flex-wrap gap-2">
      {sources.map(source => (
        <span key={`${source.source}-${source.state}-${source.message ?? ''}`} title={source.message} className={`rounded border px-2 py-1 text-[10px] font-black uppercase ${sourceTone(source.state)}`}>
          {source.source.replace(/_/g, ' ')} {source.state}
        </span>
      ))}
    </div>
  )
}

function StatCard({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <div className="rounded border border-white/10 bg-white/[0.045] p-3">
      <div className="text-[11px] font-black uppercase text-zinc-500">{label}</div>
      <div className={`mt-1 truncate text-xl font-black ${tone || 'text-white'}`}>{value}</div>
    </div>
  )
}

function seriesMax(values: Array<number | null>) {
  return Math.max(0, ...values.map(value => value ?? 0))
}

function viewerValue(point: AnalyticsMinuteRollup) {
  return point.viewerLatest || point.viewerAvg || point.viewerMax || 0
}

function rollupHasMinuteData(point: AnalyticsMinuteRollup) {
  return !point.missing && (
    (point.viewerSamples ?? 0) > 0
    || (point.chatCount ?? 0) > 0
    || (point.seventvEmoteCount ?? 0) > 0
  )
}

const SYNC_PHASE_ORDER: SyncPhase[] = [
  'starting',
  'scraping_tracker',
  'parsing_tracker',
  'resolving_vod',
  'fetching_comments',
  'writing_rollups',
  'completed',
]

const SYNC_PHASE_LABELS: Record<SyncPhase, string> = {
  starting: 'Starting sync',
  scraping_tracker: 'Scraping TwitchTracker',
  parsing_tracker: 'Parsing viewer chart',
  resolving_vod: 'Resolving Twitch VOD',
  fetching_comments: 'Fetching VOD chat',
  writing_rollups: 'Writing minute rollups',
  completed: 'Completed',
  failed: 'Failed',
}

function syncPhaseIndex(phase: SyncPhase) {
  const idx = SYNC_PHASE_ORDER.indexOf(phase)
  return idx < 0 ? 0 : idx
}

function formatElapsed(startedAt?: string) {
  if (!startedAt) return '0s'
  const ms = Date.now() - Date.parse(startedAt)
  if (!Number.isFinite(ms) || ms < 0) return '0s'
  const sec = Math.floor(ms / 1000)
  if (sec < 60) return `${sec}s`
  const min = Math.floor(sec / 60)
  return `${min}m ${sec % 60}s`
}

function SyncProgressPanel({ status }: { status: SyncStatus | null }) {
  if (!status) return null
  const activeIdx = status.phase === 'failed' ? -1 : syncPhaseIndex(status.phase)
  const visiblePhases = status.viewersOnly
    ? SYNC_PHASE_ORDER.filter(p => p !== 'fetching_comments')
    : SYNC_PHASE_ORDER.filter(p => p !== 'completed')

  return (
    <div className="mt-4 w-full max-w-md rounded-lg border border-white/10 bg-black/30 px-4 py-3 text-left">
      <div className="flex items-center justify-between gap-3 text-[11px] font-black uppercase tracking-wide text-zinc-400">
        <span>Sync progress</span>
        <span>{formatElapsed(status.startedAt)}</span>
      </div>
      <ul className="mt-3 space-y-2">
        {visiblePhases.map((phase, idx) => {
          const done = status.phase === 'completed' || idx < activeIdx
          const active = idx === activeIdx && status.phase !== 'failed' && status.phase !== 'completed'
          let detail = ''
          if (active && phase === 'fetching_comments') {
            if ((status.commentsFetched ?? 0) > 0) {
              detail = `${status.commentsFetched!.toLocaleString()} comments`
            } else if (status.message) {
              detail = status.message
            }
          } else if (active && phase === 'writing_rollups' && (status.rollupsWritten ?? 0) > 0) {
            detail = `${status.rollupsWritten} minutes`
          } else if (active && status.message) {
            detail = status.message
          }
          return (
            <li key={phase} className="flex items-start gap-2 text-xs">
              <span className={`mt-0.5 inline-flex h-4 w-4 shrink-0 items-center justify-center rounded-full text-[10px] font-black ${
                done ? 'bg-emerald-500/20 text-emerald-200' : active ? 'bg-violet-500/25 text-violet-100' : 'bg-white/5 text-zinc-600'
              }`}>
                {done ? '✓' : active ? '…' : ''}
              </span>
              <div className="min-w-0">
                <div className={`font-bold ${active ? 'text-violet-100' : done ? 'text-zinc-300' : 'text-zinc-500'}`}>
                  {SYNC_PHASE_LABELS[phase]}
                </div>
                {detail ? <div className="truncate text-[11px] font-semibold text-zinc-500">{detail}</div> : null}
              </div>
            </li>
          )
        })}
      </ul>
      {status.phase === 'failed' && status.error ? (
        <div className="mt-3 text-xs font-bold text-red-400">{status.error}</div>
      ) : null}
    </div>
  )
}

async function pollSyncUntilDone(streamId: string, onUpdate: (status: SyncStatus | null) => void) {
  const terminal: SyncPhase[] = ['completed', 'failed']
  for (;;) {
    const status = await getSyncStatus(streamId)
    onUpdate(status)
    if (!status || terminal.includes(status.phase)) {
      return status
    }
    await new Promise(resolve => setTimeout(resolve, 2000))
  }
}

function normalizeGameSegments(games: GameSegment[], rollupCount: number): GameSegment[] {
  if (!games.length || rollupCount <= 0) return games
  const needsRepair = games.every(game => (game.durationSeconds ?? 0) <= 0)
  if (!needsRepair) return games

  const totalDurationSec = rollupCount * 60
  const each = Math.max(60, Math.floor(totalDurationSec / games.length))
  let offset = 0
  return games.map((game, index) => {
    const durationSeconds = index === games.length - 1
      ? Math.max(60, totalDurationSec - offset)
      : each
    const segment = { ...game, offsetSeconds: offset, durationSeconds }
    offset += durationSeconds
    return segment
  })
}

type ChatBarPoint = {
  index: number
  value: number
  x: number
  barWidth: number
}

function decimateChatBars(
  rollups: AnalyticsMinuteRollup[],
  values: Array<number | null>,
  width: number,
  padLeft: number,
  padRight: number,
  maxBars = 200,
): ChatBarPoint[] {
  const n = rollups.length
  if (n === 0) return []
  const plotWidth = width - padLeft - padRight
  if (n <= 300) {
    return values.map((val, index) => {
      if (val === null || val === undefined) return null
      const x = n === 1 ? padLeft : padLeft + (index / (n - 1)) * plotWidth
      const barWidth = Math.max(1.5, plotWidth / n - 1)
      return { index, value: val, x, barWidth }
    }).filter((point): point is ChatBarPoint => point !== null)
  }
  const bucketCount = Math.min(maxBars, n)
  const bucketSize = Math.ceil(n / bucketCount)
  const out: ChatBarPoint[] = []
  for (let bucket = 0; bucket < bucketCount; bucket++) {
    const start = bucket * bucketSize
    const end = Math.min(n, start + bucketSize)
    let sum = 0
    let count = 0
    for (let i = start; i < end; i++) {
      const val = values[i]
      if (val !== null && val !== undefined) {
        sum += val
        count++
      }
    }
    if (count === 0) continue
    const index = Math.min(n - 1, start + Math.floor(bucketSize / 2))
    const x = n === 1 ? padLeft : padLeft + (index / (n - 1)) * plotWidth
    const barWidth = Math.max(1.5, (plotWidth / bucketCount) - 1)
    out.push({ index, value: sum / count, x, barWidth })
  }
  return out
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

function smoothSeries(values: Array<number | null>, windowSize = 5): Array<number | null> {
  const n = values.length
  if (n === 0) return []
  const result: Array<number | null> = []
  const half = Math.floor(windowSize / 2)
  for (let i = 0; i < n; i++) {
    if (values[i] === null) {
      result.push(null)
      continue
    }
    let sum = 0
    let count = 0
    const start = Math.max(0, i - half)
    const end = Math.min(n, i + half + 1)
    for (let j = start; j < end; j++) {
      const val = values[j]
      if (val !== null) {
        sum += val
        count++
      }
    }
    result.push(count > 0 ? sum / count : 0)
  }
  return result
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
    const value = viewerValue(point)
    if (value > 0) return value
    return useViewerFallback && viewerBaseline > 0 ? viewerBaseline : 0
  })
  const chat = rollups.map(point => point.missing ? null : point.chatCount)
  
  // Calculate 7TV maximum from raw values to maintain legend accuracy, then smooth for rendering
  const seventvRaw = rollups.map(point => point.missing ? null : point.seventvEmoteCount)
  const seventvMax = seriesMax(seventvRaw)
  const seventvSmoothed = smoothSeries(seventvRaw, 5)

  const viewersMax = seriesMax(viewers)
  const out: Series[] = [
    { key: 'viewers', label: 'Viewers', color: '#22d3ee', values: viewers, max: viewersMax > 0 ? viewersMax : Math.max(0, peakViewersFallback) },
    { key: 'chat', label: 'Chat/min', color: '#a78bfa', values: chat, max: seriesMax(chat) },
    { key: 'seventv', label: '7TV/min', color: '#34d399', values: seventvSmoothed, max: seventvMax },
  ]
  const palette = ['#f59e0b', '#fb7185', '#60a5fa', '#f472b6', '#facc15']
  Array.from(selected).slice(0, 5).forEach((key, index) => {
    const rawValues = rollups.map(point => point.missing ? null : point.emotes?.[key] ?? 0)
    const maxVal = seriesMax(rawValues)
    const smoothedValues = smoothSeries(rawValues, 5)
    out.push({ key, label: emoteLabel(key), color: palette[index % palette.length], values: smoothedValues, max: maxVal, dashed: true })
  })
  return out
}

function emoteLabel(key: string) {
  const parts = key.split(':')
  if (parts.length >= 3) return parts.slice(2).join(':')
  return key
}

const SECONDARY_PLOT_FRACTION = 0.36

function plotBand(
  height: number,
  padTop: number,
  padBottom: number,
  plotFraction = 1,
) {
  const fullPlotHeight = height - padTop - padBottom
  const bandHeight = fullPlotHeight * plotFraction
  const bandBottom = height - padBottom
  const bandTop = bandBottom - bandHeight
  return { bandTop, bandBottom, bandHeight }
}

function plotY(
  value: number,
  max: number,
  height: number,
  padTop: number,
  padBottom: number,
  plotFraction = 1,
) {
  const { bandTop, bandBottom, bandHeight } = plotBand(height, padTop, padBottom, plotFraction)
  const y = bandBottom - (Math.max(0, value) / max) * bandHeight
  return Math.max(bandTop, Math.min(bandBottom, y))
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
  plotFraction = 1,
) {
  const n = values.length
  if (!n) return ''
  const { bandTop, bandBottom } = plotBand(height, padTop, padBottom, plotFraction)

  const points: Array<{ x: number; y: number } | null> = values.map((value, index) => {
    if (value === null) return null
    const x = n === 1 ? padLeft : padLeft + (index / (n - 1)) * (width - padLeft - padRight)
    const y = plotY(value, max, height, padTop, padBottom, plotFraction)
    return { x, y }
  })

  let path = ''
  let segment: Array<{ x: number; y: number }> = []

  const drawSegment = (seg: Array<{ x: number; y: number }>, linear = false) => {
    if (seg.length === 0) return ''
    if (seg.length === 1) return `M${seg[0].x.toFixed(1)} ${seg[0].y.toFixed(1)}`
    if (seg.length === 2 || linear) {
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

function gapPath(
  values: Array<number | null>,
  max: number,
  width: number,
  height: number,
  padLeft: number,
  padRight: number,
  padTop: number,
  padBottom: number,
  plotFraction = 1,
) {
  const n = values.length
  let d = ''
  for (let i = 1; i < n - 1; i++) {
    if (values[i] !== null || values[i - 1] === null || values[i + 1] === null) continue
    const x1 = padLeft + ((i - 1) / (n - 1)) * (width - padLeft - padRight)
    const y1 = plotY(values[i - 1] ?? 0, max, height, padTop, padBottom, plotFraction)
    const x2 = padLeft + ((i + 1) / (n - 1)) * (width - padLeft - padRight)
    const y2 = plotY(values[i + 1] ?? 0, max, height, padTop, padBottom, plotFraction)
    d += `M${x1.toFixed(1)} ${y1.toFixed(1)} L${x2.toFixed(1)} ${y2.toFixed(1)} `
  }
  return d.trim()
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
  plotFraction = 1,
) {
  const n = values.length
  if (!n) return ''
  const { bandTop, bandBottom } = plotBand(height, padTop, padBottom, plotFraction)

  const points: Array<{ x: number; y: number } | null> = values.map((value, index) => {
    if (value === null) return null
    const x = n === 1 ? padLeft : padLeft + (index / (n - 1)) * (width - padLeft - padRight)
    const y = plotY(value, max, height, padTop, padBottom, plotFraction)
    return { x, y }
  })

  let path = ''
  let segment: Array<{ x: number; y: number }> = []

  const drawAreaSegment = (seg: Array<{ x: number; y: number }>) => {
    if (seg.length === 0) return ''
    const bottomY = height - padBottom
    
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

function plotFractionForSeries(item: Series, expandedScale: boolean) {
  return expandedScale && item.key !== 'viewers' ? SECONDARY_PLOT_FRACTION : 1
}

function AnalyticsChart({
  detail,
  selectedEmotes,
  onSelectEmote,
  selectedRollup,
  onSelectRollup,
  syncing = false,
  syncStatus = null,
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
}: {
  detail?: AnalyticsStreamDetail;
  selectedEmotes: Set<string>;
  onSelectEmote: (key: string) => void;
  selectedRollup: AnalyticsMinuteRollup | null;
  onSelectRollup: (rollup: AnalyticsMinuteRollup | null) => void;
  syncing?: boolean;
  syncStatus?: SyncStatus | null;
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
}) {
  const [hover, setHover] = useState<number | null>(null)
  const [expandedScale, setExpandedScale] = useState(false)
  const [showDots, setShowDots] = useState(true)
  const allRollups = detail?.rollups ?? []
  const rollups = useMemo(() => rollupsForChart(allRollups, isLive), [allRollups, isLive])
  const peakViewersFallback = detail?.stream?.peakViewers ?? 0
  const avgViewersFallback = detail?.stream?.avgViewers ?? 0
  const hasSyncedChat = rollups.some(point => !point.missing && (point.chatCount ?? 0) > 0)
  const hasViewerRollups = rollups.some(point => !point.missing && viewerValue(point) > 0)
  const hasFlatViewerLine = useMemo(() => {
    const values = rollups.filter(point => !point.missing).map(viewerValue).filter(value => value > 0)
    if (values.length < 10) return false
    return Math.min(...values) === Math.max(...values)
  }, [rollups])
  const useViewerFallback = !isLive
    && !hasSyncedChat
    && rollups.every(point => point.missing || viewerValue(point) === 0)
  const needsViewerResync = !isLive && hasSyncedChat && (!hasViewerRollups || hasFlatViewerLine)
  const hasChartData = useMemo(
    () => allRollups.some(rollupHasMinuteData),
    [allRollups],
  )
  const width = 1000
  const height = 360
  const padLeft = 90
  const padRight = 34
  const padTop = 34
  const padBottom = 34

  const series = useMemo(
    () => buildSeries(rollups, selectedEmotes, peakViewersFallback, avgViewersFallback, useViewerFallback),
    [rollups, selectedEmotes, peakViewersFallback, avgViewersFallback, useViewerFallback],
  )
  const chatSeries = useMemo(() => series.find(s => s.key === 'chat'), [series])
  const lineSeries = useMemo(() => series.filter(s => s.key !== 'chat'), [series])
  const chatBars = useMemo(
    () => chatSeries
      ? decimateChatBars(rollups, chatSeries.values, width, padLeft, padRight)
      : [],
    [chatSeries, rollups, width, padLeft, padRight],
  )
  const chartScaleMax = useMemo(() => {
    const viewersItem = series.find(s => s.key === 'viewers')
    const overlayMax = Math.max(
      1,
      chatSeries?.max ?? 0,
      ...lineSeries.filter(s => s.key !== 'viewers').map(s => s.max),
    )
    const hasViewerSignal = (viewersItem?.max ?? 0) > 0 || peakViewersFallback > 0
    return hasViewerSignal
      ? Math.max(1, viewersItem?.max ?? 0, peakViewersFallback)
      : overlayMax
  }, [series, chatSeries, lineSeries, peakViewersFallback])
  const scaleForSeries = useCallback((item: Series) => (
    expandedScale ? Math.max(1, item.max) : chartScaleMax
  ), [expandedScale, chartScaleMax])
  const viewerAreaPathD = useMemo(() => {
    const viewersItem = series.find(s => s.key === 'viewers')
    if (!viewersItem) return ''
    return areaPath(
      viewersItem.values,
      scaleForSeries(viewersItem),
      width,
      height,
      padLeft,
      padRight,
      padTop,
      padBottom,
    )
  }, [series, scaleForSeries, width, height, padLeft, padRight, padTop, padBottom])
  const lineSeriesPaths = useMemo(() => lineSeries.map(item => {
    const seriesMax = scaleForSeries(item)
    const plotFraction = plotFractionForSeries(item, expandedScale)
    return {
      key: item.key,
      item,
      seriesMax,
      plotFraction,
      gapPathD: gapPath(item.values, seriesMax, width, height, padLeft, padRight, padTop, padBottom, plotFraction),
      linePathD: linePath(item.values, seriesMax, width, height, padLeft, padRight, padTop, padBottom, item.key !== 'viewers', plotFraction),
    }
  }), [lineSeries, scaleForSeries, expandedScale, width, height, padLeft, padRight, padTop, padBottom])
  const hasChatData = useMemo(
    () => rollups.some(point => (point.chatCount ?? 0) > 0 || (point.seventvEmoteCount ?? 0) > 0),
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
 
  if (!hasChartData) {
    const isTwitchTracker = detail?.sources?.some(s => s.source === 'twitchtracker')
    const canShowSync = canSync || detail?.state === 'historical' || isTwitchTracker
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
              <button
                onClick={onSync}
                disabled={syncing}
                className="rounded-lg bg-violet-600 px-5 py-2.5 text-xs font-black uppercase tracking-wider text-white transition hover:bg-violet-500 active:scale-95 disabled:pointer-events-none disabled:opacity-50"
              >
                {syncing ? 'Sync in progress…' : 'Sync Historical Data'}
              </button>
              {syncing ? <SyncProgressPanel status={syncStatus} /> : null}
              {syncNotice && <div className="mt-2 text-xs font-bold text-amber-300">{syncNotice}</div>}
              {syncError && <div className="mt-2 text-xs font-bold text-red-400">{syncError}</div>}
            </div>
          )}
        </div>
      </div>
    )
  }

  const viewersItem = series.find(s => s.key === 'viewers')
  const viewerValues = viewersItem?.values.filter((v): v is number => v !== null && v > 0) ?? []
  const avgViewers = viewerValues.length > 0
    ? Math.round(viewerValues.reduce((a, b) => a + b, 0) / viewerValues.length)
    : (detail?.stream?.avgViewers ?? 0)
  const hasViewerSignal = (viewersItem?.max ?? 0) > 0 || peakViewersFallback > 0
  const overlayMax = Math.max(
    1,
    chatSeries?.max ?? 0,
    ...lineSeries.filter(s => s.key !== 'viewers').map(s => s.max),
  )
  const viewersScaleMax = hasViewerSignal
    ? Math.max(1, viewersItem?.max ?? 0, peakViewersFallback)
    : overlayMax
  const scaleForLineSeries = (item: Series) => (
    expandedScale ? Math.max(1, item.max) : viewersScaleMax
  )

  const yMax = padTop
  const viewerScale = viewersItem ? scaleForLineSeries(viewersItem) : viewersScaleMax
  const yAvg = height - padBottom - (avgViewers / viewerScale) * (height - padTop - padBottom)
  const showAvgLabel = (yAvg - yMax) > 22 && (height - padBottom - yAvg) > 22

  const hoverIndex = hover === null ? rollups.length - 1 : hover
  const hoverPoint = rollups[hoverIndex]
  const hoverX = rollups.length === 1 ? padLeft : padLeft + (hoverIndex / (rollups.length - 1)) * (width - padLeft - padRight)

  return (
    <div className="rounded border border-white/10 bg-[#0d0d12] p-3">
      {needsViewerResync ? (
        <div className="mb-3 rounded border border-amber-400/25 bg-amber-400/10 px-3 py-2 text-xs font-semibold text-amber-100">
          Viewer timeline missing from this sync. Click <span className="font-black">Re-sync viewers</span> to pull the TwitchTracker viewer chart (fast — chat/7TV stay as-is).
        </div>
      ) : null}
      {syncing ? <div className="mb-3"><SyncProgressPanel status={syncStatus} /></div> : null}
      {syncNotice ? (
        <div className="mb-3 rounded border border-amber-400/25 bg-amber-400/10 px-3 py-2 text-xs font-bold text-amber-200">{syncNotice}</div>
      ) : null}
      {syncError ? (
        <div className="mb-3 rounded border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs font-bold text-red-300">{syncError}</div>
      ) : null}
      <div className="mb-3 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex flex-wrap gap-2">
          {series.map(item => {
            const parts = item.key.split(':')
            const isEmote = parts.length >= 2
            let imageUrl: string | undefined
            if (isEmote) {
              imageUrl = getEmoteImageUrl({ provider: parts[0], id: parts[1] })
            }
            return (
              <span key={item.key} className="flex items-center gap-1.5 rounded border border-white/10 bg-white/[0.05] px-2 py-1 text-[11px] font-black uppercase text-zinc-300">
                <span className="inline-block h-2 w-2 rounded-full" style={{ background: item.color }} />
                {imageUrl && (
                  <img src={imageUrl} alt={item.label} className="h-3.5 w-3.5 object-contain inline-block align-middle" loading="lazy" />
                )}
                <span>{item.label} max {count(item.max)}</span>
              </span>
            )
          })}
        </div>
        <div className="flex items-center gap-3">
          <div className="text-xs font-bold text-zinc-500">{clock(hoverPoint?.minuteTs)} · viewers {count(hoverPoint ? viewerValue(hoverPoint) : null)} · chat {count(hoverPoint?.chatCount)} · 7TV {count(hoverPoint?.seventvEmoteCount)}</div>
          {canSync && (!hasChatData || needsViewerResync) ? (
            <button
              type="button"
              onClick={onSync}
              disabled={syncing}
              className="rounded border border-violet-400/30 bg-violet-500/10 px-2.5 py-1 text-[10px] font-black uppercase text-violet-200 transition hover:bg-violet-500/20 disabled:opacity-50"
            >
              {syncing ? 'Syncing…' : needsViewerResync ? 'Re-sync viewers' : 'Sync chat/emotes'}
            </button>
          ) : null}
          <div className="flex items-center gap-1.5">
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
              onClick={() => setShowDots(v => !v)}
              title={showDots ? 'Hide data points' : 'Show data points'}
              className={`rounded border px-2 py-1 text-[10px] font-black uppercase transition ${showDots ? 'border-cyan-400/30 bg-cyan-400/10 text-cyan-200' : 'border-white/10 bg-white/[0.04] text-zinc-500 hover:text-zinc-300'}`}
            >
              ●
            </button>
            <button
              type="button"
              onClick={() => setExpandedScale(v => !v)}
              title={expandedScale ? 'Shared viewer Y-axis (7TV/emotes in lower band)' : 'Normalize viewers to full height; 7TV stays in lower band'}
              className={`rounded border px-2 py-1 text-[10px] font-black uppercase transition ${expandedScale ? 'border-violet-400/30 bg-violet-400/10 text-violet-200' : 'border-white/10 bg-white/[0.04] text-zinc-500 hover:text-zinc-300'}`}
            >
              {expandedScale ? 'Shared' : 'Expand'}
            </button>
          </div>
        </div>
      </div>
      <div className="overflow-hidden rounded">
      <svg
        viewBox={`0 0 ${width} ${height}`}
        className="h-[300px] w-full cursor-crosshair select-none"
      >
        <defs>
          <linearGradient id="viewerAreaGradient" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#22d3ee" stopOpacity="0.25" />
            <stop offset="100%" stopColor="#22d3ee" stopOpacity="0" />
          </linearGradient>
          <clipPath id="analyticsPlotClip">
            <rect x={padLeft} y={padTop} width={width - padLeft - padRight} height={height - padTop - padBottom} />
          </clipPath>
        </defs>

        {/* Horizontal guide lines */}
        <line x1={padLeft} x2={width - padRight} y1={padTop} y2={padTop} stroke="rgba(34, 211, 238, 0.15)" strokeWidth="1" strokeDasharray="4 4" />
        {showAvgLabel && (
          <line x1={padLeft} x2={width - padRight} y1={yAvg} y2={yAvg} stroke="rgba(34, 211, 238, 0.15)" strokeWidth="1" strokeDasharray="4 4" />
        )}
        <line x1={padLeft} x2={width - padRight} y1={height - padBottom} y2={height - padBottom} stroke="rgba(255,255,255,.08)" strokeWidth="1" />

        {/* Left Y-Axis labels */}
        <g>
          {/* MAX Label */}
          <text x={padLeft - 12} y={padTop - 4} textAnchor="end" className="fill-cyan-400 text-[10px] font-black uppercase">MAX</text>
          <text x={padLeft - 12} y={padTop + 10} textAnchor="end" className="fill-cyan-400 text-sm font-black">{count(viewersScaleMax)}</text>

          {/* AVG Label */}
          {showAvgLabel && (
            <>
              <text x={padLeft - 12} y={yAvg - 4} textAnchor="end" className="fill-cyan-400/80 text-[10px] font-black uppercase">AVG</text>
              <text x={padLeft - 12} y={yAvg + 10} textAnchor="end" className="fill-cyan-400/80 text-sm font-black">{count(avgViewers)}</text>
            </>
          )}
        </g>

        <g clipPath="url(#analyticsPlotClip)">
        {/* Draw chat count as a bar chart at the bottom, scaled to 35% of the chart height */}
        {chatSeries && chatBars.map(bar => {
          const barHeight = (bar.value / Math.max(1, chatSeries.max)) * (height - padTop - padBottom) * 0.35
          const y = height - padBottom - barHeight
          const isSpike = bar.value > chatSeries.max * 0.85
          const color = isSpike ? '#f87171' : '#38bdf8'
          return (
            <rect
              key={bar.index}
              x={bar.x - bar.barWidth / 2}
              y={y}
              width={bar.barWidth}
              height={barHeight}
              fill={color}
              opacity={0.8}
            />
          )
        })}

        {/* Draw area fill for viewers */}
        {viewerAreaPathD ? (
          <path
            d={viewerAreaPathD}
            fill="url(#viewerAreaGradient)"
          />
        ) : null}

        {/* Draw line series (viewers, emotes) over the full height */}
        {lineSeriesPaths.map(({ key, item, seriesMax, plotFraction, gapPathD, linePathD }) => (
            <g key={key}>
              <path d={gapPathD} fill="none" stroke={item.color} strokeDasharray="8 9" strokeLinecap="round" strokeWidth="2" opacity=".4" />
              <path d={linePathD} fill="none" stroke={item.color} strokeLinecap="round" strokeLinejoin="round" strokeWidth={item.dashed ? 2 : 3} strokeDasharray={item.dashed ? '5 8' : undefined} />
              {/* Data point dots */}
              {showDots && item.values.map((val, idx) => {
                if (val === null) return null
                const n = rollups.length
                const cx = n === 1 ? padLeft : padLeft + (idx / (n - 1)) * (width - padLeft - padRight)
                const cy = plotY(val, seriesMax, height, padTop, padBottom, plotFraction)
                // Only show dots at reasonable intervals to avoid clutter
                const step = Math.max(1, Math.floor(n / 60))
                if (idx % step !== 0 && idx !== n - 1 && idx !== 0) return null
                return (
                  <circle
                    key={idx}
                    cx={cx}
                    cy={cy}
                    r={hover === idx ? 5 : 3}
                    fill={item.color}
                    stroke="#0d0d12"
                    strokeWidth="1.5"
                    opacity={hover === idx ? 1 : 0.7}
                    className="transition-all duration-100"
                  />
                )
              })}
            </g>
        ))}
        </g>

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
          const selX = padLeft + (selectedIdx / (rollups.length - 1)) * (width - padLeft - padRight)
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

        {/* Draw game dividers and labels */}
        {chartGames.map((segment, index) => {
          const n = rollups.length
          if (n === 0) return null
          const totalDurationSec = n * 60
          
          const startX = padLeft + (segment.offsetSeconds / totalDurationSec) * (width - padLeft - padRight)
          const durationFraction = segment.durationSeconds / totalDurationSec
          const endX = startX + durationFraction * (width - padLeft - padRight)
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
            const rect = event.currentTarget.getBoundingClientRect()
            const clientXRelative = event.clientX - rect.left
            const pct = Math.min(1, Math.max(0, clientXRelative / rect.width))
            setHover(Math.round(pct * (rollups.length - 1)))
          }}
          onMouseLeave={() => setHover(null)}
          onClick={event => {
            const rect = event.currentTarget.getBoundingClientRect()
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
      <div className="mt-3 flex flex-wrap gap-2">
        {(detail?.topEmotes ?? []).slice(0, 16).map(emote => {
          const imageUrl = getEmoteImageUrl(emote)
          return (
            <button
              key={emote.key}
              type="button"
              onClick={() => onSelectEmote(emote.key)}
              className={`flex items-center gap-1.5 rounded border px-2 py-1 text-[11px] font-black transition ${selectedEmotes.has(emote.key) ? 'border-amber-200/60 bg-amber-300/20 text-amber-100' : 'border-white/10 bg-white/[0.045] text-zinc-300 hover:bg-white/[0.08]'}`}
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
    </div>
  )
}

function getLocalDateString(startedAt?: string) {
  if (!startedAt) return ''
  const date = new Date(startedAt)
  if (isNaN(date.getTime())) return ''
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function StreamSidebar({
  login,
  streams,
  activeID,
  isLiveView,
  liveState,
  onPrefetchStream,
}: {
  login: string
  streams: AnalyticsStream[]
  activeID?: string
  isLiveView: boolean
  liveState?: string
  onPrefetchStream?: (streamId: string) => void
}) {
  const dateCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    streams.forEach(s => {
      const slug = getLocalDateString(s.startedAt)
      if (slug) counts[slug] = (counts[slug] || 0) + 1
    })
    return counts
  }, [streams])

  return (
    <div className="flex min-h-0 flex-col overflow-hidden rounded border border-white/10 bg-white/[0.035] xl:max-h-[calc(100vh-12rem)]">
      <div className="flex items-center justify-between border-b border-white/10 px-3 py-2.5">
        <span className="text-[11px] font-black uppercase tracking-wide text-zinc-500">Streams</span>
        <span className="rounded bg-white/10 px-1.5 py-0.5 text-[10px] font-black text-zinc-400">{streams.length}</span>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <Link
          to={`/analytics/${encodeURIComponent(login)}`}
          className={`block border-b border-white/5 px-3 py-2.5 transition hover:bg-white/[0.05] ${
            isLiveView ? 'border-l-2 border-l-red-400 bg-red-500/10' : 'border-l-2 border-l-transparent'
          }`}
        >
          <div className="flex items-center gap-2">
            <span className={`h-2 w-2 rounded-full ${liveState === 'live' ? 'bg-red-400 animate-pulse' : 'bg-zinc-600'}`} />
            <span className="text-sm font-black text-white">Live / Current</span>
          </div>
          <div className="mt-1 text-[10px] font-semibold text-zinc-500">
            {liveState === 'live' ? 'Collecting now' : 'Most recent session'}
          </div>
        </Link>
        {streams.length === 0 ? (
          <div className="px-3 py-4 text-center text-[11px] font-semibold text-zinc-500">
            No past streams indexed yet.
          </div>
        ) : (
          <div className="divide-y divide-white/5">
            {streams.map(stream => {
              const dateSlug = getLocalDateString(stream.startedAt)
              const isUnique = dateSlug && dateCounts[dateSlug] === 1
              const targetSlug = isUnique ? dateSlug : stream.streamId
              const isActive = !isLiveView && (activeID === stream.streamId || activeID === dateSlug || activeID === targetSlug)
              const hasMinuteData = (stream.viewerSamples ?? 0) > 0 || (stream.chatMessages ?? 0) > 0

              return (
                <Link
                  key={stream.streamId}
                  to={`/analytics/${encodeURIComponent(login)}/${encodeURIComponent(targetSlug)}`}
                  onMouseEnter={() => onPrefetchStream?.(stream.streamId)}
                  onFocus={() => onPrefetchStream?.(stream.streamId)}
                  className={`block border-l-2 px-3 py-2.5 transition hover:bg-white/[0.05] ${
                    isActive ? 'border-l-cyan-400 bg-cyan-400/10' : 'border-l-transparent'
                  }`}
                >
                  <div className="text-[10px] font-black uppercase tracking-wide text-zinc-500">
                    {formatDateTime(stream.startedAt)}
                  </div>
                  <div className="mt-0.5 line-clamp-2 text-[13px] font-bold leading-snug text-white">
                    {stream.title || 'Untitled stream'}
                  </div>
                  <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                    {stream.category ? (
                      <span className="rounded bg-violet-500/15 px-1.5 py-0.5 text-[9px] font-black uppercase text-violet-200">
                        {stream.category}
                      </span>
                    ) : null}
                    <span className={`rounded px-1.5 py-0.5 text-[9px] font-black uppercase ${
                      hasMinuteData ? 'bg-emerald-500/10 text-emerald-300' : 'bg-amber-500/10 text-amber-300'
                    }`}>
                      {hasMinuteData ? 'Synced' : 'Stats only'}
                    </span>
                  </div>
                  <div className="mt-1.5 grid grid-cols-3 gap-1 text-[10px] font-bold text-zinc-500">
                    <span>{duration(stream)}</span>
                    <span>avg {count(stream.avgViewers)}</span>
                    <span>peak {count(stream.peakViewers)}</span>
                  </div>
                </Link>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}

function TopEmoteTable({ emotes, selected, onSelect }: { emotes: AnalyticsTopEmote[]; selected: Set<string>; onSelect: (key: string) => void }) {
  if (!emotes.length) {
    return (
      <div className="grid min-h-44 place-items-center rounded border border-white/10 bg-white/[0.035] text-center">
        <div>
          <div className="text-sm font-black text-zinc-200">No emotes counted</div>
          <div className="mt-1 text-xs font-semibold text-zinc-500">Collected chat has not matched known emotes yet.</div>
        </div>
      </div>
    )
  }
  return (
    <div className="overflow-hidden rounded border border-white/10 bg-white/[0.035]">
      <div className="grid grid-cols-[minmax(0,1fr)_90px_80px] gap-3 border-b border-white/10 px-3 py-2 text-[11px] font-black uppercase text-zinc-500">
        <span>Emote</span>
        <span>Provider</span>
        <span>Uses</span>
      </div>
      {emotes.slice(0, 24).map(emote => {
        const imageUrl = getEmoteImageUrl(emote)
        return (
          <button key={emote.key} type="button" onClick={() => onSelect(emote.key)} className={`grid w-full grid-cols-[minmax(0,1fr)_90px_80px] gap-3 border-b border-white/5 px-3 py-2 text-left text-sm font-bold last:border-b-0 items-center ${selected.has(emote.key) ? 'bg-amber-300/10 text-amber-100' : 'text-zinc-300 hover:bg-white/[0.05]'}`}>
            <span className="flex items-center gap-2 min-w-0">
              {imageUrl ? (
                <span className="grid h-7 w-7 shrink-0 place-items-center rounded bg-black/30 p-1">
                  <img src={imageUrl} alt={emote.name} className="max-h-full max-w-full object-contain" loading="lazy" />
                </span>
              ) : (
                <span className="h-7 w-7 shrink-0 rounded bg-white/5 border border-white/5" />
              )}
              <span className="truncate font-black text-white" title={emote.name}>{emote.name}</span>
            </span>
            <span className="uppercase text-zinc-500">{emote.provider || '-'}</span>
            <span>{count(emote.count)}</span>
          </button>
        )
      })}
    </div>
  )
}

function formatVodOffset(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  const parts = []
  if (h > 0) parts.push(`${h}h`)
  if (m > 0 || h > 0) parts.push(`${m}m`)
  parts.push(`${s}s`)
  return parts.join('')
}

function SelectedMomentPanel({
  rollup,
  startedAt,
  vodId,
  channel,
}: {
  rollup: AnalyticsMinuteRollup | null
  startedAt?: string
  vodId?: string
  channel: string
}) {
  const [clipStatus, setClipStatus] = useState<'idle' | 'loading' | 'success' | 'error'>('idle')
  const [clipError, setClipError] = useState('')
  const [createdJobId, setCreatedJobId] = useState<string | null>(null)

  if (!rollup) {
    return (
      <div className="rounded border border-white/10 bg-white/[0.02] p-4 text-center text-xs text-zinc-500 italic">
        Click on the graph above to select a moment and clip it or view the VOD.
      </div>
    )
  }

  const timeStr = new Date(rollup.minuteTs).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
  const dateStr = new Date(rollup.minuteTs).toLocaleDateString([], { month: 'short', day: 'numeric', year: 'numeric' })
  
  let offsetSeconds = 0
  let offsetStr = ''
  if (startedAt) {
    const startMs = new Date(startedAt).getTime()
    const currentMs = new Date(rollup.minuteTs).getTime()
    offsetSeconds = Math.max(0, Math.floor((currentMs - startMs) / 1000))
    offsetStr = formatVodOffset(offsetSeconds)
  }

  const vodUrl = vodId
    ? `https://www.twitch.tv/videos/${vodId}?t=${offsetStr}`
    : undefined

  const handleCreateClip = async () => {
    setClipStatus('loading')
    try {
      const data = await triggerClipperManual(
        channel,
        `Analytics Spike (${timeStr})`,
        60.0,
        30.0
      )
      setCreatedJobId(data.job_id)
      setClipStatus('success')
      window.dispatchEvent(new CustomEvent('streamclone:clip-created'))
    } catch (err: any) {
      setClipStatus('error')
      setClipError(err.message || 'Clipper service is unreachable.')
    }
  }

  return (
    <div className="rounded border border-amber-500/20 bg-[#0d0d12] p-4 relative overflow-hidden transition-all duration-300">
      <div className="absolute left-0 right-0 top-0 h-1 bg-gradient-to-r from-amber-500/40 via-amber-400 to-amber-500/40" />
      
      <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <span className="text-xs font-black uppercase text-amber-400 bg-amber-400/10 px-2 py-0.5 rounded">Selected Moment</span>
            <span className="text-sm font-black text-white">{timeStr} · {dateStr}</span>
          </div>
          <div className="mt-2 flex flex-wrap gap-4 text-xs font-bold text-zinc-400">
            {offsetStr && (
              <span>Stream offset: <strong className="text-zinc-200">{offsetStr}</strong></span>
            )}
            <span>Viewers: <strong className="text-zinc-200">{count(viewerValue(rollup))}</strong></span>
            <span>Chat activity: <strong className="text-zinc-200">{rollup.chatCount}/min</strong></span>
            <span>7TV Emotes: <strong className="text-zinc-200">{rollup.seventvEmoteCount}/min</strong></span>
          </div>
        </div>

        <div className="flex flex-wrap gap-3 items-center">
          {vodUrl ? (
            <a
              href={vodUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2 rounded bg-violet-600 px-4 py-2 text-xs font-black text-white hover:bg-violet-700 transition shadow-lg shadow-violet-600/20"
            >
              <span>Jump into VOD</span>
              <svg className="w-3.5 h-3.5 fill-current" viewBox="0 0 24 24">
                <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 14.5v-9l6 4.5-6 4.5z"/>
              </svg>
            </a>
          ) : (
            <button
              disabled
              title="VOD ID not resolved yet. Ensure Twitch Developer OAuth settings are fully configured."
              className="flex items-center gap-2 rounded bg-zinc-800 px-4 py-2 text-xs font-black text-zinc-500 cursor-not-allowed border border-white/5"
            >
              <span>VOD Offline</span>
            </button>
          )}

          <div>
            <button
              type="button"
              onClick={handleCreateClip}
              disabled={clipStatus === 'loading'}
              className={`flex items-center gap-2 rounded px-4 py-2 text-xs font-black transition ${
                clipStatus === 'loading'
                  ? 'bg-zinc-800 text-zinc-400 border border-white/5 cursor-wait'
                  : clipStatus === 'success'
                  ? 'bg-emerald-600 text-white'
                  : 'bg-cyan-600 text-white hover:bg-cyan-700 shadow-lg shadow-cyan-600/20'
              }`}
            >
              {clipStatus === 'loading' && <span>Queuing Clip...</span>}
              {clipStatus === 'success' && <span>✓ Clip Queued!</span>}
              {clipStatus === 'error' && <span>Retry Clip</span>}
              {clipStatus === 'idle' && <span>Clip Moment</span>}
            </button>
          </div>
        </div>
      </div>

      {clipStatus === 'error' && (
        <div className="mt-3 text-xs font-semibold text-red-400 rounded border border-red-500/10 bg-red-500/5 p-2.5">
          Error: {clipError}
        </div>
      )}
      {clipStatus === 'success' && (
        <div className="mt-3 text-xs font-semibold text-emerald-400 rounded border border-emerald-500/10 bg-emerald-500/5 p-2.5 flex justify-between items-center">
          <span>Clip request successfully sent.</span>
          {createdJobId && (
            <Link to={`/studio/${createdJobId}`} className="ml-2 underline text-emerald-300 font-bold hover:text-emerald-200">
              Open in Clip Studio →
            </Link>
          )}
        </div>
      )}
    </div>
  )
}

export default function Analytics() {
  const queryClient = useQueryClient()
  const { login = '', streamId = '' } = useParams<{ login: string; streamId?: string }>()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [selectedRollup, setSelectedRollup] = useState<AnalyticsMinuteRollup | null>(null)
  const [syncing, setSyncing] = useState(false)
  const [syncStatus, setSyncStatus] = useState<SyncStatus | null>(null)
  const [syncError, setSyncError] = useState<string | null>(null)
  const [syncNotice, setSyncNotice] = useState<string | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [lastRefreshedAt, setLastRefreshedAt] = useState<number | null>(null)
  const [activeClipsTab, setActiveClipsTab] = useState<'edits' | 'twitch'>('edits')

  useEffect(() => {
    setSelectedRollup(null)
    setLastRefreshedAt(null)
  }, [login, streamId])

  useEffect(() => {
    if (!login) return
    watchAnalyticsChannel(login).catch(() => undefined)
    getChannel(login)
      .then(channel => {
        if (!channel?.id) return
        return ensureChannelEmotes(login, channel.id, ['seventv', 'twitch'])
      })
      .catch(() => undefined)
  }, [login])

  const streamsQuery = useQuery({
    queryKey: ['analytics-streams', login],
    queryFn: () => getAnalyticsStreams(login, 20),
    enabled: Boolean(login),
    refetchInterval: 30000,
  })

  const historyQuery = useQuery({
    queryKey: ['channel-stream-history', login, 'all'],
    queryFn: () => getChannelStreamHistory(login, 'all'),
    enabled: Boolean(login),
    staleTime: 120_000,
    retry: 2,
  })

  const combinedStreams = useMemo(() => {
    const local = streamsQuery.data?.items ?? []
    const tracker = historyQuery.data?.items ?? []
    
    const mappedTracker: AnalyticsStream[] = tracker.map(s => ({
      streamId: s.id,
      broadcasterId: '',
      login: login,
      displayName: login,
      title: s.title,
      category: s.category || 'Live',
      startedAt: s.startedAt || '',
      endedAt: s.endedAt || '',
      lastSeenAt: '',
      currentViewers: 0,
      avgViewers: s.avgViewers,
      peakViewers: s.peakViewers,
      viewerSamples: 0,
      chatMessages: 0,
      totalEmoteUses: 0,
      seventvEmoteUses: 0,
      tags: [] as string[],
    }))

    const trackerById = new Map(mappedTracker.map(s => [s.streamId, s]))
    const merged = local.map(item => {
      const tracker = trackerById.get(item.streamId)
      if (!tracker) return item
      const missingViewers = item.avgViewers === 0 && item.peakViewers === 0
      const hasTrackerViewers = tracker.avgViewers > 0 || tracker.peakViewers > 0
      if (!missingViewers || !hasTrackerViewers) return item
      return {
        ...item,
        avgViewers: tracker.avgViewers,
        peakViewers: tracker.peakViewers,
      }
    })
    const localIds = new Set(merged.map(s => s.streamId))
    for (const s of mappedTracker) {
      if (!localIds.has(s.streamId)) {
        merged.push(s)
      }
    }

    return merged.sort((a, b) => {
      const aTime = a.startedAt ? Date.parse(a.startedAt) : 0
      const bTime = b.startedAt ? Date.parse(b.startedAt) : 0
      return bTime - aTime
    })
  }, [streamsQuery.data?.items, historyQuery.data?.items, login])

  const matchedStream = useMemo(() => {
    if (!streamId) return undefined
    
    // 1. Try to find by streamId (numeric or date alias)
    const exactMatch = combinedStreams.find(s => s.streamId === streamId)
    if (exactMatch) return exactMatch
    
    // 2. If streamId is a date alias (YYYY-MM-DD), find a stream starting on that date
    if (/^\d{4}-\d{2}-\d{2}$/.test(streamId)) {
      return combinedStreams.find(s => {
        if (!s.startedAt) return false
        const date = new Date(s.startedAt)
        if (isNaN(date.getTime())) return false
        
        // Match UTC date YYYY-MM-DD
        const utcDateStr = date.toISOString().slice(0, 10)
        if (utcDateStr === streamId) return true
        
        // Match local date YYYY-MM-DD
        const year = date.getFullYear()
        const month = String(date.getMonth() + 1).padStart(2, '0')
        const day = String(date.getDate()).padStart(2, '0')
        const localDateStr = `${year}-${month}-${day}`
        return localDateStr === streamId
      })
    }
    
    return undefined
  }, [streamId, combinedStreams])

  const dateSlugUnresolved = useMemo(() => {
    if (!streamId || !/^\d{4}-\d{2}-\d{2}$/.test(streamId)) return false
    if (streamsQuery.isLoading || historyQuery.isLoading) return false
    return !matchedStream
  }, [streamId, matchedStream, streamsQuery.isLoading, historyQuery.isLoading])

  const targetQueryStreamId = useMemo(() => {
    if (!streamId) return ''
    if (/^\d+$/.test(streamId)) {
      return streamId
    }
    if (matchedStream) {
      return matchedStream.streamId
    }
    // Date slug: wait while loading, never pass the date string as a stream ID
    if (/^\d{4}-\d{2}-\d{2}$/.test(streamId)) {
      if (streamsQuery.isLoading || historyQuery.isLoading) return undefined
      return undefined
    }
    return undefined
  }, [streamId, matchedStream, streamsQuery.isLoading, historyQuery.isLoading])

  const prefetchStreamDetail = useCallback((id: string) => {
    if (!login || !id) return
    queryClient.prefetchQuery({
      queryKey: ['analytics-detail', login, id],
      queryFn: () => getAnalyticsStream(id),
      staleTime: 120_000,
    })
  }, [login, queryClient])

  const detailQuery = useQuery({
    queryKey: ['analytics-detail', login, targetQueryStreamId],
    queryFn: () => targetQueryStreamId ? getAnalyticsStream(targetQueryStreamId) : getAnalyticsLive(login),
    enabled: Boolean(login && (streamId === '' || targetQueryStreamId)),
    refetchInterval: streamId ? false : 15000,
    retry: false,
    placeholderData: keepPreviousData,
    staleTime: 120_000,
    refetchOnWindowFocus: !streamId,
  })

  const gamesQuery = useQuery({
    queryKey: ['stream-games', targetQueryStreamId],
    queryFn: () => targetQueryStreamId ? getStreamGameSegments(targetQueryStreamId) : Promise.resolve([]),
    enabled: Boolean(targetQueryStreamId),
  })

  const handleRefresh = async () => {
    if (!login || refreshing) return
    setRefreshing(true)
    try {
      const isLiveView = !streamId || detailQuery.data?.state === 'live'
      if (isLiveView) {
        await watchAnalyticsChannel(login).catch(() => undefined)
      }
      const refetches: Array<Promise<unknown>> = [
        streamsQuery.refetch(),
        historyQuery.refetch(),
        detailQuery.refetch(),
      ]
      if (targetQueryStreamId) {
        refetches.push(gamesQuery.refetch())
      }
      await Promise.race([
        Promise.all(refetches),
        new Promise(resolve => setTimeout(resolve, 30_000)),
      ])
      setLastRefreshedAt(Date.now())
    } finally {
      setRefreshing(false)
    }
  }

  const detailHasChartData = (data?: AnalyticsStreamDetail | null) => (
    Boolean(data?.rollups?.some(rollupHasMinuteData))
  )

  const detailHasViewerData = (data?: AnalyticsStreamDetail | null) => {
    const rollups = data?.rollups ?? []
    const values = rollups.filter(point => !point.missing).map(viewerValue).filter(value => value > 0)
    if (values.length < 3) return false
    return Math.min(...values) !== Math.max(...values)
  }

  const viewersOnlySync = useMemo(() => {
    if (!targetQueryStreamId || streamId === '') return false
    const rollups = detailQuery.data?.rollups ?? []
    const hasChat = rollups.some(point => (point.chatCount ?? 0) > 0 || (point.seventvEmoteCount ?? 0) > 0)
    const values = rollups.filter(point => !point.missing).map(viewerValue).filter(value => value > 0)
    const hasRealViewers = values.length >= 3 && Math.min(...values) !== Math.max(...values)
    return hasChat && !hasRealViewers
  }, [targetQueryStreamId, streamId, detailQuery.data?.rollups])

  const waitForSyncedDetail = async (opts?: { viewersOnly?: boolean }) => {
    const maxAttempts = opts?.viewersOnly ? 8 : 12
    const intervalMs = opts?.viewersOnly ? 2000 : 5000
    const isReady = opts?.viewersOnly ? detailHasViewerData : detailHasChartData
    for (let attempt = 0; attempt < maxAttempts; attempt++) {
      await streamsQuery.refetch()
      const result = await detailQuery.refetch()
      if (isReady(result.data)) return true
      if (attempt < maxAttempts - 1) {
        await new Promise(resolve => setTimeout(resolve, intervalMs))
      }
    }
    return isReady(detailQuery.data)
  }

  const refreshSyncedQueries = async () => {
    await Promise.all([
      gamesQuery.refetch(),
      historyQuery.refetch(),
      detailQuery.refetch(),
    ])
  }

  const handleSync = async () => {
    if (!targetQueryStreamId) return
    const viewersOnly = viewersOnlySync
    setSyncing(true)
    setSyncError(null)
    setSyncNotice(null)
    setSyncStatus(null)
    try {
      const start = await startHistoricalSync(targetQueryStreamId, login, { viewersOnly })
      if (start.status) {
        setSyncStatus(start.status)
      }
      if (!start.accepted && start.status?.phase !== 'completed' && start.status?.phase !== 'failed') {
        setSyncNotice('Sync already running — showing live progress.')
      }
      const finalStatus = await pollSyncUntilDone(targetQueryStreamId, setSyncStatus)
      if (!finalStatus) {
        setSyncError('Lost sync status — try again or use Refresh data.')
        return
      }
      if (finalStatus.phase === 'failed') {
        setSyncError(finalStatus.error || 'Sync failed.')
        return
      }
      setSyncNotice(finalStatus.resultMessage || 'Sync completed.')
      const loaded = await waitForSyncedDetail({ viewersOnly })
      await refreshSyncedQueries()
      if (!loaded) {
        setSyncNotice(current => current || 'Sync finished but chart data is still loading. Click Refresh data.')
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'An error occurred during synchronization.'
      setSyncError(message)
    } finally {
      setSyncing(false)
    }
  }

  const historicalStream = useMemo(() => {
    if (!targetQueryStreamId || !historyQuery.data?.items) return undefined
    return historyQuery.data.items.find(s => s.id === targetQueryStreamId)
  }, [targetQueryStreamId, historyQuery.data?.items])

  const needsSync = Boolean(
    targetQueryStreamId
    && historicalStream
    && !detailQuery.data
    && !detailQuery.isLoading,
  )

  const detail = useMemo(() => {
    if (detailQuery.data) {
      const base = detailQuery.data
      if (historicalStream && base.stream) {
        const s = base.stream
        const missingViewers = s.peakViewers === 0 && s.avgViewers === 0
        const hasHistoricalViewers = historicalStream.peakViewers > 0 || historicalStream.avgViewers > 0
        if (missingViewers && hasHistoricalViewers) {
          return {
            ...base,
            stream: {
              ...s,
              avgViewers: historicalStream.avgViewers,
              peakViewers: historicalStream.peakViewers,
            },
          } satisfies AnalyticsStreamDetail
        }
      }
      return base
    }
    if (historicalStream) {
      return {
        channel: login,
        state: 'historical' as const,
        stream: {
          streamId: historicalStream.id,
          broadcasterId: '',
          login: login,
          displayName: login,
          title: historicalStream.title,
          category: historicalStream.category || 'Live',
          startedAt: historicalStream.startedAt || '',
          endedAt: historicalStream.endedAt || '',
          lastSeenAt: '',
          currentViewers: 0,
          avgViewers: historicalStream.avgViewers,
          peakViewers: historicalStream.peakViewers,
          viewerSamples: 0,
          chatMessages: 0,
          totalEmoteUses: 0,
          seventvEmoteUses: 0,
          tags: [] as string[],
        },
        rollups: [],
        topEmotes: [],
        sources: [{ source: 'twitchtracker', state: 'ready' as const, message: 'Historical stats from Twitch Tracker' }],
        updatedAt: Date.now(),
      }
    }
    return undefined
  }, [detailQuery.data, historicalStream, login])

  const stream = detail?.stream
  const selectedEmotes = selected

  const toggleSelected = (key: string) => {
    setSelected(current => {
      const next = new Set(current)
      if (next.has(key)) next.delete(key)
      else if (next.size < 5) next.add(key)
      return next
    })
  }

  return (
    <main className="min-h-screen overflow-y-auto bg-[#050507] text-zinc-100">
      <div className="mx-auto flex w-full max-w-[1500px] flex-col gap-4 px-4 py-4 lg:px-6">
        <header className="flex flex-col gap-3 border-b border-white/10 pb-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2 text-xs font-black uppercase text-zinc-500">
              <Link to="/" className="rounded bg-white/10 px-2 py-1 text-zinc-200 transition hover:bg-white/15">Streamclone</Link>
              <Link to={`/c/${encodeURIComponent(login)}`} className="rounded bg-violet-400/15 px-2 py-1 text-violet-100 transition hover:bg-violet-400/25">{login}</Link>
              <span className={`rounded px-2 py-1 ${detail?.state === 'live' ? 'bg-red-500/15 text-red-100' : 'bg-white/10 text-zinc-300'}`}>{detail?.state || 'loading'}</span>
            </div>
            <h1 className="mt-3 line-clamp-2 text-2xl font-black leading-tight text-white lg:text-4xl">{stream?.title || `${login} analytics`}</h1>
            <div className="mt-2 flex flex-wrap gap-2 text-sm font-bold text-zinc-500">
              {stream?.displayName ? <span>{stream.displayName}</span> : null}
              {stream?.category ? <span>{stream.category}</span> : null}
              {stream?.startedAt ? <span>Started {relativeTime(stream.startedAt)}</span> : null}
              <span>
                {lastRefreshedAt
                  ? `Refreshed ${relativeTime(lastRefreshedAt)}`
                  : `Updated ${detail?.updatedAt ? relativeTime(detail.updatedAt) : '-'}`}
              </span>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <button
              type="button"
              onClick={handleRefresh}
              disabled={refreshing}
              className="rounded border border-white/10 bg-white/[0.05] px-3 py-1.5 text-[11px] font-black uppercase text-zinc-300 transition hover:bg-white/10 hover:text-white disabled:opacity-50"
            >
              {refreshing ? 'Refreshing…' : 'Refresh data'}
            </button>
            <SourcePills sources={detail?.sources} />
          </div>
        </header>

        <section className="grid grid-cols-2 gap-3 lg:grid-cols-6">
          <StatCard label="Current" value={count(stream?.currentViewers)} tone="text-cyan-100" />
          <StatCard label="Average" value={count(stream?.avgViewers)} />
          <StatCard label="Peak" value={count(stream?.peakViewers)} />
          <StatCard label="Chat" value={count(stream?.chatMessages)} tone="text-violet-100" />
          <StatCard label="7TV Uses" value={count(stream?.seventvEmoteUses)} tone="text-emerald-100" />
          <StatCard label="Duration" value={duration(stream)} />
        </section>

        {stream?.tags?.length ? (
          <div className="flex flex-wrap gap-2">
            {stream.tags.map(tag => <span key={tag} className="rounded border border-white/10 bg-white/[0.045] px-2 py-1 text-xs font-bold text-zinc-300">{tag}</span>)}
          </div>
        ) : null}

        {dateSlugUnresolved ? (
          <div className="rounded border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm font-bold text-red-200">
            No stream found for {streamId}. Pick a stream from the sidebar or open analytics with a numeric stream ID.
          </div>
        ) : null}

        <div className="grid gap-4 xl:grid-cols-[260px_minmax(0,1fr)_320px]">
          <aside className="min-w-0 xl:sticky xl:top-4 xl:self-start">
            <StreamSidebar
              login={login}
              streams={combinedStreams}
              activeID={stream?.streamId || streamId}
              isLiveView={!streamId}
              liveState={detail?.state}
              onPrefetchStream={prefetchStreamDetail}
            />
          </aside>
          <section className="min-w-0 space-y-4">
            <AnalyticsChart
              detail={detail}
              selectedEmotes={selectedEmotes}
              onSelectEmote={toggleSelected}
              selectedRollup={selectedRollup}
              onSelectRollup={setSelectedRollup}
              syncing={syncing}
              syncStatus={syncStatus}
              syncError={syncError}
              syncNotice={syncNotice}
              onSync={handleSync}
              notInAnalyticsDb={needsSync}
              onRefresh={handleRefresh}
              refreshing={refreshing}
              loading={detailQuery.isLoading && !historicalStream}
              games={gamesQuery.data ?? []}
              canSync={Boolean(streamId) || needsSync}
              isLive={detail?.state === 'live'}
            />
            <SelectedMomentPanel
              rollup={selectedRollup}
              startedAt={stream?.startedAt}
              vodId={detail?.vodId}
              channel={login}
            />
          </section>
          <aside className="space-y-4">
            <div className="rounded border border-white/10 bg-white/[0.035] overflow-hidden">
              <div className="flex border-b border-white/10 text-[11px] font-black uppercase bg-white/[0.015]">
                <button
                  onClick={() => setActiveClipsTab('edits')}
                  className={`flex-1 py-2 text-center transition border-r border-white/10 ${
                    activeClipsTab === 'edits'
                      ? 'bg-white/[0.04] text-white font-black'
                      : 'text-zinc-500 hover:text-zinc-300'
                  }`}
                >
                  Clipper Edits
                </button>
                <button
                  onClick={() => setActiveClipsTab('twitch')}
                  className={`flex-1 py-2 text-center transition ${
                    activeClipsTab === 'twitch'
                      ? 'bg-white/[0.04] text-white font-black'
                      : 'text-zinc-500 hover:text-zinc-300'
                  }`}
                >
                  Twitch Clips
                </button>
              </div>
              
              {activeClipsTab === 'edits' ? (
                <RecentClipsList login={login} isTab={true} />
              ) : (
                <TwitchDayClipsList
                  login={login}
                  startedAt={stream?.startedAt || ''}
                  endedAt={stream?.endedAt || new Date().toISOString()}
                />
              )}
            </div>

            <TopEmoteTable emotes={detail?.topEmotes ?? []} selected={selectedEmotes} onSelect={toggleSelected} />
          </aside>
        </div>
      </div>
    </main>
  )
}

function RecentClipsList({ login, isTab = false }: { login: string; isTab?: boolean }) {
  const [jobs, setJobs] = useState<ClipperJob[]>([])
  const [loading, setLoading] = useState(true)

  const fetchJobs = async () => {
    try {
      const res = await getClipperJobs(50, login)
      const filtered = res.items || []
      setJobs(filtered)
    } catch (err) {
      console.error('Failed to fetch clipper jobs', err)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchJobs()
    const handleClipCreated = () => {
      fetchJobs()
    }
    window.addEventListener('streamclone:clip-created', handleClipCreated)
    const timer = setInterval(fetchJobs, 5000)
    return () => {
      window.removeEventListener('streamclone:clip-created', handleClipCreated)
      clearInterval(timer)
    }
  }, [login])

  if (loading) {
    return <div className="text-zinc-500 text-xs font-semibold px-3 py-2">Loading clips list...</div>
  }

  if (!jobs.length) {
    return (
      <div className={`p-3 text-center text-xs text-zinc-500 ${!isTab ? 'rounded border border-white/10 bg-white/[0.035]' : ''}`}>
        No clips created for this streamer yet. Click on the graph to clip moments!
      </div>
    )
  }

  const itemsContent = (
    <div className="divide-y divide-white/5 max-h-[300px] overflow-y-auto">
      {jobs.map(job => (
        <div key={job.id} className="p-3">
          <div className="text-sm font-black text-white line-clamp-1" title={job.title || `Clip ${job.id.substring(0, 8)}`}>
            {job.title || `Clip ${job.id.substring(0, 8)}`}
          </div>
          <div className="mt-1 flex justify-between items-center text-[10px] font-semibold">
            <span className={`px-1.5 py-0.5 rounded text-[9px] uppercase font-black ${
              job.state === 'ready' ? 'bg-emerald-500/10 text-emerald-400' :
              job.state === 'failed' ? 'bg-red-500/10 text-red-400' :
              'bg-amber-500/10 text-amber-400 animate-pulse'
            }`}>
              {job.state}
            </span>
            <span className="text-zinc-500">{relativeTime(job.created_at)}</span>
          </div>
          {job.state === 'failed' && (
            <div className="mt-1 text-[10px] font-semibold text-red-300/90 line-clamp-2">
              {describeClipperFailure(job)}
            </div>
          )}
          <div className="mt-2 flex gap-2">
            <Link
              to={`/studio/${job.id}`}
              className="flex-1 rounded bg-violet-600/20 border border-violet-500/30 px-2 py-1 text-center text-[10px] font-bold text-violet-200 transition hover:bg-violet-600/35"
            >
              Open in Studio
            </Link>
            {job.state === 'ready' && (
              <a
                href={getClipperFinalVideoUrl(job.id)}
                download
                className="flex-1 rounded bg-emerald-600/20 border border-emerald-500/30 px-2 py-1 text-center text-[10px] font-bold text-emerald-200 transition hover:bg-emerald-600/35"
              >
                Download final
              </a>
            )}
          </div>
        </div>
      ))}
    </div>
  )

  if (isTab) {
    return itemsContent
  }

  return (
    <div className="rounded border border-white/10 bg-white/[0.035]">
      <div className="border-b border-white/10 px-3 py-2 text-[11px] font-black uppercase text-zinc-500">
        Recent Clips &amp; Edits
      </div>
      {itemsContent}
    </div>
  )
}

function TwitchDayClipsList({ login, startedAt, endedAt }: { login: string; startedAt: string; endedAt: string }) {
  const { data, isLoading, error } = useQuery({
    queryKey: ['twitch-day-clips', login, startedAt, endedAt],
    queryFn: () => getTwitchDayClips(login, startedAt, endedAt),
    enabled: Boolean(login && startedAt && endedAt),
  })

  if (isLoading) {
    return <div className="text-zinc-500 text-xs font-semibold px-3 py-2">Loading Twitch clips...</div>
  }

  if (error || !data?.items?.length) {
    return (
      <div className="p-3 text-center text-xs text-zinc-500">
        No popular Twitch clips found for this stream period.
      </div>
    )
  }

  return (
    <div className="divide-y divide-white/5 max-h-[300px] overflow-y-auto">
      {data.items.map(clip => (
        <div key={clip.id} className="p-3 flex gap-3 items-start">
          <a
            href={clip.url}
            target="_blank"
            rel="noopener noreferrer"
            className="relative shrink-0 block w-24 aspect-video rounded overflow-hidden border border-white/10 hover:border-violet-500 transition"
          >
            <img src={clip.thumbnailUrl} alt={clip.title} className="w-full h-full object-cover" />
            <span className="absolute bottom-1 right-1 bg-black/80 px-1 py-0.5 rounded text-[9px] font-black text-white">
              {(clip.durationSeconds ?? 0).toFixed(1)}s
            </span>
          </a>
          <div className="min-w-0 flex-1">
            <a
              href={clip.url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-sm font-black text-white hover:text-violet-400 transition line-clamp-2 leading-snug"
              title={clip.title}
            >
              {clip.title}
            </a>
            <div className="mt-1 flex flex-wrap items-center gap-x-2 text-[10px] font-semibold text-zinc-400">
              <span>By {clip.creatorName}</span>
              <span className="text-zinc-600">•</span>
              <span>{count(clip.viewCount)} views</span>
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

