import { memo, useCallback, useEffect, useMemo, useState } from 'react'

import type { AnalyticsMinuteRollup, AnalyticsStreamDetail, GameSegment } from '../../api.ts'
import {
  PulseMultiSignalChartInner,
  analyzeViewerCoverage,
  buildChartSeries,
  count,
  formatVodClock,
  legendDotStyle,
  minuteEmoteTotal,
  normalizeGameSegments,
  rollupHasMinuteData,
  rollupsForChart,
  rollupsHaveViewerData,
  viewerSourceLabel,
  viewerValue,
  vodClock,
} from '@streamclone/pulse-charts'
import { classifyLiveEmptyState } from '../../utils/liveEmptyState.ts'
import { liveWarmupHintLine } from '../../utils/liveCollectionWarmup.ts'
import { diagnoseLiveViewerWarmup } from '../../utils/streamQuality.ts'
import { usePlayheadStore } from '../../stores/playheadStore.ts'
import { CoreMinuteChartsNotice } from '../CoreMinuteChartsNotice.tsx'
import LiveCollectionWarmup from './LiveCollectionWarmup.tsx'
import { PlotOnChartStrip } from './PlotOnChartStrip.tsx'
import { SelectedMomentPanel } from './SelectedMomentPanel.tsx'
import { useConsoleMotion } from '../../hooks/useConsoleMotion.ts'
import type { AnalyticsTopEmote } from '../../apiTypes.ts'
import type { ReplayHeatmapDetailPoint, ReplayHeatmapPoint } from '../../types/heatmap.ts'
import type { VodLinkState } from '../../utils/twitchVodUrl.ts'
import { getEmoteImageUrl } from '../../utils/consoleFormat.ts'
import { emoteChipSelectionStyle, emoteLegendSwatchStyle } from './chartTheme.ts'
import { ConsoleEmoteImg } from './ConsoleEmoteImg.tsx'

export type AnalyticsViewMode = 'overview' | 'emotes' | 'spikes'
export type RightPanelTab = 'moments' | 'emotes' | 'clips' | 'sync'

const analyticsViewModes: Array<{ id: AnalyticsViewMode; label: string }> = [
  { id: 'overview', label: 'Overview' },
  { id: 'emotes', label: 'Emotes' },
  { id: 'spikes', label: 'Spikes' },
]

function ChartHoverReadout({
  minuteTs,
  streamStartedAt,
  viewers,
  chatCount,
  emoteTotal,
}: {
  minuteTs?: string
  streamStartedAt?: string
  viewers: number | null
  chatCount?: number | null
  emoteTotal: number | null
}) {
  return (
    <p
      className="min-w-0 truncate text-xs font-bold tabular-nums text-zinc-500"
      title="Values at the hovered minute on the chart"
    >
      {vodClock(minuteTs, streamStartedAt)} · viewers {count(viewers)} · chat {count(chatCount)}/min · emotes {count(emoteTotal)}/min
    </p>
  )
}

