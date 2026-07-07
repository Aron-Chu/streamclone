import { parseEmoteKey } from '@streamclone/pulse-core'
import type { ChartMinuteRollup } from './types.ts'
import { emoteChartColor } from './chartTheme.ts'
import {
  chartViewerValue,
  minuteEmoteTotal,
  seriesMax,
} from './chartRollupUtils.ts'

export type ChartSeries = {
  key: string
  label: string
  color: string
  values: Array<number | null>
  max: number
  dashed?: boolean
}

function emoteLabel(key: string) {
  const parts = key.split(':')
  if (parts.length >= 3) return parts.slice(2).join(':')
  return key
}

function resolveEmoteMinuteCount(rollup: ChartMinuteRollup, key: string): number {
  if (rollup.missing) return 0
  const direct = rollup.emotes?.[key]
  if (direct != null && direct > 0) return direct
  const targetName = parseEmoteKey(key).name.trim().toLowerCase()
  if (!targetName || !rollup.emotes) return 0
  for (const [rollupKey, count] of Object.entries(rollup.emotes)) {
    if (count <= 0) continue
    if (rollupKey === key) return count
    if (parseEmoteKey(rollupKey).name.trim().toLowerCase() === targetName) return count
  }
  return 0
}

export function buildChartSeries(
  rollups: ChartMinuteRollup[],
  selected: Set<string>,
  peakViewersFallback = 0,
  avgViewersFallback = 0,
  useViewerFallback = false,
): ChartSeries[] {
  const viewerBaseline = avgViewersFallback > 0 ? avgViewersFallback : peakViewersFallback
  const viewers = rollups.map(point => {
    if (point.missing) return null
    const value = chartViewerValue(point)
    if (value > 0) return value
    return useViewerFallback && viewerBaseline > 0 ? viewerBaseline : 0
  })
  const chat = rollups.map(point => (point.missing ? null : (point.chatCount ?? null)))
  const emotesRaw = rollups.map(point => (point.missing ? null : minuteEmoteTotal(point)))
  const emotesMax = seriesMax(emotesRaw)
  const viewersMax = seriesMax(viewers)
  const out: ChartSeries[] = [
    {
      key: 'viewers',
      label: 'Viewers',
      color: '#22d3ee',
      values: viewers,
      max: viewersMax > 0 ? viewersMax : Math.max(0, peakViewersFallback),
    },
    {
      key: 'chat',
      label: 'Chat/min',
      color: '#a78bfa',
      values: chat,
      max: seriesMax(chat),
    },
    {
      key: 'emotes',
      label: 'Emotes/min',
      color: '#34d399',
      values: emotesRaw,
      max: emotesMax,
    },
  ]
  const selectedKeys = Array.from(selected).slice(0, 5)
  selectedKeys.forEach((key, index) => {
    const rawValues = rollups.map((point) => {
      if (point.missing) return null
      const count = resolveEmoteMinuteCount(point, key)
      return count > 0 ? count : null
    })
    const maxVal = seriesMax(rawValues)
    out.push({
      key,
      label: emoteLabel(key),
      color: emoteChartColor(index),
      values: rawValues,
      max: maxVal,
      dashed: true,
    })
  })
  return out
}
