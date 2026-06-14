import type { AnalyticsStream } from '../api'

export type StreamAnalyticsStatus = 'current-live' | 'synced' | 'stats-only' | 'sync-interrupted' | 'unknown'

export function resolveStreamAnalyticsStatus(
  streamId: string,
  streams: AnalyticsStream[] | undefined,
  liveStreamId?: string | null,
): StreamAnalyticsStatus {
  const stream = streams?.find(item => item.streamId === streamId)
  if (!stream) return 'unknown'
  if (liveStreamId && stream.streamId === liveStreamId) return 'current-live'
  if ((stream.viewerSamples ?? 0) > 0 || (stream.chatMessages ?? 0) > 0) return 'synced'
  return 'stats-only'
}

export function streamAnalyticsStatusLabel(status: StreamAnalyticsStatus): string {
  switch (status) {
    case 'current-live':
      return 'Current live'
    case 'synced':
      return 'Synced'
    case 'stats-only':
      return 'Stats only'
    case 'sync-interrupted':
      return 'Sync interrupted'
    default:
      return 'Not tracked'
  }
}

export function streamAnalyticsStatusClass(status: StreamAnalyticsStatus): string {
  switch (status) {
    case 'current-live':
      return 'bg-red-500/10 text-red-200 border-red-400/20'
    case 'synced':
      return 'bg-emerald-500/10 text-emerald-300 border-emerald-400/20'
    case 'stats-only':
      return 'bg-amber-500/10 text-amber-200 border-amber-400/20'
    case 'sync-interrupted':
      return 'bg-orange-500/10 text-orange-200 border-orange-400/20'
    default:
      return 'bg-zinc-500/10 text-zinc-400 border-zinc-400/20'
  }
}
