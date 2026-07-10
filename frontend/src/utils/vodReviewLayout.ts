/**
 * Layout helpers for VOD review (Twitch embed + sidebar).
 * Deep-link query flags may still say "fromAnalytics" for compat; no Analytics product UI.
 */

export type AnalyticsVodSidebarTab = 'chat' | 'pulse'

export function isEmbedAnalyticsVodReview(
  showTwitchEmbed: boolean,
  fromAnalytics: boolean,
): boolean {
  return showTwitchEmbed && fromAnalytics
}

export function defaultAnalyticsVodSidebarTab(
  _fromAnalytics: boolean,
  _streamId: string | null | undefined,
): AnalyticsVodSidebarTab {
  return 'chat'
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
 * Resolves total VOD seconds for banners.
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
