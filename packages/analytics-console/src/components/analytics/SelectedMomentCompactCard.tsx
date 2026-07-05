import React from 'react'
import type { AnalyticsMinuteRollup, AnalyticsTopEmote } from '../../apiTypes.ts'
import type { ReplayHeatmapDetailPoint, ReplayHeatmapPoint } from '../../types/heatmap.ts'
import { buildSelectedMomentDisplay } from '../../utils/selectedMomentDisplay.ts'
import type { VodLinkState } from '../../utils/twitchVodUrl.ts'
import { count, getEmoteImageUrl } from '../../utils/consoleFormat.ts'
import { ConsoleEmoteImg } from './ConsoleEmoteImg.tsx'
import { useConsoleMotion } from '../../hooks/useConsoleMotion.ts'

export interface SelectedMomentCompactCardProps {
  rollup: AnalyticsMinuteRollup
  rollups: AnalyticsMinuteRollup[]
  startedAt?: string
  vodLinkState: VodLinkState
  topEmotesCatalog?: AnalyticsTopEmote[]
  heatmapPoint?: ReplayHeatmapPoint | null
  heatmapDetail?: ReplayHeatmapDetailPoint | null
  onOpenAnalytics?: () => void
}

export function SelectedMomentCompactCard({
  rollup,
  rollups,
  startedAt,
  vodLinkState,
  topEmotesCatalog,
  heatmapPoint,
  heatmapDetail,
  onOpenAnalytics,
}: SelectedMomentCompactCardProps) {
  const { motionEnabled } = useConsoleMotion()
  const display = buildSelectedMomentDisplay({
    rollup,
    rollups,
    startedAt,
    vodLinkState,
    topEmotesCatalog,
    heatmapPoint,
    heatmapDetail,
  })

  return (
    <div
      key={rollup.minuteTs}
      className={`mt-2 overflow-hidden rounded border border-violet-400/15 border-l-2 border-l-violet-400/55 bg-white/[0.025] px-3 py-2.5${motionEnabled ? ' sc-selected-moment-panel' : ''}`}
      aria-label={`Selected moment at ${display.offsetStr || display.scoreModel.reasonLabel}`}
    >
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <span className="text-[9px] font-black uppercase tracking-wide text-zinc-500">Selected moment</span>
        {display.offsetStr ? (
          <span className="text-xs font-black tabular-nums text-zinc-100">{display.offsetStr}</span>
        ) : null}
      </div>
      <p className="mt-1 text-sm font-bold text-zinc-100">{display.scoreModel.reasonLabel}</p>
      <p className="mt-0.5 text-[11px] font-semibold text-zinc-500">{display.activityLine}</p>
      {display.momentEmotes.length > 0 ? (
        <ul className="mt-2 grid list-none gap-1.5 p-0">
          {display.momentEmotes.map(emote => {
            const imageUrl = getEmoteImageUrl(emote)
            return (
              <li key={emote.key} className="flex items-center gap-2">
                <ConsoleEmoteImg
                  src={imageUrl}
                  name={emote.name}
                  className="h-[18px] w-[18px] object-contain"
                  fallbackClassName="inline-flex h-[18px] w-[18px] items-center justify-center rounded bg-white/10 text-[8px] font-black text-zinc-400"
                />
                <span className="min-w-0 flex-1 truncate text-xs font-bold text-zinc-200">{emote.name}</span>
                <span className="text-[11px] font-bold tabular-nums text-zinc-500">{count(emote.count)}</span>
              </li>
            )
          })}
        </ul>
      ) : null}
      {!display.vodUrl && vodLinkState.detail ? (
        <p className="mt-2 text-[10px] font-semibold leading-snug text-zinc-600">{vodLinkState.detail}</p>
      ) : null}
      <div className="mt-2.5 flex flex-wrap gap-1.5">
        {display.vodUrl ? (
          <a
            href={display.vodUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex rounded border border-violet-400/25 bg-violet-500/12 px-2.5 py-1 text-[10px] font-black text-violet-100 transition hover:border-violet-300/40 hover:bg-violet-500/20"
          >
            Jump
          </a>
        ) : (
          <span
            title={vodLinkState.detail}
            className="inline-flex rounded border border-white/10 bg-zinc-800/80 px-2.5 py-1 text-[10px] font-black text-zinc-500"
          >
            {vodLinkState.status === 'linked' ? 'Jump' : vodLinkState.label}
          </span>
        )}
        {onOpenAnalytics ? (
          <button
            type="button"
            onClick={onOpenAnalytics}
            className="inline-flex rounded border border-white/10 bg-white/[0.04] px-2.5 py-1 text-[10px] font-black text-zinc-200 transition hover:border-white/20 hover:bg-white/[0.07]"
          >
            Open Analytics
          </button>
        ) : null}
      </div>
    </div>
  )
}
