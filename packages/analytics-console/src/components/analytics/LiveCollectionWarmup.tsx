import { useEffect, useState } from 'react'

import {
  LIVE_TRACKING_BADGE,
  LIVE_TRACKING_NOT_SYNC_NOTE,
  liveChartWarmupProgress,
  liveWarmupHintLine,
  liveWarmupStatusLine,
} from '../../utils/liveCollectionWarmup.ts'
import { COLLECTING_FIRST_MINUTES_MESSAGE } from '../../utils/liveEmptyState.ts'

const BAR_HEIGHTS = [0.35, 0.55, 0.75, 0.95, 0.7, 0.5, 0.85, 0.6, 0.4, 0.8]

export interface LiveCollectionWarmupProps {
  rollupMinuteCount: number
  viewerSamples?: number
  chatMessages?: number
  autoRefreshSec?: number
}

function useReducedMotion(): boolean {
  const [reduced, setReduced] = useState(() =>
    typeof window !== 'undefined' && typeof window.matchMedia === 'function'
      ? window.matchMedia('(prefers-reduced-motion: reduce)').matches
      : false,
  )
  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    const onChange = () => setReduced(mq.matches)
    onChange()
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])
  return reduced
}

function ActivityBars({ reducedMotion }: { reducedMotion: boolean }) {
  return (
    <div
      className="flex h-14 items-end justify-center gap-1.5"
      aria-hidden
    >
      {BAR_HEIGHTS.map((height, index) => (
        <span
          key={index}
          className={`w-2 rounded-sm bg-gradient-to-t from-violet-600/40 to-emerald-400/70 ${
            reducedMotion ? '' : 'animate-pulse'
          }`}
          style={{
            height: `${Math.round(height * 100)}%`,
            animationDelay: reducedMotion ? undefined : `${index * 120}ms`,
            animationDuration: reducedMotion ? undefined : '1.4s',
          }}
        />
      ))}
    </div>
  )
}

export default function LiveCollectionWarmup({
  rollupMinuteCount,
  viewerSamples = 0,
  chatMessages = 0,
  autoRefreshSec = 15,
}: LiveCollectionWarmupProps) {
  const reducedMotion = useReducedMotion()
  const progress = liveChartWarmupProgress(rollupMinuteCount)
  const hint = liveWarmupHintLine({ viewerSamples, chatMessages })

  return (
    <div className="mx-auto w-full max-w-lg text-center">
      <div className="mb-4 rounded-xl border border-violet-400/20 bg-violet-500/5 px-4 py-5">
        <ActivityBars reducedMotion={reducedMotion} />
      </div>

      <div className="flex flex-wrap items-center justify-center gap-2">
        <span className="inline-flex items-center gap-1.5 rounded-full border border-red-400/30 bg-red-500/10 px-2.5 py-1 text-[10px] font-black uppercase tracking-wide text-red-200">
          <span className={`h-2 w-2 rounded-full bg-red-400 ${reducedMotion ? '' : 'animate-pulse'}`} />
          {LIVE_TRACKING_BADGE}
        </span>
        <span className="rounded-full border border-white/10 bg-white/[0.04] px-2.5 py-1 text-[10px] font-semibold text-zinc-400">
          Refreshes every {autoRefreshSec}s
        </span>
      </div>

      <div className="mt-3 flex items-center justify-center gap-2">
        {!reducedMotion ? (
          <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-violet-500/30 border-t-violet-400" />
        ) : null}
        <h3 className="text-base font-black text-zinc-100">{COLLECTING_FIRST_MINUTES_MESSAGE}</h3>
      </div>

      <p className="mt-2 text-sm font-semibold text-zinc-400">
        {liveWarmupStatusLine(progress)}
      </p>

      <div className="mt-4">
        <div className="mb-1.5 flex justify-between text-[10px] font-bold uppercase tracking-wide text-zinc-500">
          <span>Minute buckets</span>
          <span>
            {progress.readyMinutes} / {progress.targetMinutes}
          </span>
        </div>
        <div className="h-2 overflow-hidden rounded-full bg-white/5">
          <div
            className={`h-full rounded-full bg-gradient-to-r from-violet-500 to-emerald-400 transition-[width] duration-700 ease-out ${
              reducedMotion ? '' : 'animate-pulse'
            }`}
            style={{ width: `${Math.max(progress.percent, progress.readyMinutes > 0 ? 8 : 4)}%` }}
          />
        </div>
        <div className="mt-2 flex justify-center gap-2">
          {Array.from({ length: progress.targetMinutes }, (_, i) => (
            <span
              key={i}
              className={`h-2.5 w-2.5 rounded-full border ${
                i < progress.readyMinutes
                  ? 'border-emerald-400/50 bg-emerald-400/80'
                  : 'border-white/10 bg-white/[0.04]'
              }`}
              title={i < progress.readyMinutes ? 'Minute bucket ready' : 'Waiting for minute boundary'}
            />
          ))}
        </div>
      </div>

      <ul className="mt-4 space-y-1 text-left text-[11px] font-semibold text-zinc-500">
        <li className="flex items-start gap-2">
          <span className="mt-0.5 text-violet-300">•</span>
          <span>Polls Twitch for live viewer counts (~every 15s).</span>
        </li>
        <li className="flex items-start gap-2">
          <span className="mt-0.5 text-emerald-300">•</span>
          <span>Reads chat over IRC and counts emotes (7TV + Twitch).</span>
        </li>
        <li className="flex items-start gap-2">
          <span className="mt-0.5 text-zinc-400">•</span>
          <span>{LIVE_TRACKING_NOT_SYNC_NOTE}</span>
        </li>
      </ul>

      {hint ? (
        <p className="mt-3 text-xs font-bold text-emerald-300/90">{hint}</p>
      ) : null}
    </div>
  )
}
