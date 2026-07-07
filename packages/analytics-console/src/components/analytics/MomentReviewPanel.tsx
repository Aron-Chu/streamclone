import { useEffect, useMemo, useRef } from 'react'
import {
  findRollupForMomentCandidate,
  formatHeatOffset,
  heatmapEmoteToRollupHit,
  heatmapPointsToMomentCandidates,
  LIVE_HEAT_RANKED_SUBTITLE,
  normalizeMinuteBucket,
  rollupFallbackMomentCandidates,
  topEmotesFromRollup,
  type RollupEmoteHit,
} from '@streamclone/pulse-core'
import type { AnalyticsMinuteRollup, AnalyticsTopEmote } from '../../apiTypes.ts'
import type { ReplayHeatmapPoint } from '../../types/heatmap.ts'
import { minuteEmoteTotal, rollupHasMinuteData, viewerValue } from './chartRollupUtils.ts'
import { count, getEmoteImageUrl, rollupOffsetSeconds } from '../../utils/consoleFormat.ts'
import { ConsoleEmoteImg } from './ConsoleEmoteImg.tsx'
import { useConsoleMotion } from '../../hooks/useConsoleMotion.ts'

function momentRowClass(selected: boolean, previewed: boolean): string {
  if (selected) {
    return 'border-l-amber-400 border-amber-500/25 bg-amber-500/10 ring-1 ring-amber-400/15'
  }
  if (previewed) {
    return 'border-l-amber-300/60 border-transparent bg-amber-500/[0.05]'
  }
  return 'border-l-transparent border-white/[0.07] bg-white/[0.028] hover:bg-white/[0.04]'
}

function momentRowEmotes(
  rollup: AnalyticsMinuteRollup,
  catalog: AnalyticsTopEmote[] | undefined,
  heatmapPoints: ReplayHeatmapPoint[] | undefined,
): RollupEmoteHit[] {
  const fromRollup = topEmotesFromRollup(rollup, 3, catalog)
  if (fromRollup.length > 0) return fromRollup
  if (!heatmapPoints?.length) return []
  const bucket = normalizeMinuteBucket(rollup.minuteTs)
  const point = heatmapPoints.find(
    (entry) =>
      entry.minuteTs === rollup.minuteTs
      || normalizeMinuteBucket(entry.minuteTs) === bucket,
  )
  if (!point?.topEmotes?.length) return []
  return point.topEmotes
    .slice(0, 3)
    .map((emote) => heatmapEmoteToRollupHit(emote, catalog))
    .filter((emote): emote is RollupEmoteHit => Boolean(emote))
}

type MomentCandidateRow = {
  rollup: AnalyticsMinuteRollup
  scoreLabel: string
  reasonLabel: string
  estimated: boolean
}

