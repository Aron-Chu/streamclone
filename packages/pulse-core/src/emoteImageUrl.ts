const TWITCH_CDN_TEMPLATE = 'https://static-cdn.jtvnw.net/emoticons/v2/%s/default/dark/2.0'
const SEVEN_TV_CDN_TEMPLATE = 'https://cdn.7tv.app/emote/%s/4x.webp'
const FFZ_CDN_TEMPLATE = 'https://cdn.frankerfacez.com/emoticon/%s/4'
const BTTV_CDN_TEMPLATE = 'https://cdn.betterttv.net/emote/%s/3x'

const LOCAL_EMOTE_ID = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/

export function localEmotePath(id: string, scale = '1x'): string {
  const resolvedScale = scale.trim() || '1x'
  return `/emotes/${id}/${resolvedScale}.webp`
}

export function isLocalEmoteUuid(id: string): boolean {
  return LOCAL_EMOTE_ID.test(id.trim())
}

export function isBrokenLocalEmotePath(url: string): boolean {
  const match = url.match(/^\/emotes\/([^/]+)\//)
  if (!match) return false
  return !isLocalEmoteUuid(match[1])
}

export interface ResolveEmoteImageUrlOptions {
  provider?: string
  id?: string
  imageUrl?: string
  scale?: string
}

export function resolveEmoteImageUrl(opts: ResolveEmoteImageUrlOptions): string {
  const scale = opts.scale?.trim() || '1x'
  const id = opts.id?.trim() ?? ''
  const imageUrl = opts.imageUrl?.trim()

  if (imageUrl && !isBrokenLocalEmotePath(imageUrl)) {
    return imageUrl
  }
  if (!id) {
    return imageUrl ?? ''
  }

  const provider = (opts.provider ?? '').trim().toLowerCase()

  switch (provider) {
    case 'twitch':
      return TWITCH_CDN_TEMPLATE.replace('%s', id)
    case 'seventv':
    case '7tv':
      return isLocalEmoteUuid(id) ? localEmotePath(id, scale) : SEVEN_TV_CDN_TEMPLATE.replace('%s', id)
    case 'ffz':
    case 'frankerfacez':
      return isLocalEmoteUuid(id) ? localEmotePath(id, scale) : FFZ_CDN_TEMPLATE.replace('%s', id)
    case 'bttv':
    case 'betterttv':
      return isLocalEmoteUuid(id) ? localEmotePath(id, scale) : BTTV_CDN_TEMPLATE.replace('%s', id)
    default:
      return localEmotePath(id, scale)
  }
}
