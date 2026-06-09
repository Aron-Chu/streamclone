import type { ChannelEmote } from './api'
import { normalizeBrowserOriginUrl } from './config'

export type CaptionToken =
  | { kind: 'text'; value: string }
  | { kind: 'emote'; name: string; url: string }

export function tokenizeCaptionText(text: string, emotes: ChannelEmote[]): CaptionToken[] {
  if (!text) return []
  const lookup = new Map<string, ChannelEmote>()
  for (const emote of emotes) {
    const key = emote.name.trim().toLowerCase()
    if (key && !lookup.has(key)) lookup.set(key, emote)
  }

  const tokens: CaptionToken[] = []
  const parts = text.split(/(\s+)/)
  for (const part of parts) {
    if (!part) continue
    if (/^\s+$/.test(part)) {
      tokens.push({ kind: 'text', value: part })
      continue
    }
    const emote = lookup.get(part.toLowerCase())
    if (emote) {
      tokens.push({
        kind: 'emote',
        name: emote.name,
        url: normalizeBrowserOriginUrl(emote.url, ['/emotes/']),
      })
      continue
    }
    tokens.push({ kind: 'text', value: part })
  }
  return tokens
}

export function CaptionRichText({
  text,
  emotes,
  emoteClassName = 'caption-inline-emote',
}: {
  text: string
  emotes: ChannelEmote[]
  emoteClassName?: string
}) {
  const tokens = tokenizeCaptionText(text, emotes)
  return (
    <>
      {tokens.map((token, index) => {
        if (token.kind === 'emote') {
          return (
            <img
              key={`${token.name}-${index}`}
              src={token.url}
              alt={token.name}
              title={token.name}
              className={emoteClassName}
            />
          )
        }
        return <span key={`text-${index}`}>{token.value}</span>
      })}
    </>
  )
}

export const CAPTION_EMOJIS = [
  '😂', '🔥', '💀', '😭', '👀', '💯', '🙏', '❤️', '😎', '🤣',
  '😱', '🎉', '👍', '👎', '😤', '🤯', '🫡', '✨', '😈', '🥶',
] as const
