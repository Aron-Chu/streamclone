export function formatEmoteProviderLabel(provider?: string): string | undefined {
  const normalized = (provider ?? '').trim().toLowerCase()
  if (!normalized) return undefined
  if (normalized === 'seventv' || normalized === '7tv') return '7TV'
  if (normalized === 'twitch') return 'Twitch'
  if (normalized === 'ffz' || normalized === 'frankerfacez') return 'FFZ'
  if (normalized === 'bttv' || normalized === 'betterttv') return 'BTTV'
  if (normalized === 'custom') return undefined
  return provider?.trim() || undefined
}

export function formatEmoteTooltipLabel(name?: string, provider?: string, fallbackId?: string): string {
  const trimmedName = (name ?? '').trim()
  const label = trimmedName || (fallbackId ?? '').trim() || 'Emote'
  const providerLabel = formatEmoteProviderLabel(provider)
  return providerLabel ? `${label} · ${providerLabel}` : label
}

export function formatEmoteStackTooltipLabel(
  baseName: string,
  baseProvider: string | undefined,
  overlays: Array<{ name: string; provider?: string }>,
): string {
  const parts = [formatEmoteTooltipLabel(baseName, baseProvider)]
  for (const overlay of overlays) {
    parts.push(formatEmoteTooltipLabel(overlay.name, overlay.provider))
  }
  return parts.join(' ')
}
