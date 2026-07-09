import { useEffect, useMemo, useState } from 'react'
import { heatmapPointsToMomentCandidates, isEmoteSpikeReason } from '@streamclone/pulse-core'
import type { AnalyticsTopEmote, PulseRecapMoment } from '../apiTypes.ts'
import type { ReplayHeatmapPoint } from '../types/heatmap.ts'

export const MOMENTS_INITIAL_VISIBLE = 5
export const MOMENTS_MAX_VISIBLE = 20
export const EMOTES_INITIAL_VISIBLE = 3

const MOMENT_DEDUPE_TOLERANCE_SECONDS = 60

export interface MomentScoreTone {
  badge: string
  text: string
}

export interface MomentRankAccent {
  badge: string
}

export interface MomentReasonChipTone {
  chip: string
}

export function momentScoreTone(score: number): MomentScoreTone {
  if (score >= 80) {
    return {
      badge: 'border-emerald-400/25 bg-emerald-500/15',
      text: 'text-emerald-200',
    }
  }
  if (score >= 60) {
    return {
      badge: 'border-cyan-400/25 bg-cyan-500/15',
      text: 'text-cyan-200',
    }
  }
  return {
    badge: 'border-amber-400/25 bg-amber-500/15',
    text: 'text-amber-200',
  }
}

export function momentRankAccent(index: number): MomentRankAccent {
  if (index === 0) {
    return {
      badge: 'bg-amber-500/20 text-amber-200',
    }
  }
  if (index === 1) {
    return {
      badge: 'bg-violet-500/20 text-violet-200',
    }
  }
  if (index === 2) {
    return {
      badge: 'bg-cyan-500/20 text-cyan-200',
    }
  }
  return {
    badge: 'bg-white/[0.06] text-zinc-500',
  }
}

export function momentReasonChipTone(reason: string): MomentReasonChipTone {
  const normalized = reason.trim().toLowerCase()
  if (normalized.includes('chat') && normalized.includes('spike')) {
    return { chip: 'border-cyan-400/20 bg-cyan-500/10 text-cyan-200' }
  }
  if (isEmoteSpikeReason(normalized)) {
    return { chip: 'border-emerald-400/20 bg-emerald-500/10 text-emerald-200' }
  }
  if (normalized.includes('viewer')) {
    return { chip: 'border-violet-400/20 bg-violet-500/10 text-violet-200' }
  }
  if (normalized.includes('game')) {
    return { chip: 'border-violet-400/20 bg-violet-500/10 text-violet-200' }
  }
  return { chip: 'border-white/10 bg-white/[0.04] text-zinc-400' }
}

export function collapsedVisibleCount(
  total: number,
  expanded: boolean,
  initialCount: number,
  maxCount: number,
): number {
  const capped = Math.min(total, maxCount)
  return expanded ? capped : Math.min(capped, initialCount)
}

export function collapseListSlice<T>(
  items: readonly T[],
  expanded: boolean,
  initialCount: number,
  maxCount: number,
): T[] {
  return items.slice(0, collapsedVisibleCount(items.length, expanded, initialCount, maxCount))
}

export function useCollapsedList<T>(
  items: readonly T[],
  initialCount: number,
  maxCount: number,
  resetKey?: string | number | null,
) {
  const [expanded, setExpanded] = useState(false)

  useEffect(() => {
    setExpanded(false)
  }, [resetKey])

  const cappedItems = useMemo(() => items.slice(0, maxCount), [items, maxCount])
  const visible = useMemo(
    () => collapseListSlice(cappedItems, expanded, initialCount, maxCount),
    [cappedItems, expanded, initialCount, maxCount],
  )
  const canExpand = cappedItems.length > initialCount
  const hiddenCount = expanded ? 0 : Math.max(0, cappedItems.length - initialCount)

  return {
    visible,
    expanded,
    canExpand,
    hiddenCount,
    toggle: () => setExpanded((value) => !value),
  }
}

export function enrichRecapMomentsFromHeatmap(
  recapMoments: PulseRecapMoment[],
  heatmapPoints: ReplayHeatmapPoint[] | undefined,
  streamStartedAt: string | undefined,
  topEmotesCatalog: AnalyticsTopEmote[] | undefined,
  maxCount: number,
  minBeforeBackfill = MOMENTS_INITIAL_VISIBLE,
): PulseRecapMoment[] {
  if (recapMoments.length >= minBeforeBackfill || !heatmapPoints?.length) {
    return recapMoments.slice(0, maxCount)
  }

  const candidates = heatmapPointsToMomentCandidates(
    heatmapPoints,
    streamStartedAt,
    topEmotesCatalog,
    maxCount,
  )
  const merged = [...recapMoments]

  for (const candidate of candidates) {
    if (merged.length >= maxCount) break
    const duplicate = merged.some(
      (moment) => Math.abs(moment.offsetSeconds - candidate.offsetSeconds) <= MOMENT_DEDUPE_TOLERANCE_SECONDS,
    )
    if (duplicate) continue
    merged.push({
      offsetSeconds: candidate.offsetSeconds,
      score: candidate.score,
      reasons: [candidate.reason],
    })
  }

  return merged
    .sort((a, b) => {
      if (b.score !== a.score) return b.score - a.score
      return a.offsetSeconds - b.offsetSeconds
    })
    .slice(0, maxCount)
}

export function CollapseListFooter({
  expanded,
  canExpand,
  hiddenCount,
  onToggle,
  expandLabel,
  collapseLabel,
}: {
  expanded: boolean
  canExpand: boolean
  hiddenCount: number
  onToggle: () => void
  expandLabel: (hiddenCount: number) => string
  collapseLabel: string
}) {
  if (!canExpand) return null

  return (
    <button
      type="button"
      onClick={onToggle}
      aria-expanded={expanded}
      className="w-full border-t border-white/[0.07] px-3 py-2 text-center text-[10px] font-black uppercase tracking-wide text-zinc-400 transition hover:bg-white/[0.04] hover:text-zinc-200"
    >
      {expanded ? collapseLabel : expandLabel(hiddenCount)}
    </button>
  )
}
