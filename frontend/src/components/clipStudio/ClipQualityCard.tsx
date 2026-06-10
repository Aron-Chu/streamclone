import type { ClipperJob } from '../../api'
import { pickReasonLabel } from './utils'

export function ClipQualityCard({ job }: { job: ClipperJob }) {
  const ctx = job.moment_context
  if (!ctx && !job.reason) return null

  const topEmotes = ctx?.top_emotes?.slice(0, 3) ?? []

  return (
    <div className="clip-quality-card">
      <div className="clip-quality-card-title">Why this moment</div>
      <p className="clip-quality-card-reason" title={job.reason || undefined}>
        {pickReasonLabel(ctx?.pick_reason)}
        {job.reason && ctx?.pick_reason !== job.reason ? ` · ${job.reason}` : ''}
      </p>
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
        {ctx?.moment_score != null && (
          <span>Score <strong>{ctx.moment_score.toFixed(2)}</strong></span>
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
