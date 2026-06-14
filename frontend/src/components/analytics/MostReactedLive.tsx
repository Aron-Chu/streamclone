import { useQuery } from '@tanstack/react-query'

import { getAnalyticsLive, type AnalyticsStreamDetail } from '../../api.ts'
import { buildVodDeepLink } from '../../utils/vodDeepLink.ts'
import {
  LIVE_HEAT_COLLECTING_LABEL,
  LIVE_HEAT_MAX_EMOTES,
  LIVE_HEAT_REFRESH_MS,
  LIVE_HEAT_SUBTITLE,
  LIVE_HEAT_TITLE,
  deriveLiveHeat,
  formatHeatOffset,
  type LiveHeatPoint,
} from '../../utils/liveHeat.ts'

/**
 * MostReactedLive renders the "Most Reacted So Far" section on a live stream's
 * analytics page (Requirement 19.1, 19.2). It shows up to 10 scored moment
 * points computed from live rollups once at least 5 completed minute rollups
 * exist, refreshes every 30 seconds, and carries the honest "based on chat and
 * emote activity" subtitle (never "Most Replayed"). The trailing incomplete
 * minute is rendered muted and labeled "Collecting" to signal its score may
 * change once the minute closes.
 *
 * All ranking lives in ../../utils/liveHeat.ts so it can be unit-tested without
 * rendering.
 */

export interface MostReactedLiveProps {
  login: string
  /** Resolved VOD id, when known, for "Jump back" deep links. */
  vodId?: string
  /** Disable polling when off-screen or the stream is not live. */
  enabled?: boolean
  className?: string
}

function toLiveHeatInput(detail: AnalyticsStreamDetail) {
  return {
    state: detail.state,
    rollups: detail.rollups ?? [],
    topEmotes: detail.topEmotes ?? [],
  }
}

function EmoteStack({ point }: { point: LiveHeatPoint }) {
  if (point.topEmotes.length === 0) return null
  return (
    <div className="flex items-center gap-1">
      {point.topEmotes.slice(0, LIVE_HEAT_MAX_EMOTES).map(emote => (
        <span
          key={emote.key}
          className="flex items-center"
          title={`${emote.name}${emote.provider ? ` · ${emote.provider}` : ''} · ${emote.count.toLocaleString()}`}
        >
          {emote.imageUrl ? (
            <img
              src={emote.imageUrl}
              alt={emote.name}
              width={20}
              height={20}
              className="h-5 w-5 object-contain"
              loading="lazy"
            />
          ) : (
            <span className="text-[11px] font-semibold text-zinc-300">{emote.name}</span>
          )}
        </span>
      ))}
    </div>
  )
}

function ScoreBadge({ score, muted }: { score: number; muted?: boolean }) {
  return (
    <span
      className={`min-w-[2.25rem] rounded-md px-1.5 py-0.5 text-center text-xs font-black tabular-nums ${
        muted
          ? 'bg-white/5 text-zinc-500'
          : 'bg-violet-500/15 text-violet-200 border border-violet-400/30'
      }`}
    >
      {score}
    </span>
  )
}

function MomentRow({
  point,
  login,
  vodId,
}: {
  point: LiveHeatPoint
  login: string
  vodId?: string
}) {
  const offsetLabel = formatHeatOffset(point.offsetSeconds)
  const canJump = Boolean(vodId)

  const body = (
    <div
      className={`flex items-center justify-between gap-3 rounded-lg border px-3 py-2 ${
        point.collecting
          ? 'border-white/5 bg-white/[0.02] opacity-60'
          : 'border-white/10 bg-white/5 hover:border-violet-400/40'
      }`}
    >
      <div className="flex min-w-0 items-center gap-3">
        <ScoreBadge score={point.score} muted={point.collecting} />
        <div className="flex min-w-0 flex-col">
          <span className="flex items-center gap-2 text-xs font-bold text-zinc-200">
            <span className="tabular-nums text-zinc-400">{offsetLabel}</span>
            {point.collecting ? (
              <span className="rounded-full border border-amber-400/30 bg-amber-500/10 px-2 py-0.5 text-[9px] font-black uppercase tracking-wide text-amber-300">
                {LIVE_HEAT_COLLECTING_LABEL}
              </span>
            ) : (
              <span className="text-[11px] font-semibold text-zinc-400">{point.reasonLabel}</span>
            )}
          </span>
          <span className="text-[11px] font-semibold text-zinc-500">
            {point.chatCount.toLocaleString()} chat · {point.emoteCount.toLocaleString()} emotes
          </span>
        </div>
      </div>
      <EmoteStack point={point} />
    </div>
  )

  if (point.collecting || !canJump) {
    return body
  }

  return (
    <a
      href={buildVodDeepLink(login, vodId as string, point.offsetSeconds)}
      className="block focus:outline-none focus-visible:ring-2 focus-visible:ring-violet-400/60"
      aria-label={`Play moment in Streamclone at ${offsetLabel}, score ${point.score}, ${point.reasonLabel}`}
    >
      {body}
    </a>
  )
}

export function MostReactedLive({ login, vodId, enabled = true, className }: MostReactedLiveProps) {
  const query = useQuery({
    queryKey: ['analytics-live-heat', login],
    queryFn: () => getAnalyticsLive(login),
    enabled: Boolean(login) && enabled,
    refetchInterval: LIVE_HEAT_REFRESH_MS,
    retry: false,
  })

  if (!query.data) return null

  const heat = deriveLiveHeat(toLiveHeatInput(query.data))

  // Section only appears once enough completed rollups exist (Req 19.1).
  if (!heat.visible) return null

  return (
    <section
      className={
        className ??
        'flex flex-col gap-3 rounded-xl border border-white/10 bg-zinc-950/60 p-4'
      }
      aria-label={LIVE_HEAT_TITLE}
    >
      <div className="flex flex-col gap-0.5">
        <h3 className="text-sm font-black uppercase tracking-wide text-zinc-200">
          {LIVE_HEAT_TITLE}
        </h3>
        <p className="text-[11px] font-semibold text-zinc-500">{LIVE_HEAT_SUBTITLE}</p>
      </div>

      <div className="flex flex-col gap-2">
        {heat.points.map(point => (
          <MomentRow key={point.minuteTs} point={point} login={login} vodId={vodId} />
        ))}
        {heat.collectingPoint && (
          <MomentRow point={heat.collectingPoint} login={login} vodId={vodId} />
        )}
      </div>
    </section>
  )
}

export default MostReactedLive
