import type { AnalyticsStream } from '../../apiTypes.ts'
import { buildTwitchVodUrl, type VodLinkState } from '../../utils/twitchVodUrl.ts'
import { duration, formatDateTime } from '../../utils/consoleFormat.ts'

export function PastBroadcastBanner({
  isLiveRoute,
  isActiveLiveCollector,
  stream,
  syncing = false,
  hasChartData = false,
  vodLinkState,
  sessionStreamId,
  channelLogin,
  buildSessionPath,
}: {
  isLiveRoute: boolean
  isActiveLiveCollector: boolean
  stream?: AnalyticsStream
  syncing?: boolean
  hasChartData?: boolean
  vodLinkState?: VodLinkState
  sessionStreamId?: string
  channelLogin: string
  buildSessionPath: (login: string, streamId: string) => string
}) {
  if (isActiveLiveCollector || syncing || !hasChartData) return null

  const startedLabel = stream?.startedAt ? formatDateTime(stream.startedAt) : null
  const durationLabel = duration(stream)
  const endedLabel = stream?.endedAt ? formatDateTime(stream.endedAt) : null
  const vodUrl = vodLinkState?.status === 'linked' && vodLinkState.vodId
    ? buildTwitchVodUrl(vodLinkState.vodId)
    : null
  const sessionPath = sessionStreamId?.trim()
    ? buildSessionPath(channelLogin, sessionStreamId.trim())
    : null

  if (isLiveRoute) {
    const parts = ['Streamer offline', 'Showing last broadcast']
    if (startedLabel) parts.push(`Started ${startedLabel}`)
    if (durationLabel && durationLabel !== '-') parts.push(durationLabel)

    return (
      <div
        className="rounded border border-white/[0.08] bg-white/[0.025] px-3 py-2.5 text-[11px] font-semibold text-zinc-300"
        role="status"
      >
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span>{parts.join(' · ')}</span>
          {sessionPath ? (
            <a
              href={sessionPath}
              className="rounded border border-white/10 bg-white/[0.04] px-2 py-0.5 text-[10px] font-black uppercase text-zinc-200 transition hover:bg-white/[0.08]"
            >
              Open session page
            </a>
          ) : null}
          {vodUrl ? (
            <a
              href={vodUrl}
              target="_blank"
              rel="noreferrer"
              className="rounded border border-cyan-400/20 bg-cyan-400/10 px-2 py-0.5 text-[10px] font-black uppercase text-cyan-100 transition hover:bg-cyan-400/15"
            >
              Watch VOD
            </a>
          ) : null}
        </div>
      </div>
    )
  }

  const parts = ['Past broadcast']
  if (endedLabel) parts.push(`Ended ${endedLabel}`)
  else if (startedLabel) parts.push(`Started ${startedLabel}`)
  if (durationLabel && durationLabel !== '-') parts.push(durationLabel)

  return (
    <div
      className="rounded border border-white/[0.06] bg-white/[0.02] px-3 py-2 text-[11px] font-semibold text-zinc-400"
      role="status"
    >
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <span>{parts.join(' · ')}</span>
        {vodUrl ? (
          <a
            href={vodUrl}
            target="_blank"
            rel="noreferrer"
            className="rounded border border-cyan-400/20 bg-cyan-400/10 px-2 py-0.5 text-[10px] font-black uppercase text-cyan-100 transition hover:bg-cyan-400/15"
          >
            Watch VOD
          </a>
        ) : null}
      </div>
    </div>
  )
}
