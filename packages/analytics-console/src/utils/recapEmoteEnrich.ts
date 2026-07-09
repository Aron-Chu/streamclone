import type { AnalyticsMinuteRollup, AnalyticsTopEmote, PulseRecapEmote, PulseRecapMoment } from '../apiTypes.ts'
import type { ReplayHeatmapPoint } from '../types/heatmap.ts'
import { heatmapEmoteToRollupHit, normalizeMinuteBucket, topEmotesFromRollup } from '@streamclone/pulse-core'
import { parseEmoteKey } from '../emoteUtils.ts'
import { getEmoteImageUrl, rollupOffsetSeconds } from './consoleFormat.ts'
import { isBackendEmoteProxyUrl } from './emoteImageUrl.ts'
import { findNearestRollupByOffset } from './momentSelection.ts'

function recapEmoteLookupKey(code: string, provider?: string): string {
  const name = code.trim().toLowerCase()
  const prov = (provider ?? 'seventv').trim().toLowerCase()
  return `${prov}:${name}`
}

function catalogEntryForRecap(
  emote: PulseRecapEmote,
  catalog: AnalyticsTopEmote[],
): AnalyticsTopEmote | undefined {
  const keys = new Set<string>()
  keys.add(recapEmoteLookupKey(emote.code, emote.provider))
  keys.add(emote.code.trim().toLowerCase())
  for (const entry of catalog) {
    const name = entry.name.trim().toLowerCase()
    if (!name) continue
    const provider = (entry.provider ?? parseEmoteKey(entry.key).provider).toLowerCase()
    if (keys.has(recapEmoteLookupKey(entry.name, provider)) || keys.has(name)) {
      return entry
    }
  }
  return undefined
}

function hasResolvableEmoteUrl(emote: Pick<PulseRecapEmote, 'provider' | 'id' | 'imageUrl'>): boolean {
  const resolved = getEmoteImageUrl({
    provider: emote.provider,
    id: emote.id,
    imageUrl: emote.imageUrl,
  })
  return Boolean(resolved && !isBackendEmoteProxyUrl(resolved))
}

export function enrichRecapEmoteFromCatalog(
  emote: PulseRecapEmote,
  catalog: AnalyticsTopEmote[] | undefined,
): PulseRecapEmote {
  if (!catalog?.length || hasResolvableEmoteUrl(emote)) {
    return emote
  }
  const match = catalogEntryForRecap(emote, catalog)
  if (!match) return emote
  const parsed = parseEmoteKey(match.key)
  const id = match.id?.trim() || parsed.id || emote.id?.trim() || undefined
  const imageUrl = match.imageUrl?.trim() || emote.imageUrl?.trim() || undefined
  const provider = emote.provider ?? match.provider ?? parsed.provider
  if (!id && !imageUrl) return emote
  return {
    ...emote,
    id,
    imageUrl,
    provider: provider === 'unknown' ? emote.provider : provider,
  }
}

export function enrichRecapEmotesFromCatalog(
  emotes: PulseRecapEmote[],
  catalog: AnalyticsTopEmote[] | undefined,
): PulseRecapEmote[] {
  if (!emotes.length) return emotes
  return emotes.map((emote) => enrichRecapEmoteFromCatalog(emote, catalog))
}

export function resolveBurstDisplayEmote(
  emote: PulseRecapEmote,
  catalog?: AnalyticsTopEmote[],
): PulseRecapEmote {
  const enriched = enrichRecapEmoteFromCatalog(emote, catalog)
  const burstCount = emote.count ?? 0
  if (burstCount > 0) {
    return { ...enriched, count: burstCount }
  }
  return enriched
}

function rollupHitsToRecapEmotes(
  hits: ReturnType<typeof topEmotesFromRollup>,
  catalog?: AnalyticsTopEmote[],
): PulseRecapEmote[] {
  return hits.map((hit) => {
    const parsed = parseEmoteKey(hit.key)
    const emote: PulseRecapEmote = {
      code: hit.name,
      count: hit.count,
      provider: hit.provider,
      id: parsed.id || undefined,
      imageUrl: hit.image_url,
    }
    return enrichRecapEmoteFromCatalog(emote, catalog)
  })
}

export function findPeakEmoteMinuteFromRollups(args: {
  rollups: AnalyticsMinuteRollup[]
  streamStartedAt: string
  topEmotesCatalog?: AnalyticsTopEmote[]
}): { emote: PulseRecapEmote; offsetSeconds: number } | null {
  let best: { emote: PulseRecapEmote; offsetSeconds: number; count: number } | null = null

  for (const rollup of args.rollups) {
    const hits = topEmotesFromRollup(rollup, 1, args.topEmotesCatalog)
    if (!hits.length) continue
    const emote = rollupHitsToRecapEmotes(hits, args.topEmotesCatalog)[0]
    if (!best || hits[0].count > best.count) {
      best = {
        emote,
        offsetSeconds: rollupOffsetSeconds(rollup, args.streamStartedAt),
        count: hits[0].count,
      }
    }
  }

  if (!best) return null
  return { emote: best.emote, offsetSeconds: best.offsetSeconds }
}

