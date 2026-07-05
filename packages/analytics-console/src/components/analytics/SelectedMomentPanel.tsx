import { useMemo } from 'react'
import type { AnalyticsMinuteRollup, AnalyticsTopEmote } from '../../apiTypes.ts'
import type { ReplayHeatmapDetailPoint, ReplayHeatmapPoint } from '../../types/heatmap.ts'
import { minuteEmoteTotal, viewerValue } from './chartRollupUtils.ts'
import type { VodLinkState } from '../../utils/twitchVodUrl.ts'
import { buildSelectedMomentDisplay } from '../../utils/selectedMomentDisplay.ts'
import { count, getEmoteImageUrl } from '../../utils/consoleFormat.ts'
import { ConsoleEmoteImg } from './ConsoleEmoteImg.tsx'
import { EmoteProviderBadge } from './ConsoleBits.tsx'
import { useConsoleMotion } from '../../hooks/useConsoleMotion.ts'

export function SelectedMomentPanel({
  rollup,
  rollups,
  startedAt,
  vodLinkState,
  topEmotesCatalog,
  heatmapPoint,
  heatmapDetail,
}: {
  rollup: AnalyticsMinuteRollup | null
  rollups: AnalyticsMinuteRollup[]
  startedAt?: string
  vodLinkState: VodLinkState
  topEmotesCatalog?: AnalyticsTopEmote[]
  heatmapPoint?: ReplayHeatmapPoint | null
  heatmapDetail?: ReplayHeatmapDetailPoint | null
}) {
  const { motionEnabled } = useConsoleMotion()
  const display = useMemo(
    () =>
      rollup
        ? buildSelectedMomentDisplay({
            rollup,
            rollups,
            startedAt,
            vodLinkState,
            topEmotesCatalog,
            heatmapPoint,
            heatmapDetail,
          })
        : null,
    [heatmapDetail, heatmapPoint, rollup, rollups, startedAt, topEmotesCatalog, vodLinkState],
  )

  if (!rollup || !display) {
    return (
      <div className="rounded border border-white/10 bg-[#0d0d12] p-4 text-center text-xs text-zinc-500 italic">
        Click the graph or a ranked row to select a moment.
      </div>
    )
  }

  const timeStr = new Date(rollup.minuteTs).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
  const dateStr = new Date(rollup.minuteTs).toLocaleDateString([], { month: 'short', day: 'numeric', year: 'numeric' })
  const { scoreModel, vodUrl } = display
  const offsetStr = display.offsetStr

  return (
    <div
      key={rollup.minuteTs}
      className={`relative overflow-hidden rounded border border-amber-500/10 bg-[#0d0d12] p-4${motionEnabled ? ' sc-selected-moment-panel' : ''}`}
    >
      <div className="absolute left-0 right-0 top-0 h-1 bg-gradient-to-r from-amber-500/25 via-amber-400/60 to-amber-500/25" />

      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="rounded bg-amber-500/10 px-2 py-0.5 text-xs font-black uppercase text-amber-300/80">
            Selected Moment
          </span>
          <span className="text-sm font-black text-white">
            {timeStr} · {dateStr}
          </span>
        </div>
        {vodUrl ? (
          <a
            href={vodUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex shrink-0 self-start whitespace-nowrap rounded border border-violet-400/20 bg-violet-500/10 px-3 py-1.5 text-xs font-black text-violet-100 transition hover:border-violet-300/40 hover:bg-violet-500/20"
            title={offsetStr ? `Open Twitch VOD at ${offsetStr}` : 'Open Twitch VOD'}
          >
            {vodLinkState.label}
            {offsetStr ? ` · ${offsetStr}` : ''}
          </a>
        ) : (
          <span
            title={vodLinkState.detail}
            className="inline-flex shrink-0 self-start whitespace-nowrap rounded border border-white/5 bg-zinc-800 px-3 py-1.5 text-xs font-black text-zinc-500"
          >
            {vodLinkState.label}
          </span>
        )}
      </div>

      {!vodUrl && vodLinkState.detail ? (
        <p className="mt-2 text-[11px] font-semibold leading-snug text-zinc-500">{vodLinkState.detail}</p>
      ) : null}

      <div className="mt-3 grid grid-cols-2 gap-x-4 gap-y-1 text-xs font-bold text-zinc-400 sm:grid-cols-3 lg:grid-cols-4">
        {offsetStr ? (
          <span>
            Stream offset: <strong className="text-zinc-200">{offsetStr}</strong>
          </span>
        ) : null}
        <span>
          Score:{' '}
          <strong className={scoreModel.estimated ? 'text-amber-200' : 'text-emerald-300'}>
            {scoreModel.label}
          </strong>
        </span>
        <span>
          Reason: <strong className="text-zinc-200">{scoreModel.reasonLabel}</strong>
        </span>
        {scoreModel.confidence !== null ? (
          <span>
            Confidence: <strong className="text-zinc-200">{Math.round(scoreModel.confidence * 100)}%</strong>
          </span>
        ) : null}
        <span>
          Viewers: <strong className="text-zinc-200">{count(viewerValue(rollup))}</strong>
        </span>
        <span>
          Chat activity: <strong className="text-zinc-200">{rollup.chatCount}/min</strong>
        </span>
        <span title="Emote uses in this selected minute">
          Emotes: <strong className="text-zinc-200">{minuteEmoteTotal(rollup)}/min</strong>
        </span>
        {(rollup.seventvEmoteCount ?? 0) > 0 ? (
          <span>
            7TV: <strong className="text-emerald-300">{rollup.seventvEmoteCount}/min</strong>
          </span>
        ) : null}
      </div>

      {display.momentEmotes.length > 0 ? (
        <div className="mt-3 flex flex-wrap gap-2">
          {display.momentEmotes.map((emote) => {
            const emoteImageUrl = getEmoteImageUrl(emote)
            return (
              <span
                key={emote.key}
                title={`${emote.name}: ${count(emote.count)} uses this minute`}
                className="inline-flex items-center gap-1.5 rounded border border-white/10 bg-white/[0.04] px-2 py-1 text-[11px] font-bold text-zinc-300"
              >
                <ConsoleEmoteImg
                  src={emoteImageUrl}
                  name={emote.name}
                  className="h-5 w-5 object-contain"
                  fallbackClassName="inline-flex h-5 w-5 items-center justify-center rounded bg-white/10 text-[9px] font-black text-zinc-400"
                />
                <span>{emote.name}</span>
                <EmoteProviderBadge provider={emote.provider} />
                <span className="text-zinc-500">{count(emote.count)}</span>
              </span>
            )
          })}
        </div>
      ) : null}

      {scoreModel.detailComponents.length > 0 ? (
        <div className="mt-3 flex flex-wrap gap-2">
          {scoreModel.detailComponents.slice(0, 4).map((component) => (
            <span
              key={component.key}
              className="rounded border border-white/10 bg-white/[0.035] px-2 py-1 text-[10px] font-bold text-zinc-400"
            >
              {component.key.replace(/_/g, ' ')} <strong className="text-zinc-200">{Math.round(component.weightedScore)}</strong>
            </span>
          ))}
        </div>
      ) : null}
    </div>
  )
}
