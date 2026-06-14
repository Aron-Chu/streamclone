// Activity waveform derivation from analytics minute rollups (Part B).
//
// Pure helpers that turn AnalyticsMinuteRollup rows into layered activity
// series (chat/min, 7TV, Twitch, FFZ, total emotes), per-stream normalization,
// and local peak detection. Kept out of React so the Node test runner can
// cover normalization and peak logic without rendering.

import type { EmoteProviderKind } from '../emoteUtils.ts'

/** Layer identifiers rendered in the activity waveform. */
export type ActivityWaveformLayerId =
  | 'chat'
  | 'seventv'
  | 'twitch'
  | 'ffz'
  | 'total_emotes'

export const ACTIVITY_WAVEFORM_LAYER_ORDER: ActivityWaveformLayerId[] = [
  'chat',
  'total_emotes',
  'seventv',
  'twitch',
  'ffz',
]

export interface ActivityWaveformLayerMeta {
  id: ActivityWaveformLayerId
  label: string
  color: string
}

export const ACTIVITY_WAVEFORM_LAYERS: ActivityWaveformLayerMeta[] = [
  { id: 'chat', label: 'Chat/min', color: '#a78bfa' },
  { id: 'total_emotes', label: 'Emotes/min', color: '#fbbf24' },
  { id: 'seventv', label: '7TV/min', color: '#34d399' },
  { id: 'twitch', label: 'Twitch/min', color: '#c084fc' },
  { id: 'ffz', label: 'FFZ/min', color: '#38bdf8' },
]

export const ACTIVITY_WAVEFORM_LAYER_PREFS_KEY = 'streamclone.activityLayers'

/** Maximum ranked peaks returned for click/highlight affordances. */
export const ACTIVITY_WAVEFORM_MAX_PEAKS = 12
/** Canvas draw cap for long VODs — hover/click still use full minute resolution. */
export const ACTIVITY_WAVEFORM_MAX_DRAW_POINTS = 480

/** Minimum combined normalized score for a minute to qualify as a peak. */
export const ACTIVITY_WAVEFORM_PEAK_MIN_SCORE = 0.35

/** Minimal rollup shape (mirrors AnalyticsMinuteRollup). */
export interface ActivityWaveformRollup {
  minuteTs?: string
  chatCount?: number
  totalEmoteCount?: number
  seventvEmoteCount?: number
  emotes?: Record<string, number>
  missing?: boolean
}

export type ActivityWaveformLayerValues = Record<ActivityWaveformLayerId, number>
export type ActivityWaveformLayerVisibility = Record<ActivityWaveformLayerId, boolean>

export interface ActivityWaveformPoint {
  minuteTs: string
  offsetSeconds: number
  index: number
  raw: ActivityWaveformLayerValues
  normalized: ActivityWaveformLayerValues
}

export interface ActivityWaveformPeak {
  minuteTs: string
  offsetSeconds: number
  index: number
  score: number
  dominantLayer: ActivityWaveformLayerId
  raw: ActivityWaveformLayerValues
}

export interface ActivityWaveformResult {
  points: ActivityWaveformPoint[]
  peaks: ActivityWaveformPeak[]
  layerMax: ActivityWaveformLayerValues
  totalDurationSec: number
  hasData: boolean
  emptyReason?: string
}

function parseEmoteProvider(key: string): EmoteProviderKind {
  const provider = key.split(':')[0]?.toLowerCase() ?? ''
  if (provider === 'seventv' || provider === 'twitch' || provider === 'ffz' || provider === 'bttv') {
    return provider
  }
  return 'unknown'
}

function emoteTotalOf(rollup: ActivityWaveformRollup): number {
  const total = rollup.totalEmoteCount ?? 0
  if (total > 0) return total
  if (!rollup.emotes) return 0
  return Object.values(rollup.emotes).reduce((sum, count) => sum + Math.max(0, count), 0)
}

function emoteCountForProvider(rollup: ActivityWaveformRollup, provider: EmoteProviderKind): number {
  if (provider === 'seventv' && (rollup.seventvEmoteCount ?? 0) > 0) {
    return rollup.seventvEmoteCount ?? 0
  }
  if (!rollup.emotes) return 0
  let total = 0
  for (const [key, count] of Object.entries(rollup.emotes)) {
    if (parseEmoteProvider(key) === provider) total += Math.max(0, count)
  }
  return total
}

