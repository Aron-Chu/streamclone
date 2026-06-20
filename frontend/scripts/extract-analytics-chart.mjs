import fs from 'node:fs'
import path from 'node:path'

const root = path.resolve(import.meta.dirname, '..')
const srcPath = path.join(root, 'src/components/Analytics.tsx')
const outPath = path.join(root, 'src/components/analytics/AnalyticsChart.tsx')
const lines = fs.readFileSync(srcPath, 'utf8').split(/\r?\n/)

const startIdx = lines.findIndex(line => line.startsWith('function normalizeGameSegments('))
const endIdx = lines.findIndex((line, idx) => idx > startIdx && line.startsWith('function getLocalDateString('))
if (startIdx < 0 || endIdx < 0) {
  console.error('Could not find chart boundaries', { startIdx, endIdx })
  process.exit(1)
}

const header = `import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'

import type { AnalyticsMinuteRollup, AnalyticsStreamDetail, AnalyticsTopEmote, GameSegment } from '../../api.ts'
import { classifyLiveEmptyState } from '../../utils/liveEmptyState.ts'
import { computeChartCursorSync } from '../../utils/chartCursorSync.ts'
import { resolveEmoteImageUrl } from '../../utils/emoteImageUrl.ts'
import { usePlayheadStore } from '../../stores/playheadStore.ts'
import { CoreMinuteChartsNotice } from '../OptionalServicesPanel.tsx'
import LiveCollectionWarmup from './LiveCollectionWarmup.tsx'
import { CHART_THEME, hexToRgba, legendDotStyle } from './chartTheme.ts'
import {
  analyzeViewerCoverage,
  clock,
  count,
  formatVodClock,
  minuteEmoteTotal,
  rollupHasMinuteData,
  rollupsHaveViewerData,
  seriesMax,
  viewerValue,
} from './chartRollupUtils.ts'

function getEmoteImageUrl(emote: { provider?: string; id?: string; imageUrl?: string }) {
  const url = resolveEmoteImageUrl({
    provider: emote.provider,
    id: emote.id,
    imageUrl: emote.imageUrl,
    scale: '1x',
  })
  return url || undefined
}

type Series = {
  key: string
  label: string
  color: string
  values: Array<number | null>
  max: number
  dashed?: boolean
}

export type AnalyticsViewMode = 'overview' | 'emotes' | 'spikes'

const analyticsViewModes: Array<{ id: AnalyticsViewMode; label: string }> = [
  { id: 'overview', label: 'Overview' },
  { id: 'emotes', label: 'Emotes' },
  { id: 'spikes', label: 'Spikes' },
]

`

const body = lines.slice(startIdx, endIdx).join('\n')
const footer = '\n\nexport default memo(AnalyticsChart)\n'
fs.writeFileSync(outPath, header + body + footer)

const importLine = "import AnalyticsChart, { type AnalyticsViewMode } from './analytics/AnalyticsChart.tsx'"
const kept = [...lines.slice(0, startIdx), ...lines.slice(endIdx)]
const importAnchor = kept.findIndex(line => line.includes("import LiveCollectionWarmup from './analytics/LiveCollectionWarmup'"))
if (importAnchor >= 0) {
  kept.splice(importAnchor, 0, importLine)
}
// Remove duplicate AnalyticsViewMode type and analyticsViewModes const
const typeIdx = kept.findIndex(line => line.startsWith('type AnalyticsViewMode'))
if (typeIdx >= 0) {
  let removeEnd = typeIdx + 1
  while (removeEnd < kept.length && !kept[removeEnd].startsWith('function ') && !kept[removeEnd].startsWith('const analyticsViewModes')) {
    removeEnd++
  }
  const modesEnd = kept.findIndex((line, idx) => idx >= typeIdx && line.trim() === ']')
  if (modesEnd >= 0) {
    kept.splice(typeIdx, modesEnd - typeIdx + 1)
  }
}
fs.writeFileSync(srcPath, kept.join('\n'))
console.log('Extracted', startIdx + 1, 'to', endIdx, 'into', outPath)
