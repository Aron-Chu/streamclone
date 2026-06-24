import type { HeatmapEmote } from './types/heatmap'
import { parseEmoteKey } from './emoteKey'
import { resolveEmoteImageUrl } from './emoteImageUrl'
import { clampMomentScore } from './momentScore'

export type MomentScoringRollup = {
  minuteTs?: string
  missing?: boolean
  viewerSamples?: number
  viewerLatest?: number
  viewerAvg?: number
  viewerMax?: number
  chatCount?: number
  totalEmoteCount?: number
  emotes?: Record<string, number>
}

export type CatalogTopEmote = {
  key: string
  name: string
  id?: string
  provider?: string
  imageUrl?: string
  count: number
}

function viewerValue(point: MomentScoringRollup) {
  return point.viewerLatest || point.viewerAvg || point.viewerMax || 0
}

function minuteEmoteTotal(point: MomentScoringRollup) {
  const total = point.totalEmoteCount ?? 0
  if (total > 0) return total
  if (!point.emotes) return 0
  return Object.values(point.emotes).reduce((sum, count) => sum + count, 0)
}

function rollupHasMinuteData(point: MomentScoringRollup) {
  return !point.missing && (
    (point.viewerSamples ?? 0) > 0
    || viewerValue(point) > 0
    || (point.chatCount ?? 0) > 0
    || minuteEmoteTotal(point) > 0
  )
}

export type MomentReason =
  | 'viewer_spike'
  | 'chat_spike'
  | 'emote_spike'
  | 'seventv_spike'
  | 'twitch_emote_spike'
  | 'ffz_spike'
  | 'manual'

export type StreamBaselines = { chat: number; emotes: number; viewers: number }

export type RollupEmoteHit = {
  key: string
  name: string
  provider?: string
  count: number
  image_url?: string
}

function getEmoteImageUrl(emote: { provider?: string; id?: string; imageUrl?: string }) {
  const url = resolveEmoteImageUrl({
    provider: emote.provider,
    id: emote.id,
    imageUrl: emote.imageUrl,
    scale: '1x',
  })
  return url || undefined
}

export function computeStreamBaselines(rollups: MomentScoringRollup[]): StreamBaselines {
  const data = rollups.filter(point => !point.missing && rollupHasMinuteData(point))
  if (!data.length) return { chat: 1, emotes: 1, viewers: 1 }
  return {
    chat: data.reduce((sum, point) => sum + (point.chatCount ?? 0), 0) / data.length || 1,
    emotes: data.reduce((sum, point) => sum + minuteEmoteTotal(point), 0) / data.length || 1,
    viewers: data.reduce((sum, point) => sum + viewerValue(point), 0) / data.length || 1,
  }
}

export function topEmotesFromRollup(
  rollup: MomentScoringRollup,
  limit = 5,
  catalog?: CatalogTopEmote[],
): RollupEmoteHit[] {
  if (!rollup.emotes) return []
  const byKey = new Map(catalog?.map(item => [item.key, item]) ?? [])
  const byName = new Map(catalog?.map(item => [item.name.toLowerCase(), item]) ?? [])
  return Object.entries(rollup.emotes)
    .sort((a, b) => b[1] - a[1])
    .slice(0, limit)
    .map(([key, count]) => {
      const parsed = parseEmoteKey(key)
      const match = byKey.get(key) ?? byName.get(parsed.name.toLowerCase())
      return {
        key,
        name: match?.name ?? parsed.name,
        provider: match?.provider ?? (parsed.provider !== 'unknown' ? parsed.provider : undefined),
        count,
        image_url: match
          ? getEmoteImageUrl(match)
          : (parsed.id ? getEmoteImageUrl({ provider: parsed.provider, id: parsed.id }) : undefined),
      }
    })
}

export function detectPickReason(
  rollup: MomentScoringRollup,
  baselines: StreamBaselines,
  catalog?: CatalogTopEmote[],
): MomentReason {
  const chatMult = (rollup.chatCount ?? 0) / baselines.chat
  const emoteMult = minuteEmoteTotal(rollup) / baselines.emotes
  const viewerMult = viewerValue(rollup) / baselines.viewers
  if (chatMult >= 2 && chatMult >= emoteMult) return 'chat_spike'
  if (emoteMult >= 2) {
    const top = topEmotesFromRollup(rollup, 1, catalog)[0]
    if (top?.provider === 'seventv') return 'seventv_spike'
    if (top?.provider === 'twitch') return 'twitch_emote_spike'
    if (top?.provider === 'ffz') return 'ffz_spike'
    return 'emote_spike'
  }
  if (viewerMult >= 1.5) return 'viewer_spike'
  return 'manual'
}

