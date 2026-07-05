import type { AnalyticsTopEmote } from '../../apiTypes.ts'
import { parseEmoteKey } from '../../emoteUtils.ts'
import { count, getEmoteImageUrl } from '../../utils/consoleFormat.ts'
import { ConsoleEmoteImg } from './ConsoleEmoteImg.tsx'
import { EmoteProviderBadge } from './ConsoleBits.tsx'
import { emoteChartColorForKey, emoteChipSelectionStyle } from './chartTheme.ts'

export function TopEmoteTable({
  emotes,
  selected,
  plottedKeys,
  onSelect,
  embedded = false,
}: {
  emotes: AnalyticsTopEmote[]
  /** User toggled selection (may be empty while auto-plot fills chart). */
  selected: Set<string>
  /** Keys currently drawn on the chart, in stable plot order (matches chart line colors). */
  plottedKeys: string[]
  onSelect: (key: string) => void
  embedded?: boolean
}) {
  const plottedSet = new Set(plottedKeys)
  if (!emotes.length) {
    return (
      <div
        className={`grid min-h-44 place-items-center text-center ${
          embedded ? 'px-3 py-4' : 'rounded border border-white/10 bg-white/[0.035]'
        }`}
      >
        <div>
          <div className="text-sm font-semibold text-zinc-300">No emotes counted</div>
          <div className="mt-1 text-xs text-zinc-500">
            Collected chat has not matched known emotes yet.
          </div>
        </div>
      </div>
    )
  }
  return (
    <div
      className={`overflow-hidden ${
        embedded ? 'sc-console-scroll max-h-[calc(100vh-14rem)] overflow-y-auto' : 'rounded border border-white/10 bg-white/[0.035]'
      }`}
    >
      <div className="grid grid-cols-[minmax(0,1fr)_auto_auto] gap-2 border-b border-white/10 px-2 py-1.5 text-[10px] font-black uppercase text-zinc-500">
        <span>Emote</span>
        <span>Provider</span>
        <span className="text-right">Uses</span>
      </div>
      {emotes.slice(0, 20).map((emote) => {
        const imageUrl = getEmoteImageUrl(emote)
        const isSelected = selected.has(emote.key)
        const isPlotted = plottedSet.has(emote.key)
        const plotColor = isPlotted ? emoteChartColorForKey(emote.key, plottedKeys) : undefined
        const chipStyle = emoteChipSelectionStyle(plotColor, { selected: isSelected, plotted: isPlotted })
        const provider = emote.provider || parseEmoteKey(emote.key).provider
        return (
          <button
            key={emote.key}
            type="button"
            onClick={() => onSelect(emote.key)}
            className="grid w-full grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-2 border-b border-white/5 px-2 py-1.5 text-left text-xs last:border-b-0 transition hover:bg-white/[0.03]"
            style={{
              backgroundColor: chipStyle.backgroundColor,
              color: chipStyle.color,
              boxShadow: isPlotted && plotColor ? `inset 0 0 0 1px ${chipStyle.borderColor}` : undefined,
            }}
          >
            <span className="flex min-w-0 items-center gap-1.5">
              <span className="grid h-6 w-6 shrink-0 place-items-center rounded bg-black/30 p-0.5">
                <ConsoleEmoteImg
                  src={imageUrl}
                  name={emote.name}
                  className="max-h-full max-w-full object-contain"
                  fallbackClassName="inline-flex h-4 w-4 items-center justify-center rounded bg-white/10 text-[8px] font-black text-zinc-400"
                />
              </span>
              <span className="truncate font-semibold" title={emote.name}>
                {emote.name}
              </span>
            </span>
            <span className="flex shrink-0 items-center">
              {provider && provider !== 'unknown' ? (
                <EmoteProviderBadge provider={provider} />
              ) : (
                <span className="text-[10px] text-zinc-600">—</span>
              )}
            </span>
            <span className="shrink-0 text-right font-mono text-[11px] font-semibold tabular-nums text-zinc-400">
              {count(emote.count)}
            </span>
          </button>
        )
      })}
    </div>
  )
}
