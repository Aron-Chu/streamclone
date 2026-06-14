// Live stats derivation for the Analytics Live Stats Band (Requirement 18).
//
// Pure, dependency-free helpers that turn an AnalyticsStreamDetail-shaped
// payload (the response of GET /v1/analytics/channels/{login}/live) into the
// compact set of numbers the Live_Stats_Band renders every 15 seconds:
// current viewers + 5-minute delta, chat/min (1-minute and 5-minute average
// with a trend arrow), emotes/min split by provider, the top emotes, a data
// confidence state, and a 60-point sparkline series.
//
// These helpers are intentionally kept out of the React component so they can
// be unit-tested (task 15.2) without rendering. They define their own minimal
// input shapes (mirroring AnalyticsMinuteRollup / AnalyticsTopEmote from
// api.ts) so the module stays free of runtime imports and is importable by the
// node --experimental-strip-types test runner.

/** Refresh cadence for the Live_Stats_Band (Req 18.1). */
export const LIVE_REFRESH_MS = 15000

/** Request timeout after which the band keeps last values + stale indicator (Req 18.5). */
export const LIVE_REQUEST_TIMEOUT_MS = 5000

/** Number animation duration window in ms (Req 18.2). */
export const NUMBER_ANIM_MIN_MS = 200
export const NUMBER_ANIM_MAX_MS = 300

/** Maximum sparkline points at 1-point-per-minute resolution (Req 18.3). */
export const SPARKLINE_MAX_POINTS = 60

/** Maximum number of top emote images displayed per update cycle (Req 18.1). */
export const MAX_TOP_EMOTES = 3

/**
 * Window (in minutes) used to compute the viewer delta and the 5-minute chat
 * average. The "5-minute delta" compares the current viewers to the value five
 * completed minutes earlier (Req 18.1).
 */
export const DELTA_WINDOW_MINUTES = 5

/**
 * Relative tolerance for the chat trend arrow. The 1-minute rate is "stable"
 * when it is within 10% of the 5-minute average, otherwise up/down (Req 18.1).
 */
export const TREND_STABLE_RATIO = 0.1

/**
 * Minimum number of completed chat-bearing rollup minutes before the band is
 * confident enough to report "Synced" for a stream that is no longer live.
 */
export const SYNCED_ROLLUP_THRESHOLD = 5

export type TrendDirection = 'up' | 'down' | 'stable'

/**
 * Data confidence states surfaced by the band (Req 18.1). They communicate how
 * much to trust the displayed numbers, not how exciting the stream is.
 */
export type LiveConfidenceState =
  | 'Waiting for first minute'
  | 'Collecting'
  | 'Stats only'
  | 'Synced'

/** Collection state of the stream detail response. Mirrors AnalyticsStreamDetail.state. */
export type StreamCollectionState = 'live' | 'historical' | 'not_collected' | 'syncing'

/** Minimal rollup shape needed for live stats. Mirrors AnalyticsMinuteRollup. */
export interface LiveStatsRollup {
  minuteTs?: string
  viewerAvg?: number
  viewerMax?: number
  viewerLatest?: number
  viewerSamples?: number
  chatCount?: number
  totalEmoteCount?: number
  seventvEmoteCount?: number
  emotes?: Record<string, number>
  missing?: boolean
}

/** Minimal top-emote shape. Mirrors AnalyticsTopEmote. */
export interface LiveTopEmote {
  key?: string
  name: string
  id?: string
  provider?: string
  imageUrl?: string
  count: number
}

export interface LiveStatsInput {
  state: StreamCollectionState
  rollups: LiveStatsRollup[]
  topEmotes?: LiveTopEmote[]
  /** Stream-level average viewers (TwitchTracker / Helix), 0 when unknown. */
  avgViewers?: number
  /** Stream-level peak viewers (TwitchTracker / Helix), 0 when unknown. */
  peakViewers?: number
}

