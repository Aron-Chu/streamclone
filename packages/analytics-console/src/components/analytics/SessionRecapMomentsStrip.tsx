import { useEffect, useMemo, useRef, useState } from 'react'
import {
  formatMomentTimeLabel,
  mergeRecapMoments,
  recapMomentReasonLabel,
} from '@streamclone/pulse-core'
import type {
  AnalyticsMinuteRollup,
  AnalyticsTopEmote,
  PulseRecapMoment,
  PulseStreamRecap,
} from '../../apiTypes.ts'
import type { ReplayHeatmapPoint } from '../../types/heatmap.ts'
import { resolveMomentEmotesForOffset } from '../../utils/recapEmoteEnrich.ts'
import { resolveMomentRowStats } from '../../utils/momentRowDisplay.ts'
import {
  CollapseListFooter,
  enrichRecapMomentsFromHeatmap,
  momentRankAccent,
  momentReasonChipTone,
  momentScoreTone,
  MOMENTS_INITIAL_VISIBLE,
  MOMENTS_MAX_VISIBLE,
  useCollapsedList,
} from '../../utils/momentListDisplay.tsx'
import { count, getEmoteImageUrl } from '../../utils/consoleFormat.ts'
import { ConsoleEmoteImg } from './ConsoleEmoteImg.tsx'

