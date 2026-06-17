import { useCallback, useRef, useState } from 'react'

import { formatVodTimestamp } from './VodModeControls.tsx'

export interface VodSeekBarProps {
  currentSec: number
  totalSec: number | null
  onSeek: (absoluteOffsetSec: number) => void
  className?: string
}

function clampSeek(value: number, total: number) {
  if (!Number.isFinite(total) || total <= 0) return Math.max(0, value)
  return Math.max(0, Math.min(total - 0.5, value))
}

export function VodSeekBar({ currentSec, totalSec, onSeek, className }: VodSeekBarProps) {
  const trackRef = useRef<HTMLDivElement>(null)
  const [dragging, setDragging] = useState(false)
  const [previewSec, setPreviewSec] = useState<number | null>(null)

  const total = totalSec != null && Number.isFinite(totalSec) && totalSec > 0 ? totalSec : null
  const displaySec = dragging && previewSec != null ? previewSec : currentSec
  const progress = total ? Math.min(100, Math.max(0, (displaySec / total) * 100)) : 0

  const seekFromClientX = useCallback((clientX: number) => {
    const track = trackRef.current
    if (!track || !total) return
    const rect = track.getBoundingClientRect()
    if (rect.width <= 0) return
    const ratio = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width))
    const next = clampSeek(Math.floor(ratio * total), total)
    setPreviewSec(next)
    return next
  }, [total])

  const commitSeek = useCallback((sec: number | undefined) => {
    if (sec == null || !Number.isFinite(sec)) return
    onSeek(sec)
    setPreviewSec(null)
    setDragging(false)
  }, [onSeek])

  return (
    <div className={className ?? 'px-3 pb-1 lg:px-5'}>
      <div className="flex items-center gap-2">
        <span className="w-[4.5rem] shrink-0 font-mono text-[11px] font-bold tabular-nums text-zinc-300">
          {formatVodTimestamp(displaySec)}
        </span>
        <div
          ref={trackRef}
          role="slider"
          aria-label="VOD playback position"
          aria-valuemin={0}
          aria-valuemax={total ?? 0}
          aria-valuenow={Math.floor(displaySec)}
          tabIndex={0}
          className="relative h-2 min-w-0 flex-1 cursor-pointer rounded-full bg-white/10"
          onPointerDown={event => {
            if (!total) return
            event.currentTarget.setPointerCapture(event.pointerId)
            setDragging(true)
            seekFromClientX(event.clientX)
          }}
          onPointerMove={event => {
            if (!dragging || !total) return
            seekFromClientX(event.clientX)
          }}
          onPointerUp={event => {
            if (!dragging && !total) return
            if (dragging) {
              event.currentTarget.releasePointerCapture(event.pointerId)
            }
            const next = previewSec ?? seekFromClientX(event.clientX)
            commitSeek(next)
          }}
          onKeyDown={event => {
            if (!total) return
            const step = event.shiftKey ? 60 : 10
            if (event.key === 'ArrowLeft') {
              event.preventDefault()
              onSeek(clampSeek(currentSec - step, total))
            } else if (event.key === 'ArrowRight') {
              event.preventDefault()
              onSeek(clampSeek(currentSec + step, total))
            } else if (event.key === 'Home') {
              event.preventDefault()
              onSeek(0)
            } else if (event.key === 'End') {
              event.preventDefault()
              onSeek(clampSeek(total - 1, total))
            }
          }}
        >
          <div
            className="absolute inset-y-0 left-0 rounded-full bg-violet-500/80"
            style={{ width: `${progress}%` }}
          />
          <div
            className="absolute top-1/2 h-3.5 w-3.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white bg-violet-400 shadow-md shadow-black/40"
            style={{ left: `${progress}%` }}
          />
        </div>
        <span className="w-[4.5rem] shrink-0 text-right font-mono text-[11px] font-bold tabular-nums text-zinc-500">
          {formatVodTimestamp(total)}
        </span>
      </div>
    </div>
  )
}

export default VodSeekBar
