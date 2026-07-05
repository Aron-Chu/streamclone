import type { ChartMinuteRollup } from './types.ts'
import { rollupHasMinuteData } from './chartRollupUtils.ts'

export function rollupsForChart(rollups: ChartMinuteRollup[], isLive: boolean) {
  if (!rollups.length) return rollups
  const dataIndices = rollups
    .map((point, index) => rollupHasMinuteData(point) ? index : -1)
    .filter(index => index >= 0)
  if (!dataIndices.length) return rollups

  const first = dataIndices[0]
  const last = isLive ? rollups.length - 1 : dataIndices[dataIndices.length - 1]
  const pad = isLive ? 5 : 3
  const start = Math.max(0, first - pad)
  const end = Math.min(rollups.length, last + pad + 1)
  return rollups.slice(start, end)
}
