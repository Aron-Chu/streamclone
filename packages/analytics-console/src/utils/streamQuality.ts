import type { AnalyticsMinuteRollup, AnalyticsStreamDetail } from '../apiTypes.ts'
import { analyzeViewerCoverage, viewerValue, vodClock } from '../components/analytics/chartRollupUtils.ts'

export type StreamQualityIssue =
  | 'stats_only'
  | 'viewer_resync'
  | 'live_viewer_warmup'
  | 'partial_chat'
  | 'syncing'
  | 'refresh_only_hint'

export interface StreamQualityDiagnosis {
  issues: StreamQualityIssue[]
  message: string
  suggestedAction: 'sync_full' | 'sync_viewers' | 'sync_chat' | 'wait_sync' | 'none'
  actionLabel?: string
}

export interface StreamSummaryMetrics {
  sync_health_state?: string
  data_coverage_pct?: number
  minutesWithData?: number
  viewerSampleCount?: number
}

export type AnalyticsQualityLabel = 'Good' | 'Partial' | 'Limited' | 'No data'

/** Derive coarse analytics quality label for portal honesty (design §13A.4). */
export function deriveAnalyticsQualityLabel(input: {
  analyticsQuality?: string
  summaryMetrics?: StreamSummaryMetrics
  rollupCount?: number
  chatMessages?: number
  vodId?: string
}): AnalyticsQualityLabel {
  const quality = (input.analyticsQuality ?? '').toLowerCase()
  const coverage = input.summaryMetrics?.data_coverage_pct
  const syncHealth = (input.summaryMetrics?.sync_health_state ?? '').toLowerCase()
  const rollups = input.rollupCount ?? 0
  const hasChat = (input.chatMessages ?? 0) > 0

  if (rollups === 0 && !hasChat && (coverage == null || coverage <= 0)) {
    return 'No data'
  }
  if (quality === 'limited' || quality === 'warming' || syncHealth === 'stats_only') {
    return 'Limited'
  }
  if (quality === 'full_pulse' || (coverage != null && coverage >= 80)) {
    return 'Good'
  }
  if (quality === 'partial_pulse' || (coverage != null && coverage >= 40) || rollups > 0) {
    return 'Partial'
  }
  return 'No data'
}

export function analyticsQualityChipClass(label: AnalyticsQualityLabel): string {
  switch (label) {
    case 'Good':
      return 'border-emerald-400/25 bg-emerald-500/10 text-emerald-200'
    case 'Partial':
      return 'border-amber-400/25 bg-amber-500/10 text-amber-200'
    case 'Limited':
      return 'border-orange-400/25 bg-orange-500/10 text-orange-200'
    default:
      return 'border-white/10 bg-white/[0.04] text-zinc-400'
  }
}

function rollupsHaveChat(rollups: AnalyticsMinuteRollup[]): boolean {
  return rollups.some(
    (row) => !row.missing && ((row.chatCount ?? 0) > 0 || (row.totalEmoteCount ?? 0) > 0),
  )
}

function needsViewerResync(rollups: AnalyticsMinuteRollup[], isLive: boolean): boolean {
  if (isLive || !rollupsHaveChat(rollups)) return false
  const coverage = analyzeViewerCoverage(rollups)
  return (
    !coverage.hasViewerRollups
    || coverage.hasFlatViewerLine
    || coverage.hasPartialTail
    || coverage.hasShortSpan
  )
}

