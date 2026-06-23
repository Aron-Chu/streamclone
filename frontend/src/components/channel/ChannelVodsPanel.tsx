import { Link } from 'react-router-dom'

import type { AnalyticsStream, SourceStatus, StreamStat } from '../../api'
import { buildVodDeepLink } from '@streamclone/pulse-core'
import { buildTwitchVodUrl } from '../../utils/twitchVodUrl'
import { vodThumbnailUrl } from '../../utils/vodThumbnail'
import {
  resolveStreamAnalyticsStatus,
  streamAnalyticsStatusClass,
  streamAnalyticsStatusLabel,
} from '../../utils/streamAnalyticsStatus'

function fullCount(value: number | undefined) {
  if (value == null || !Number.isFinite(value)) return '—'
  return value.toLocaleString()
}

function count(value: number | undefined) {
  if (value == null || !Number.isFinite(value)) return '—'
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return value.toLocaleString()
}

function EmptyPanel({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="grid min-h-32 place-items-center rounded border border-white/10 bg-white/[0.035] px-4 text-center">
      <div>
        <div className="text-sm font-black text-zinc-200">{title}</div>
        <div className="mt-1 text-xs font-semibold text-zinc-500">{detail}</div>
      </div>
    </div>
  )
}

export interface ChannelVodsPanelProps {
  rows: StreamStat[] | undefined
  analyticsStreams: AnalyticsStream[] | undefined
  liveStreamId?: string | null
  sources?: SourceStatus[]
  channel: string
}

export function ChannelVodsPanel({
  rows,
  analyticsStreams,
  liveStreamId,
  sources,
  channel,
}: ChannelVodsPanelProps) {
  if (!rows?.length) {
    const historySource = sources?.find(source => source.source === 'stream_history')
    return (
      <EmptyPanel
        title="No stream history yet"
        detail={historySource ? historySource.message || 'Stream rows load from Twitch Helix when available.' : 'Open Full Analytics to sync historical sessions.'}
      />
    )
  }

  const analyticsByStreamId = new Map((analyticsStreams ?? []).map(stream => [stream.streamId, stream]))

  return (
    <div className="space-y-3">
      <p className="text-xs font-semibold leading-relaxed text-zinc-500">
        VODs open in Streamclone when a VOD ID is available; Twitch remains available as a fallback.
      </p>
      <div className="overflow-x-auto rounded border border-white/10 bg-white/[0.035]">
        <div className="grid min-w-[1040px] grid-cols-[minmax(300px,1.45fr)_88px_88px_88px_88px_120px_minmax(200px,1fr)] gap-3 border-b border-white/10 px-3 py-2 text-[11px] font-black uppercase text-zinc-500">
          <span>Stream</span>
          <span>Duration</span>
          <span>Average</span>
          <span>Peak</span>
          <span>Watched</span>
          <span>Analytics</span>
          <span>Actions</span>
        </div>
        {rows.map(row => {
          const status = resolveStreamAnalyticsStatus(row.id, analyticsStreams, liveStreamId)
          const thumb = vodThumbnailUrl(row.thumbnailUrl || analyticsByStreamId.get(row.id)?.thumbnailUrl)
          return (
            <div
              key={row.id}
              className="grid min-w-[1040px] grid-cols-[minmax(300px,1.45fr)_88px_88px_88px_88px_120px_minmax(200px,1fr)] gap-3 border-b border-white/5 px-3 py-3 text-sm font-bold text-zinc-300 transition last:border-b-0 hover:bg-white/[0.05]"
            >
              <div className="flex min-w-0 items-center gap-3">
                <div className="grid aspect-video w-24 shrink-0 place-items-center overflow-hidden rounded border border-white/10 bg-zinc-900 text-[10px] font-black uppercase text-zinc-600">
                  {thumb ? (
                    <img
                      src={thumb}
                      alt=""
                      className="h-full w-full object-cover"
                      loading="lazy"
                      decoding="async"
                    />
                  ) : (
                    <span>VOD</span>
                  )}
                </div>
                <div className="min-w-0">
                  <div className="truncate font-black text-white">{row.title}</div>
                  <div className="mt-0.5 truncate text-xs text-zinc-500">{row.category || row.id}</div>
                </div>
              </div>
              <span className="text-xs">{row.durationMinutes ? `${fullCount(row.durationMinutes)}m` : '—'}</span>
              <span>{fullCount(row.avgViewers)}</span>
              <span>{fullCount(row.peakViewers)}</span>
              <span>{count(row.hoursWatched)}</span>
              <span>
                <span
                  className={`inline-flex rounded border px-2 py-0.5 text-[10px] font-black uppercase ${streamAnalyticsStatusClass(status)}`}
                  title={
                    status === 'synced'
                      ? 'Minute-level chat and viewer rollups are available in Analytics.'
                      : status === 'stats-only'
                        ? 'Session averages only — open Analytics and sync chat/emotes for charts.'
                        : status === 'current-live'
                          ? 'This is the current live broadcast.'
                          : status === 'sync-interrupted'
                            ? 'The last sync did not finish. Open Analytics to retry.'
                          : 'No analytics row yet — watch the channel or open Analytics to start tracking.'
                  }
                >
                  {streamAnalyticsStatusLabel(status)}
                </span>
              </span>
              <div className="flex flex-wrap items-center gap-2">
                <Link
                  to={`/analytics/${encodeURIComponent(channel)}/${encodeURIComponent(row.id)}`}
                  className="rounded border border-white/10 bg-white/[0.04] px-2 py-1 text-[11px] font-black uppercase text-cyan-300 hover:border-cyan-400/40"
                >
                  Analytics
                </Link>
                {row.videoId ? (
                  <Link
                    to={buildVodDeepLink(channel, row.videoId, 0, row.id)}
                    className="rounded border border-violet-400/20 bg-violet-500/10 px-2 py-1 text-[11px] font-black uppercase text-violet-200 hover:border-violet-300/40"
                  >
                    Play VOD
                  </Link>
                ) : (
                  <span className="text-[11px] font-semibold text-zinc-600">No VOD ID</span>
                )}
                {row.videoId ? (
                  <a
                    href={buildTwitchVodUrl(row.videoId)}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="rounded border border-white/10 bg-white/[0.04] px-2 py-1 text-[11px] font-black uppercase text-zinc-300 hover:border-violet-300/40"
                  >
                    Twitch
                  </a>
                ) : null}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

export default ChannelVodsPanel
