import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'

import { studioPath } from '../utils/studioLink'

import {
  ensureChannelEmotes,
  getAnalyticsStream,
  getAnalyticsStreams,
  getPulseBookmarks,
  getPulseStreamRecap,
  getTimeseriesStatus,
  createPulseBookmark,
  deletePulseBookmark,
  prefetchAnalyticsTracker,
  getChannel,
  getChannelStreamHistory,
  watchAnalyticsChannel,
  getClipperJobs,
  getClipperFinalVideoUrl,
  describeClipperFailure,
  describeClipperJobState,
  isClipperAuthFailure,
  isClipperAuthFailureMessage,
  isClipperJobRetryable,
  retryClipperJob,
  getSyncStatus,
  startHistoricalSync,
  getStreamGameSegments,
  getReplayHeatmap,
  getReplayHeatmapDetail,
  getClipperTwitchStatus,
  triggerClipperManual,
  type SyncPhase,
  type SyncStatus,
  type SyncChatProgress,
  getTwitchDayClips,
  getSetupWelcome,
  type AnalyticsStream,
  type AnalyticsStreamDetail,
  type AnalyticsTopEmote,
  type PulseBookmark,
  type PulseStreamRecap,
  type SourceStatus,
  type AnalyticsMinuteRollup,
  type ClipperJob,
} from '../api'
import TierIndicator from './analytics/TierIndicator'
import ClipsTabEmptyState from './analytics/ClipsTabEmptyState'
import AnalyticsChart, { type AnalyticsViewMode, type RightPanelTab } from './analytics/AnalyticsChart.tsx'
import type { ReplayHeatmapDetailPoint, ReplayHeatmapPoint } from '../types/heatmap'
import { formatHeatOffset, LIVE_HEAT_RANKED_SUBTITLE } from '@streamclone/pulse-core'
import { syncCtaLabel, type SyncStreamState } from '../utils/syncLabel'
import {
  analyticsStreamPathSlug,
  pickSyncedLiveStreamTarget,
  streamIsSidebarVisible,
} from '../utils/syncedLiveStream.ts'
import {
  classifyStatCards,
  STAT_PLACEHOLDER_MUTED_CLASS,
  type StreamCollectionState,
} from '../utils/statCards'
import {
  isActiveLiveCollectorStream,
  isPlaceholderStreamTitle,
  isSyncPrefetchPlaceholder,
} from '../utils/analyticsStreamRow'

import { coreMinuteChartsNeedScraper } from '../setupProfile'
import { buildTwitchVodUrl, resolveAnalyticsVodId } from '../utils/twitchVodUrl'
import { buildVodDeepLink } from '@streamclone/pulse-core'
import { buildMomentScoreModel } from '@streamclone/pulse-core'
import {
  computeMomentScore100,
  computeStreamBaselines,
  detectPickReason,
  heatmapEmotesFromRollup,
  topEmotesFromRollup,
} from '@streamclone/pulse-core'
import {
  analyzeViewerCoverage,
  computeRollupChatStats,
  computeRollupViewerStats,
  minuteEmoteTotal,
  rollupHasMinuteData,
  rollupsHaveViewerData,
  viewerValue,
} from './analytics/chartRollupUtils'
import { useAnalyticsLive } from '../hooks/useAnalyticsLive'
import { CoreMinuteChartsNotice } from './OptionalServicesPanel'
import ClipperAuthHelp from './ClipperAuthHelp'
import StackStatusButton from './StackStatusButton'
import {
  emoteProviderLabel,
  emoteProviderTone,
  parseEmoteKey,
} from '../emoteUtils'
import {
  chatFetchCleanupLabel,
  chatFetchDetailLabel,
  viewerStepBadge,
  viewerTrackerStepProgress,
} from '../utils/syncProgressLabels'
import { pulseDashboardUrl } from '../utils/pulseDashboard.ts'
import { resolveEmoteImageUrl } from '../utils/emoteImageUrl.ts'

function getEmoteImageUrl(emote: { provider?: string; id?: string; imageUrl?: string }) {
  const url = resolveEmoteImageUrl({
    provider: emote.provider,
    id: emote.id,
    imageUrl: emote.imageUrl,
    scale: '1x',
  })
  return url || undefined
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

function rollupOffsetSeconds(rollup: AnalyticsMinuteRollup, startedAt?: string): number {
  if (!startedAt) return 0
  const startMs = new Date(startedAt).getTime()
  const minuteMs = new Date(rollup.minuteTs).getTime()
  if (!Number.isFinite(startMs) || !Number.isFinite(minuteMs)) return 0
  return Math.max(0, Math.floor((minuteMs - startMs) / 1000))
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

function ChatCoverageBadge({ detail }: { detail?: AnalyticsStreamDetail }) {
  const pct = detail?.chatCoveragePct ?? detail?.chatCoverage?.coveragePct
  if (pct === undefined || pct <= 0) return null
  const partial = detail?.chatCoverage?.partial
  const title = partial
    ? `Chat spans ${detail?.chatCoverage?.chatSpanMinutes ?? 0} of ${detail?.chatCoverage?.streamSpanMinutes ?? 0} stream minutes — re-sync later for more`
    : 'Chat rollups cover most of the stream timeline'
  return (
    <span
      title={title}
      className={`rounded border px-2 py-1 text-[10px] font-black uppercase ${
        partial ? 'border-amber-400/25 bg-amber-500/10 text-amber-200' : 'border-emerald-400/20 bg-emerald-500/10 text-emerald-300'
      }`}
    >
      {Math.round(pct)}% chat coverage
    </span>
  )
}

function streamStateLabel(state?: AnalyticsStreamDetail['state'] | 'not found' | 'loading', isHistoricalRoute = false) {
  if (state === 'not found') return 'not found'
  if (isHistoricalRoute && (state === 'live' || state === 'loading' || !state)) return 'historical'
  if (state === 'live') return 'live'
  if (state === 'syncing') return 'syncing'
  if (state === 'historical') return 'historical'
  if (state === 'not_collected') return 'stats only'
  return state || 'loading'
}

function displayStreamTitle(stream?: AnalyticsStream, login?: string, fallbacks: Array<string | undefined> = []) {
  if (!isPlaceholderStreamTitle(stream?.title)) return stream!.title!.trim()
  for (const candidate of fallbacks) {
    const trimmed = candidate?.trim() ?? ''
    if (trimmed && !isPlaceholderStreamTitle(trimmed)) return trimmed
  }
  return `${login ?? stream?.login ?? 'Stream'} analytics`
}

function StatCard({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <div className="rounded border border-white/10 bg-white/[0.035] p-3">
      <div className="text-[11px] font-black uppercase text-zinc-500">{label}</div>
      <div className={`mt-1 truncate text-xl font-black ${tone || 'text-white'}`}>{value}</div>
    </div>
  )
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

function formatVodClock(sec?: number) {
  if (sec == null || sec < 0) return '0s'
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  if (h > 0) return `${h}h ${String(m).padStart(2, '0')}m`
  if (m > 0) return `${m}m ${String(s).padStart(2, '0')}s`
  return `${s}s`
}

function chatTimelineProgress(chat?: SyncChatProgress) {
  const total = chat?.vodDurationSec ?? 0
  if (total <= 0) return null
  const timeline = Math.max(0, chat?.timelineSec ?? 0)
  const pct = Math.min(100, Math.round((timeline / total) * 100))
  return { pct, timeline, total }
}

function chatTimelineEta(chat?: SyncChatProgress, startedAt?: string) {
  const progress = chatTimelineProgress(chat)
  if (!progress || progress.timeline <= 0 || !startedAt) return ''
  if (progress.pct >= 95 || chat?.indexPhase === 'tokenizing' || chat?.indexPhase === 'writing') return ''
  const elapsedMs = Math.max(1, Date.now() - Date.parse(startedAt))
  const rate = progress.timeline / elapsedMs
  if (rate <= 0) return ''
  const remainingSec = Math.max(0, progress.total - progress.timeline)
  const remainingMin = Math.ceil((remainingSec / rate) / 60_000)
  return remainingMin > 0 ? `~${remainingMin} min remaining` : ''
}

function chatIndexProgress(chat?: SyncChatProgress, rollupsWritten = 0) {
  const expected = chat?.rollupsExpected ?? 0
  if (expected <= 0) return null
  const written = Math.max(0, rollupsWritten)
  const pct = Math.min(100, Math.round((written / expected) * 100))
  return { pct, written, expected }
}

function chatFetchProgressState(
  status: SyncStatus,
  chatTimeline: ReturnType<typeof chatTimelineProgress>,
  segmentDone: number,
  segmentTotal: number,
) {
  const indexPhase = status.chat?.indexPhase
  const segmentsTrackable = segmentTotal > 1
  const segmentsComplete = !segmentsTrackable || segmentDone >= segmentTotal
  const segmentsIncomplete = status.chat?.segmentsIncomplete ?? Math.max(0, segmentTotal - segmentDone)
  const cleanupPhase = status.chat?.cleanupPhase
  const timelinePct = chatTimeline?.pct ?? 0
  const chatFetchStarted = Boolean(
    status.chat?.active ||
    status.phase === 'fetching_comments' ||
    indexPhase === 'fetching' ||
    indexPhase === 'tokenizing' ||
    indexPhase === 'writing' ||
    segmentDone > 0 ||
    (status.chat?.commentsFetched ?? 0) > 0,
  )
  const chatFetchDone = !status.viewersOnly && (
    segmentsTrackable
      ? segmentsComplete
      : (
          indexPhase === 'done' ||
          status.phase === 'writing_rollups' ||
          status.phase === 'completed' ||
          status.phase === 'exporting_archive' ||
          status.phase === 'export_pending' ||
          indexPhase === 'tokenizing' ||
          indexPhase === 'writing'
        )
  )
  const chatFetchActive = !status.viewersOnly && !chatFetchDone && chatFetchStarted && (
    Boolean(status.chat?.active) ||
    status.phase === 'fetching_comments' ||
    (segmentsTrackable && !segmentsComplete) ||
    indexPhase === 'fetching'
  )
  const segmentCleanup = segmentsTrackable && !segmentsComplete && (
    cleanupPhase === 'parallel_cleanup' ||
    cleanupPhase === 'serial_retry' ||
    segmentsIncomplete > 0 ||
    (timelinePct >= 99 && segmentDone > 0)
  )
  return { chatFetchDone, chatFetchActive, chatFetchStarted, segmentsTrackable, segmentsComplete, segmentCleanup, timelinePct, segmentsIncomplete, cleanupPhase }
}

function chatSegmentPlanLabel(chat?: SyncChatProgress) {
  if (!chat) return ''
  const total = chat.segmentsTotal ?? 0
  const hotSplits = chat.hotSplits ?? 0
  const initial = chat.initialSegments ?? (hotSplits > 0 && total > 0 ? Math.max(0, total - hotSplits) : 0)
  const parts: string[] = []
  if (total > 0) {
    if (initial > 0 && initial !== total) {
      parts.push(`${initial.toLocaleString()} → ${total.toLocaleString()} segments`)
    } else {
      parts.push(`${total.toLocaleString()} segments`)
    }
  } else if (initial > 0) {
    parts.push(`${initial.toLocaleString()} segments`)
  }
  if ((chat.effectiveSegmentSec ?? 0) > 0) {
    parts.push(`${chat.effectiveSegmentSec?.toLocaleString()}s base`)
  }
  return parts.join(' · ')
}

function syncIndexPhaseDetail(chat?: SyncChatProgress, rollupsWritten = 0) {
  switch (chat?.indexPhase) {
    case 'tokenizing':
      return 'Tokenizing emotes…'
    case 'writing': {
      const index = chatIndexProgress(chat, rollupsWritten)
      return index ? `Writing ${index.written}/${index.expected} chat minutes` : 'Writing chat minutes…'
    }
    case 'finalizing':
      return 'Finalizing chat index…'
    case 'done':
      return 'Chat indexed'
    default:
      return ''
  }
}

function shouldRefetchChartDuringSync(status: SyncStatus | null, viewersOnly: boolean) {
  if (!status) return false
  const viewerReady = status.viewerStatus === 'ok'
    || status.viewerStatus === 'pending_backfill'
    || status.viewerStatus === 'backfilling'
  if (viewersOnly) {
    return viewerReady || (status.rollupsWritten ?? 0) > 0
  }
  if (viewerReady) {
    return true
  }
  if ((status.rollupsWritten ?? 0) > 0) {
    return ['parsing_tracker', 'resolving_vod', 'fetching_comments', 'writing_rollups'].includes(status.phase)
  }
  if ((status.chat?.segmentsDone ?? 0) > 0 || (status.chat?.commentsFetched ?? 0) > 0) {
    return status.phase === 'fetching_comments'
  }
  return false
}

function syncPollChartCallbacks(
  refetchChart: () => void,
  viewersOnly: boolean,
) {
  let lastRefetchKey = ''
  return {
    onPhase: (phase: SyncPhase) => {
      if (['parsing_tracker', 'resolving_vod', 'fetching_comments', 'writing_rollups', 'exporting_archive', 'export_pending', 'completed'].includes(phase)) {
        refetchChart()
      }
    },
    onProgress: (status: SyncStatus) => {
      if (!shouldRefetchChartDuringSync(status, viewersOnly)) return
      const key = `${status.phase}:${status.rollupsWritten ?? 0}:${status.viewerStatus ?? ''}:${status.chat?.timelineSec ?? 0}:${status.chat?.indexPhase ?? ''}`
      if (key === lastRefetchKey) return
      lastRefetchKey = key
      refetchChart()
    },
  }
}

function friendlySyncNotice(message: string | null | undefined, fallback = 'Sync completed.') {
  const text = message?.trim()
  if (!text) return fallback
  if (text === 'Stream synced (viewers only — VOD comments unavailable)') {
    return 'Viewer timeline synced; Twitch VOD comments are unavailable right now.'
  }
  if (text === 'Stream synced (viewers only — VOD not found in Helix/TwitchTracker; chat/7TV skipped)') {
    return 'Viewer timeline synced; VOD was not found in Helix/TwitchTracker, so chat/7TV were skipped.'
  }
  if (text === 'Stream synced (viewers only — VOD chat skipped: broadcaster ID missing; re-sync after Helix credentials are set)') {
    return 'Viewer timeline synced; VOD chat skipped because broadcaster ID is missing. Re-sync after Helix credentials are set.'
  }
  return text
}

type SegmentCellState = 'done' | 'active' | 'queued' | 'retry'

function segmentGridBuckets(
  total: number,
  done: number,
  active: boolean,
  throttled: boolean,
  maxBuckets = 48,
): SegmentCellState[] {
  if (total <= 0) return []
  const buckets = Math.min(total, maxBuckets)
  const segsPerBucket = total / buckets
  return Array.from({ length: buckets }, (_, i) => {
    const bucketStart = Math.floor(i * segsPerBucket)
    const bucketEnd = Math.floor((i + 1) * segsPerBucket)
    if (bucketEnd <= done) return 'done'
    if (bucketStart <= done && done < bucketEnd) {
      if (throttled) return 'retry'
      if (active) return 'active'
    }
    return 'queued'
  })
}

function syncOverallEta(status: SyncStatus, chatTimeline: ReturnType<typeof chatTimelineProgress>) {
  const chatEta = chatTimelineEta(status.chat, status.startedAt)
  if (chatEta) return chatEta
  if (status.chat?.indexPhase === 'tokenizing') return 'Indexing emotes…'
  if (status.chat?.indexPhase === 'writing') return 'Writing rollups & emotes…'
  if (status.chat?.indexPhase === 'finalizing') return 'Finalizing chat index…'
  if (chatTimeline && chatTimeline.pct < 100 && status.chat?.active) return ''
  return ''
}

function syncOverallProgress(
  status: SyncStatus,
  viewerStepState: 'done' | 'active' | 'pending' | 'failed' | 'partial',
  chatStepState: 'done' | 'active' | 'pending',
  rollupStepState: 'done' | 'active' | 'pending',
  viewerPct: number,
  chatFetchPct: number,
  rollupPct: number,
  chatOnlyPath: boolean,
) {
  if (status.phase === 'completed') return { pct: 100, stageLabel: 'Complete' }
  if (status.phase === 'export_pending') return { pct: 100, stageLabel: 'Archive pending' }
  if (status.phase === 'failed') return { pct: 0, stageLabel: 'Failed' }

  const lerp = (range: [number, number], innerPct: number) =>
    range[0] + ((range[1] - range[0]) * Math.max(0, Math.min(100, innerPct))) / 100

  const rollupRange: [number, number] = [85, 100]
  const chatRange: [number, number] = chatOnlyPath ? [5, 85] : [30, 85]
  const viewerRange: [number, number] = [10, 30]
  const vodRange: [number, number] = [0, 10]

  if (rollupStepState === 'done') return { pct: 100, stageLabel: 'Complete' }
  if (rollupStepState === 'active') {
    return { pct: lerp(rollupRange, rollupPct), stageLabel: 'Rollups & emotes' }
  }
  if (chatStepState === 'active') {
    return { pct: lerp(chatRange, chatFetchPct), stageLabel: 'VOD chat fetch' }
  }
  if (chatStepState === 'done') {
    return { pct: chatRange[0], stageLabel: 'Rollups & emotes' }
  }
  if (!chatOnlyPath) {
    if (viewerStepState === 'done' || viewerStepState === 'partial') {
      return { pct: chatRange[0], stageLabel: 'VOD chat fetch' }
    }
    if (viewerStepState === 'active') {
      return { pct: lerp(viewerRange, viewerPct), stageLabel: 'Viewers (TwitchTracker)' }
    }
  }
  if (status.phase === 'resolving_vod') {
    return { pct: lerp(vodRange, 60), stageLabel: 'VOD lookup' }
  }
  if (status.phase === 'starting') {
    return { pct: lerp(vodRange, 15), stageLabel: 'Initial setup' }
  }
  if (status.phase === 'exporting_archive') {
    return { pct: 98, stageLabel: 'Archive export' }
  }
  return { pct: vodRange[0], stageLabel: chatOnlyPath ? 'VOD lookup' : 'Initial setup' }
}

function isTerminalSyncPhase(phase: SyncPhase | undefined) {
  return phase === 'completed' || phase === 'export_pending' || phase === 'failed'
}

function SyncStepIcon({ state }: { state: 'done' | 'active' | 'pending' | 'failed' | 'partial' }) {
  if (state === 'failed') {
    return (
      <span className="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-amber-500/15 text-amber-300">
        <svg className="h-3 w-3" viewBox="0 0 12 12" fill="none" aria-hidden>
          <path d="M6 2.5v3.25M6 8.75h.01" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
        </svg>
      </span>
    )
  }
  if (state === 'partial') {
    return (
      <span className="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-amber-500/15 text-amber-300">
        <svg className="h-3 w-3" viewBox="0 0 12 12" fill="none" aria-hidden>
          <path d="M2.5 6h7M6 2.5v7" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
        </svg>
      </span>
    )
  }
  if (state === 'done') {
    return (
      <span className="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-emerald-500/15 text-emerald-400">
        <svg className="h-3 w-3" viewBox="0 0 12 12" fill="none" aria-hidden>
          <path d="M2.5 6l2.5 2.5 4.5-4.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </span>
    )
  }
  if (state === 'active') {
    return (
      <span className="inline-flex h-5 w-5 shrink-0 items-center justify-center">
        <span className="h-4 w-4 animate-spin rounded-full border-2 border-violet-500/25 border-t-violet-400" />
      </span>
    )
  }
  return (
    <span className="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-dashed border-zinc-600/80 text-[9px] text-zinc-600">
      ○
    </span>
  )
}

function useActiveSyncTick(active: boolean, intervalMs = 1000): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!active) return
    const id = window.setInterval(() => setNow(Date.now()), intervalMs)
    return () => window.clearInterval(id)
  }, [active, intervalMs])
  return now
}

