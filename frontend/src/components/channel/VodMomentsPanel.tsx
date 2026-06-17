import { useMemo, useState } from 'react'

import type { AnalyticsStreamDetail } from '../../api.ts'
import {
  LIVE_HEAT_MAX_EMOTES,
  LIVE_HEAT_SUBTITLE,
  LIVE_HEAT_TITLE,
  deriveLiveHeat,
  formatHeatOffset,
  type LiveHeatPoint,
} from '../../utils/liveHeat.ts'
import { nearestMomentIndex } from '../../utils/vodSeek.ts'

export interface VodMomentsPanelProps {
  detail?: AnalyticsStreamDetail | null
  currentOffsetSec: number
  onSeekMoment: (offsetSeconds: number) => void
  isLoading?: boolean
  isError?: boolean
  className?: string
}

function toLiveHeatInput(detail: AnalyticsStreamDetail) {
  return {
    state: detail.state,
    rollups: detail.rollups ?? [],
    topEmotes: detail.topEmotes ?? [],
    streamStartedAt: detail.stream?.startedAt,
  }
}

function EmoteStack({ point }: { point: LiveHeatPoint }) {
  if (point.topEmotes.length === 0) return null
  return (
    <div className="flex flex-wrap items-center gap-1">
      {point.topEmotes.slice(0, LIVE_HEAT_MAX_EMOTES).map(emote => (
        <span
          key={emote.key}
          className="inline-flex items-center"
          title={`${emote.name}${emote.provider ? ` · ${emote.provider}` : ''} · ${emote.count.toLocaleString()}`}
        >
          {emote.imageUrl ? (
            <img
              src={emote.imageUrl}
              alt={emote.name}
              width={22}
              height={22}
              className="h-[22px] w-[22px] object-contain"
              loading="lazy"
            />
          ) : (
            <span className="text-[11px] font-semibold text-zinc-300">{emote.name}</span>
          )}
        </span>
      ))}
    </div>
  )
}

function MomentRow({
  point,
  active,
  onSeek,
}: {
  point: LiveHeatPoint
  active: boolean
  onSeek: () => void
}) {
  const offsetLabel = formatHeatOffset(point.offsetSeconds)
  return (
    <button
      type="button"
      onClick={onSeek}
      className={`flex w-full flex-col gap-1.5 rounded-lg border px-3 py-2.5 text-left transition focus:outline-none focus-visible:ring-2 focus-visible:ring-violet-400/60 ${
        active
          ? 'border-violet-400/50 bg-violet-500/15'
          : 'border-white/10 bg-white/[0.04] hover:border-violet-400/35 hover:bg-white/[0.06]'
      }`}
      aria-label={`Play moment at ${offsetLabel}, score ${point.score}, ${point.reasonLabel}`}
      aria-current={active ? 'true' : undefined}
    >
      <div className="flex items-center gap-2">
        <span
          className={`shrink-0 rounded-md px-2 py-0.5 text-xs font-black tabular-nums ${
            active
              ? 'border border-violet-400/40 bg-violet-500/25 text-violet-100'
              : 'border border-violet-400/30 bg-violet-500/15 text-violet-200'
          }`}
        >
          {point.score}
        </span>
        <span className="font-mono text-xs font-bold tabular-nums text-zinc-300">{offsetLabel}</span>
        <span className="min-w-0 truncate text-[11px] font-semibold text-zinc-400">{point.reasonLabel}</span>
      </div>
      <div className="flex items-end justify-between gap-2">
        <span className="text-[11px] font-semibold text-zinc-500">
          {point.chatCount.toLocaleString()} chat · {point.emoteCount.toLocaleString()} emotes
        </span>
        <EmoteStack point={point} />
      </div>
    </button>
  )
}

export function VodMomentsPanel({
  detail,
  currentOffsetSec,
  onSeekMoment,
  isLoading,
  isError,
  className,
}: VodMomentsPanelProps) {
  const heat = useMemo(
    () => (detail ? deriveLiveHeat(toLiveHeatInput(detail)) : null),
    [detail],
  )
  const points = heat?.points ?? []
  const offsets = useMemo(() => points.map(p => p.offsetSeconds), [points])
  const nearestIdx = useMemo(
    () => nearestMomentIndex(offsets, currentOffsetSec),
    [offsets, currentOffsetSec],
  )
  const [focusIdx, setFocusIdx] = useState<number | null>(null)
  const activeIdx = focusIdx ?? (nearestIdx >= 0 ? nearestIdx : null)

  const goPrev = () => {
    if (!points.length) return
    const idx = activeIdx ?? 0
    const next = Math.max(0, idx - 1)
    setFocusIdx(next)
    onSeekMoment(points[next].offsetSeconds)
  }

  const goNext = () => {
    if (!points.length) return
    const idx = activeIdx ?? 0
    const next = Math.min(points.length - 1, idx + 1)
    setFocusIdx(next)
    onSeekMoment(points[next].offsetSeconds)
  }

  if (isLoading && !detail) {
    return (
      <div className={className ?? 'flex flex-1 items-center justify-center px-4 py-6 text-xs font-semibold text-zinc-500'}>
        Loading moments…
      </div>
    )
  }

  if (isError) {
    return (
      <div className={className ?? 'flex flex-1 items-center justify-center px-4 py-6 text-xs font-semibold text-zinc-500'}>
        Moments unavailable. Re-sync this stream in Analytics.
      </div>
    )
  }

  if (!heat?.visible || !points.length) {
    return null
  }

  return (
    <section
      className={className ?? 'flex flex-col gap-2'}
      aria-label={LIVE_HEAT_TITLE}
    >
      <div className="flex shrink-0 items-center justify-between gap-2">
        <div className="min-w-0">
          <h4 className="text-xs font-black uppercase tracking-wide text-zinc-300">{LIVE_HEAT_TITLE}</h4>
          <p className="text-[10px] font-semibold text-zinc-600">{LIVE_HEAT_SUBTITLE}</p>
        </div>
        <div className="flex shrink-0 gap-1">
          <button
            type="button"
            onClick={goPrev}
            disabled={activeIdx !== null && activeIdx <= 0}
            className="rounded border border-white/10 bg-white/5 px-2.5 py-1 text-[10px] font-black uppercase text-zinc-300 transition hover:bg-white/10 disabled:opacity-40"
            aria-label="Previous moment"
          >
            Prev
          </button>
          <button
            type="button"
            onClick={goNext}
            disabled={activeIdx !== null && activeIdx >= points.length - 1}
            className="rounded border border-white/10 bg-white/5 px-2.5 py-1 text-[10px] font-black uppercase text-zinc-300 transition hover:bg-white/10 disabled:opacity-40"
            aria-label="Next moment"
          >
            Next
          </button>
        </div>
      </div>
      <div className="flex flex-col gap-2">
        {points.map((point, index) => (
          <MomentRow
            key={point.minuteTs}
            point={point}
            active={activeIdx === index}
            onSeek={() => {
              setFocusIdx(index)
              onSeekMoment(point.offsetSeconds)
            }}
          />
        ))}
      </div>
    </section>
  )
}

export default VodMomentsPanel
