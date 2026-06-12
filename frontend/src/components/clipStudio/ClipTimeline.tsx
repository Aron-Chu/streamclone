import { useMemo } from 'react'
import type { RefObject } from 'react'
import type { CaptionWord, ClipperJob } from '../../api'
import { buildActivityBars, formatTime, spikePositionInSource } from './utils'

interface ClipTimelineProps {
  progressRef: RefObject<HTMLDivElement | null>
  captions: CaptionWord[]
  duration: number
  currentTime: number
  trimStart: number
  trimEnd: number
  isPlaying: boolean
  job: ClipperJob
  previewRelativeTime: number
  previewMode: 'source' | 'final'
  onScrub: (e: React.MouseEvent<HTMLDivElement>) => void
  onTrimInput: (field: 'start' | 'end', value: string) => void
  onSeekTo: (time: number) => void
  onTogglePlay: () => void
  onTrimStartChange: (value: number) => void
  onTrimEndChange: (value: number) => void
  onApplyTrimWindow: (window: 'setup' | 'spike' | 'payoff' | 'full') => void
}

const CHAPTER_CHIPS: { id: 'setup' | 'spike' | 'payoff' | 'full'; label: string; window: 'setup' | 'spike' | 'payoff' | 'full' }[] = [
  { id: 'setup', label: 'Setup', window: 'setup' },
  { id: 'spike', label: 'Spike', window: 'spike' },
  { id: 'payoff', label: 'Payoff', window: 'payoff' },
  { id: 'full', label: 'Full', window: 'full' },
]

