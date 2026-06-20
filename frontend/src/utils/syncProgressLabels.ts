import type { SyncChatProgress, SyncStatus, SyncTrackerProgress } from '../api'

const DEFAULT_TT_EXPECTED_MS = 45_000

export function trackerPhaseLabel(phase: SyncTrackerProgress['phase']): string {
  switch (phase) {
    case 'direct_http':
      return 'Fast HTTP fetch'
    case 'browser':
      return 'Browser scrape (Camoufox)'
    case 'parsing':
      return 'Parsing viewer chart'
    default:
      return 'TwitchTracker'
  }
}

export function viewerTrackerElapsedMs(status: SyncStatus, nowMs = Date.now()): number {
  const tracker = status.tracker
  if (tracker?.elapsedMs != null && tracker.elapsedMs > 0) return tracker.elapsedMs
  if (!status.startedAt) return 0
  return Math.max(0, nowMs - new Date(status.startedAt).getTime())
}

export function viewerTrackerExpectedMs(status: SyncStatus): number {
  const expected = status.tracker?.expectedMs
  if (expected != null && expected > 0) return expected
  return DEFAULT_TT_EXPECTED_MS
}

export function viewerTrackerStepProgress(
  status: SyncStatus,
  nowMs = Date.now(),
): { pct: number; detail: string; phaseLabel: string; elapsedSec: number; expectedSec: number } {
  const tracker = status.tracker
  const expectedMs = viewerTrackerExpectedMs(status)
  const elapsedMs = viewerTrackerElapsedMs(status, nowMs)
  const elapsedSec = Math.floor(elapsedMs / 1000)
  const expectedSec = Math.max(1, Math.round(expectedMs / 1000))
  const phaseLabel = trackerPhaseLabel(tracker?.phase)

  if (status.phase === 'parsing_tracker') {
    const parsePct = Math.min(98, 88 + Math.min(10, Math.floor(elapsedMs / 500)))
    return {
      pct: parsePct,
      detail: tracker?.message || 'Parsing meta#ecs viewer timeline',
      phaseLabel: 'Parsing viewer chart',
      elapsedSec,
      expectedSec,
    }
  }

  if (status.phase === 'scraping_tracker' && (tracker?.active || tracker?.phase)) {
    const ramp = Math.min(88, 8 + (elapsedMs / expectedMs) * 80)
    const detail = tracker?.message || `${phaseLabel} · ${elapsedSec}s / ~${expectedSec}s`
    return { pct: Math.round(ramp), detail, phaseLabel, elapsedSec, expectedSec }
  }

  if (status.viewerStatus === 'backfilling') {
    return {
      pct: Math.min(95, 55 + (elapsedMs / expectedMs) * 35),
      detail: tracker?.message || 'Retrying TwitchTracker in background',
      phaseLabel: 'Background backfill',
      elapsedSec,
      expectedSec,
    }
  }

  return {
    pct: 0,
    detail: tracker?.message || 'Waiting for TwitchTracker viewer minutes',
    phaseLabel,
    elapsedSec,
    expectedSec,
  }
}

export function viewerStepBadge(
  viewerStepState: 'done' | 'active' | 'pending' | 'failed' | 'partial',
  viewerPct: number,
  viewerStatus: SyncStatus['viewerStatus'],
): string {
  if (viewerStepState === 'done') return '100%'
  if (viewerStepState === 'partial') {
    return viewerStatus === 'backfilling' ? 'Backfill' : 'Partial'
  }
  if (viewerStepState === 'failed') return 'Unavailable'
  if (viewerStepState === 'active') return `${viewerPct}%`
  return 'Pending'
}

export function chatFetchCleanupLabel(
  chat: SyncChatProgress | undefined,
  segmentDone: number,
  segmentTotal: number,
  options: { timelinePct?: number; segmentCleanup?: boolean } = {},
): string {
  if (!chat) return ''
  const timelinePct = options.timelinePct ?? 0
  const cleanupPhase = chat.cleanupPhase
  const remaining = chat.segmentsIncomplete ?? Math.max(0, segmentTotal - segmentDone)
  if (timelinePct >= 100 && (options.segmentCleanup || cleanupPhase)) {
    return `${remaining.toLocaleString()} segment${remaining === 1 ? '' : 's'} remaining`
  }
  if (cleanupPhase === 'serial_retry') {
    return `Serial retry: ${segmentDone.toLocaleString()}/${segmentTotal.toLocaleString()} segments closed`
  }
  if (cleanupPhase === 'parallel_cleanup') {
    return `Cleanup: ${remaining.toLocaleString()} segments remaining`
  }
  if (chat.segmentsIncomplete && chat.segmentsIncomplete > 0 && segmentTotal > 1) {
    return `Cleanup: ${chat.segmentsIncomplete.toLocaleString()} segments remaining`
  }
  return ''
}

export function chatFetchDetailLabel(
  status: SyncStatus,
  chatFetchDone: boolean,
  segmentsTrackable: boolean,
  segmentCleanup: boolean,
  timelinePct: number,
  segmentDone: number,
  segmentTotal: number,
): string {
  if (status.chat?.indexPhase === 'finalizing') return 'Finalizing chat index'
  if (timelinePct >= 100 && status.chat?.indexPhase === 'writing') return 'Writing rollups & emotes'
  if (timelinePct >= 100 && (segmentCleanup || status.chat?.cleanupPhase)) return 'Finalizing chat index'
  if (chatFetchDone) return 'All comments fetched'
  if (status.chat?.throttled) return 'Waiting on rate limit'
  const cleanupLabel = chatFetchCleanupLabel(status.chat, segmentDone, segmentTotal, { timelinePct, segmentCleanup })
  if (cleanupLabel) return cleanupLabel
  if (segmentsTrackable) {
    if (segmentCleanup) return 'Final segment cleanup'
    return `Segments closed: ${segmentDone.toLocaleString()}/${segmentTotal.toLocaleString()} · scan ${timelinePct}%`
  }
  return `${(status.chat?.commentsFetched ?? 0).toLocaleString()} comments indexed`
}

export function viewerStatusShowsExistingChart(
  viewerStatus: string | undefined,
  viewerDataFromExisting: boolean,
): boolean {
  if (viewerStatus === 'ok' || viewerStatus === 'skipped') return true
  if ((viewerStatus === 'pending_backfill' || viewerStatus === 'backfilling') && viewerDataFromExisting) {
    return true
  }
  return false
}
