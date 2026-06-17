/** Grafana Emote Pulse dashboard (local stack default). */
export const PULSE_DASHBOARD_URL =
  'http://localhost:3000/d/streamclone-emote-pulse/emote-pulse?from=now-7d&to=now'

/** Grafana Streamclone Ops dashboard (Prometheus / service metrics). */
export const PULSE_OPS_DASHBOARD_URL =
  'http://localhost:3000/d/streamclone-ops/streamclone-ops?from=now-15m&to=now'

export const PULSE_GRAFANA_HOME_URL = 'http://localhost:3000/'

export const PULSE_DASHBOARD_LINKS = [
  { href: PULSE_DASHBOARD_URL, label: 'Emote Pulse' },
  { href: PULSE_OPS_DASHBOARD_URL, label: 'Ops' },
] as const

export interface PulseDashboardStream {
  streamId?: string
  startedAt?: string
  endedAt?: string
}

/** Deep link to Grafana with channel/stream vars pre-selected. */
export function pulseDashboardUrl(
  channel: string,
  stream?: PulseDashboardStream,
  fallbackStreamId?: string,
): string {
  const url = new URL(PULSE_DASHBOARD_URL)
  if (channel) url.searchParams.set('var-channel', channel)
  const streamId = stream?.streamId || fallbackStreamId
  if (streamId) url.searchParams.set('var-stream', streamId)
  if (stream?.startedAt) {
    const startMs = Date.parse(stream.startedAt)
    const endMs = stream.endedAt ? Date.parse(stream.endedAt) : Date.now()
    if (Number.isFinite(startMs) && Number.isFinite(endMs) && endMs > startMs) {
      url.searchParams.set('from', String(startMs))
      url.searchParams.set('to', String(endMs))
      url.searchParams.set('var-stream_start', String(startMs))
      url.searchParams.set('var-stream_end', String(endMs))
    }
  }
  return url.toString()
}
