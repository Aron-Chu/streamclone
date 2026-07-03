import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import AnalyticsChart, { type AnalyticsViewMode } from './analytics/AnalyticsChart.tsx'
import {
  getAnalyticsLive,
  getAnalyticsStream,
  getAnalyticsStreams,
  getChannelStreamHistory,
  getPulseStreamRecap,
  getStreamGameSegments,
  getSyncStatus,
  watchAnalyticsChannel,
  type AnalyticsMinuteRollup,
} from '../api.ts'
import type { AnalyticsStream, AnalyticsStreamDetail } from '../apiTypes.ts'
import { useAnalyticsLive } from '../hooks/useAnalyticsLive.ts'
import { syncCtaLabel } from '../utils/syncLabel.ts'
import { findNearestRollupByOffset, parseDeepLinkOffset } from '../utils/momentSelection.ts'
import {
  classifyStatCards,
  STAT_PLACEHOLDER_MUTED_CLASS,
  type StreamCollectionState,
} from '../utils/statCards.ts'
import { isActiveLiveCollectorStream, isSyncPrefetchPlaceholder } from '../utils/analyticsStreamRow.ts'
import { buildTwitchVodUrl, resolveAnalyticsVodId } from '../utils/twitchVodUrl.ts'
import {
  computeRollupChatStats,
  computeRollupViewerStats,
  rollupHasMinuteData,
} from './analytics/chartRollupUtils.ts'
import { count, displayStreamTitle, duration, relativeTime, streamStateLabel } from '../utils/consoleFormat.ts'
import { ChatCoverageBadge, SourcePills, StatCard } from './analytics/ConsoleBits.tsx'
import { StreamSidebar } from './analytics/StreamSidebar.tsx'
import { TopEmoteTable } from './analytics/TopEmoteTable.tsx'
import { MomentReviewPanel } from './analytics/MomentReviewPanel.tsx'
import { SelectedMomentPanel } from './analytics/SelectedMomentPanel.tsx'
import { StreamRecapPanel } from './analytics/StreamRecapPanel.tsx'
import { SyncStatusPanel } from './analytics/SyncStatusPanel.tsx'

type RightPanelTab = 'moments' | 'emotes' | 'status'

interface HistoryStreamItem {
  streamId?: string
  id?: string
  displayName?: string
  title?: string
  category?: string
  startedAt?: string
  endedAt?: string | null
  avgViewers?: number
  peakViewers?: number
  viewerSamples?: number
  chatMessages?: number
  vodId?: string
  videoId?: string
}

export interface AnalyticsConsoleProps {
  mode?: 'public' | 'local' | string
  showGameSegments?: boolean
  /** When true, use lg breakpoint for 3-col layout (portal Figma shell already consumes sidebar width). */
  shellNested?: boolean
  buildSessionPath?: (login: string, streamId: string) => string
  buildChannelPath?: (login: string) => string
}

function defaultSessionPath(login: string, streamId: string): string {
  return `/analytics/${encodeURIComponent(login)}/${encodeURIComponent(streamId)}`
}

function defaultChannelPath(login: string): string {
  return `/analytics/${encodeURIComponent(login)}`
}

