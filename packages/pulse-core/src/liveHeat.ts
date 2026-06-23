import type { ReplayHeatmapPoint } from './types/heatmap.ts'
import { resolveEmoteImageUrl } from './emoteImageUrl.ts'
import { buildMomentScoreModel } from './momentScore.ts'
import {
  computeStreamBaselines,
  detectPickReason,
  fallbackMomentScore100,
  type StreamBaselines,
} from './momentScoring.ts'

export const LIVE_HEAT_REFRESH_MS = 30000
export const LIVE_HEAT_MIN_COMPLETED_ROLLUPS = 5
export const LIVE_HEAT_MAX_POINTS = 10
export const LIVE_HEAT_MAX_EMOTES = 3
export const LIVE_HEAT_SUBTITLE = 'based on chat and emote activity'
export const LIVE_HEAT_RANKED_SUBTITLE = 'Ranked by reaction score'
export const LIVE_HEAT_TITLE = 'Most Reacted So Far'
export const LIVE_HEAT_COLLECTING_LABEL = 'Collecting'

export type LiveHeatStreamState = 'live' | 'historical' | 'not_collected' | 'syncing'

export interface LiveHeatRollup {
  minuteTs?: string
  viewerSamples?: number
  chatCount?: number
  totalEmoteCount?: number
  seventvEmoteCount?: number
  emotes?: Record<string, number>
  missing?: boolean
}

export interface LiveHeatCatalogEmote {
  key?: string
  name: string
  id?: string
  provider?: string
  imageUrl?: string
  count: number
}

export interface LiveHeatEmote {
  key: string
  name: string
  id?: string
  provider?: string
  imageUrl?: string
  count: number
}

export type LiveHeatReason =
  | 'chat_spike'
  | 'emote_spike'
  | 'seventv_spike'
  | 'twitch_emote_spike'
  | 'ffz_spike'
  | 'viewer_spike'
  | 'manual'

export interface LiveHeatPoint {
  minuteTs: string
  offsetSeconds: number
  score: number
  estimated: boolean
  reason: LiveHeatReason
  reasonLabel: string
  chatCount: number
  emoteCount: number
  topEmotes: LiveHeatEmote[]
  collecting: boolean
}

export interface LiveHeatInput {
  state: LiveHeatStreamState
  rollups: LiveHeatRollup[]
  topEmotes?: LiveHeatCatalogEmote[]
  heatmapPoints?: ReplayHeatmapPoint[]
  streamStartedAt?: string
}

export interface LiveHeatResult {
  visible: boolean
  completedRollupCount: number
  points: LiveHeatPoint[]
  collectingPoint: LiveHeatPoint | null
  subtitle: string
}

const REASON_LABELS: Record<LiveHeatReason, string> = {
  chat_spike: 'Chat spike',
  emote_spike: 'Emote spike',
  seventv_spike: '7TV emote spike',
  twitch_emote_spike: 'Twitch emote spike',
  ffz_spike: 'FFZ emote spike',
  viewer_spike: 'Viewer spike',
  manual: 'Moment',
}

function isDataRollup(r: LiveHeatRollup): boolean {
  if (r.missing) return false
  return (
    (r.viewerSamples ?? 0) > 0 ||
    (r.chatCount ?? 0) > 0 ||
    (r.totalEmoteCount ?? 0) > 0
  )
}

function emoteTotalOf(r: LiveHeatRollup): number {
  const total = r.totalEmoteCount ?? 0
  if (total > 0) return total
  if (r.emotes) {
    return Object.values(r.emotes).reduce((sum, n) => sum + Math.max(0, n), 0)
  }
  return 0
}

function parseEmoteKey(key: string): { provider?: string; id?: string; name: string } {
  const parts = key.split(':')
  if (parts.length >= 3) {
    const [provider, id, ...rest] = parts
    return { provider, id, name: rest.join(':') || key }
  }
  if (parts.length === 2) {
    return { provider: parts[0], name: parts[1] }
  }
  return { name: key }
}

function topEmotesFromRollup(
  r: LiveHeatRollup,
  catalog: Map<string, LiveHeatCatalogEmote>,
  byName: Map<string, LiveHeatCatalogEmote>,
  limit = LIVE_HEAT_MAX_EMOTES,
): LiveHeatEmote[] {
  if (!r.emotes) return []
  return Object.entries(r.emotes)
    .filter(([, count]) => count > 0)
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .slice(0, limit)
    .map(([key, count]) => {
      const parsed = parseEmoteKey(key)
      const match = catalog.get(key) ?? byName.get(parsed.name.toLowerCase())
      const id = match?.id ?? parsed.id
      const provider = match?.provider ?? (parsed.provider && parsed.provider !== 'unknown' ? parsed.provider : undefined)
      const imageUrl = resolveEmoteImageUrl({
        provider,
        id,
        imageUrl: match?.imageUrl,
        scale: '1x',
      })
      return {
        key,
        name: match?.name ?? parsed.name,
        id,
        provider,
        imageUrl: imageUrl || undefined,
        count,
      }
    })
}

function liveHeatCatalogAsTopEmotes(catalog: LiveHeatCatalogEmote[]) {
  return catalog
    .filter((emote): emote is LiveHeatCatalogEmote & { key: string } => Boolean(emote.key))
    .map(emote => ({
      key: emote.key,
      name: emote.name,
      id: emote.id,
      provider: emote.provider,
      imageUrl: emote.imageUrl,
      count: emote.count,
    }))
}

