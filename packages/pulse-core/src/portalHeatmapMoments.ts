import { buildMomentScoreModel, momentScoreReasonLabel } from './momentScore.ts'
import {
  computeStreamBaselines,
  detectPickReason,
  fallbackMomentScore100,
  type CatalogTopEmote,
  type MomentScoringRollup,
  type RollupEmoteHit,
  topEmotesFromRollup,
} from './momentScoring.ts'
import { resolveEmoteImageUrl } from './emoteImageUrl.ts'
import type { HeatmapEmote, ReplayHeatmapPoint } from './types/heatmap.ts'

export const PORTAL_MOMENT_MAX_CANDIDATES = 10

const EMOTE_FAMILY_REASONS = new Set([
  'emote_spike',
  'seventv_spike',
  'twitch_emote_spike',
  'ffz_spike',
])

export function normalizeMinuteBucket(value?: string): string {
  if (!value) return ''
  const ms = Date.parse(value)
  if (!Number.isFinite(ms)) return value.trim()
  return String(Math.floor(ms / 60_000))
}

export type PortalMomentCandidate = {
  minuteTs: string
  minuteBucket: string
  offsetSeconds: number
  score: number
  scoreLabel: string
  reason: string
  reasonLabel: string
  topEmote?: RollupEmoteHit
  estimated: boolean
}

function offsetSecondsFromMinuteTs(minuteTs: string, streamStartedAt?: string): number {
  const minuteMs = Date.parse(minuteTs)
  const startMs = streamStartedAt ? Date.parse(streamStartedAt) : Number.NaN
  if (!Number.isFinite(minuteMs) || !Number.isFinite(startMs)) return 0
  return Math.max(0, Math.round((minuteMs - startMs) / 1000))
}

function catalogMatchForHeatmapEmote(
  emote: HeatmapEmote,
  catalog?: CatalogTopEmote[],
): CatalogTopEmote | undefined {
  if (!catalog?.length) return undefined
  const name = emote.name.trim().toLowerCase()
  const provider = (emote.provider ?? 'unknown').trim().toLowerCase()
  return catalog.find(entry => {
    const entryName = entry.name.trim().toLowerCase()
    if (entryName !== name) return false
    const entryProvider = (entry.provider ?? 'unknown').trim().toLowerCase()
    return entryProvider === provider || entryProvider === 'unknown' || provider === 'unknown'
  })
}

export function heatmapEmoteToRollupHit(
  emote: HeatmapEmote,
  catalog?: CatalogTopEmote[],
): RollupEmoteHit | undefined {
  if (!emote.name?.trim()) return undefined
  const provider = emote.provider?.trim().toLowerCase()
  const match = catalogMatchForHeatmapEmote(emote, catalog)
  const resolvedProvider =
    provider && provider !== 'unknown' ? provider : match?.provider
  const imageUrl =
    emote.imageUrl?.trim()
    || match?.imageUrl?.trim()
    || resolveEmoteImageUrl({
      provider: resolvedProvider,
      id: emote.id?.trim() || match?.id,
      scale: '1x',
    })
  return {
    key: emote.id?.trim() || emote.name.trim(),
    name: emote.name.trim(),
    provider: resolvedProvider && resolvedProvider !== 'unknown' ? resolvedProvider : undefined,
    count: emote.count ?? 0,
    image_url: imageUrl || undefined,
  }
}

export function honestMomentReasonLabel(
  reason: string,
  topEmote?: RollupEmoteHit,
  rollup?: MomentScoringRollup,
): string {
  if (topEmote?.name) return momentScoreReasonLabel(reason)
  if (!EMOTE_FAMILY_REASONS.has(reason)) return momentScoreReasonLabel(reason)
  if (!rollup) return momentScoreReasonLabel('chat_spike')
  if ((rollup.chatCount ?? 0) > 0) return momentScoreReasonLabel('chat_spike')
  if ((rollup.viewerLatest ?? rollup.viewerAvg ?? rollup.viewerMax ?? 0) > 0) {
    return momentScoreReasonLabel('viewer_spike')
  }
  return momentScoreReasonLabel('manual')
}

function rollupHasReactionData(rollup: MomentScoringRollup): boolean {
  return (rollup.chatCount ?? 0) > 0 || (rollup.totalEmoteCount ?? 0) > 0
}

function rollupHasMinuteData(rollup: MomentScoringRollup): boolean {
  if (rollup.missing) return false
  return (
    (rollup.viewerSamples ?? 0) > 0
    || (rollup.viewerLatest ?? 0) > 0
    || (rollup.viewerAvg ?? 0) > 0
    || (rollup.viewerMax ?? 0) > 0
    || (rollup.chatCount ?? 0) > 0
    || (rollup.totalEmoteCount ?? 0) > 0
  )
}

