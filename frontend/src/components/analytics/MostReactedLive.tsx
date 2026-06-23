import type { AnalyticsStreamDetail } from '../../api.ts'
import type { ReplayHeatmapPoint } from '../../types/heatmap.ts'
import { Link } from 'react-router-dom'
import { useAnalyticsLive } from '../../hooks/useAnalyticsLive.ts'
import { buildMomentJumpLink } from '@streamclone/pulse-core'
import {
  LIVE_HEAT_COLLECTING_LABEL,
  LIVE_HEAT_MAX_EMOTES,
  LIVE_HEAT_REFRESH_MS,
  LIVE_HEAT_SUBTITLE,
  LIVE_HEAT_TITLE,
  deriveLiveHeat,
  formatHeatOffset,
  type LiveHeatPoint,
} from '@streamclone/pulse-core'

/**
 * MostReactedLive renders the "Most Reacted So Far" section on a live stream's
 * analytics page (Requirement 19.1, 19.2). It shows up to 10 scored moment
 * points computed from live rollups once at least 5 completed minute rollups
 * exist, refreshes every 30 seconds, and carries the honest "based on chat and
 * emote activity" subtitle (never "Most Replayed"). The trailing incomplete
 * minute is rendered muted and labeled "Collecting" to signal its score may
 * change once the minute closes.
 *
 * All ranking lives in @streamclone/pulse-core so it can be unit-tested without
 * rendering.
 */

export interface MostReactedLiveProps {
  login: string
  /** When provided, derive heat locally and skip the live fetch. */
  detail?: AnalyticsStreamDetail
  /** Resolved VOD id, when known, for "Jump back" deep links. */
  vodId?: string
  /** Analytics stream id for sid= deep link (Twitch embed + activity graph). */
  analyticsStreamId?: string
  /** Stream start time for absolute VOD seek offsets. */
  streamStartedAt?: string
  /** Backend replay heatmap points for consistent Top Moments scoring. */
  heatmapPoints?: ReplayHeatmapPoint[]
  /** Disable polling when off-screen or the stream is not live. */
  enabled?: boolean
  className?: string
}

function toLiveHeatInput(detail: AnalyticsStreamDetail, heatmapPoints?: ReplayHeatmapPoint[]) {
  return {
    state: detail.state,
    rollups: detail.rollups ?? [],
    topEmotes: detail.topEmotes ?? [],
    heatmapPoints,
    streamStartedAt: detail.stream?.startedAt,
  }
}

function ScoreBadge({ score, estimated, muted }: { score: number; estimated?: boolean; muted?: boolean }) {
  return (
    <span
      className={`min-w-[2.25rem] rounded-md px-1.5 py-0.5 text-center text-xs font-black tabular-nums ${
        muted
          ? 'bg-white/5 text-zinc-500'
          : 'bg-violet-500/15 text-violet-200 border border-violet-400/30'
      }`}
      title={estimated ? 'Estimated from local rollups until heatmap scoring is available.' : 'Backend replay heatmap score.'}
    >
      {estimated ? `~${score}` : score}
    </span>
  )
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

function MomentRow({
  point,
  login,
  vodId,
  analyticsStreamId,
}: {
  point: LiveHeatPoint
  login: string
  vodId?: string
  analyticsStreamId?: string
}) {
  const offsetLabel = formatHeatOffset(point.offsetSeconds)
  const canJump = !point.collecting && Boolean(login)

  const body = (
    <div
      className={`flex items-center justify-between gap-3 rounded-lg border px-3 py-2 transition ${
        point.collecting
          ? 'border-white/5 bg-white/[0.02] opacity-60'
          : canJump
            ? 'border-white/10 bg-white/5 hover:border-violet-300/40 hover:bg-white/[0.07] cursor-pointer'
            : 'border-white/10 bg-white/5'
      }`}
    >
      <div className="flex min-w-0 items-center gap-3">
        <ScoreBadge score={point.score} estimated={point.estimated} muted={point.collecting} />
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

  const jumpHref = buildMomentJumpLink(login, point.offsetSeconds, { vodId, analyticsStreamId })
  const jumpLabel = vodId
    ? `Play moment in Streamclone at ${offsetLabel}`
    : `Open analytics at ${offsetLabel}`

  return (
    <Link
      to={jumpHref}
      className="block focus:outline-none focus-visible:ring-2 focus-visible:ring-violet-400/60"
      aria-label={`${jumpLabel}, score ${point.score}, ${point.reasonLabel}`}
    >
      {body}
    </Link>
  )
}

export function MostReactedLive({
  login,
  detail,
  vodId,
  analyticsStreamId,
  heatmapPoints,
  enabled = true,
  className,
}: MostReactedLiveProps) {
  const selfFetch = detail === undefined
  const query = useAnalyticsLive(login, {
    enabled: Boolean(login) && enabled && selfFetch,
    refetchInterval: LIVE_HEAT_REFRESH_MS,
  })

  const resolvedDetail = detail ?? query.data
  if (!resolvedDetail) return null

  const heat = deriveLiveHeat(toLiveHeatInput(resolvedDetail, heatmapPoints))

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
          <MomentRow key={point.minuteTs} point={point} login={login} vodId={vodId} analyticsStreamId={analyticsStreamId} />
        ))}
        {heat.collectingPoint && (
          <MomentRow point={heat.collectingPoint} login={login} vodId={vodId} analyticsStreamId={analyticsStreamId} />
        )}
      </div>
    </section>
  )
}

export default MostReactedLive
