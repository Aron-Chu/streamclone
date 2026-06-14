import type { ReplayHeatmapPoint } from '../types/heatmap'

export interface HeatmapColumn {
  offsetSeconds: number
  score: number
  reason: string
  confidence: number
  point: ReplayHeatmapPoint
}

/**
 * Decimates heatmap points into at most `widthPx` pixel columns,
 * keeping only the max-score point per column. Used to ensure
 * the heatmap lane renders no more than one visual element per
 * visible pixel column (Requirements 14.3, 24.1).
 *
 * On score tie within a column, the point with the lower offset wins.
 */
export function decimateToPixels(
  points: ReplayHeatmapPoint[],
  widthPx: number,
  totalDurationSec: number,
): HeatmapColumn[] {
  if (
    !Number.isFinite(widthPx)
    || !Number.isFinite(totalDurationSec)
    || widthPx <= 0
    || totalDurationSec <= 0
    || points.length === 0
  ) {
    return []
  }

  const safeWidth = Math.floor(widthPx)
  const secsPerColumn = totalDurationSec / safeWidth
  if (!Number.isFinite(secsPerColumn) || secsPerColumn <= 0) return []
  const columns: (HeatmapColumn | null)[] = new Array(safeWidth).fill(null)

  for (const point of points) {
    if (!Number.isFinite(point.offsetSeconds) || !Number.isFinite(point.score)) continue
    const col = Math.floor(point.offsetSeconds / secsPerColumn)
    if (col < 0 || col >= safeWidth) continue

    const existing = columns[col]
    if (
      existing === null ||
      point.score > existing.score ||
      (point.score === existing.score && point.offsetSeconds < existing.offsetSeconds)
    ) {
      columns[col] = {
        offsetSeconds: point.offsetSeconds,
        score: point.score,
        reason: point.reason,
        confidence: point.confidence,
        point,
      }
    }
  }

  return columns.filter((c): c is HeatmapColumn => c !== null)
}
