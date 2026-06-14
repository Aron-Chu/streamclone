// Pure, React-free VOD relay error classification and copy mapping.
//
// Extracted from VodErrorState.tsx so the deterministic helpers
// (describeVodError, isVodErrorRetryable) can be imported by the Node test
// runner (node --experimental-strip-types), which cannot load .tsx files.
// VodErrorState.tsx re-exports everything here, so existing importers are
// unaffected.

// VOD relay error codes surfaced by POST /v1/stream/vod/start plus the
// client-detected HLS proxy auth condition (Requirement 2.8). Keep this list
// aligned with the backend orchestrator error codes.
export type VodErrorCode =
  | 'invalid_vod_id'
  | 'vod_unavailable'
  | 'upstream_token_failed'
  | 'capacity_reached'
  | 'hls_not_ready'
  | 'vod_start_failed'
  | 'hls_proxy_auth'

export interface VodErrorInput {
  code?: string | null
  message?: string | null
  retryable?: boolean | null
  reason?: string | null
}

export interface VodErrorContext {
  channelLogin?: string | null
  vodId?: string | null
  /** Deep link included `from=analytics` and/or `sid=` (Req 34.3). */
  fromAnalytics?: boolean
  /** Resolved analytics page href when analytics context is present. */
  analyticsHref?: string | null
  /** Analytics stream id from the `sid` query param (enables Re-sync). */
  analyticsStreamId?: string | null
}

export type VodErrorActionKind = 'retry' | 'analytics' | 'twitch' | 'hard-refresh' | 'resync'

export interface VodErrorAction {
  kind: VodErrorActionKind
  label: string
  href?: string
  external?: boolean
  primary?: boolean
}

export interface VodErrorDescriptor {
  code: VodErrorCode | 'unknown'
  title: string
  description: string
  retryable: boolean
  actions: VodErrorAction[]
}

// Maximum automatic retries for the hls_not_ready code (Requirement 2.5).
export const HLS_NOT_READY_MAX_AUTO_RETRIES = 2

// Codes that never offer a retry action (Requirement 2.7).
const NON_RETRYABLE_CODES = new Set<VodErrorCode>(['invalid_vod_id', 'vod_unavailable'])

// Codes that are always retryable regardless of the API retryable flag
// (Requirements 2.4, 2.5).
const ALWAYS_RETRYABLE_CODES = new Set<VodErrorCode>([
  'capacity_reached',
  'hls_not_ready',
  'hls_proxy_auth',
])

function knownCode(code?: string | null): VodErrorCode | null {
  switch (code) {
    case 'invalid_vod_id':
    case 'vod_unavailable':
    case 'upstream_token_failed':
    case 'capacity_reached':
    case 'hls_not_ready':
    case 'vod_start_failed':
    case 'hls_proxy_auth':
      return code
    default:
      return null
  }
}

/**
 * isVodErrorRetryable classifies whether a VOD start error should expose a
 * retry action. Retry presence matches the `retryable` flag per error code
 * (Requirement 2.7, Property 29):
 *   - invalid_vod_id, vod_unavailable      -> never retryable
 *   - capacity_reached, hls_not_ready,
 *     hls_proxy_auth                       -> always retryable
 *   - vod_start_failed, upstream_token_failed, unknown -> follow the flag
 */
export function isVodErrorRetryable(error: VodErrorInput): boolean {
  const code = knownCode(error.code)
  if (code && NON_RETRYABLE_CODES.has(code)) return false
  if (code && ALWAYS_RETRYABLE_CODES.has(code)) return true
  return error.retryable === true
}

function twitchUrl(ctx: VodErrorContext): string {
  if (ctx.vodId) return `https://www.twitch.tv/videos/${encodeURIComponent(ctx.vodId)}`
  if (ctx.channelLogin) return `https://www.twitch.tv/${encodeURIComponent(ctx.channelLogin)}`
  return 'https://www.twitch.tv'
}

function analyticsAction(ctx: VodErrorContext, primary = false): VodErrorAction {
  const href =
    ctx.analyticsHref ??
    (ctx.channelLogin ? `/analytics/${encodeURIComponent(ctx.channelLogin)}` : '/analytics')
  return {
    kind: 'analytics',
    label: 'Back to Analytics',
    href,
    primary,
  }
}

function resyncAction(): VodErrorAction {
  return { kind: 'resync', label: 'Re-sync' }
}

function retryAction(label = 'Retry'): VodErrorAction {
  return { kind: 'retry', label, primary: true }
}