function SyncProgressBar({
  pct,
  tone,
  pending = false,
}: {
  pct: number
  tone: 'green' | 'violet' | 'amber' | 'muted'
  pending?: boolean
}) {
  const fillClass = {
    green: 'bg-emerald-500',
    violet: 'bg-violet-500',
    amber: 'bg-amber-400',
    muted: 'bg-zinc-600',
  }[tone]
  return (
    <div className={`mt-2 h-1.5 overflow-hidden rounded-full ${pending ? 'border border-dashed border-zinc-700/80 bg-zinc-900/40' : 'bg-white/[0.07]'}`}>
      {!pending ? (
        <div
          className={`h-full rounded-full transition-[width] duration-500 ${fillClass}`}
          style={{ width: `${Math.max(pct > 0 ? 2 : 0, Math.min(100, pct))}%` }}
        />
      ) : null}
    </div>
  )
}

function SegmentGridLegend() {
  const items: Array<{ tone: string; label: string }> = [
    { tone: 'bg-emerald-500/80', label: 'Done' },
    { tone: 'bg-violet-500/80', label: 'Active' },
    { tone: 'bg-zinc-600/50', label: 'Queued' },
    { tone: 'bg-red-500/70', label: 'Retry' },
  ]
  return (
    <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-[9px] font-semibold text-zinc-500">
      {items.map(item => (
        <span key={item.label} className="inline-flex items-center gap-1">
          <span className={`inline-block h-2 w-2 rounded-sm ${item.tone}`} />
          {item.label}
        </span>
      ))}
    </div>
  )
}