export function AnalyticsConsole({
  mode: _mode = 'public',
  showGameSegments: _showGameSegments = true,
  shellNested = false,
  buildSessionPath = defaultSessionPath,
  buildChannelPath = defaultChannelPath,
}: AnalyticsConsoleProps = {}) {
  const navigate = useNavigate()
  const location = useLocation()
  const { login = '', streamId = '' } = useParams<{ login: string; streamId?: string }>()
  const channelLogin = login.trim().toLowerCase()
  const isHistoricalRoute = Boolean(streamId)
  const isLiveRoute = !streamId

  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [selectedRollup, setSelectedRollup] = useState<AnalyticsMinuteRollup | null>(null)
  const [viewMode, setViewMode] = useState<AnalyticsViewMode>('overview')
  const [rightPanelTab, setRightPanelTab] = useState<RightPanelTab>('moments')
  const [syncedOnlyFilter, setSyncedOnlyFilter] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [lastRefreshedAt, setLastRefreshedAt] = useState<number | null>(null)
  const appliedDeepLinkKey = useRef<string | null>(null)

  useEffect(() => {
    setSelectedRollup(null)
    setLastRefreshedAt(null)
  }, [channelLogin, streamId])

  useEffect(() => {
    if (!channelLogin) return
    watchAnalyticsChannel(channelLogin).catch(() => undefined)
  }, [channelLogin])

  const liveQuery = useAnalyticsLive(channelLogin, { enabled: isLiveRoute })
  const streamsQuery = useQuery({
    queryKey: ['analytics-console-streams', channelLogin],
    queryFn: () => getAnalyticsStreams(channelLogin, 20),
    enabled: Boolean(channelLogin),
    staleTime: 30_000,
    refetchInterval: 30_000,
  })
  const historyQuery = useQuery({
    queryKey: ['analytics-console-history', channelLogin],
    queryFn: () => getChannelStreamHistory(channelLogin, 'all'),
    enabled: Boolean(channelLogin),
    staleTime: 120_000,
    retry: 2,
  })

  const activeStreamId =
    streamId || liveQuery.data?.stream?.streamId || streamsQuery.data?.items?.[0]?.streamId || ''

  const historicalDetailQuery = useQuery({
    queryKey: ['analytics-console-detail', activeStreamId, channelLogin],
    queryFn: async () => {
      if (!activeStreamId) return null
      return getAnalyticsStream(activeStreamId, { sparse: false, channel: channelLogin })
    },
    enabled: Boolean(channelLogin && activeStreamId && isHistoricalRoute),
    staleTime: 15_000,
  })

  const detailQuery = isLiveRoute ? liveQuery : historicalDetailQuery
  const detail = (detailQuery.data ?? undefined) as AnalyticsStreamDetail | undefined

  const gamesQuery = useQuery({
    queryKey: ['analytics-console-games', activeStreamId],
    queryFn: () => getStreamGameSegments(activeStreamId),
    enabled: Boolean(activeStreamId),
    staleTime: 60_000,
  })

  const syncQuery = useQuery({
    queryKey: ['analytics-console-sync', activeStreamId],
    queryFn: () => getSyncStatus(activeStreamId),
    enabled: Boolean(activeStreamId),
    staleTime: 30_000,
  })

  const recapQuery = useQuery({
    queryKey: ['analytics-console-recap', activeStreamId],
    queryFn: () => getPulseStreamRecap(activeStreamId),
    enabled: Boolean(activeStreamId) && detail?.state !== 'live',
    staleTime: 120_000,
    retry: 1,
  })

  const isActiveLiveCollector = isActiveLiveCollectorStream(detail?.stream, detail?.state)
  const isLive = isLiveRoute && (detail?.state === 'live' || Boolean(liveQuery.data?.stream?.streamId))

  useEffect(() => {
    appliedDeepLinkKey.current = null
  }, [activeStreamId, location.hash, location.search])

  useEffect(() => {
    const rollups = detail?.rollups ?? []
    if (!rollups.length) return
    const deepLinkKey = `${activeStreamId}:${location.hash}:${location.search}`
    if (appliedDeepLinkKey.current === deepLinkKey) return
    const offsetSeconds = parseDeepLinkOffset(location.hash, location.search)
    if (offsetSeconds == null) return
    const nearest = findNearestRollupByOffset(rollups, detail?.stream?.startedAt, offsetSeconds)
    if (nearest) setSelectedRollup(nearest)
    appliedDeepLinkKey.current = deepLinkKey
  }, [activeStreamId, detail?.rollups, detail?.stream?.startedAt, location.hash, location.search])

  const activeStreamKey = detail?.stream?.streamId || activeStreamId || channelLogin
  const [prevRailStreamKey, setPrevRailStreamKey] = useState(activeStreamKey)
  if (activeStreamKey !== prevRailStreamKey) {
    setPrevRailStreamKey(activeStreamKey)
    setRightPanelTab('moments')
  }

  const toggleSelected = useCallback((key: string) => {
    setSelected((current) => {
      const next = new Set(current)
      if (next.has(key)) next.delete(key)
      else if (next.size < 5) next.add(key)
      return next
    })
  }, [])

  const handleViewMode = useCallback((next: AnalyticsViewMode) => {
    setViewMode(next)
    if (next === 'emotes') setRightPanelTab('emotes')
    else if (next === 'spikes') setRightPanelTab('moments')
  }, [])

  const handleRefresh = useCallback(async () => {
    if (!channelLogin || refreshing) return
    setRefreshing(true)
    try {
      const refetches: Array<Promise<unknown>> = [
        streamsQuery.refetch(),
        historyQuery.refetch(),
        detailQuery.refetch(),
      ]
      if (activeStreamId) {
        refetches.push(gamesQuery.refetch())
        refetches.push(syncQuery.refetch())
      }
      await Promise.race([Promise.all(refetches), new Promise((resolve) => setTimeout(resolve, 30_000))])
      setLastRefreshedAt(Date.now())
    } finally {
      setRefreshing(false)
    }
  }, [channelLogin, refreshing, streamsQuery, historyQuery, detailQuery, gamesQuery, syncQuery, activeStreamId])

  const stream = detail?.stream
  const streamVodId = resolveAnalyticsVodId(detail)
  const rollupCount = detail?.rollups?.length ?? 0
  const isLongStreamChart = rollupCount >= 360

  const headerState = isHistoricalRoute
    ? detail?.state && detail.state !== 'live'
      ? detail.state
      : 'historical'
    : detail?.state || (detailQuery.isLoading ? 'loading' : 'not_collected')

  const headerStats = useMemo(() => {
    const rollups = detail?.rollups ?? []
    const viewerStats = computeRollupViewerStats(rollups)
    const chatStats = computeRollupChatStats(rollups)
    return {
      current: viewerStats?.current ?? stream?.currentViewers,
      avg: viewerStats?.avg ?? stream?.avgViewers,
      peak: viewerStats?.peak ?? stream?.peakViewers,
      chat: chatStats.chat > 0 ? chatStats.chat : stream?.chatMessages,
      emotes: chatStats.emotes > 0 ? chatStats.emotes : undefined,
    }
  }, [detail?.rollups, stream])

  const statCardClasses = useMemo(
    () =>
      classifyStatCards({
        state: (detail?.state as StreamCollectionState) ?? 'not_collected',
        avgViewers: stream?.avgViewers ?? 0,
        peakViewers: stream?.peakViewers ?? 0,
        rollups: detail?.rollups ?? [],
      }),
    [detail?.state, detail?.rollups, stream?.avgViewers, stream?.peakViewers],
  )

  const chartEmoteKeys = useMemo(() => {
    const topEmoteKey = detail?.topEmotes?.[0]?.key
    if (viewMode === 'overview') {
      if (selected.size > 0) return selected
      return topEmoteKey ? new Set([topEmoteKey]) : selected
    }
    if (selected.size > 0) return selected
    return new Set((detail?.topEmotes ?? []).slice(0, 4).map((emote) => emote.key))
  }, [viewMode, selected, detail?.topEmotes])

  const sidebarStreams = useMemo(() => {
    const local = streamsQuery.data?.items ?? []
    const historyItems = ((historyQuery.data as { items?: HistoryStreamItem[] } | undefined)?.items ?? [])
    const mappedHistory: AnalyticsStream[] = historyItems
      .map((s) => ({
        streamId: s.streamId ?? s.id ?? '',
        login: channelLogin,
        displayName: s.displayName ?? channelLogin,
        title: s.title,
        category: s.category || 'Live',
        startedAt: s.startedAt || '',
        endedAt: s.endedAt || '',
        currentViewers: 0,
        avgViewers: s.avgViewers,
        peakViewers: s.peakViewers,
        viewerSamples: s.viewerSamples ?? 0,
        chatMessages: s.chatMessages ?? 0,
        vodId: s.vodId ?? s.videoId,
      }))
      .filter((s) => s.streamId)

    const historyById = new Map(mappedHistory.map((s) => [s.streamId, s]))
    const merged = local.map((item) => {
      const history = historyById.get(item.streamId)
      if (!history) return item
      return {
        ...item,
        startedAt: history.startedAt || item.startedAt,
        endedAt: history.endedAt || item.endedAt,
        title: history.title || item.title,
        category: history.category || item.category,
        avgViewers: (item.avgViewers ?? 0) > 0 ? item.avgViewers : history.avgViewers,
        peakViewers: (item.peakViewers ?? 0) > 0 ? item.peakViewers : history.peakViewers,
      }
    })
    const localIds = new Set(merged.map((s) => s.streamId))
    for (const s of mappedHistory) {
      if (!localIds.has(s.streamId)) merged.push(s)
    }
    return merged
      .filter((s) => !isSyncPrefetchPlaceholder(s))
      .sort((a, b) => {
        const aTime = a.startedAt ? Date.parse(a.startedAt) : 0
        const bTime = b.startedAt ? Date.parse(b.startedAt) : 0
        return bTime - aTime
      })
  }, [streamsQuery.data?.items, historyQuery.data, channelLogin])

  const syncLabel = useMemo(() => {
    const rollups = detail?.rollups ?? []
    const hasChat = rollups.some((row) => (row.chatCount ?? 0) > 0 || (row.totalEmoteCount ?? 0) > 0)
    const hasViewers = rollups.some((row) => (row.viewerSamples ?? 0) > 0 || (row.viewerAvg ?? 0) > 0)
    return syncCtaLabel({ syncing: false, hasChatRollups: hasChat, hasViewerSamples: hasViewers })
  }, [detail?.rollups])

  if (!channelLogin) {
    return (
      <section className="analytics-console" aria-label="Streamclone analytics console">
        <p className="muted">Missing channel login.</p>
      </section>
    )
  }

  const headerTitle = displayStreamTitle(stream, channelLogin)

  return (
    <section className="analytics-console text-zinc-100" aria-label={`Analytics for ${channelLogin}`}>
      <div className="flex w-full flex-col gap-4">
        <header className="flex flex-col gap-3 border-b border-white/10 pb-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2 text-xs font-black uppercase text-zinc-500">
              <Link to="/analytics" className="rounded bg-white/10 px-2 py-1 text-zinc-200 transition hover:bg-white/15">
                Analytics hub
              </Link>
              <Link
                to={buildChannelPath(channelLogin)}
                className="rounded bg-violet-400/15 px-2 py-1 text-violet-100 transition hover:bg-violet-400/25"
              >
                {channelLogin}
              </Link>
              <span
                className={`rounded px-2 py-1 ${
                  headerState === 'live'
                    ? 'bg-red-500/15 text-red-100'
                    : headerState === 'syncing'
                      ? 'bg-violet-500/15 text-violet-100'
                      : headerState === 'historical'
                        ? 'bg-cyan-500/10 text-cyan-100'
                        : 'bg-white/10 text-zinc-300'
                }`}
              >
                {streamStateLabel(
                  headerState as AnalyticsStreamDetail['state'] | 'not found' | 'loading',
                  isHistoricalRoute,
                )}
              </span>
              <ChatCoverageBadge detail={detail} />
            </div>
            <h1
              className="mt-3 truncate text-2xl font-black leading-tight text-white lg:text-3xl"
              title={headerTitle}
            >
              {headerTitle}
            </h1>
            <div className="mt-2 flex flex-wrap gap-2 text-sm font-bold text-zinc-500">
              {stream?.displayName ? <span>{stream.displayName}</span> : null}
              {stream?.category ? <span>{stream.category}</span> : null}
              {stream?.startedAt ? <span>Started {relativeTime(stream.startedAt)}</span> : null}
              <span>
                {lastRefreshedAt
                  ? `Refreshed ${relativeTime(lastRefreshedAt)}`
                  : `Updated ${detail?.updatedAt ? relativeTime(detail.updatedAt) : '-'}`}
              </span>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            {activeStreamId && isLiveRoute ? (
              <button
                type="button"
                onClick={() => navigate(buildSessionPath(channelLogin, activeStreamId))}
                className="rounded border border-white/10 bg-white/[0.05] px-3 py-1.5 text-[11px] font-black uppercase text-zinc-300 transition hover:bg-white/10 hover:text-white"
              >
                Open session page
              </button>
            ) : null}
            <button
              type="button"
              onClick={() => void handleRefresh()}
              disabled={refreshing}
              className="rounded border border-white/10 bg-white/[0.05] px-3 py-1.5 text-[11px] font-black uppercase text-zinc-300 transition hover:bg-white/10 hover:text-white disabled:opacity-50"
            >
              {refreshing ? 'Refreshing…' : 'Refresh data'}
            </button>
            <SourcePills sources={detail?.sources} />
          </div>
        </header>

        <section className="grid grid-cols-2 gap-3 lg:grid-cols-6">
          <StatCard
            label="Current"
            value={statCardClasses.current.placeholder ?? count(headerStats.current)}
            tone={statCardClasses.current.muted ? STAT_PLACEHOLDER_MUTED_CLASS : 'text-cyan-300/90'}
          />
          <StatCard
            label="Average"
            value={statCardClasses.average.placeholder ?? count(headerStats.avg)}
            tone={statCardClasses.average.muted ? STAT_PLACEHOLDER_MUTED_CLASS : undefined}
          />
          <StatCard
            label="Peak"
            value={statCardClasses.peak.placeholder ?? count(headerStats.peak)}
            tone={statCardClasses.peak.muted ? STAT_PLACEHOLDER_MUTED_CLASS : undefined}
          />
          <StatCard
            label="Chat"
            value={statCardClasses.chat.placeholder ?? count(headerStats.chat)}
            tone={statCardClasses.chat.muted ? STAT_PLACEHOLDER_MUTED_CLASS : 'text-violet-300/90'}
          />
          <StatCard
            label="Emote Uses"
            value={statCardClasses.emoteUses.placeholder ?? count(headerStats.emotes)}
            tone={statCardClasses.emoteUses.muted ? STAT_PLACEHOLDER_MUTED_CLASS : 'text-emerald-300/90'}
          />
          <StatCard label="Duration" value={duration(stream)} />
        </section>

        <div
          className={
            shellNested
              ? 'grid gap-4 lg:grid-cols-[220px_minmax(0,1fr)_280px]'
              : 'grid gap-4 xl:grid-cols-[260px_minmax(0,1fr)_320px]'
          }
        >
          <aside className="order-3 min-w-0 xl:order-none xl:sticky xl:top-4 xl:self-start">
            <StreamSidebar
              login={channelLogin}
              streams={sidebarStreams}
              activeID={isHistoricalRoute ? streamId : undefined}
              isLiveView={isLiveRoute || isActiveLiveCollector}
              liveState={isActiveLiveCollector ? 'live' : isLiveRoute ? detail?.state : undefined}
              syncedOnly={syncedOnlyFilter}
              onSyncedOnlyChange={setSyncedOnlyFilter}
              buildSessionPath={buildSessionPath}
              buildChannelPath={buildChannelPath}
            />
          </aside>

          <section className="order-1 min-w-0 xl:order-none">
            <div className="min-w-0 space-y-4">
              <AnalyticsChart
                detail={detail}
                selectedEmotes={chartEmoteKeys}
                onSelectEmote={toggleSelected}
                selectedRollup={selectedRollup}
                onSelectRollup={setSelectedRollup}
                onRefresh={() => void handleRefresh()}
                refreshing={refreshing}
                loading={detailQuery.isLoading && !detail}
                games={gamesQuery.data ?? []}
                canSync={false}
                isLive={isActiveLiveCollector}
                syncCtaLabel={syncLabel}
                syncViewerStatus={syncQuery.data?.viewerStatus}
                viewMode={viewMode}
                onViewModeChange={handleViewMode}
              />
              <SelectedMomentPanel
                rollup={selectedRollup}
                rollups={detail?.rollups ?? []}
                startedAt={stream?.startedAt}
                vodId={streamVodId}
                topEmotesCatalog={detail?.topEmotes}
              />
              {streamVodId ? (
                <p className="text-[11px] font-semibold text-zinc-500">
                  Select a moment to open the VOD on Twitch.
                  <a
                    href={buildTwitchVodUrl(streamVodId)}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="ml-1 text-violet-300 hover:text-violet-200"
                  >
                    Open full VOD
                  </a>
                  {isLongStreamChart ? ' Long streams (6h+) may feel slower while hovering the chart.' : ''}
                </p>
              ) : null}
            </div>
          </section>

          <aside className="order-2 space-y-4 xl:order-none">
            {recapQuery.data ? <StreamRecapPanel recap={recapQuery.data} /> : null}
            <div className="rounded border border-white/10 bg-white/[0.035] overflow-hidden">
              <div className="flex border-b border-white/10 text-[10px] font-black uppercase bg-white/[0.015]">
                {(['moments', 'emotes', 'status'] as const).map((tab) => (
                  <button
                    key={tab}
                    type="button"
                    onClick={() => setRightPanelTab(tab)}
                    className={`flex-1 py-2 text-center transition border-r border-white/10 last:border-r-0 ${
                      rightPanelTab === tab ? 'bg-white/[0.04] text-white' : 'text-zinc-500 hover:text-zinc-300'
                    }`}
                  >
                    {tab === 'moments' ? 'Moments' : tab === 'emotes' ? 'Emotes' : 'Status'}
                  </button>
                ))}
              </div>
              <div className="p-0">
                {rightPanelTab === 'moments' ? (
                  <MomentReviewPanel
                    rollups={detail?.rollups ?? []}
                    selectedRollup={selectedRollup}
                    onSelectRollup={setSelectedRollup}
                    topEmotesCatalog={detail?.topEmotes}
                    streamStartedAt={stream?.startedAt}
                    embedded
                  />
                ) : null}
                {rightPanelTab === 'emotes' ? (
                  <TopEmoteTable emotes={detail?.topEmotes ?? []} selected={chartEmoteKeys} onSelect={toggleSelected} embedded />
                ) : null}
                {rightPanelTab === 'status' ? (
                  <SyncStatusPanel detail={detail} syncStatus={syncQuery.data} />
                ) : null}
              </div>
            </div>
          </aside>
        </div>
      </div>
    </section>
  )
}
