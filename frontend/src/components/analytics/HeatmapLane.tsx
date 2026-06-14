import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'

import type { HeatmapEmote, ReplayHeatmapPoint } from '../../types/heatmap'
import { decimateToPixels, type HeatmapColumn } from '../../utils/heatmapDecimate'
import { formatDuration } from '../../utils/durationFormat'

export interface HeatmapLaneProps {
  points: ReplayHeatmapPoint[]
  totalDurationSec: number
  className?: string
  onPeakSelect?: (point: ReplayHeatmapPoint) => void
  onPlayAction?: (offsetSeconds: number) => void
}

const LANE_HEIGHT = 18
const TOOLTIP_OFFSET_Y = -8

/**
 * Score-to-color gradient:
 *   0   → rgb(39, 39, 42)   zinc-800
 *   50  → rgb(245, 158, 11) amber-500
 *   100 → rgb(239, 68, 68)  red-500
 */
function scoreToColor(score: number): string {
  const t = Math.max(0, Math.min(100, score)) / 100
  let r: number, g: number, b: number

  if (t <= 0.5) {
    const s = t / 0.5
    r = Math.round(39 + (245 - 39) * s)
    g = Math.round(39 + (158 - 39) * s)
    b = Math.round(42 + (11 - 42) * s)
  } else {
    const s = (t - 0.5) / 0.5
    r = Math.round(245 + (239 - 245) * s)
    g = Math.round(158 + (68 - 158) * s)
    b = Math.round(11 + (68 - 11) * s)
  }

  return `rgb(${r}, ${g}, ${b})`
}

const REASON_LABELS: Record<string, string> = {
  chat_spike: 'Chat spike',
  seventv_spike: '7TV spike',
  twitch_emote_spike: 'Twitch emote spike',
  ffz_spike: 'FFZ spike',
  viewer_spike: 'Viewer spike',
  game_change: 'Game change',
  manual: 'Manual',
}


interface EmoteImageState {
  url: string
  loaded: boolean
  error: boolean
}

const MAX_CONCURRENT_EMOTE_REQUESTS = 3

/**
 * Lazy-loads emote images on hover/select, capping at 3 concurrent requests
 * (Requirements 24.2, 24.3, 24.4).
 */
function useEmoteLazyLoader() {
  const activeRequests = useRef(0)
  const cache = useRef<Map<string, EmoteImageState>>(new Map())
  const queue = useRef<Array<{ emote: HeatmapEmote; resolve: () => void }>>([])

  const processQueue = useCallback(() => {
    while (queue.current.length > 0 && activeRequests.current < MAX_CONCURRENT_EMOTE_REQUESTS) {
      const item = queue.current.shift()
      if (!item) break
      activeRequests.current++
      const img = new Image()
      img.onload = () => {
        activeRequests.current--
        const entry = cache.current.get(item.emote.id)
        if (entry) entry.loaded = true
        item.resolve()
        processQueue()
      }
      img.onerror = () => {
        activeRequests.current--
        const entry = cache.current.get(item.emote.id)
        if (entry) entry.error = true
        item.resolve()
        processQueue()
      }
      img.src = item.emote.imageUrl
    }
  }, [])

  const loadEmotes = useCallback(
    (emotes: HeatmapEmote[]): Promise<void> => {
      const promises: Promise<void>[] = []
      for (const emote of emotes.slice(0, 3)) {
        if (cache.current.has(emote.id)) continue
        cache.current.set(emote.id, { url: emote.imageUrl, loaded: false, error: false })
        const p = new Promise<void>(resolve => {
          queue.current.push({ emote, resolve })
        })
        promises.push(p)
      }
      processQueue()
      return Promise.all(promises).then(() => undefined)
    },
    [processQueue],
  )

  const getState = useCallback((id: string): EmoteImageState | undefined => {
    return cache.current.get(id)
  }, [])

  return { loadEmotes, getState }
}

interface TooltipData {
  x: number
  y: number
  column: HeatmapColumn
}

/**
 * HeatmapLane renders a thin canvas-based heat band below the analytics chart
 * timeline (Requirement 14). It uses pixel-column decimation for performance,
 * a color gradient from zinc-800 (score 0) through amber-500 (50) to red-500 (100),
 * and shows a tooltip on hover with offset, reason, emotes, chat rate, and a Play action.
 *
 * When no points are available or all major signals have 0 confidence, the lane
 * renders in a muted empty state (Requirement 14.5).
 *
 * No animation is run when the tab is hidden (Requirement 24.4).
 */