function SyncProgressPanel({
  status,
  chartChatMinutes = 0,
  viewerDataFromExisting = false,
  chatOnlyPath = false,
}: {
  status: SyncStatus | null
  chartChatMinutes?: number
  viewerDataFromExisting?: boolean
  chatOnlyPath?: boolean
}) {
  const trackerTickActive = Boolean(
    status?.tracker?.active
    || status?.viewerStatus === 'backfilling'
    || (status != null && ['scraping_tracker', 'parsing_tracker'].includes(status.phase)),
  )
  const nowMs = useActiveSyncTick(trackerTickActive)

  if (!status) return null
  if (status.stale && !isTerminalSyncPhase(status.phase)) {
    return (
      <div className="w-full rounded-xl border border-amber-400/30 bg-amber-400/10 px-4 py-3 text-left">
        <div className="text-xs font-black uppercase tracking-wide text-amber-200">Sync interrupted</div>
        <div className="mt-1 text-[11px] font-semibold text-amber-100/90">
          The analytics service restarted or lost the sync worker. Click sync again to retry.
        </div>
        {status.error ? <div className="mt-2 text-xs font-bold text-red-300">{status.error}</div> : null}
      </div>
    )
  }

  const chatTimeline = chatTimelineProgress(status.chat)
  const chatIndex = chatIndexProgress(status.chat, status.rollupsWritten ?? 0)
  const indexPhase = status.chat?.indexPhase
  const isIndexing = indexPhase === 'tokenizing' || indexPhase === 'writing' || indexPhase === 'finalizing'
  const viewerBackfillPending = status.viewerStatus === 'pending_backfill' || status.viewerStatus === 'backfilling'
  const viewersChartReady = status.viewerStatus === 'ok'
    || status.viewerStatus === 'skipped'
    || (status.rollupsWritten ?? 0) > 0
    || (viewerBackfillPending && viewerDataFromExisting)
  const segmentTotal = status.chat?.segmentsTotal ?? 0
  const segmentDone = status.chat?.segmentsDone ?? 0
  const {
    chatFetchDone,
    chatFetchActive,
    segmentsTrackable,
    segmentCleanup,
    timelinePct: chatTimelinePct,
    segmentsIncomplete,
  } = chatFetchProgressState(status, chatTimeline, segmentDone, segmentTotal)
  const chatFetchPct = segmentsTrackable
    ? Math.round((segmentDone / segmentTotal) * 100)
    : (chatTimeline?.pct ?? 0)
  const chatPhaseLabel = chatFetchDetailLabel(
    status,
    chatFetchDone,
    segmentsTrackable,
    segmentCleanup,
    chatTimelinePct,
    segmentDone,
    segmentTotal,
  )
  const chatCleanupLabel = chatFetchCleanupLabel(status.chat, segmentDone, segmentTotal, {
    timelinePct: chatTimelinePct,
    segmentCleanup,
  })
  const chatTailFinalizing = chatPhaseLabel === 'Finalizing chat index'
  const chatTailWriting = chatPhaseLabel === 'Writing rollups & emotes'
  const chatSectionTitle = chatTailFinalizing || chatTailWriting ? chatPhaseLabel : 'VOD Chat Fetch (Twitch GQL)'
  const chatSectionDetail = chatTailFinalizing
    ? (chatCleanupLabel || 'Finishing segment cleanup')
    : chatTailWriting
      ? 'All comments fetched'
      : chatPhaseLabel
  const chatTailSegmentPlan = chatTimelinePct >= 100 ? chatSegmentPlanLabel(status.chat) : ''

  const viewerFailed = status.viewerStatus === 'failed'
  const viewerStepUsesExisting = viewerDataFromExisting || chatOnlyPath || status.viewerStatus === 'skipped'
  const viewerTracker = viewerTrackerStepProgress(status, nowMs)
  const viewerStepState: 'done' | 'active' | 'pending' | 'failed' | 'partial' = viewersChartReady && !viewerBackfillPending
    ? 'done'
    : status.viewerStatus === 'backfilling'
      ? 'active'
      : viewerBackfillPending
        ? 'partial'
        : viewerFailed
          ? 'failed'
          : (['scraping_tracker', 'parsing_tracker'].includes(status.phase) ? 'active' : 'pending')
  const viewerPct = viewersChartReady && !viewerBackfillPending
    ? 100
    : viewerBackfillPending
      ? (viewerDataFromExisting ? 100 : 40)
      : viewerFailed
        ? 0
        : viewerStepState === 'active'
          ? viewerTracker.pct
          : 0
  const viewerDetailLabel = viewersChartReady && !viewerBackfillPending
    ? (viewerStepUsesExisting ? 'Using existing viewer rollups' : 'Viewer chart ready')
    : status.viewerStatus === 'backfilling' || status.viewerStatus === 'pending_backfill'
      ? (status.tracker?.message || viewerTracker.detail)
      : viewerStepState === 'pending'
        ? 'Waiting for TwitchTracker viewer minutes'
        : viewerFailed
          ? (status.tracker?.message || 'Viewer chart unavailable — chat and emote Pulse still sync')
          : viewerTracker.detail

  const chatStepState: 'done' | 'active' | 'pending' = chatFetchDone
    ? 'done'
    : chatFetchActive
      ? 'active'
      : 'pending'

  const rollupStepState: 'done' | 'active' | 'pending' = indexPhase === 'done' || status.phase === 'completed' || status.phase === 'exporting_archive' || status.phase === 'export_pending'
    ? 'done'
    : isIndexing || (status.rollupsWritten ?? 0) > 0
      ? 'active'
      : 'pending'
  const rollupPct = chatIndex?.pct ?? (indexPhase === 'tokenizing' ? 12 : 0)
  const overallEta = syncOverallEta(status, chatTimeline)
  const overallProgress = syncOverallProgress(
    status,
    viewerStepState,
    chatStepState,
    rollupStepState,
    viewerPct,
    chatFetchPct,
    rollupPct,
    chatOnlyPath,
  )
  const overallStageLabel = chatTailWriting
    ? 'Writing rollups & emotes'
    : chatTailFinalizing
      ? 'Finalizing chat index'
      : overallProgress.stageLabel

  const segmentCells = segmentTotal > 1
    ? segmentGridBuckets(segmentTotal, segmentDone, Boolean(status.chat?.active), Boolean(status.chat?.throttled))
    : []
  const showIndexingBanner = viewersChartReady && (chatFetchActive || isIndexing)

  return (
    <div className="w-full rounded-xl border border-white/10 bg-[#111118]/90 px-4 py-4 text-left shadow-lg shadow-black/20">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="inline-flex h-7 w-7 items-center justify-center rounded-lg border border-white/10 bg-white/[0.04] text-zinc-400">
            <svg className="h-3.5 w-3.5 animate-spin" style={{ animationDuration: '2.5s' }} viewBox="0 0 16 16" fill="none" aria-hidden>
              <path d="M8 1.5v3M8 11.5v3M1.5 8h3M11.5 8h3" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
              <path d="M3.05 3.05l2.12 2.12M10.83 10.83l2.12 2.12M3.05 12.95l2.12-2.12M10.83 5.17l2.12-2.12" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
            </svg>
          </span>
          <div>
            <div className="text-[11px] font-black uppercase tracking-[0.14em] text-zinc-300">Sync progress</div>
            <div className="text-[10px] font-semibold text-zinc-500">
              {overallStageLabel}
              {chatOnlyPath ? ' · VOD chat indexing (viewer chart unchanged)' : ''}
            </div>
          </div>
        </div>
        <div className="text-right text-[10px] font-semibold text-zinc-500">
          <div className="font-black tabular-nums text-violet-200">{Math.round(overallProgress.pct)}%</div>
          <div>Elapsed {formatElapsed(status.startedAt)}</div>
          {overallEta ? <div className="text-violet-300/90">{overallEta}</div> : null}
        </div>
      </div>

      <div className="mt-3">
        <SyncProgressBar pct={overallProgress.pct} tone="violet" />
      </div>

      {showIndexingBanner ? (
        <div className="mt-3 rounded-lg border border-cyan-500/20 bg-cyan-500/10 px-3 py-2 text-[11px] font-semibold leading-snug text-cyan-100">
          {viewerStepUsesExisting
            ? 'Viewer chart ready. VOD chat and emotes are still being indexed.'
            : 'Viewer minutes are on the chart. VOD chat and emotes are still being indexed.'}
        </div>
      ) : null}

      {status.chat?.throttled ? (
        <div className="mt-2 rounded-lg border border-red-500/25 bg-red-500/10 px-3 py-2 text-[11px] font-semibold text-red-200">
          Twitch rate limit — GQL fetch is backing off before retrying segments.
        </div>
      ) : null}

      {viewerFailed ? (
        <div className="mt-2 rounded-lg border border-amber-500/25 bg-amber-500/10 px-3 py-2 text-[11px] font-semibold leading-snug text-amber-100">
          {viewerDetailLabel}
          <div className="mt-1 text-[10px] font-medium text-amber-200/80">
            Re-sync viewers after connectivity recovers, or use StreamPulse for hosted minute charts.
          </div>
        </div>
      ) : null}

      <div className="mt-4 space-y-4">
        {/* Viewers */}
        <section>
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2.5">
              <SyncStepIcon state={viewerStepState} />
              <div>
                <div className="text-[11px] font-bold text-zinc-200">Viewers (TwitchTracker)</div>
                <div className="text-[10px] font-semibold text-zinc-500">
                  {viewerDetailLabel}
                </div>
              </div>
            </div>
            <span className={`text-[11px] font-black tabular-nums ${
              viewerStepState === 'done' ? 'text-emerald-400' : viewerStepState === 'failed' || viewerStepState === 'partial' ? 'text-amber-300' : 'text-zinc-400'
            }`}>
              {viewerStepBadge(viewerStepState, viewerPct, status.viewerStatus)}
            </span>
          </div>
          <SyncProgressBar pct={viewerPct} tone={viewerFailed ? 'amber' : 'green'} pending={viewerStepState === 'pending'} />
          {viewerStepState === 'active' || viewerStepState === 'partial' ? (
            <div className="mt-2.5 space-y-1 text-[10px] font-semibold text-zinc-500">
              {status.tracker?.phase ? (
                <div>
                  <span className="text-zinc-400">Phase:</span>{' '}
                  {viewerTracker.phaseLabel}
                </div>
              ) : null}
              {(viewerTracker.elapsedSec > 0 || trackerTickActive) ? (
                <div>
                  <span className="text-zinc-400">Elapsed:</span>{' '}
                  {viewerTracker.elapsedSec}s
                  {viewerStepState === 'active' && viewerTracker.expectedSec > 0
                    ? ` · budget ~${viewerTracker.expectedSec}s`
                    : ''}
                </div>
              ) : null}
              {status.tracker?.url ? (
                <div className="truncate">
                  <span className="text-zinc-400">Source:</span>{' '}
                  <span className="text-zinc-500">{status.tracker.url.replace(/^https?:\/\//, '')}</span>
                </div>
              ) : null}
            </div>
          ) : null}
        </section>

        {/* VOD Chat Fetch */}
        {!status.viewersOnly ? (
          <section>
            <div className="flex items-center justify-between gap-3">
              <div className="flex items-center gap-2.5">
                <SyncStepIcon state={chatStepState} />
                <div>
                  <div className="text-[11px] font-bold text-zinc-200">{chatSectionTitle}</div>
                  <div className="text-[10px] font-semibold text-zinc-500">
                    {chatSectionDetail}
                  </div>
                </div>
              </div>
              <span className={`text-[11px] font-black tabular-nums ${chatStepState === 'done' ? 'text-emerald-400' : 'text-violet-300'}`}>
                {chatStepState === 'pending' ? 'Pending' : `${chatFetchPct}%`}
              </span>
            </div>
            <SyncProgressBar pct={chatFetchPct} tone="violet" pending={chatStepState === 'pending'} />

            {chatStepState !== 'pending' && (chatTimeline || segmentsTrackable) ? (
              <div className="mt-2.5 space-y-1 text-[10px] font-semibold text-zinc-500">
                {chatTailSegmentPlan ? (
                  <div>
                    <span className="text-zinc-400">Segment plan:</span>{' '}
                    {chatTailSegmentPlan}
                  </div>
                ) : null}
                {segmentTotal > 1 ? (
                  <div>
                    <span className="text-zinc-400">Segments closed:</span>{' '}
                    {segmentDone.toLocaleString()} / {segmentTotal.toLocaleString()}
                    {segmentsIncomplete > 0 ? ` · ${segmentsIncomplete.toLocaleString()} remaining` : ''}
                  </div>
                ) : null}
                {(status.chat?.gqlPages ?? 0) > 0 ? (
                  <div>
                    <span className="text-zinc-400">GQL pages:</span>{' '}
                    {(status.chat?.gqlPages ?? 0).toLocaleString()}
                  </div>
                ) : null}
                {chatTimeline ? (
                  <div>
                    <span className="text-zinc-400">Timeline scanned:</span>{' '}
                    {chatTimeline.pct}% · {formatVodClock(chatTimeline.timeline)} / {formatVodClock(chatTimeline.total)}
                  </div>
                ) : null}
                {(status.chat?.commentsFetched ?? 0) > 0 ? (
                  <div>
                    <span className="text-zinc-400">Comments indexed:</span>{' '}
                    {(status.chat?.commentsFetched ?? 0).toLocaleString()}
                    {(status.chat?.commentsSaved ?? 0) > 0 ? ` · ${(status.chat?.commentsSaved ?? 0).toLocaleString()} saved` : ''}
                  </div>
                ) : null}
                {(status.chat?.hotSplits ?? 0) > 0 ? (
                  <div>
                    <span className="text-zinc-400">Hot splits:</span>{' '}
                    {(status.chat?.hotSplits ?? 0).toLocaleString()}
                    {status.chat?.hotSegmentSplitReason ? ` · last ${status.chat.hotSegmentSplitReason.replace(/_/g, ' ')}` : ''}
                  </div>
                ) : null}
                {(status.chat?.autoClosedSegments ?? 0) > 0 ? (
                  <div>
                    <span className="text-zinc-400">Auto-closed segments:</span>{' '}
                    {(status.chat?.autoClosedSegments ?? 0).toLocaleString()}
                  </div>
                ) : null}
                {status.chat?.summaryRefreshDeferred ? (
                  <div>
                    <span className="text-zinc-400">Summary refresh:</span> deferred until finalizing
                  </div>
                ) : null}
                {(status.chat?.concurrency ?? 0) > 1 ? (
                  <div>
                    <span className="text-zinc-400">Parallelism:</span> {status.chat?.concurrency} workers
                  </div>
                ) : null}
              </div>
            ) : null}

            {segmentCells.length > 1 ? (
              <div className="mt-3">
                <div className="flex items-center justify-between text-[9px] font-bold uppercase tracking-wide text-zinc-600">
                  <span>0h</span>
                  {chatTimeline ? <span>{formatVodClock(chatTimeline.total)}</span> : null}
                </div>
                <div className="mt-1 grid gap-0.5" style={{ gridTemplateColumns: `repeat(${segmentCells.length}, minmax(0, 1fr))` }}>
                  {segmentCells.map((cell, i) => (
                    <div
                      key={`seg-cell-${i}`}
                      className={`h-3 rounded-sm transition-colors duration-300 ${
                        cell === 'done'
                          ? 'bg-emerald-500/85'
                          : cell === 'active'
                            ? 'bg-violet-500/90 animate-pulse'
                            : cell === 'retry'
                              ? 'bg-red-500/80'
                              : 'bg-zinc-700/45'
                      }`}
                    />
                  ))}
                </div>
                <SegmentGridLegend />
              </div>
            ) : null}
          </section>
        ) : null}

        {/* Rollups & Emotes */}
        {!status.viewersOnly ? (
          <section>
            <div className="flex items-center justify-between gap-3">
              <div className="flex items-center gap-2.5">
                <SyncStepIcon state={rollupStepState} />
                <div>
                  <div className="text-[11px] font-bold text-zinc-200">Rollups &amp; Emotes</div>
                  <div className="text-[10px] font-semibold text-zinc-500">
                    {rollupStepState === 'done'
                      ? 'Minute rollups saved'
                      : rollupStepState === 'active'
                        ? syncIndexPhaseDetail(status.chat, status.rollupsWritten ?? 0) || 'Indexing chat and emotes'
                        : 'Minute rollups and emote tokenization'}
                  </div>
                </div>
              </div>
              <span className={`text-[11px] font-black tabular-nums ${
                rollupStepState === 'done' ? 'text-emerald-400' : rollupStepState === 'active' ? 'text-amber-300' : 'text-zinc-500'
              }`}>
                {rollupStepState === 'pending' ? 'Pending' : rollupStepState === 'done' ? '100%' : `${rollupPct}%`}
              </span>
            </div>
            <SyncProgressBar pct={rollupPct} tone="amber" pending={rollupStepState === 'pending'} />
            {rollupStepState === 'pending' ? (
              <div className="mt-2 flex items-center gap-1.5 text-[10px] font-semibold text-zinc-600">
                <span aria-hidden>⏱</span>
                {chatFetchActive ? 'Indexing starts as each VOD segment finishes' : 'Starts after chat fetch completes'}
              </div>
            ) : null}
            {rollupStepState === 'active' && chartChatMinutes > 0 ? (
              <div className="mt-2 text-[10px] font-semibold text-cyan-400/90">
                {chartChatMinutes.toLocaleString()} chat minutes visible on chart
              </div>
            ) : null}
          </section>
        ) : null}
      </div>

      {status.phase === 'failed' && status.error ? (
        <div className="mt-4 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs font-bold text-red-300">
          {status.error}
        </div>
      ) : null}
      {status.phase === 'export_pending' ? (
        <div className="mt-4 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs font-bold text-amber-200">
          {status.error || 'Sync data is written, but archive export still needs confirmation.'}
        </div>
      ) : null}
    </div>
  )
}

async function pollSyncUntilDone(
  streamId: string,
  onUpdate: (status: SyncStatus | null) => void,
  opts?: { onPhase?: (phase: SyncPhase) => void; onProgress?: (status: SyncStatus) => void },
) {
  const terminal: SyncPhase[] = ['completed', 'export_pending', 'failed']
  let lastPhase: SyncPhase | null = null
  let lastRollupsWritten = -1
  let lastGood: SyncStatus | null = null
  let consecutiveFailures = 0
  for (;;) {
    let status: SyncStatus | null = null
    try {
      status = await getSyncStatus(streamId)
      consecutiveFailures = 0
    } catch {
      consecutiveFailures++
      const message = consecutiveFailures <= 2
        ? 'Reconnecting to sync status…'
        : consecutiveFailures <= 5
          ? 'Sync status temporarily unavailable — retrying…'
          : 'Sync status unavailable — still retrying…'
      if (lastGood) {
        onUpdate({ ...lastGood, message })
      } else {
        onUpdate(null)
      }
      if (consecutiveFailures > 10) {
        return lastGood
      }
      await new Promise(resolve => setTimeout(resolve, 2000))
      continue
    }
    if (status) {
      lastGood = status
    }
    onUpdate(status)
    if (status && (status.rollupsWritten ?? 0) !== lastRollupsWritten) {
      lastRollupsWritten = status.rollupsWritten ?? 0
      opts?.onProgress?.(status)
    }
    if (status && status.phase !== lastPhase) {
      lastPhase = status.phase
      opts?.onPhase?.(status.phase)
    }
    if (!status || terminal.includes(status.phase) || status.stale) {
      return status
    }
    await new Promise(resolve => setTimeout(resolve, 2000))
  }
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
  syncing,
  syncedOnly,
  onSyncedOnlyChange,
  coreMinuteChartsBlocked = false,
  activeRollupStats,
}: {
  login: string
  streams: AnalyticsStream[]
  activeID?: string
  isLiveView: boolean
  liveState?: string
  onPrefetchStream?: (streamId: string) => void
  syncing?: boolean
  syncedOnly?: boolean
  onSyncedOnlyChange?: (value: boolean) => void
  coreMinuteChartsBlocked?: boolean
  activeRollupStats?: { avg: number; peak: number; current: number } | null
}) {
  const dateCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    streams.forEach(s => {
      const slug = getLocalDateString(s.startedAt)
      if (slug) counts[slug] = (counts[slug] || 0) + 1
    })
    return counts
  }, [streams])

  const visibleStreams = useMemo(() => {
    return streams.filter(s => streamIsSidebarVisible(s, Boolean(syncedOnly)))
  }, [streams, syncedOnly])

  // Below the lg breakpoint (1024px) the archive starts collapsed to at most
  // 2 stream rows with a toggle to reveal the full list (Requirements 5.2,
  // 5.3). At lg+ the full list is always shown (the `lg:block` resets the
  // mobile-only `hidden`), so this state only affects the mobile layout.
  const [archiveExpanded, setArchiveExpanded] = useState(false)
  const MOBILE_COLLAPSED_ROWS = 2
  const hasCollapsibleRows = visibleStreams.length > MOBILE_COLLAPSED_ROWS

  return (
    <div className="flex min-h-0 flex-col overflow-hidden rounded border border-white/10 bg-white/[0.035] xl:max-h-[calc(100vh-12rem)]">
      <div className="flex items-center justify-between border-b border-white/10 px-3 py-2.5">
        <span className="text-[11px] font-black uppercase tracking-wide text-zinc-500">Streams</span>
        <span className="rounded bg-white/10 px-1.5 py-0.5 text-[10px] font-black text-zinc-400">{visibleStreams.length}{syncedOnly ? `/${streams.length}` : ''}</span>
      </div>
      {onSyncedOnlyChange ? (
        <label className="flex cursor-pointer items-center gap-2 border-b border-white/5 px-3 py-2 text-[10px] font-semibold text-zinc-400">
          <input
            type="checkbox"
            checked={Boolean(syncedOnly)}
            onChange={e => onSyncedOnlyChange(e.target.checked)}
            className="accent-violet-500"
          />
          Synced only (hide stats-only rows)
        </label>
      ) : null}
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
            {liveState === 'live' ? 'Live tracking' : 'Most recent session'}
          </div>
        </Link>
        {streams.length === 0 ? (
          <div className="px-3 py-4 text-center text-[11px] font-semibold text-zinc-500">
            No past streams indexed yet.
          </div>
        ) : visibleStreams.length === 0 ? (
          <div className="px-3 py-4 text-center text-[11px] font-semibold text-zinc-500">
            No synced streams match this filter. Turn off &quot;Synced only&quot; to see stats-only sessions.
          </div>
        ) : (
          <div className="divide-y divide-white/5">
            {visibleStreams.map((stream, rowIndex) => {
              const dateSlug = getLocalDateString(stream.startedAt)
              const isUnique = dateSlug && dateCounts[dateSlug] === 1
              const targetSlug = isUnique ? dateSlug : stream.streamId
              const isActive = !isLiveView && (activeID === stream.streamId || activeID === dateSlug || activeID === targetSlug)
              const hasMinuteData = (stream.viewerSamples ?? 0) > 0 || (stream.chatMessages ?? 0) > 0
              const isSyncingActive = Boolean(syncing && isActive)
              const rollupStats = isSyncingActive ? activeRollupStats : null
              // Hide rows beyond the first 2 on mobile while collapsed; lg:block
              // always reveals them on desktop (Requirement 5.2).
              const mobileHiddenClass =
                !archiveExpanded && rowIndex >= MOBILE_COLLAPSED_ROWS ? 'hidden lg:block' : ''

              return (
                <Link
                  key={stream.streamId}
                  to={`/analytics/${encodeURIComponent(login)}/${encodeURIComponent(targetSlug)}`}
                  onMouseEnter={() => onPrefetchStream?.(stream.streamId)}
                  onFocus={() => onPrefetchStream?.(stream.streamId)}
                  className={`block border-l-2 px-3 py-2.5 transition hover:bg-white/[0.05] ${mobileHiddenClass} ${
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
                    <span
                      className={`rounded px-1.5 py-0.5 text-[9px] font-black uppercase ${
                        isSyncingActive
                          ? 'bg-violet-500/10 text-violet-300'
                          : hasMinuteData
                            ? 'bg-emerald-500/10 text-emerald-300'
                            : 'bg-amber-500/10 text-amber-300'
                      }`}
                      title={
                        isSyncingActive
                          ? 'Sync in progress — partial chart data may already be visible.'
                          : hasMinuteData
                            ? 'Minute-level viewer, chat, and emote rollups are synced for charts.'
                            : coreMinuteChartsBlocked
                              ? 'Session stats only. Minute charts live on StreamPulse.'
                              : 'Session stats only (duration, title). Open the stream detail page to sync minute charts.'
                      }
                    >
                      {isSyncingActive ? 'Syncing' : hasMinuteData ? 'Synced' : 'Stats only'}
                    </span>
                  </div>
                  {!hasMinuteData && isActive && coreMinuteChartsBlocked ? (
                    <div className="mt-1.5">
                      <CoreMinuteChartsNotice compact />
                    </div>
                  ) : null}
                  <div className="mt-1.5 grid grid-cols-3 gap-1 text-[10px] font-bold text-zinc-500">
                    <span>{duration(stream)}</span>
                    <span>avg {count(rollupStats?.avg ?? stream.avgViewers)}</span>
                    <span>peak {count(rollupStats?.peak ?? stream.peakViewers)}</span>
                  </div>
                </Link>
              )
            })}
          </div>
        )}
        {hasCollapsibleRows ? (
          <button
            type="button"
            onClick={() => setArchiveExpanded(prev => !prev)}
            aria-expanded={archiveExpanded}
            className="block w-full border-t border-white/10 px-3 py-2 text-center text-[10px] font-black uppercase tracking-wide text-zinc-400 transition hover:bg-white/[0.05] hover:text-white lg:hidden"
          >
            {archiveExpanded ? 'Show fewer streams' : `Show all ${visibleStreams.length} streams`}
          </button>
        ) : null}
      </div>
    </div>
  )
}

function TopEmoteTable({ emotes, selected, onSelect, embedded = false }: { emotes: AnalyticsTopEmote[]; selected: Set<string>; onSelect: (key: string) => void; embedded?: boolean }) {
  if (!emotes.length) {
    return (
      <div className={`grid min-h-44 place-items-center text-center ${embedded ? 'px-3 py-4' : 'rounded border border-white/10 bg-white/[0.035]'}`}>
        <div>
          <div className="text-sm font-black text-zinc-200">No emotes counted</div>
          <div className="mt-1 text-xs font-semibold text-zinc-500">Collected chat has not matched known emotes yet.</div>
        </div>
      </div>
    )
  }
  return (
    <div className={`overflow-hidden ${embedded ? 'max-h-[calc(100vh-14rem)] overflow-y-auto' : 'rounded border border-white/10 bg-white/[0.035]'}`}>
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
            <span className="flex items-center">
              {(() => {
                const provider = emote.provider || parseEmoteKey(emote.key).provider
                return provider && provider !== 'unknown'
                  ? <EmoteProviderBadge provider={provider} />
                  : <span className="text-zinc-500">-</span>
              })()}
            </span>
            <span>{count(emote.count)}</span>
          </button>
        )
      })}
    </div>
  )
}

function EmoteProviderBadge({ provider }: { provider?: string }) {
  if (!provider) return null
  return (
    <span className={`rounded border px-1.5 py-0.5 text-[9px] font-black uppercase ${emoteProviderTone(provider)}`}>
      {emoteProviderLabel(provider)}
    </span>
  )
}

function MomentReviewPanel({
  rollups,
  selectedRollup,
  onSelectRollup,
  topEmotesCatalog,
  heatmapPoints,
  streamStartedAt,
  embedded = false,
}: {
  rollups: AnalyticsMinuteRollup[]
  selectedRollup: AnalyticsMinuteRollup | null
  onSelectRollup: (rollup: AnalyticsMinuteRollup) => void
  topEmotesCatalog?: AnalyticsTopEmote[]
  heatmapPoints?: ReplayHeatmapPoint[]
  streamStartedAt?: string
  embedded?: boolean
}) {
  const baselines = useMemo(() => computeStreamBaselines(rollups), [rollups])
  const heatmapPointMap = useMemo(() => {
    if (!heatmapPoints || heatmapPoints.length === 0) return null
    const map = new Map<string, ReplayHeatmapPoint>()
    for (const p of heatmapPoints) {
      map.set(p.minuteTs, p)
    }
    return map
  }, [heatmapPoints])
  const candidates = useMemo(() => {
    const rows = rollups
      .filter(point => !point.missing && rollupHasMinuteData(point))
      .map(point => {
        const fallbackReason = detectPickReason(point, baselines, topEmotesCatalog)
        const scoreModel = buildMomentScoreModel({
          heatmapPoint: heatmapPointMap?.get(point.minuteTs),
          fallbackScore100: computeMomentScore100(point, baselines, rollups),
          fallbackReason,
          fallbackTopEmotes: heatmapEmotesFromRollup(point, 5, topEmotesCatalog),
        })
        return {
          rollup: point,
          score: scoreModel.score,
          scoreLabel: scoreModel.label,
          reasonLabel: scoreModel.reasonLabel,
          topEmote: topEmotesFromRollup(point, 1, topEmotesCatalog)[0],
          estimated: scoreModel.estimated,
        }
      })
    const sorted = [...rows].sort((a, b) => b.score - a.score)
    return sorted.slice(0, 10)
  }, [rollups, baselines, topEmotesCatalog, heatmapPointMap])

  if (candidates.length < 2) {
    return (
      <div className={`${embedded ? 'px-3 py-4' : 'rounded border border-white/10 bg-[#0d0d12] p-3'} text-center text-[11px] font-semibold text-zinc-500`}>
        {rollups.some(rollupHasMinuteData) ? 'Not enough peaks yet — sync chat or wait for more minutes.' : 'Sync chat/emotes to surface ranked moments.'}
      </div>
    )
  }

  return (
    <div className={embedded ? 'p-3' : 'rounded border border-white/10 bg-[#0d0d12] p-3'}>
      <div className="mb-2 flex flex-col gap-0.5">
        <div className="text-[11px] font-black uppercase text-zinc-500">Top Moments</div>
        <p className="text-[10px] font-semibold text-zinc-600">{LIVE_HEAT_RANKED_SUBTITLE}</p>
      </div>
      <div className="flex max-h-56 flex-col gap-1.5 overflow-y-auto">
        {candidates.map(({ rollup, scoreLabel, reasonLabel, topEmote, estimated }) => {
          const offsetLabel = streamStartedAt
            ? formatHeatOffset(rollupOffsetSeconds(rollup, streamStartedAt))
            : new Date(rollup.minuteTs).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
          const active = selectedRollup?.minuteTs === rollup.minuteTs
          return (
            <button
              key={rollup.minuteTs}
              type="button"
              onClick={() => onSelectRollup(rollup)}
              className={`grid grid-cols-[5.25rem_minmax(0,1fr)_auto] items-center gap-2 rounded px-2.5 py-2 text-left text-xs transition ${
                active ? 'border border-amber-500/20 bg-amber-500/10' : 'border border-transparent bg-white/[0.03] hover:bg-white/[0.05]'
              }`}
            >
              <span className="font-mono text-[11px] font-bold tabular-nums text-zinc-400">{offsetLabel}</span>
              <span className="min-w-0">
                <span className="block truncate font-semibold text-zinc-400">{reasonLabel}</span>
                {topEmote ? (
                  <span className="mt-1 flex min-w-0 items-center gap-1.5">
                    {topEmote.image_url ? (
                      <img src={topEmote.image_url} alt="" className="h-4 w-4 shrink-0 object-contain" loading="lazy" />
                    ) : null}
                    <span className="truncate font-bold text-zinc-300">{topEmote.name}</span>
                    <EmoteProviderBadge provider={topEmote.provider} />
                    <span className="shrink-0 text-zinc-500">{count(topEmote.count)}</span>
                  </span>
                ) : null}
              </span>
              <span
                className="font-black text-amber-300/80"
                title={estimated ? 'Estimated from local rollups until heatmap scoring is available.' : 'Backend replay heatmap score.'}
              >
                {scoreLabel}
              </span>
            </button>
          )
        })}
      </div>
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
  rollups,
  startedAt,
  vodId,
  channel,
  streamId,
  topEmotesCatalog,
  heatmapPoint,
  heatmapDetail,
  isLiveView,
  channelLive,
}: {
  rollup: AnalyticsMinuteRollup | null
  rollups: AnalyticsMinuteRollup[]
  startedAt?: string
  vodId?: string
  channel: string
  streamId?: string
  topEmotesCatalog?: AnalyticsTopEmote[]
  heatmapPoint?: ReplayHeatmapPoint | null
  heatmapDetail?: ReplayHeatmapDetailPoint | null
  isLiveView?: boolean
  channelLive?: boolean
}) {
  const queryClient = useQueryClient()
  const [clipStatus, setClipStatus] = useState<'idle' | 'loading' | 'success' | 'error'>('idle')
  const [clipError, setClipError] = useState('')
  const [createdJobId, setCreatedJobId] = useState<string | null>(null)
  const [bookmarkStatus, setBookmarkStatus] = useState<'idle' | 'loading' | 'success' | 'error'>('idle')
  const [bookmarkError, setBookmarkError] = useState('')
  const baselines = useMemo(() => computeStreamBaselines(rollups), [rollups])
  const bookmarkQueryKey = ['pulse-bookmarks', channel, streamId ?? '', vodId ?? '']
  const bookmarksQuery = useQuery({
    queryKey: bookmarkQueryKey,
    queryFn: () => getPulseBookmarks({ login: channel, streamId, vodId, limit: 20 }),
    enabled: Boolean(channel),
  })

  if (!rollup) {
    return (
      <div className="rounded border border-white/10 bg-[#0d0d12] p-4 text-center text-xs text-zinc-500 italic">
        Click the graph or a ranked row to select a moment.
      </div>
    )
  }

  const timeStr = new Date(rollup.minuteTs).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
  const dateStr = new Date(rollup.minuteTs).toLocaleDateString([], { month: 'short', day: 'numeric', year: 'numeric' })

  let offsetSeconds = 0
  let offsetStr = ''
  if (startedAt) {
    offsetSeconds = rollupOffsetSeconds(rollup, startedAt)
    offsetStr = formatVodOffset(offsetSeconds)
  }

  const fallbackReason = detectPickReason(rollup, baselines, topEmotesCatalog)
  const scoreModel = buildMomentScoreModel({
    heatmapPoint,
    heatmapDetail,
    fallbackScore100: computeMomentScore100(rollup, baselines, rollups),
    fallbackReason,
    fallbackTopEmotes: heatmapEmotesFromRollup(rollup, 5, topEmotesCatalog),
  })
  const vodUrl = vodId ? buildTwitchVodUrl(vodId, offsetSeconds) : undefined
  const streamcloneVodUrl = vodId ? buildVodDeepLink(channel, vodId, offsetSeconds, streamId) : undefined
  const canClipLive = isLiveView && channelLive !== false
  const canExportVod = !isLiveView && Boolean(vodId)

  const refreshBookmarks = () => {
    void queryClient.invalidateQueries({ queryKey: bookmarkQueryKey })
  }

  const handleSaveMoment = async () => {
    setBookmarkStatus('loading')
    setBookmarkError('')
    try {
      await createPulseBookmark({
        login: channel,
        streamId,
        vodId,
        offsetSeconds,
        label: `${scoreModel.reasonLabel} at ${formatHeatOffset(offsetSeconds)}`,
        notes: '',
        score: Math.round(scoreModel.score),
        source: 'web',
      })
      setBookmarkStatus('success')
      refreshBookmarks()
      setTimeout(() => setBookmarkStatus('idle'), 1800)
    } catch (err: any) {
      setBookmarkStatus('error')
      setBookmarkError(err.message || 'Could not save this moment.')
    }
  }

  const handleDeleteBookmark = async (id: string) => {
    await deletePulseBookmark(id)
    refreshBookmarks()
  }

  const handleCreateClip = async () => {
    if (!rollup) return
    setClipStatus('loading')
    setClipError('')
    try {
      if (!canClipLive && !canExportVod) {
        setClipStatus('error')
        setClipError(
          isLiveView
            ? 'Channel is not live right now. Clip moments from the live view require an active broadcast.'
            : 'VOD ID is not resolved for this session yet. Wait for VOD metadata or open the stream on Twitch before exporting a past moment.',
        )
        return
      }
      const authStatus = await getClipperTwitchStatus().catch(() => null)
      if (authStatus && !authStatus.ok) {
        const blockingCodes = canExportVod
          ? ['twitch_not_configured', 'invalid_token']
          : ['twitch_not_configured', 'invalid_token', 'missing_scope', 'client_id_mismatch']
        if (blockingCodes.includes(authStatus.failure_code || '')) {
          setClipStatus('error')
          setClipError(
            authStatus.remediation
              || describeClipperFailure({ failure_code: authStatus.failure_code }),
          )
          return
        }
      }
      const pickReason = scoreModel.reason
      const chatMultiplier = (rollup.chatCount ?? 0) / baselines.chat
      const data = await triggerClipperManual(channel, {
        title: canExportVod ? `Analytics Export (${timeStr})` : `Analytics Spike (${timeStr})`,
        duration: 60.0,
        final_duration: 30.0,
        reason: `${pickReason} at ${timeStr}`,
        moment_context: {
          stream_id: streamId,
          minute_ts: rollup.minuteTs,
          vod_id: vodId,
          vod_offset_seconds: offsetSeconds,
          source_kind: canExportVod ? 'vod' : 'live',
          viewer_count: viewerValue(rollup),
          chat_per_min: rollup.chatCount ?? 0,
          emote_per_min: minuteEmoteTotal(rollup),
          top_emotes: topEmotesFromRollup(rollup, 5, topEmotesCatalog),
          chat_multiplier: Math.round(chatMultiplier * 10) / 10,
          pick_reason: pickReason,
          moment_score: Math.round(scoreModel.score),
        },
      })
      setCreatedJobId(data.job_id)
      setClipStatus('success')
      window.dispatchEvent(new CustomEvent('streamclone:clip-created'))
    } catch (err: any) {
      setClipStatus('error')
      setClipError(err.message || 'Clipper service is unreachable.')
    }
  }

  return (
    <div className="rounded border border-amber-500/10 bg-[#0d0d12] p-4 relative overflow-hidden transition-all duration-300">
      <div className="absolute left-0 right-0 top-0 h-1 bg-gradient-to-r from-amber-500/25 via-amber-400/60 to-amber-500/25" />

      <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <span className="text-xs font-black uppercase text-amber-300/80 bg-amber-500/10 px-2 py-0.5 rounded">Selected Moment</span>
            <span className="text-sm font-black text-white">{timeStr} · {dateStr}</span>
          </div>
          <div className="mt-2 flex flex-wrap gap-4 text-xs font-bold text-zinc-400">
            {offsetStr ? (
              <span>Stream offset: <strong className="text-zinc-200">{offsetStr}</strong></span>
            ) : null}
            <span>
              Score:{' '}
              <strong className={scoreModel.estimated ? 'text-amber-200' : 'text-emerald-300'}>
                {scoreModel.label}
              </strong>
            </span>
            <span>Reason: <strong className="text-zinc-200">{scoreModel.reasonLabel}</strong></span>
            {scoreModel.confidence !== null ? (
              <span>Confidence: <strong className="text-zinc-200">{Math.round(scoreModel.confidence * 100)}%</strong></span>
            ) : null}
            <span>Viewers: <strong className="text-zinc-200">{count(viewerValue(rollup))}</strong></span>
            <span>Chat activity: <strong className="text-zinc-200">{rollup.chatCount}/min</strong></span>
            <span>Emotes: <strong className="text-zinc-200">{minuteEmoteTotal(rollup)}/min</strong></span>
            {(rollup.seventvEmoteCount ?? 0) > 0 ? (
              <span>7TV: <strong className="text-emerald-300">{rollup.seventvEmoteCount}/min</strong></span>
            ) : null}
          </div>
          {topEmotesFromRollup(rollup, 4, topEmotesCatalog).length > 0 ? (
            <div className="mt-3 flex flex-wrap gap-2">
              {topEmotesFromRollup(rollup, 4, topEmotesCatalog).map(emote => (
                <span
                  key={emote.key}
                  className="inline-flex items-center gap-1.5 rounded border border-white/10 bg-white/[0.04] px-2 py-1 text-[11px] font-bold text-zinc-300"
                >
                  {emote.image_url ? (
                    <img src={emote.image_url} alt="" className="h-4 w-4 object-contain" loading="lazy" />
                  ) : null}
                  <span>{emote.name}</span>
                  <EmoteProviderBadge provider={emote.provider} />
                  <span className="text-zinc-500">{count(emote.count)}</span>
                </span>
              ))}
            </div>
          ) : null}
          {scoreModel.detailComponents.length > 0 ? (
            <div className="mt-3 flex flex-wrap gap-2">
              {scoreModel.detailComponents.slice(0, 4).map(component => (
                <span
                  key={component.key}
                  className="rounded border border-white/10 bg-white/[0.035] px-2 py-1 text-[10px] font-bold text-zinc-400"
                >
                  {component.key.replace(/_/g, ' ')}{' '}
                  <strong className="text-zinc-200">{Math.round(component.weightedScore)}</strong>
                </span>
              ))}
            </div>
          ) : null}
        </div>

        <div className="flex flex-wrap gap-3 items-center">
          {streamcloneVodUrl ? (
            <Link
              to={streamcloneVodUrl}
              className="flex items-center gap-2 rounded bg-violet-600 px-4 py-2 text-xs font-black text-white transition hover:bg-violet-700 shadow-lg shadow-violet-600/20"
            >
              <span>Play in Streamclone</span>
              <svg className="w-3.5 h-3.5 fill-current" viewBox="0 0 24 24">
                <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 14.5v-9l6 4.5-6 4.5z"/>
              </svg>
            </Link>
          ) : (
            <button
              disabled
              title="VOD ID not resolved yet. Ensure Twitch Developer OAuth settings are fully configured."
              className="flex items-center gap-2 rounded bg-zinc-800 px-4 py-2 text-xs font-black text-zinc-500 cursor-not-allowed border border-white/5"
            >
              VOD not linked yet
            </button>
          )}
          {vodUrl ? (
            <a
              href={vodUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2 rounded border border-violet-400/20 bg-violet-500/10 px-4 py-2 text-xs font-black text-violet-100 transition hover:border-violet-300/40 hover:bg-violet-500/20"
            >
              Open on Twitch
            </a>
          ) : null}

          <div>
            <button
              type="button"
              onClick={() => void handleSaveMoment()}
              disabled={bookmarkStatus === 'loading'}
              className={`mr-3 rounded border px-4 py-2 text-xs font-black transition ${
                bookmarkStatus === 'success'
                  ? 'border-emerald-400/30 bg-emerald-500/15 text-emerald-100'
                  : 'border-violet-400/30 bg-violet-500/10 text-violet-100 hover:bg-violet-500/20 disabled:cursor-wait disabled:opacity-60'
              }`}
              title="Save a private Pulse bookmark. This never creates a public clip."
            >
              {bookmarkStatus === 'loading' ? 'Saving...' : bookmarkStatus === 'success' ? 'Saved' : 'Save Moment'}
            </button>
            {canClipLive ? (
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
                {clipStatus === 'idle' && <span>Clip Live Moment</span>}
              </button>
            ) : canExportVod ? (
              <button
                type="button"
                onClick={handleCreateClip}
                disabled={clipStatus === 'loading'}
                className={`flex items-center gap-2 rounded px-4 py-2 text-xs font-black transition ${
                  clipStatus === 'loading'
                    ? 'bg-zinc-800 text-zinc-400 border border-white/5 cursor-wait'
                    : clipStatus === 'success'
                    ? 'bg-emerald-600 text-white'
                    : 'bg-violet-600 text-white hover:bg-violet-700 shadow-lg shadow-violet-600/20'
                }`}
              >
                {clipStatus === 'loading' && <span>Exporting...</span>}
                {clipStatus === 'success' && <span>✓ Export Queued!</span>}
                {clipStatus === 'error' && <span>Retry Export</span>}
                {clipStatus === 'idle' && <span>Export Moment</span>}
              </button>
            ) : (
              <button
                type="button"
                disabled
                title="Twitch clip creation only works while the channel is live. Export moment requires a synced VOD ID for past streams."
                className="flex items-center gap-2 rounded border border-white/10 bg-zinc-900 px-4 py-2 text-xs font-black text-zinc-500 cursor-not-allowed"
              >
                Clip requires live channel
              </button>
            )}
          </div>
        </div>
      </div>

      {clipStatus === 'error' && (
        <div className="mt-3 text-xs font-semibold text-red-400 rounded border border-red-500/10 bg-red-500/5 p-2.5 space-y-2">
          <div>Error: {clipError}</div>
          {isClipperAuthFailureMessage(clipError) ? (
            <ClipperAuthHelp compact onSynced={() => { setClipError(''); setClipStatus('idle') }} />
          ) : null}
        </div>
      )}
      {bookmarkStatus === 'error' && (
        <div className="mt-3 rounded border border-red-500/10 bg-red-500/5 p-2.5 text-xs font-semibold text-red-400">
          Bookmark error: {bookmarkError}
        </div>
      )}
      <SavedMomentsPanel
        bookmarks={bookmarksQuery.data?.items ?? []}
        loading={bookmarksQuery.isLoading}
        onDelete={id => void handleDeleteBookmark(id)}
      />
      {clipStatus === 'success' && (
        <div className="mt-3 text-xs font-semibold text-emerald-400 rounded border border-emerald-500/10 bg-emerald-500/5 p-2.5 flex justify-between items-center">
          <span>
            {canExportVod
              ? 'VOD export queued — open Clip Studio to edit while the segment downloads (may take 1–3 min for long VODs).'
              : 'Clip queued — open Clip Studio to edit while the source downloads (~30–90s).'}
          </span>
          {createdJobId ? (
            <Link to={studioPath(createdJobId)} className="ml-2 underline text-emerald-300 font-bold hover:text-emerald-200">
              Open in Clip Studio →
            </Link>
          ) : null}
        </div>
      )}
    </div>
  )
}

function SavedMomentsPanel({
  bookmarks,
  loading,
  onDelete,
}: {
  bookmarks: PulseBookmark[]
  loading: boolean
  onDelete: (id: string) => void
}) {
  return (
    <section className="mt-4 rounded border border-white/10 bg-white/[0.025] p-3">
      <div className="mb-2 flex items-center justify-between gap-2">
        <div>
          <h3 className="text-[11px] font-black uppercase text-zinc-400">Saved Moments</h3>
          <p className="text-[10px] font-semibold text-zinc-600">Private markers · never auto-create public clips</p>
        </div>
        <span className="rounded-full bg-violet-500/15 px-2 py-1 text-[10px] font-black text-violet-200">
          {bookmarks.length} saved
        </span>
      </div>
      {loading ? (
        <p className="text-xs font-semibold text-zinc-500">Loading saved moments...</p>
      ) : bookmarks.length === 0 ? (
        <p className="text-xs font-semibold text-zinc-500">No saved moments for this stream yet.</p>
      ) : (
        <div className="flex max-h-48 flex-col gap-2 overflow-y-auto">
          {bookmarks.map(bookmark => (
            <div key={bookmark.id} className="grid grid-cols-[4.75rem_minmax(0,1fr)_auto] items-center gap-2 rounded bg-black/20 px-2.5 py-2 text-xs">
              <span className="font-mono font-bold text-violet-200">{formatHeatOffset(bookmark.offsetSeconds)}</span>
              <span className="min-w-0">
                <span className="block truncate font-bold text-zinc-200">{bookmark.label}</span>
                <span className="text-[10px] font-semibold uppercase text-zinc-600">{bookmark.source}</span>
              </span>
              <button
                type="button"
                onClick={() => onDelete(bookmark.id)}
                className="rounded border border-white/10 px-2 py-1 text-[10px] font-black uppercase text-zinc-500 transition hover:border-red-400/30 hover:text-red-200"
              >
                Remove
              </button>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}

function StreamRecapPanel({
  recap,
}: {
  recap: PulseStreamRecap
}) {
  const topMoment = recap.topMoments[0]
  return (
    <section className="rounded border border-emerald-500/15 bg-emerald-500/[0.04] p-3">
      <div className="mb-3 flex items-start justify-between gap-3">
        <div>
          <h3 className="text-xs font-black uppercase text-emerald-100">Stream Recap</h3>
          <p className="text-[10px] font-semibold text-emerald-200/60">Derived from Pulse rollups and heatmap scoring</p>
        </div>
        {topMoment ? (
          <span className="rounded bg-emerald-400/15 px-2 py-1 text-[10px] font-black text-emerald-100">
            Top {topMoment.score}
          </span>
        ) : null}
      </div>
      <div className="grid grid-cols-2 gap-2 text-xs">
        <div className="rounded bg-black/20 p-2">
          <div className="text-[10px] font-black uppercase text-zinc-500">Messages</div>
          <div className="mt-1 font-black text-zinc-100">{count(recap.totalMessages)}</div>
        </div>
        <div className="rounded bg-black/20 p-2">
          <div className="text-[10px] font-black uppercase text-zinc-500">Peak Chat</div>
          <div className="mt-1 font-black text-zinc-100">{count(recap.peakChatPerMin)}/min</div>
        </div>
      </div>
      {recap.biggestChatSpike || recap.funniestEmoteBurst ? (
        <div className="mt-2 grid gap-2 text-xs">
          {recap.biggestChatSpike ? (
            <div className="rounded bg-black/20 px-2 py-1.5 font-semibold text-zinc-300">
              Biggest spike at <strong className="text-emerald-100">{formatHeatOffset(recap.biggestChatSpike.offsetSeconds)}</strong>
              {' '}({count(recap.biggestChatSpike.chatPerMin)}/min)
            </div>
          ) : null}
          {recap.funniestEmoteBurst ? (
            <div className="rounded bg-black/20 px-2 py-1.5 font-semibold text-zinc-300">
              Emote burst at <strong className="text-emerald-100">{formatHeatOffset(recap.funniestEmoteBurst.offsetSeconds)}</strong>
              {recap.funniestEmoteBurst.code ? ` · ${recap.funniestEmoteBurst.code}` : ''} ({count(recap.funniestEmoteBurst.count)})
            </div>
          ) : null}
        </div>
      ) : null}
      {recap.topEmotes.length > 0 ? (
        <div className="mt-3">
          <div className="mb-1 text-[10px] font-black uppercase text-zinc-500">Top 7TV</div>
          <div className="flex flex-wrap gap-1.5">
            {recap.topEmotes.slice(0, 5).map(emote => (
              <span key={emote.code} className="rounded border border-white/10 bg-white/[0.04] px-2 py-1 text-[10px] font-bold text-zinc-300">
                {emote.code} <span className="text-zinc-500">{count(emote.count)}</span>
              </span>
            ))}
          </div>
        </div>
      ) : null}
      {recap.topMoments.length > 0 ? (
        <div className="mt-3">
          <div className="mb-1 text-[10px] font-black uppercase text-zinc-500">Top Moments</div>
          <div className="flex max-h-40 flex-col gap-1.5 overflow-y-auto">
            {recap.topMoments.slice(0, 5).map(moment => (
              <div key={`${moment.offsetSeconds}-${moment.score}`} className="grid grid-cols-[4.25rem_1fr_auto] items-center gap-2 rounded bg-black/20 px-2 py-1.5 text-xs">
                <span className="font-mono font-bold text-emerald-100">{formatHeatOffset(moment.offsetSeconds)}</span>
                <span className="truncate text-zinc-400">{moment.reasons[0]?.replace(/_/g, ' ') || 'moment'}</span>
                <span className="font-black text-emerald-200">{moment.score}</span>
              </div>
            ))}
          </div>
        </div>
      ) : null}
      {recap.clipCandidates.length > 0 ? (
        <p className="mt-3 text-[10px] font-semibold text-zinc-500">
          {recap.clipCandidates.length} clip candidates ranked from the same Pulse scores.
        </p>
      ) : null}
    </section>
  )
}

export default function Analytics() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
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
  const [rightPanelTab, setRightPanelTab] = useState<RightPanelTab>('moments')
  const [analyticsViewMode, setAnalyticsViewMode] = useState<AnalyticsViewMode>('overview')
  const [syncedOnlyFilter, setSyncedOnlyFilter] = useState(false)
  const [autoPrefetchedStreamId, setAutoPrefetchedStreamId] = useState<string | null>(null)

  const isLiveRoute = !streamId
  const isHistoricalRoute = Boolean(streamId)

  useEffect(() => {
    setSelectedRollup(null)
    setLastRefreshedAt(null)
  }, [login, streamId])

  useEffect(() => {
    if (!login) return
    watchAnalyticsChannel(login).catch(() => undefined)
    const warmEmotes = () => {
      getChannel(login)
        .then(channel => {
          if (!channel?.id) return
          return ensureChannelEmotes(login, channel.id, ['seventv', 'twitch'])
        })
        .catch(() => undefined)
    }
    const requestIdle = typeof window.requestIdleCallback === 'function'
      ? window.requestIdleCallback.bind(window)
      : null
    const cancelIdle = typeof window.cancelIdleCallback === 'function'
      ? window.cancelIdleCallback.bind(window)
      : null
    if (requestIdle && cancelIdle) {
      const idleId = requestIdle(warmEmotes, { timeout: 3000 })
      return () => cancelIdle(idleId)
    }
    const timer = setTimeout(warmEmotes, 1000)
    return () => clearTimeout(timer)
  }, [login])

  const streamsQuery = useQuery({
    queryKey: ['analytics-streams', login],
    queryFn: () => getAnalyticsStreams(login, 20),
    enabled: Boolean(login),
    refetchInterval: 30000,
  })

  const setupQuery = useQuery({
    queryKey: ['setup-welcome'],
    queryFn: getSetupWelcome,
    staleTime: 60_000,
  })
  const timeseriesStatusQuery = useQuery({
    queryKey: ['analytics-timeseries-status'],
    queryFn: getTimeseriesStatus,
    staleTime: 30_000,
    refetchInterval: 60_000,
    retry: false,
  })

  const coreMinuteChartsBlocked = useMemo(
    () => coreMinuteChartsNeedScraper(
      setupQuery.data?.profile ?? 'core',
      setupQuery.data?.services.scraper ?? 'offline',
    ),
    [setupQuery.data],
  )

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
      // Helix history is authoritative for schedule metadata; local DB may have
      // placeholder startedAt from UpsertStreamPlaceholder (time.Now()).
      return {
        ...item,
        startedAt: tracker.startedAt || item.startedAt,
        endedAt: tracker.endedAt || item.endedAt,
        title: tracker.title || item.title,
        category: tracker.category || item.category,
        avgViewers: item.avgViewers > 0 ? item.avgViewers : tracker.avgViewers,
        peakViewers: item.peakViewers > 0 ? item.peakViewers : tracker.peakViewers,
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

  const routableStreams = useMemo(
    () => combinedStreams.filter(stream => !isSyncPrefetchPlaceholder(stream)),
    [combinedStreams],
  )

  const matchedStream = useMemo(() => {
    if (!streamId) return undefined

    const exactMatch = combinedStreams.find(s => s.streamId === streamId)
    if (exactMatch) return exactMatch

    if (/^\d{4}-\d{2}-\d{2}$/.test(streamId)) {
      return routableStreams.find(s => {
        if (!s.startedAt) return false
        const date = new Date(s.startedAt)
        if (isNaN(date.getTime())) return false

        const utcDateStr = date.toISOString().slice(0, 10)
        if (utcDateStr === streamId) return true

        const year = date.getFullYear()
        const month = String(date.getMonth() + 1).padStart(2, '0')
        const day = String(date.getDate()).padStart(2, '0')
        const localDateStr = `${year}-${month}-${day}`
        return localDateStr === streamId
      })
    }

    return undefined
  }, [streamId, combinedStreams, routableStreams])

  const dateSlugUnresolved = useMemo(() => {
    if (!streamId || !/^\d{4}-\d{2}-\d{2}$/.test(streamId)) return false
    if (streamsQuery.isLoading || historyQuery.isLoading) return false
    const unresolved = !matchedStream
    return unresolved
  }, [streamId, matchedStream, streamsQuery.isLoading, historyQuery.isLoading, login, combinedStreams.length, historyQuery.data?.items?.length])

  const targetQueryStreamId = useMemo(() => {
    if (!streamId) return ''
    if (/^\d+$/.test(streamId)) {
      return streamId
    }
    if (matchedStream) {
      return matchedStream.streamId
    }
    // Date slug: resolve from tracker history first so detail fetch does not wait on local merge.
    if (/^\d{4}-\d{2}-\d{2}$/.test(streamId)) {
      if (historyQuery.data?.items?.length) {
        const fromHistory = historyQuery.data.items.find(s => {
          if (!s.startedAt) return false
          if (s.startedAt.slice(0, 10) === streamId) return true
          return getLocalDateString(s.startedAt) === streamId
        })
        if (fromHistory) return fromHistory.id
      }
      if (streamsQuery.isLoading || historyQuery.isLoading) return undefined
      return undefined
    }
    return undefined
  }, [streamId, matchedStream, streamsQuery.isLoading, historyQuery.isLoading, historyQuery.data?.items])

  useEffect(() => {
    setSyncing(false)
    setSyncError(null)
    setSyncNotice(null)
    setSyncStatus(null)
    setAutoPrefetchedStreamId(null)
  }, [targetQueryStreamId])

  const prefetchStreamDetail = useCallback((id: string) => {
    if (!login || !id) return
    prefetchAnalyticsTracker(id, login).catch(() => undefined)
    queryClient.prefetchQuery({
      queryKey: ['analytics-detail', login, id],
      queryFn: () => getAnalyticsStream(id, { channel: login }),
      staleTime: 120_000,
    })
  }, [login, queryClient])

  const liveDetailQuery = useAnalyticsLive(login ?? '', {
    enabled: Boolean(login && isLiveRoute),
  })

  const historicalDetailQuery = useQuery({
    queryKey: ['analytics-detail', login, targetQueryStreamId],
    queryFn: () => getAnalyticsStream(targetQueryStreamId!, { channel: login }),
    enabled: Boolean(login && targetQueryStreamId && !isLiveRoute),
    refetchInterval: query => {
      const data = query.state.data as AnalyticsStreamDetail | undefined
      if (data?.state === 'live' || data?.state === 'syncing') return 15_000
      return false
    },
    retry: false,
    placeholderData: (previousData, previousQuery) => {
      const prevKey = previousQuery?.queryKey?.[2]
      if (targetQueryStreamId && prevKey === targetQueryStreamId) return previousData
      return undefined
    },
    staleTime: 120_000,
    refetchOnWindowFocus: false,
  })

  const detailQuery = isLiveRoute ? liveDetailQuery : historicalDetailQuery

  useEffect(() => {
    if (!login || !targetQueryStreamId || isLiveRoute || syncing) return
    if (autoPrefetchedStreamId === targetQueryStreamId) return
    const data = detailQuery.data
    if (!data || data.state === 'live' || data.state === 'syncing') return
    const coverage = analyzeViewerCoverage(data.rollups ?? [])
    if (coverage.hasViewerRollups && !coverage.hasFlatViewerLine && !coverage.hasPartialTail && !coverage.hasShortSpan) {
      return
    }
    setAutoPrefetchedStreamId(targetQueryStreamId)
    prefetchAnalyticsTracker(targetQueryStreamId, login).catch(() => undefined)
  }, [login, targetQueryStreamId, isLiveRoute, syncing, detailQuery.data, autoPrefetchedStreamId])

  const gamesQuery = useQuery({
    queryKey: ['stream-games', targetQueryStreamId],
    queryFn: () => targetQueryStreamId ? getStreamGameSegments(targetQueryStreamId) : Promise.resolve([]),
    enabled: Boolean(targetQueryStreamId),
  })

  const heatmapQuery = useQuery({
    queryKey: ['replay-heatmap', targetQueryStreamId, login],
    queryFn: () => targetQueryStreamId ? getReplayHeatmap(targetQueryStreamId, 60, login) : Promise.resolve(null),
    enabled: Boolean(targetQueryStreamId),
    staleTime: 120_000,
    retry: 1,
  })

  const heatmapDetailQuery = useQuery({
    queryKey: ['replay-heatmap-detail', targetQueryStreamId, login],
    queryFn: () => targetQueryStreamId ? getReplayHeatmapDetail(targetQueryStreamId, 60, login) : Promise.resolve(null),
    enabled: Boolean(targetQueryStreamId && selectedRollup),
    staleTime: 120_000,
    retry: 1,
  })

  const recapQuery = useQuery({
    queryKey: ['pulse-stream-recap', targetQueryStreamId],
    queryFn: () => targetQueryStreamId ? getPulseStreamRecap(targetQueryStreamId) : Promise.resolve(null),
    enabled: Boolean(targetQueryStreamId && !isLiveRoute),
    staleTime: 120_000,
    retry: 1,
  })

  const handleRefresh = async () => {
    if (!login || refreshing) return
    setRefreshing(true)
    try {
      const isLiveView = isLiveRoute && detailQuery.data?.state !== 'historical'
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
        refetches.push(heatmapQuery.refetch())
        if (selectedRollup) refetches.push(heatmapDetailQuery.refetch())
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

  const detailHasChartData = (data?: AnalyticsStreamDetail | null) => {
    const rollups = data?.rollups ?? []
    return rollups.some(rollupHasMinuteData) || rollupsHaveViewerData(rollups)
  }

  const detailHasViewerData = (data?: AnalyticsStreamDetail | null) => {
    const rollups = data?.rollups ?? []
    const coverage = analyzeViewerCoverage(rollups)
    return coverage.hasViewerRollups
      && !coverage.hasFlatViewerLine
      && !coverage.hasPartialTail
      && !coverage.hasShortSpan
  }

  const viewersOnlySync = useMemo(() => {
    if (!targetQueryStreamId || streamId === '') return false
    const rollups = detailQuery.data?.rollups ?? []
    const hasChat = rollups.some(point => (point.chatCount ?? 0) > 0 || minuteEmoteTotal(point) > 0)
    const coverage = analyzeViewerCoverage(rollups)
    const hasRealViewers = coverage.hasViewerRollups
      && !coverage.hasFlatViewerLine
      && !coverage.hasPartialTail
      && !coverage.hasShortSpan
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

  const refetchChartDuringSync = () => {
    void detailQuery.refetch()
    if (targetQueryStreamId) void gamesQuery.refetch()
  }

  useEffect(() => {
    if (!syncing || !targetQueryStreamId) return
    refetchChartDuringSync()
    const timer = window.setInterval(() => {
      refetchChartDuringSync()
    }, 3000)
    return () => window.clearInterval(timer)
  // eslint-disable-next-line react-hooks/exhaustive-deps -- poll chart rollups while Redis sync is active
  }, [syncing, targetQueryStreamId])

  const handleSync = async (opts?: { forceChat?: boolean; chatOnly?: boolean }) => {
    if (!targetQueryStreamId) return
    const viewersOnly = opts?.chatOnly ? false : viewersOnlySync
    const forceChat = Boolean(opts?.forceChat)
    const pollCallbacks = syncPollChartCallbacks(refetchChartDuringSync, viewersOnly)
    const hintVodId = historyQuery.data?.items?.find(s => s.id === targetQueryStreamId)?.videoId
      || detailQuery.data?.vodId
      || undefined
    setSyncing(true)
    setSyncError(null)
    setSyncNotice(null)
    setSyncStatus(null)
    setRightPanelTab('sync')
    refetchChartDuringSync()
    try {
      const start = await startHistoricalSync(targetQueryStreamId, login, { viewersOnly, vodId: hintVodId, forceChat })
      if (start.status) {
        setSyncStatus(start.status)
      }
      if (!start.accepted && !isTerminalSyncPhase(start.status?.phase)) {
        setSyncNotice('Sync already running — showing live progress.')
      }
      const finalStatus = await pollSyncUntilDone(targetQueryStreamId, setSyncStatus, pollCallbacks)
      if (!finalStatus) {
        setSyncError('Lost sync status — try again or use Refresh data.')
        return
      }
      if (finalStatus.stale) {
        setSyncNotice('Sync interrupted — click to retry.')
        return
      }
      if (finalStatus.phase === 'failed') {
        setSyncError(finalStatus.error || 'Sync failed.')
        return
      }
      if (finalStatus.phase === 'export_pending') {
        setSyncNotice(finalStatus.error || 'Sync finished; archive export is pending.')
        await refreshSyncedQueries()
        return
      }
      setSyncNotice(friendlySyncNotice(finalStatus.resultMessage))
      const loaded = await waitForSyncedDetail({ viewersOnly })
      await refreshSyncedQueries()
      if (!loaded) {
        const hasViewers = detailHasViewerData(detailQuery.data)
        if (!viewersOnly && hasViewers) {
          setSyncNotice('Viewer chart synced — chat/emote minutes may still be finalizing. Use Refresh data if bands stay empty.')
        } else {
          setSyncNotice(current => current || 'Sync finished but chart data is still loading. Click Refresh data.')
        }
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'An error occurred during synchronization.'
      setSyncError(message)
    } finally {
      setSyncing(false)
    }
  }

  useEffect(() => {
    if (!targetQueryStreamId || syncing) return
    let cancelled = false
    void (async () => {
      const status = await getSyncStatus(targetQueryStreamId).catch(() => null)
      if (cancelled) return
      const detailSyncing = detailQuery.data?.state === 'syncing'
      const statusTerminal = Boolean(
        status
        && (isTerminalSyncPhase(status.phase) || status.stale),
      )
      const statusActive = Boolean(
        status
        && !statusTerminal
        && !status.stale,
      )
      if (statusTerminal) {
        if (detailSyncing) {
          void detailQuery.refetch()
        }
        return
      }
      if (!detailSyncing && !statusActive) return
      setSyncing(true)
      setRightPanelTab('sync')
      if (status) setSyncStatus(status)
      setSyncNotice('Sync in progress — resuming live progress.')
      const resumeViewersOnly = status?.viewersOnly ?? viewersOnlySync
      const finalStatus = await pollSyncUntilDone(
        targetQueryStreamId,
        setSyncStatus,
        syncPollChartCallbacks(refetchChartDuringSync, resumeViewersOnly),
      )
      if (cancelled) return
      setSyncing(false)
      if (!finalStatus) {
        setSyncNotice('')
        return
      }
      if (finalStatus.stale) {
        setSyncNotice('Sync interrupted — click to retry.')
        return
      }
      if (finalStatus.phase === 'failed') {
        setSyncError(finalStatus.error || 'Sync failed.')
        return
      }
      if (finalStatus.phase === 'completed') {
        setSyncNotice(friendlySyncNotice(finalStatus.resultMessage))
        await refreshSyncedQueries()
      } else if (finalStatus.phase === 'export_pending') {
        setSyncNotice(finalStatus.error || 'Sync finished; archive export is pending.')
        await refreshSyncedQueries()
      }
    })()
    return () => { cancelled = true }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- resume when detail or Redis reports active sync
  }, [targetQueryStreamId, detailQuery.data?.state])

  const historicalStream = useMemo(() => {
    if (!targetQueryStreamId || !historyQuery.data?.items) return undefined
    return historyQuery.data.items.find(s => s.id === targetQueryStreamId)
  }, [targetQueryStreamId, historyQuery.data?.items])

  const detailQueryMatchesRoute = useMemo(() => {
    const data = detailQuery.data
    if (!data) return false
    if (isLiveRoute) {
      return data.state === 'live' || data.state === 'not_collected' || !data.stream?.streamId
    }
    if (!targetQueryStreamId) return false
    const sid = data.stream?.streamId
    return !sid || sid === targetQueryStreamId
  }, [detailQuery.data, isLiveRoute, targetQueryStreamId])

  const needsSync = Boolean(
    targetQueryStreamId
    && historicalStream
    && !detailQuery.isLoading
    && !syncing
    && (!detailQuery.data || detailQuery.data.state === 'not_collected'),
  )

  const detail = useMemo(() => {
    const routeDetail = detailQueryMatchesRoute ? detailQuery.data : undefined
    if (routeDetail) {
      const base = routeDetail
      if (historicalStream && base.stream) {
        const s = base.stream
        const missingViewers = s.peakViewers === 0 && s.avgViewers === 0
        const hasHistoricalViewers = historicalStream.peakViewers > 0 || historicalStream.avgViewers > 0
        const placeholderTitle = isPlaceholderStreamTitle(s.title)
        const resolvedTitle = placeholderTitle
          ? (historicalStream.title?.trim() || matchedStream?.title?.trim())
          : undefined
        if ((missingViewers && hasHistoricalViewers) || resolvedTitle) {
          return {
            ...base,
            stream: {
              ...s,
              ...(missingViewers && hasHistoricalViewers
                ? {
                    avgViewers: historicalStream.avgViewers,
                    peakViewers: historicalStream.peakViewers,
                  }
                : {}),
              ...(resolvedTitle ? { title: resolvedTitle } : {}),
            },
          } satisfies AnalyticsStreamDetail
        }
      }
      return base
    }
    if (isHistoricalRoute && matchedStream) {
      return {
        channel: login,
        state: 'historical' as const,
        stream: matchedStream,
        rollups: [],
        topEmotes: [],
        sources: [{ source: 'twitchtracker', state: 'ready' as const, message: 'Historical stats from Twitch Tracker' }],
        updatedAt: Date.now(),
      }
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
  }, [detailQuery.data, detailQueryMatchesRoute, historicalStream, login, isHistoricalRoute, matchedStream])

  const headerState = useMemo(() => {
    if (dateSlugUnresolved) return 'not found'
    if (isHistoricalRoute) {
      if (detailQueryMatchesRoute && detail?.state && detail.state !== 'live') return detail.state
      if (syncing || detail?.state === 'syncing') return 'syncing'
      return 'historical'
    }
    return detail?.state || ((isLiveRoute ? liveDetailQuery.isLoading : detailQuery.isLoading) ? 'loading' : 'not_collected')
  }, [dateSlugUnresolved, isHistoricalRoute, detailQueryMatchesRoute, detail?.state, syncing, detailQuery.isLoading, isLiveRoute, liveDetailQuery.isLoading])

  const stream = detail?.stream
  const streamVodId = resolveAnalyticsVodId(detail)
  const rollupCount = detail?.rollups?.length ?? 0
  const isLongStreamChart = rollupCount >= 360
  const selectedHeatmapDetail = useMemo(() => {
    if (!selectedRollup) return null
    return heatmapDetailQuery.data?.points?.find(point => point.minuteTs === selectedRollup.minuteTs) ?? null
  }, [heatmapDetailQuery.data?.points, selectedRollup])
  const selectedHeatmapBackendPoint = useMemo(() => {
    if (!selectedRollup) return null
    return selectedHeatmapDetail
      ?? heatmapQuery.data?.points?.find(point => point.minuteTs === selectedRollup.minuteTs)
      ?? null
  }, [heatmapQuery.data?.points, selectedHeatmapDetail, selectedRollup])

  const selectRollupWithHeatmap = useCallback((rollup: AnalyticsMinuteRollup | null) => {
    setSelectedRollup(rollup)
  }, [])

  const liveHasRichHistory = useMemo(() => {
    if (!isLiveRoute) return false
    const rollups = detail?.rollups ?? []
    const hasLiveRollups = rollups.some(rollupHasMinuteData)
    if (hasLiveRollups) return false
    return combinedStreams.some(s => (s.viewerSamples ?? 0) > 0 || (s.chatMessages ?? 0) > 0)
  }, [isLiveRoute, detail?.rollups, combinedStreams])

  const sidebarStreams = routableStreams

  const isActiveLiveCollector = isActiveLiveCollectorStream(detail?.stream, detail?.state)

  const needsLiveCollectorRedirect = useMemo(() => {
    if (detailQuery.isLoading || streamsQuery.isLoading) return false
    if (isHistoricalRoute) {
      if (historyQuery.isLoading) return false
      // Historical slug resolved — stay on that session (stats-only or synced).
      if (matchedStream) return false
      if (historicalStream?.endedAt) return false
      if (!isSyncPrefetchPlaceholder(detail?.stream)) return false
      return true
    }
    if (!isLiveRoute) return false
    const rollups = detail?.rollups ?? []
    if (rollups.some(rollupHasMinuteData) || rollupsHaveViewerData(rollups)) return false
    // Active live collector: show warmup panel instead of bouncing to an older synced session.
    if (isActiveLiveCollectorStream(detail?.stream, detail?.state)) return false
    if ((detail?.stream?.currentViewers ?? 0) > 0) return false
    return true
  }, [
    detail?.rollups,
    detail?.stream,
    detailQuery.isLoading,
    historicalStream?.endedAt,
    historyQuery.isLoading,
    isHistoricalRoute,
    isLiveRoute,
    matchedStream,
    streamsQuery.isLoading,
  ])

  const syncedLiveStreamTarget = useMemo(() => {
    if (!needsLiveCollectorRedirect) return undefined
    return pickSyncedLiveStreamTarget(combinedStreams, {
      liveStreamId: detail?.stream?.streamId,
      channelLive: true,
    })
  }, [combinedStreams, detail?.stream?.streamId, needsLiveCollectorRedirect])

  const syncedLiveStreamSlug = useMemo(() => {
    if (!syncedLiveStreamTarget) return undefined
    return analyticsStreamPathSlug(syncedLiveStreamTarget, combinedStreams)
  }, [syncedLiveStreamTarget, combinedStreams])

  useEffect(() => {
    if (!login || !syncedLiveStreamSlug || !needsLiveCollectorRedirect) return
    if (detailQuery.isLoading || streamsQuery.isLoading) return
    if (syncedLiveStreamTarget?.streamId === targetQueryStreamId) return
    navigate(
      `/analytics/${encodeURIComponent(login)}/${encodeURIComponent(syncedLiveStreamSlug)}`,
      { replace: true },
    )
  }, [
    detailQuery.isLoading,
    login,
    navigate,
    needsLiveCollectorRedirect,
    streamsQuery.isLoading,
    syncedLiveStreamSlug,
    syncedLiveStreamTarget?.streamId,
    targetQueryStreamId,
  ])

  const chatOnlySyncAvailable = useMemo(() => {
    if (!targetQueryStreamId || isLiveRoute || coreMinuteChartsBlocked) return false
    const rollups = detail?.rollups ?? []
    const viewerSamples = detail?.stream?.viewerSamples ?? matchedStream?.viewerSamples ?? 0
    const hasViewerData = viewerSamples > 0 || detailHasViewerData(detail)
    if (!hasViewerData) return false
    const hasChat = rollups.some(point => (point.chatCount ?? 0) > 0 || minuteEmoteTotal(point) > 0)
      || (detail?.stream?.chatMessages ?? 0) > 0
    return !hasChat || Boolean(detail?.chatCoverage?.partial)
  }, [targetQueryStreamId, isLiveRoute, coreMinuteChartsBlocked, detail, matchedStream])

  const viewerDataFromExisting = useMemo(() => {
    if (!targetQueryStreamId || isLiveRoute) return false
    const viewerSamples = detail?.stream?.viewerSamples ?? matchedStream?.viewerSamples ?? 0
    return viewerSamples > 0 || detailHasViewerData(detail)
  }, [targetQueryStreamId, isLiveRoute, detail, matchedStream])

  const headerSyncLabel = useMemo(() => {
    // Req 4.1: a single canonical label shared across header, chart-empty,
    // right-rail sync area, and sync panel for the current stream state.
    const hasChatRollups =
      (detail?.rollups ?? []).some(point => !point.missing && (point.chatCount ?? 0) > 0) ||
      (detail?.stream?.chatMessages ?? 0) > 0
    const state: SyncStreamState = {
      hasViewerSamples: viewerDataFromExisting,
      hasChatRollups,
      syncing,
    }
    return syncCtaLabel(state)
  }, [syncing, viewerDataFromExisting, detail?.rollups, detail?.stream?.chatMessages])

  const headerStats = useMemo(() => {
    const rollups = detail?.rollups ?? []
    const viewerStats = computeRollupViewerStats(rollups)
    const chatStats = computeRollupChatStats(rollups)
    const chatSegments = syncStatus?.chat
    const emoteSegmentLabel = syncing && (chatSegments?.segmentsTotal ?? 0) > 0
      ? `${(chatSegments?.segmentsDone ?? 0).toLocaleString()} / ${(chatSegments?.segmentsTotal ?? 0).toLocaleString()} segments`
      : null
    return {
      current: viewerStats?.current ?? stream?.currentViewers,
      avg: viewerStats?.avg ?? stream?.avgViewers,
      peak: viewerStats?.peak ?? stream?.peakViewers,
      chat: chatStats.chat > 0 ? chatStats.chat : stream?.chatMessages,
      emotes: chatStats.emotes > 0 ? chatStats.emotes : stream?.totalEmoteUses,
      emoteLabel: emoteSegmentLabel,
    }
  }, [detail?.rollups, stream, syncing, syncStatus?.chat])
  // Req 6: classify each stat card as numeric or a source-appropriate placeholder
  // ("Stats only" / "Needs sync" / "Collecting") with a muted style (Req 6.5).
  const statCardClasses = useMemo(
    () =>
      classifyStatCards({
        state: (detail?.state as StreamCollectionState) ?? 'not_collected',
        avgViewers: stream?.avgViewers ?? 0,
        peakViewers: stream?.peakViewers ?? 0,
        rollups: detail?.rollups ?? [],
      }),
    [detail?.state, detail?.rollups, stream?.avgViewers, stream?.peakViewers],
  )
  // Req 3.2: the right rail resets to the Moments tab whenever the selected
  // stream changes (a full reload remounts and also defaults to Moments).
  const activeStreamKey = stream?.streamId || targetQueryStreamId || streamId
  const [prevRailStreamKey, setPrevRailStreamKey] = useState(activeStreamKey)
  if (activeStreamKey !== prevRailStreamKey) {
    setPrevRailStreamKey(activeStreamKey)
    setRightPanelTab('moments')
  }
  const sidebarRollupStats = useMemo(
    () => (syncing ? computeRollupViewerStats(detail?.rollups ?? []) : null),
    [syncing, detail?.rollups],
  )
  const chartEmoteKeys = useMemo(() => {
    const topEmoteKey = detail?.topEmotes?.[0]?.key
    if (analyticsViewMode === 'overview') {
      // Always surface the single most-used emote, even with nothing selected.
      if (selected.size > 0) return selected
      return topEmoteKey ? new Set([topEmoteKey]) : selected
    }
    if (selected.size > 0) return selected
    return new Set((detail?.topEmotes ?? []).slice(0, 4).map(emote => emote.key))
  }, [analyticsViewMode, selected, detail?.topEmotes])
  const timeseriesStatus = timeseriesStatusQuery.data
  const pulseTimeseriesReady =
    timeseriesStatus?.state === 'ready' &&
    (!timeseriesStatus.backfillState || timeseriesStatus.backfillState === 'completed')
  const pulseReady = setupQuery.data?.services.pulse === 'ready' || pulseTimeseriesReady
  const clipperReady = setupQuery.data?.services.clipper === 'ready'
  const pulseUrl = useMemo(
    () => pulseDashboardUrl(login, stream, targetQueryStreamId || undefined),
    [login, stream, targetQueryStreamId],
  )

  const toggleSelected = useCallback((key: string) => {
    setSelected(current => {
      const next = new Set(current)
      if (next.has(key)) next.delete(key)
      else if (next.size < 5) next.add(key)
      return next
    })
  }, [])

  const handleAnalyticsViewMode = useCallback((mode: AnalyticsViewMode) => {
    setAnalyticsViewMode(mode)
    if (mode === 'emotes') {
      setRightPanelTab('emotes')
    } else if (mode === 'spikes') {
      setRightPanelTab('moments')
    }
  }, [])

  return (
    <main className="min-h-screen overflow-y-auto bg-[#050507] text-zinc-100">
      <div className="mx-auto flex w-full max-w-[1500px] flex-col gap-4 px-4 py-4 lg:px-6">
        <header className="flex flex-col gap-3 border-b border-white/10 pb-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2 text-xs font-black uppercase text-zinc-500">
              <Link to="/" className="rounded bg-white/10 px-2 py-1 text-zinc-200 transition hover:bg-white/15">Live directory</Link>
              <Link to={`/c/${encodeURIComponent(login)}`} className="rounded bg-violet-400/15 px-2 py-1 text-violet-100 transition hover:bg-violet-400/25">{login}</Link>
              <span className={`rounded px-2 py-1 ${
                headerState === 'live'
                  ? 'bg-red-500/15 text-red-100'
                  : headerState === 'syncing'
                    ? 'bg-violet-500/15 text-violet-100'
                    : headerState === 'historical'
                      ? 'bg-cyan-500/10 text-cyan-100'
                      : 'bg-white/10 text-zinc-300'
              }`}>
                {streamStateLabel(headerState as AnalyticsStreamDetail['state'] | 'not found' | 'loading', isHistoricalRoute)}
              </span>
              <ChatCoverageBadge detail={detailQueryMatchesRoute ? detail : undefined} />
              <TierIndicator />
            </div>
            <h1 className="mt-3 truncate text-2xl font-black leading-tight text-white lg:text-3xl" title={displayStreamTitle(stream, login, [historicalStream?.title, matchedStream?.title])}>
              {displayStreamTitle(stream, login, [historicalStream?.title, matchedStream?.title])}
            </h1>
            <div className="mt-2 flex flex-wrap gap-2 text-sm font-bold text-zinc-500">
              {stream?.displayName ? <span>{stream.displayName}</span> : null}
              {stream?.category ? <span>{stream.category}</span> : null}
              {stream?.startedAt ? <span>Started {relativeTime(stream.startedAt)}</span> : null}
              {isHistoricalRoute && stream?.streamId ? (
                <>
                  <span className="font-mono text-[11px] text-zinc-600">{stream.streamId}</span>
                  <Link
                    to={`/logs/${encodeURIComponent(login)}/${encodeURIComponent(stream.streamId)}`}
                    className="rounded border border-violet-400/30 bg-violet-500/10 px-2 py-0.5 text-[11px] font-black uppercase text-violet-200 transition hover:bg-violet-500/20"
                  >
                    Open full chat log
                  </Link>
                </>
              ) : null}
              <span>
                {lastRefreshedAt
                  ? `Refreshed ${relativeTime(lastRefreshedAt)}`
                  : `Updated ${detail?.updatedAt ? relativeTime(detail.updatedAt) : '-'}`}
              </span>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <StackStatusButton />
            {pulseReady ? (
              <a
                href={pulseUrl}
                target="_blank"
                rel="noreferrer"
                className="rounded border border-emerald-400/30 bg-emerald-500/10 px-3 py-1.5 text-[11px] font-black uppercase text-emerald-100 transition hover:bg-emerald-500/20"
              >
                Open in Pulse
              </a>
            ) : null}
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
          <StatCard
            label="Current"
            value={statCardClasses.current.placeholder ?? count(headerStats.current)}
            tone={statCardClasses.current.muted ? STAT_PLACEHOLDER_MUTED_CLASS : 'text-cyan-300/90'}
          />
          <StatCard
            label="Average"
            value={statCardClasses.average.placeholder ?? count(headerStats.avg)}
            tone={statCardClasses.average.muted ? STAT_PLACEHOLDER_MUTED_CLASS : undefined}
          />
          <StatCard
            label="Peak"
            value={statCardClasses.peak.placeholder ?? count(headerStats.peak)}
            tone={statCardClasses.peak.muted ? STAT_PLACEHOLDER_MUTED_CLASS : undefined}
          />
          <StatCard
            label="Chat"
            value={statCardClasses.chat.placeholder ?? count(headerStats.chat)}
            tone={statCardClasses.chat.muted ? STAT_PLACEHOLDER_MUTED_CLASS : 'text-violet-300/90'}
          />
          <StatCard
            label="Emote Uses"
            value={statCardClasses.emoteUses.placeholder ?? (headerStats.emoteLabel ?? count(headerStats.emotes))}
            tone={statCardClasses.emoteUses.muted ? STAT_PLACEHOLDER_MUTED_CLASS : 'text-emerald-300/90'}
          />
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
          <aside className="order-3 min-w-0 xl:order-none xl:sticky xl:top-4 xl:self-start">
            <StreamSidebar
              login={login}
              streams={sidebarStreams}
              activeID={isHistoricalRoute ? (targetQueryStreamId || streamId) : undefined}
              isLiveView={isLiveRoute || isActiveLiveCollector}
              liveState={isActiveLiveCollector ? 'live' : (isLiveRoute ? detail?.state : undefined)}
              onPrefetchStream={prefetchStreamDetail}
              syncing={syncing}
              syncedOnly={syncedOnlyFilter}
              onSyncedOnlyChange={setSyncedOnlyFilter}
              coreMinuteChartsBlocked={coreMinuteChartsBlocked}
              activeRollupStats={sidebarRollupStats}
            />
          </aside>
          <section className="order-1 min-w-0 xl:order-none">
            <div className="min-w-0 space-y-4">
              <AnalyticsChart
                detail={detail}
                selectedEmotes={chartEmoteKeys}
                onSelectEmote={toggleSelected}
                selectedRollup={selectedRollup}
                onSelectRollup={selectRollupWithHeatmap}
                syncing={syncing}
                syncError={syncError}
                syncNotice={syncNotice}
                onSync={() => void handleSync(chatOnlySyncAvailable ? { chatOnly: true } : undefined)}
                onChatOnlySync={chatOnlySyncAvailable ? () => void handleSync({ chatOnly: true, forceChat: true }) : undefined}
                notInAnalyticsDb={needsSync}
                onRefresh={handleRefresh}
                refreshing={refreshing}
                loading={(isLiveRoute ? liveDetailQuery.isLoading : detailQuery.isLoading) && !matchedStream && !historicalStream}
                games={gamesQuery.data ?? []}
                canSync={!coreMinuteChartsBlocked && (Boolean(streamId) || needsSync)}
                isLive={isActiveLiveCollector}
                coreMinuteChartsBlocked={coreMinuteChartsBlocked}
                liveHasRichHistory={liveHasRichHistory}
                chatOnlySyncAvailable={chatOnlySyncAvailable}
                syncCtaLabel={headerSyncLabel}
                syncViewerStatus={syncStatus?.viewerStatus}
                viewMode={analyticsViewMode}
                onViewModeChange={handleAnalyticsViewMode}
              />
              <div className="space-y-4">
                <SelectedMomentPanel
                  rollup={selectedRollup}
                  rollups={detail?.rollups ?? []}
                  startedAt={stream?.startedAt}
                  vodId={streamVodId}
                  channel={login}
                  streamId={stream?.streamId || targetQueryStreamId || streamId}
                  topEmotesCatalog={detail?.topEmotes}
                  heatmapPoint={selectedHeatmapBackendPoint}
                  heatmapDetail={selectedHeatmapDetail}
                  isLiveView={detail?.state === 'live'}
                  channelLive={detail?.state === 'live'}
                />
              </div>
              {streamVodId ? (
                <p className="text-[11px] font-semibold text-zinc-500">
                  Select a moment to play the VOD in Streamclone, or open Twitch as a fallback.
                  <a
                    href={buildTwitchVodUrl(streamVodId)}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="ml-1 text-violet-300 hover:text-violet-200"
                  >
                    Open full VOD
                  </a>
                  {isLongStreamChart ? ' Long streams (6h+) may feel slower while hovering the chart.' : ''}
                </p>
              ) : null}
            </div>
          </section>
          <aside className="order-2 space-y-4 xl:order-none">
            {recapQuery.data ? (
              <StreamRecapPanel recap={recapQuery.data} />
            ) : null}
            <div className="rounded border border-white/10 bg-white/[0.035] overflow-hidden">
              <div className="flex border-b border-white/10 text-[10px] font-black uppercase bg-white/[0.015]">
                {(['moments', 'emotes', 'clips', 'sync'] as const).map(tab => (
                  <button
                    key={tab}
                    type="button"
                    onClick={() => setRightPanelTab(tab)}
                    className={`flex-1 py-2 text-center transition border-r border-white/10 last:border-r-0 ${
                      rightPanelTab === tab
                        ? 'bg-white/[0.04] text-white'
                        : 'text-zinc-500 hover:text-zinc-300'
                    }`}
                  >
                    {tab === 'moments' ? 'Moments' : tab === 'emotes' ? 'Emotes' : tab === 'clips' ? 'Clips' : 'Status'}
                  </button>
                ))}
              </div>
              <div className="p-0">
                {rightPanelTab === 'moments' ? (
                  <MomentReviewPanel
                    rollups={detail?.rollups ?? []}
                    selectedRollup={selectedRollup}
                    onSelectRollup={selectRollupWithHeatmap}
                    topEmotesCatalog={detail?.topEmotes}
                    heatmapPoints={heatmapDetailQuery.data?.points ?? heatmapQuery.data?.points}
                    streamStartedAt={stream?.startedAt}
                    embedded
                  />
                ) : null}
                {rightPanelTab === 'emotes' ? (
                  <TopEmoteTable emotes={detail?.topEmotes ?? []} selected={chartEmoteKeys} onSelect={toggleSelected} embedded />
                ) : null}
                {rightPanelTab === 'clips' ? (
                  <div>
                    <div className="flex border-b border-white/10 text-[10px] font-black uppercase">
                      <button
                        type="button"
                        onClick={() => setActiveClipsTab('edits')}
                        className={`flex-1 py-2 text-center transition border-r border-white/10 ${
                          activeClipsTab === 'edits' ? 'bg-white/[0.04] text-white' : 'text-zinc-500 hover:text-zinc-300'
                        }`}
                      >
                        Clipper
                      </button>
                      <button
                        type="button"
                        onClick={() => setActiveClipsTab('twitch')}
                        className={`flex-1 py-2 text-center transition ${
                          activeClipsTab === 'twitch' ? 'bg-white/[0.04] text-white' : 'text-zinc-500 hover:text-zinc-300'
                        }`}
                      >
                        Twitch
                      </button>
                    </div>
                    {activeClipsTab === 'edits' ? (
                      <RecentClipsList
                        login={login}
                        clipperReady={clipperReady}
                        isTab={true}
                        rollups={detail?.rollups ?? []}
                        onSync={() => void handleSync(chatOnlySyncAvailable ? { chatOnly: true } : undefined)}
                        onOpenMoments={() => setRightPanelTab('moments')}
                      />
                    ) : (
                      <TwitchDayClipsList
                        login={login}
                        startedAt={stream?.startedAt || ''}
                        endedAt={stream?.endedAt || new Date().toISOString()}
                      />
                    )}
                  </div>
                ) : null}
                {rightPanelTab === 'sync' ? (
                  <div className="space-y-3 p-3">
                    {chatOnlySyncAvailable ? (
                      <div className="rounded border border-cyan-500/20 bg-cyan-500/10 px-3 py-2 text-[11px] font-semibold leading-snug text-cyan-100">
                        Viewer minutes are already synced. VOD chat uses Twitch GQL only — no scraper profile required.
                      </div>
                    ) : null}
                    {syncing ? (
                      <SyncProgressPanel
                        status={syncStatus}
                        chartChatMinutes={(detail?.rollups ?? []).filter(p => (p.chatCount ?? 0) > 0).length}
                        viewerDataFromExisting={viewerDataFromExisting}
                        chatOnlyPath={chatOnlySyncAvailable}
                      />
                    ) : null}
                    {syncNotice ? <div className="text-xs font-bold text-amber-200">{syncNotice}</div> : null}
                    {syncError ? <div className="text-xs font-bold text-red-300">{syncError}</div> : null}
                    {!syncing && !syncNotice && !syncError ? (
                      <div className="text-[11px] font-semibold text-zinc-500">
                        Sync progress appears here while VOD chat and emotes index. Start sync from the chart when a stream needs minute data.
                      </div>
                    ) : null}
                  </div>
                ) : null}
              </div>
            </div>
          </aside>
        </div>
      </div>
    </main>
  )
}

function RecentClipsList({
  login,
  clipperReady,
  isTab = false,
  rollups,
  onSync,
  onOpenMoments,
}: {
  login: string
  clipperReady: boolean
  isTab?: boolean
  rollups?: AnalyticsMinuteRollup[]
  onSync?: () => void
  onOpenMoments?: () => void
}) {
  const [jobs, setJobs] = useState<ClipperJob[]>([])
  const [loading, setLoading] = useState(true)
  const [retryingId, setRetryingId] = useState<string | null>(null)
  const [retryError, setRetryError] = useState<string | null>(null)

  const fetchJobs = async () => {
    if (!clipperReady) {
      setJobs([])
      setLoading(false)
      return
    }
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

  const handleRetryJob = async (jobId: string) => {
    setRetryingId(jobId)
    setRetryError(null)
    try {
      const result = await retryClipperJob(jobId)
      window.dispatchEvent(new CustomEvent('streamclone:clip-created'))
      if (result.job.id !== jobId) {
        window.location.href = studioPath(result.job.id)
        return
      }
      await fetchJobs()
    } catch (err: unknown) {
      setRetryError(err instanceof Error ? err.message : 'Could not retry clip job.')
    } finally {
      setRetryingId(null)
    }
  }

  useEffect(() => {
    if (!clipperReady) {
      setJobs([])
      setLoading(false)
      return
    }
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
  }, [login, clipperReady])

  if (!clipperReady) {
    return (
      <div className={`p-3 text-center text-xs font-semibold text-zinc-500 ${!isTab ? 'rounded border border-white/10 bg-white/[0.035]' : ''}`}>
        Clip Studio is offline. Start Clip Studio from Stack status to view Streamclone clip jobs.
      </div>
    )
  }

  if (loading) {
    return <div className="text-zinc-500 text-xs font-semibold px-3 py-2">Loading clips list...</div>
  }

  if (!jobs.length) {
    // Req 35: honest Clips empty state — instruct sync-first when no chat/emote
    // rollups exist, or point at Moments/heatmap peaks when rollups exist but no
    // clip jobs are queued. Never "click the graph".
    if (rollups) {
      return (
        <div className={`p-3 ${!isTab ? 'rounded border border-white/10 bg-white/[0.035]' : ''}`}>
          <ClipsTabEmptyState
            rollups={rollups}
            clipJobCount={0}
            onSync={onSync}
            onOpenMoments={onOpenMoments}
          />
        </div>
      )
    }
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
              {describeClipperJobState(job)}
            </span>
            <span className="text-zinc-500">{relativeTime(job.created_at)}</span>
          </div>
          {job.state === 'failed' && (
            <>
              <div className="mt-1 text-[10px] font-semibold text-red-300/90 line-clamp-3">
                {describeClipperFailure(job)}
              </div>
              {isClipperAuthFailure(job) || isClipperAuthFailureMessage(job.error_message) ? (
                <ClipperAuthHelp compact onSynced={() => void fetchJobs()} />
              ) : null}
            </>
          )}
          <div className="mt-2 flex gap-2">
            {isClipperJobRetryable(job) ? (
              <button
                type="button"
                onClick={() => void handleRetryJob(job.id)}
                disabled={retryingId === job.id}
                className="flex-1 rounded bg-amber-600/20 border border-amber-500/30 px-2 py-1 text-center text-[10px] font-bold text-amber-100 transition hover:bg-amber-600/35 disabled:opacity-50"
              >
                {retryingId === job.id ? 'Retrying…' : 'Retry'}
              </button>
            ) : null}
            <Link
              to={studioPath(job.id)}
              className="flex-1 rounded bg-violet-600/20 border border-violet-500/30 px-2 py-1 text-center text-[10px] font-bold text-violet-200 transition hover:bg-violet-600/35"
            >
              Open in Studio
            </Link>
            {job.state === 'ready' && job.artifact_available === 1 && (
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
      {retryError ? (
        <div className="px-3 py-2 text-[10px] font-semibold text-red-300">{retryError}</div>
      ) : null}
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