export function MomentReviewPanel({
  rollups,
  selectedRollup,
  previewRollup,
  onSelectRollup,
  onPreviewRollup,
  topEmotesCatalog,
  heatmapPoints,
  streamStartedAt,
  embedded = false,
}: {
  rollups: AnalyticsMinuteRollup[]
  selectedRollup: AnalyticsMinuteRollup | null
  previewRollup?: AnalyticsMinuteRollup | null
  onSelectRollup: (rollup: AnalyticsMinuteRollup) => void
  onPreviewRollup?: (rollup: AnalyticsMinuteRollup | null) => void
  topEmotesCatalog?: AnalyticsTopEmote[]
  heatmapPoints?: ReplayHeatmapPoint[]
  streamStartedAt?: string
  embedded?: boolean
}) {
  const { motionEnabled } = useConsoleMotion()
  const scrollRef = useRef<HTMLDivElement>(null)

  const candidates = useMemo((): MomentCandidateRow[] => {
    const catalog = topEmotesCatalog
    const momentCandidates = heatmapPoints?.length
      ? heatmapPointsToMomentCandidates(heatmapPoints, streamStartedAt, catalog)
      : rollupFallbackMomentCandidates(rollups, catalog, streamStartedAt)

    const rows: MomentCandidateRow[] = []
    for (const candidate of momentCandidates) {
      const rollup = findRollupForMomentCandidate(rollups, candidate) as AnalyticsMinuteRollup | undefined
      if (!rollup) continue
      rows.push({
        rollup,
        scoreLabel: candidate.scoreLabel,
        reasonLabel: candidate.reasonLabel,
        estimated: candidate.estimated,
      })
    }
    return rows
  }, [heatmapPoints, rollups, streamStartedAt, topEmotesCatalog])

  useEffect(() => {
    if (!selectedRollup?.minuteTs) return
    const container = scrollRef.current
    if (!container) return
    const row = container.querySelector<HTMLElement>(
      `[data-moment-row][data-minute-ts="${CSS.escape(selectedRollup.minuteTs)}"]`,
    )
    row?.scrollIntoView({ block: 'nearest', behavior: motionEnabled ? 'smooth' : 'instant' })
  }, [selectedRollup?.minuteTs, motionEnabled])

  if (candidates.length < 2) {
    return (
      <div
        className={`${
          embedded ? 'px-3 py-4' : 'rounded border border-white/[0.07] bg-[#0d0d12] p-3'
        } text-center text-[11px] font-semibold text-zinc-500`}
      >
        {rollups.some(rollupHasMinuteData)
          ? 'Not enough peaks yet — wait for more minutes.'
          : 'Ranked moments appear once live minute buckets land.'}
      </div>
    )
  }

  return (
    <div className={embedded ? 'p-3' : 'rounded border border-white/[0.07] bg-[#0d0d12] p-3'}>
      <div className="mb-2 flex flex-col gap-0.5">
        <div className="text-[11px] font-black uppercase text-zinc-500">Top Moments</div>
        <p className="text-[10px] font-semibold text-zinc-600">{LIVE_HEAT_RANKED_SUBTITLE}</p>
      </div>
      <div ref={scrollRef} className="sc-console-scroll flex max-h-72 flex-col gap-1 overflow-y-auto">
        {candidates.map(({ rollup, scoreLabel, reasonLabel, estimated }) => {
          const offsetLabel = streamStartedAt
            ? formatHeatOffset(rollupOffsetSeconds(rollup, streamStartedAt))
            : new Date(rollup.minuteTs).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
          const isSelected = selectedRollup?.minuteTs === rollup.minuteTs
          const isPreviewed = !isSelected && previewRollup?.minuteTs === rollup.minuteTs
          const rowEmotes = momentRowEmotes(rollup, topEmotesCatalog, heatmapPoints)
          const viewers = viewerValue(rollup)
          const chatPerMin = rollup.chatCount ?? 0
          const emotesPerMin = minuteEmoteTotal(rollup)
          const scoreTitle = estimated
            ? `Score ${scoreLabel} — estimated from rollups until heatmap scoring is available.`
            : `Score ${scoreLabel} — backend replay heatmap.`

          return (
            <button
              key={rollup.minuteTs}
              type="button"
              data-moment-row
              data-minute-ts={rollup.minuteTs}
              onClick={() => onSelectRollup(rollup)}
              onMouseEnter={() => onPreviewRollup?.(rollup)}
              onMouseLeave={() => onPreviewRollup?.(null)}
              title={`${reasonLabel} · ${scoreTitle}`}
              className={`flex w-full flex-col gap-1.5 rounded border border-l-2 px-2 py-1.5 text-left text-xs transition ${momentRowClass(isSelected, isPreviewed)}`}
            >
              <div className="flex min-w-0 items-baseline justify-between gap-2">
                <span className="shrink-0 font-mono text-[10px] font-bold tabular-nums text-zinc-400">
                  {offsetLabel}
                </span>
                <span className="min-w-0 truncate text-[10px] font-semibold tabular-nums text-zinc-400">
                  {count(viewers)} viewers · {count(chatPerMin)}/min chat · {count(emotesPerMin)}/min emotes
                </span>
                <span
                  className="shrink-0 text-[10px] font-bold tabular-nums text-zinc-600"
                  title={scoreTitle}
                >
                  {scoreLabel}
                </span>
              </div>
              <div className="truncate text-[10px] font-semibold text-zinc-500">{reasonLabel}</div>
              {rowEmotes.length > 0 ? (
                <div className="flex min-w-0 flex-wrap items-center gap-1">
                  {rowEmotes.map((emote) => {
                    const emoteImageUrl = getEmoteImageUrl(emote)
                    return (
                      <span
                        key={`${rollup.minuteTs}-${emote.key}`}
                        title={`${emote.name}: ${count(emote.count)} uses`}
                        className="inline-flex max-w-full items-center gap-1 rounded border border-white/10 bg-white/[0.04] px-1.5 py-0.5 text-[10px] font-bold text-zinc-300"
                      >
                        <ConsoleEmoteImg
                          src={emoteImageUrl}
                          name={emote.name}
                          width={14}
                          height={14}
                          className="h-3.5 w-3.5 shrink-0 object-contain"
                          fallbackClassName="inline-flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded bg-white/[0.06] text-[7px] font-black text-zinc-500"
                        />
                        <span className="truncate">{emote.name}</span>
                        <span className="shrink-0 text-zinc-500">{count(emote.count)}</span>
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
  )
}
