import type { ExtensionEmoteLike } from './extensionAdapters.ts'
import type { LiveHeatCatalogEmote, LiveHeatEmote, LiveHeatPoint, LiveHeatReason } from './liveHeat.ts'
import { resolveEmoteImageUrl } from './emoteImageUrl.ts'
import { momentScoreReasonLabel } from './momentScore.ts'

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

const KNOWN_REASONS = new Set<LiveHeatReason>([
  'chat_spike',
  'emote_spike',
  'seventv_spike',
  'twitch_emote_spike',
  'ffz_spike',
  'viewer_spike',
  'manual',
])

function minuteTsFromOffset(startedAt: string | undefined, offsetSeconds: number): string {
  if (!startedAt) return ''
  const base = Date.parse(startedAt)
  if (!Number.isFinite(base)) return ''
  return new Date(base + offsetSeconds * 1000).toISOString()
}

function normalizePeakReason(reasons: string[]): LiveHeatReason {
  const first = reasons[0]?.trim()
  if (first && KNOWN_REASONS.has(first as LiveHeatReason)) {
    return first as LiveHeatReason
  }
  return 'manual'
}

function catalogProvider(provider?: string): string | undefined {
  if (!provider) return undefined
  const lower = provider.trim().toLowerCase()
  if (lower === '7tv') return 'seventv'
  return lower
}

function emoteCatalogKey(emote: ExtensionEmoteLike): string {
  const provider = catalogProvider(emote.provider) ?? 'unknown'
  const id = emote.id ?? emote.name
  return `${provider}:${id}:${emote.name}`
}

function buildCatalogMap(catalogEmotes?: ExtensionEmoteLike[]): Map<string, LiveHeatCatalogEmote> {
  const catalog = new Map<string, LiveHeatCatalogEmote>()
  for (const emote of catalogEmotes ?? []) {
    if (!emote.name) continue
    catalog.set(emoteCatalogKey(emote), {
      key: emoteCatalogKey(emote),
      name: emote.name,
      id: emote.id,
      provider: catalogProvider(emote.provider),
      imageUrl: emote.imageUrl,
      count: Math.max(0, emote.count ?? 0),
    })
  }
  return catalog
}

function mapPeakTopEmotes(
  peak: ExtensionPeakLike,
  catalog: Map<string, LiveHeatCatalogEmote>,
): LiveHeatEmote[] {
  const source = peak.topEmotes ?? []
  return source
    .filter(emote => emote.name)
    .slice(0, 3)
    .map(emote => {
      const key = emoteCatalogKey(emote)
      const match = catalog.get(key)
      const provider = match?.provider ?? catalogProvider(emote.provider)
      const id = match?.id ?? emote.id
      const imageUrl = resolveEmoteImageUrl({
        provider,
        id,
        imageUrl: match?.imageUrl ?? emote.imageUrl,
        scale: '1x',
      })
      return {
        key,
        name: match?.name ?? emote.name,
        id,
        provider,
        imageUrl: imageUrl || undefined,
        count: Math.max(0, emote.count ?? 0),
      }
    })
}

/** True when the payload includes an explicit peaks field (even an empty array). */
export function extensionSupportsPeaks(payload: unknown): boolean {
  if (typeof payload !== 'object' || payload === null) return false
  return (payload as { peaks?: unknown }).peaks !== undefined
}

/** Map backend BFF peaks to live heat points for Most Reacted UI. */
export function peaksToLiveHeatPoints(
  peaks: ExtensionPeakLike[],
  startedAt?: string,
  catalogEmotes?: ExtensionEmoteLike[],
): LiveHeatPoint[] {
  const catalog = buildCatalogMap(catalogEmotes)
  return peaks
    .slice()
    .sort((a, b) => b.score - a.score || a.offsetSeconds - b.offsetSeconds)
    .map(peak => {
      const reason = normalizePeakReason(peak.reasons)
      return {
        minuteTs: minuteTsFromOffset(startedAt, peak.offsetSeconds),
        offsetSeconds: peak.offsetSeconds,
        score: Math.round(peak.score),
        estimated: false,
        reason,
        reasonLabel: peak.reasonLabel?.trim() || momentScoreReasonLabel(peak.reasons[0] ?? ''),
        chatCount: Math.max(0, Math.round(peak.chatCount ?? 0)),
        emoteCount: Math.max(0, Math.round(peak.emoteCount ?? 0)),
        topEmotes: mapPeakTopEmotes(peak, catalog),
        collecting: false,
      }
    })
}
