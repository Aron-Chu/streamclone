import { normalizeVodId } from './vodId.ts'

export const STREAMPULSE_ANALYTICS_ORIGIN = 'https://streampulse.stream/analytics'

export function buildHostedAnalyticsMomentLink(
  login: string,
  offsetSeconds: number,
  analyticsStreamId?: string,
): string {
  const safeOffset = Number.isFinite(offsetSeconds)
    ? Math.max(0, Math.trunc(offsetSeconds))
    : 0
  const params = new URLSearchParams()
  if (safeOffset > 0) params.set('offset', String(safeOffset))
  const suffix = params.toString()
  const query = suffix ? `?${suffix}` : ''
  const streamId = analyticsStreamId?.trim()
  const base = `${STREAMPULSE_ANALYTICS_ORIGIN}/${encodeURIComponent(login)}`
  if (streamId) {
    return `${base}/${encodeURIComponent(streamId)}${query}`
  }
  return `${base}${query}`
}

/** @deprecated Local /analytics routes removed — use buildHostedAnalyticsMomentLink. */
export function buildAnalyticsMomentLink(
  login: string,
  offsetSeconds: number,
  analyticsStreamId?: string,
): string {
  return buildHostedAnalyticsMomentLink(login, offsetSeconds, analyticsStreamId)
}

export function buildMomentJumpLink(
  login: string,
  offsetSeconds: number,
  options?: { vodId?: string; analyticsStreamId?: string },
): string {
  const vodId = options?.vodId?.trim()
  if (vodId) {
    return buildVodDeepLink(login, vodId, offsetSeconds, options?.analyticsStreamId)
  }
  return buildHostedAnalyticsMomentLink(login, offsetSeconds, options?.analyticsStreamId)
}

export function buildVodDeepLink(
  login: string,
  vodId: string,
  offsetSeconds: number,
  analyticsStreamId?: string,
): string {
  const safeOffset = Number.isFinite(offsetSeconds)
    ? Math.max(0, Math.trunc(offsetSeconds))
    : 0
  const params = new URLSearchParams({
    vod: vodId,
    offset: String(safeOffset),
  })
  const streamId = analyticsStreamId?.trim()
  if (streamId) {
    params.set('from', 'analytics')
    params.set('sid', streamId)
  }
  return `/c/${encodeURIComponent(login)}?${params.toString()}`
}

export function buildVodSeekTarget(
  offsetSeconds: number,
  seekSeconds: number,
): number {
  const offset = Number.isFinite(offsetSeconds) ? offsetSeconds : 0
  const seek = Number.isFinite(seekSeconds) ? seekSeconds : 0
  return Math.max(0, offset - seek)
}

export const VOD_RELAY_SEEK_PAD_SECONDS = 30

export function estimateVodRelaySeekSeconds(offsetSeconds: number): number {
  const offset = Number.isFinite(offsetSeconds) ? Math.max(0, Math.trunc(offsetSeconds)) : 0
  return Math.max(0, offset - VOD_RELAY_SEEK_PAD_SECONDS)
}

export function estimateVodPlayerSeekTarget(offsetSeconds: number): number {
  return buildVodSeekTarget(offsetSeconds, estimateVodRelaySeekSeconds(offsetSeconds))
}

export interface VodStartRequestBody {
  vod_id: string
  offset_seconds: number
  quality?: string
  latency_mode?: string
}

export function buildVodStartRequestBody(
  vodId: string,
  offsetSeconds = 0,
  quality?: string,
  latencyMode?: string,
): VodStartRequestBody {
  return {
    vod_id: vodId,
    offset_seconds: Number.isFinite(offsetSeconds)
      ? Math.max(0, Math.trunc(offsetSeconds))
      : 0,
    quality,
    latency_mode: latencyMode,
  }
}

export interface VodAnalyticsContext {
  fromAnalytics: boolean
  streamId: string
  analyticsHref: string | null
}

export function preferTwitchEmbedReview(
  isVodPlayback: boolean,
  fromAnalytics: boolean,
  streamId: string,
): boolean {
  return isVodPlayback && fromAnalytics && streamId.trim().length > 0
}

export function parseVodAnalyticsContext(
  searchParams: { get(name: string): string | null },
  channelLogin: string,
  isVodPlayback: boolean,
): VodAnalyticsContext {
  const streamId = (searchParams.get('sid') ?? '').trim()
  const fromMarker = (searchParams.get('from') ?? '').trim().toLowerCase() === 'analytics'
  if (!isVodPlayback || !channelLogin) {
    return { fromAnalytics: false, streamId: '', analyticsHref: null }
  }
  const fromAnalytics = fromMarker || streamId.length > 0
  const offsetSeconds = Math.max(0, Number.parseInt(searchParams.get('offset') || '0', 10) || 0)
  const analyticsHref = fromAnalytics
    ? buildHostedAnalyticsMomentLink(channelLogin, offsetSeconds, streamId || undefined)
    : null
  return { fromAnalytics, streamId, analyticsHref }
}

export { normalizeVodId }
