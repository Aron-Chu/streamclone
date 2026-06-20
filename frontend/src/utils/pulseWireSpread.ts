import type {
  PulseWireChannelSpreadMeta,
  PulseWireChannelSpreadResponse,
  PulseWireMatchExplanation,
} from '../pulseWireApi.ts'

function pulseWireLabel(value: string) {
  return value.replace(/_/g, ' ')
}

export function formatChannelMatchReason(match?: PulseWireMatchExplanation): string | undefined {
  if (!match) return undefined
  if (match.factors?.length) return `Matched: ${match.factors.join(' · ')}`
  const parts = [pulseWireLabel(match.sourceType), pulseWireLabel(match.matchedBy)].filter(Boolean)
  return parts.length ? `Matched: ${parts.join(' · ')}` : undefined
}

export function matchConfidenceLabel(confidence?: number): 'High' | 'Medium' | 'Low' | undefined {
  if (confidence == null || !Number.isFinite(confidence)) return undefined
  if (confidence >= 0.8) return 'High'
  if (confidence >= 0.5) return 'Medium'
  return 'Low'
}

export function normalizeChannelSpreadResponse(raw: PulseWireChannelSpreadResponse): PulseWireChannelSpreadResponse {
  return {
    ...raw,
    items: raw.items ?? [],
    probableItems: raw.probableItems ?? [],
    meta: raw.meta ?? {},
  }
}

export function isSpreadBackfillWarming(meta?: PulseWireChannelSpreadMeta): boolean {
  return meta?.backfill?.state === 'warming'
}
