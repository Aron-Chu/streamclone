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
  getReplayHeatmap,
  getStreamGameSegments,
  getStreamSummary,
  getSyncStatus,
  startHistoricalSync,
  watchAnalyticsChannel,
  type AnalyticsMinuteRollup,
} from '../api.ts'
import type { AnalyticsStream, AnalyticsStreamDetail, SyncStatus } from '../apiTypes.ts'
import type { HeatmapResponse } from '../types/heatmap.ts'
import { useAnalyticsLive } from '../hooks/useAnalyticsLive.ts'
import { syncCtaLabel } from '../utils/syncLabel.ts'
import { findNearestRollupByOffset, parseDeepLinkOffset } from '../utils/momentSelection.ts'
import {
  classifyStatCards,
  STAT_PLACEHOLDER_MUTED_CLASS,
  type StreamCollectionState,
} from '../utils/statCards.ts'
import { isActiveLiveCollectorStream, isSyncPrefetchPlaceholder } from '../utils/analyticsStreamRow.ts'
import { analyticsStreamPathSlug, pickSyncedLiveStreamTarget } from '../utils/syncedLiveStream.ts'
import {
  isDateSlugUnresolved,
  resolveMatchedStream,
  resolveTargetQueryStreamId,
} from '../utils/streamRouteResolution.ts'
import { buildTwitchVodUrl, resolveAnalyticsVodId, resolveVodLinkState } from '../utils/twitchVodUrl.ts'
import {
  computeRollupChatStats,
  computeRollupViewerStats,
  rollupHasMinuteData,
  rollupsHaveViewerData,
} from './analytics/chartRollupUtils.ts'
import {
  type EmotePlotSelection,
  resolveChartEmoteKeys,
  toggleEmotePlotSelection,
} from '../utils/emotePlotSelection.ts'
import { count, displayStreamTitle, duration, relativeTime, streamStateLabel } from '../utils/consoleFormat.ts'
import { deriveChartGameSegments } from '../utils/gameSegmentChart.ts'
import { ChatCoverageBadge, StatCard, ViewerSourceBadge, AnalyticsQualityChip, CoverageFacets, CoverageStartBanner } from './analytics/ConsoleBits.tsx'
import { StreamSidebar } from './analytics/StreamSidebar.tsx'
import { TopEmoteTable } from './analytics/TopEmoteTable.tsx'
import { MomentReviewPanel } from './analytics/MomentReviewPanel.tsx'
import { StreamRecapPanel } from './analytics/StreamRecapPanel.tsx'
import { SyncStatusPanel } from './analytics/SyncStatusPanel.tsx'
import { StreamQualityBanner } from './analytics/StreamQualityBanner.tsx'
import { diagnoseStreamQuality } from '../utils/streamQuality.ts'
import { isTerminalSyncPhase, pollSyncUntilDone } from '../utils/syncPolling.ts'

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
  /** Enable sync / re-sync CTAs (local dev portal; hosted waits for API-008 rate limits). */
  enableSyncActions?: boolean
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
  mode = 'public',
  showGameSegments = true,
  shellNested = false,
  enableSyncActions = false,
  buildSessionPath = defaultSessionPath,
  buildChannelPath = defaultChannelPath,
}: AnalyticsConsoleProps = {}) {
  const navigate = useNavigate()
  const location = useLocation()
  const { login = '', streamId = '' } = useParams<{ login: string; streamId?: string }>()
  const channelLogin = login.trim().toLowerCase()
  const isHistoricalRoute = Boolean(streamId)
  const isLiveRoute = !streamId
  /** Layer 2 (games, recap, heatmap, summary, sync detail) only on explicit session routes. */
  const layer2Enabled = isHistoricalRoute

  const [emotePlotSelection, setEmotePlotSelection] = useState<EmotePlotSelection>('auto')
  const [selectedRollup, setSelectedRollup] = useState<AnalyticsMinuteRollup | null>(null)
  const [previewRollup, setPreviewRollup] = useState<AnalyticsMinuteRollup | null>(null)
  const [viewMode, setViewMode] = useState<AnalyticsViewMode>('overview')
  const [rightPanelTab, setRightPanelTab] = useState<RightPanelTab>('moments')
  const [syncedOnlyFilter, setSyncedOnlyFilter] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [syncError, setSyncError] = useState<string | null>(null)
  const [syncNotice, setSyncNotice] = useState<string | null>(null)
  const [liveSyncStatus, setLiveSyncStatus] = useState<SyncStatus | null>(null)
  const [lastRefreshedAt, setLastRefreshedAt] = useState<number | null>(null)
  const appliedDeepLinkKey = useRef<string | null>(null)

  useEffect(() => {
    setSelectedRollup(null)
    setPreviewRollup(null)
    setLastRefreshedAt(null)
    setEmotePlotSelection('auto')
    setSyncing(false)
    setSyncError(null)
    setSyncNotice(null)
    setLiveSyncStatus(null)
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

  const historyItems = (historyQuery.data as { items?: HistoryStreamItem[] } | undefined)?.items
  const listsLoading = streamsQuery.isLoading || historyQuery.isLoading

  const sidebarStreams = useMemo(() => {
    const local = streamsQuery.data?.items ?? []
    const historyItemsList = historyItems ?? []
    const mappedHistory: AnalyticsStream[] = historyItemsList
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
        viewerSamples: Math.max(item.viewerSamples ?? 0, history.viewerSamples ?? 0),
        chatMessages: Math.max(item.chatMessages ?? 0, history.chatMessages ?? 0),
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
  }, [streamsQuery.data?.items, historyItems, channelLogin])

  const matchedStream = useMemo(() => {
    if (!streamId) return undefined
    return resolveMatchedStream(streamId, sidebarStreams, sidebarStreams)
  }, [streamId, sidebarStreams])

  const targetQueryStreamId = useMemo(() => {
    if (isLiveRoute) {
      return liveQuery.data?.stream?.streamId || streamsQuery.data?.items?.[0]?.streamId || ''
    }
    const resolved = resolveTargetQueryStreamId(streamId, matchedStream, historyItems, listsLoading)
    return resolved ?? ''
  }, [
    isLiveRoute,
    streamId,
    matchedStream,
    historyItems,
    listsLoading,
    liveQuery.data?.stream?.streamId,
    streamsQuery.data?.items,
  ])

  const sessionResolving = Boolean(
    isHistoricalRoute && listsLoading && /^\d{4}-\d{2}-\d{2}$/.test(streamId),
  )
  const sessionNotFound = Boolean(
    isHistoricalRoute && isDateSlugUnresolved(streamId, matchedStream, listsLoading),
  )

  const liveActiveStreamId =
    liveQuery.data?.stream?.streamId || streamsQuery.data?.items?.[0]?.streamId || ''

  const historicalDetailQuery = useQuery({
    queryKey: ['analytics-console-detail', targetQueryStreamId, channelLogin],
    queryFn: async () => {
      if (!targetQueryStreamId) return null
      return getAnalyticsStream(targetQueryStreamId, { sparse: false, channel: channelLogin })
    },
    enabled: Boolean(channelLogin && targetQueryStreamId && isHistoricalRoute && !sessionNotFound),
    staleTime: 15_000,
  })

  const detailQuery = isLiveRoute ? liveQuery : historicalDetailQuery
  const detail = (detailQuery.data ?? undefined) as AnalyticsStreamDetail | undefined
  const chartDetailReady = Boolean(
    detail?.rollups?.some(rollupHasMinuteData) || detail?.stream?.streamId,
  )

  const gamesQuery = useQuery({
    queryKey: ['analytics-console-games', targetQueryStreamId],
    queryFn: () => getStreamGameSegments(targetQueryStreamId),
    enabled: Boolean(layer2Enabled && showGameSegments && targetQueryStreamId),
    staleTime: 60_000,
  })

  const syncQuery = useQuery({
    queryKey: ['analytics-console-sync', targetQueryStreamId],
    queryFn: () => getSyncStatus(targetQueryStreamId),
    enabled: Boolean(layer2Enabled && targetQueryStreamId && chartDetailReady),
    staleTime: 30_000,
    refetchInterval: syncing ? 2000 : false,
  })

  const summaryQuery = useQuery({
    queryKey: ['analytics-console-summary', targetQueryStreamId, channelLogin],
    queryFn: () => getStreamSummary(targetQueryStreamId, channelLogin),
    enabled: Boolean(layer2Enabled && targetQueryStreamId && channelLogin && chartDetailReady),
    staleTime: 30_000,
  })

  const recapQuery = useQuery({
    queryKey: ['analytics-console-recap', targetQueryStreamId],
    queryFn: () => getPulseStreamRecap(targetQueryStreamId),
    enabled: Boolean(layer2Enabled && targetQueryStreamId && chartDetailReady),
    staleTime: 120_000,
    retry: 1,
  })

  const heatmapQuery = useQuery({
    queryKey: ['analytics-console-heatmap', targetQueryStreamId, channelLogin],
    queryFn: async () => {
      const data = await getReplayHeatmap(targetQueryStreamId, 60, channelLogin)
      return (data ?? null) as HeatmapResponse | null
    },
    enabled: Boolean(layer2Enabled && targetQueryStreamId && channelLogin && chartDetailReady),
    staleTime: 120_000,
    retry: 1,
  })

  const heatmapPoints = heatmapQuery.data?.points

  const recapForStream =
    recapQuery.isSuccess
    && recapQuery.data
    && recapQuery.data.streamId === targetQueryStreamId
      ? recapQuery.data
      : null

  const isActiveLiveCollector = isActiveLiveCollectorStream(detail?.stream, detail?.state)
  const isLive = isLiveRoute && (detail?.state === 'live' || Boolean(liveQuery.data?.stream?.streamId))

  useEffect(() => {
    appliedDeepLinkKey.current = null
  }, [targetQueryStreamId, location.hash, location.search])

  useEffect(() => {
    const rollups = detail?.rollups ?? []
    if (!rollups.length) return
    const deepLinkKey = `${targetQueryStreamId}:${location.hash}:${location.search}`
    if (appliedDeepLinkKey.current === deepLinkKey) return
    const offsetSeconds = parseDeepLinkOffset(location.hash, location.search)
    if (offsetSeconds == null) return
    const nearest = findNearestRollupByOffset(rollups, detail?.stream?.startedAt, offsetSeconds)
    if (nearest) setSelectedRollup(nearest)
    appliedDeepLinkKey.current = deepLinkKey
  }, [targetQueryStreamId, detail?.rollups, detail?.stream?.startedAt, location.hash, location.search])

  const activeStreamKey = detail?.stream?.streamId || targetQueryStreamId || channelLogin
  const [prevRailStreamKey, setPrevRailStreamKey] = useState(activeStreamKey)
  if (activeStreamKey !== prevRailStreamKey) {
    setPrevRailStreamKey(activeStreamKey)
    setRightPanelTab('moments')
  }

  const toggleSelected = useCallback((key: string) => {
    setEmotePlotSelection((current) =>
      toggleEmotePlotSelection(current, key, detail?.topEmotes ?? [], viewMode),
    )
  }, [detail?.topEmotes, viewMode])

  const clearEmotePlots = useCallback(() => {
    setEmotePlotSelection('none')
  }, [])

  const resetEmotePlots = useCallback(() => {
    setEmotePlotSelection('auto')
  }, [])

  const handleViewMode = useCallback((next: AnalyticsViewMode) => {
    setViewMode(next)
    if (next === 'emotes') setRightPanelTab('emotes')
    else if (next === 'spikes') setRightPanelTab('moments')
  }, [])

  const handleJumpToRecapOffset = useCallback(
    (offsetSeconds: number) => {
      const rollups = detail?.rollups ?? []
      if (!rollups.length) return
      const nearest = findNearestRollupByOffset(rollups, detail?.stream?.startedAt, offsetSeconds)
      if (nearest) setSelectedRollup(nearest)
    },
    [detail?.rollups, detail?.stream?.startedAt],
  )

  const handleRefresh = useCallback(async () => {
    if (!channelLogin || refreshing) return
    setRefreshing(true)
    try {
      const refetches: Array<Promise<unknown>> = [
        streamsQuery.refetch(),
        historyQuery.refetch(),
        detailQuery.refetch(),
      ]
      if (layer2Enabled && targetQueryStreamId) {
        if (showGameSegments) refetches.push(gamesQuery.refetch())
        refetches.push(syncQuery.refetch())
        refetches.push(summaryQuery.refetch())
        refetches.push(recapQuery.refetch())
        refetches.push(heatmapQuery.refetch())
      }
      await Promise.race([Promise.all(refetches), new Promise((resolve) => setTimeout(resolve, 30_000))])
      setLastRefreshedAt(Date.now())
    } finally {
      setRefreshing(false)
    }
  }, [
    channelLogin,
    refreshing,
    streamsQuery,
    historyQuery,
    detailQuery,
    gamesQuery,
    syncQuery,
    summaryQuery,
    recapQuery,
    heatmapQuery,
    layer2Enabled,
    showGameSegments,
    targetQueryStreamId,
  ])

  const refetchChartDuringSync = useCallback(async () => {
    await detailQuery.refetch()
    await streamsQuery.refetch()
    if (layer2Enabled) {
      await summaryQuery.refetch()
    }
  }, [detailQuery, streamsQuery, summaryQuery, layer2Enabled])

  const handleSync = useCallback(
    async (opts?: { viewersOnly?: boolean; forceChat?: boolean }) => {
      if (!enableSyncActions || !targetQueryStreamId || syncing) return
      setSyncing(true)
      setSyncError(null)
      setSyncNotice(null)
      setLiveSyncStatus(null)
      setRightPanelTab('status')
      try {
        const start = (await startHistoricalSync(targetQueryStreamId, channelLogin, {
          viewersOnly: opts?.viewersOnly,
          forceChat: opts?.forceChat,
          vodId: detail?.stream?.vodId,
        })) as { accepted?: boolean; status?: SyncStatus }
        if (start.status) setLiveSyncStatus(start.status)
        if (start.accepted === false && start.status && !isTerminalSyncPhase(start.status.phase)) {
          setSyncNotice('Sync already running — showing live progress.')
        }
        const finalStatus = await pollSyncUntilDone(targetQueryStreamId, setLiveSyncStatus, {
          onProgress: () => {
            void refetchChartDuringSync()
          },
        })
        if (!finalStatus) {
          setSyncError('Lost sync status — try again or use Refresh data.')
          return
        }
        if (finalStatus.stale) {
          setSyncNotice('Sync interrupted — click sync to retry.')
          return
        }
        if (finalStatus.phase === 'failed') {
          setSyncError(finalStatus.error || 'Sync failed.')
          return
        }
        setSyncNotice('Sync finished — charts updated.')
        await refetchChartDuringSync()
        await syncQuery.refetch()
      } catch (err: unknown) {
        setSyncError(err instanceof Error ? err.message : 'Sync failed.')
      } finally {
        setSyncing(false)
      }
    },
    [
      enableSyncActions,
      targetQueryStreamId,
      syncing,
      channelLogin,
      detail?.stream?.vodId,
      refetchChartDuringSync,
      syncQuery,
    ],
  )

  useEffect(() => {
    if (!enableSyncActions || !layer2Enabled || !targetQueryStreamId) return
    let cancelled = false
    void (async () => {
      const status = await getSyncStatus(targetQueryStreamId).catch(() => null)
      if (cancelled || !status || isTerminalSyncPhase(status.phase) || status.stale) return
      setSyncing(true)
      setLiveSyncStatus(status)
      setSyncNotice('Sync in progress — resuming live progress.')
      setRightPanelTab('status')
      const finalStatus = await pollSyncUntilDone(targetQueryStreamId, setLiveSyncStatus, {
        onProgress: () => {
          void refetchChartDuringSync()
        },
      })
      if (cancelled) return
      if (finalStatus && !finalStatus.stale && finalStatus.phase !== 'failed') {
        await refetchChartDuringSync()
      }
      setSyncing(false)
    })()
    return () => {
      cancelled = true
    }
  }, [enableSyncActions, layer2Enabled, targetQueryStreamId, refetchChartDuringSync])

  const stream = detail?.stream
  const streamVodId = resolveAnalyticsVodId(detail, recapForStream?.vodId)
  const vodLinkState = useMemo(
    () =>
      resolveVodLinkState({
        detail,
        recapVodId: recapForStream?.vodId,
        isLiveCollector: isActiveLiveCollector,
      }),
    [detail, recapForStream?.vodId, isActiveLiveCollector],
  )
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

  const chartEmoteKeys = useMemo(
    () => resolveChartEmoteKeys(emotePlotSelection, detail?.topEmotes ?? [], viewMode),
    [emotePlotSelection, detail?.topEmotes, viewMode],
  )

  const chartEmoteKeysOrdered = useMemo(() => Array.from(chartEmoteKeys), [chartEmoteKeys])

  const chartGames = useMemo(() => {
    if (!layer2Enabled || !showGameSegments) return []
    return deriveChartGameSegments(targetQueryStreamId, detail, gamesQuery.data, {
      // Public portal: only render backend game segments, never silent category synthesis.
      allowCategoryFallback: mode !== 'public' && showGameSegments,
    })
  }, [layer2Enabled, showGameSegments, mode, targetQueryStreamId, detail, gamesQuery.data])

  const needsLiveCollectorRedirect = useMemo(() => {
    if (detailQuery.isLoading || streamsQuery.isLoading) return false
    if (isHistoricalRoute) return false
    if (!isLiveRoute) return false
    const rollups = detail?.rollups ?? []
    if (rollups.some(rollupHasMinuteData) || rollupsHaveViewerData(rollups)) return false
    if (isActiveLiveCollectorStream(detail?.stream, detail?.state)) return false
    if ((detail?.stream?.currentViewers ?? 0) > 0) return false
    return sidebarStreams.some((s) => (s.viewerSamples ?? 0) > 0 || (s.chatMessages ?? 0) > 0)
  }, [
    detail?.rollups,
    detail?.stream,
    detail?.state,
    detailQuery.isLoading,
    isHistoricalRoute,
    isLiveRoute,
    sidebarStreams,
    streamsQuery.isLoading,
  ])

  const syncedLiveStreamTarget = useMemo(() => {
    if (!needsLiveCollectorRedirect) return undefined
    return pickSyncedLiveStreamTarget(sidebarStreams, {
      liveStreamId: detail?.stream?.streamId,
      channelLive: true,
    })
  }, [sidebarStreams, detail?.stream?.streamId, needsLiveCollectorRedirect])

  const syncedLiveStreamSlug = useMemo(() => {
    if (!syncedLiveStreamTarget) return undefined
    return analyticsStreamPathSlug(syncedLiveStreamTarget, sidebarStreams)
  }, [syncedLiveStreamTarget, sidebarStreams])

  useEffect(() => {
    if (!channelLogin || !syncedLiveStreamSlug || !needsLiveCollectorRedirect) return
    if (detailQuery.isLoading || streamsQuery.isLoading) return
    if (syncedLiveStreamTarget?.streamId === targetQueryStreamId) return
    navigate(buildSessionPath(channelLogin, syncedLiveStreamSlug), { replace: true })
  }, [
    buildSessionPath,
    channelLogin,
    detailQuery.isLoading,
    navigate,
    needsLiveCollectorRedirect,
    streamsQuery.isLoading,
    syncedLiveStreamSlug,
    syncedLiveStreamTarget?.streamId,
    targetQueryStreamId,
  ])

  const activeMinutesUnavailable = Boolean(
    isHistoricalRoute
    && !detailQuery.isLoading
    && matchedStream
    && (detail?.minutesUnavailable || !detail?.rollups?.some(rollupHasMinuteData))
    && ((matchedStream.viewerSamples ?? 0) > 0 || (matchedStream.chatMessages ?? 0) > 0),
  )

  const handlePreviewRecapOffset = useCallback(
    (offsetSeconds: number | null) => {
      if (offsetSeconds == null) {
        setPreviewRollup(null)
        return
      }
      const rollups = detail?.rollups ?? []
      if (!rollups.length) return
      const nearest = findNearestRollupByOffset(rollups, detail?.stream?.startedAt, offsetSeconds)
      if (nearest) setPreviewRollup(nearest)
    },
    [detail?.rollups, detail?.stream?.startedAt],
  )

  const syncLabel = useMemo(() => {
    const rollups = detail?.rollups ?? []
    const hasChat = rollups.some((row) => (row.chatCount ?? 0) > 0 || (row.totalEmoteCount ?? 0) > 0)
    const hasViewers = rollups.some((row) => (row.viewerSamples ?? 0) > 0 || (row.viewerAvg ?? 0) > 0)
    return syncCtaLabel({ syncing, hasChatRollups: hasChat, hasViewerSamples: hasViewers })
  }, [detail?.rollups, syncing])

  const qualityDiagnosis = useMemo(
    () =>
      diagnoseStreamQuality({
        detail,
        summaryMetrics: summaryQuery.data?.metrics,
        analyticsQuality: summaryQuery.data?.analyticsQuality ?? detail?.analyticsQuality,
        isLive: isActiveLiveCollector,
        syncing: syncing || detail?.state === 'syncing',
      }),
    [detail, summaryQuery.data, isActiveLiveCollector, syncing],
  )

  const activeSyncStatus = liveSyncStatus ?? syncQuery.data ?? null

  if (!channelLogin) {
    return (
      <section className="analytics-console" aria-label="Streamclone analytics console">
        <p className="muted">Missing channel login.</p>
      </section>
    )
  }

  const headerTitle = displayStreamTitle(stream, channelLogin)

  return (
    <section className="analytics-console text-zinc-200" aria-label={`Analytics for ${channelLogin}`}>
      <div className="flex w-full flex-col gap-4">
        <header className="flex flex-col gap-3 border-b border-white/[0.07] pb-4 lg:flex-row lg:items-center lg:justify-between">
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
              {mode === 'public' ? (
                <>
                  <ViewerSourceBadge source={detail?.viewerSource} />
                  <AnalyticsQualityChip detail={detail} summaryMetrics={summaryQuery.data?.metrics} />
                  <CoverageFacets detail={detail} summaryMetrics={summaryQuery.data?.metrics} />
                </>
              ) : null}
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
            {liveActiveStreamId && isLiveRoute ? (
              <button
                type="button"
                onClick={() => navigate(buildSessionPath(channelLogin, liveActiveStreamId))}
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
              title="Reload chart and stats from server (does not start a sync job)"
            >
              {refreshing ? 'Refreshing…' : 'Refresh data'}
            </button>
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
              ? 'grid gap-4 lg:grid-cols-[minmax(260px,280px)_minmax(0,1fr)_minmax(260px,280px)]'
              : 'grid gap-4 xl:grid-cols-[minmax(260px,280px)_minmax(0,1fr)_minmax(260px,280px)]'
          }
        >
          <aside className="order-3 min-w-0 w-full xl:order-none xl:sticky xl:top-4 xl:self-start">
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
              activeMinutesUnavailable={activeMinutesUnavailable}
            />
          </aside>

          <section className="order-1 min-w-0 xl:order-none">
            {sessionResolving ? (
              <div className="rounded border border-white/[0.07] bg-white/[0.025] px-4 py-8 text-center text-sm font-semibold text-zinc-500">
                Resolving session…
              </div>
            ) : null}
            {sessionNotFound ? (
              <div className="rounded border border-amber-500/20 bg-amber-500/[0.06] px-4 py-8 text-center text-sm font-semibold text-amber-100/90">
                Session not found for <span className="font-mono">{streamId}</span>. Pick another stream from the sidebar.
              </div>
            ) : null}
            {!sessionResolving && !sessionNotFound ? (
            <div className="min-w-0 space-y-4">
              <CoverageStartBanner offsetSeconds={detail?.coverageStartOffsetSeconds} />
              <StreamQualityBanner
                diagnosis={qualityDiagnosis}
                syncing={syncing}
                canSync={enableSyncActions}
                onSync={() => void handleSync()}
                onSyncViewers={() => void handleSync({ viewersOnly: true })}
              />
              {syncError ? (
                <p className="text-[11px] font-semibold text-red-400">{syncError}</p>
              ) : null}
              {syncNotice && !syncError ? (
                <p className="text-[11px] font-semibold text-violet-300">{syncNotice}</p>
              ) : null}
              <AnalyticsChart
                detail={detail}
                selectedEmotes={chartEmoteKeys}
                onSelectEmote={toggleSelected}
                onClearEmotePlots={clearEmotePlots}
                onResetEmotePlots={resetEmotePlots}
                selectedRollup={selectedRollup}
                previewRollup={previewRollup}
                onSelectRollup={setSelectedRollup}
                onRefresh={() => void handleRefresh()}
                refreshing={refreshing}
                loading={detailQuery.isLoading && !detail}
                games={chartGames}
                canSync={enableSyncActions}
                syncing={syncing}
                onSync={() => void handleSync()}
                onChatOnlySync={enableSyncActions ? () => void handleSync({ forceChat: true }) : undefined}
                isLive={isActiveLiveCollector}
                syncCtaLabel={syncLabel}
                syncViewerStatus={activeSyncStatus?.viewerStatus}
                viewMode={viewMode}
                onViewModeChange={handleViewMode}
                vodLinkState={vodLinkState}
                topEmotesCatalog={detail?.topEmotes}
                onOpenAnalytics={() => setRightPanelTab('moments')}
              />
              {streamVodId ? (
                <p className="text-[11px] font-semibold text-zinc-500">
                  VOD {streamVodId} — select a moment for a timestamped jump, or{' '}
                  <a
                    href={buildTwitchVodUrl(streamVodId)}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-violet-300 hover:text-violet-200"
                  >
                    open the full VOD
                  </a>
                  {isLongStreamChart ? ' Long streams (6h+) may feel slower while hovering the chart.' : ''}
                </p>
              ) : vodLinkState.detail ? (
                <p className="text-[11px] font-semibold text-zinc-500">{vodLinkState.detail}</p>
              ) : null}
            </div>
            ) : null}
          </section>

          <aside className="order-2 w-full min-w-0 space-y-3 xl:order-none">
            {layer2Enabled && recapForStream ? (
              <StreamRecapPanel
                recap={recapForStream}
                topEmotesCatalog={detail?.topEmotes}
                onJumpToOffset={handleJumpToRecapOffset}
                onPreviewOffset={handlePreviewRecapOffset}
              />
            ) : null}
            <div className="w-full overflow-hidden rounded border border-white/[0.07] bg-white/[0.025]">
              <div className="flex border-b border-white/[0.07] text-[10px] font-black uppercase bg-white/[0.012]">
                {(['moments', 'emotes', 'status'] as const).map((tab) => (
                  <button
                    key={tab}
                    type="button"
                    onClick={() => setRightPanelTab(tab)}
                    className={`flex-1 py-2 text-center transition border-r border-white/[0.07] last:border-r-0 ${
                      rightPanelTab === tab ? 'bg-white/[0.028] text-zinc-200' : 'text-zinc-500 hover:text-zinc-400'
                    }`}
                  >
                    {tab === 'moments' ? 'Moments' : tab === 'emotes' ? 'Emotes' : 'Status'}
                  </button>
                ))}
              </div>
              <div className="p-0">
                {rightPanelTab === 'moments' ? (
                  <MomentReviewPanel
                    rollups={detail?.momentRollups ?? detail?.rollups ?? []}
                    selectedRollup={selectedRollup}
                    previewRollup={previewRollup}
                    onSelectRollup={setSelectedRollup}
                    onPreviewRollup={setPreviewRollup}
                    topEmotesCatalog={detail?.topEmotes}
                    heatmapPoints={layer2Enabled ? heatmapPoints : undefined}
                    streamStartedAt={stream?.startedAt}
                    embedded
                  />
                ) : null}
                {rightPanelTab === 'emotes' ? (
                  <TopEmoteTable
                    emotes={detail?.topEmotes ?? []}
                    selected={chartEmoteKeys}
                    plottedKeys={chartEmoteKeysOrdered}
                    onSelect={toggleSelected}
                    embedded
                  />
                ) : null}
                {rightPanelTab === 'status' && layer2Enabled ? (
                  <SyncStatusPanel detail={detail} syncStatus={activeSyncStatus} />
                ) : rightPanelTab === 'status' ? (
                  <p className="p-4 text-[11px] font-semibold text-zinc-500">
                    Open a session from the sidebar for sync status and Layer 2 detail.
                  </p>
                ) : null}
              </div>
            </div>
          </aside>
        </div>
      </div>
    </section>
  )
}
