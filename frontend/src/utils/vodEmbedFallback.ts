import { ApiError } from '../api'

export interface VodEmbedFallbackContext {
  fromAnalytics: boolean
}

/**
 * Analytics VOD review should fall back to Twitch embed when local relay cannot
 * start in time (gateway timeout, hls_not_ready, usher block, etc.).
 */
export function shouldUseTwitchEmbedFallback(
  error: unknown,
  ctx: VodEmbedFallbackContext,
): boolean {
  if (!ctx.fromAnalytics) {
    if (error instanceof ApiError) {
      return (error.reason ?? '').trim().toLowerCase() === 'usher_404'
    }
    return false
  }

  if (error instanceof ApiError) {
    const reason = (error.reason ?? '').trim().toLowerCase()
    if (reason === 'usher_404') return true
    if (error.status >= 502 && error.status <= 504) return true
    switch (error.code) {
      case 'hls_not_ready':
      case 'upstream_token_failed':
      case 'vod_start_failed':
      case 'capacity_reached':
      case 'vod_unavailable':
        return true
      default:
        break
    }
  }

  if (error instanceof Error) {
    return /bad gateway|gateway timeout|502|504|hls_not_ready|vod start failed/i.test(error.message)
  }

  return false
}
