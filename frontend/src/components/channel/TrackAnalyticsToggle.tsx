export interface TrackAnalyticsToggleProps {
  tracked: boolean
  pending: boolean
  onToggle: (next: boolean) => void
  prominent?: boolean
  /** Shown when analytics collection is off (Pulse uses “Track streamer”). */
  trackLabel?: string
  /** Shown when collection is on. */
  trackingLabel?: string
}

export default function TrackAnalyticsToggle({
  tracked,
  pending,
  onToggle,
  prominent = false,
  trackLabel = 'Track analytics',
  trackingLabel = 'Tracking analytics',
}: TrackAnalyticsToggleProps) {
  return (
    <button
      type="button"
      disabled={pending}
      onClick={() => onToggle(!tracked)}
      className={`rounded font-black uppercase tracking-wide transition disabled:opacity-60 ${
        prominent ? 'px-4 py-2 text-xs' : 'px-2.5 py-1 text-[11px]'
      } ${
        tracked
          ? 'bg-violet-600 text-white hover:bg-violet-500'
          : 'border border-violet-400/30 bg-violet-500/10 text-violet-200 hover:border-violet-300/50'
      }`}
      title={
        tracked
          ? 'Live chat and emote rollups are collected for this stream. Click to stop tracking.'
          : 'Collect live chat, emote spikes, and minute rollups for this stream.'
      }
    >
      {pending ? 'Saving…' : tracked ? trackingLabel : trackLabel}
    </button>
  )
}