export function ClipTimeline({
  progressRef,
  captions,
  duration,
  currentTime,
  trimStart,
  trimEnd,
  isPlaying,
  job,
  previewRelativeTime,
  previewMode,
  onScrub,
  onTrimInput,
  onSeekTo,
  onTogglePlay,
  onTrimStartChange,
  onTrimEndChange,
  onApplyTrimWindow,
}: ClipTimelineProps) {
  const trimDuration = trimEnd - trimStart
  const spikePos = spikePositionInSource(job, duration)
  const activityBars = useMemo(
    () => buildActivityBars(duration, captions, spikePos),
    [duration, captions, spikePos],
  )

  return (
    <footer className="shrink-0 border-t border-white/[0.08] bg-[#0a0b10]/95 px-4 py-3">
      <div className="mb-2 flex flex-wrap items-center gap-2">
        {CHAPTER_CHIPS.map(chip => (
          <button
            key={chip.id}
            type="button"
            onClick={() => onApplyTrimWindow(chip.window)}
            className="rounded-full border border-white/10 bg-white/[0.04] px-2.5 py-0.5 text-[10px] font-semibold text-zinc-400 transition hover:border-violet-500/30 hover:text-violet-200"
          >
            {chip.label}
          </button>
        ))}
        <div className="ml-auto flex flex-wrap items-center gap-3 text-[10px] text-zinc-500">
          <label className="flex items-center gap-1">
            In
            <input
              type="text"
              value={formatTime(trimStart)}
              onChange={e => onTrimInput('start', e.target.value)}
              onBlur={e => onTrimInput('start', e.target.value)}
              className="w-16 rounded border border-white/10 bg-black/40 px-1.5 py-0.5 font-mono text-zinc-200"
            />
          </label>
          <label className="flex items-center gap-1">
            Out
            <input
              type="text"
              value={formatTime(trimEnd)}
              onChange={e => onTrimInput('end', e.target.value)}
              onBlur={e => onTrimInput('end', e.target.value)}
              className="w-16 rounded border border-white/10 bg-black/40 px-1.5 py-0.5 font-mono text-zinc-200"
            />
          </label>
          <span>{trimDuration.toFixed(1)}s export</span>
          {previewMode === 'source' && <span className="text-cyan-500/70">Loop in trim</span>}
        </div>
      </div>

      {duration > 0 && (
        <div className="mb-2 flex h-8 items-end gap-px rounded bg-black/30 px-1 pb-1" aria-hidden="true">
          {activityBars.map((height, idx) => (
            <div
              key={idx}
              className="flex-1 rounded-sm bg-violet-500/40"
              style={{ height: `${Math.max(8, height * 100)}%` }}
            />
          ))}
        </div>
      )}

      <div
        ref={progressRef as React.Ref<HTMLDivElement>}
        className="relative h-10 cursor-pointer rounded-lg bg-zinc-900/80 ring-1 ring-white/10"
        onClick={onScrub}
      >
        {duration > 0 && captions.map((cap, idx) => (
          <div
            key={idx}
            className="absolute top-1 z-[2] h-2 rounded-sm bg-emerald-500/50"
            style={{
              left: `${(cap.start / duration) * 100}%`,
              width: `${Math.max(0.5, ((cap.end - cap.start) / duration) * 100)}%`,
            }}
            onClick={e => { e.stopPropagation(); onSeekTo(cap.start) }}
            title={cap.text}
          />
        ))}
        {spikePos != null && duration > 0 && (
          <div
            className="absolute top-0 z-[3] h-full w-0.5 bg-rose-400 shadow-[0_0_8px_rgba(251,113,133,0.8)]"
            style={{ left: `${(spikePos / duration) * 100}%` }}
            title="Analytics moment spike"
          />
        )}
        {duration > 0 && (
          <div
            className="absolute top-0 z-[1] h-full rounded bg-cyan-500/15 ring-1 ring-cyan-400/30"
            style={{
              left: `${(trimStart / duration) * 100}%`,
              width: `${((trimEnd - trimStart) / duration) * 100}%`,
            }}
          />
        )}
        <div
          className="absolute top-0 z-[2] h-full w-px bg-cyan-400"
          style={{ left: `${duration ? (currentTime / duration) * 100 : 0}%` }}
        />
        <div
          className="absolute top-1/2 z-[4] h-6 w-1 -translate-y-1/2 cursor-ew-resize rounded bg-cyan-400 shadow-lg"
          style={{ left: `${duration ? (trimStart / duration) * 100 : 0}%`, marginLeft: '-2px' }}
          onMouseDown={e => {
            e.stopPropagation()
            const el = progressRef.current
            const onMove = (mv: MouseEvent) => {
              if (!el || !duration) return
              const rect = el.getBoundingClientRect()
              const pct = Math.max(0, Math.min((mv.clientX - rect.left) / rect.width, trimEnd / duration - 0.01))
              onTrimStartChange(pct * duration)
            }
            const onUp = () => {
              window.removeEventListener('mousemove', onMove)
              window.removeEventListener('mouseup', onUp)
            }
            window.addEventListener('mousemove', onMove)
            window.addEventListener('mouseup', onUp)
          }}
        />
        <div
          className="absolute top-1/2 z-[4] h-6 w-1 -translate-y-1/2 cursor-ew-resize rounded bg-violet-400 shadow-lg"
          style={{ left: `${duration ? (trimEnd / duration) * 100 : 0}%`, marginLeft: '-2px' }}
          onMouseDown={e => {
            e.stopPropagation()
            const el = progressRef.current
            const onMove = (mv: MouseEvent) => {
              if (!el || !duration) return
              const rect = el.getBoundingClientRect()
              const pct = Math.max(trimStart / duration + 0.01, Math.min((mv.clientX - rect.left) / rect.width, 1))
              onTrimEndChange(pct * duration)
            }
            const onUp = () => {
              window.removeEventListener('mousemove', onMove)
              window.removeEventListener('mouseup', onUp)
            }
            window.addEventListener('mousemove', onMove)
            window.addEventListener('mouseup', onUp)
          }}
        />
      </div>

      <div className="mt-2 flex flex-wrap items-center justify-between gap-2 text-[10px] text-zinc-500">
        <span>Playhead {formatTime(currentTime)} / {formatTime(duration)}</span>
        <span>Export {formatTime(trimStart)} → {formatTime(trimEnd)}</span>
        {previewMode === 'source' && <span>Caption rel {previewRelativeTime.toFixed(1)}s</span>}
        <div className="flex items-center gap-1">
          <button type="button" className="rounded p-1.5 text-zinc-400 hover:bg-white/10" onClick={() => onSeekTo(trimStart)} title="Trim start">
            <svg width="14" height="14" fill="currentColor" viewBox="0 0 16 16"><path d="M4 4a.5.5 0 0 1 1 0v3.248l6.267-3.636c.54-.313 1.232.066 1.232.696v7.384c0 .63-.692 1.01-1.232.697L5 8.753V12a.5.5 0 0 1-1 0V4z"/></svg>
          </button>
          <button type="button" className="rounded-full bg-cyan-500/20 p-2 text-cyan-300 hover:bg-cyan-500/30" onClick={onTogglePlay}>
            {isPlaying ? (
              <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M5.5 3.5A1.5 1.5 0 0 1 7 5v6a1.5 1.5 0 0 1-3 0V5a1.5 1.5 0 0 1 1.5-1.5zm5 0A1.5 1.5 0 0 1 12 5v6a1.5 1.5 0 0 1-3 0V5a1.5 1.5 0 0 1 1.5-1.5z"/></svg>
            ) : (
              <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M11.596 8.697l-6.363 3.692c-.54.313-1.233-.066-1.233-.697V4.308c0-.63.692-1.01 1.233-.696l6.363 3.692a.802.802 0 0 1 0 1.393z"/></svg>
            )}
          </button>
          <button type="button" className="rounded p-1.5 text-zinc-400 hover:bg-white/10" onClick={() => onSeekTo(trimEnd)} title="Trim end">
            <svg width="14" height="14" fill="currentColor" viewBox="0 0 16 16"><path d="M12.5 4a.5.5 0 0 0-1 0v3.248L5.233 3.612C4.693 3.3 4 3.678 4 4.308v7.384c0 .63.692 1.01 1.233.697L11.5 8.753V12a.5.5 0 0 0 1 0V4z"/></svg>
          </button>
        </div>
      </div>
    </footer>
  )
}
