import type { AnalyticsStream } from '../api.ts'

export type StreamSyncBadge = 'synced' | 'partial' | 'stats_only' | 'syncing'

/** Stream list row has both viewer and chat minute rollups synced into analytics DB. */
export function streamHasSyncedMinutes(stream: AnalyticsStream): boolean {
  return (stream.viewerSamples ?? 0) > 0 && (stream.chatMessages ?? 0) > 0
}

export function streamSyncBadgeState(stream: AnalyticsStream, syncing = false): StreamSyncBadge {
  if (syncing) return 'syncing'
  const hasViewers = (stream.viewerSamples ?? 0) > 0
  const hasChat = (stream.chatMessages ?? 0) > 0
  if (hasViewers && hasChat) return 'synced'
  if (hasViewers || hasChat) return 'partial'
  return 'stats_only'
}

export function streamSyncBadgeLabel(badge: StreamSyncBadge): string {
  switch (badge) {
    case 'syncing':
      return 'Syncing'
    case 'synced':
      return 'Synced'
    case 'partial':
      return 'Partial'
    default:
      return 'Stats only'
  }
}

export function streamSyncBadgeTitle(badge: StreamSyncBadge, stream: AnalyticsStream): string {
  switch (badge) {
    case 'syncing':
      return 'Sync in progress — partial chart data may already be visible.'
    case 'synced':
      return 'Minute-level viewer, chat, and emote rollups are synced for charts.'
    case 'partial': {
      const hasViewers = (stream.viewerSamples ?? 0) > 0
      const hasChat = (stream.chatMessages ?? 0) > 0
      if (hasViewers && !hasChat) return 'Viewer minutes synced; chat rollups missing or partial.'
      if (!hasViewers && hasChat) return 'Chat rollups synced; viewer minutes missing or partial.'
      return 'Partial minute coverage — chart may be incomplete.'
    }
    default:
      return 'Session stats only (duration, title). Open the stream detail page to see minute charts.'
  }
}

/** Sidebar row is navigable when it has minute data or TwitchTracker session stats. */
export function streamIsSidebarVisible(stream: AnalyticsStream, syncedOnly: boolean): boolean {
  if (!syncedOnly) return true
  if (streamHasSyncedMinutes(stream)) return true
  if (stream.endedAt?.trim()) return true
  return (stream.peakViewers ?? 0) > 0 || (stream.avgViewers ?? 0) > 0
}

export function getAnalyticsStreamDateSlug(startedAt?: string): string {
  if (!startedAt) return ''
  const date = new Date(startedAt)
  if (Number.isNaN(date.getTime())) return ''
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

/** Route slug for /analytics/{login}/{slug} — date when unique that day, else stream id. */
export function analyticsStreamPathSlug(
  stream: AnalyticsStream,
  allStreams: AnalyticsStream[],
): string {
  const dateSlug = getAnalyticsStreamDateSlug(stream.startedAt)
  if (!dateSlug) return stream.streamId
  const sameDay = allStreams.filter(s => getAnalyticsStreamDateSlug(s.startedAt) === dateSlug).length
  return sameDay === 1 ? dateSlug : stream.streamId
}

/**
 * When the live collector route has no chart minutes but a synced session exists,
 * pick the sidebar stream to open instead of the empty live placeholder row.
 * With server-side dedupe, this mainly handles legacy rows and live-endpoint mismatches.
 */
export function pickSyncedLiveStreamTarget(
  combinedStreams: AnalyticsStream[],
  opts?: { liveStreamId?: string; channelLive?: boolean },
): AnalyticsStream | undefined {
  const visible = combinedStreams.filter(
    stream => !stream.canonicalStreamId || stream.canonicalStreamId === stream.streamId,
  )
  const synced = visible.filter(streamHasSyncedMinutes)
  if (synced.length === 0) return undefined

  const liveStreamId = opts?.liveStreamId?.trim()
  const channelLive = opts?.channelLive ?? false

  if (liveStreamId) {
    const same = synced.find(stream => stream.streamId === liveStreamId)
    if (same) return same
  }

  const openSynced = synced.filter(stream => !stream.endedAt)
  if (openSynced.length > 0) return openSynced[0]

  if (channelLive) {
    const liveRow = liveStreamId
      ? visible.find(stream => stream.streamId === liveStreamId)
      : visible[0]
    const collectorIsStale = !liveRow || !streamHasSyncedMinutes(liveRow)
    const newestSynced = synced[0]
    const referenceMs = streamReferenceTimeMs(liveRow) ?? newestStreamReferenceTimeMs(visible) ?? Date.now()
    if (collectorIsStale && newestSynced && isRecentSyncedSession(newestSynced, referenceMs)) {
      return newestSynced
    }
    return undefined
  }

  return synced[0]
}

/** Canonical session row for `/analytics/:login` → `/analytics/:login/:slug` redirects. */
export function resolveCanonicalLiveSessionTarget(
  combinedStreams: AnalyticsStream[],
  opts?: {
    liveStreamId?: string
    channelLive?: boolean
    channelLogin?: string
    startedAt?: string
  },
): AnalyticsStream | undefined {
  const liveStreamId = opts?.liveStreamId?.trim()
  const channelLive = opts?.channelLive ?? false

  if (liveStreamId) {
    const visible = combinedStreams.find(
      (stream) =>
        stream.streamId === liveStreamId
        && (!stream.canonicalStreamId || stream.canonicalStreamId === stream.streamId),
    )
    if (visible) return visible
    if (channelLive) {
      return {
        streamId: liveStreamId,
        login: opts?.channelLogin ?? '',
        startedAt: opts?.startedAt ?? '',
      }
    }
  }

  return pickSyncedLiveStreamTarget(combinedStreams, {
    liveStreamId,
    channelLive,
  })
}

/** Long streams stay "live" for many hours; allow redirect for recent synced sessions. */
function isRecentSyncedSession(stream: AnalyticsStream, referenceMs = Date.now()): boolean {
  if (!stream.startedAt) return false
  const started = Date.parse(stream.startedAt)
  if (Number.isNaN(started)) return false
  return referenceMs-started < 48 * 60 * 60 * 1000
}

function streamReferenceTimeMs(stream?: AnalyticsStream): number | undefined {
  for (const value of [stream?.endedAt, stream?.lastSeenAt, stream?.startedAt]) {
    if (!value) continue
    const parsed = Date.parse(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return undefined
}

function newestStreamReferenceTimeMs(streams: AnalyticsStream[]): number | undefined {
  let newest: number | undefined
  for (const stream of streams) {
    const parsed = streamReferenceTimeMs(stream)
    if (parsed === undefined) continue
    if (newest === undefined || parsed > newest) newest = parsed
  }
  return newest
}