export function HeatmapLane({
  points,
  totalDurationSec,
  className,
  onPeakSelect,
  onPlayAction,
}: HeatmapLaneProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const columnsRef = useRef<HeatmapColumn[]>([])
  const [containerWidth, setContainerWidth] = useState(0)
  const [tooltip, setTooltip] = useState<TooltipData | null>(null)
  const tooltipRef = useRef<HTMLDivElement>(null)
  const rafRef = useRef<number | null>(null)
  const pendingTooltip = useRef<TooltipData | null>(null)
  const { loadEmotes, getState } = useEmoteLazyLoader()
  const [, forceUpdate] = useState(0)
  const safeTotalDurationSec = Number.isFinite(totalDurationSec) && totalDurationSec > 0
    ? totalDurationSec
    : Math.max(60, points.length * 60)

  const isMuted =
    points.length === 0 ||
    points.every(
      p => p.confidence === 0 || (p.score === 0),
    )

  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const observer = new ResizeObserver(entries => {
      for (const entry of entries) {
        const w = Math.floor(entry.contentRect.width)
        if (w > 0) setContainerWidth(w)
      }
    })
    observer.observe(container)
    return () => observer.disconnect()
  }, [])

  useLayoutEffect(() => {
    if (containerWidth <= 0 || isMuted) {
      columnsRef.current = []
      return
    }
    columnsRef.current = decimateToPixels(points, containerWidth, safeTotalDurationSec)
  }, [points, containerWidth, safeTotalDurationSec, isMuted])

  useLayoutEffect(() => {
    const canvas = canvasRef.current
    if (!canvas || containerWidth <= 0) return

    if (isMuted) {
      const ctx = canvas.getContext('2d')
      if (!ctx) return
      const dpr = window.devicePixelRatio || 1
      canvas.width = Math.floor(containerWidth * dpr)
      canvas.height = Math.floor(LANE_HEIGHT * dpr)
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
      ctx.clearRect(0, 0, containerWidth, LANE_HEIGHT)
      ctx.fillStyle = 'rgba(39, 39, 42, 0.3)'
      ctx.fillRect(0, 0, containerWidth, LANE_HEIGHT)
      return
    }

    if (document.hidden) return

    const dpr = window.devicePixelRatio || 1
    canvas.width = Math.floor(containerWidth * dpr)
    canvas.height = Math.floor(LANE_HEIGHT * dpr)

    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.clearRect(0, 0, containerWidth, LANE_HEIGHT)

    ctx.fillStyle = 'rgb(39, 39, 42)'
    ctx.fillRect(0, 0, containerWidth, LANE_HEIGHT)

    const secsPerColumn = safeTotalDurationSec / containerWidth
    const columns = columnsRef.current

    for (const col of columns) {
      if (col.score <= 0) continue
      const x = Math.floor(col.offsetSeconds / secsPerColumn)
      if (x < 0 || x >= containerWidth) continue
      ctx.fillStyle = scoreToColor(col.score)
      ctx.fillRect(x, 0, 1, LANE_HEIGHT)
    }
  }, [containerWidth, points, safeTotalDurationSec, isMuted])

  useEffect(() => {
    const handleVisibility = () => {
      if (!document.hidden && canvasRef.current && containerWidth > 0 && !isMuted) {
        const canvas = canvasRef.current
        const dpr = window.devicePixelRatio || 1
        canvas.width = Math.floor(containerWidth * dpr)
        canvas.height = Math.floor(LANE_HEIGHT * dpr)
        const ctx = canvas.getContext('2d')
        if (!ctx) return
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
        ctx.clearRect(0, 0, containerWidth, LANE_HEIGHT)
        ctx.fillStyle = 'rgb(39, 39, 42)'
        ctx.fillRect(0, 0, containerWidth, LANE_HEIGHT)

        const secsPerColumn = safeTotalDurationSec / containerWidth
        const columns = columnsRef.current
        for (const col of columns) {
          if (col.score <= 0) continue
          const x = Math.floor(col.offsetSeconds / secsPerColumn)
          if (x < 0 || x >= containerWidth) continue
          ctx.fillStyle = scoreToColor(col.score)
          ctx.fillRect(x, 0, 1, LANE_HEIGHT)
        }
      }
    }
    document.addEventListener('visibilitychange', handleVisibility)
    return () => document.removeEventListener('visibilitychange', handleVisibility)
  }, [containerWidth, safeTotalDurationSec, isMuted])

  const updateTooltipPosition = useCallback(() => {
    if (pendingTooltip.current) {
      setTooltip(pendingTooltip.current)
    }
    rafRef.current = null
  }, [])

  const handleMouseMove = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (isMuted || containerWidth <= 0) return

      const rect = containerRef.current?.getBoundingClientRect()
      if (!rect) return
      const localX = e.clientX - rect.left
      const secsPerColumn = safeTotalDurationSec / containerWidth
      const offsetSec = localX * secsPerColumn

      const columns = columnsRef.current
      let best: HeatmapColumn | null = null
      let bestDist = Infinity

      for (const col of columns) {
        if (col.score <= 0) continue
        const dist = Math.abs(col.offsetSeconds - offsetSec)
        if (dist < bestDist && dist < secsPerColumn * 3) {
          bestDist = dist
          best = col
        }
      }

      if (best) {
        pendingTooltip.current = { x: e.clientX - rect.left, y: TOOLTIP_OFFSET_Y, column: best }
        if (best.point.topEmotes?.length) {
          loadEmotes(best.point.topEmotes).then(() => forceUpdate(n => n + 1))
        }
      } else {
        pendingTooltip.current = null
      }

      if (rafRef.current === null) {
        rafRef.current = requestAnimationFrame(updateTooltipPosition)
      }
    },
    [isMuted, containerWidth, safeTotalDurationSec, loadEmotes, updateTooltipPosition],
  )

  const handleMouseLeave = useCallback(() => {
    pendingTooltip.current = null
    setTooltip(null)
    if (rafRef.current !== null) {
      cancelAnimationFrame(rafRef.current)
      rafRef.current = null
    }
  }, [])

  const handleClick = useCallback(() => {
    if (!tooltip) return
    onPeakSelect?.(tooltip.column.point)
  }, [tooltip, onPeakSelect])

  useEffect(() => {
    return () => {
      if (rafRef.current !== null) {
        cancelAnimationFrame(rafRef.current)
      }
    }
  }, [])

  if (isMuted) {
    return (
      <div
        ref={containerRef}
        className={`relative w-full select-none ${className ?? ''}`}
        style={{ height: LANE_HEIGHT }}
      >
        <canvas
          ref={canvasRef}
          className="h-full w-full"
          style={{ height: LANE_HEIGHT }}
          aria-hidden="true"
        />
        <div className="absolute inset-0 flex items-center justify-center">
          <span className="text-[10px] font-medium text-zinc-600">
            {points.length === 0 ? 'No heatmap data' : 'Insufficient signal confidence'}
          </span>
        </div>
      </div>
    )
  }

  return (
    <div
      ref={containerRef}
      className={`relative w-full cursor-crosshair select-none ${className ?? ''}`}
      style={{ height: LANE_HEIGHT }}
      onMouseMove={handleMouseMove}
      onMouseLeave={handleMouseLeave}
      onClick={handleClick}
      role="img"
      aria-label="Heatmap showing moment intensity across the stream timeline"
    >
      <canvas
        ref={canvasRef}
        className="h-full w-full"
        style={{ height: LANE_HEIGHT }}
        aria-hidden="true"
      />

      {tooltip && (
        <div
          ref={tooltipRef}
          className="pointer-events-none absolute z-50 rounded-lg border border-white/10 bg-zinc-900/95 px-3 py-2 shadow-xl backdrop-blur-sm"
          style={{
            left: Math.min(tooltip.x, containerWidth - 200),
            bottom: LANE_HEIGHT - tooltip.y,
            transform: 'translateX(-50%)',
            minWidth: 160,
          }}
        >
          <div className="flex flex-col gap-1.5">
            <div className="flex items-center justify-between gap-3">
              <span className="text-xs font-black tabular-nums text-white">
                {formatDuration(tooltip.column.offsetSeconds)}
              </span>
              <span className="rounded bg-white/10 px-1.5 py-0.5 text-[10px] font-bold text-zinc-300">
                {REASON_LABELS[tooltip.column.reason] ?? tooltip.column.reason}
              </span>
            </div>

            {tooltip.column.point.topEmotes && tooltip.column.point.topEmotes.length > 0 && (
              <div className="flex items-center gap-1.5">
                {tooltip.column.point.topEmotes.slice(0, 3).map(emote => {
                  const state = getState(emote.id)
                  return (
                    <span
                      key={emote.id}
                      className="flex items-center gap-0.5"
                      title={`${emote.name} (${emote.count})`}
                    >
                      {state?.loaded ? (
                        <img
                          src={emote.imageUrl}
                          alt={emote.name}
                          width={20}
                          height={20}
                          className="h-5 w-5 object-contain"
                        />
                      ) : (
                        <span className="flex h-5 w-5 items-center justify-center rounded bg-white/5 text-[8px] text-zinc-500">
                          {emote.name.slice(0, 2)}
                        </span>
                      )}
                    </span>
                  )
                })}
              </div>
            )}

            <div className="flex items-center justify-between gap-2">
              <span className="text-[10px] text-zinc-400">
                Score {Math.round(tooltip.column.score)}/100
              </span>
              {onPlayAction && (
                <button
                  className="pointer-events-auto rounded bg-violet-600/80 px-2 py-0.5 text-[10px] font-bold text-white hover:bg-violet-500 active:bg-violet-700"
                  onClick={e => {
                    e.stopPropagation()
                    onPlayAction(tooltip.column.offsetSeconds)
                  }}
                >
                  ▶ Play
                </button>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default HeatmapLane
