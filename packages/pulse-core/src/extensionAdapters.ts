import type {
  LiveHeatCatalogEmote,
  LiveHeatEmote,
  LiveHeatInput,
  LiveHeatPoint,
  LiveHeatReason,
  LiveHeatRollup,
} from './liveHeat.ts'
import { LIVE_HEAT_MAX_EMOTES } from './liveHeat.ts'
import { displayMomentReasonLabel } from './momentScore.ts'
import { resolveEmoteImageUrl } from './emoteImageUrl.ts'
import type { LiveStatsInput, LiveStatsRollup, LiveTopEmote } from './liveStats.ts'

export interface ExtensionEmoteLike {
  id?: string
  name: string
  imageUrl?: string
  count: number
  provider?: string
}

export interface ExtensionRollupLike {
  offsetSeconds: number
  chatCount?: number
  sevenTvEmoteCount?: number
  totalEmoteCount?: number
  viewerCount?: number
  topEmotes?: ExtensionEmoteLike[]
  missing?: boolean
}

export interface ExtensionPeakLike {
  offsetSeconds: number
  score: number
  reasons: string[]
  reasonLabel?: string
  dominantSignal?: string
  chatCount?: number
  emoteCount?: number
  topEmotes?: ExtensionEmoteLike[]
}

export interface ExtensionPulseLike {
  isLive: boolean
  startedAt?: string
  topEmotes?: ExtensionEmoteLike[]
  rollups: ExtensionRollupLike[]
  fullRollups?: ExtensionRollupLike[]
  peaks?: ExtensionPeakLike[]
}

/** True when the backend sent a peaks field (including an empty warming array). */
export function extensionSupportsPeaks(payload: ExtensionPulseLike | null): boolean {
  return payload != null && payload.peaks !== undefined
}

function peakReasonToLiveHeatReason(reason: string | undefined): LiveHeatReason {
  const normalized = (reason ?? 'manual').trim() as LiveHeatReason
  const allowed: LiveHeatReason[] = [
    'chat_spike',
    'emote_spike',
    'seventv_spike',
    'twitch_emote_spike',
    'ffz_spike',
    'viewer_spike',
    'manual',
  ]
  return allowed.includes(normalized) ? normalized : 'manual'
}

function peakTopEmotesToLiveHeatEmotes(
  peak: ExtensionPeakLike,
  catalog: Map<string, LiveHeatCatalogEmote>,
  byName: Map<string, LiveHeatCatalogEmote>,
): LiveHeatEmote[] {
  return (peak.topEmotes ?? [])
    .filter(emote => emote.name)
    .slice(0, LIVE_HEAT_MAX_EMOTES)
    .map(emote => {
      const key = emoteCatalogKey(emote)
      const match = catalog.get(key) ?? byName.get(emote.name.toLowerCase())
      const provider = catalogProvider(emote.provider) ?? match?.provider
      const id = emote.id ?? match?.id
      const imageUrl = resolveEmoteImageUrl({
        provider,
        id,
        imageUrl: emote.imageUrl ?? match?.imageUrl,
        scale: '1x',
      })
      return {
        key,
        name: emote.name,
        id,
        provider: displayProvider(emote.provider) ?? match?.provider,
        imageUrl: imageUrl || undefined,
        count: Math.max(0, emote.count ?? 0),
      }
    })
}

