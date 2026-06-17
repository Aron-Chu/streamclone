export type SocialPlatform =
  | 'twitch'
  | 'discord'
  | 'x'
  | 'youtube'
  | 'instagram'
  | 'tiktok'
  | 'kick'
  | 'generic'

export interface SocialLinkMeta {
  platform: SocialPlatform
  label: string
  host: string
}

const PLATFORM_LABELS: Record<Exclude<SocialPlatform, 'generic'>, string> = {
  twitch: 'Twitch',
  discord: 'Discord',
  x: 'Twitter',
  youtube: 'YouTube',
  instagram: 'Instagram',
  tiktok: 'TikTok',
  kick: 'Kick',
}

function normalizeHost(hostname: string) {
  return hostname.toLowerCase().replace(/^www\./, '')
}

function detectPlatform(host: string): SocialPlatform {
  if (host === 'twitch.tv' || host.endsWith('.twitch.tv')) return 'twitch'
  if (host === 'discord.com' || host === 'discord.gg' || host.endsWith('.discord.com')) return 'discord'
  if (host === 'x.com' || host === 'twitter.com' || host.endsWith('.x.com') || host.endsWith('.twitter.com')) return 'x'
  if (host === 'youtube.com' || host === 'youtu.be' || host.endsWith('.youtube.com')) return 'youtube'
  if (host === 'instagram.com' || host.endsWith('.instagram.com')) return 'instagram'
  if (host === 'tiktok.com' || host.endsWith('.tiktok.com')) return 'tiktok'
  if (host === 'kick.com' || host.endsWith('.kick.com')) return 'kick'
  return 'generic'
}

export function compactLinkLabel(value: string, max = 28) {
  const trimmed = value.trim()
  if (trimmed.length <= max) return trimmed
  return `${trimmed.slice(0, max - 1)}…`
}

export function resolveSocialLinkMeta(url: string, title?: string): SocialLinkMeta {
  let host = ''
  try {
    host = normalizeHost(new URL(url).hostname)
  } catch {
    return {
      platform: 'generic',
      label: compactLinkLabel(title?.trim() || url),
      host: '',
    }
  }

  const platform = detectPlatform(host)
  if (platform === 'generic') {
    return {
      platform,
      label: compactLinkLabel(title?.trim() || host),
      host,
    }
  }

  return {
    platform,
    label: compactLinkLabel(title?.trim() || PLATFORM_LABELS[platform]),
    host,
  }
}
