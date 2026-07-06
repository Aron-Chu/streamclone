import type { ChartGameSegment, ChartMinuteRollup } from './types.ts'
import { gameSegmentPlotBounds } from './gameSegmentChart.ts'

const GAME_ACCENT = '#f97316'

export interface GameSegmentOverlayProps {
  segments: ChartGameSegment[]
  rollups: ChartMinuteRollup[]
  streamStartedAt?: string
  padLeft: number
  plotWidth: number
  /** Top edge of the dedicated game strip (px). */
  gameBandTop: number
  /** Height of the game strip band (px). */
  gameBandHeight?: number
  /** Minimum label band width (px) before hiding title text. Default 28. */
  minLabelWidth?: number
}

export function GameSegmentOverlay({
  segments,
  rollups,
  streamStartedAt,
  padLeft,
  plotWidth,
  gameBandTop,
  gameBandHeight = 16,
  minLabelWidth = 28,
}: GameSegmentOverlayProps) {
  return (
    <>
      {segments.map((segment, index) => {
        const bounds = gameSegmentPlotBounds(
          segment,
          rollups,
          streamStartedAt,
          padLeft,
          plotWidth,
        )
        if (!bounds) return null
        const { startX, endX, centerX, textWidth } = bounds
        const bandWidth = Math.max(1, endX - startX)
        const maxChars = Math.floor(textWidth / 7.5)
        const isEstimated = segment.source === 'category_fallback'
        const title = isEstimated ? `Est. ${segment.gameName}` : segment.gameName
        const displayTitle = title.length > maxChars
          ? `${title.slice(0, Math.max(0, maxChars - 3))}...`
          : title

        return (
          <g key={segment.id ?? index}>
            <rect
              x={startX}
              y={gameBandTop}
              width={bandWidth}
              height={gameBandHeight}
              rx={4}
              fill={isEstimated ? 'rgba(249, 115, 22, 0.08)' : 'rgba(249, 115, 22, 0.14)'}
              stroke={isEstimated ? 'rgba(249, 115, 22, 0.28)' : 'rgba(249, 115, 22, 0.38)'}
              strokeWidth={0.75}
              strokeDasharray={isEstimated ? '4 3' : undefined}
              shapeRendering="geometricPrecision"
            />
            {segment.offsetSeconds > 0 ? (
              <line
                x1={startX}
                x2={startX}
                y1={gameBandTop + gameBandHeight}
                y2={gameBandTop + gameBandHeight + 120}
                stroke={GAME_ACCENT}
                strokeWidth="1"
                strokeDasharray="5 6"
                opacity="0.45"
              />
            ) : null}
            {textWidth > minLabelWidth ? (
              <text
                x={centerX}
                y={gameBandTop + gameBandHeight * 0.68}
                fill={GAME_ACCENT}
                fontSize="9"
                fontWeight="800"
                textAnchor="middle"
                opacity="0.96"
              >
                {displayTitle}
              </text>
            ) : null}
          </g>
        )
      })}
    </>
  )
}
