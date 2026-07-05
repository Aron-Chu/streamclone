import type { ChartGameSegment, ChartMinuteRollup } from './types.ts'
import { gameSegmentPlotBounds } from './gameSegmentChart.ts'

export interface GameSegmentOverlayProps {
  segments: ChartGameSegment[]
  rollups: ChartMinuteRollup[]
  streamStartedAt?: string
  padLeft: number
  padTop: number
  padBottom: number
  plotWidth: number
  height: number
}

export function GameSegmentOverlay({
  segments,
  rollups,
  streamStartedAt,
  padLeft,
  padTop,
  padBottom,
  plotWidth,
  height,
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
        const { startX, centerX, textWidth } = bounds
        const maxChars = Math.floor(textWidth / 8)
        const displayTitle = segment.gameName.length > maxChars
          ? `${segment.gameName.slice(0, Math.max(0, maxChars - 3))}...`
          : segment.gameName

        return (
          <g key={segment.id ?? index}>
            {segment.offsetSeconds > 0 ? (
              <line
                x1={startX}
                y1={padTop}
                x2={startX}
                y2={height - padBottom}
                stroke="#f97316"
                strokeWidth="1.5"
                strokeDasharray="4 4"
                opacity="0.6"
              />
            ) : null}
            {textWidth > 30 ? (
              <g>
                <rect
                  x={startX + 4}
                  y={padTop - 24}
                  width={textWidth - 8}
                  height={18}
                  rx={4}
                  fill="#f97316"
                  opacity="0.12"
                />
                <text
                  x={centerX}
                  y={padTop - 11}
                  fill="#f97316"
                  fontSize="9.5"
                  fontWeight="900"
                  textAnchor="middle"
                  opacity="0.95"
                >
                  {displayTitle}
                </text>
              </g>
            ) : null}
          </g>
        )
      })}
    </>
  )
}
