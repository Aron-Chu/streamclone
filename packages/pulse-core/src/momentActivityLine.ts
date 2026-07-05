import type { LiveHeatPoint, LiveHeatReason } from './liveHeat.ts'

export type MomentActivityLineInput = Pick<
  LiveHeatPoint,
  'reason' | 'chatCount' | 'emoteCount' | 'viewerCount' | 'viewerDelta'
>

function formatCompactCount(value: number): string {
  return new Intl.NumberFormat('en-US', {
    notation: value >= 10_000 ? 'compact' : 'standard',
    maximumFractionDigits: 1,
  }).format(Math.max(0, value))
}

function formatViewerDeltaLabel(delta: number): string {
  if (delta === 0) return 'no change'
  const prefix = delta > 0 ? '+' : ''
  return `${prefix}${formatCompactCount(delta)} viewers`
}

function reactionParts(chatCount: number, emoteCount: number): string[] {
  const parts: string[] = []
  if (chatCount > 0) parts.push(`${formatCompactCount(chatCount)} chat`)
  if (emoteCount > 0) parts.push(`${formatCompactCount(emoteCount)} emotes`)
  return parts
}

function shouldLeadWithViewers(input: MomentActivityLineInput): boolean {
  const chat = input.chatCount ?? 0
  const emotes = input.emoteCount ?? 0
  const viewers = input.viewerCount ?? 0
  if (viewers <= 0) return false
  if (input.reason === 'viewer_spike') return true
  return chat + emotes === 0
}

/** Human-readable activity line for selected moment cards and inspectors. */
export function formatMomentActivityLine(input: MomentActivityLineInput): string {
  const chat = Math.max(0, input.chatCount ?? 0)
  const emotes = Math.max(0, input.emoteCount ?? 0)
  const viewers = Math.max(0, input.viewerCount ?? 0)
  const parts: string[] = []

  if (shouldLeadWithViewers(input)) {
    if (input.viewerDelta != null && Number.isFinite(input.viewerDelta) && input.viewerDelta !== 0) {
      parts.push(formatViewerDeltaLabel(input.viewerDelta))
    } else {
      parts.push(`${formatCompactCount(viewers)} viewers`)
    }
  }

  parts.push(...reactionParts(chat, emotes))

  if (parts.length === 0) {
    return '0 chat · 0 emotes'
  }
  return parts.join(' · ')
}

export function isViewerSpikeReason(reason: LiveHeatReason | string | undefined): boolean {
  const normalized = (reason ?? '').trim().toLowerCase()
  if (normalized === 'viewer_spike') return true
  return normalized.includes('viewer') && normalized.includes('spike')
}
