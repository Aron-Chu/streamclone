import type { AnalyticsMinuteRollup, AnalyticsStreamDetail } from '../apiTypes.ts'
import { analyzeViewerCoverage } from '../components/analytics/chartRollupUtils.ts'

export type StreamQualityIssue =
  | 'stats_only'
  | 'viewer_resync'
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
