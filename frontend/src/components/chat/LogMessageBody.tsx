import { type ReactNode } from 'react'

import type { ChannelEmote } from '../../api'
import { normalizeBrowserOriginUrl } from '../../config'
import { tokenizeEmoteText } from '../../emoteText'
import { resolveEmoteImageUrl } from '../../utils/emoteImageUrl'
import { linkifyText } from '../../utils/linkifyText'
import { ChatEmoteImage, ChatEmoteStack } from './ChatEmoteTooltipLayer'

export type LogEmoteFrag = {
  name: string
  id: string
  provider: string
  imageUrl: string
}

export function EmoteStack({
  baseName,
  baseUrl,
  baseProvider,
  baseId,
  overlays,
}: {
  baseName: string
  baseUrl: string
  baseProvider?: string
  baseId?: string
  overlays: Array<{ name: string; url: string; provider?: string; id?: string }>
}) {
  return (
    <ChatEmoteStack
      baseName={baseName}
      baseUrl={normalizeBrowserOriginUrl(baseUrl, ['/emotes/'])}
      baseProvider={baseProvider}
      baseId={baseId}
      overlays={overlays.map(overlay => ({
        ...overlay,
        url: normalizeBrowserOriginUrl(overlay.url, ['/emotes/']),
      }))}
    />
  )
}

function fragsToChannelEmotes(frags: LogEmoteFrag[]): ChannelEmote[] {
  return frags.map(frag => ({
    name: frag.name,
    emote_id: frag.id,
    url: frag.imageUrl,
    zw: false,
    provider: (frag.provider || 'custom') as ChannelEmote['provider'],
  }))
}

function emoteImageUrl(frag: LogEmoteFrag) {
  return resolveEmoteImageUrl({
    provider: frag.provider,
    id: frag.id,
    imageUrl: frag.imageUrl,
    scale: '1x',
  })
}

function renderEmoteSegments(segments: ReturnType<typeof tokenizeEmoteText>, frags: LogEmoteFrag[], keyPrefix: string): ReactNode[] {
  const fragByName = new Map(frags.map(frag => [frag.name, frag]))
  const nodes: ReactNode[] = []
  let index = 0

  while (index < segments.length) {
    const segment = segments[index]
    if (segment.kind === 'text') {
      nodes.push(<span key={`${keyPrefix}-t-${index}`}>{linkifyText(segment.value, `${keyPrefix}-t-${index}`)}</span>)
      index++
      continue
    }

    const baseFrag = fragByName.get(segment.name) ?? frags.find(frag => frag.name === segment.name)
    const baseUrl = baseFrag ? emoteImageUrl(baseFrag) : segment.emote.url
    const overlays: Array<{ name: string; url: string }> = []
    let next = index + 1
    while (next < segments.length) {
      const overlaySegment = segments[next]
      if (overlaySegment.kind !== 'emote' || !overlaySegment.emote.zw) break
      const overlayFrag = fragByName.get(overlaySegment.name)
      overlays.push({
        name: overlaySegment.name,
        url: overlayFrag ? emoteImageUrl(overlayFrag) : overlaySegment.emote.url,
      })
      next++
    }

    if (overlays.length) {
      const baseFrag = fragByName.get(segment.name)
      nodes.push(
        <EmoteStack
          key={`${keyPrefix}-e-${index}`}
          baseName={segment.name}
          baseUrl={baseUrl}
          baseProvider={baseFrag?.provider ?? segment.emote.provider}
          baseId={baseFrag?.id ?? segment.emote.emote_id}
          overlays={overlays.map(overlay => {
            const overlayFrag = fragByName.get(overlay.name)
            return {
              ...overlay,
              provider: overlayFrag?.provider,
              id: overlayFrag?.id,
            }
          })}
        />,
      )
    } else {
      const frag = fragByName.get(segment.name)
      nodes.push(
        <ChatEmoteImage
          key={`${keyPrefix}-e-${index}`}
          src={normalizeBrowserOriginUrl(baseUrl, ['/emotes/'])}
          name={segment.name}
          provider={frag?.provider ?? segment.emote.provider}
          fallbackId={frag?.id ?? segment.emote.emote_id}
          className="inline-block align-middle drop-shadow"
          style={{ height: '1.65em' }}
          decoding="async"
          loading="lazy"
        />,
      )
    }
    index = next
  }

  return nodes
}

export function LogMessageBody({
  text,
  emoteFrags,
  keyPrefix,
}: {
  text?: string
  emoteFrags?: LogEmoteFrag[]
  keyPrefix: string
}) {
  const body = text ?? ''
  if (!emoteFrags?.length) {
    return <span>{linkifyText(body, keyPrefix)}</span>
  }

  const segments = tokenizeEmoteText(body, fragsToChannelEmotes(emoteFrags))
  return <span>{renderEmoteSegments(segments, emoteFrags, keyPrefix)}</span>
}

export default LogMessageBody
