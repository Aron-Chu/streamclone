// Live heat ("Most Reacted So Far") derivation for the live analytics page
// (Requirement 19.1, 19.2).
//
// Pure, dependency-free helpers that rank the most reacted moments of an
// in-progress stream from its live minute rollups (the response of
// GET /v1/analytics/channels/{login}/live). The ranking is a simple
// chat + emote activity score consistent with the Most_Reacted glossary
// label, NOT the YouTube-style Most_Replayed watch-replay metric.
//
// Behaviour (Req 19.1, 19.2):
//   - The "Most Reacted So Far" section is only shown once at least 5
//     completed (closed) minute rollups exist for the current stream.
//   - Up to 10 scored moment points are returned, ranked by reaction score.
//   - The trailing incomplete minute (the most recent, still-collecting
//     bucket while the stream is live) is surfaced separately, flagged so the
//     UI can mute it and label it "Collecting" because its score may change
//     once the minute closes.
//   - The subtitle communicates that scores are based on chat and emote
//     activity.
//
// These helpers are intentionally kept out of the React component (mirroring
// utils/liveStats.ts) so they can be unit-tested without rendering and stay
// importable by the node --experimental-strip-types test runner. They define
// their own minimal input shapes (mirroring AnalyticsMinuteRollup /
// AnalyticsTopEmote from api.ts).

import { resolveEmoteImageUrl } from './emoteImageUrl.ts'

/** Refresh cadence for the live heat section (Req 19.1). */
export const LIVE_HEAT_REFRESH_MS = 30000

/** Minimum completed rollups before the section is shown (Req 19.1). */
export const LIVE_HEAT_MIN_COMPLETED_ROLLUPS = 5

/** Maximum scored moment points returned (Req 19.1). */
export const LIVE_HEAT_MAX_POINTS = 10

/** Maximum top-emote images attached to a point. */
export const LIVE_HEAT_MAX_EMOTES = 3

/** Subtitle copy clarifying the score source (Req 19.1). */
export const LIVE_HEAT_SUBTITLE = 'based on chat and emote activity'

/** Section heading copy — honest "Most Reacted", never "Most Replayed" (Req 19.1). */
export const LIVE_HEAT_TITLE = 'Most Reacted So Far'

/** Label for the trailing incomplete minute bucket (Req 19.2). */
export const LIVE_HEAT_COLLECTING_LABEL = 'Collecting'

/** Collection state of the stream detail response. Mirrors AnalyticsStreamDetail.state. */
export type LiveHeatStreamState = 'live' | 'historical' | 'not_collected' | 'syncing'

/** Minimal rollup shape needed for live heat. Mirrors AnalyticsMinuteRollup. */
export interface LiveHeatRollup {
  minuteTs?: string
  viewerSamples?: number
  chatCount?: number
  totalEmoteCount?: number
  seventvEmoteCount?: number
  emotes?: Record<string, number>
  missing?: boolean
}

/** Minimal top-emote catalog shape. Mirrors AnalyticsTopEmote. */
export interface LiveHeatCatalogEmote {
  key?: string
  name: string
  id?: string
  provider?: string
  imageUrl?: string
  count: number
}

/** A top emote attached to a moment point. */
export interface LiveHeatEmote {
  key: string
  name: string
  id?: string
  provider?: string
  imageUrl?: string
  count: number
}

export type LiveHeatReason =
  | 'chat_spike'
  | 'emote_spike'
  | 'seventv_spike'
  | 'twitch_emote_spike'
  | 'ffz_spike'

export interface LiveHeatPoint {
  /** Minute bucket timestamp (ISO 8601) of the rollup. */
  minuteTs: string
  /** Whole-second offset from the first available rollup minute. */
  offsetSeconds: number
  /** Reaction score in [0, 100]. */
  score: number
  reason: LiveHeatReason
  reasonLabel: string
  chatCount: number
  emoteCount: number
  topEmotes: LiveHeatEmote[]
  /** True for the trailing incomplete minute (muted "Collecting"). */
  collecting: boolean
}

export interface LiveHeatInput {
  state: LiveHeatStreamState
  rollups: LiveHeatRollup[]
  topEmotes?: LiveHeatCatalogEmote[]
  /** When set, offsets are absolute seconds from stream start (VOD seek). */
  streamStartedAt?: string
}

