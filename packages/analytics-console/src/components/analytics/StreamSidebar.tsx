import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import type { AnalyticsStream } from '../../apiTypes.ts'
import { streamIsSidebarVisible, analyticsStreamPathSlug, streamSyncBadgeState, streamSyncBadgeLabel, streamSyncBadgeTitle } from '../../utils/syncedLiveStream.ts'
import { count, duration, formatDateTime } from '../../utils/consoleFormat.ts'
import { CoreMinuteChartsNotice } from '../CoreMinuteChartsNotice.tsx'

export function StreamSidebar({
  login,
  streams,
  activeID,
  isLiveView,
  liveState,
  syncing,
  syncedOnly,
  onSyncedOnlyChange,
  coreMinuteChartsBlocked = false,
  activeRollupStats,
  activeMinutesUnavailable = false,
  buildSessionPath,
  buildChannelPath,
}: {
  login: string
  streams: AnalyticsStream[]
  activeID?: string
  isLiveView: boolean
  liveState?: string
  syncing?: boolean
  syncedOnly?: boolean
  onSyncedOnlyChange?: (value: boolean) => void
  coreMinuteChartsBlocked?: boolean
  activeRollupStats?: { avg: number; peak: number; current: number } | null
  activeMinutesUnavailable?: boolean
  buildSessionPath: (login: string, streamId: string) => string
  buildChannelPath: (login: string) => string
}) {
  const visibleStreams = useMemo(() => {
    return streams.filter((s) => streamIsSidebarVisible(s, Boolean(syncedOnly)))
  }, [streams, syncedOnly])

  const [archiveExpanded, setArchiveExpanded] = useState(false)
  const MOBILE_COLLAPSED_ROWS = 2
  const hasCollapsibleRows = visibleStreams.length > MOBILE_COLLAPSED_ROWS

  return (
    <div className="flex min-h-0 w-full flex-col overflow-hidden rounded border border-white/10 bg-white/[0.035] xl:max-h-[calc(100vh-12rem)]">
      <div className="flex items-center justify-between border-b border-white/10 px-3 py-2.5">
        <span className="text-[11px] font-black uppercase tracking-wide text-zinc-500">Streams</span>
        <span className="rounded bg-white/10 px-1.5 py-0.5 text-[10px] font-black text-zinc-400">
          {visibleStreams.length}
          {syncedOnly ? `/${streams.length}` : ''}
        </span>
      </div>
      {onSyncedOnlyChange ? (
        <label className="flex cursor-pointer items-center gap-2 border-b border-white/5 px-3 py-2 text-[10px] font-semibold text-zinc-400">
          <input
            type="checkbox"
            checked={Boolean(syncedOnly)}
            onChange={(e) => onSyncedOnlyChange(e.target.checked)}
            className="accent-violet-500"
          />
          Synced only (hide stats-only rows)
        </label>
      ) : null}
      <div className="sc-console-scroll min-h-0 flex-1 overflow-y-auto">
        <Link
          to={buildChannelPath(login)}
          className={`sc-stream-row block border-b border-white/5 px-3 py-2.5 transition hover:bg-white/[0.05] ${
            isLiveView ? 'border-l-2 border-l-red-400 bg-red-500/10' : 'border-l-2 border-l-transparent'
          }`}
        >
          <div className="flex items-center gap-2">
            <span className={`h-2 w-2 rounded-full ${liveState === 'live' ? 'bg-red-400 animate-pulse' : 'bg-zinc-600'}`} />
            <span className="text-sm font-black text-white">Live / Current</span>
          </div>
          <div className="mt-1 text-[10px] font-semibold text-zinc-500">
            {liveState === 'live' ? 'Live tracking' : 'Most recent session'}
          </div>
        </Link>
        {streams.length === 0 ? (
          <div className="px-3 py-4 text-center text-[11px] font-semibold text-zinc-500">
            No past streams indexed yet.
          </div>
        ) : visibleStreams.length === 0 ? (
          <div className="px-3 py-4 text-center text-[11px] font-semibold text-zinc-500">
            No synced streams match this filter. Turn off &quot;Synced only&quot; to see stats-only sessions.
          </div>
        ) : (
          <div className="divide-y divide-white/5">
            {visibleStreams.map((stream, rowIndex) => {
              const targetSlug = analyticsStreamPathSlug(stream, streams)
              const dateSlug = targetSlug !== stream.streamId ? targetSlug : ''
              const isActive = !isLiveView && (activeID === stream.streamId || activeID === dateSlug || activeID === targetSlug)
              const syncBadge = streamSyncBadgeState(stream, Boolean(syncing && isActive))
              const isSyncingActive = syncBadge === 'syncing'
              const rollupStats = isSyncingActive ? activeRollupStats : null
              const mobileHiddenClass = !archiveExpanded && rowIndex >= MOBILE_COLLAPSED_ROWS ? 'hidden lg:block' : ''
              const badgeTone =
                syncBadge === 'syncing'
                  ? 'bg-violet-500/10 text-violet-300'
                  : syncBadge === 'synced'
                    ? 'bg-emerald-500/10 text-emerald-300'
                    : syncBadge === 'partial'
                      ? 'bg-amber-500/10 text-amber-300'
                      : 'bg-amber-500/10 text-amber-300'

              return (
                <Link
                  key={stream.streamId}
                  to={buildSessionPath(login, targetSlug)}
                  className={`sc-stream-row block border-l-2 px-3 py-2.5 transition hover:bg-white/[0.05] ${mobileHiddenClass} ${
                    isActive ? 'border-l-cyan-400 bg-cyan-400/10' : 'border-l-transparent'
                  }`}
                >
                  <div className="text-[10px] font-black uppercase tracking-wide text-zinc-500">
                    {formatDateTime(stream.startedAt)}
                  </div>
                  <div className="mt-0.5 line-clamp-2 text-[13px] font-bold leading-snug text-white">
                    {stream.title || 'Untitled stream'}
                  </div>
                  <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                    {(stream.gamesSummary || stream.category) ? (
                      <span className="rounded bg-violet-500/15 px-1.5 py-0.5 text-[9px] font-black uppercase text-violet-200">
                        {stream.gamesSummary || stream.category}
                      </span>
                    ) : null}
                    <span
                      className={`rounded px-1.5 py-0.5 text-[9px] font-black uppercase ${badgeTone}`}
                      title={streamSyncBadgeTitle(syncBadge, stream)}
                    >
                      {streamSyncBadgeLabel(syncBadge)}
                    </span>
                  </div>
                  {isActive && activeMinutesUnavailable ? (
                    <p className="mt-1.5 text-[10px] font-semibold leading-snug text-amber-200/80">
                      Session metadata synced; minute chart unavailable — try refresh or pick another session.
                    </p>
                  ) : null}
                  {syncBadge === 'stats_only' && isActive && coreMinuteChartsBlocked ? (
                    <div className="mt-1.5">
                      <CoreMinuteChartsNotice />
                    </div>
                  ) : null}
                  <div className="mt-1.5 grid grid-cols-3 gap-1 text-[10px] font-bold text-zinc-500">
                    <span>{duration(stream)}</span>
                    <span>avg {count(rollupStats?.avg ?? stream.avgViewers)}</span>
                    <span>peak {count(rollupStats?.peak ?? stream.peakViewers)}</span>
                  </div>
                </Link>
              )
            })}
          </div>
        )}
        {hasCollapsibleRows ? (
          <button
            type="button"
            onClick={() => setArchiveExpanded((prev) => !prev)}
            aria-expanded={archiveExpanded}
            className="block w-full border-t border-white/10 px-3 py-2 text-center text-[10px] font-black uppercase tracking-wide text-zinc-400 transition hover:bg-white/[0.05] hover:text-white lg:hidden"
          >
            {archiveExpanded ? 'Show fewer streams' : `Show all ${visibleStreams.length} streams`}
          </button>
        ) : null}
      </div>
    </div>
  )
}
