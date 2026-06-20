import { memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'

import type { AnalyticsMinuteRollup } from '../../api'
import { formatDuration } from '../../utils/durationFormat'
import {
  ACTIVITY_WAVEFORM_LAYERS,
  ACTIVITY_WAVEFORM_LAYER_ORDER,
  activityWaveformOffsetToX,
  bucketActivityPointsForDraw,
  deriveActivityWaveform,
  loadLayerPrefs,
  peakLayerToReason,
  saveLayerPrefs,
  type ActivityWaveformLayerId,
  type ActivityWaveformLayerVisibility,
  type ActivityWaveformPeak,
  type ActivityWaveformPoint,
} from '../../utils/activityWaveform'

export interface ActivityWaveformProps {
  rollups: AnalyticsMinuteRollup[]
  totalDurationSec?: number
  className?: string
  variant?: 'analytics' | 'player'
  currentOffsetSec?: number | null
  highlightOffsetSec?: number | null
  onSeek?: (offsetSeconds: number) => void
  onSelectPoint?: (point: ActivityWaveformPoint) => void
  onPeakSelect?: (peak: ActivityWaveformPeak) => void
  onPlayAction?: (offsetSeconds: number) => void
  playHrefForOffset?: (offsetSeconds: number) => string | undefined
  showLayerToggles?: boolean
}

const ANALYTICS_HEIGHT = 56
const PLAYER_HEIGHT = 24

function useReducedMotion(): boolean {
  const [reduced, setReduced] = useState<boolean>(() =>
    typeof window !== 'undefined' && typeof window.matchMedia === 'function'
      ? window.matchMedia('(prefers-reduced-motion: reduce)').matches
      : false,
  )

  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    const onChange = () => setReduced(mq.matches)
    onChange()
    if (typeof mq.addEventListener === 'function') {
      mq.addEventListener('change', onChange)
      return () => mq.removeEventListener('change', onChange)
    }
    mq.addListener(onChange)
    return () => mq.removeListener(onChange)
  }, [])

  return reduced
}

interface TooltipState {
  x: number
  point: ActivityWaveformPoint
  peak: ActivityWaveformPeak | null
}