export interface LiveHeatResult {
  /** Whether the section should render (Req 19.1: >= 5 completed rollups). */
  visible: boolean
  /** Number of completed (closed, data-bearing) rollup minutes. */
  completedRollupCount: number
  /** Up to LIVE_HEAT_MAX_POINTS scored points, ranked by score descending. */
  points: LiveHeatPoint[]
  /** The trailing incomplete minute, muted + labeled "Collecting" (Req 19.2). */
  collectingPoint: LiveHeatPoint | null
  /** Subtitle copy (Req 19.1). */
  subtitle: string
}

interface ScoreBaselines {
  chat: number
  emotes: number
}

const REASON_LABELS: Record<LiveHeatReason, string> = {
  chat_spike: 'Chat spike',
  emote_spike: 'Emote spike',
  seventv_spike: '7TV emote spike',
  twitch_emote_spike: 'Twitch emote spike',
  ffz_spike: 'FFZ emote spike',
}

/** Reaction-bearing rollup (not a synthetic gap-fill row). */
function isDataRollup(r: LiveHeatRollup): boolean {
  if (r.missing) return false
  return (
    (r.viewerSamples ?? 0) > 0 ||
    (r.chatCount ?? 0) > 0 ||
    (r.totalEmoteCount ?? 0) > 0
  )
}

/** Total emote count for a minute, falling back to the emotes map when needed. */
function emoteTotalOf(r: LiveHeatRollup): number {
  const total = r.totalEmoteCount ?? 0
  if (total > 0) return total
  if (r.emotes) {
    return Object.values(r.emotes).reduce((sum, n) => sum + Math.max(0, n), 0)
  }
  return 0
}

/** Parse an emote rollup key of the form "provider:id:name" (best effort). */
function parseEmoteKey(key: string): { provider?: string; id?: string; name: string } {
  const parts = key.split(':')
  if (parts.length >= 3) {
    const [provider, id, ...rest] = parts
    return { provider, id, name: rest.join(':') || key }
  }
  if (parts.length === 2) {
    return { provider: parts[0], name: parts[1] }
  }
  return { name: key }
}

function topEmotesFromRollup(
  r: LiveHeatRollup,
  catalog: Map<string, LiveHeatCatalogEmote>,
  byName: Map<string, LiveHeatCatalogEmote>,
  limit = LIVE_HEAT_MAX_EMOTES,
): LiveHeatEmote[] {
  if (!r.emotes) return []
  return Object.entries(r.emotes)
    .filter(([, count]) => count > 0)
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .slice(0, limit)
    .map(([key, count]) => {
      const parsed = parseEmoteKey(key)
      const match = catalog.get(key) ?? byName.get(parsed.name.toLowerCase())
      const id = match?.id ?? parsed.id
      const provider = match?.provider ?? (parsed.provider && parsed.provider !== 'unknown' ? parsed.provider : undefined)
      const imageUrl = resolveEmoteImageUrl({
        provider,
        id,
        imageUrl: match?.imageUrl,
        scale: '1x',
      })
      return {
        key,
        name: match?.name ?? parsed.name,
        id,
        provider,
        imageUrl: imageUrl || undefined,
        count,
      }
    })
}

function computeBaselines(rollups: LiveHeatRollup[]): ScoreBaselines {
  if (!rollups.length) return { chat: 1, emotes: 1 }
  const chat = rollups.reduce((sum, r) => sum + Math.max(0, r.chatCount ?? 0), 0) / rollups.length
  const emotes = rollups.reduce((sum, r) => sum + emoteTotalOf(r), 0) / rollups.length
  return { chat: chat || 1, emotes: emotes || 1 }
}

/**
 * Reaction score in [0, 100] from chat + emote activity relative to the
 * stream's own per-minute baseline (Most_Reacted). Chat is weighted slightly
 * above emotes; both are normalized against twice the baseline so a minute at
 * 2x average activity reaches the top of the scale.
 */
function reactionScore(r: LiveHeatRollup, baselines: ScoreBaselines): number {
  const chatNorm = Math.min(1, Math.max(0, r.chatCount ?? 0) / Math.max(baselines.chat * 2, 1))
  const emoteNorm = Math.min(1, emoteTotalOf(r) / Math.max(baselines.emotes * 2, 1))
  const weighted = chatNorm * 0.6 + emoteNorm * 0.4
  return Math.round(Math.min(100, Math.max(0, weighted * 100)))
}

function detectReason(
  r: LiveHeatRollup,
  baselines: ScoreBaselines,
  topEmotes: LiveHeatEmote[],
): LiveHeatReason {
  const chatMult = (r.chatCount ?? 0) / Math.max(baselines.chat, 1)
  const emoteMult = emoteTotalOf(r) / Math.max(baselines.emotes, 1)
  if (emoteMult >= 2 && emoteMult >= chatMult) {
    const provider = topEmotes[0]?.provider
    if (provider === 'seventv') return 'seventv_spike'
    if (provider === 'twitch') return 'twitch_emote_spike'
    if (provider === 'ffz') return 'ffz_spike'
    return 'emote_spike'
  }
  return 'chat_spike'
}

