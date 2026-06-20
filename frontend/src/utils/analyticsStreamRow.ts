import type { AnalyticsStream } from '../api.ts'

export function isPlaceholderStreamTitle(title?: string) {
  const trimmed = title?.trim() ?? ''
  return trimmed === '' || trimmed === 'Syncing...' || trimmed === 'Syncing…'
}

/** Legacy prefetch stub — API now dedupes server-side; keep for older rows during rollout. */
export function isSyncPrefetchPlaceholder(stream?: AnalyticsStream) {
  if (!stream) return false
  if (stream.canonicalStreamId && stream.canonicalStreamId !== stream.streamId) {
    return true
  }
  if (stream.endedAt) return false
  if ((stream.viewerSamples ?? 0) > 0 || (stream.chatMessages ?? 0) > 0) return false
  if (stream.broadcasterId === 'pending') return true
  return isPlaceholderStreamTitle(stream?.title)
}

export function isActiveLiveCollectorStream(stream?: AnalyticsStream, state?: string) {
  return state === 'live' && !isSyncPrefetchPlaceholder(stream)
}
