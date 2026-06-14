import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'

import {
  ensureChannelEmotes,
  getAnalyticsLive,
  getAnalyticsStream,
  getAnalyticsStreams,
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
  type SourceStatus,
  type AnalyticsMinuteRollup,
  type ClipperJob,
  type GameSegment,
} from '../api'
import TierIndicator from './analytics/TierIndicator'
import ClipsTabEmptyState from './analytics/ClipsTabEmptyState'
import MomentDrawer from './analytics/MomentDrawer'
import LiveStatsBand from './analytics/LiveStatsBand'
import MostReactedLive from './analytics/MostReactedLive'
import type { HeatmapEmote, ReplayHeatmapDetailPoint, ReplayHeatmapPoint } from '../types/heatmap'
import { syncCtaLabel, type SyncStreamState } from '../utils/syncLabel'
import {
  classifyStatCards,
  STAT_PLACEHOLDER_MUTED_CLASS,
  type StreamCollectionState,
} from '../utils/statCards'
import { classifyLiveEmptyState, COLLECTING_FIRST_MINUTES_MESSAGE } from '../utils/liveEmptyState'

import { coreMinuteChartsNeedScraper } from '../setupProfile'
import { buildTwitchVodUrl, resolveAnalyticsVodId } from '../utils/twitchVodUrl'
import { buildVodDeepLink } from '../utils/vodDeepLink'
import { usePlayheadStore } from '../stores/playheadStore'
import { computeChartCursorSync } from '../utils/chartCursorSync'
import { buildMomentScoreModel, clampMomentScore } from '../utils/momentScore'
import { CHART_THEME, hexToRgba, legendDotStyle } from './analytics/chartTheme'
import OptionalServicesPanel, { CoreMinuteChartsNotice } from './OptionalServicesPanel'
import ClipperAuthHelp from './ClipperAuthHelp'
import StackStatusButton from './StackStatusButton'
import {
  emoteCountForProvider,
  emoteProviderLabel,
  emoteProviderTone,
  parseEmoteKey,
} from '../emoteUtils'

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