/** Live-only: chat/emotes in early timeline but Helix viewer samples start later. */
export function diagnoseLiveViewerWarmup(
  rollups: AnalyticsMinuteRollup[],
  isLive: boolean,
  streamStartedAt?: string,
): { message: string } | null {
  if (!isLive || rollups.length < 2) return null

  let firstChatIdx = -1
  let firstViewerIdx = -1
  for (let i = 0; i < rollups.length; i++) {
    const row = rollups[i]
    if (row.missing) continue
    if (
      firstChatIdx < 0
      && ((row.chatCount ?? 0) > 0 || (row.totalEmoteCount ?? 0) > 0)
    ) {
      firstChatIdx = i
    }
    if (
      firstViewerIdx < 0
      && ((row.viewerSamples ?? 0) > 0 || viewerValue(row) > 0)
    ) {
      firstViewerIdx = i
    }
    if (firstChatIdx >= 0 && firstViewerIdx >= 0) break
  }
  if (firstChatIdx < 0 || firstViewerIdx < 0 || firstViewerIdx <= firstChatIdx) {
    return null
  }

  const midPoint = Math.max(1, Math.floor(rollups.length / 2))
  if (firstChatIdx >= midPoint) return null

  const viewerRow = rollups[firstViewerIdx]
  const offsetLabel = vodClock(viewerRow.minuteTs, streamStartedAt)
  return {
    message: `Viewer samples started at ${offsetLabel}; chat may begin earlier.`,
  }
}

export function diagnoseStreamQuality(input: {
  detail?: AnalyticsStreamDetail
  summaryMetrics?: StreamSummaryMetrics
  analyticsQuality?: string
  isLive?: boolean
  syncing?: boolean
}): StreamQualityDiagnosis | null {
  const { detail, summaryMetrics, analyticsQuality, isLive = false, syncing = false } = input
  if (!detail && !summaryMetrics) return null

  const rollups = detail?.rollups ?? []
  const syncHealth = summaryMetrics?.sync_health_state ?? ''
  const issues: StreamQualityIssue[] = []

  if (syncing || detail?.state === 'syncing' || analyticsQuality === 'syncing') {
    issues.push('syncing')
    return {
      issues,
      message: detail?.syncPhase
        ? `Sync in progress (${detail.syncPhase.replace(/_/g, ' ')})`
        : 'Sync in progress — chart data may update as rollups are written.',
      suggestedAction: 'wait_sync',
      actionLabel: undefined,
    }
  }

  const statsOnly =
    syncHealth === 'stats_only'
    || analyticsQuality === 'limited'
    || analyticsQuality === 'warming'
    || (
      !rollups.some((r) => !r.missing && ((r.viewerSamples ?? 0) > 0 || (r.chatCount ?? 0) > 0))
      && (detail?.stream?.avgViewers ?? 0) > 0
    )

  if (statsOnly && !rollupsHaveChat(rollups)) {
    issues.push('stats_only')
    return {
      issues,
      message:
        'Session metadata only (duration, averages). Minute-level viewers, chat, and emotes are not synced yet.',
      suggestedAction: 'sync_full',
      actionLabel: 'Sync chat & emotes',
    }
  }

  if (needsViewerResync(rollups, isLive)) {
    issues.push('viewer_resync')
    return {
      issues,
      message: 'Viewer chart looks incomplete or flat. Chat data is present but viewer minutes may need a refresh.',
      suggestedAction: 'sync_viewers',
      actionLabel: 'Re-sync viewers',
    }
  }

  const liveViewerWarmup = diagnoseLiveViewerWarmup(rollups, isLive, detail?.stream?.startedAt)
  if (liveViewerWarmup) {
    issues.push('live_viewer_warmup')
    return {
      issues,
      message: liveViewerWarmup.message,
      suggestedAction: 'none',
    }
  }

  if (detail?.chatCoverage?.partial || (detail?.chatCoveragePct != null && detail.chatCoveragePct < 35)) {
    issues.push('partial_chat')
    return {
      issues,
      message: 'Chat coverage is partial for this stream. A full sync can fill missing segments.',
      suggestedAction: 'sync_chat',
      actionLabel: 'Sync chat & emotes',
    }
  }

  if (issues.length === 0 && canSyncActionsHelp(detail, summaryMetrics)) {
    return {
      issues: ['refresh_only_hint'],
      message: 'Refresh data reloads charts from the server. Use sync actions below to pull missing minute rollups.',
      suggestedAction: 'none',
    }
  }

  return null
}

function canSyncActionsHelp(
  detail?: AnalyticsStreamDetail,
  summaryMetrics?: StreamSummaryMetrics,
): boolean {
  const state = summaryMetrics?.sync_health_state ?? ''
  return state === 'partial' || state === 'viewer_only' || state === 'chat_only' || detail?.state === 'historical'
}
