import type { AnalyticsStream, AnalyticsStreamDetail } from '../api'

/**
 * Layout helpers for analytics-origin VOD review (Twitch embed + Pulse sidebar).
 */

export type AnalyticsVodSidebarTab = 'chat' | 'pulse'

export function isEmbedAnalyticsVodReview(
  showTwitchEmbed: boolean,
  fromAnalytics: boolean,
): boolean {
  return showTwitchEmbed && fromAnalytics
}

export function defaultAnalyticsVodSidebarTab(
  fromAnalytics: boolean,
  streamId: string | null | undefined,
): AnalyticsVodSidebarTab {
  return fromAnalytics && (streamId?.trim().length ?? 0) > 0 ? 'pulse' : 'chat'
}

function streamDurationFromBounds(stream?: AnalyticsStream): number | null {
  if (!stream?.startedAt || !stream.endedAt) return null
  const startMs = Date.parse(stream.startedAt)
  const endMs = Date.parse(stream.endedAt)
  if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs <= startMs) return null
  return Math.floor((endMs - startMs) / 1000)
}

export function resolveVodDetailDurationSec(
  detail: AnalyticsStreamDetail | null | undefined,
): number | null {
  const fromDetail = detail?.vodDurationSec
  if (fromDetail != null && Number.isFinite(fromDetail) && fromDetail > 0) {
    return fromDetail
  }
  const fromBounds = streamDurationFromBounds(detail?.stream)
  if (fromBounds != null) return fromBounds
  const rollupMinutes = detail?.rollups?.length ?? 0
  if (rollupMinutes > 0) {
    return rollupMinutes * 60
  }
  return null
}

function pickPositiveDuration(...values: Array<number | null | undefined>): number | null {
  for (const value of values) {
    if (value != null && Number.isFinite(value) && value > 0) {
      return value
    }
  }
  return null
}

/**
 * Resolves total VOD seconds using the maximum of all known duration sources so a
 * partial synced rollup window cannot shrink the displayed length.
 */
export function resolveVodTotalDurationSec(input: {
  rollupDurationSec: number | null
  embedDurationSec: number | null
  vodDetailDurationSec: number | null
  relaySeekableEndSec: number | null | undefined
}): number | null {
  let max: number | null = null
  for (const value of [
    input.vodDetailDurationSec,
    input.rollupDurationSec,
    input.embedDurationSec,
    input.relaySeekableEndSec ?? null,
  ]) {
    if (value != null && Number.isFinite(value) && value > 0) {
      max = max == null ? value : Math.max(max, value)
    }
  }
  return max
}

/**
 * Resolves total VOD seconds for banners and Pulse charts.
 * Embed review prefers rollup/detail duration because Twitch getDuration() is often null.
 */
export function resolveVodBannerTotalSec(input: {
  preferRollupDuration: boolean
  rollupDurationSec: number | null
  embedDurationSec: number | null
  vodDetailDurationSec: number | null
  relaySeekableEndSec: number | null | undefined
}): number | null {
  if (input.preferRollupDuration) {
    return pickPositiveDuration(
      input.rollupDurationSec,
      input.vodDetailDurationSec,
      input.embedDurationSec,
    )
  }
  return pickPositiveDuration(
    input.rollupDurationSec,
    input.relaySeekableEndSec ?? null,
    input.embedDurationSec,
    input.vodDetailDurationSec,
  )
}
