import { Link } from 'react-router-dom'
import type { ClipperJob } from '../../api'

interface StudioTopBarProps {
  job: ClipperJob
  canPreviewSource: boolean
  canPreviewFinal: boolean
  sourceUrl: string
  finalUrl: string
  onExport: () => void
  exportDisabled: boolean
}

export function StudioTopBar({
  job,
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

  return (
    <header className="studio-top-bar">
      <div className="studio-top-bar-left">
        <Link to={`/analytics/${job.channel}`} className="clip-studio-back-link">
          <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16">
            <path fillRule="evenodd" d="M15 8a.5.5 0 0 0-.5-.5H2.707l3.147-3.146a.5.5 0 1 0-.708-.708l-4 4a.5.5 0 0 0 0 .708l4 4a.5.5 0 0 0 .708-.708L2.707 8.5H14.5A.5.5 0 0 0 15 8z"/>
          </svg>
          Back
        </Link>
        <div className="studio-top-bar-brand">
          <h1>Clip Studio</h1>
          <span className="studio-top-bar-meta">
            {job.channel}
            {streamTime ? ` · ${streamTime}` : ''}
            <span className="clip-studio-job-id">#{job.id.substring(0, 8)}</span>
          </span>
        </div>
      </div>
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
