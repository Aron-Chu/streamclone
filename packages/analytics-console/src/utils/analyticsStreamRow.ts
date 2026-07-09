import type { AnalyticsStream } from '../api.ts'
import type { AnalyticsMinuteRollup, AnalyticsStreamDetail } from '../apiTypes.ts'

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

const staleOpenStreamAgeMs = 48 * 60 * 60 * 1000

function rollupHasActivity(rollup: AnalyticsMinuteRollup): boolean {
  return (rollup.chatCount ?? 0) > 0
    || (rollup.totalEmoteCount ?? 0) > 0
    || (rollup.seventvEmoteCount ?? 0) > 0
    || (rollup.viewerAvg ?? 0) > 0
    || (rollup.viewerMax ?? 0) > 0
    || (rollup.viewerLatest ?? 0) > 0
    || (rollup.viewerSamples ?? 0) > 0
}

/** True when the channel detail reflects an actively tracked live collector session. */
export function resolveChannelActuallyLive(detail?: AnalyticsStreamDetail | null): boolean {
  if (!detail || detail.state !== 'live') return false
  if ((detail.stream?.currentViewers ?? 0) > 0) return true
  if ((detail.rollups ?? []).some(rollupHasActivity)) return true
  const startedAt = detail.stream?.startedAt
  if (startedAt) {
    const startedMs = Date.parse(startedAt)
    if (Number.isFinite(startedMs) && Date.now() - startedMs > staleOpenStreamAgeMs) {
      return false
    }
  }
  return isActiveLiveCollectorStream(detail.stream, detail.state)
}
