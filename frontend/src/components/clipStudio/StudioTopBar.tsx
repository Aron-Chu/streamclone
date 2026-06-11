import { Link } from 'react-router-dom'
import type { ClipperJob } from '../../api'
import StackStatusButton from '../StackStatusButton'
import { formatHighlightRange, spikePositionInSource } from './utils'

interface StudioTopBarProps {
  job: ClipperJob
  trimStart: number
  trimEnd: number
  duration: number
  canPreviewSource: boolean
  canPreviewFinal: boolean
  sourceUrl: string
  finalUrl: string
  onExport: () => void
  exportDisabled: boolean
}

export function StudioTopBar({
  job,
  trimStart,
  trimEnd,
  duration,
  canPreviewSource,
  canPreviewFinal,
  sourceUrl,
  finalUrl,
  onExport,
  exportDisabled,
}: StudioTopBarProps) {
  const ctx = job.moment_context
  const streamTime = ctx?.minute_ts
    ? new Date(ctx.minute_ts).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
    : null
  const spikePos = spikePositionInSource(job, duration)
  const showHighlight = spikePos != null

  return (
    <header className="studio-top-bar">
      <div className="studio-top-bar-left">
        <StackStatusButton className="!text-[10px]" />
        <Link to={`/analytics/${job.channel}`} className="clip-studio-back-link">
          <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16">
            <path fillRule="evenodd" d="M15 8a.5.5 0 0 0-.5-.5H2.707l3.147-3.146a.5.5 0 1 0-.708-.708l-4 4a.5.5 0 0 0 0 .708l4 4a.5.5 0 0 0 .708-.708L2.707 8.5H14.5A.5.5 0 0 0 15 8z"/>
          </svg>
          Back
        </Link>
        <div className="studio-top-bar-brand">
          <span className="studio-channel-avatar" aria-hidden="true">
            {job.channel.charAt(0).toUpperCase()}
          </span>
          <div>
            <h1>Clip Studio</h1>
            <span className="studio-top-bar-meta">
              {job.channel}
              {streamTime ? ` · ${streamTime}` : ''}
              <span className="clip-studio-job-id">#{job.id.substring(0, 8)}</span>
            </span>
          </div>
        </div>
      </div>

      {showHighlight && (
        <div className="studio-highlight-pill" title="Trim window around detected moment">
          <span className="studio-highlight-pill-dot" />
          Auto highlight · {formatHighlightRange(trimStart, trimEnd)}
        </div>
      )}

      <div className="studio-top-bar-actions">
        {canPreviewSource && (
          <a href={sourceUrl} className="clip-studio-btn-secondary" download>Source</a>
        )}
        {canPreviewFinal && (
          <a href={finalUrl} className="clip-studio-btn-secondary" download>Final MP4</a>
        )}
        <button className="btn-export-primary" onClick={onExport} disabled={exportDisabled}>
          Export Short
        </button>
      </div>
    </header>
  )
}
