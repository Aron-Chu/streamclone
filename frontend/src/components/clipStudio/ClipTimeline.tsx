import type { RefObject } from 'react'
import type { CaptionWord, ClipperJob } from '../../api'
import { formatTime, spikePositionInSource } from './utils'

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
}

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
}: ClipTimelineProps) {
  const trimDuration = trimEnd - trimStart
  const spikePos = spikePositionInSource(job, duration)

  return (
    <footer className="clip-timeline">
      <div className="timeline-trim-inputs">
        <label>
          In
          <input
            type="text"
            value={formatTime(trimStart)}
            onChange={e => onTrimInput('start', e.target.value)}
            onBlur={e => onTrimInput('start', e.target.value)}
          />
        </label>
        <label>
          Out
          <input
            type="text"
            value={formatTime(trimEnd)}
            onChange={e => onTrimInput('end', e.target.value)}
            onBlur={e => onTrimInput('end', e.target.value)}
          />
        </label>
        <span className="timeline-duration-label">Duration: {trimDuration.toFixed(1)}s</span>
      </div>

      <div className="timeline-scrubber-wrapper" ref={progressRef as React.Ref<HTMLDivElement>} onClick={onScrub}>
        <div className="timeline-scrubber">
          {duration > 0 && captions.map((cap, idx) => (
            <div
              key={idx}
              className="timeline-caption-block"
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
              className="timeline-spike-marker"
              style={{ left: `${(spikePos / duration) * 100}%` }}
              title="Analytics moment spike"
            />
          )}
          <div
            className="timeline-trim-region"
            style={{
              left: `${(trimStart / duration) * 100}%`,
              width: `${((trimEnd - trimStart) / duration) * 100}%`,
            }}
          />
          <div className="timeline-progress" style={{ width: `${duration ? (currentTime / duration) * 100 : 0}%` }} />
          <div className="timeline-playhead" style={{ left: `${duration ? (currentTime / duration) * 100 : 0}%` }} />
          <div
            className="trim-handle trim-handle-left"
            style={{ left: `${duration ? (trimStart / duration) * 100 : 0}%` }}
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
            className="trim-handle trim-handle-right"
            style={{ left: `${duration ? (trimEnd / duration) * 100 : 0}%` }}
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
      </div>

      <div className="timeline-stats">
        <span>Playhead: {formatTime(currentTime)} / {formatTime(duration)}</span>
        <span>Export window: {formatTime(trimStart)} → {formatTime(trimEnd)}</span>
        {previewMode === 'source' && <span>Caption rel: {previewRelativeTime.toFixed(1)}s</span>}
      </div>

      <div className="timeline-controls">
        <button className="btn-circle" onClick={() => onSeekTo(trimStart)} title="Go to trim start">
          <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M4 4a.5.5 0 0 1 1 0v3.248l6.267-3.636c.54-.313 1.232.066 1.232.696v7.384c0 .63-.692 1.01-1.232.697L5 8.753V12a.5.5 0 0 1-1 0V4z"/></svg>
        </button>
        <button className="btn-circle play" onClick={onTogglePlay}>
          {isPlaying ? (
            <svg width="18" height="18" fill="currentColor" viewBox="0 0 16 16"><path d="M5.5 3.5A1.5 1.5 0 0 1 7 5v6a1.5 1.5 0 0 1-3 0V5a1.5 1.5 0 0 1 1.5-1.5zm5 0A1.5 1.5 0 0 1 12 5v6a1.5 1.5 0 0 1-3 0V5a1.5 1.5 0 0 1 1.5-1.5z"/></svg>
          ) : (
            <svg width="18" height="18" fill="currentColor" viewBox="0 0 16 16" style={{ marginLeft: '2px' }}><path d="M11.596 8.697l-6.363 3.692c-.54.313-1.233-.066-1.233-.697V4.308c0-.63.692-1.01 1.233-.696l6.363 3.692a.802.802 0 0 1 0 1.393z"/></svg>
          )}
        </button>
        <button className="btn-circle" onClick={() => onSeekTo(trimEnd)} title="Go to trim end">
          <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M12.5 4a.5.5 0 0 0-1 0v3.248L5.233 3.612C4.693 3.3 4 3.678 4 4.308v7.384c0 .63.692 1.01 1.233.697L11.5 8.753V12a.5.5 0 0 0 1 0V4z"/></svg>
        </button>
      </div>
    </footer>
  )
}