/** Rank portal Top Moments from backend replay heatmap (authoritative scores + emote IDs). */
export function heatmapPointsToMomentCandidates(
  heatmapPoints: ReplayHeatmapPoint[],
  streamStartedAt?: string,
  catalog?: CatalogTopEmote[],
  maxCandidates = PORTAL_MOMENT_MAX_CANDIDATES,
): PortalMomentCandidate[] {
  const ranked = heatmapPoints
    .filter(point => Number.isFinite(point.score) && point.score > 0 && point.minuteTs)
    .sort((a, b) => {
      if (b.score !== a.score) return b.score - a.score
      return offsetSecondsFromMinuteTs(a.minuteTs, streamStartedAt)
        - offsetSecondsFromMinuteTs(b.minuteTs, streamStartedAt)
    })
    .slice(0, maxCandidates)

  return ranked.map(point => {
    const topEmote = point.topEmotes?.[0]
      ? heatmapEmoteToRollupHit(point.topEmotes[0], catalog)
      : undefined
    const reason = point.reason?.trim() || 'manual'
    const score = Math.round(point.score)
    return {
      minuteTs: point.minuteTs,
      minuteBucket: normalizeMinuteBucket(point.minuteTs),
      offsetSeconds: offsetSecondsFromMinuteTs(point.minuteTs, streamStartedAt),
      score,
      scoreLabel: `${score}/100`,
      reason,
      reasonLabel: honestMomentReasonLabel(reason, topEmote),
      topEmote,
      estimated: false,
    }
  })
}

type FallbackRow = PortalMomentCandidate & { reactionRank: number }

/** Client fallback when heatmap is empty/warming — 25% of max score cutoff, deprioritize non-reaction windows. */
export function rollupFallbackMomentCandidates(
  rollups: MomentScoringRollup[],
  catalog?: CatalogTopEmote[],
  streamStartedAt?: string,
  maxCandidates = PORTAL_MOMENT_MAX_CANDIDATES,
): PortalMomentCandidate[] {
  const dataRollups = rollups.filter(rollupHasMinuteData)
  if (!dataRollups.length) return []

  const baselines = computeStreamBaselines(dataRollups)
  const hasReactionCoverage = dataRollups.some(rollupHasReactionData)

  const rows: FallbackRow[] = dataRollups.map(rollup => {
    const reason = detectPickReason(rollup, baselines, catalog)
    const scoreModel = buildMomentScoreModel({
      fallbackScore100: fallbackMomentScore100(rollup, baselines, dataRollups),
      fallbackReason: reason,
      fallbackTopEmotes: topEmotesFromRollup(rollup, 3, catalog).map(emote => ({
        id: emote.key,
        name: emote.name,
        imageUrl: emote.image_url ?? '',
        count: emote.count,
        provider: emote.provider ?? 'unknown',
      })),
    })
    const topEmote = topEmotesFromRollup(rollup, 1, catalog)[0]
    const score = Math.round(scoreModel.score)
    return {
      minuteTs: rollup.minuteTs ?? '',
      minuteBucket: normalizeMinuteBucket(rollup.minuteTs),
      offsetSeconds: offsetSecondsFromMinuteTs(rollup.minuteTs ?? '', streamStartedAt),
      score,
      scoreLabel: scoreModel.label,
      reason: scoreModel.reason,
      reasonLabel: honestMomentReasonLabel(scoreModel.reason, topEmote, rollup),
      topEmote,
      estimated: true,
      reactionRank: rollupHasReactionData(rollup) ? 0 : hasReactionCoverage ? 1 : 0,
    }
  })

  const maxScore = Math.max(...rows.map(row => row.score))
  if (maxScore <= 0) return []
  const cutoff = Math.max(1, Math.round(maxScore * 0.25))

  return rows
    .filter(row => row.score >= cutoff)
    .sort((a, b) => {
      if (a.reactionRank !== b.reactionRank) return a.reactionRank - b.reactionRank
      if (b.score !== a.score) return b.score - a.score
      return a.offsetSeconds - b.offsetSeconds
    })
    .slice(0, maxCandidates)
    .map(({ reactionRank: _reactionRank, ...candidate }) => candidate)
}

/** Join a heatmap candidate to the nearest minute rollup for chart highlight / VOD jump. */
export function findRollupForMomentCandidate(
  rollups: MomentScoringRollup[],
  candidate: PortalMomentCandidate,
): MomentScoringRollup | undefined {
  if (!rollups.length) return undefined
  if (candidate.minuteBucket) {
    const byBucket = rollups.find(
      rollup => normalizeMinuteBucket(rollup.minuteTs) === candidate.minuteBucket,
    )
    if (byBucket) return byBucket
  }
  if (candidate.minuteTs) {
    const byTs = rollups.find(rollup => rollup.minuteTs === candidate.minuteTs)
    if (byTs) return byTs
  }
  const targetMs = Date.parse(candidate.minuteTs)
  if (!Number.isFinite(targetMs)) return undefined
  let best: MomentScoringRollup | undefined
  let bestDistance = Number.POSITIVE_INFINITY
  for (const rollup of rollups) {
    const ms = Date.parse(rollup.minuteTs ?? '')
    if (!Number.isFinite(ms)) continue
    const distance = Math.abs(ms - targetMs)
    if (distance < bestDistance) {
      best = rollup
      bestDistance = distance
    }
  }
  return bestDistance <= 60_000 ? best : undefined
}
