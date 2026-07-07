/** Twitch VOD watch URL with optional seek offset (seconds from stream start). */
export function formatTwitchVodTimeParam(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  if (h > 0) return `${h}h${m}m${sec}s`
  if (m > 0) return `${m}m${sec}s`
  return `${sec}s`
}

export function buildTwitchVodUrl(vodId: string, offsetSeconds = 0): string {
  const id = vodId.trim()
  if (!id) return 'https://www.twitch.tv'
  const base = `https://www.twitch.tv/videos/${encodeURIComponent(id)}`
  if (offsetSeconds <= 0) return base
  return `${base}?t=${formatTwitchVodTimeParam(offsetSeconds)}`
}

export function resolveAnalyticsVodId(
  detail?: {
    vodId?: string
    stream?: { vodId?: string }
  },
  recapVodId?: string,
): string | undefined {
  const id = detail?.vodId?.trim() || detail?.stream?.vodId?.trim() || recapVodId?.trim()
  return id || undefined
}

export type VodLinkStatus = 'linked' | 'live' | 'syncing' | 'unavailable'

export interface VodLinkState {
  status: VodLinkStatus
  vodId?: string
  /** Short label for the action chip in Selected Moment. */
  label: string
  /** Longer explanation when no link is available. */
  detail: string
}

export function resolveVodLinkState(input: {
  detail?: {
    vodId?: string
    state?: string
    syncPhase?: string
    stream?: { vodId?: string; endedAt?: string | null }
  }
  recapVodId?: string
  isLiveCollector?: boolean
}): VodLinkState {
  const vodId = resolveAnalyticsVodId(input.detail, input.recapVodId)
  if (vodId) {
    return {
      status: 'linked',
      vodId,
      label: 'Jump to VOD',
      detail: '',
    }
  }

  const syncing =
    input.detail?.state === 'syncing'
    || Boolean(input.detail?.syncPhase?.trim())
  if (syncing) {
    return {
      status: 'syncing',
      label: 'VOD syncing…',
      detail: 'The VOD archive is still syncing. The Twitch link will appear once the VOD ID resolves.',
    }
  }

  const endedAt = input.detail?.stream?.endedAt?.trim()
  const isLive = input.isLiveCollector ?? (input.detail?.state === 'live' || !endedAt)
  if (isLive) {
    return {
      status: 'live',
      label: 'Live — no VOD yet',
      detail: 'This session is still live. A timestamped VOD link appears automatically after the broadcast ends.',
    }
  }

  return {
    status: 'unavailable',
    label: 'VOD unavailable',
    detail: 'No Twitch VOD exists for this session — it may have been deleted, expired, or was never archived.',
  }
}
