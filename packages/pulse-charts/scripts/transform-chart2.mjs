import fs from 'node:fs'

const path = 'c:/Users/Aron/twitch-7tv-clone/packages/pulse-charts/src/PulseMultiSignalChart.tsx'
let s = fs.readFileSync(path, 'utf8')

// Fix broken GameSegment type replacement
s = s.replace(/ChartChartGameSegment/g, 'ChartGameSegment')

// Export inner component
s = s.replace(
  'export const PulseMultiSignalChart = memo(PulseMultiSignalChartInner)',
  'export const PulseMultiSignalChartInner = memo(PulseMultiSignalChartInnerImpl)',
)
s = s.replace('function PulseMultiSignalChartInner(', 'function PulseMultiSignalChartInnerImpl(')

// Replace props block - from function to opening brace of body after type
const propsOld = /function PulseMultiSignalChartInnerImpl\(\{[\s\S]*?\}\) \{/
const propsNew = `function PulseMultiSignalChartInnerImpl({
  rollups: allRollups,
  games = [],
  streamStartedAt,
  chartStreamId = null,
  peakViewersFallback = 0,
  avgViewersFallback = 0,
  viewerSource,
  selectedEmotes = new Set<string>(),
  selectedRollup = null,
  previewRollup = null,
  onSelectRollup,
  syncing = false,
  isLive = false,
  showSpikes: showSpikesProp = false,
  showDots: showDotsProp = false,
  activityExpanded: activityExpandedProp = false,
  height: heightProp,
  playhead = null,
  variant = 'compact',
  motionEnabled = true,
}: {
  rollups: ChartMinuteRollup[]
  games?: ChartGameSegment[]
  streamStartedAt?: string
  chartStreamId?: string | null
  peakViewersFallback?: number
  avgViewersFallback?: number
  viewerSource?: string
  selectedEmotes?: Set<string>
  selectedRollup?: ChartMinuteRollup | null
  previewRollup?: ChartMinuteRollup | null
  onSelectRollup?: (rollup: ChartMinuteRollup | null) => void
  syncing?: boolean
  isLive?: boolean
  showSpikes?: boolean
  showDots?: boolean
  activityExpanded?: boolean
  height?: number
  playhead?: { streamId: string; offsetSeconds: number; isPlaying: boolean } | null
  variant?: 'console' | 'compact'
  motionEnabled?: boolean
}) {`

if (!propsOld.test(s)) {
  console.error('props block not found')
  process.exit(1)
}
s = s.replace(propsOld, propsNew)

// Playhead
s = s.replace(
  `  const playheadStreamId = usePlayheadStore(s => s.streamId)
  const playheadOffsetSeconds = usePlayheadStore(s => s.offsetSeconds)
  const playheadPlaying = usePlayheadStore(s => s.isPlaying)
  const cursorSync = computeChartCursorSync({
    chartStreamId: detail?.stream?.streamId ?? null,
    playhead: { streamId: playheadStreamId, isPlaying: playheadPlaying, offsetSeconds: playheadOffsetSeconds },
  })`,
  `  const cursorSync = computeChartCursorSync({
    chartStreamId,
    playhead: playhead ?? { streamId: '', offsetSeconds: 0, isPlaying: false },
  })`,
)

// Motion + spikes state
s = s.replace('  const [activityExpanded, setActivityExpanded] = useState(false)\n  const [showDots, setShowDots] = useState(false)', '  const [activityExpanded, setActivityExpanded] = useState(activityExpandedProp)\n  const [showDots, setShowDots] = useState(showDotsProp)')
s = s.replace('  const { motionEnabled } = useConsoleMotion()', '  // motionEnabled from props')
s = s.replace('  const [showSpikes, setShowSpikes] = useState(false)', '  const [showSpikes, setShowSpikes] = useState(showSpikesProp)')
s = s.replace(`  useEffect(() => {
    if (viewMode === 'spikes' || viewMode === 'emotes') setShowSpikes(true)
    if (viewMode === 'overview') setFocusedSeriesKey(null)
  }, [viewMode])`, `  useEffect(() => { setShowSpikes(showSpikesProp) }, [showSpikesProp])
  useEffect(() => { setShowDots(showDotsProp) }, [showDotsProp])
  useEffect(() => { setActivityExpanded(activityExpandedProp) }, [activityExpandedProp])`)

// Rollups from props
s = s.replace('  const allRollups = detail?.rollups ?? []\n  const streamStartedAt = detail?.stream?.startedAt\n  const rollups = useMemo(() => rollupsForChart(allRollups, isLive), [allRollups, isLive])', '  const rollups = allRollups')
s = s.replace('  const peakViewersFallback = detail?.stream?.peakViewers ?? 0\n  const avgViewersFallback = detail?.stream?.avgViewers ?? 0', '')

// viewer source
s = s.replace('viewerChartSmoothWindow(allRollups, detail?.viewerSource)', 'viewerChartSmoothWindow(allRollups, viewerSource)')
s = s.replace('[plotWidthPx, allRollups, detail?.viewerSource]', '[plotWidthPx, allRollups, viewerSource]')

// Remove empty state blocks - replace loading check
s = s.replace(
  /  if \(loading && !detail\) \{[\s\S]*?  \}\n\n  if \(!canRenderChart && \(detail\?\.state === 'syncing' \|\| syncing\)\) \{[\s\S]*?  \}\n\n  if \(!canRenderChart\) \{[\s\S]*?  \}\n\n/,
  `  if (!canRenderChart) {
    return (
      <div className="pulse-chart-empty" style={{ minHeight: heightProp ?? 200, display: 'grid', placeItems: 'center', color: '#71717a', fontSize: 12, padding: 12, textAlign: 'center' }}>
        Chart minutes not available yet.
      </div>
    )
  }

`,
)

// Height
s = s.replace('const height = CHART_VIEWBOX_HEIGHT', 'const height = heightProp ?? CHART_VIEWBOX_HEIGHT')

// chartGames normalize - use games prop directly (already normalized by wrapper)
s = s.replace('const chartGames = normalizeGameSegments(games, rollups.length)', 'const chartGames = games')

// playhead startedAt refs
s = s.replace(/detail\?\.stream\?\.startedAt/g, 'streamStartedAt')
s = s.replace(/detail\?\.stream\?\.avgViewers \?\? 0/g, 'avgViewersFallback')
s = s.replace(/detail\?\.viewerSource/g, 'viewerSource')

// onSelectRollup optional
s = s.replace(
  'if (rollups[idx]) {\n              onSelectRollup(rollups[idx])\n            }',
  'if (rollups[idx]) {\n              onSelectRollup?.(rollups[idx] ?? null)\n            }',
)
s = s.replace(
  'if (rollups[idx]) onSelectRollup(rollups[idx])',
  'if (rollups[idx]) onSelectRollup?.(rollups[idx] ?? null)',
)

// handleSpikeSelect
s = s.replace('if (rollups[idx]) onSelectRollup(rollups[idx])', 'if (rollups[idx]) onSelectRollup?.(rollups[idx] ?? null)')

// Hide console toolbar when compact
s = s.replace(
  '      <div className="mb-3 space-y-2">',
  `      {variant === 'console' ? <div className="mb-3 space-y-2">`,
)
// Close toolbar section before svg - find pattern after legend div
s = s.replace(
  '        </div>\n      </div>\n      <div className="overflow-hidden rounded">',
  '        </div>\n      </div> : null}\n      <div className="overflow-hidden rounded">',
)

// Hide banners when compact
s = s.replace(
  '    <div className="sc-chart-root rounded border border-white/10 bg-[#0d0d12] p-3" data-view-mode={viewMode}>',
  '    <div className="sc-chart-root rounded border border-white/10 bg-[#0d0d12] p-3" data-variant={variant}>',
)
s = s.replace(
  /      \{needsViewerResync \? \([\s\S]*?      \{syncError \? \([\s\S]*?      \) : null\}\n/,
  '',
)

// Remove emote footer for compact
s = s.replace(
  /      \{\(detail\?\.topEmotes \?\? \[\]\)\.length > 0 \? \([\s\S]*?      \) : null\}\n    <\/div>/,
  '    </div>',
)

// Game overlay -> GameSegmentOverlay
s = s.replace(
  /        \{\/\* Draw game dividers and labels \*\/\}[\s\S]*?        \}\)\}\n\n        \{\!cursorSync\.synced/,
  `        <GameSegmentOverlay
          segments={chartGames}
          rollups={rollups}
          streamStartedAt={streamStartedAt}
          padLeft={padLeft}
          padTop={padTop}
          padBottom={padBottom}
          plotWidth={plotWidthPx}
          height={height}
        />

        {!cursorSync.synced`,
)

// Remove partialChatCoverage and viewer backfill banners - already removed needsViewerResync block partially

// Fix remaining detail references
s = s.replace(/detail\?\.chatCoverage[^\n]*/g, '')
s = s.replace(/detail\?\.vodId[^\n]*/g, '')
s = s.replace(/detail\?\.sources[^\n]*/g, '')
s = s.replace(/detail\?\.state[^\n]*/g, '')
s = s.replace(/detail\?\.stream\?\.viewerSamples[^\n]*/g, '')
s = s.replace(/detail\?\.stream\?\.chatMessages[^\n]*/g, '')

// viewMode references in toolbar - wrap in variant check already partially done
s = s.replace(/viewMode/g, "'overview'")

// Fix broken replacements from viewMode
s = s.replace(/onViewModeChange\('overview'\)/g, 'undefined')
s = s.replace(/data-view-mode=\{'overview'\}/g, "data-view-mode='overview'")

fs.writeFileSync(path, s)
console.log('transform2 done')