function parseMinuteMs(minuteTs: string | undefined): number {
  if (!minuteTs) return Number.NaN
  return Date.parse(minuteTs)
}

function buildPoint(
  r: LiveHeatRollup,
  baselines: ScoreBaselines,
  firstMs: number,
  catalog: Map<string, LiveHeatCatalogEmote>,
  byName: Map<string, LiveHeatCatalogEmote>,
  collecting: boolean,
  streamStartedMs?: number,
): LiveHeatPoint {
  const topEmotes = topEmotesFromRollup(r, catalog, byName)
  const reason = detectReason(r, baselines, topEmotes)
  const minuteMs = parseMinuteMs(r.minuteTs)
  const anchorMs = Number.isFinite(streamStartedMs) ? streamStartedMs! : firstMs
  const offsetSeconds =
    Number.isFinite(minuteMs) && Number.isFinite(anchorMs)
      ? Math.max(0, Math.round((minuteMs - anchorMs) / 1000))
      : 0
  return {
    minuteTs: r.minuteTs ?? '',
    offsetSeconds,
    score: reactionScore(r, baselines),
    reason,
    reasonLabel: REASON_LABELS[reason],
    chatCount: Math.max(0, Math.round(r.chatCount ?? 0)),
    emoteCount: Math.max(0, Math.round(emoteTotalOf(r))),
    topEmotes,
    collecting,
  }
}

/**
 * Derive the "Most Reacted So Far" live heat from a live stream detail payload
 * (Req 19.1, 19.2). Pure and deterministic: identical input yields identical
 * output, with a stable tie-break (higher score, then earlier offset).
 *
 * The trailing incomplete minute is the most recent data-bearing rollup while
 * the stream is live; it is excluded from the completed-rollup count and the
 * ranked points, and returned separately as `collectingPoint`.
 */
export function deriveLiveHeat(input: LiveHeatInput): LiveHeatResult {
  const isLive = input.state === 'live' || input.state === 'syncing'
  const catalog = new Map<string, LiveHeatCatalogEmote>(
    (input.topEmotes ?? []).filter(e => e.key).map(e => [e.key as string, e]),
  )
  const byName = new Map<string, LiveHeatCatalogEmote>(
    (input.topEmotes ?? []).map(e => [e.name.toLowerCase(), e]),
  )

  // Data-bearing rollups, sorted oldest first by minute timestamp.
  const dataRollups = (input.rollups ?? [])
    .filter(isDataRollup)
    .slice()
    .sort((a, b) => parseMinuteMs(a.minuteTs) - parseMinuteMs(b.minuteTs))

  // The trailing incomplete minute (newest bucket) is still collecting while
  // the stream is live. It is not a "completed" rollup (Req 19.2).
  let completed = dataRollups
  let collectingRollup: LiveHeatRollup | null = null
  if (isLive && dataRollups.length > 0) {
    collectingRollup = dataRollups[dataRollups.length - 1]
    completed = dataRollups.slice(0, -1)
  }

  const baselines = computeBaselines(completed.length ? completed : dataRollups)
  const firstMs = parseMinuteMs(dataRollups[0]?.minuteTs)
  const streamStartedMs = parseMinuteMs(input.streamStartedAt)

  const collectingPoint = collectingRollup
    ? buildPoint(collectingRollup, baselines, firstMs, catalog, byName, true, streamStartedMs)
    : null

  const visible = completed.length >= LIVE_HEAT_MIN_COMPLETED_ROLLUPS

  const points = visible
    ? completed
        .map(r => buildPoint(r, baselines, firstMs, catalog, byName, false, streamStartedMs))
        .filter(p => p.score > 0)
        .sort((a, b) => b.score - a.score || a.offsetSeconds - b.offsetSeconds)
        .slice(0, LIVE_HEAT_MAX_POINTS)
    : []

  return {
    visible,
    completedRollupCount: completed.length,
    points,
    collectingPoint,
    subtitle: LIVE_HEAT_SUBTITLE,
  }
}

/** Format whole seconds as HH:MM:SS for offset labels. */
export function formatHeatOffset(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds))
  const hh = Math.floor(s / 3600)
  const mm = Math.floor((s % 3600) / 60)
  const ss = s % 60
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${pad(hh)}:${pad(mm)}:${pad(ss)}`
}
