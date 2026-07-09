import {
  heatmapEmoteToRollupHit,
  normalizeMinuteBucket,
  topEmotesFromRollup,
  type RollupEmoteHit,
} from '@streamclone/pulse-core'
import type {
  AnalyticsMinuteRollup,
  AnalyticsTopEmote,
  PulseRecapEmote,
  PulseRecapMoment,
} from '../apiTypes.ts'
import type { ReplayHeatmapPoint } from '../types/heatmap.ts'
import { minuteEmoteTotal } from '../components/analytics/chartRollupUtils.ts'
import { viewerValue } from '../components/analytics/chartRollupUtils.ts'
import { findNearestRollupByOffset } from './momentSelection.ts'
import { enrichRecapEmotesFromCatalog, resolveMomentEmotesForOffset } from './recapEmoteEnrich.ts'

export interface MomentRowStats {
  viewers: number
  chatPerMin: number
  emotesPerMin: number
}

export function recapEmoteToRollupHit(emote: PulseRecapEmote): RollupEmoteHit {
  const code = emote.code.trim()
  const provider = (emote.provider ?? 'unknown').trim().toLowerCase()
  const id = emote.id?.trim()
  const key = id ? `${provider}:${id}` : `${provider}:${code}`
  return {
    key,
    name: code,
    count: emote.count ?? 0,
    provider: emote.provider ?? provider,
    image_url: emote.imageUrl,
  }
}

export function recapEmotesToRollupHits(
  emotes: PulseRecapEmote[],
  catalog?: AnalyticsTopEmote[],
): RollupEmoteHit[] {
  return enrichRecapEmotesFromCatalog(emotes, catalog).map(recapEmoteToRollupHit)
}

/** Prefer recap minute stats; fall back to nearest rollup minute. */
export function resolveMomentRowStats(args: {
  moment?: PulseRecapMoment
  rollups?: AnalyticsMinuteRollup[]
  streamStartedAt?: string
}): MomentRowStats {
  const { moment, rollups, streamStartedAt } = args
  const rollup =
    rollups?.length && streamStartedAt && moment
      ? findNearestRollupByOffset(rollups, streamStartedAt, moment.offsetSeconds)
      : null

  const viewers =
    (moment?.viewerCount ?? 0) > 0
      ? moment!.viewerCount!
      : rollup
        ? viewerValue(rollup) ?? 0
        : 0
  const chatPerMin =
    (moment?.chatCount ?? 0) > 0
      ? moment!.chatCount!
      : rollup?.chatCount ?? 0
  const emotesPerMin =
    (moment?.emoteCount ?? 0) > 0
      ? moment!.emoteCount!
      : rollup
        ? minuteEmoteTotal(rollup)
        : 0

  return { viewers, chatPerMin, emotesPerMin }
}

/** Rollup → heatmap point → recap topEmotes for display chips. */
export function resolveRollupDisplayEmotes(args: {
  rollup: AnalyticsMinuteRollup
  rollups?: AnalyticsMinuteRollup[]
  streamStartedAt?: string
  heatmapPoints?: ReplayHeatmapPoint[]
  recapMoment?: PulseRecapMoment | null
  topEmotesCatalog?: AnalyticsTopEmote[]
  limit?: number
}): RollupEmoteHit[] {
  const {
    rollup,
    rollups,
    streamStartedAt,
    heatmapPoints,
    recapMoment,
    topEmotesCatalog,
    limit = 3,
  } = args

  const fromRollup = topEmotesFromRollup(rollup, limit, topEmotesCatalog)
  if (fromRollup.length > 0) return fromRollup

  if (heatmapPoints?.length) {
    const bucket = normalizeMinuteBucket(rollup.minuteTs)
    const point = heatmapPoints.find(
      (entry) =>
        entry.minuteTs === rollup.minuteTs
        || normalizeMinuteBucket(entry.minuteTs) === bucket,
    )
    if (point?.topEmotes?.length) {
      const fromHeatmap = point.topEmotes
        .slice(0, limit)
        .map((emote) => heatmapEmoteToRollupHit(emote, topEmotesCatalog))
        .filter((emote): emote is RollupEmoteHit => Boolean(emote))
      if (fromHeatmap.length > 0) return fromHeatmap
    }
  }

  if (recapMoment?.topEmotes?.length) {
    return recapEmotesToRollupHits(recapMoment.topEmotes, topEmotesCatalog).slice(0, limit)
  }

  if (recapMoment && rollups?.length && streamStartedAt) {
    const fromOffset = resolveMomentEmotesForOffset({
      moment: recapMoment,
      rollups,
      streamStartedAt,
      heatmapPoints,
      topEmotesCatalog,
      limit,
    })
    if (fromOffset.length > 0) {
      return recapEmotesToRollupHits(fromOffset, topEmotesCatalog)
    }
  }

  return []
}
