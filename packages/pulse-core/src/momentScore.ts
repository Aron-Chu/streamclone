import type { HeatmapEmote, ReplayHeatmapDetailPoint, ReplayHeatmapPoint } from './types/heatmap.ts'

const REASON_LABELS: Record<string, string> = {
  chat_spike: 'Chat spike',
  seventv_spike: '7TV emote spike',
  twitch_emote_spike: 'Twitch emote spike',
  ffz_spike: 'FFZ emote spike',
  viewer_spike: 'Viewer spike',
  emote_spike: 'Emote spike',
  game_change: 'Game change',
  manual: 'Moment',
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
