import type { ReplayHeatmapPoint } from '../../types/heatmap'
import { formatDuration } from '../../utils/durationFormat'

const REASON_LABELS: Record<string, string> = {
  chat_spike: 'Chat spike',
  seventv_spike: '7TV spike',
  twitch_emote_spike: 'Twitch emote spike',
  ffz_spike: 'FFZ spike',
  viewer_spike: 'Viewer spike',
  emote_spike: 'Emote spike',
  game_change: 'Game change',
  manual: 'Manual',
}

export interface MomentDrawerProps {
  selectedPoint: ReplayHeatmapPoint
  canPlay?: boolean
  canClip?: boolean
  playHref?: string
}

export function MomentDrawer({
  selectedPoint,
  playHref,
}: MomentDrawerProps) {
  const reasonLabel = REASON_LABELS[selectedPoint.reason] ?? selectedPoint.reason.replace(/_/g, ' ')

  return (
    <div className="rounded border border-violet-500/15 bg-violet-500/[0.04] px-3 py-2.5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-[10px] font-black uppercase text-violet-300/90">Moment</span>
            <span className="text-xs font-black tabular-nums text-white">
              {formatDuration(selectedPoint.offsetSeconds)}
            </span>
            <span className="rounded border border-white/10 bg-white/[0.04] px-1.5 py-0.5 text-[10px] font-bold text-zinc-300">
              {reasonLabel}
            </span>
            <span className="text-[10px] font-bold text-zinc-500">
              Score {Math.round(selectedPoint.score)}/100
            </span>
          </div>
          {selectedPoint.topEmotes.length > 0 ? (
            <div className="mt-2 flex flex-wrap items-center gap-2">
              {selectedPoint.topEmotes.slice(0, 4).map(emote => (
                <span
                  key={emote.id}
                  className="inline-flex items-center gap-1 rounded border border-white/10 bg-white/[0.04] px-1.5 py-0.5 text-[10px] font-bold text-zinc-300"
                  title={`${emote.name} (${emote.count})`}
                >
                  {emote.imageUrl ? (
                    <img src={emote.imageUrl} alt="" className="h-4 w-4 object-contain" loading="lazy" />
                  ) : null}
                  <span>{emote.name}</span>
                  <span className="text-zinc-500">{emote.count}</span>
                </span>
              ))}
            </div>
          ) : null}
        </div>
        {playHref ? (
          <a
            href={playHref}
            target="_blank"
            rel="noopener noreferrer"
            className="shrink-0 rounded bg-violet-600 px-3 py-1.5 text-[11px] font-black text-white transition hover:bg-violet-700"
          >
            Watch on Twitch
          </a>
        ) : null}
      </div>
    </div>
  )
}

export default MomentDrawer