export function SessionRecapMomentsStrip({
  recap,
  streamStartedAt,
  selectedOffsetSeconds,
  onSelectOffset,
  onPreviewOffset,
  layout = 'rightRail',
  rollups = [],
  heatmapPoints,
  topEmotesCatalog,
}: {
  recap: PulseStreamRecap
  streamStartedAt?: string
  selectedOffsetSeconds?: number | null
  onSelectOffset: (offsetSeconds: number) => void
  onPreviewOffset?: (offsetSeconds: number | null) => void
  layout?: 'belowChart' | 'rightRail'
  rollups?: AnalyticsMinuteRollup[]
  heatmapPoints?: ReplayHeatmapPoint[]
  topEmotesCatalog?: AnalyticsTopEmote[]
}) {
  const hasReactionCoverage = useMemo(() => {
    const moments = [...(recap.topMoments ?? []), ...(recap.clipCandidates ?? [])]
    return moments.some(
      moment => (moment.chatCount ?? 0) > 0 || (moment.emoteCount ?? 0) > 0,
    ) || (recap.totalMessages ?? 0) > 0
  }, [recap])

  const moments = useMemo(() => {
    const merged = mergeRecapMoments(recap, undefined, MOMENTS_MAX_VISIBLE, hasReactionCoverage)
    return enrichRecapMomentsFromHeatmap(
      merged,
      heatmapPoints,
      streamStartedAt,
      topEmotesCatalog,
      MOMENTS_MAX_VISIBLE,
      MOMENTS_INITIAL_VISIBLE,
    )
  }, [recap, hasReactionCoverage, heatmapPoints, streamStartedAt, topEmotesCatalog])

  const {
    visible: visibleMoments,
    expanded: momentsExpanded,
    canExpand: canExpandMoments,
    hiddenCount: hiddenMomentCount,
    toggle: toggleMomentsExpanded,
  } = useCollapsedList(moments, MOMENTS_INITIAL_VISIBLE, MOMENTS_MAX_VISIBLE, recap.streamId)

  const [selectedOffset, setSelectedOffset] = useState<number | null>(
    selectedOffsetSeconds ?? moments[0]?.offsetSeconds ?? null,
  )
  const stripSelectedOffset = useRef<number | null>(null)

  useEffect(() => {
    if (selectedOffsetSeconds == null) return
    if (stripSelectedOffset.current != null) {
      const delta = Math.abs(selectedOffsetSeconds - stripSelectedOffset.current)
      if (delta <= 90) {
        stripSelectedOffset.current = null
      }
      return
    }
    setSelectedOffset(selectedOffsetSeconds)
  }, [selectedOffsetSeconds])

  useEffect(() => {
    stripSelectedOffset.current = null
  }, [recap.streamId])

  useEffect(() => {
    if (selectedOffset != null) return
    if (moments[0]) setSelectedOffset(moments[0].offsetSeconds)
  }, [moments, selectedOffset])

  const highlightOffset = selectedOffset ?? selectedOffsetSeconds ?? moments[0]?.offsetSeconds ?? null
  const highlightedMoment = useMemo(() => {
    if (moments.length === 0 || highlightOffset == null) return null
    let best = moments[0]
    let bestDelta = Math.abs(moments[0].offsetSeconds - highlightOffset)
    for (const moment of moments) {
      const delta = Math.abs(moment.offsetSeconds - highlightOffset)
      if (delta < bestDelta) {
        best = moment
        bestDelta = delta
      }
    }
    return bestDelta <= 90 ? best : null
  }, [highlightOffset, moments])

  if (moments.length === 0) return null

  function selectMoment(moment: PulseRecapMoment) {
    stripSelectedOffset.current = moment.offsetSeconds
    setSelectedOffset(moment.offsetSeconds)
    onSelectOffset(moment.offsetSeconds)
  }

  const isRightRail = layout === 'rightRail'

  return (
    <section
      className={`relative flex min-h-0 flex-col overflow-hidden rounded border border-white/[0.07] bg-white/[0.025] ${
        isRightRail ? 'flex-1' : 'scroll-mt-24'
      }`}
      aria-label="Pulse moments recap"
    >
      <div className="absolute left-0 right-0 top-0 h-0.5 bg-gradient-to-r from-amber-500/20 via-amber-400/50 to-amber-500/20" />

      <div className="shrink-0 px-3 pb-2 pt-3">
        <h3 className="text-[11px] font-black uppercase text-zinc-400">Pulse Moments</h3>
        <p className="mt-0.5 text-[10px] font-semibold text-zinc-600">
          Select a row — stats and emotes below the chart update
        </p>
      </div>

      <div className="sc-console-scroll min-h-0 flex-1 overflow-y-auto border-y border-white/[0.07]">
        <div className="flex flex-col gap-0.5 p-1">
          {visibleMoments.map((moment, index) => {
            const time = formatMomentTimeLabel({
              startedAtIso: streamStartedAt,
              offsetSeconds: moment.offsetSeconds,
            })
            const selected = highlightedMoment?.offsetSeconds === moment.offsetSeconds
            const stats = resolveMomentRowStats({
              moment,
              rollups,
              streamStartedAt,
            })
            const rowEmotes = resolveMomentEmotesForOffset({
              moment,
              rollups,
              streamStartedAt,
              heatmapPoints,
              topEmotesCatalog,
              limit: 3,
            })
            const reasonLabel = recapMomentReasonLabel(moment)
            const reasonCode = moment.reasons?.[0] ?? reasonLabel
            const rankAccent = momentRankAccent(index)
            const scoreTone = momentScoreTone(moment.score)
            const reasonTone = momentReasonChipTone(reasonCode)
            const rowBorder = selected
              ? 'border-amber-500/25 bg-amber-500/10 ring-1 ring-amber-400/15'
              : 'border-white/[0.07] bg-white/[0.028] hover:bg-white/[0.04]'
            const rowMotion = selected ? ' sc-moment-row-selected' : ''

            return (
              <button
                key={`${moment.offsetSeconds}:${moment.score}:${index}`}
                type="button"
                onClick={() => selectMoment(moment)}
                onMouseEnter={() => onPreviewOffset?.(moment.offsetSeconds)}
                onMouseLeave={() => onPreviewOffset?.(null)}
                className={`flex w-full flex-col gap-0.5 rounded border px-2 py-1 text-left text-xs transition ${rowBorder}${rowMotion}`}
              >
                <div className="flex min-w-0 items-center justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-1.5">
                    <span
                      className={`inline-flex h-4 min-w-[1.25rem] shrink-0 items-center justify-center rounded px-1 text-[9px] font-black tabular-nums ${rankAccent.badge}`}
                    >
                      #{index + 1}
                    </span>
                    <span className="font-mono text-[10px] font-bold tabular-nums text-zinc-300">
                      {time.primary}
                    </span>
                    {time.secondary ? (
                      <span className="truncate text-[9px] font-semibold text-zinc-600">{time.secondary}</span>
                    ) : null}
                  </div>
                  <span
                    className={`shrink-0 rounded border px-1.5 py-0.5 text-[10px] font-black tabular-nums ${scoreTone.badge} ${scoreTone.text}`}
                  >
                    {moment.score}
                  </span>
                </div>

                <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                  <span
                    className={`inline-flex max-w-full truncate rounded border px-1.5 py-0.5 text-[9px] font-black uppercase ${reasonTone.chip}`}
                  >
                    {reasonLabel}
                  </span>
                  <span className="min-w-0 truncate text-[9px] font-semibold tabular-nums text-zinc-500">
                    {count(stats.viewers)} viewers · {count(stats.chatPerMin)}/min · {count(stats.emotesPerMin)} emotes
                  </span>
                </div>

                {rowEmotes.length > 0 ? (
                  <div className="flex items-center gap-0.5 pt-0.5">
                    {rowEmotes.map((emote) => {
                      const emoteImageUrl = getEmoteImageUrl(emote)
                      const emoteLabel = `${emote.code}: ${count(emote.count)} uses`
                      return (
                        <span
                          key={`${moment.offsetSeconds}-${emote.code}-${emote.provider ?? ''}`}
                          title={emoteLabel}
                          aria-label={emoteLabel}
                          className="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded border border-white/10 bg-black/25 p-0.5"
                        >
                          <ConsoleEmoteImg
                            src={emoteImageUrl}
                            name={emote.code}
                            width={14}
                            height={14}
                            className="h-3.5 w-3.5 object-contain"
                            fallbackClassName="inline-flex h-3.5 w-3.5 items-center justify-center rounded bg-white/[0.06] text-[7px] font-black text-zinc-500"
                          />
                        </span>
                      )
                    })}
                  </div>
                ) : null}
              </button>
            )
          })}
        </div>
      </div>

      <CollapseListFooter
        expanded={momentsExpanded}
        canExpand={canExpandMoments}
        hiddenCount={hiddenMomentCount}
        onToggle={toggleMomentsExpanded}
        expandLabel={(count) => `Show ${count} more moment${count === 1 ? '' : 's'}`}
        collapseLabel="Show fewer moments"
      />
    </section>
  )
}
