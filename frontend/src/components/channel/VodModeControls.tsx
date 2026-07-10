import { Link } from 'react-router-dom'

// Placeholder rendered when total VOD duration (or current time) is not yet
// known to the player. Requirement 20.3.
export const VOD_DURATION_PLACEHOLDER = '--:--:--'

/**
 * Formats whole seconds as HH:MM:SS (minutes/seconds zero-padded to 2 digits,
 * hours at least 2 digits). Matches `^\d{2,}:\d{2}:\d{2}$`. Requirement 20.2.
 *
 * A dedicated shared duration formatter is the scope of task 13.6; this small
 * local helper is used here to keep VodModeControls self-contained until that
 * shared utility lands.
 */
export function formatVodTimestamp(totalSeconds: number | null | undefined): string {
  if (totalSeconds == null || !Number.isFinite(totalSeconds)) {
    return VOD_DURATION_PLACEHOLDER
  }
  const s = Math.max(0, Math.floor(totalSeconds))
  const hh = Math.floor(s / 3600)
  const mm = Math.floor((s % 3600) / 60)
  const ss = s % 60
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${pad(hh)}:${pad(mm)}:${pad(ss)}`
}

export interface VodModeControlsProps {
  /** Normalized VOD identifier (`^\d{5,20}$`). */
  vodId: string
  /** Requested deep-link offset in seconds (used for the banner label). */
  offsetSeconds: number
  /** Channel login; "Back to live channel" navigates to `/c/{login}`. */
  channelLogin: string
  /** Current playback position in seconds, or null when unknown. */
  currentTimeSec: number | null
  /** Total VOD duration in seconds, or null when unknown (placeholder shown). */
  totalDurationSec: number | null
  /** Full chat log page for this stream, when available. */
  chatLogHref?: string | null
  /** True while the relay is repositioning after a far seek. */
  seekPending?: boolean
  className?: string
}

/**
 * VodModeControls renders the VOD review-mode banner shown in the channel
 * workspace when a `?vod=&offset=` deep link is active.
 */
export function VodModeControls({
  vodId,
  offsetSeconds,
  channelLogin,
  currentTimeSec,
  totalDurationSec,
  chatLogHref,
  seekPending = false,
  className,
}: VodModeControlsProps) {
  const offsetLabel = formatVodTimestamp(offsetSeconds)
  const currentLabel = formatVodTimestamp(currentTimeSec)
  const totalLabel = formatVodTimestamp(totalDurationSec)

  return (
    <div
      role="status"
      aria-label={`VOD review mode for video ${vodId} at offset ${offsetLabel}`}
      className={
        className ??
        'pointer-events-auto flex flex-wrap items-center gap-x-3 gap-y-2 rounded-xl border border-violet-400/30 bg-zinc-950/85 px-3 py-2 text-xs font-bold text-zinc-200 shadow-lg shadow-black/40 backdrop-blur-md'
      }
    >
      <span className="rounded bg-violet-500/20 px-2 py-1 text-[10px] font-black uppercase tracking-wide text-violet-100">
        VOD mode
      </span>
      {seekPending ? (
        <span className="flex items-center gap-1.5 rounded bg-amber-500/20 px-2 py-1 text-[10px] font-black uppercase tracking-wide text-amber-100">
          <span className="inline-block h-2.5 w-2.5 animate-spin rounded-full border border-amber-200/30 border-t-amber-100" aria-hidden />
          Repositioning…
        </span>
      ) : null}
      <span className="font-mono text-zinc-300" title="Twitch VOD identifier">
        #{vodId}
      </span>
      <span className="text-zinc-500" aria-hidden>
        •
      </span>
      <span className="text-zinc-400">
        Offset <span className="font-mono text-zinc-200">{offsetLabel}</span>
      </span>
      <span
        className="font-mono text-zinc-200"
        aria-label={`Playback ${currentLabel} of ${totalLabel}`}
        title="Current playback time / total VOD duration"
      >
        {currentLabel} / {totalLabel}
      </span>
      <div className="ml-auto flex flex-wrap items-center gap-2">
        <Link
          to={`/c/${encodeURIComponent(channelLogin)}`}
          className="rounded border border-white/15 bg-white/5 px-3 py-1.5 text-[11px] font-black text-zinc-100 transition hover:bg-white/10 focus:outline-none focus-visible:ring-2 focus-visible:ring-violet-400"
        >
          Back to live channel
        </Link>
        {chatLogHref ? (
          <Link
            to={chatLogHref}
            className="rounded border border-cyan-400/30 bg-cyan-500/15 px-3 py-1.5 text-[11px] font-black text-cyan-100 transition hover:bg-cyan-500/25 focus:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400"
          >
            Chat log
          </Link>
        ) : null}
      </div>
    </div>
  )
}

export default VodModeControls