/** Per-provider emote rate for the current (most recent completed) minute. */
export interface EmoteProviderRate {
  provider: string
  perMinute: number
}

export interface LiveStats {
  currentViewers: number
  /** Signed change in viewers over the last DELTA_WINDOW_MINUTES, or null if unknown. */
  viewerDelta5m: number | null
  /** Chat messages in the most recent completed minute. */
  chatPerMin1m: number
  /** Average chat messages/minute over the last DELTA_WINDOW_MINUTES completed minutes. */
  chatPerMin5m: number
  chatTrend: TrendDirection
  /** Emotes/min split by provider for the most recent completed minute (Req 18.1). */
  emoteProviderRates: EmoteProviderRate[]
  /** Whether at least one provider reported emote activity this minute. */
  hasProviderSplit: boolean
  /** Total emotes/min for the most recent completed minute. */
  totalEmotePerMin: number
  /** Up to MAX_TOP_EMOTES top emotes with images. */
  topEmotes: LiveTopEmote[]
  confidence: LiveConfidenceState
  /** Count of non-missing rollup minutes available. */
  completedRollupCount: number
  /** Up to SPARKLINE_MAX_POINTS chat-per-minute values, oldest first (Req 18.3). */
  sparkline: number[]
}

/** A rollup that actually carries data (not a synthetic gap-fill row). */
function isCompletedRollup(r: LiveStatsRollup): boolean {
  if (r.missing) return false
  return (
    (r.viewerSamples ?? 0) > 0 ||
    (r.chatCount ?? 0) > 0 ||
    (r.totalEmoteCount ?? 0) > 0
  )
}

function viewerOf(r: LiveStatsRollup): number {
  const latest = r.viewerLatest ?? 0
  if (latest > 0) return latest
  return r.viewerAvg ?? 0
}

/**
 * Decide the trend arrow for the 1-minute chat rate relative to the 5-minute
 * average (Req 18.1). The rate is "stable" when within TREND_STABLE_RATIO of
 * the average. When the average is zero, any positive 1-minute rate trends up.
 */
export function computeTrend(
  oneMinRate: number,
  fiveMinAvg: number,
  ratio: number = TREND_STABLE_RATIO,
): TrendDirection {
  const one = Number.isFinite(oneMinRate) ? oneMinRate : 0
  const five = Number.isFinite(fiveMinAvg) ? fiveMinAvg : 0

  if (five <= 0) {
    return one > 0 ? 'up' : 'stable'
  }

  const tolerance = Math.abs(five) * ratio
  const diff = one - five
  if (Math.abs(diff) <= tolerance) return 'stable'
  return diff > 0 ? 'up' : 'down'
}

/**
 * Split the most recent completed minute's emote count by provider (Req 18.1).
 * The live rollup distinguishes 7TV from the remaining (Twitch/FFZ/native)
 * counts, so we surface "7TV" and "Other" lanes when each has activity.
 */
export function splitEmoteProviderRates(rollup: LiveStatsRollup | undefined): EmoteProviderRate[] {
  if (!rollup) return []
  const total = Math.max(0, Math.round(rollup.totalEmoteCount ?? 0))
  const seventv = Math.max(0, Math.round(rollup.seventvEmoteCount ?? 0))
  const other = Math.max(0, total - seventv)

  const rates: EmoteProviderRate[] = []
  if (seventv > 0) rates.push({ provider: '7TV', perMinute: seventv })
  if (other > 0) rates.push({ provider: 'Other', perMinute: other })
  return rates
}

/**
 * Determine the data-confidence state for the band (Req 18.1).
 *
 *  - "Waiting for first minute": collecting but no completed rollup minute yet.
 *  - "Collecting": live stream still warming up (some minutes, below the
 *     synced threshold).
 *  - "Synced": enough completed chat-bearing minutes (or a synced historical
 *     stream) that the numbers are trustworthy.
 *  - "Stats only": only stream-level averages exist, with no minute-level
 *     chat/viewer rollups to derive live activity from.
 */
