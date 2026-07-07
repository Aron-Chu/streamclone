import type { HeatmapEmote, ReplayHeatmapDetailPoint, ReplayHeatmapPoint } from './types/heatmap.ts'

const REASON_LABELS: Record<string, string> = {
  chat_spike: 'Chat spike',
  seventv_spike: 'Emote spike',
  twitch_emote_spike: 'Emote spike',
  ffz_spike: 'Emote spike',
  bttv_spike: 'Emote spike',
  viewer_spike: 'Viewer spike',
  emote_spike: 'Emote spike',
  game_change: 'Game change',
  manual: 'Moment',
}

const EMOTE_SPIKE_REASONS = new Set([
  'emote_spike',
  'seventv_spike',
  'twitch_emote_spike',
  'ffz_spike',
  'bttv_spike',
])

export function isEmoteSpikeReason(reason: string): boolean {
  const normalized = reason.trim().toLowerCase()
  if (EMOTE_SPIKE_REASONS.has(normalized)) return true
  return normalized.includes('emote') && normalized.includes('spike')
}

/** User-facing spike label — provider-specific codes collapse to Emote spike. */
export function displayMomentReasonLabel(reason: string, reasonLabel?: string): string {
  if (isEmoteSpikeReason(reason)) return REASON_LABELS.emote_spike
  const label = reasonLabel?.trim()
  if (label && label.toLowerCase().includes('emote spike')) return REASON_LABELS.emote_spike
  if (reason.trim()) return momentScoreReasonLabel(reason)
  return label || REASON_LABELS.manual
}

export interface MomentScoreModel {
  score: number
  label: string
  reason: string
  reasonLabel: string
  confidence: number | null
  estimated: boolean
  topEmotes: HeatmapEmote[]
  detailComponents: Array<{
    key: string
    rawScore: number
    weightedScore: number
    confidence: number
  }>
}

export interface MomentScoreInput {
  heatmapPoint?: ReplayHeatmapPoint | null
  heatmapDetail?: ReplayHeatmapDetailPoint | null
  fallbackScore100: number
  fallbackReason: string
  fallbackTopEmotes?: HeatmapEmote[]
}

export function clampMomentScore(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.max(0, Math.min(100, value))
}

export function momentScoreReasonLabel(reason: string): string {
  const normalized = reason.trim()
  if (!normalized) return 'Moment'
  return REASON_LABELS[normalized] ?? normalized.replace(/_/g, ' ')
}

export function buildMomentScoreModel(input: MomentScoreInput): MomentScoreModel {
  const backendPoint = input.heatmapDetail ?? input.heatmapPoint ?? null
  const hasBackendScore = backendPoint && Number.isFinite(backendPoint.score)
  const score = clampMomentScore(hasBackendScore ? backendPoint.score : input.fallbackScore100)
  const reason = backendPoint?.reason || input.fallbackReason || 'manual'
  const estimated = !hasBackendScore
  const components = input.heatmapDetail?.components
    ? Object.entries(input.heatmapDetail.components)
        .map(([key, component]) => ({ key, ...component }))
        .filter(component =>
          Number.isFinite(component.rawScore)
          && Number.isFinite(component.weightedScore)
          && Number.isFinite(component.confidence),
        )
        .sort((a, b) => b.weightedScore - a.weightedScore)
    : []

  return {
    score,
    label: `${estimated ? '~' : ''}${Math.round(score)}/100`,
    reason,
    reasonLabel: momentScoreReasonLabel(reason),
    confidence: backendPoint && Number.isFinite(backendPoint.confidence) ? backendPoint.confidence : null,
    estimated,
    topEmotes: backendPoint?.topEmotes?.length ? backendPoint.topEmotes : (input.fallbackTopEmotes ?? []),
    detailComponents: components,
  }
}
