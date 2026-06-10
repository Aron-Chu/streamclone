import type { ChannelEmote } from './api'

export type EmoteTextSegment =
  | { kind: 'text'; value: string }
  | { kind: 'emote'; name: string; emote: ChannelEmote }

export function buildEmoteLookup(emotes: ChannelEmote[]): Map<string, ChannelEmote> {
  const map = new Map<string, ChannelEmote>()
  for (const emote of emotes) {
    if (emote.name) map.set(emote.name, emote)
  }
  return map
}

export function matchZeroWidthChain(emoteMap: Map<string, ChannelEmote>, text: string) {
  const runes = [...text]
  const parts: Array<{ name: string; emote: ChannelEmote }> = []
  let remaining = runes
  while (remaining.length > 0) {
    let bestLen = 0
    let best: ChannelEmote | undefined
    for (let size = 1; size <= remaining.length; size++) {
      const name = remaining.slice(0, size).join('')
      const emote = emoteMap.get(name)
      if (emote?.zw) {
        bestLen = size
        best = emote
      }
    }
    if (bestLen === 0 || !best) return null
    parts.push({ name: remaining.slice(0, bestLen).join(''), emote: best })
    remaining = remaining.slice(bestLen)
  }
  return parts
}

export function splitZeroWidthSuffix(emoteMap: Map<string, ChannelEmote>, word: string) {
  const runes = [...word]
  for (let baseLen = runes.length - 1; baseLen > 0; baseLen--) {
    const base = runes.slice(0, baseLen).join('')
    if (!emoteMap.has(base)) continue
    const overlays = matchZeroWidthChain(emoteMap, runes.slice(baseLen).join(''))
    if (overlays) return { base, overlays }
  }
  return null
}

/** Tokenize caption/chat text with zero-width emote combo support. */
export function tokenizeEmoteText(text: string, emotes: ChannelEmote[]): EmoteTextSegment[] {
  if (!text) return []
  if (!emotes.length) return [{ kind: 'text', value: text }]

  const emoteMap = buildEmoteLookup(emotes)
  const segments: EmoteTextSegment[] = []
  const runes = [...text]
  let index = 0

  const flushText = (value: string) => {
    if (value) segments.push({ kind: 'text', value })
  }

  let pending = ''
  while (index < runes.length) {
    if (runes[index] === ' ') {
      let end = index
      while (end < runes.length && runes[end] === ' ') end++
      pending += runes.slice(index, end).join('')
      index = end
      continue
    }

    let end = index
    while (end < runes.length && runes[end] !== ' ') end++
    const word = runes.slice(index, end).join('')

    const emote = emoteMap.get(word)
    if (emote) {
      if (emote.zw && segments.length > 0 && segments[segments.length - 1].kind === 'emote' && !pending.trim()) {
        pending = ''
      }
      flushText(pending)
      pending = ''
      segments.push({ kind: 'emote', name: word, emote })
    } else {
      const split = splitZeroWidthSuffix(emoteMap, word)
      if (split) {
        flushText(pending)
        pending = ''
        segments.push({ kind: 'emote', name: split.base, emote: emoteMap.get(split.base)! })
        for (const overlay of split.overlays) {
          segments.push({ kind: 'emote', name: overlay.name, emote: overlay.emote })
        }
      } else {
        pending += word
      }
    }
    index = end
  }
  flushText(pending)
  return segments
}