export function liveConfidenceState(input: LiveStatsInput): LiveConfidenceState {
  const rollups = input.rollups ?? []
  const completed = rollups.filter(isCompletedRollup)
  const chatBearing = completed.filter(r => (r.chatCount ?? 0) > 0).length
  const hasTrackerAverages = (input.avgViewers ?? 0) > 0 || (input.peakViewers ?? 0) > 0
  const isLive = input.state === 'live' || input.state === 'syncing'

  if (completed.length === 0) {
    if (isLive) return 'Waiting for first minute'
    if (hasTrackerAverages) return 'Stats only'
    return 'Waiting for first minute'
  }

  if (chatBearing >= SYNCED_ROLLUP_THRESHOLD) return 'Synced'

  if (isLive) return 'Collecting'

  if (chatBearing > 0) return 'Synced'
  if (hasTrackerAverages) return 'Stats only'
  return 'Collecting'
}

/**
 * Build the sparkline series (chat messages per minute) for up to the most
 * recent SPARKLINE_MAX_POINTS completed minutes, oldest first (Req 18.3).
 */
export function buildSparkline(
  rollups: LiveStatsRollup[],
  maxPoints: number = SPARKLINE_MAX_POINTS,
): number[] {
  const completed = (rollups ?? []).filter(isCompletedRollup)
  const limit = Math.max(0, Math.floor(maxPoints))
  const recent = limit > 0 ? completed.slice(-limit) : []
  return recent.map(r => Math.max(0, Math.round(r.chatCount ?? 0)))
}

/**
 * Derive the full set of live stats from a stream detail payload (Req 18.1,
 * 18.3). Pure and deterministic: identical input yields identical output.
 */
export function deriveLiveStats(input: LiveStatsInput): LiveStats {
  const rollups = input.rollups ?? []
  const completed = rollups.filter(isCompletedRollup)
  const last = completed[completed.length - 1]

  const currentViewers = last ? viewerOf(last) : 0

  let viewerDelta5m: number | null = null
  if (completed.length > DELTA_WINDOW_MINUTES) {
    const prior = completed[completed.length - 1 - DELTA_WINDOW_MINUTES]
    if (prior) viewerDelta5m = currentViewers - viewerOf(prior)
  }

  const chatPerMin1m = last ? Math.max(0, Math.round(last.chatCount ?? 0)) : 0

  const lastFive = completed.slice(-DELTA_WINDOW_MINUTES)
  const chatPerMin5m =
    lastFive.length > 0
      ? lastFive.reduce((sum, r) => sum + Math.max(0, r.chatCount ?? 0), 0) / lastFive.length
      : 0

  const chatTrend = computeTrend(chatPerMin1m, chatPerMin5m)

  const emoteProviderRates = splitEmoteProviderRates(last)
  const totalEmotePerMin = last ? Math.max(0, Math.round(last.totalEmoteCount ?? 0)) : 0

  const topEmotes = (input.topEmotes ?? [])
    .filter(e => (e.count ?? 0) >= 0)
    .slice(0, MAX_TOP_EMOTES)

  return {
    currentViewers,
    viewerDelta5m,
    chatPerMin1m,
    chatPerMin5m: Math.round(chatPerMin5m * 10) / 10,
    chatTrend,
    emoteProviderRates,
    hasProviderSplit: emoteProviderRates.length > 0,
    totalEmotePerMin,
    topEmotes,
    confidence: liveConfidenceState(input),
    completedRollupCount: completed.length,
    sparkline: buildSparkline(rollups),
  }
}

/** Glyph for a trend arrow direction, for compact rendering. */
export function trendArrowGlyph(trend: TrendDirection): string {
  switch (trend) {
    case 'up':
      return '▲'
    case 'down':
      return '▼'
    default:
      return '▬'
  }
}
