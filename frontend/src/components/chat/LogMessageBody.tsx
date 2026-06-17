import { type ReactNode } from 'react'

import type { ChannelEmote } from '../../api'
import { normalizeBrowserOriginUrl } from '../../config'
import { tokenizeEmoteText } from '../../emoteText'
import { resolveEmoteImageUrl } from '../../utils/emoteImageUrl'
import { linkifyText } from '../../utils/linkifyText'

export type LogEmoteFrag = {
  name: string
  id: string
  provider: string
  imageUrl: string
}

export function EmoteStack({
  baseName,
  baseUrl,
  overlays,
}: {
  baseName: string
  baseUrl: string
  overlays: Array<{ name: string; url: string }>
}) {
  const title = [baseName, ...overlays.map(overlay => overlay.name)].join(' ')
  return (
    <span className="relative inline-block align-middle" style={{ height: '1.65em', lineHeight: 0 }} title={title}>
      <img
        src={normalizeBrowserOriginUrl(baseUrl, ['/emotes/'])}
        alt={baseName}
        className="inline-block h-full w-auto max-w-none align-middle drop-shadow"
        decoding="async"
        loading="lazy"
      />
      {overlays.map((overlay, index) => (
        <img
          key={`${overlay.name}-${index}`}
          src={normalizeBrowserOriginUrl(overlay.url, ['/emotes/'])}
          alt={overlay.name}
          title={overlay.name}
          className="pointer-events-none absolute inset-0 z-10 h-full w-full object-contain drop-shadow"
          decoding="async"
          loading="lazy"
        />
      ))}
    </span>
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
      nodes.push(
        <EmoteStack
          key={`${keyPrefix}-e-${index}`}
          baseName={segment.name}
          baseUrl={baseUrl}
          overlays={overlays}
        />,
      )
    } else {
      nodes.push(
        <img
          key={`${keyPrefix}-e-${index}`}
          src={normalizeBrowserOriginUrl(baseUrl, ['/emotes/'])}
          alt={segment.name}
          title={segment.name}
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
