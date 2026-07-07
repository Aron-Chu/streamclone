import type { AnalyticsTopEmote, PulseRecapEmote } from '../apiTypes.ts'
import { parseEmoteKey } from '../emoteUtils.ts'
import { getEmoteImageUrl } from './consoleFormat.ts'

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

export function enrichRecapEmoteFromCatalog(
  emote: PulseRecapEmote,
  catalog: AnalyticsTopEmote[] | undefined,
): PulseRecapEmote {
  const resolved = getEmoteImageUrl({
    provider: emote.provider,
    id: emote.id,
    imageUrl: emote.imageUrl,
  })
  if (!catalog?.length || resolved) {
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
