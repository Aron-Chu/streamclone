import { useMemo } from 'react'
import type { ChartGameSegment, ChartMinuteRollup, ChartPlayhead } from './types.ts'
import { normalizeGameSegments } from './gameSegments.ts'
import { rollupsForChart } from './chartSession.ts'
import { PulseMultiSignalChartInner } from './PulseMultiSignalChart.tsx'

export interface PulseMultiSignalChartProps {
  rollups: ChartMinuteRollup[]
  games?: ChartGameSegment[]
  streamStartedAt?: string
  chartStreamId?: string | null
  peakViewersFallback?: number
  avgViewersFallback?: number
  viewerSource?: string
  selectedRollupIndex?: number | null
  previewRollupIndex?: number | null
  onSelectRollupIndex?: (index: number | null) => void
  selectedEmoteKeys?: Set<string>
  showSpikes?: boolean
  showMomentDots?: boolean
  showDots?: boolean
  activityExpanded?: boolean
  isLive?: boolean
  height?: number
  playhead?: ChartPlayhead | null
  loading?: boolean
  emptyMessage?: string
  syncing?: boolean
  variant?: 'console' | 'compact'
  motionEnabled?: boolean
  durationSeconds?: number
  chromeless?: boolean
}

export function PulseMultiSignalChart({
  rollups: allRollups,
  games = [],
  streamStartedAt,
  chartStreamId = null,
  peakViewersFallback = 0,
  avgViewersFallback = 0,
  viewerSource,
  selectedRollupIndex = null,
  previewRollupIndex = null,
  onSelectRollupIndex,
  selectedEmoteKeys = new Set(),
  showSpikes = false,
  showMomentDots = false,
  showDots = false,
  activityExpanded = false,
  isLive = false,
  height,
  playhead = null,
  loading = false,
  emptyMessage = 'Chart data not available yet.',
  syncing = false,
  variant = 'compact',
  motionEnabled = true,
  durationSeconds = 0,
  chromeless = false,
}: PulseMultiSignalChartProps) {
  const chartRollups = useMemo(() => rollupsForChart(allRollups, isLive), [allRollups, isLive])
  const selectedRollup = selectedRollupIndex != null ? chartRollups[selectedRollupIndex] ?? null : null
  const previewRollup = previewRollupIndex != null ? chartRollups[previewRollupIndex] ?? null : null
  const chartGames = useMemo(() => {
    const span = durationSeconds > 0 ? durationSeconds : Math.max(chartRollups.length * 60, 60)
    return normalizeGameSegments(games, span)
  }, [games, durationSeconds, chartRollups.length])

  if (loading && chartRollups.length === 0) {
    return (
      <div className="pulse-chart-empty" style={{ minHeight: height ?? 200, display: 'grid', placeItems: 'center', color: '#71717a', fontSize: 12 }}>
        Loading chart…
      </div>
    )
  }

  if (chartRollups.length === 0) {
    return (
      <div className="pulse-chart-empty" style={{ minHeight: height ?? 200, display: 'grid', placeItems: 'center', color: '#71717a', fontSize: 12, textAlign: 'center', padding: 12 }}>
        {emptyMessage}
      </div>
    )
  }

  return (
    <PulseMultiSignalChartInner
      rollups={chartRollups}
      games={chartGames}
      streamStartedAt={streamStartedAt}
      chartStreamId={chartStreamId}
      peakViewersFallback={peakViewersFallback}
      avgViewersFallback={avgViewersFallback}
      viewerSource={viewerSource}
      selectedEmotes={selectedEmoteKeys}
      selectedRollup={selectedRollup}
      previewRollup={previewRollup}
      onSelectRollup={rollup => {
        if (!onSelectRollupIndex) return
        if (!rollup) {
          onSelectRollupIndex(null)
          return
        }
        const idx = chartRollups.findIndex(point => point.minuteTs === rollup.minuteTs)
        onSelectRollupIndex(idx >= 0 ? idx : null)
      }}
      isLive={isLive}
      syncing={syncing}
      showSpikes={showSpikes || showMomentDots}
      showDots={showDots}
      activityExpanded={activityExpanded}
      height={height}
      playhead={playhead}
      variant={variant}
      motionEnabled={motionEnabled}
      chromeless={chromeless}
    />
  )
}
