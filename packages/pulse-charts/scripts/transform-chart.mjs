import fs from 'node:fs'

const path = 'c:/Users/Aron/twitch-7tv-clone/packages/pulse-charts/src/PulseMultiSignalChart.tsx'
let s = fs.readFileSync(path, 'utf8')

const header = `import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { parseEmoteKey } from '@streamclone/pulse-core'
import type { ChartGameSegment, ChartMinuteRollup, ChartPlayhead } from './types.ts'
import { computeChartCursorSync } from './chartCursorSync.ts'
import { lerpActivityLayout } from './emotePlotSelection.ts'
import { useSmoothedScalar } from './useSmoothedScalar.ts'
import { GameSegmentOverlay } from './GameSegmentOverlay.tsx'
import { normalizeGameSegments } from './gameSegments.ts'
import { CHART_THEME, emoteChartColor, hexToRgba, legendDotStyle } from './chartTheme.ts'
import {
  analyzeViewerCoverage,
  chartViewerValue,
  count,
  decimateSeriesForRender,
  formatVodClock,
  vodClock,
  minuteEmoteTotal,
  rollupHasMinuteData,
  rollupsHaveViewerData,
  rollingMedianWindow,
  viewerChartSmoothWindow,
  viewerSourceLabel,
  seriesMax,
  viewerValue,
} from './chartRollupUtils.ts'
`

s = s.replace(/^import[\s\S]*?from '\.\/chartRollupUtils\.ts'\n/, header)
s = s.replace(/AnalyticsMinuteRollup/g, 'ChartMinuteRollup')
s = s.replace(/GameSegment/g, 'ChartGameSegment')
s = s.replace(/ChartChartGameSegment/g, 'ChartGameSegment')

s = s.replace(/function normalizeGameSegments\([\s\S]*?\n\}\n\nfunction rollupsForChart/, 'function rollupsForChart')
s = s.replace(/export type AnalyticsViewMode[\s\S]*?const ChartHoverReadout = memo/, 'const ChartHoverReadout = memo')
s = s.replace(/export type RightPanelTab[\s\S]*?const ChartHoverReadout = memo/, 'const ChartHoverReadout = memo')

s = s.replace(/function AnalyticsChart\(/, 'function PulseMultiSignalChartInner(')
s = s.replace(/export default memo\(AnalyticsChart\)/, 'export const PulseMultiSignalChart = memo(PulseMultiSignalChartInner)')

fs.writeFileSync(path, s)
console.log('done', s.length)