/** Map BFF extension peaks to ranked LiveHeatPoint rows (backend scores, not estimated). */
export function peaksToLiveHeatPoints(
  peaks: ExtensionPeakLike[],
  startedAt?: string,
  catalogEmotes?: ExtensionEmoteLike[],
): LiveHeatPoint[] {
  const catalog = new Map<string, LiveHeatCatalogEmote>(
    (catalogEmotes ?? [])
      .filter(emote => emote.name)
      .map(emote => [emoteCatalogKey(emote), extensionEmoteToCatalogEmote(emote)]),
  )
  const byName = new Map<string, LiveHeatCatalogEmote>(
    (catalogEmotes ?? [])
      .filter(emote => emote.name)
      .map(emote => [emote.name.toLowerCase(), extensionEmoteToCatalogEmote(emote)]),
  )

  return peaks.map(peak => {
    const reasonCode = peak.reasons[0]
    const reason = peakReasonToLiveHeatReason(reasonCode)
    const reasonLabel = displayMomentReasonLabel(reasonCode ?? '', peak.reasonLabel)
    return {
      minuteTs: minuteTsFromOffset(startedAt, peak.offsetSeconds) ?? '',
      offsetSeconds: Math.max(0, peak.offsetSeconds),
      score: Math.round(peak.score),
      estimated: false,
      reason,
      reasonLabel,
      chatCount: Math.max(0, Math.round(peak.chatCount ?? 0)),
      emoteCount: Math.max(0, Math.round(peak.emoteCount ?? 0)),
      topEmotes: peakTopEmotesToLiveHeatEmotes(peak, catalog, byName),
      collecting: false,
    }
  })
}

/** Rollups used for live stats / Most Reacted — prefer recent window, fall back to full. */
export function extensionRollupsForDerivation(payload: ExtensionPulseLike | null): ExtensionRollupLike[] {
  if (!payload) return []
  if (payload.rollups.length > 0) return payload.rollups
  return payload.fullRollups ?? []
}

function catalogProvider(provider?: string): string | undefined {
  if (!provider) return undefined
  const lower = provider.trim().toLowerCase()
  if (lower === '7tv') return 'seventv'
  return lower
}

function displayProvider(provider?: string): string | undefined {
  if (!provider) return undefined
  const lower = provider.trim().toLowerCase()
  if (lower === '7tv' || lower === 'seventv') return '7TV'
  if (lower === 'twitch') return 'Twitch'
  if (lower === 'ffz') return 'FFZ'
  return provider
}

function emoteCatalogKey(emote: ExtensionEmoteLike): string {
  const provider = catalogProvider(emote.provider) ?? 'unknown'
  const id = emote.id ?? emote.name
  return `${provider}:${id}:${emote.name}`
}

function minuteTsFromOffset(startedAt: string | undefined, offsetSeconds: number): string | undefined {
  if (!startedAt) return undefined
  const base = Date.parse(startedAt)
  if (!Number.isFinite(base)) return undefined
  return new Date(base + offsetSeconds * 1000).toISOString()
}

function rollupTopEmotesToEmotesMap(topEmotes?: ExtensionEmoteLike[]): Record<string, number> {
  if (!topEmotes?.length) return {}
  const map: Record<string, number> = {}
  for (const emote of topEmotes) {
    const key = emoteCatalogKey(emote)
    map[key] = (map[key] ?? 0) + Math.max(0, emote.count ?? 0)
  }
  return map
}

function isSevenTvProvider(provider?: string): boolean {
  if (!provider) return false
  const lower = provider.trim().toLowerCase()
  return lower === '7tv' || lower === 'seventv'
}

function extensionRollupToStatsRollup(r: ExtensionRollupLike, startedAt?: string): LiveStatsRollup {
  const viewerCount = r.viewerCount ?? 0
  let totalEmoteCount = r.totalEmoteCount
  let seventvEmoteCount = r.sevenTvEmoteCount

  if ((totalEmoteCount ?? 0) === 0 && (r.topEmotes?.length ?? 0) > 0) {
    totalEmoteCount = r.topEmotes!.reduce((sum, emote) => sum + Math.max(0, emote.count ?? 0), 0)
    seventvEmoteCount = r.topEmotes!
      .filter(emote => isSevenTvProvider(emote.provider))
      .reduce((sum, emote) => sum + Math.max(0, emote.count ?? 0), 0)
  }
  if ((totalEmoteCount ?? 0) === 0 && (seventvEmoteCount ?? 0) > 0) {
    totalEmoteCount = seventvEmoteCount
  }

  return {
    minuteTs: minuteTsFromOffset(startedAt, r.offsetSeconds),
    chatCount: r.chatCount,
    totalEmoteCount,
    seventvEmoteCount,
    viewerLatest: viewerCount > 0 ? viewerCount : undefined,
    viewerSamples: viewerCount > 0 ? 1 : undefined,
    missing: r.missing,
  }
}

