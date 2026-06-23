export type EmoteProviderKind = 'seventv' | 'twitch' | 'ffz' | 'bttv' | 'unknown'

export function parseEmoteKey(key: string): { provider: EmoteProviderKind; id: string; name: string } {
  const parts = key.split(':')
  if (parts.length >= 3) {
    const provider = parts[0].toLowerCase()
    const normalized: EmoteProviderKind =
      provider === 'seventv' || provider === 'twitch' || provider === 'ffz' || provider === 'bttv' ? provider : 'unknown'
    return { provider: normalized, id: parts[1], name: parts.slice(2).join(':') }
  }
  return { provider: 'unknown', id: '', name: key }
}