function emptyLayerValues(): ActivityWaveformLayerValues {
  return { chat: 0, seventv: 0, twitch: 0, ffz: 0, total_emotes: 0 }
}

function isDataRollup(rollup: ActivityWaveformRollup): boolean {
  if (rollup.missing) return false
  return (
    (rollup.chatCount ?? 0) > 0
    || emoteTotalOf(rollup) > 0
    || (rollup.seventvEmoteCount ?? 0) > 0
  )
}

/** Raw per-layer value for one rollup minute. */
export function layerValueForRollup(
  rollup: ActivityWaveformRollup,
  layer: ActivityWaveformLayerId,
): number {
  switch (layer) {
    case 'chat':
      return Math.max(0, rollup.chatCount ?? 0)
    case 'seventv':
      return emoteCountForProvider(rollup, 'seventv')
    case 'twitch':
      return emoteCountForProvider(rollup, 'twitch')
    case 'ffz':
      return emoteCountForProvider(rollup, 'ffz')
    case 'total_emotes':
      return emoteTotalOf(rollup)
    default:
      return 0
  }
}

/** Default layer visibility — chat/min on; emote overlays off until toggled. */
export function defaultLayerVisibility(): ActivityWaveformLayerVisibility {
  return {
    chat: true,
    seventv: false,
    twitch: false,
    ffz: false,
    total_emotes: false,
  }
}

/** Read persisted layer toggles from localStorage (SSR-safe). */
export function loadLayerPrefs(): ActivityWaveformLayerVisibility {
  const defaults = defaultLayerVisibility()
  if (typeof window === 'undefined' || !window.localStorage) return defaults
  try {
    const raw = window.localStorage.getItem(ACTIVITY_WAVEFORM_LAYER_PREFS_KEY)
    if (!raw) return defaults
    const parsed = JSON.parse(raw) as Partial<ActivityWaveformLayerVisibility>
    return { ...defaults, ...parsed }
  } catch {
    return defaults
  }
}

/** Persist layer toggles to localStorage (SSR-safe). */
export function saveLayerPrefs(prefs: ActivityWaveformLayerVisibility): void {
  if (typeof window === 'undefined' || !window.localStorage) return
  try {
    window.localStorage.setItem(ACTIVITY_WAVEFORM_LAYER_PREFS_KEY, JSON.stringify(prefs))
  } catch {
    // Ignore quota / privacy mode failures.
  }
}

function computeLayerMax(points: ActivityWaveformPoint[]): ActivityWaveformLayerValues {
  const max = emptyLayerValues()
  for (const point of points) {
    for (const layer of ACTIVITY_WAVEFORM_LAYER_ORDER) {
      max[layer] = Math.max(max[layer], point.raw[layer])
    }
  }
  for (const layer of ACTIVITY_WAVEFORM_LAYER_ORDER) {
    if (max[layer] <= 0) max[layer] = 1
  }
  return max
}

/** Normalize a raw layer value against the stream max for that layer. */
export function normalizeLayerValue(value: number, layerMax: number): number {
  const max = Math.max(1, layerMax)
  return Math.min(1, Math.max(0, value / max))
}

function combinedScore(
  normalized: ActivityWaveformLayerValues,
  visibility: ActivityWaveformLayerVisibility,
): number {
  let sum = 0
  let count = 0
  for (const layer of ACTIVITY_WAVEFORM_LAYER_ORDER) {
    if (!visibility[layer]) continue
    sum += normalized[layer]
    count += 1
  }
  return count > 0 ? sum / count : 0
}

function dominantLayer(
  normalized: ActivityWaveformLayerValues,
  visibility: ActivityWaveformLayerVisibility,
): ActivityWaveformLayerId {
  let best: ActivityWaveformLayerId = 'chat'
  let bestValue = -1
  for (const layer of ACTIVITY_WAVEFORM_LAYER_ORDER) {
    if (!visibility[layer]) continue
    if (normalized[layer] > bestValue) {
      bestValue = normalized[layer]
      best = layer
    }
  }
  return best
}

function combinedRawScore(values: ActivityWaveformLayerValues): number {
  return values.chat + values.total_emotes
}

