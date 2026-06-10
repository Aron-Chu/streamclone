import type { CaptionPreset, ClipperJob } from '../../api'
import { ClipQualityCard } from './ClipQualityCard'
import { formatHighlightRange, formatTime } from './utils'

interface ClipDetailsPanelProps {
  job: ClipperJob
  trimStart: number
  trimEnd: number
  trimDuration: number
  captionPreset: CaptionPreset
  layout: string
  captionsCount: number
  isTranscribing: boolean
  onToggleCaptions: (enabled: boolean) => void
  onToggleFacecamFocus: (enabled: boolean) => void
  onCenterOnSpike: () => void
  onApplyDurationPreset: (seconds: number) => void
  onOpenCaptionsTab: () => void
}

function ToggleSwitch({
  checked,
  onChange,
  label,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  label: string
}) {
  return (
    <label className="studio-toggle-row">
      <span>{label}</span>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        className={`studio-toggle ${checked ? 'on' : ''}`}
        onClick={() => onChange(!checked)}
      >
        <span className="studio-toggle-knob" />
      </button>
    </label>
  )
}

export function ClipDetailsPanel({
  job,
  trimStart,
  trimEnd,
  trimDuration,
  captionPreset,
  layout,
  captionsCount,
  isTranscribing,
  onToggleCaptions,
  onToggleFacecamFocus,
  onCenterOnSpike,
  onApplyDurationPreset,
  onOpenCaptionsTab,
}: ClipDetailsPanelProps) {
  const ctx = job.moment_context
  const streamDate = ctx?.minute_ts
    ? new Date(ctx.minute_ts).toLocaleDateString([], { month: 'short', day: 'numeric', year: 'numeric' })
    : null
  const streamTime = ctx?.minute_ts
    ? new Date(ctx.minute_ts).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
    : null
  const title = job.title || `${job.channel} highlight`

  return (
    <aside className="clip-details-panel">
      <div className="studio-panel-card">
        <div className="studio-panel-card-title">Clip details</div>
        <h2 className="clip-details-title">{title}</h2>
        <div className="clip-details-meta">
          <span>{job.channel}</span>
          {streamDate && <span>{streamDate}{streamTime ? ` · ${streamTime}` : ''}</span>}
        </div>
        <dl className="clip-details-times">
          <div><dt>In</dt><dd>{formatTime(trimStart)}</dd></div>
          <div><dt>Out</dt><dd>{formatTime(trimEnd)}</dd></div>
          <div><dt>Duration</dt><dd>{trimDuration.toFixed(1)}s</dd></div>
        </dl>
        <div className="clip-details-range">{formatHighlightRange(trimStart, trimEnd)}</div>
      </div>

      <ClipQualityCard job={job} />

      <div className="studio-panel-card">
        <div className="studio-panel-card-title">Quick actions</div>
        <ToggleSwitch
          label="Captions"
          checked={captionPreset !== 'none'}
          onChange={onToggleCaptions}
        />
        <ToggleSwitch
          label="Facecam focus"
          checked={layout === 'stacked_game_face'}
          onChange={onToggleFacecamFocus}
        />
        <button type="button" className="studio-quick-action-btn" onClick={onCenterOnSpike}>
          Center on spike
        </button>
        <div className="clip-details-duration-row">
          {[18, 30, 45].map(sec => (
            <button
              key={sec}
              type="button"
              className={`clip-details-duration-btn ${Math.abs(trimDuration - sec) < 0.5 ? 'active' : ''}`}
              onClick={() => onApplyDurationPreset(sec)}
            >
              {sec}s
            </button>
          ))}
        </div>
      </div>

      <div className="studio-panel-card studio-caption-status">
        <div className="studio-panel-card-title">Caption status</div>
        <div className="caption-status-row">
          {isTranscribing ? (
            <span className="caption-status-badge transcribing">Transcribing…</span>
          ) : (
            <span className="caption-status-badge">
              {captionsCount > 0 ? `${captionsCount} lines` : 'No captions yet'}
            </span>
          )}
        </div>
        <button type="button" className="studio-quick-action-btn" onClick={onOpenCaptionsTab}>
          Edit captions
        </button>
      </div>
    </aside>
  )
}
