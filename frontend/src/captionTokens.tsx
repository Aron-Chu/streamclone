import type { ChannelEmote } from './api'
import { normalizeBrowserOriginUrl } from './config'
import { tokenizeEmoteText } from './emoteText'

export type CaptionToken =
  | { kind: 'text'; value: string }
  | { kind: 'emote'; name: string; url: string }

export function tokenizeCaptionText(text: string, emotes: ChannelEmote[]): CaptionToken[] {
  return tokenizeEmoteText(text, emotes).map(segment => {
    if (segment.kind === 'text') return { kind: 'text', value: segment.value }
    return {
      kind: 'emote',
      name: segment.name,
      url: normalizeBrowserOriginUrl(segment.emote.url, ['/emotes/']),
    }
  })
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