function extensionRollupToHeatRollup(r: ExtensionRollupLike, startedAt?: string): LiveHeatRollup {
  const viewerCount = r.viewerCount ?? 0
  return {
    minuteTs: minuteTsFromOffset(startedAt, r.offsetSeconds),
    chatCount: r.chatCount,
    totalEmoteCount: r.totalEmoteCount,
    seventvEmoteCount: r.sevenTvEmoteCount,
    emotes: rollupTopEmotesToEmotesMap(r.topEmotes),
    viewerSamples: viewerCount > 0 ? 1 : 0,
    missing: r.missing,
  }
}

export function aggregateTopEmotesFromExtensionRollups(
  rollups: ExtensionRollupLike[],
  limit = 3,
): LiveTopEmote[] {
  const byKey = new Map<string, LiveTopEmote>()
  for (const rollup of rollups) {
    for (const emote of rollup.topEmotes ?? []) {
      if (!emote.name) continue
      const key = emoteCatalogKey(emote)
      const existing = byKey.get(key)
      const count = Math.max(0, emote.count ?? 0)
      if (existing) {
        byKey.set(key, { ...existing, count: existing.count + count })
        continue
      }
      byKey.set(key, {
        key,
        name: emote.name,
        id: emote.id,
        provider: displayProvider(emote.provider),
        imageUrl: emote.imageUrl,
        count,
      })
    }
  }
  return [...byKey.values()]
    .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name))
    .slice(0, limit)
}

function extensionEmoteToLiveTopEmote(emote: ExtensionEmoteLike): LiveTopEmote {
  return {
    key: emoteCatalogKey(emote),
    name: emote.name,
    id: emote.id,
    provider: displayProvider(emote.provider),
    imageUrl: emote.imageUrl,
    count: emote.count,
  }
}

function extensionEmoteToCatalogEmote(emote: ExtensionEmoteLike): LiveHeatCatalogEmote {
  return {
    key: emoteCatalogKey(emote),
    name: emote.name,
    id: emote.id,
    provider: catalogProvider(emote.provider),
    imageUrl: emote.imageUrl,
    count: emote.count,
  }
}

/** Map extension pulse payload fields to web LiveStatsBand input. */
export function toLiveStatsInputFromExtension(payload: ExtensionPulseLike | null): LiveStatsInput {
  const rollupsSource = extensionRollupsForDerivation(payload)
  if (!payload || rollupsSource.length === 0) {
    return { state: payload?.isLive ? 'live' : 'historical', rollups: [] }
  }

  const rollups = rollupsSource.map(r => extensionRollupToStatsRollup(r, payload.startedAt))
  const topEmotesSource =
    (payload.topEmotes ?? []).length > 0
      ? payload.topEmotes!.map(extensionEmoteToLiveTopEmote)
      : aggregateTopEmotesFromExtensionRollups(
          rollupsSource.filter(r => (r.topEmotes?.length ?? 0) > 0),
        )

  return {
    state: payload.isLive ? 'live' : 'historical',
    rollups,
    topEmotes: topEmotesSource,
  }
}

/** Map extension pulse payload fields to web MostReactedLive input. */
export function toLiveHeatInputFromExtension(payload: ExtensionPulseLike | null): LiveHeatInput {
  if (!payload) {
    return { state: 'not_collected', rollups: [] }
  }

  const rollupsSource = extensionRollupsForDerivation(payload)

  return {
    state: payload.isLive ? 'live' : 'historical',
    rollups: rollupsSource.map(r => extensionRollupToHeatRollup(r, payload.startedAt)),
    topEmotes: (payload.topEmotes ?? []).map(extensionEmoteToCatalogEmote),
    streamStartedAt: payload.startedAt,
  }
}
