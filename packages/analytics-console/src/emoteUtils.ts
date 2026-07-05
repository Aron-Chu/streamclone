import type { AnalyticsMinuteRollup, AnalyticsTopEmote, ChannelEmote } from './api'

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

export function emoteProviderLabel(provider?: string): string {
  if (provider === 'seventv') return '7TV'
  if (provider === 'twitch') return 'Twitch'
  if (provider === 'ffz') return 'FFZ'
  if (provider === 'bttv') return 'BTTV'
  return provider ? provider.toUpperCase() : 'Emote'
}

export function emoteProviderTone(provider?: string): string {
  if (provider === 'seventv') return 'text-emerald-300 bg-emerald-500/15 border-emerald-500/25'
  if (provider === 'twitch') return 'text-violet-300 bg-violet-500/15 border-violet-500/25'
  if (provider === 'ffz') return 'text-sky-300 bg-sky-500/15 border-sky-500/25'
  if (provider === 'bttv') return 'text-amber-300 bg-amber-500/15 border-amber-500/25'
  return 'text-zinc-400 bg-white/[0.06] border-white/10'
}

export function emoteCountForProvider(rollup: AnalyticsMinuteRollup, provider: EmoteProviderKind): number {
  if (provider === 'seventv' && (rollup.seventvEmoteCount ?? 0) > 0) {
    return rollup.seventvEmoteCount ?? 0
  }
  if (!rollup.emotes) return 0
  let total = 0
  for (const [key, count] of Object.entries(rollup.emotes)) {
    if (parseEmoteKey(key).provider === provider) total += count
  }
  return total
}

export function sortChannelEmotesByUsage(emotes: ChannelEmote[], topEmotes?: AnalyticsTopEmote[]) {
  const usage = new Map<string, number>()
  for (const emote of topEmotes ?? []) {
    const key = emote.name.trim().toLowerCase()
    if (!key) continue
    usage.set(key, (usage.get(key) ?? 0) + emote.count)
  }
  return [...emotes].sort((left, right) => {
    const leftKey = left.name.trim().toLowerCase()
    const rightKey = right.name.trim().toLowerCase()
    const leftCount = usage.get(leftKey) ?? 0
    const rightCount = usage.get(rightKey) ?? 0
    if (rightCount !== leftCount) return rightCount - leftCount
    return left.name.localeCompare(right.name)
  })
}

export function emoteLoadPercent(count: number, total: number, fallback?: number) {
  if (total > 0) return Math.min(100, Math.floor((count / total) * 100))
  return Math.max(0, Math.min(100, fallback ?? 0))
}

export function formatEmoteProviderProgress(input: {
  count?: number
  total?: number
  pending?: number
  failed?: number
  percent?: number
  state: string
  active?: boolean
}) {
  const count = input.count ?? 0
  const total = input.total ?? 0
  const pending = input.pending ?? 0
  const failed = input.failed ?? 0
  if (!total && !count) {
    if (input.state === 'ready') return 'Loaded; no emotes found'
    if (count > 0) return `${count} loaded`
    return input.active ? 'Selected for loading' : 'Not selected'
  }
  const percent = emoteLoadPercent(count, total, input.percent)
  let label = `${count}/${total} (${percent}%)`
  if (pending > 0) label += ` · ${pending} pending`
  if (failed > 0) label += ` · ${failed} failed`
  return label
}