function percentile(values: number[], p: number): number {
  if (!values.length) return 1
  const sorted = [...values].sort((a, b) => a - b)
  const idx = Math.min(sorted.length - 1, Math.max(0, Math.ceil(p * sorted.length) - 1))
  return sorted[idx] || 1
}

function computeMomentScore(
  rollup: MomentScoringRollup,
  baselines: StreamBaselines,
  rollups?: MomentScoringRollup[],
): number {
  const chatNorm = Math.min(1, (rollup.chatCount ?? 0) / Math.max(baselines.chat * 2, 1))
  const emoteNorm = Math.min(1, minuteEmoteTotal(rollup) / Math.max(baselines.emotes * 2, 1))
  const viewerNorm = Math.min(1, viewerValue(rollup) / Math.max(baselines.viewers * 1.5, 1))

  const topEmotes = topEmotesFromRollup(rollup, 3)
  const emoteTotal = Math.max(1, minuteEmoteTotal(rollup))
  const keywordNorm = topEmotes.length > 0
    ? Math.min(1, topEmotes[0].count / (emoteTotal * 0.4))
    : 0

  let noveltyNorm = 0.5
  if (rollups?.length) {
    const idx = rollups.findIndex(point => point.minuteTs === rollup.minuteTs)
    if (idx > 0) {
      const prior = rollups.slice(Math.max(0, idx - 5), idx).filter(point => !point.missing)
      if (prior.length > 0) {
        const priorChat = prior.reduce((sum, point) => sum + (point.chatCount ?? 0), 0) / prior.length
        const delta = (rollup.chatCount ?? 0) - priorChat
        noveltyNorm = Math.min(1, Math.max(0, delta / Math.max(baselines.chat, 1)))
      }
    }
  }

  const weighted =
    chatNorm * 0.35 +
    emoteNorm * 0.25 +
    viewerNorm * 0.15 +
    keywordNorm * 0.15 +
    noveltyNorm * 0.10
  return weighted * 10
}

export function computeMomentScore100(
  rollup: MomentScoringRollup,
  baselines: StreamBaselines,
  rollups?: MomentScoringRollup[],
): number {
  return clampMomentScore(computeMomentScore(rollup, baselines, rollups) * 10)
}

export function computeP95ReactionScore100(
  rollup: MomentScoringRollup,
  rollups: MomentScoringRollup[],
): number {
  const chatValues = rollups.map(r => Math.max(0, r.chatCount ?? 0))
  const emoteValues = rollups.map(r => minuteEmoteTotal(r))
  const chatP95 = percentile(chatValues, 0.95)
  const emoteP95 = percentile(emoteValues, 0.95)
  const chatNorm = Math.min(1, (rollup.chatCount ?? 0) / Math.max(chatP95, 1))
  const emoteNorm = Math.min(1, minuteEmoteTotal(rollup) / Math.max(emoteP95, 1))
  const weighted = chatNorm * 0.6 + emoteNorm * 0.4
  return Math.round(clampMomentScore(weighted * 100))
}

export function fallbackMomentScore100(
  rollup: MomentScoringRollup,
  baselines: StreamBaselines,
  rollups: MomentScoringRollup[],
): number {
  const multi = computeMomentScore100(rollup, baselines, rollups)
  const p95 = computeP95ReactionScore100(rollup, rollups)
  return Math.round(Math.min(multi, p95))
}

export function heatmapEmotesFromRollup(
  rollup: MomentScoringRollup,
  limit = 5,
  catalog?: CatalogTopEmote[],
): HeatmapEmote[] {
  return topEmotesFromRollup(rollup, limit, catalog).map(emote => ({
    id: emote.key,
    name: emote.name,
    imageUrl: emote.image_url ?? '',
    count: emote.count,
    provider: emote.provider ?? 'unknown',
  }))
}

export function peakEmoteCount(rollup: MomentScoringRollup): number {
  if (!rollup.emotes) return 0
  return Math.max(0, ...Object.values(rollup.emotes))
}