export function resolveRecapBurstHighlight(args: {
  burst: {
    offsetSeconds: number
    code?: string
    count: number
    provider?: string
  }
  rollups?: AnalyticsMinuteRollup[]
  streamStartedAt?: string
  topEmotesCatalog?: AnalyticsTopEmote[]
}): { emote: PulseRecapEmote; offsetSeconds: number } | null {
  const peak =
    args.rollups?.length && args.streamStartedAt
      ? findPeakEmoteMinuteFromRollups({
          rollups: args.rollups,
          streamStartedAt: args.streamStartedAt,
          topEmotesCatalog: args.topEmotesCatalog,
        })
      : null

  const burstCount = args.burst.count ?? 0
  if (peak && (peak.emote.count ?? 0) > burstCount) {
    return {
      emote: resolveBurstDisplayEmote(peak.emote, args.topEmotesCatalog),
      offsetSeconds: peak.offsetSeconds,
    }
  }

  const burstCode = args.burst.code?.trim()
  if (!burstCode) {
    return peak
      ? {
          emote: resolveBurstDisplayEmote(peak.emote, args.topEmotesCatalog),
          offsetSeconds: peak.offsetSeconds,
        }
      : null
  }

  const base: PulseRecapEmote = {
    code: burstCode,
    count: burstCount,
    provider: args.burst.provider ?? 'seventv',
  }
  return {
    emote: resolveBurstDisplayEmote(base, args.topEmotesCatalog),
    offsetSeconds: args.burst.offsetSeconds,
  }
}

export function resolveRecapDisplayEmotes(
  recapTopEmotes: PulseRecapEmote[],
  sessionCatalog: AnalyticsTopEmote[] | undefined,
  limit = 5,
): PulseRecapEmote[] {
  const enrichedRecap = enrichRecapEmotesFromCatalog(recapTopEmotes, sessionCatalog)
  if (!sessionCatalog?.length) {
    return enrichedRecap.slice(0, limit)
  }

  const merged: PulseRecapEmote[] = []
  const seen = new Set<string>()
  const byCount = [...sessionCatalog].sort((left, right) => right.count - left.count)

  for (const entry of byCount) {
    if (merged.length >= limit) break
    const name = entry.name.trim()
    if (!name) continue
    const key = name.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)

    const recapMatch = enrichedRecap.find((emote) => emote.code.toLowerCase() === key)
    if (recapMatch) {
      merged.push(recapMatch)
      continue
    }

    merged.push(
      enrichRecapEmoteFromCatalog(
        {
          code: name,
          count: entry.count,
          provider: entry.provider ?? parseEmoteKey(entry.key).provider,
          id: entry.id,
          imageUrl: entry.imageUrl,
        },
        sessionCatalog,
      ),
    )
  }

  for (const emote of enrichedRecap) {
    if (merged.length >= limit) break
    const key = emote.code.trim().toLowerCase()
    if (!key || seen.has(key)) continue
    seen.add(key)
    merged.push(emote)
  }

  return merged.slice(0, limit)
}

export function resolveMomentEmotesForOffset(args: {
  moment: PulseRecapMoment
  rollups?: AnalyticsMinuteRollup[]
  streamStartedAt?: string
  heatmapPoints?: ReplayHeatmapPoint[]
  topEmotesCatalog?: AnalyticsTopEmote[]
  limit?: number
}): PulseRecapEmote[] {
  const {
    moment,
    rollups,
    streamStartedAt,
    heatmapPoints,
    topEmotesCatalog,
    limit = 3,
  } = args

  if (moment.topEmotes?.length) {
    return enrichRecapEmotesFromCatalog(moment.topEmotes, topEmotesCatalog).slice(0, limit)
  }

  const rollup =
    rollups?.length && streamStartedAt
      ? findNearestRollupByOffset(rollups, streamStartedAt, moment.offsetSeconds)
      : null

  if (rollup) {
    const fromRollup = topEmotesFromRollup(rollup, limit, topEmotesCatalog)
    if (fromRollup.length > 0) {
      return rollupHitsToRecapEmotes(fromRollup, topEmotesCatalog)
    }
  }

  if (heatmapPoints?.length && rollup) {
    const bucket = normalizeMinuteBucket(rollup.minuteTs)
    const point = heatmapPoints.find(
      (entry) =>
        entry.minuteTs === rollup.minuteTs
        || normalizeMinuteBucket(entry.minuteTs) === bucket,
    )
    if (point?.topEmotes?.length) {
      return point.topEmotes.slice(0, limit).map((emote) => {
        const hit = heatmapEmoteToRollupHit(emote, topEmotesCatalog)
        if (!hit) {
          return enrichRecapEmoteFromCatalog(
            {
              code: emote.name,
              count: emote.count,
              provider: emote.provider,
              id: emote.id,
              imageUrl: emote.imageUrl,
            },
            topEmotesCatalog,
          )
        }
        return rollupHitsToRecapEmotes([hit], topEmotesCatalog)[0]
      })
    }
  }

  return []
}