function AnalyticsChart({
  detail,
  selectedEmotes,
  onSelectEmote,
  onClearEmotePlots,
  onResetEmotePlots,
  selectedRollup,
  previewRollup = null,
  onSelectRollup,
  syncing = false,
  syncError = null,
  syncNotice = null,
  onSync = () => {},
  onRefresh = () => {},
  refreshing = false,
  loading = false,
  games = [],
  canSync = false,
  isLive = false,
  notInAnalyticsDb = false,
  coreMinuteChartsBlocked = false,
  liveHasRichHistory = false,
  chatOnlySyncAvailable = false,
  onChatOnlySync,
  syncCtaLabel: syncCtaLabelText,
  syncViewerStatus,
  viewMode,
  onViewModeChange,
  vodLinkState,
  topEmotesCatalog,
  heatmapPoint,
  heatmapDetail,
  onOpenAnalytics: _onOpenAnalytics,
}: {
  detail?: AnalyticsStreamDetail
  selectedEmotes: Set<string>
  onSelectEmote: (key: string) => void
  onClearEmotePlots?: () => void
  onResetEmotePlots?: () => void
  selectedRollup: AnalyticsMinuteRollup | null
  previewRollup?: AnalyticsMinuteRollup | null
  onSelectRollup: (rollup: AnalyticsMinuteRollup | null) => void
  syncing?: boolean
  syncError?: string | null
  syncNotice?: string | null
  onSync?: () => void
  onRefresh?: () => void
  refreshing?: boolean
  loading?: boolean
  games?: GameSegment[]
  canSync?: boolean
  isLive?: boolean
  notInAnalyticsDb?: boolean
  coreMinuteChartsBlocked?: boolean
  liveHasRichHistory?: boolean
  chatOnlySyncAvailable?: boolean
  onChatOnlySync?: () => void
  syncCtaLabel?: string
  syncViewerStatus?: string
  viewMode: AnalyticsViewMode
  onViewModeChange: (mode: AnalyticsViewMode) => void
  vodLinkState?: VodLinkState
  topEmotesCatalog?: AnalyticsTopEmote[]
  heatmapPoint?: ReplayHeatmapPoint | null
  heatmapDetail?: ReplayHeatmapDetailPoint | null
  onOpenAnalytics?: () => void
}) {
  const [showSpikes, setShowSpikes] = useState(false)
  const [showDots, setShowDots] = useState(false)
  const [activityExpanded, setActivityExpanded] = useState(() => selectedEmotes.size > 0)
  const [focusedSeriesKey, setFocusedSeriesKey] = useState<string | null>(null)
  const [hoverRollup, setHoverRollup] = useState<AnalyticsMinuteRollup | null>(null)
  const playheadStreamId = usePlayheadStore(s => s.streamId)
  const playheadOffsetSeconds = usePlayheadStore(s => s.offsetSeconds)
  const playheadPlaying = usePlayheadStore(s => s.isPlaying)
  const { motionEnabled } = useConsoleMotion()

  useEffect(() => {
    if (viewMode === 'spikes') setShowSpikes(true)
    if (viewMode === 'overview') setFocusedSeriesKey(null)
    if (viewMode === 'emotes') {
      setActivityExpanded(true)
      setFocusedSeriesKey(null)
    }
  }, [viewMode])

  const allRollups = detail?.rollups ?? []
  const streamStartedAt = detail?.stream?.startedAt
  const rollups = useMemo(() => rollupsForChart(allRollups, isLive), [allRollups, isLive])
  const peakViewersFallback = detail?.stream?.peakViewers ?? 0
  const avgViewersFallback = detail?.stream?.avgViewers ?? 0
  const hasSyncedChat = rollups.some(point => !point.missing && (point.chatCount ?? 0) > 0)
  const viewerCoverage = useMemo(() => analyzeViewerCoverage(rollups), [rollups])
  const useViewerFallback = !isLive
    && !hasSyncedChat
    && rollups.every(point => point.missing || viewerValue(point) === 0)
  const needsViewerResync = !isLive && hasSyncedChat && (
    !viewerCoverage.hasViewerRollups
    || viewerCoverage.hasFlatViewerLine
    || viewerCoverage.hasPartialTail
    || viewerCoverage.hasShortSpan
  )
  const liveViewerWarmup = useMemo(
    () => diagnoseLiveViewerWarmup(rollups, isLive, streamStartedAt),
    [rollups, isLive, streamStartedAt],
  )
  const hasChartData = useMemo(() => allRollups.some(rollupHasMinuteData), [allRollups])
  const hasViewerChartData = useMemo(() => rollupsHaveViewerData(allRollups), [allRollups])
  const hasChatData = useMemo(
    () => rollups.some(point => (point.chatCount ?? 0) > 0 || minuteEmoteTotal(point) > 0),
    [rollups],
  )
  const liveViewerCollectHint = useMemo(
    () => (isLive && hasChatData && !hasViewerChartData
      ? liveWarmupHintLine({
        viewerSamples: detail?.stream?.viewerSamples,
        chatMessages: detail?.stream?.chatMessages,
      })
      : null),
    [isLive, hasChatData, hasViewerChartData, detail?.stream?.viewerSamples, detail?.stream?.chatMessages],
  )
  const canRenderChart = hasChartData || hasViewerChartData
  const viewerBackfillPending = syncing && (syncViewerStatus === 'pending_backfill' || syncViewerStatus === 'backfilling')
  const partialChatCoverage = !isLive && !syncing && Boolean(detail?.chatCoverage?.partial)

  const series = useMemo(
    () => buildChartSeries(rollups, selectedEmotes, peakViewersFallback, avgViewersFallback, useViewerFallback),
    [rollups, selectedEmotes, peakViewersFallback, avgViewersFallback, useViewerFallback],
  )
  const perEmoteSeries = useMemo(() => series.filter(item => item.dashed), [series])
  const plottedEmoteKeys = useMemo(() => perEmoteSeries.map(item => item.key), [perEmoteSeries])
  const topEmoteByKey = useMemo(() => {
    const map = new Map<string, AnalyticsTopEmote>()
    for (const emote of detail?.topEmotes ?? []) {
      if (emote.key) map.set(emote.key, emote)
    }
    return map
  }, [detail?.topEmotes])

  useEffect(() => {
    if (plottedEmoteKeys.length > 0) setActivityExpanded(true)
  }, [plottedEmoteKeys.length])

  const chartGames = useMemo(
    () => normalizeGameSegments(games, Math.max(rollups.length * 60, 60)),
    [games, rollups.length],
  )

  const hoverPoint = hoverRollup ?? rollups[rollups.length - 1] ?? null
  const toggleSeriesFocus = useCallback((seriesKey: string) => {
    setFocusedSeriesKey(current => (current === seriesKey ? null : seriesKey))
  }, [])

  if (loading && !detail) {
    return (
      <div className="grid min-h-80 place-items-center rounded border border-white/10 bg-[#0d0d12]/50 px-4 text-center">
        <div className="text-sm font-bold text-zinc-500">Loading chart data…</div>
      </div>
    )
  }

  if (!canRenderChart && (detail?.state === 'syncing' || syncing)) {
    return (
      <div className="grid min-h-80 place-items-center rounded border border-white/10 bg-[#0d0d12]/50 backdrop-blur-md px-4 text-center">
        <div>
          <div className="text-base font-black text-zinc-100">Syncing chart data…</div>
          <div className="mt-1 text-sm font-semibold text-zinc-500 max-w-md">
            Viewer minutes appear as soon as TwitchTracker finishes. Chat and emotes fill in segment by segment.
          </div>
          <div className="mt-2 text-xs font-semibold text-zinc-600">
            Step-by-step progress is in the Sync tab on the right.
          </div>
          {syncNotice ? <div className="mt-2 text-xs font-bold text-amber-300">{syncNotice}</div> : null}
          {syncError ? <div className="mt-2 text-xs font-bold text-red-400">{syncError}</div> : null}
        </div>
      </div>
    )
  }

  if (!canRenderChart) {
    if (coreMinuteChartsBlocked) {
      return (
        <div className="grid min-h-80 place-items-center rounded border border-white/10 bg-[#0d0d12]/50 backdrop-blur-md px-4 text-center">
          <CoreMinuteChartsNotice />
        </div>
      )
    }
    if (isLive && liveHasRichHistory) {
      return (
        <div className="grid min-h-80 place-items-center rounded border border-white/10 bg-[#0d0d12]/50 backdrop-blur-md px-4 text-center">
          <div>
            <div className="text-base font-black text-zinc-100">Live collector has no minute rollups yet</div>
            <div className="mt-1 text-sm font-semibold text-zinc-500 max-w-md">
              The IRC collector is running but has not written chart minutes for this session. Past synced streams are in the left rail — pick one for full charts, or wait and refresh.
            </div>
            <button
              type="button"
              onClick={onRefresh}
              disabled={refreshing}
              className="mt-5 rounded-lg border border-white/10 bg-white/[0.05] px-5 py-2.5 text-xs font-black uppercase tracking-wider text-zinc-200 transition hover:bg-white/10 disabled:opacity-50"
            >
              {refreshing ? 'Refreshing…' : 'Refresh data'}
            </button>
          </div>
        </div>
      )
    }
    const isTwitchTracker = detail?.sources?.some(s => s.source === 'twitchtracker')
    const canShowSync = canSync
    const liveEmpty = classifyLiveEmptyState({
      collectingNow: isLive,
      rollupCount: rollups.filter(point => !point.missing).length,
    })
    if (liveEmpty.kind === 'collecting-first-minutes') {
      const minuteDataCount = rollups.filter(point => rollupHasMinuteData(point)).length
      return (
        <div className="grid min-h-80 place-items-center rounded border border-white/10 bg-[#0d0d12]/50 backdrop-blur-md px-4 py-8 text-center">
          <LiveCollectionWarmup
            rollupMinuteCount={minuteDataCount}
            viewerSamples={detail?.stream?.viewerSamples}
            chatMessages={detail?.stream?.chatMessages}
          />
        </div>
      )
    }
    return (
      <div className="grid min-h-80 place-items-center rounded border border-white/10 bg-[#0d0d12]/50 backdrop-blur-md px-4 text-center">
        <div>
          <div className="text-base font-black text-zinc-100">
            {canSync && isTwitchTracker ? 'Chat & Emotes Offline' : 'No recent data'}
          </div>
          <div className="mt-1 text-sm font-semibold text-zinc-500 max-w-md">
            {canSync && isTwitchTracker
              ? 'This stream has TwitchTracker averages only. Sync pulls minute-level viewers, chat, and 7TV data (large VODs can take a few minutes).'
              : isTwitchTracker
                ? 'This stream has TwitchTracker averages only. Minute charts appear when backend collection completes.'
                : 'Analytics start collecting when this channel is viewed in Streamclone.'}
          </div>
          {notInAnalyticsDb && canSync ? (
            <div className="mt-2 text-[11px] font-semibold text-zinc-600">
              Stream not in analytics DB yet — sync will create it.
            </div>
          ) : null}
          {canShowSync ? (
            <div className="mt-5 flex w-full flex-col items-center gap-2">
              {chatOnlySyncAvailable && onChatOnlySync ? (
                <button
                  type="button"
                  onClick={onChatOnlySync}
                  disabled={syncing}
                  className="rounded-lg border border-violet-400/30 bg-violet-500/10 px-5 py-2.5 text-xs font-black uppercase tracking-wider text-violet-200 transition hover:bg-violet-500/20 disabled:opacity-50"
                >
                  Re-sync chat only
                </button>
              ) : null}
              <button
                type="button"
                onClick={onSync}
                disabled={syncing}
                className="rounded-lg border border-violet-400/30 bg-violet-500/10 px-5 py-2.5 text-xs font-black uppercase tracking-wider text-violet-200 transition hover:bg-violet-500/20 disabled:opacity-50"
              >
                {syncing ? 'Syncing…' : (syncCtaLabelText ?? 'Sync chat/emotes')}
              </button>
              {syncNotice ? <div className="mt-2 text-xs font-bold text-amber-300">{syncNotice}</div> : null}
              {syncError ? <div className="mt-2 text-xs font-bold text-red-400">{syncError}</div> : null}
            </div>
          ) : null}
        </div>
      </div>
    )
  }

  return (
    <div className="sc-chart-root rounded border border-white/10 bg-[#0d0d12] p-3" data-view-mode={viewMode}>
      {canSync && needsViewerResync ? (
        <div className="mb-3 rounded border border-amber-400/25 bg-amber-400/10 px-3 py-2 text-xs font-semibold text-amber-100">
          Viewer timeline is incomplete for this sync. Click <span className="font-black">Re-sync viewers</span> to pull the TwitchTracker viewer chart (fast — chat/7TV stay as-is).
        </div>
      ) : null}
      {liveViewerWarmup ? (
        <div className="mb-3 rounded border border-sky-400/25 bg-sky-400/10 px-3 py-2 text-xs font-semibold text-sky-100">
          {liveViewerWarmup.message}
        </div>
      ) : null}
      {liveViewerCollectHint ? (
        <div className="mb-3 rounded border border-violet-400/20 bg-violet-500/10 px-3 py-2 text-xs font-semibold text-violet-100">
          {liveViewerCollectHint}
        </div>
      ) : null}
      {partialChatCoverage ? (
        <div className="mb-3 rounded border border-amber-400/25 bg-amber-400/10 px-3 py-2 text-xs font-semibold text-amber-100">
          Chat only covers the first {formatVodClock((detail?.chatCoverage?.chatSpanMinutes ?? 0) * 60)} of this{' '}
          {formatVodClock((detail?.chatCoverage?.streamSpanMinutes ?? 0) * 60)} stream
          {detail?.vodId ? ` (VOD ${detail.vodId})` : ''}.
          {canSync ? ' Twitch may still be processing the archive — re-sync later.' : ' Backend collection may fill gaps when available.'}
        </div>
      ) : null}
      {viewerBackfillPending && hasViewerChartData ? (
        <div className="mb-3 rounded border border-cyan-400/25 bg-cyan-500/10 px-3 py-2 text-xs font-semibold text-cyan-100">
          Viewer line from {detail?.viewerSource === 'live' ? 'live collection' : viewerSourceLabel(detail?.viewerSource) || 'existing rollups'}; TwitchTracker backfill{' '}
          {syncViewerStatus === 'backfilling' ? 'running in background' : 'pending'}.
        </div>
      ) : null}
      {syncNotice ? (
        <div className="mb-3 rounded border border-amber-400/25 bg-amber-400/10 px-3 py-2 text-xs font-bold text-amber-200">{syncNotice}</div>
      ) : null}
      {syncError ? (
        <div className="mb-3 rounded border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs font-bold text-red-300">{syncError}</div>
      ) : null}

      <div className="mb-3 space-y-2">
        <div className="flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            {detail?.viewerSource ? (
              <div className="inline-flex shrink-0 items-center rounded border border-white/10 bg-white/[0.04] px-2 py-1 text-[10px] font-bold uppercase tracking-wide text-zinc-400">
                Viewers: {viewerSourceLabel(detail.viewerSource) || detail.viewerSource}
              </div>
            ) : null}
            <div className="inline-flex shrink-0 rounded border border-white/10 bg-white/[0.035] p-1 text-[10px] font-black uppercase">
              {analyticsViewModes.map(mode => (
                <button
                  key={mode.id}
                  type="button"
                  onClick={() => onViewModeChange(mode.id)}
                  className={`rounded px-3 py-1.5 transition ${
                    viewMode === mode.id
                      ? 'bg-white text-zinc-950'
                      : 'text-zinc-500 hover:bg-white/10 hover:text-zinc-200'
                  }`}
                >
                  {mode.label}
                </button>
              ))}
            </div>
          </div>
          <div className="flex min-w-0 flex-wrap items-center gap-2 lg:justify-end lg:gap-3">
            <ChartHoverReadout
              minuteTs={hoverPoint?.minuteTs}
              streamStartedAt={streamStartedAt}
              viewers={hoverPoint ? viewerValue(hoverPoint) : null}
              chatCount={hoverPoint?.chatCount}
              emoteTotal={hoverPoint ? minuteEmoteTotal(hoverPoint) : null}
            />
            {canSync && !coreMinuteChartsBlocked && (!hasChatData || needsViewerResync) ? (
              <button
                type="button"
                onClick={onSync}
                disabled={syncing}
                className="shrink-0 rounded border border-violet-400/30 bg-violet-500/10 px-2.5 py-1 text-[10px] font-black uppercase text-violet-200 transition hover:bg-violet-500/20 disabled:opacity-50"
              >
                {syncing ? 'Syncing…' : needsViewerResync ? 'Re-sync viewers' : 'Sync chat/emotes'}
              </button>
            ) : null}
            <div className="inline-flex shrink-0 items-center gap-0.5 rounded border border-white/10 bg-white/[0.03] p-0.5">
              <button
                type="button"
                onClick={onRefresh}
                disabled={refreshing}
                aria-label={refreshing ? 'Refreshing chart and stats' : 'Refresh chart and stats'}
                title="Reload chart and stats from server"
                className="rounded px-2 py-1 text-[10px] font-black uppercase text-zinc-500 transition hover:bg-white/[0.06] hover:text-zinc-300 disabled:opacity-50"
              >
                {refreshing ? '…' : '↻'}
              </button>
              <button
                type="button"
                onClick={() => setShowSpikes(value => !value)}
                aria-pressed={showSpikes}
                className={`rounded px-2 py-1 text-[10px] font-black uppercase transition ${showSpikes ? 'bg-emerald-400/10 text-emerald-200' : 'text-zinc-500 hover:bg-white/[0.06] hover:text-zinc-300'}`}
              >
                Spikes
              </button>
              <button
                type="button"
                onClick={() => setShowDots(value => !value)}
                aria-pressed={showDots}
                className={`rounded px-2 py-1 text-[10px] font-black uppercase transition ${showDots ? 'bg-cyan-400/10 text-cyan-200' : 'text-zinc-500 hover:bg-white/[0.06] hover:text-zinc-300'}`}
              >
                Dots
              </button>
              <button
                type="button"
                onClick={() => setActivityExpanded(value => !value)}
                aria-pressed={activityExpanded}
                className={`rounded px-2 py-1 text-[10px] font-black uppercase transition ${activityExpanded ? 'bg-violet-400/10 text-violet-200' : 'text-zinc-500 hover:bg-white/[0.06] hover:text-zinc-300'}`}
              >
                {activityExpanded ? 'Reset' : 'Expand'}
              </button>
            </div>
          </div>
        </div>

        <div className="flex max-h-24 flex-wrap gap-2 overflow-y-auto rounded border border-white/10 bg-white/[0.02] px-2 py-1.5 pr-1 sm:max-h-none">
          {series.map(item => {
            const catalogEmote = topEmoteByKey.get(item.key)
            const imageUrl = catalogEmote ? getEmoteImageUrl(catalogEmote) : undefined
            const isFocused = focusedSeriesKey === item.key
            const isDimmed = focusedSeriesKey != null && !isFocused
            const chipStyle = item.dashed
              ? emoteChipSelectionStyle(item.color, { selected: isFocused, plotted: isFocused })
              : null
            return (
              <button
                key={item.key}
                type="button"
                onClick={() => toggleSeriesFocus(item.key)}
                title={isFocused ? 'Click to show all series' : `Highlight ${item.label}`}
                className={`inline-flex shrink-0 items-center gap-1 rounded border px-2 py-0.5 text-[10px] font-black uppercase transition ${
                  chipStyle
                    ? ''
                    : isFocused
                      ? 'border-white/35 bg-white/[0.12] text-zinc-100 ring-1 ring-white/20'
                      : isDimmed
                        ? 'border-white/5 bg-transparent text-zinc-600 opacity-40'
                        : 'border-white/10 bg-white/[0.03] text-zinc-400 hover:border-white/20 hover:text-zinc-200'
                } ${isDimmed && chipStyle ? 'opacity-40' : ''}`}
                style={
                  chipStyle
                    ? {
                        borderColor: chipStyle.borderColor,
                        backgroundColor: chipStyle.backgroundColor,
                        color: chipStyle.color,
                      }
                    : undefined
                }
              >
                {item.dashed ? (
                  <span style={emoteLegendSwatchStyle(item.color)} aria-hidden="true" />
                ) : (
                  <span className="inline-block h-1.5 w-1.5 shrink-0 rounded-full" style={legendDotStyle(item.color)} />
                )}
                {imageUrl ? (
                  <ConsoleEmoteImg
                    src={imageUrl}
                    name={item.label}
                    className="inline-block h-3.5 w-3.5 shrink-0 object-contain"
                    fallbackClassName="inline-flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded bg-white/10 text-[8px] font-black text-zinc-400"
                  />
                ) : null}
                <span className="whitespace-nowrap">
                  {item.label} max {count(item.max)}
                </span>
              </button>
            )
          })}
        </div>
      </div>

      <PulseMultiSignalChartInner
        chromeless
        variant="console"
        rollups={rollups}
        games={chartGames}
        streamStartedAt={streamStartedAt}
        chartStreamId={detail?.stream?.streamId ?? null}
        peakViewersFallback={peakViewersFallback}
        avgViewersFallback={avgViewersFallback}
        viewerSource={detail?.viewerSource}
        selectedEmotes={selectedEmotes}
        selectedRollup={selectedRollup}
        previewRollup={previewRollup}
        onSelectRollup={onSelectRollup}
        syncing={syncing}
        isLive={isLive}
        showSpikes={showSpikes}
        showDots={showDots}
        activityExpanded={activityExpanded}
        motionEnabled={motionEnabled}
        playhead={{
          streamId: playheadStreamId ?? '',
          offsetSeconds: playheadOffsetSeconds,
          isPlaying: playheadPlaying,
        }}
        onHoverRollupChange={setHoverRollup}
        focusedSeriesKey={focusedSeriesKey}
        onFocusedSeriesKeyChange={setFocusedSeriesKey}
      />

      {(detail?.topEmotes ?? []).length > 0 ? (
        <PlotOnChartStrip
          topEmotes={detail?.topEmotes ?? []}
          plottedKeys={plottedEmoteKeys}
          onToggleEmote={onSelectEmote}
          onClear={onClearEmotePlots}
          onReset={onResetEmotePlots}
        />
      ) : null}

      {selectedRollup && vodLinkState ? (
        <SelectedMomentPanel
          rollup={selectedRollup}
          rollups={allRollups}
          startedAt={streamStartedAt}
          vodLinkState={vodLinkState}
          topEmotesCatalog={topEmotesCatalog ?? detail?.topEmotes}
          heatmapPoint={heatmapPoint}
          heatmapDetail={heatmapDetail}
        />
      ) : null}
    </div>
  )
}

export default memo(AnalyticsChart)