function scoreLiveHeatPoint(
  r: LiveHeatRollup,
  baselines: StreamBaselines,
  completedRollups: LiveHeatRollup[],
  heatmapPointMap: Map<string, ReplayHeatmapPoint> | null,
  catalog: LiveHeatCatalogEmote[],
): { score: number; estimated: boolean; reason: LiveHeatReason; reasonLabel: string } {
  const fallbackReason = detectPickReason(r, baselines, liveHeatCatalogAsTopEmotes(catalog))
  const scoreModel = buildMomentScoreModel({
    heatmapPoint: r.minuteTs ? heatmapPointMap?.get(r.minuteTs) : undefined,
    fallbackScore100: fallbackMomentScore100(r, baselines, completedRollups),
    fallbackReason,
  })
  const reason = (scoreModel.reason in REASON_LABELS
    ? scoreModel.reason
    : fallbackReason) as LiveHeatReason
  return {
    score: Math.round(scoreModel.score),
    estimated: scoreModel.estimated,
    reason,
    reasonLabel: scoreModel.reasonLabel,
  }
}

function parseMinuteMs(minuteTs: string | undefined): number {
  if (!minuteTs) return Number.NaN
  return Date.parse(minuteTs)
}

function filterRankedHeatPoints(points: LiveHeatPoint[]): LiveHeatPoint[] {
  if (!points.length) return []
  const maxScore = Math.max(...points.map(point => point.score))
  if (maxScore <= 0) return []
  const cutoff = Math.max(1, Math.round(maxScore * 0.25))
  return points
    .filter(point => point.score >= cutoff)
    .sort((a, b) => b.score - a.score || a.offsetSeconds - b.offsetSeconds)
    .slice(0, LIVE_HEAT_MAX_POINTS)
}

function buildPoint(
  r: LiveHeatRollup,
  baselines: StreamBaselines,
  completedRollups: LiveHeatRollup[],
  heatmapPointMap: Map<string, ReplayHeatmapPoint> | null,
  firstMs: number,
  catalog: Map<string, LiveHeatCatalogEmote>,
  byName: Map<string, LiveHeatCatalogEmote>,
  catalogList: LiveHeatCatalogEmote[],
  collecting: boolean,
  streamStartedMs?: number,
): LiveHeatPoint {
  const topEmotes = topEmotesFromRollup(r, catalog, byName)
  const scored = scoreLiveHeatPoint(r, baselines, completedRollups, heatmapPointMap, catalogList)
  const minuteMs = parseMinuteMs(r.minuteTs)
  const anchorMs = Number.isFinite(streamStartedMs) ? streamStartedMs! : firstMs
  const offsetSeconds =
    Number.isFinite(minuteMs) && Number.isFinite(anchorMs)
      ? Math.max(0, Math.round((minuteMs - anchorMs) / 1000))
      : 0
  return {
    minuteTs: r.minuteTs ?? '',
    offsetSeconds,
    score: scored.score,
    estimated: scored.estimated,
    reason: scored.reason,
    reasonLabel: scored.reasonLabel,
    chatCount: Math.max(0, Math.round(r.chatCount ?? 0)),
    emoteCount: Math.max(0, Math.round(emoteTotalOf(r))),
    topEmotes,
    collecting,
  }
}

export function deriveLiveHeat(input: LiveHeatInput): LiveHeatResult {
  const isLive = input.state === 'live' || input.state === 'syncing'
  const catalog = new Map<string, LiveHeatCatalogEmote>(
    (input.topEmotes ?? []).filter(e => e.key).map(e => [e.key as string, e]),
  )
  const byName = new Map<string, LiveHeatCatalogEmote>(
    (input.topEmotes ?? []).map(e => [e.name.toLowerCase(), e]),
  )

  const dataRollups = (input.rollups ?? [])
    .filter(isDataRollup)
    .slice()
    .sort((a, b) => parseMinuteMs(a.minuteTs) - parseMinuteMs(b.minuteTs))

  let completed = dataRollups
  let collectingRollup: LiveHeatRollup | null = null
  if (isLive && dataRollups.length > 0) {
    collectingRollup = dataRollups[dataRollups.length - 1]
    completed = dataRollups.slice(0, -1)
  }

  const baselineRollups = completed.length ? completed : dataRollups
  const baselines = computeStreamBaselines(baselineRollups)
  const heatmapPointMap = input.heatmapPoints?.length
    ? new Map(input.heatmapPoints.map(point => [point.minuteTs, point]))
    : null
  const catalogList = input.topEmotes ?? []
  const firstMs = parseMinuteMs(dataRollups[0]?.minuteTs)
  const streamStartedMs = parseMinuteMs(input.streamStartedAt)

  const collectingPoint = collectingRollup
    ? buildPoint(
        collectingRollup,
        baselines,
        completed,
        heatmapPointMap,
        firstMs,
        catalog,
        byName,
        catalogList,
        true,
        streamStartedMs,
      )
    : null

  const visible = completed.length >= LIVE_HEAT_MIN_COMPLETED_ROLLUPS

  const points = visible
    ? filterRankedHeatPoints(
        completed.map(r => buildPoint(
          r,
          baselines,
          completed,
          heatmapPointMap,
          firstMs,
          catalog,
          byName,
          catalogList,
          false,
          streamStartedMs,
        )),
      )
    : []

  return {
    visible,
    completedRollupCount: completed.length,
    points,
    collectingPoint,
    subtitle: LIVE_HEAT_SUBTITLE,
  }
}

export function formatHeatOffset(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds))
  const hh = Math.floor(s / 3600)
  const mm = Math.floor((s % 3600) / 60)
  const ss = s % 60
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${pad(hh)}:${pad(mm)}:${pad(ss)}`
}