/** Downsample minute points for canvas paint on 6h+ streams without changing click mapping. */
export function bucketActivityPointsForDraw(
  points: ActivityWaveformPoint[],
  maxBuckets = ACTIVITY_WAVEFORM_MAX_DRAW_POINTS,
): ActivityWaveformPoint[] {
  if (points.length <= maxBuckets) return points
  const bucketed: ActivityWaveformPoint[] = []
  const bucketSize = points.length / maxBuckets
  for (let bucket = 0; bucket < maxBuckets; bucket += 1) {
    const start = Math.floor(bucket * bucketSize)
    const end = Math.min(points.length, Math.floor((bucket + 1) * bucketSize))
    if (start >= end) continue
    let best = points[start]
    let bestScore = combinedRawScore(best.raw)
    for (let i = start + 1; i < end; i += 1) {
      const candidate = points[i]
      const score = combinedRawScore(candidate.raw)
      if (score >= bestScore) {
        bestScore = score
        best = candidate
      }
    }
    bucketed.push(best)
  }
  return bucketed
}

/**
 * Detect local maxima in the combined normalized activity signal.
 * Peaks must exceed ACTIVITY_WAVEFORM_PEAK_MIN_SCORE and beat neighbours.
 */
export function detectActivityPeaks(
  points: ActivityWaveformPoint[],
  visibility: ActivityWaveformLayerVisibility = defaultLayerVisibility(),
  maxPeaks = ACTIVITY_WAVEFORM_MAX_PEAKS,
): ActivityWaveformPeak[] {
  if (points.length < 3) return []

  const scores = points.map(point => combinedScore(point.normalized, visibility))
  const candidates: ActivityWaveformPeak[] = []

  for (let i = 1; i < points.length - 1; i++) {
    const score = scores[i]
    if (score < ACTIVITY_WAVEFORM_PEAK_MIN_SCORE) continue
    if (score < scores[i - 1] || score < scores[i + 1]) continue
    const point = points[i]
    candidates.push({
      minuteTs: point.minuteTs,
      offsetSeconds: point.offsetSeconds,
      index: point.index,
      score: Math.round(score * 100),
      dominantLayer: dominantLayer(point.normalized, visibility),
      raw: { ...point.raw },
    })
  }

  return candidates
    .sort((a, b) => b.score - a.score || a.offsetSeconds - b.offsetSeconds)
    .slice(0, maxPeaks)
    .sort((a, b) => a.offsetSeconds - b.offsetSeconds)
}

/**
 * Build layered activity waveform data from minute rollups.
 * Offset seconds are index * 60 from the first rollup row (consistent with Analytics chart).
 */
export function deriveActivityWaveform(
  rollups: ActivityWaveformRollup[],
  visibility: ActivityWaveformLayerVisibility = defaultLayerVisibility(),
): ActivityWaveformResult {
  const dataRollups = rollups.filter(isDataRollup)
  if (!dataRollups.length) {
    return {
      points: [],
      peaks: [],
      layerMax: emptyLayerValues(),
      totalDurationSec: Math.max(0, rollups.length * 60),
      hasData: false,
      emptyReason: rollups.length === 0 ? 'No rollup data' : 'Sync chat to see activity',
    }
  }

  const points: ActivityWaveformPoint[] = rollups.map((rollup, index) => {
    const raw = emptyLayerValues()
    for (const layer of ACTIVITY_WAVEFORM_LAYER_ORDER) {
      raw[layer] = rollup.missing ? 0 : layerValueForRollup(rollup, layer)
    }
    return {
      minuteTs: rollup.minuteTs ?? '',
      offsetSeconds: index * 60,
      index,
      raw,
      normalized: { ...raw },
    }
  })

  const layerMax = computeLayerMax(points)
  for (const point of points) {
    for (const layer of ACTIVITY_WAVEFORM_LAYER_ORDER) {
      point.normalized[layer] = normalizeLayerValue(point.raw[layer], layerMax[layer])
    }
  }

  const peaks = detectActivityPeaks(points, visibility)

  return {
    points,
    peaks,
    layerMax,
    totalDurationSec: Math.max(60, rollups.length * 60),
    hasData: true,
  }
}

/** Map a waveform peak layer to a heatmap-style reason string. */
export function peakLayerToReason(layer: ActivityWaveformLayerId): string {
  switch (layer) {
    case 'chat':
      return 'chat_spike'
    case 'seventv':
      return 'seventv_spike'
    case 'twitch':
      return 'twitch_emote_spike'
    case 'ffz':
      return 'ffz_spike'
    case 'total_emotes':
      return 'chat_spike'
    default:
      return 'manual'
  }
}
