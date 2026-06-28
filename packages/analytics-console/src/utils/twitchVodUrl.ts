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

export function resolveAnalyticsVodId(detail?: {
  vodId?: string
  stream?: { vodId?: string }
}): string | undefined {
  const id = detail?.vodId?.trim() || detail?.stream?.vodId?.trim()
  return id || undefined
}