function rollupOffsetSeconds(rollup: AnalyticsMinuteRollup, startedAt?: string): number {
  if (!startedAt) return 0
  const startMs = new Date(startedAt).getTime()
  const minuteMs = new Date(rollup.minuteTs).getTime()
  if (!Number.isFinite(startMs) || !Number.isFinite(minuteMs)) return 0
  return Math.max(0, Math.floor((minuteMs - startMs) / 1000))
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

function isPlaceholderStreamTitle(title?: string) {
  const trimmed = title?.trim() ?? ''
  return trimmed === '' || trimmed === 'Syncing...' || trimmed === 'Syncing…'
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

function seriesMax(values: Array<number | null>) {
  return Math.max(0, ...values.map(value => value ?? 0))
}

function viewerValue(point: AnalyticsMinuteRollup) {
  return point.viewerLatest || point.viewerAvg || point.viewerMax || 0
}

function analyzeViewerCoverage(rollups: AnalyticsMinuteRollup[]) {
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

function minuteEmoteTotal(point: AnalyticsMinuteRollup) {
  const total = point.totalEmoteCount ?? 0
  if (total > 0) return total
  if (!point.emotes) return 0
  return Object.values(point.emotes).reduce((sum, count) => sum + count, 0)
}

function computeStreamBaselines(rollups: AnalyticsMinuteRollup[]) {
  const data = rollups.filter(point => !point.missing && rollupHasMinuteData(point))
  if (!data.length) return { chat: 1, emotes: 1, viewers: 1 }
  return {
    chat: data.reduce((sum, point) => sum + (point.chatCount ?? 0), 0) / data.length || 1,
    emotes: data.reduce((sum, point) => sum + minuteEmoteTotal(point), 0) / data.length || 1,
    viewers: data.reduce((sum, point) => sum + viewerValue(point), 0) / data.length || 1,
  }
}

type MomentReason =
  | 'viewer_spike'
  | 'chat_spike'
  | 'emote_spike'
  | 'seventv_spike'
  | 'twitch_emote_spike'
  | 'ffz_spike'
  | 'manual'

function detectPickReason(
  rollup: AnalyticsMinuteRollup,
  baselines: { chat: number; emotes: number; viewers: number },
  catalog?: AnalyticsTopEmote[],
): MomentReason {
  const chatMult = (rollup.chatCount ?? 0) / baselines.chat
  const emoteMult = minuteEmoteTotal(rollup) / baselines.emotes
  const viewerMult = viewerValue(rollup) / baselines.viewers
  if (chatMult >= 2 && chatMult >= emoteMult) return 'chat_spike'
  if (emoteMult >= 2) {
    const top = topEmotesFromRollup(rollup, 1, catalog)[0]
    if (top?.provider === 'seventv') return 'seventv_spike'
    if (top?.provider === 'twitch') return 'twitch_emote_spike'
    if (top?.provider === 'ffz') return 'ffz_spike'
    return 'emote_spike'
  }
  if (viewerMult >= 1.5) return 'viewer_spike'
  return 'manual'
}

function computeMomentScore(
  rollup: AnalyticsMinuteRollup,
  baselines: { chat: number; emotes: number; viewers: number },
  rollups?: AnalyticsMinuteRollup[],
): number {
  const chatNorm = Math.min(1, (rollup.chatCount ?? 0) / Math.max(baselines.chat * 2, 1))
  const emoteNorm = Math.min(1, minuteEmoteTotal(rollup) / Math.max(baselines.emotes * 2, 1))
  const viewerNorm = Math.min(1, viewerValue(rollup) / Math.max(baselines.viewers * 1.5, 1))

  const topEmotes = topEmotesFromRollup(rollup, 3)
  const emoteTotal = Math.max(1, minuteEmoteTotal(rollup))
  const keywordNorm = topEmotes.length > 0
    ? Math.min(1, topEmotes[0].count / (emoteTotal * 0.4))
    : 0

  let noveltyNorm = 0.5
  if (rollups?.length) {
    const idx = rollups.findIndex(point => point.minuteTs === rollup.minuteTs)
    if (idx > 0) {
      const prior = rollups.slice(Math.max(0, idx - 5), idx).filter(point => !point.missing)
      if (prior.length > 0) {
        const priorChat = prior.reduce((sum, point) => sum + (point.chatCount ?? 0), 0) / prior.length
        const delta = (rollup.chatCount ?? 0) - priorChat
        noveltyNorm = Math.min(1, Math.max(0, delta / Math.max(baselines.chat, 1)))
      }
    }
  }

  const weighted =
    chatNorm * 0.35 +
    emoteNorm * 0.25 +
    viewerNorm * 0.15 +
    keywordNorm * 0.15 +
    noveltyNorm * 0.10
  return weighted * 10
}

type RollupEmoteHit = {
  key: string
  name: string
  provider?: string
  count: number
  image_url?: string
}

function topEmotesFromRollup(
  rollup: AnalyticsMinuteRollup,
  limit = 5,
  catalog?: AnalyticsTopEmote[],
): RollupEmoteHit[] {
  if (!rollup.emotes) return []
  const byKey = new Map(catalog?.map(item => [item.key, item]) ?? [])
  const byName = new Map(catalog?.map(item => [item.name.toLowerCase(), item]) ?? [])
  return Object.entries(rollup.emotes)
    .sort((a, b) => b[1] - a[1])
    .slice(0, limit)
    .map(([key, count]) => {
      const parsed = parseEmoteKey(key)
      const match = byKey.get(key) ?? byName.get(parsed.name.toLowerCase())
      return {
        key,
        name: match?.name ?? parsed.name,
        provider: match?.provider ?? (parsed.provider !== 'unknown' ? parsed.provider : undefined),
        count,
        image_url: match ? getEmoteImageUrl(match) : (parsed.id ? `/emotes/${parsed.id}/1x.webp` : undefined),
      }
    })
}

function computeMomentScore100(
  rollup: AnalyticsMinuteRollup,
  baselines: { chat: number; emotes: number; viewers: number },
  rollups?: AnalyticsMinuteRollup[],
): number {
  return clampMomentScore(computeMomentScore(rollup, baselines, rollups) * 10)
}

function heatmapEmotesFromRollup(
  rollup: AnalyticsMinuteRollup,
  limit = 5,
  catalog?: AnalyticsTopEmote[],
): HeatmapEmote[] {
  return topEmotesFromRollup(rollup, limit, catalog).map(emote => ({
    id: emote.key,
    name: emote.name,
    imageUrl: emote.image_url ?? '',
    count: emote.count,
    provider: emote.provider ?? 'unknown',
  }))
}

function peakEmoteCount(rollup: AnalyticsMinuteRollup): number {
  if (!rollup.emotes) return 0
  return Math.max(0, ...Object.values(rollup.emotes))
}

function rollupHasMinuteData(point: AnalyticsMinuteRollup) {
  return !point.missing && (
    (point.viewerSamples ?? 0) > 0
    || viewerValue(point) > 0
    || (point.chatCount ?? 0) > 0
    || minuteEmoteTotal(point) > 0
  )
}

function rollupsHaveViewerData(rollups: AnalyticsMinuteRollup[]) {
  return rollups.some(point => !point.missing && viewerValue(point) > 0)
}

function computeRollupViewerStats(rollups: AnalyticsMinuteRollup[]) {
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

function computeRollupChatStats(rollups: AnalyticsMinuteRollup[]) {
  let chat = 0
  let emotes = 0
  for (const point of rollups) {
    if (point.missing) continue
    chat += point.chatCount ?? 0
    emotes += minuteEmoteTotal(point)
  }
  return { chat, emotes }
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
  const segmentCleanup = segmentsTrackable && !segmentsComplete && timelinePct >= 99
  return { chatFetchDone, chatFetchActive, chatFetchStarted, segmentsTrackable, segmentsComplete, segmentCleanup, timelinePct }
}

function chatFetchDetailLabel(
  status: SyncStatus,
  chatFetchDone: boolean,
  segmentsTrackable: boolean,
  segmentCleanup: boolean,
  timelinePct: number,
  segmentDone: number,
  segmentTotal: number,
): string {
  if (chatFetchDone) return 'All comments fetched'
  if (status.chat?.throttled) return 'Waiting on rate limit'
  if (segmentsTrackable) {
    if (segmentCleanup) return 'Final segment cleanup'
    return `Timeline indexed: ${timelinePct}% · ${segmentDone.toLocaleString()}/${segmentTotal.toLocaleString()} segments`
  }
  return `${(status.chat?.commentsFetched ?? 0).toLocaleString()} comments indexed`
}

function syncIndexPhaseDetail(chat?: SyncChatProgress, rollupsWritten = 0) {
  switch (chat?.indexPhase) {
    case 'tokenizing':
      return 'Tokenizing emotes…'
    case 'writing': {
      const index = chatIndexProgress(chat, rollupsWritten)
      return index ? `Writing ${index.written}/${index.expected} chat minutes` : 'Writing chat minutes…'
    }
    case 'done':
      return 'Chat indexed'
    default:
      return ''
  }
}

function shouldRefetchChartDuringSync(status: SyncStatus | null, viewersOnly: boolean) {
  if (!status) return false
  if (viewersOnly) {
    return status.viewerStatus === 'ok' || (status.rollupsWritten ?? 0) > 0
  }
  if (status.viewerStatus === 'ok') {
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
      if (['parsing_tracker', 'resolving_vod', 'fetching_comments', 'writing_rollups', 'completed'].includes(phase)) {
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
  if (status.chat?.indexPhase === 'writing') return 'Writing rollups…'
  if (chatTimeline && chatTimeline.pct < 100 && status.chat?.active) return ''
  return ''
}

function syncOverallProgress(
  status: SyncStatus,
  viewerStepState: 'done' | 'active' | 'pending' | 'failed',
  chatStepState: 'done' | 'active' | 'pending',
  rollupStepState: 'done' | 'active' | 'pending',
  viewerPct: number,
  chatFetchPct: number,
  rollupPct: number,
  chatOnlyPath: boolean,
) {
  if (status.phase === 'completed') return { pct: 100, stageLabel: 'Complete' }
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
    if (viewerStepState === 'done') {
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
  return { pct: vodRange[0], stageLabel: chatOnlyPath ? 'VOD lookup' : 'Initial setup' }
}

function SyncStepIcon({ state }: { state: 'done' | 'active' | 'pending' | 'failed' }) {
  if (state === 'failed') {
    return (
      <span className="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-amber-500/15 text-amber-300">
        <svg className="h-3 w-3" viewBox="0 0 12 12" fill="none" aria-hidden>
          <path d="M6 2.5v3.25M6 8.75h.01" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
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
  if (!status) return null
  if (status.stale && status.phase !== 'completed' && status.phase !== 'failed') {
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
  const isIndexing = indexPhase === 'tokenizing' || indexPhase === 'writing'
  const viewersChartReady = status.viewerStatus === 'ok' || (status.rollupsWritten ?? 0) > 0
  const segmentTotal = status.chat?.segmentsTotal ?? 0
  const segmentDone = status.chat?.segmentsDone ?? 0
  const {
    chatFetchDone,
    chatFetchActive,
    segmentsTrackable,
    segmentCleanup,
    timelinePct: chatTimelinePct,
  } = chatFetchProgressState(status, chatTimeline, segmentDone, segmentTotal)
  const chatFetchPct = segmentsTrackable
    ? Math.round((segmentDone / segmentTotal) * 100)
    : (chatTimeline?.pct ?? 0)
  const chatFetchLabel = chatFetchDetailLabel(
    status,
    chatFetchDone,
    segmentsTrackable,
    segmentCleanup,
    chatTimelinePct,
    segmentDone,
    segmentTotal,
  )

  const viewerFailed = status.viewerStatus === 'failed'
  const viewerStepState: 'done' | 'active' | 'pending' | 'failed' = viewersChartReady
    ? 'done'
    : viewerFailed
      ? 'failed'
      : (['scraping_tracker', 'parsing_tracker'].includes(status.phase) ? 'active' : 'pending')
  const viewerPct = viewersChartReady
    ? 100
    : viewerFailed
      ? 0
      : status.phase === 'scraping_tracker'
        ? 35
        : status.phase === 'parsing_tracker'
          ? 70
          : 0

  const chatStepState: 'done' | 'active' | 'pending' = chatFetchDone
    ? 'done'
    : chatFetchActive
      ? 'active'
      : 'pending'

  const rollupStepState: 'done' | 'active' | 'pending' = indexPhase === 'done' || status.phase === 'completed'
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

  const segmentCells = segmentTotal > 1
    ? segmentGridBuckets(segmentTotal, segmentDone, Boolean(status.chat?.active), Boolean(status.chat?.throttled))
    : []
  const viewerStepUsesExisting = viewerDataFromExisting || chatOnlyPath || status.viewerStatus === 'skipped'
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
              {overallProgress.stageLabel}
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

      <div className="mt-4 space-y-4">
        {/* Viewers */}
        <section>
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2.5">
              <SyncStepIcon state={viewerStepState} />
              <div>
                <div className="text-[11px] font-bold text-zinc-200">Viewers (TwitchTracker)</div>
                <div className="text-[10px] font-semibold text-zinc-500">
                  {viewersChartReady
                    ? (viewerStepUsesExisting ? 'Using existing viewer rollups' : 'Viewer chart ready')
                    : viewerStepState === 'pending'
                      ? 'Waiting for TwitchTracker viewer minutes'
                      : status.tracker?.message || 'Scraping viewer timeline'}
                </div>
              </div>
            </div>
            <span className={`text-[11px] font-black tabular-nums ${
              viewerStepState === 'done' ? 'text-emerald-400' : viewerStepState === 'failed' ? 'text-amber-300' : 'text-zinc-400'
            }`}>
              {viewerStepState === 'done' ? '100%' : viewerStepState === 'failed' ? 'Skipped' : viewerStepState === 'active' ? `${viewerPct}%` : 'Pending'}
            </span>
          </div>
          <SyncProgressBar pct={viewerPct} tone="green" pending={viewerStepState === 'pending'} />
        </section>

        {/* VOD Chat Fetch */}
        {!status.viewersOnly ? (
          <section>
            <div className="flex items-center justify-between gap-3">
              <div className="flex items-center gap-2.5">
                <SyncStepIcon state={chatStepState} />
                <div>
                  <div className="text-[11px] font-bold text-zinc-200">VOD Chat Fetch (Twitch GQL)</div>
                  <div className="text-[10px] font-semibold text-zinc-500">
                    {chatFetchLabel}
                  </div>
                </div>
              </div>
              <span className={`text-[11px] font-black tabular-nums ${chatStepState === 'done' ? 'text-emerald-400' : 'text-violet-300'}`}>
                {chatStepState === 'pending' ? 'Pending' : `${chatFetchPct}%`}
              </span>
            </div>
            <SyncProgressBar pct={chatFetchPct} tone="violet" pending={chatStepState === 'pending'} />

            {chatStepState !== 'pending' && chatTimeline ? (
              <div className="mt-2.5 space-y-1 text-[10px] font-semibold text-zinc-500">
                <div>
                  <span className="text-zinc-400">Timeline coverage:</span>{' '}
                  {chatTimeline.pct}% · {formatVodClock(chatTimeline.timeline)} / {formatVodClock(chatTimeline.total)}
                </div>
                {segmentTotal > 1 ? (
                  <div>
                    <span className="text-zinc-400">Segments completed:</span>{' '}
                    {segmentDone.toLocaleString()} / {segmentTotal.toLocaleString()}
                  </div>
                ) : null}
                {((status.chat?.commentsFetched ?? 0) > 0 || (status.chat?.gqlPages ?? 0) > 0) ? (
                  <div>
                    <span className="text-zinc-400">Comments / pages:</span>{' '}
                    {(status.chat?.commentsFetched ?? 0).toLocaleString()} comments
                    {(status.chat?.gqlPages ?? 0) > 0 ? ` · ${(status.chat?.gqlPages ?? 0).toLocaleString()} GQL pages` : ''}
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
    </div>
  )
}

async function pollSyncUntilDone(
  streamId: string,
  onUpdate: (status: SyncStatus | null) => void,
  opts?: { onPhase?: (phase: SyncPhase) => void; onProgress?: (status: SyncStatus) => void },
) {
  const terminal: SyncPhase[] = ['completed', 'failed']
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
    const value = viewerValue(point)
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
}) {
  const [hover, setHover] = useState<number | null>(null)
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
  const seriesFocusOpacity = useCallback((seriesKey: string, base: number) => {
    if (!focusedSeriesKey) return base
    return seriesKey === focusedSeriesKey ? base : base * 0.14
  }, [focusedSeriesKey])
  const toggleSeriesFocus = useCallback((seriesKey: string) => {
    setFocusedSeriesKey(current => current === seriesKey ? null : seriesKey)
  }, [])
  const allRollups = detail?.rollups ?? []
  const rollups = useMemo(() => rollupsForChart(allRollups, isLive), [allRollups, isLive])
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
  const partialChatCoverage = !isLive && !syncing && Boolean(detail?.chatCoverage?.partial)
  const width = 1000
  const height = CHART_VIEWBOX_HEIGHT
  const padLeft = 90
  const padRight = 34
  const padTop = 34
  const padBottom = 34

  const series = useMemo(
    () => buildSeries(rollups, selectedEmotes, peakViewersFallback, avgViewersFallback, useViewerFallback),
    [rollups, selectedEmotes, peakViewersFallback, avgViewersFallback, useViewerFallback],
  )
  const viewersItem = useMemo(() => series.find(s => s.key === 'viewers'), [series])
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
      viewersItem.values,
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
  }, [viewersItem, viewerAxis, width, height, padLeft, padRight, padTop, padBottom])
  const viewerTailStart = useMemo(() => {
    if (!needsViewerResync || !viewersItem) return -1
    return findEstimatedViewerTailStart(viewersItem.values)
  }, [needsViewerResync, viewersItem])
  const viewerLineSegments = useMemo(() => {
    if (!viewersItem) return []
    if (viewerTailStart <= 0) {
      return [{ values: viewersItem.values, estimated: false }]
    }
    return [
      {
        values: viewersItem.values.map((value, index) => (index < viewerTailStart ? value : null)),
        estimated: false,
      },
      {
        values: viewersItem.values.map((value, index) => (index >= viewerTailStart - 1 ? value : null)),
        estimated: true,
      },
    ]
  }, [viewersItem, viewerTailStart])
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
      return (
        <div className="grid min-h-80 place-items-center rounded border border-white/10 bg-[#0d0d12]/50 backdrop-blur-md px-4 text-center">
          <div>
            <div className="flex items-center justify-center gap-2">
              <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-violet-500/30 border-t-violet-400" />
              <div className="text-base font-black text-zinc-100">{COLLECTING_FIRST_MINUTES_MESSAGE}</div>
            </div>
            <div className="mt-1 text-sm font-semibold text-zinc-500 max-w-md">
              Live collection is active. The chart appears as soon as the first minutes of viewer, chat, and emote data arrive.
            </div>
          </div>
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
        <div className="flex items-center gap-3">
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
              onClick={() => setShowSpikes(v => !v)}
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
                ? `Fit: viewers ${count(viewerAxis.min)}–${count(viewerAxis.max)}, total emotes ${count(activityScaleMin)}–${count(activityScaleMax)}, selected emotes ${count(selectedEmoteScaleMin)}–${count(selectedEmoteScaleMax)}. Click for peak scale.`
                : `Peak: viewers 0–${count(viewerPeakAxis.max)}, emotes 0–${count(activityScaleMax)}. Click to zoom selected emotes into visible min–max.`}
              className={`rounded border px-2 py-1 text-[10px] font-black uppercase transition ${expandedScale ? 'border-violet-400/30 bg-violet-400/10 text-violet-200' : 'border-white/10 bg-white/[0.04] text-zinc-500 hover:text-zinc-300'}`}
            >
              {expandedScale ? 'Fit' : 'Peak'}
            </button>
          </div>
        </div>
      </div>
      <div className="overflow-hidden rounded">
      <svg
        viewBox={`0 0 ${width} ${height}`}
        className="h-[min(420px,52vh)] min-h-[320px] w-full cursor-crosshair select-none"
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
            setHover(Math.round(pct * (rollups.length - 1)))
          }}
          onMouseLeave={() => setHover(null)}
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
      <div className="mt-3 flex flex-wrap gap-1.5">
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
  onSync,
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
  onSync?: () => void
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
    if (!syncedOnly) return streams
    return streams.filter(s => (s.viewerSamples ?? 0) > 0 || (s.chatMessages ?? 0) > 0)
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
            {liveState === 'live' ? 'Collecting now' : 'Most recent session'}
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
                              ? 'Session stats only. Minute charts require the Analytics (scraper) tier.'
                              : 'Session stats only (duration, title). Use Sync chat/emotes on the stream detail page for minute charts.'
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
                  {!hasMinuteData && isActive && onSync && !coreMinuteChartsBlocked ? (
                    <button
                      type="button"
                      onClick={e => {
                        e.preventDefault()
                        e.stopPropagation()
                        onSync()
                      }}
                      disabled={syncing}
                      className="mt-1.5 rounded border border-violet-400/30 bg-violet-500/10 px-2 py-0.5 text-[9px] font-black uppercase text-violet-200 hover:bg-violet-500/20 disabled:opacity-50"
                    >
                      {syncing ? 'Syncing…' : 'Sync for charts'}
                    </button>
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

type MomentSortMode = 'score' | 'chat' | 'emotes' | 'seventv' | 'twitch' | 'top_emote' | 'time'

const MOMENT_SORT_OPTIONS: Array<{ id: MomentSortMode; label: string }> = [
  { id: 'score', label: 'Score' },
  { id: 'chat', label: 'Chat' },
  { id: 'emotes', label: 'Emotes' },
  { id: 'seventv', label: '7TV' },
  { id: 'twitch', label: 'Twitch' },
  { id: 'top_emote', label: 'Top emote' },
  { id: 'time', label: 'Latest' },
]

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
  embedded = false,
}: {
  rollups: AnalyticsMinuteRollup[]
  selectedRollup: AnalyticsMinuteRollup | null
  onSelectRollup: (rollup: AnalyticsMinuteRollup) => void
  topEmotesCatalog?: AnalyticsTopEmote[]
  heatmapPoints?: ReplayHeatmapPoint[]
  embedded?: boolean
}) {
  const [sortBy, setSortBy] = useState<MomentSortMode>('score')
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
    const sorted = [...rows]
    if (sortBy === 'chat') {
      sorted.sort((a, b) => (b.rollup.chatCount ?? 0) - (a.rollup.chatCount ?? 0))
    } else if (sortBy === 'emotes') {
      sorted.sort((a, b) => minuteEmoteTotal(b.rollup) - minuteEmoteTotal(a.rollup))
    } else if (sortBy === 'seventv') {
      sorted.sort((a, b) => emoteCountForProvider(b.rollup, 'seventv') - emoteCountForProvider(a.rollup, 'seventv'))
    } else if (sortBy === 'twitch') {
      sorted.sort((a, b) => emoteCountForProvider(b.rollup, 'twitch') - emoteCountForProvider(a.rollup, 'twitch'))
    } else if (sortBy === 'top_emote') {
      sorted.sort((a, b) => peakEmoteCount(b.rollup) - peakEmoteCount(a.rollup))
    } else if (sortBy === 'time') {
      sorted.sort((a, b) => b.rollup.minuteTs.localeCompare(a.rollup.minuteTs))
    } else {
      sorted.sort((a, b) => b.score - a.score)
    }
    return sorted.slice(0, 10)
  }, [rollups, baselines, sortBy, topEmotesCatalog, heatmapPointMap])

  if (candidates.length < 2) {
    return (
      <div className={`${embedded ? 'px-3 py-4' : 'rounded border border-white/10 bg-[#0d0d12] p-3'} text-center text-[11px] font-semibold text-zinc-500`}>
        {rollups.some(rollupHasMinuteData) ? 'Not enough peaks yet — sync chat or wait for more minutes.' : 'Sync chat/emotes to surface ranked moments.'}
      </div>
    )
  }

  return (
    <div className={embedded ? 'p-3' : 'rounded border border-white/10 bg-[#0d0d12] p-3'}>
      <div className="mb-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="text-[11px] font-black uppercase text-zinc-500">Top Moments</div>
        <div className="flex flex-wrap gap-1">
          {MOMENT_SORT_OPTIONS.map(option => (
            <button
              key={option.id}
              type="button"
              onClick={() => setSortBy(option.id)}
              className={`rounded border px-2 py-0.5 text-[10px] font-black uppercase transition ${
                sortBy === option.id
                  ? 'border-violet-400/30 bg-violet-500/10 text-violet-200'
                  : 'border-white/10 bg-white/[0.03] text-zinc-500 hover:text-zinc-300'
              }`}
            >
              {option.label}
            </button>
          ))}
        </div>
      </div>
      <div className="flex max-h-56 flex-col gap-1.5 overflow-y-auto">
        {candidates.map(({ rollup, scoreLabel, reasonLabel, topEmote, estimated }) => {
          const timeStr = new Date(rollup.minuteTs).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
          const active = selectedRollup?.minuteTs === rollup.minuteTs
          return (
            <button
              key={rollup.minuteTs}
              type="button"
              onClick={() => onSelectRollup(rollup)}
              className={`grid grid-cols-[72px_minmax(0,1fr)_auto] items-center gap-2 rounded px-2.5 py-2 text-left text-xs transition ${
                active ? 'border border-amber-500/20 bg-amber-500/10' : 'border border-transparent bg-white/[0.03] hover:bg-white/[0.05]'
              }`}
            >
              <span className="font-bold text-zinc-200">{timeStr}</span>
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
  const [clipStatus, setClipStatus] = useState<'idle' | 'loading' | 'success' | 'error'>('idle')
  const [clipError, setClipError] = useState('')
  const [createdJobId, setCreatedJobId] = useState<string | null>(null)
  const baselines = useMemo(() => computeStreamBaselines(rollups), [rollups])

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
      {clipStatus === 'success' && (
        <div className="mt-3 text-xs font-semibold text-emerald-400 rounded border border-emerald-500/10 bg-emerald-500/5 p-2.5 flex justify-between items-center">
          <span>
            {canExportVod
              ? 'VOD export queued — open Clip Studio to edit while the segment downloads (may take 1–3 min for long VODs).'
              : 'Clip queued — open Clip Studio to edit while the source downloads (~30–90s).'}
          </span>
          {createdJobId ? (
            <Link to={`/studio/${createdJobId}`} className="ml-2 underline text-emerald-300 font-bold hover:text-emerald-200">
              Open in Clip Studio →
            </Link>
          ) : null}
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
  const [selectedHeatmapPeak, setSelectedHeatmapPeak] = useState<ReplayHeatmapPoint | null>(null)
  const [syncing, setSyncing] = useState(false)
  const [syncStatus, setSyncStatus] = useState<SyncStatus | null>(null)
  const [syncError, setSyncError] = useState<string | null>(null)
  const [syncNotice, setSyncNotice] = useState<string | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [lastRefreshedAt, setLastRefreshedAt] = useState<number | null>(null)
  const [activeClipsTab, setActiveClipsTab] = useState<'edits' | 'twitch'>('edits')
  const [rightPanelTab, setRightPanelTab] = useState<'moments' | 'emotes' | 'clips' | 'sync'>('moments')
  const [syncedOnlyFilter, setSyncedOnlyFilter] = useState(false)

  const isLiveRoute = !streamId
  const isHistoricalRoute = Boolean(streamId)

  useEffect(() => {
    setSelectedRollup(null)
    setSelectedHeatmapPeak(null)
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
    // Date slug: wait while loading, never pass the date string as a stream ID
    if (/^\d{4}-\d{2}-\d{2}$/.test(streamId)) {
      if (streamsQuery.isLoading || historyQuery.isLoading) return undefined
      return undefined
    }
    return undefined
  }, [streamId, matchedStream, streamsQuery.isLoading, historyQuery.isLoading])

  useEffect(() => {
    setSyncing(false)
    setSyncError(null)
    setSyncNotice(null)
    setSyncStatus(null)
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

  const detailQuery = useQuery({
    queryKey: ['analytics-detail', login, targetQueryStreamId],
    queryFn: () => targetQueryStreamId ? getAnalyticsStream(targetQueryStreamId, { channel: login }) : getAnalyticsLive(login),
    enabled: Boolean(login && (streamId === '' || targetQueryStreamId)),
    refetchInterval: streamId ? false : 15000,
    retry: false,
    placeholderData: (previousData, previousQuery) => {
      const prevKey = previousQuery?.queryKey?.[2]
      if (streamId === '') {
        return prevKey === '' || prevKey === undefined ? previousData : undefined
      }
      if (targetQueryStreamId && prevKey === targetQueryStreamId) return previousData
      return undefined
    },
    staleTime: 120_000,
    refetchOnWindowFocus: !streamId,
  })

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
      if (!start.accepted && start.status?.phase !== 'completed' && start.status?.phase !== 'failed') {
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
        && (status.phase === 'completed' || status.phase === 'failed' || status.stale),
      )
      const statusActive = Boolean(
        status
        && !statusTerminal
        && status.phase !== 'completed'
        && status.phase !== 'failed'
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
    return detail?.state || (detailQuery.isLoading ? 'loading' : 'not_collected')
  }, [dateSlugUnresolved, isHistoricalRoute, detailQueryMatchesRoute, detail?.state, syncing, detailQuery.isLoading])

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
    if (!rollup) {
      setSelectedHeatmapPeak(null)
      return
    }
    const rollups = detail?.rollups ?? []
    const heatmapPoints = heatmapQuery.data?.points ?? []
    const matched = heatmapPoints.find(point => point.minuteTs === rollup.minuteTs)
    if (matched) {
      setSelectedHeatmapPeak(matched)
      return
    }
    const baselines = computeStreamBaselines(rollups)
    setSelectedHeatmapPeak({
      offsetSeconds: rollupOffsetSeconds(rollup, stream?.startedAt),
      durationSeconds: 60,
      score: computeMomentScore100(rollup, baselines, rollups),
      confidence: 0.55,
      reason: detectPickReason(rollup, baselines, detail?.topEmotes),
      topEmotes: heatmapEmotesFromRollup(rollup, 3, detail?.topEmotes),
      vodId: streamVodId ?? null,
      streamId: stream?.streamId || targetQueryStreamId || streamId || '',
      minuteTs: rollup.minuteTs,
    })
  }, [detail?.rollups, detail?.topEmotes, heatmapQuery.data?.points, stream?.startedAt, stream?.streamId, streamId, streamVodId, targetQueryStreamId])

  const liveHasRichHistory = useMemo(() => {
    if (!isLiveRoute) return false
    const rollups = detail?.rollups ?? []
    const hasLiveRollups = rollups.some(rollupHasMinuteData)
    if (hasLiveRollups) return false
    return combinedStreams.some(s => (s.viewerSamples ?? 0) > 0 || (s.chatMessages ?? 0) > 0)
  }, [isLiveRoute, detail?.rollups, combinedStreams])

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

  const canHeaderSync = !coreMinuteChartsBlocked && Boolean(targetQueryStreamId || needsSync)
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
    if (selected.size > 0) return selected
    return new Set((detail?.topEmotes ?? []).slice(0, 4).map(emote => emote.key))
  }, [selected, detail?.topEmotes])

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
                <span className="font-mono text-[11px] text-zinc-600">{stream.streamId}</span>
              ) : null}
              <span>
                {lastRefreshedAt
                  ? `Refreshed ${relativeTime(lastRefreshedAt)}`
                  : `Updated ${detail?.updatedAt ? relativeTime(detail.updatedAt) : '-'}`}
              </span>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            {canHeaderSync ? (
              <button
                type="button"
                onClick={() => void handleSync(chatOnlySyncAvailable ? { chatOnly: true } : undefined)}
                disabled={syncing || (isHistoricalRoute && !targetQueryStreamId)}
                className="rounded-lg bg-violet-600 px-4 py-2 text-[11px] font-black uppercase tracking-wide text-white transition hover:bg-violet-500 disabled:opacity-50"
              >
                {headerSyncLabel}
              </button>
            ) : null}
            <StackStatusButton />
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

        {setupQuery.data?.services.scraper === 'offline' ? (
          <OptionalServicesPanel variant="banner" focus="scraper" channelLogin={login} />
        ) : null}

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

        {isLiveRoute && detail?.state === 'live' ? (
          <LiveStatsBand
            login={login}
            enabled
            className="rounded border border-white/10 bg-[#0d0d12] p-3"
          />
        ) : null}

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
          <aside className="order-2 min-w-0 lg:order-none xl:sticky xl:top-4 xl:self-start">
            <StreamSidebar
              login={login}
              streams={combinedStreams}
              activeID={isHistoricalRoute ? (targetQueryStreamId || streamId) : undefined}
              isLiveView={isLiveRoute}
              liveState={isLiveRoute ? detail?.state : undefined}
              onPrefetchStream={prefetchStreamDetail}
              onSync={coreMinuteChartsBlocked ? undefined : () => void handleSync()}
              syncing={syncing}
              syncedOnly={syncedOnlyFilter}
              onSyncedOnlyChange={setSyncedOnlyFilter}
              coreMinuteChartsBlocked={coreMinuteChartsBlocked}
              activeRollupStats={sidebarRollupStats}
            />
          </aside>
          <section className="order-1 min-w-0 space-y-4 lg:order-none">
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
              loading={detailQuery.isLoading && !matchedStream && !historicalStream}
              games={gamesQuery.data ?? []}
              canSync={!coreMinuteChartsBlocked && (Boolean(streamId) || needsSync)}
              isLive={isLiveRoute && detail?.state === 'live'}
              coreMinuteChartsBlocked={coreMinuteChartsBlocked}
              liveHasRichHistory={liveHasRichHistory}
              chatOnlySyncAvailable={chatOnlySyncAvailable}
              syncCtaLabel={headerSyncLabel}
            />
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
            {selectedHeatmapPeak ? (
              <MomentDrawer
                selectedPoint={selectedHeatmapPeak}
                canPlay={Boolean(streamVodId)}
                canClip={Boolean(streamVodId) || (isLiveRoute && detail?.state === 'live')}
              />
            ) : null}
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
              isLiveView={isLiveRoute}
              channelLive={detail?.state === 'live'}
            />
          </section>
          <aside className="order-3 space-y-4 lg:order-none">
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
                    {tab === 'moments' ? 'Moments' : tab === 'emotes' ? 'Emotes' : tab === 'clips' ? 'Clips' : 'Sync'}
                  </button>
                ))}
              </div>
              <div className="p-0">
                {rightPanelTab === 'moments' ? (
                  <>
                    {isLiveRoute && detail?.state === 'live' ? (
                      <MostReactedLive
                        login={login}
                        vodId={streamVodId}
                        enabled
                        className="flex flex-col gap-3 border-b border-white/10 p-3"
                      />
                    ) : null}
                    <MomentReviewPanel
                      rollups={detail?.rollups ?? []}
                      selectedRollup={selectedRollup}
                      onSelectRollup={selectRollupWithHeatmap}
                      topEmotesCatalog={detail?.topEmotes}
                      heatmapPoints={heatmapDetailQuery.data?.points ?? heatmapQuery.data?.points}
                      embedded
                    />
                  </>
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
                    {canHeaderSync ? (
                      <div className="flex flex-col gap-2">
                        <button
                          type="button"
                          onClick={() => void handleSync(chatOnlySyncAvailable ? { chatOnly: true } : undefined)}
                          disabled={syncing || (isHistoricalRoute && !targetQueryStreamId)}
                          className="rounded-lg bg-violet-600 px-4 py-2 text-[11px] font-black uppercase text-white transition hover:bg-violet-500 disabled:opacity-50"
                        >
                          {headerSyncLabel}
                        </button>
                        {chatOnlySyncAvailable ? (
                          <button
                            type="button"
                            onClick={() => void handleSync({ chatOnly: true, forceChat: true })}
                            disabled={syncing}
                            className="rounded-lg border border-cyan-400/30 bg-cyan-500/10 px-4 py-2 text-[10px] font-black uppercase text-cyan-100 transition hover:bg-cyan-500/20 disabled:opacity-50"
                          >
                            Re-sync chat only
                          </button>
                        ) : null}
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
                        Start a sync from here or use the header button. Progress appears while VOD chat and emotes index.
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
  isTab = false,
  rollups,
  onSync,
  onOpenMoments,
}: {
  login: string
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
        window.location.href = `/studio/${result.job.id}`
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
              to={`/studio/${job.id}`}
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