function hexToRgba(hex: string, alpha: number): string {
  const normalized = hex.replace('#', '')
  const full = normalized.length === 3
    ? normalized.split('').map(ch => ch + ch).join('')
    : normalized
  const int = Number.parseInt(full, 16)
  const r = (int >> 16) & 255
  const g = (int >> 8) & 255
  const b = int & 255
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

function findNearestPeak(
  peaks: ActivityWaveformPeak[],
  offsetSeconds: number,
  toleranceSec: number,
): ActivityWaveformPeak | null {
  let best: ActivityWaveformPeak | null = null
  let bestDist = Infinity
  for (const peak of peaks) {
    const dist = Math.abs(peak.offsetSeconds - offsetSeconds)
    if (dist < bestDist && dist <= toleranceSec) {
      bestDist = dist
      best = peak
    }
  }
  return best
}

function ActivityWaveformInner({
  rollups,
  totalDurationSec,
  className,
  variant = 'analytics',
  currentOffsetSec,
  highlightOffsetSec,
  onSeek,
  onSelectPoint,
  onPeakSelect,
  onPlayAction,
  playHrefForOffset,
  showLayerToggles = variant === 'analytics',
}: ActivityWaveformProps) {
  const laneHeight = variant === 'player' ? PLAYER_HEIGHT : ANALYTICS_HEIGHT
  const containerRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [containerWidth, setContainerWidth] = useState(0)
  const [layerVisibility, setLayerVisibility] = useState<ActivityWaveformLayerVisibility>(() => loadLayerPrefs())
  const [tooltip, setTooltip] = useState<TooltipState | null>(null)
  const [pinnedTooltip, setPinnedTooltip] = useState<TooltipState | null>(null)
  const hoverFrameRef = useRef<number | null>(null)
  const pendingHoverXRef = useRef<number | null>(null)
  const reducedMotion = useReducedMotion()
  const activeTooltip = pinnedTooltip ?? tooltip

  const waveform = useMemo(
    () => deriveActivityWaveform(rollups, layerVisibility),
    [rollups, layerVisibility],
  )

  const rawDurationSec = totalDurationSec ?? waveform.totalDurationSec
  const durationSec = Number.isFinite(rawDurationSec) && rawDurationSec > 0
    ? rawDurationSec
    : Math.max(60, waveform.points.length * 60)

  const globalPeak = useMemo(() => {
    if (!waveform.hasData || waveform.points.length === 0) return null
    let bestPoint: ActivityWaveformPoint | null = null
    let bestScore = -1
    for (const point of waveform.points) {
      let peakScore = 0
      for (const layer of ACTIVITY_WAVEFORM_LAYER_ORDER) {
        if (!layerVisibility[layer]) continue
        peakScore = Math.max(peakScore, point.normalized[layer])
      }
      if (peakScore > bestScore) {
        bestScore = peakScore
        bestPoint = point
      }
    }
    return bestScore > 0 ? bestPoint : null
  }, [layerVisibility, waveform])

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

  const toggleLayer = useCallback((layer: ActivityWaveformLayerId) => {
    setLayerVisibility(current => {
      const next = { ...current, [layer]: !current[layer] }
      const anyOn = ACTIVITY_WAVEFORM_LAYER_ORDER.some(id => next[id])
      if (!anyOn) return current
      saveLayerPrefs(next)
      return next
    })
  }, [])

  const drawCanvas = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas || containerWidth <= 0) return

    const dpr = window.devicePixelRatio || 1
    canvas.width = Math.floor(containerWidth * dpr)
    canvas.height = Math.floor(laneHeight * dpr)

    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.clearRect(0, 0, containerWidth, laneHeight)

    ctx.fillStyle = variant === 'player' ? 'rgba(39, 39, 42, 0.4)' : 'rgb(39, 39, 42)'
    ctx.fillRect(0, 0, containerWidth, laneHeight)

    if (!waveform.hasData || waveform.points.length === 0) return
    if (document.hidden && !reducedMotion) return

    const points = bucketActivityPointsForDraw(waveform.points)
    const stepX = containerWidth / Math.max(1, points.length - 1)
    const baseline = laneHeight - 1

    for (const layerMeta of ACTIVITY_WAVEFORM_LAYERS) {
      if (!layerVisibility[layerMeta.id]) continue
      ctx.beginPath()
      ctx.moveTo(0, baseline)
      for (let i = 0; i < points.length; i++) {
        const x = i * stepX
        const height = points[i].normalized[layerMeta.id] * (laneHeight - 4)
        ctx.lineTo(x, baseline - height)
      }
      ctx.lineTo(containerWidth, baseline)
      ctx.closePath()
      ctx.fillStyle = hexToRgba(layerMeta.color, variant === 'player' ? 0.45 : 0.38)
      ctx.fill()
    }

    if (highlightOffsetSec != null && Number.isFinite(highlightOffsetSec)) {
      const originX = activityWaveformOffsetToX(highlightOffsetSec, durationSec, containerWidth)
      ctx.strokeStyle = variant === 'player'
        ? 'rgba(167, 139, 250, 0.85)'
        : 'rgba(167, 139, 250, 0.7)'
      ctx.lineWidth = variant === 'player' ? 1 : 1
      ctx.setLineDash(variant === 'player' ? [2, 2] : [3, 3])
      ctx.beginPath()
      ctx.moveTo(originX, 0)
      ctx.lineTo(originX, laneHeight)
      ctx.stroke()
      ctx.setLineDash([])
    }
    if (currentOffsetSec != null && Number.isFinite(currentOffsetSec)) {
      const playheadX = activityWaveformOffsetToX(currentOffsetSec, durationSec, containerWidth)
      ctx.strokeStyle = variant === 'player'
        ? 'rgba(255, 255, 255, 0.92)'
        : 'rgba(255, 255, 255, 0.85)'
      ctx.lineWidth = variant === 'player' ? 2 : 2
      ctx.beginPath()
      ctx.moveTo(playheadX, 0)
      ctx.lineTo(playheadX, laneHeight)
      ctx.stroke()
    }
  }, [containerWidth, currentOffsetSec, durationSec, highlightOffsetSec, laneHeight, layerVisibility, reducedMotion, variant, waveform])

  useLayoutEffect(() => {
    drawCanvas()
  }, [drawCanvas])

  useEffect(() => {
    const handleVisibility = () => {
      if (!document.hidden) drawCanvas()
    }
    document.addEventListener('visibilitychange', handleVisibility)
    return () => document.removeEventListener('visibilitychange', handleVisibility)
  }, [drawCanvas])

  const resolveHover = useCallback(
    (localX: number): TooltipState | null => {
      if (!waveform.hasData || waveform.points.length === 0 || containerWidth <= 0) return null
      const ratio = Math.max(0, Math.min(1, localX / containerWidth))
      const offsetSec = ratio * durationSec
      const index = Math.min(
        waveform.points.length - 1,
        Math.max(0, Math.round((offsetSec / Math.max(durationSec, 1)) * (waveform.points.length - 1))),
      )
      const point = waveform.points[index]
      const tolerance = Math.max(90, durationSec / waveform.points.length * 1.5)
      const peak = findNearestPeak(waveform.peaks, point.offsetSeconds, tolerance)
      return { x: localX, point, peak }
    },
    [containerWidth, durationSec, waveform],
  )

  useEffect(() => () => {
    if (hoverFrameRef.current !== null) {
      cancelAnimationFrame(hoverFrameRef.current)
    }
  }, [])

  const handleMouseMove = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (pinnedTooltip) return
      const rect = containerRef.current?.getBoundingClientRect()
      if (!rect) return
      pendingHoverXRef.current = e.clientX - rect.left
      if (hoverFrameRef.current !== null) return
      hoverFrameRef.current = requestAnimationFrame(() => {
        hoverFrameRef.current = null
        const localX = pendingHoverXRef.current
        if (localX === null) return
        setTooltip(resolveHover(localX))
      })
    },
    [pinnedTooltip, resolveHover],
  )

  const handleMouseLeave = useCallback(() => {
    if (!pinnedTooltip) setTooltip(null)
  }, [pinnedTooltip])

  useEffect(() => {
    if (!pinnedTooltip) return
    const onDocumentPointerDown = (event: PointerEvent) => {
      const root = containerRef.current
      if (!root || root.contains(event.target as Node)) return
      setPinnedTooltip(null)
      setTooltip(null)
    }
    document.addEventListener('pointerdown', onDocumentPointerDown)
    return () => document.removeEventListener('pointerdown', onDocumentPointerDown)
  }, [pinnedTooltip])

  const handleClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      const rect = containerRef.current?.getBoundingClientRect()
      if (!rect || containerWidth <= 0) return
      const localX = e.clientX - rect.left
      const ratio = Math.max(0, Math.min(1, localX / containerWidth))
      const offsetSec = Math.round(ratio * durationSec)

      const hover = resolveHover(localX)
      if (!hover) return

      setPinnedTooltip(hover)
      setTooltip(hover)
      onSelectPoint?.(hover.point)
      if (hover.peak) {
        onPeakSelect?.(hover.peak)
      }

      onSeek?.(offsetSec)
    },
    [containerWidth, durationSec, onPeakSelect, onSeek, onSelectPoint, playHrefForOffset, resolveHover, rollups.length],
  )

  const layerToggleRow = showLayerToggles ? (
    <div className="mb-1.5 flex flex-wrap items-center gap-1.5">
      {ACTIVITY_WAVEFORM_LAYERS.filter(layer => layer.id !== 'total_emotes').map(layer => {
        const active = layerVisibility[layer.id]
        return (
          <button
            key={layer.id}
            type="button"
            onClick={() => toggleLayer(layer.id)}
            className={`rounded border px-2 py-0.5 text-[10px] font-bold uppercase transition ${
              active
                ? 'border-white/15 bg-white/[0.06] text-zinc-200'
                : 'border-white/5 bg-transparent text-zinc-600 hover:text-zinc-400'
            }`}
            aria-pressed={active}
          >
            <span
              className="mr-1 inline-block h-1.5 w-1.5 rounded-full"
              style={{ backgroundColor: active ? layer.color : 'rgb(82, 82, 91)' }}
              aria-hidden
            />
            {layer.label}
          </button>
        )
      })}
      {globalPeak && variant === 'analytics' ? (
        playHrefForOffset?.(globalPeak.offsetSeconds) ? (
          <a
            href={playHrefForOffset(globalPeak.offsetSeconds)}
            target="_blank"
            rel="noopener noreferrer"
            className="ml-auto rounded border border-violet-500/30 bg-violet-500/10 px-2 py-0.5 text-[10px] font-bold text-violet-200 transition hover:bg-violet-500/20"
            onClick={() => onSeek?.(globalPeak.offsetSeconds)}
          >
            Most reacted {formatDuration(globalPeak.offsetSeconds)}
          </a>
        ) : (
          <button
            type="button"
            className="ml-auto rounded border border-violet-500/30 bg-violet-500/10 px-2 py-0.5 text-[10px] font-bold text-violet-200 transition hover:bg-violet-500/20"
            onClick={() => {
              onSeek?.(globalPeak.offsetSeconds)
              onPlayAction?.(globalPeak.offsetSeconds)
            }}
          >
            Most reacted {formatDuration(globalPeak.offsetSeconds)}
          </button>
        )
      ) : null}
    </div>
  ) : null

  if (!waveform.hasData) {
    return (
      <div className={className}>
        {layerToggleRow}
        <div
          ref={containerRef}
          className="relative w-full select-none"
          style={{ height: laneHeight }}
        >
          <canvas
            ref={canvasRef}
            className="h-full w-full"
            style={{ height: laneHeight }}
            aria-hidden="true"
          />
          <div className="absolute inset-0 flex items-center justify-center">
            <span className="text-[10px] font-medium text-zinc-600">
              {waveform.emptyReason ?? 'No activity data'}
            </span>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className={className}>
      {layerToggleRow}
      <div
        ref={containerRef}
        className={`relative w-full select-none ${variant === 'player' || onSeek ? 'cursor-pointer' : 'cursor-crosshair'}`}
        style={{ height: laneHeight }}
        onMouseMove={handleMouseMove}
        onMouseLeave={handleMouseLeave}
        onClick={handleClick}
        role={variant === 'player' ? 'slider' : 'img'}
        aria-label="Activity waveform across the stream timeline"
        aria-valuemin={variant === 'player' ? 0 : undefined}
        aria-valuemax={variant === 'player' ? durationSec : undefined}
        aria-valuenow={variant === 'player' ? (currentOffsetSec ?? tooltip?.point.offsetSeconds ?? 0) : undefined}
        tabIndex={variant === 'player' ? 0 : undefined}
      >
        <canvas
          ref={canvasRef}
          className={`h-full w-full ${reducedMotion ? '' : 'transition-opacity duration-300'}`}
          style={{ height: laneHeight }}
          aria-hidden="true"
        />

        {activeTooltip ? (
          <div
            className={`absolute z-50 rounded-lg border border-white/10 bg-zinc-900/95 px-3 py-2 shadow-xl backdrop-blur-sm ${
              pinnedTooltip ? 'pointer-events-auto' : 'pointer-events-none'
            }`}
            style={{
              left: Math.min(activeTooltip.x, containerWidth - 220),
              bottom: laneHeight + 6,
              transform: 'translateX(-50%)',
              minWidth: 160,
            }}
          >
            <div className="flex flex-col gap-1.5">
              <div className="flex items-center justify-between gap-3">
                <span className="text-xs font-black tabular-nums text-white">
                  {formatDuration(activeTooltip.point.offsetSeconds)}
                </span>
                {activeTooltip.peak ? (
                  <span className="rounded bg-white/10 px-1.5 py-0.5 text-[10px] font-bold text-zinc-300">
                    Peak · {peakLayerToReason(activeTooltip.peak.dominantLayer).replace(/_/g, ' ')}
                  </span>
                ) : null}
              </div>

              <div className="grid grid-cols-2 gap-x-3 gap-y-0.5 text-[10px] text-zinc-400">
                {ACTIVITY_WAVEFORM_LAYERS.filter(layer => layerVisibility[layer.id]).map(layer => (
                  <span key={layer.id}>
                    <span style={{ color: layer.color }}>{layer.label}</span>
                    {' '}
                    <strong className="text-zinc-200">{Math.round(activeTooltip.point.raw[layer.id])}</strong>
                  </span>
                ))}
              </div>

              {playHrefForOffset?.(activeTooltip.point.offsetSeconds) ? (
                <div className="flex justify-end">
                  <a
                    href={playHrefForOffset(activeTooltip.point.offsetSeconds)}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="rounded bg-violet-600/80 px-2 py-0.5 text-[10px] font-bold text-white hover:bg-violet-500 active:bg-violet-700"
                    onClick={e => e.stopPropagation()}
                  >
                    ▶ Watch on Twitch
                  </a>
                </div>
              ) : onPlayAction ? (
                <div className="flex justify-end">
                  <button
                    type="button"
                    className="pointer-events-auto rounded bg-violet-600/80 px-2 py-0.5 text-[10px] font-bold text-white hover:bg-violet-500 active:bg-violet-700"
                    onClick={e => {
                      e.stopPropagation()
                      onPlayAction(activeTooltip.point.offsetSeconds)
                    }}
                  >
                    ▶ Watch on Twitch
                  </button>
                </div>
              ) : null}
            </div>
          </div>
        ) : null}
      </div>
    </div>
  )
}

const ActivityWaveform = memo(ActivityWaveformInner)
export { ActivityWaveform }
export default ActivityWaveform
