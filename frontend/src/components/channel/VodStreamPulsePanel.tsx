import { Link } from 'react-router-dom'

import type { AnalyticsStreamDetail, AnalyticsTopEmote } from '../../api.ts'
import { normalizeBrowserOriginUrl } from '../../config.ts'
import { resolveEmoteImageUrl } from '../../utils/emoteImageUrl.ts'
import { pulseDashboardUrl } from '../../utils/pulseDashboard.ts'
import VodMomentsPanel from './VodMomentsPanel.tsx'

function emoteImageUrl(emote: AnalyticsTopEmote): string | undefined {
  const url = resolveEmoteImageUrl({
    provider: emote.provider,
    id: emote.id,
    imageUrl: emote.imageUrl,
    scale: '2x',
  })
  return url ? normalizeBrowserOriginUrl(url, ['/emotes/']) : undefined
}

function formatCount(value: number) {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return value.toLocaleString()
}

export interface VodStreamPulsePanelProps {
  channelLogin: string
  streamId: string
  detail?: AnalyticsStreamDetail | null
  currentOffsetSec?: number
  onSeekMoment?: (offsetSeconds: number) => void
  isLoading?: boolean
  isError?: boolean
  className?: string
}

export function VodStreamPulsePanel({
  channelLogin,
  streamId,
  detail,
  currentOffsetSec = 0,
  onSeekMoment,
  isLoading,
  isError,
  className,
}: VodStreamPulsePanelProps) {
  const stream = detail?.stream
  const topEmotes = (detail?.topEmotes ?? []).slice(0, 12)
  const grafanaUrl = pulseDashboardUrl(channelLogin, {
    streamId,
    startedAt: stream?.startedAt,
    endedAt: stream?.endedAt,
  })
  const chatLogHref = `/logs/${encodeURIComponent(channelLogin)}/${encodeURIComponent(streamId)}`

  return (
    <div className={className ?? 'flex h-full min-h-0 flex-1 flex-col overflow-hidden'}>
      <div className="shrink-0 border-b border-white/10 px-3 py-3">
        <div className="min-w-0">
          <h3 className="text-sm font-black text-violet-100">Emote Pulse</h3>
          <p className="mt-1 text-[11px] font-semibold leading-relaxed text-zinc-500">
            Synced rollups for this VOD. Click moments to jump; heatmap tracks playback.
          </p>
        </div>
        <div className="mt-2.5 flex flex-wrap gap-2">
          <Link
            to={chatLogHref}
            className="rounded border border-cyan-400/30 bg-cyan-500/15 px-3 py-1.5 text-[11px] font-black uppercase text-cyan-100 transition hover:bg-cyan-500/25"
          >
            Chat log
          </Link>
          <a
            href={grafanaUrl}
            target="_blank"
            rel="noreferrer"
            className="rounded border border-violet-400/30 bg-violet-500/15 px-3 py-1.5 text-[11px] font-black uppercase text-violet-100 transition hover:bg-violet-500/25"
          >
            Open Grafana
          </a>
        </div>
      </div>

      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-3 py-3">
        {onSeekMoment ? (
          <VodMomentsPanel
            detail={detail}
            currentOffsetSec={currentOffsetSec}
            onSeekMoment={onSeekMoment}
            isLoading={isLoading}
            isError={isError}
          />
        ) : null}

        {isLoading && !detail ? (
          <div className="rounded-lg border border-white/10 bg-white/[0.035] px-4 py-6 text-center text-xs font-semibold text-zinc-500">
            Loading stream pulse…
          </div>
        ) : isError ? (
          <div className="rounded-lg border border-white/10 bg-white/[0.035] px-4 py-6 text-center text-xs font-semibold text-zinc-500">
            Pulse data unavailable. Re-sync this stream in Analytics.
          </div>
        ) : topEmotes.length ? (
          <section className="space-y-2 pb-1">
            <h4 className="text-[11px] font-black uppercase tracking-wide text-zinc-400">Top emotes (synced)</h4>
            <div className="grid grid-cols-2 gap-2">
              {topEmotes.map(emote => {
                const image = emoteImageUrl(emote)
                return (
                  <div
                    key={emote.key ?? emote.name}
                    title={`${emote.name} · ${formatCount(emote.count)}`}
                    className="flex min-w-0 items-center gap-2 rounded-lg border border-white/10 bg-white/[0.035] px-2 py-2"
                  >
                    {image ? (
                      <img src={image} alt={emote.name} className="h-7 w-7 shrink-0 object-contain" loading="lazy" />
                    ) : null}
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-xs font-black text-zinc-200">{emote.name}</div>
                      <div className="text-[10px] font-semibold text-zinc-500">{formatCount(emote.count)}</div>
                    </div>
                  </div>
                )
              })}
            </div>
          </section>
        ) : !onSeekMoment ? (
          <div className="rounded-lg border border-white/10 bg-white/[0.035] px-4 py-5 text-center text-xs font-semibold text-zinc-500">
            No emote rollups yet. Sync chat/emotes in Analytics, then return here.
          </div>
        ) : null}
      </div>
    </div>
  )
}

export default VodStreamPulsePanel
