import { useMemo } from 'react'
import {
  count,
  emoteChartColorForKey,
  emoteLegendSwatchStyle,
} from '@streamclone/pulse-charts'
import type { AnalyticsTopEmote } from '../../apiTypes.ts'
import { MAX_PLOTTED_EMOTES } from '../../utils/emotePlotSelection.ts'
import { getEmoteImageUrl } from '../../utils/consoleFormat.ts'
import { emoteChipSelectionStyle } from './chartTheme.ts'
import { ConsoleEmoteImg } from './ConsoleEmoteImg.tsx'

const VISIBLE_CHIP_LIMIT = 16

export interface PlotOnChartStripProps {
  topEmotes: AnalyticsTopEmote[]
  plottedKeys: string[]
  onToggleEmote: (key: string) => void
  onClear?: () => void
  onReset?: () => void
  maxPlotted?: number
}

export function PlotOnChartStrip({
  topEmotes,
  plottedKeys,
  onToggleEmote,
  onClear,
  onReset,
  maxPlotted = MAX_PLOTTED_EMOTES,
}: PlotOnChartStripProps) {
  const plottedSet = useMemo(() => new Set(plottedKeys), [plottedKeys])
  if (topEmotes.length === 0) return null

  const visibleEmotes = topEmotes.slice(0, VISIBLE_CHIP_LIMIT)
  const hiddenCount = Math.max(0, topEmotes.length - VISIBLE_CHIP_LIMIT)
  const plottedCount = plottedKeys.length

  return (
    <div className="mt-3 overflow-hidden rounded border border-white/10 bg-white/[0.02]">
      <div className="flex flex-wrap items-center gap-2 border-b border-white/10 px-2.5 py-2">
        <span className="shrink-0 text-[10px] font-black uppercase tracking-wide text-zinc-500">
          Plot emotes
        </span>
        {plottedCount > 0 ? (
          <span className="text-[9px] font-bold uppercase tracking-wide text-zinc-600">
            {plottedCount}/{maxPlotted} on chart
          </span>
        ) : null}
        <span className="min-w-0 flex-1" />
        {onClear ? (
          <button
            type="button"
            onClick={onClear}
            className="rounded border border-white/10 px-2 py-0.5 text-[10px] font-black uppercase text-zinc-500 transition hover:border-white/20 hover:text-zinc-300"
          >
            Clear plots
          </button>
        ) : null}
        {onReset ? (
          <button
            type="button"
            onClick={onReset}
            className="rounded border border-white/10 px-2 py-0.5 text-[10px] font-black uppercase text-zinc-500 transition hover:border-white/20 hover:text-zinc-300"
          >
            Reset
          </button>
        ) : null}
      </div>
      <div className="flex max-h-32 flex-wrap gap-1.5 overflow-y-auto px-2 py-2 pr-1 sm:max-h-none">
        {visibleEmotes.map(emote => {
          const imageUrl = getEmoteImageUrl(emote)
          const isPlotted = plottedSet.has(emote.key)
          const plotColor = isPlotted ? emoteChartColorForKey(emote.key, plottedKeys) : undefined
          const chipStyle = emoteChipSelectionStyle(plotColor, { selected: isPlotted, plotted: isPlotted })
          const atCap = !isPlotted && plottedCount >= maxPlotted
          return (
            <button
              key={emote.key}
              type="button"
              onClick={() => onToggleEmote(emote.key)}
              disabled={atCap}
              title={
                atCap
                  ? `Max ${maxPlotted} emotes on chart`
                  : `${emote.name}: ${count(emote.count)} total uses — click to ${isPlotted ? 'hide' : 'show'} on chart`
              }
              className="inline-flex min-w-0 items-center gap-1 rounded border px-1.5 py-0.5 text-[10px] font-black transition disabled:cursor-not-allowed disabled:opacity-45"
              style={{
                borderColor: chipStyle.borderColor,
                backgroundColor: chipStyle.backgroundColor,
                color: chipStyle.color,
                boxShadow: isPlotted && plotColor ? `inset 2px 0 0 ${plotColor}` : undefined,
              }}
              aria-pressed={isPlotted}
            >
              {isPlotted && plotColor ? (
                <span style={emoteLegendSwatchStyle(plotColor)} aria-hidden="true" />
              ) : null}
              <ConsoleEmoteImg
                src={imageUrl}
                name={emote.name}
                className="h-4 w-4 shrink-0 object-contain"
                fallbackClassName="inline-flex h-4 w-4 shrink-0 items-center justify-center rounded bg-white/10 text-[8px] font-black text-zinc-400"
              />
              <span className="truncate">{emote.name}</span>
              <span className="shrink-0 font-bold tabular-nums text-zinc-500">{count(emote.count)}</span>
            </button>
          )
        })}
      </div>
      {hiddenCount > 0 ? (
        <p className="border-t border-white/5 px-2.5 py-1.5 text-[10px] font-semibold text-zinc-600">
          {hiddenCount} more emotes — use the Emotes tab for the full ranked list
        </p>
      ) : null}
    </div>
  )
}
