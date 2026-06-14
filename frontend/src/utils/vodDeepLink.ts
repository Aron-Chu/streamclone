import { normalizeVodId } from './vodId.ts'

// Pure, dependency-light helpers for the analytics -> VOD-mode deep link flow.
//
// These encode the trust-critical contract from Requirement 25 (VOD Playback
// Smoke Test) as React/DOM-free functions so they can be tested directly by the
// node test runner (which cannot render .tsx or load Vite `import.meta.env`
// modules such as config.ts / api.ts). The same helpers are wired into the
// production surfaces (Analytics Watch on Twitch links, Channel VOD start
// request + seek), so the smoke test guards the real code paths.

/**
 * Builds the canonical VOD deep link `/c/{login}?vod={vodId}&offset={offset}`.
 *
 * Used by the channel VODs-tab "Play VOD" link. `login` and `vodId` are URL-encoded;
 * `offsetSeconds` is coerced to a whole number >= 0 (Requirement 1.3, 25.2).
 */
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

/**
 * Computes the player seek target after a VOD relay starts.
 *
 * Lands the viewer at the requested moment after accounting for relay preroll:
 * `Math.max(0, offset_seconds - seek_seconds)` (Requirement 1.5, 25.3).
 */
export function buildVodSeekTarget(
  offsetSeconds: number,
  seekSeconds: number,
): number {
  const offset = Number.isFinite(offsetSeconds) ? offsetSeconds : 0
  const seek = Number.isFinite(seekSeconds) ? seekSeconds : 0
  return Math.max(0, offset - seek)
}

/** Request body sent to `POST /v1/stream/vod/start`. */
export interface VodStartRequestBody {
  vod_id: string
  offset_seconds: number
  quality?: string
  latency_mode?: string
}

/**
 * Shapes the `POST /v1/stream/vod/start` request body.
 *
 * The relay contract requires the snake_case JSON field `vod_id` (not `vodId`
 * or query-only passthrough) and a whole-number `offset_seconds` >= 0
 * (Requirement 1.3, 25.3). `quality` / `latency_mode` are optional and omitted
 * from the wire payload by `JSON.stringify` when undefined.
 */
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

/**
 * Re-export so deep-link callers can normalize a raw VOD value before building
 * the link or request body without reaching for vodId.ts directly.
 */
export { normalizeVodId }

/** Analytics context parsed from `from=analytics` and `sid=` deep-link params. */
export interface VodAnalyticsContext {
  fromAnalytics: boolean
  streamId: string
  analyticsHref: string | null
}

/**
 * Parses analytics deep-link query params for VOD-mode "Back to Analytics" and
 * Re-sync actions (Req 20.5, 34.3).
 */
/**
 * Analytics VOD review uses Twitch embed + local rollups instead of the HLS relay.
 * Relay often fails with anonymous Usher tokens even when Twitch's player works.
 */
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
  const analyticsHref = streamId
    ? `/analytics/${encodeURIComponent(channelLogin)}/${encodeURIComponent(streamId)}`
    : fromMarker
      ? `/analytics/${encodeURIComponent(channelLogin)}`
      : null
  return { fromAnalytics, streamId, analyticsHref }
}
