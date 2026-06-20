import type { PulseWireTimelineStep } from '../../pulseWireApi'
import { formatRelativeTime, formatTimelineTime } from '../../utils/pulseWireFormat'

const DOT: Record<string, string> = {
  pulse_origin: '#9147FF',
  reddit_thread: '#FF4500',
  youtube_video: '#FF0000',
  twitch_clip: '#9147FF',
  x_post: '#1D9BF0',
  tiktok_video: '#00D7B0',
}

type Props = {
  timeline?: PulseWireTimelineStep[]
  showTimestamps?: boolean
  className?: string
}

export default function SpreadTimeline({ timeline, showTimestamps = true, className = '' }: Props) {
  if (!timeline?.length) return null
  return (
    <div className={className}>
      <p className="mb-2 text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Spread timeline</p>
      <div className="flex flex-wrap items-center gap-2">
        {timeline.map((step, index) => {
          const stepBody = (
            <>
              <span
                className="h-2 w-2 rounded-full"
                style={{ backgroundColor: DOT[step.sourceType] ?? '#7A7A85' }}
              />
              <span>{step.label}</span>
              {showTimestamps && step.at ? (
                <span className="text-[10px] text-[#7A7A85]" title={new Date(step.at).toLocaleString()}>
                  {formatTimelineTime(step.at)} · {formatRelativeTime(step.at)}
                </span>
              ) : null}
            </>
          )
          return (
            <span key={`${step.sourceType}-${step.at}-${index}`} className="inline-flex items-center gap-2">
              {index > 0 ? <span className="text-sm text-[#7A7A85]">→</span> : null}
              {step.sourceUrl ? (
                <a
                  href={step.sourceUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-2 rounded-full border border-[#2A2A2E] bg-[#1B1B1F] px-2.5 py-1 text-xs font-medium text-[#D6D6DE] transition hover:border-[#A970FF]/40"
                >
                  {stepBody}
                </a>
              ) : (
                <span className="inline-flex items-center gap-2 rounded-full border border-[#2A2A2E] bg-[#1B1B1F] px-2.5 py-1 text-xs font-medium text-[#D6D6DE]">
                  {stepBody}
                </span>
              )}
            </span>
          )
        })}
      </div>
    </div>
  )
}
