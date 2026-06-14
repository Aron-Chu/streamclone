import type { AnalyticsStream } from '../api.ts'

export function isPlaceholderStreamTitle(title?: string) {
  const trimmed = title?.trim() ?? ''
  return trimmed === '' || trimmed === 'Syncing...' || trimmed === 'Syncing…'
}

/** Sync prefetch row in analytics DB — not the IRC/Helix live collector stream. */
export function isSyncPrefetchPlaceholder(stream?: AnalyticsStream) {
  if (stream?.endedAt) return false
  if ((stream?.viewerSamples ?? 0) > 0 || (stream?.chatMessages ?? 0) > 0) return false
  // Title may be replaced by TwitchTracker merge in the sidebar; pending id is authoritative.
  if (stream?.broadcasterId === 'pending') return true
  return isPlaceholderStreamTitle(stream?.title)
}

export function isActiveLiveCollectorStream(stream?: AnalyticsStream, state?: string) {
  return state === 'live' && !isSyncPrefetchPlaceholder(stream)
}