/**
 * describeVodError maps a structured VOD start error to user-facing copy and
 * the actions to render. Pure and deterministic so it can be unit/property
 * tested independently of React (Requirements 1.7, 2.1-2.8).
 */
export function describeVodError(error: VodErrorInput, ctx: VodErrorContext = {}): VodErrorDescriptor {
  const code = knownCode(error.code)
  const retryable = isVodErrorRetryable(error)
  const detail = typeof error.message === 'string' ? error.message.trim() : ''

  switch (code) {
    case 'invalid_vod_id':
      return {
        code,
        title: 'VOD link is invalid',
        description:
          'This VOD link is invalid or the analytics data is stale. Head back to analytics and pick a fresh moment.',
        retryable: false,
        actions: [analyticsAction(ctx, true)],
      }
    case 'vod_unavailable': {
      if (ctx.fromAnalytics) {
        const actions: VodErrorAction[] = [
          { kind: 'twitch', label: 'Open on Twitch', href: twitchUrl(ctx), external: true, primary: true },
        ]
        if (ctx.analyticsHref) {
          actions.push(analyticsAction(ctx))
        }
        if (ctx.analyticsStreamId) {
          actions.push(resyncAction())
        }
        const reason = (error.reason ?? '').trim().toLowerCase()
        const usherBlocked = reason === 'usher_404'
        return {
          code,
          title: "VOD won't play in Streamclone",
          description: usherBlocked
            ? 'This VOD plays on Twitch in your browser, but Streamclone could not relay it with an anonymous token. Sign in with Twitch (top bar), then retry — or open the video on Twitch at this moment.'
            : 'This VOD may be deleted, subscriber-only, or restricted on Twitch — or the VOD id from analytics is stale. Re-sync stream metadata, open the video on Twitch to verify, or return to analytics.',
          retryable: false,
          actions,
        }
      }
      {
        const reason = (error.reason ?? '').trim().toLowerCase()
        const usherBlocked = reason === 'usher_404'
        return {
          code,
          title: usherBlocked ? "VOD won't play in Streamclone relay" : 'VOD unavailable',
          description: usherBlocked
            ? 'This VOD may play on Twitch in your browser, but Streamclone could not relay it. Use the embedded Twitch player when available, or open the video on Twitch.'
            : 'This VOD is deleted, subscriber-only, or not yet published. You can open the stream on Twitch instead.',
          retryable: false,
          actions: [
            { kind: 'twitch', label: 'Open on Twitch', href: twitchUrl(ctx), external: true, primary: true },
            analyticsAction(ctx),
          ],
        }
      }
    }
    case 'upstream_token_failed':
      return {
        code,
        title: 'Upstream authentication issue',
        description:
          'Streamclone could not authenticate with Twitch upstream. Check your token configuration, then try again.',
        retryable,
        actions: [...(retryable ? [retryAction()] : []), analyticsAction(ctx)],
      }
    case 'capacity_reached':
      return {
        code,
        title: 'Relay capacity reached',
        description: 'All relay slots are busy right now. Try again in a little while.',
        retryable: true,
        actions: [retryAction('Try again')],
      }
    case 'hls_not_ready':
      return {
        code,
        title: 'Relay is still warming up',
        description:
          'The relay started but the local HLS stream did not publish in time. Retrying automatically — you can also retry manually.',
        retryable: true,
        actions: [retryAction()],
      }
    case 'hls_proxy_auth':
      return {
        code,
        title: 'Local HLS proxy auth issue',
        description:
          'The relay started, but the local HLS proxy keeps returning 401. This usually means the MediaMTX hlsCDNSecret and the Caddy Bearer header do not match. Hard refresh after the stack restarts rather than assuming the VOD was removed from Twitch.',
        retryable: true,
        actions: [retryAction(), { kind: 'hard-refresh', label: 'Hard refresh' }],
      }
    case 'vod_start_failed':
      return {
        code,
        title: 'VOD playback failed',
        description: detail
          ? `VOD playback failed: ${detail}`
          : 'VOD playback failed for an unexpected reason. You can retry or head back to analytics.',
        retryable,
        actions: [...(retryable ? [retryAction()] : []), analyticsAction(ctx)],
      }
    default:
      return {
        code: 'unknown',
        title: 'VOD playback failed',
        description: detail || 'VOD playback failed for an unexpected reason.',
        retryable,
        actions: [...(retryable ? [retryAction()] : []), analyticsAction(ctx)],
      }
  }
}
