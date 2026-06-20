// Display helpers shared by Pulse Wire dashboard + streamer profile.
// Purely presentational; nullable inputs render an honest em dash.

import type { PulseWireRankModel, PulseWireWindow } from '../pulseWireApi'

export function formatViewers(value?: number | null): string {
  if (value == null || !Number.isFinite(value)) return '—'
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(value >= 10_000_000 ? 0 : 1)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(value >= 10_000 ? 0 : 1)}K`
  return String(Math.round(value))
}

export function formatDeltaPct(value?: number | null): string {
  if (value == null || !Number.isFinite(value)) return '—'
  const rounded = Math.round(value)
  return `${rounded > 0 ? '+' : ''}${rounded}%`
}

export function formatRankDelta(value?: number | null): string {
  if (value == null || !Number.isFinite(value) || value === 0) return '—'
  return `${value > 0 ? '▲' : '▼'}${Math.abs(Math.round(value))}`
}

export function deltaTone(value?: number | null): string {
  if (value == null || !Number.isFinite(value) || value === 0) return 'text-[#7A7A85]'
  return value > 0 ? 'text-[#3FCB7E]' : 'text-[#FF5C57]'
}

export function windowLabel(window: PulseWireWindow): string {
  switch (window) {
    case 'today':
      return 'Today'
    case '7d':
      return '7 days'
    default:
      return '24 hours'
  }
}

export function windowShortLabel(window: PulseWireWindow): string {
  switch (window) {
    case 'today':
      return 'today'
    case '7d':
      return '7d'
    default:
      return '24h'
  }
}

export function windowEditionTitle(window: PulseWireWindow, mode: 'trending' | 'wire' = 'wire'): string {
  if (mode === 'trending') {
    return "What's hot in streaming culture"
  }
  switch (window) {
    case 'today':
      return 'Breaking cross-platform edition'
    case '7d':
      return 'Weekly cross-platform recap'
    default:
      return 'Cross-platform edition'
  }
}

export function windowTagline(window: PulseWireWindow, mode: 'trending' | 'wire' = 'wire'): string {
  if (mode === 'trending') {
    return 'Reddit, clips, and partner bans — open the source, no corroboration checklist.'
  }
  switch (window) {
    case 'today':
      return 'Stories spreading across multiple sources since UTC midnight.'
    case '7d':
      return 'Multi-source stories and cumulative spread over the last seven days.'
    default:
      return 'Stories spreading across multiple sources in the last 24 hours.'
  }
}

export function pulseWireModeSubtitle(mode: 'trending' | 'wire'): string {
  return mode === 'wire'
    ? 'Cross-platform stories with attached receipts.'
    : 'Streaming culture feed — Reddit, clips, and bans.'
}

export function rankModelLabel(rankModel?: PulseWireRankModel | string | null): string {
  if (!rankModel) return 'Window rank'
  switch (rankModel) {
    case 'breaking':
      return 'Breaking rank'
    case 'daily_wire':
      return 'Daily wire rank'
    case 'weekly_recap':
      return 'Weekly impact rank'
    case 'legacy':
      return 'Legacy trend rank'
    default:
      return rankModel.replace(/_/g, ' ')
  }
}

export function formatSince(iso?: string | null, window?: PulseWireWindow): string {
  if (!iso) {
    return window ? `Since ${windowLabel(window).toLowerCase()} start` : 'Since window start'
  }
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return 'Since window start'
  return `Since ${date.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    timeZoneName: 'short',
  })}`
}

export function formatRelativeTime(iso?: string | null, nowMs = Date.now()): string {
  if (!iso) return '—'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '—'
  const deltaMs = nowMs - date.getTime()
  if (deltaMs < 0) return 'just now'
  const minutes = Math.floor(deltaMs / 60_000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 7) return `${days}d ago`
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

export function formatTimelineTime(iso?: string | null): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
}

export function formatCompactCount(value?: number | null): string {
  if (value == null || Number.isNaN(value)) return '—'
  return new Intl.NumberFormat(undefined, {
    notation: value >= 10_000 ? 'compact' : 'standard',
    maximumFractionDigits: 1,
  }).format(value)
}

/** Engagement counts treat 0 as unknown (repair backlog) rather than literal zero. */
export function formatEngagementCount(value?: number | null): string {
  if (value == null || Number.isNaN(value) || value <= 0) return '—'
  return formatCompactCount(value)
}

export function hasEngagementCounts(score?: number | null, comments?: number | null): boolean {
  return (score ?? 0) > 0 || (comments ?? 0) > 0
}
