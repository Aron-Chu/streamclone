import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import type { ReplayHeatmapPoint } from '../../types/heatmap'
import { buildHeatmapPeakAriaLabel } from '../../utils/heatmapAriaLabel'

const MAX_GLOW_PEAKS = 3
const GLOW_DURATION_MS = 2000

export interface HeatmapAccessibilityProps {
  points: ReplayHeatmapPoint[]
  totalDurationSec: number
  containerWidth: number
  onPeakSelect?: (point: ReplayHeatmapPoint) => void
  maxPeaks?: number
}

function useReducedMotion(): boolean {
  const [reduced, setReduced] = useState(() => {
    if (typeof window === 'undefined') return false
    return window.matchMedia('(prefers-reduced-motion: reduce)').matches
  })

  useEffect(() => {
    const mql = window.matchMedia('(prefers-reduced-motion: reduce)')
    const handler = (e: MediaQueryListEvent) => setReduced(e.matches)
    mql.addEventListener('change', handler)
    return () => mql.removeEventListener('change', handler)
  }, [])

  return reduced
}

function buildAriaLabel(point: ReplayHeatmapPoint): string {
  return buildHeatmapPeakAriaLabel(point.offsetSeconds, point.score, point.reason)
}

export function HeatmapAccessibility({
  points,
  totalDurationSec,
  containerWidth,
  onPeakSelect,
  maxPeaks = 15,
}: HeatmapAccessibilityProps) {
  const reducedMotion = useReducedMotion()
  const [activeIndex, setActiveIndex] = useState(0)
  const toolbarRef = useRef<HTMLDivElement>(null)
  const buttonRefs = useRef<(HTMLButtonElement | null)[]>([])

  const topPeaks = useMemo(() => {
    if (points.length === 0) return []
    const sorted = [...points].sort((a, b) => b.score - a.score)
    return sorted.slice(0, maxPeaks).sort((a, b) => a.offsetSeconds - b.offsetSeconds)
  }, [points, maxPeaks])

  useEffect(() => {
    setActiveIndex(0)
  }, [topPeaks])

  const peakPositions = useMemo(() => {
    if (totalDurationSec <= 0 || containerWidth <= 0) return []
    return topPeaks.map(p => Math.floor((p.offsetSeconds / totalDurationSec) * containerWidth))
  }, [topPeaks, totalDurationSec, containerWidth])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      if (topPeaks.length === 0) return

      let newIndex = activeIndex
      let handled = false

      switch (e.key) {
        case 'ArrowRight':
          newIndex = (activeIndex + 1) % topPeaks.length
          handled = true
          break
        case 'ArrowLeft':
          newIndex = (activeIndex - 1 + topPeaks.length) % topPeaks.length
          handled = true
          break
        case 'Home':
          newIndex = 0
          handled = true
          break
        case 'End':
          newIndex = topPeaks.length - 1
          handled = true
          break
        case 'Enter':
        case ' ':
          e.preventDefault()
          onPeakSelect?.(topPeaks[activeIndex])
          return
      }

      if (handled) {
        e.preventDefault()
        setActiveIndex(newIndex)
        buttonRefs.current[newIndex]?.focus()
      }
    },
    [activeIndex, topPeaks, onPeakSelect],
  )

  const handleButtonClick = useCallback(
    (index: number) => {
      setActiveIndex(index)
      onPeakSelect?.(topPeaks[index])
    },
    [topPeaks, onPeakSelect],
  )

  if (topPeaks.length === 0) return null

  return (
    <div
      ref={toolbarRef}
      role="toolbar"
      aria-label="Heatmap moment peaks"
      onKeyDown={handleKeyDown}
      className="pointer-events-none absolute inset-0"
    >
      {topPeaks.map((peak, index) => {
        const xPx = peakPositions[index]
        const isActive = index === activeIndex
        const canGlow = !reducedMotion && peak.score >= 80 && index < MAX_GLOW_PEAKS

        return (
          <button
            key={`${peak.offsetSeconds}-${peak.score}`}
            ref={el => { buttonRefs.current[index] = el }}
            type="button"
            tabIndex={isActive ? 0 : -1}
            aria-label={buildAriaLabel(peak)}
            onClick={() => handleButtonClick(index)}
            className={[
              'pointer-events-auto absolute top-0 h-full',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400 focus-visible:ring-offset-1 focus-visible:ring-offset-zinc-900',
              reducedMotion ? '' : 'transition-shadow duration-150',
              canGlow ? 'animate-heatmap-glow' : '',
            ]
              .filter(Boolean)
              .join(' ')}
            style={{
              left: `${xPx}px`,
              width: '6px',
              transform: 'translateX(-3px)',
              ...(reducedMotion ? {} : {}),
            }}
          >
            <span className="sr-only">{buildAriaLabel(peak)}</span>
            {isActive && (
              <span
                className={[
                  'absolute inset-x-0 top-0 h-full border-x-2 border-violet-400',
                  reducedMotion ? '' : 'transition-opacity duration-150',
                ].join(' ')}
                aria-hidden="true"
              />
            )}
          </button>
        )
      })}

      {!reducedMotion && (
        <style>{`
          @keyframes heatmap-glow {
            0%, 100% { box-shadow: 0 0 4px 1px rgba(167, 139, 250, 0.4); }
            50% { box-shadow: 0 0 8px 2px rgba(167, 139, 250, 0.7); }
          }
          .animate-heatmap-glow {
            animation: heatmap-glow ${GLOW_DURATION_MS}ms ease-in-out infinite;
          }
          @media (prefers-reduced-motion: reduce) {
            .animate-heatmap-glow {
              animation: none !important;
            }
          }
        `}</style>
      )}
    </div>
  )
}

export default HeatmapAccessibility
