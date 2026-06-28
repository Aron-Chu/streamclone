import { MIN_LIVE_ROLLUPS_FOR_CHART } from './liveEmptyState.ts'

export const LIVE_TRACKING_BADGE = 'Live tracking'
export const LIVE_TRACKING_NOT_SYNC_NOTE =
  'Automatic while the stream is live — not the same as Sync historical data (VOD backfill).'

export interface LiveChartWarmupProgress {
  readyMinutes: number
  targetMinutes: number
  percent: number
  chartReady: boolean
}

export function liveChartWarmupProgress(rollupCount: number): LiveChartWarmupProgress {
  const ready = Math.max(0, Math.floor(rollupCount ?? 0))
  const target = MIN_LIVE_ROLLUPS_FOR_CHART
  const clamped = Math.min(ready, target)
  return {
    readyMinutes: clamped,
    targetMinutes: target,
    percent: Math.min(100, Math.round((clamped / target) * 100)),
    chartReady: ready >= target,
  }
}

export function liveWarmupStatusLine(progress: LiveChartWarmupProgress): string {
  if (progress.readyMinutes === 0) {
    return 'Waiting for the first completed minute bucket (viewers + chat + emotes).'
  }
  if (progress.readyMinutes < progress.targetMinutes) {
    return `${progress.readyMinutes} of ${progress.targetMinutes} minute buckets ready — chart opens on the next boundary.`
  }
  return 'Minute buckets ready — chart should appear on the next refresh.'
}

export function liveWarmupHintLine(opts?: {
  viewerSamples?: number
  chatMessages?: number
}): string | null {
  const viewers = opts?.viewerSamples ?? 0
  const chat = opts?.chatMessages ?? 0
  if (viewers <= 0 && chat <= 0) return null
  const parts: string[] = []
  if (viewers > 0) parts.push('viewer samples arriving')
  if (chat > 0) parts.push('chat being counted')
  return `Heads-up: ${parts.join(' and ')} — rolling up into chart minutes now.`
}
