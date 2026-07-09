export type { ChartGameSegment, ChartMinuteRollup, ChartPlayhead } from './types.ts'
export { PulseMultiSignalChart, type PulseMultiSignalChartProps } from './PulseMultiSignalChartPublic.tsx'
export { PulseMultiSignalChartInner } from './PulseMultiSignalChart.tsx'
export { normalizeGameSegments, hasMeaningfulGameSegments, gameSegmentKey, gameSegmentOverlapsOffsetRange, gameSegmentVisibleSecondsInRange } from './gameSegments.ts'
export { gameSegmentPlotBounds } from './gameSegmentChart.ts'
export { GameSegmentOverlay, type GameSegmentOverlayProps } from './GameSegmentOverlay.tsx'
export { rollupsForChart } from './chartSession.ts'
export { buildChartSeries, type ChartSeries } from './chartSeries.ts'
export { CHART_THEME, emoteChartColor, emoteChartColorForKey, hexToRgba, emoteChipSelectionStyle, emoteLegendSwatchStyle, legendDotStyle } from './chartTheme.ts'
export {
  count,
  vodClock,
  formatVodClock,
  minuteEmoteTotal,
  rollupHasMinuteData,
  rollupsHaveViewerData,
  analyzeViewerCoverage,
  viewerSourceLabel,
  chartViewerValue,
  viewerValue,
} from './chartRollupUtils.ts'
