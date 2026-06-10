import type { ClipperJob } from '../../api'
import { hookStrengthScore, pickReasonLabel } from './utils'

export function ClipQualityCard({ job }: { job: ClipperJob }) {
  const ctx = job.moment_context
  if (!ctx && !job.reason) return null

  const topEmotes = ctx?.top_emotes?.slice(0, 3) ?? []
  const hookScore = hookStrengthScore(ctx)

  return (
    <div className="clip-quality-card studio-panel-card">
      <div className="clip-quality-card-title">Moment insight</div>
      <p className="clip-quality-card-reason" title={job.reason || undefined}>
        {pickReasonLabel(ctx?.pick_reason)}
        {job.reason && ctx?.pick_reason !== job.reason ? ` · ${job.reason}` : ''}
      </p>
      {hookScore != null && (
        <div className="hook-strength-meter">
          <div className="hook-strength-header">
            <span>Hook strength</span>
            <strong>{hookScore}/100</strong>
          </div>
          <div className="hook-strength-track">
            <div className="hook-strength-fill" style={{ width: `${hookScore}%` }} />
          </div>
        </div>
      )}
      <div className="clip-quality-card-stats">
        {ctx?.viewer_count != null && (
          <span><strong>{ctx.viewer_count}</strong> viewers</span>
        )}
        {ctx?.chat_per_min != null && (
          <span><strong>{ctx.chat_per_min}</strong> chat/min</span>
        )}
        {ctx?.emote_per_min != null && (
          <span><strong>{ctx.emote_per_min}</strong> emotes/min</span>
        )}
        {ctx?.chat_multiplier != null && (
          <span><strong>{ctx.chat_multiplier.toFixed(1)}x</strong> chat vs baseline</span>
        )}
      </div>
      {topEmotes.length > 0 && (
        <div className="clip-quality-card-emotes">
          {topEmotes.map(emote => (
            <span key={emote.name} className="clip-quality-emote-chip" title={`${emote.name} ×${emote.count}`}>
              {emote.image_url || emote.imageUrl ? (
                <img src={emote.image_url || emote.imageUrl} alt={emote.name} />
              ) : null}
              {emote.name} ×{emote.count}
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
