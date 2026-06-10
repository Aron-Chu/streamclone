import type { ClipperJob } from '../../api'
import type { RenderStatus } from './types'

const PIPELINE_STEPS = [
  'queued',
  'creating_clip',
  'waiting_for_clip',
  'downloading',
  'transcribing',
  'rendering',
  'ready',
] as const

function stepIndex(state: string): number {
  const idx = PIPELINE_STEPS.indexOf(state as typeof PIPELINE_STEPS[number])
  return idx >= 0 ? idx : 0
}

interface JobProgressOverlayProps {
  job: ClipperJob
  renderStatus: RenderStatus
  renderErrorMsg: string
  isTranscribing: boolean
  canPreviewFinal: boolean
  finalUrl: string
  failureMessage?: string
  onRetry?: () => void
}

export function JobProgressOverlay({
  job,
  renderStatus,
  renderErrorMsg,
  isTranscribing,
  canPreviewFinal,
  finalUrl,
  failureMessage = '',
  onRetry,
}: JobProgressOverlayProps) {
  const activeState = isTranscribing ? 'transcribing' : renderStatus === 'rendering' ? 'rendering' : job.state
  const currentIdx = stepIndex(activeState)
  const showPipeline = job.state !== 'ready' && job.state !== 'failed' && job.state !== 'purged'
    || renderStatus === 'rendering'
    || isTranscribing
  const showFailed = job.state === 'failed' || renderStatus === 'failed'

  if (!showPipeline && !showFailed && renderStatus === 'idle') return null

  return (
    <div className="job-progress-overlay">
      {showPipeline ? (
        <div className="job-progress-steps">
          {PIPELINE_STEPS.map((step, idx) => (
            <div
              key={step}
              className={`job-progress-step ${idx < currentIdx ? 'done' : ''} ${idx === currentIdx ? 'active' : ''}`}
            >
              <span className="job-progress-dot" />
              <span className="job-progress-label">{step.replace(/_/g, ' ')}</span>
            </div>
          ))}
        </div>
      ) : null}
      {showFailed ? (
        <div className="clip-studio-progress-card">
          <span className="progress-status progress-status-failed">Clip job failed</span>
          <p className="progress-error">{failureMessage || renderErrorMsg || 'Processing failed'}</p>
          {onRetry ? (
            <button type="button" className="btn-export" onClick={onRetry}>
              Retry clip job
            </button>
          ) : null}
        </div>
      ) : null}
      {renderStatus !== 'idle' && renderStatus !== 'failed' && (
        <div className="clip-studio-progress-card">
          <span className={`progress-status progress-status-${renderStatus}`}>
            {renderStatus === 'rendering' && 'Rendering on server...'}
            {renderStatus === 'success' && 'Render complete'}
          </span>
          {renderStatus === 'success' && canPreviewFinal && (
            <a href={finalUrl} className="btn-download" download>Download rendered MP4</a>
          )}
        </div>
      )}
    </div>
  )
}
