import { normalizeVodId } from './vodId.ts'

export function buildMomentJumpLink(
  login: string,
  offsetSeconds: number,
  options?: { vodId?: string },
): string {
  const vodId = options?.vodId?.trim()
  if (vodId) {
    return buildVodDeepLink(login, vodId, offsetSeconds)
  }
  return `/c/${encodeURIComponent(login)}`
}

export function buildVodDeepLink(
  login: string,
  vodId: string,
  offsetSeconds: number,
): string {
  const safeOffset = Number.isFinite(offsetSeconds)
    ? Math.max(0, Math.trunc(offsetSeconds))
    : 0
  const params = new URLSearchParams({
    vod: vodId,
    offset: String(safeOffset),
  })
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
    return { fromAnalytics: false, streamId: '' }
  }
  const fromAnalytics = fromMarker || streamId.length > 0
  return { fromAnalytics, streamId }
}

export { normalizeVodId }
