import { useEffect, useLayoutEffect, useRef, useState } from 'react'

import type { AnalyticsStreamDetail } from '../../api.ts'
import { useAnalyticsLive } from '../../hooks/useAnalyticsLive.ts'
import {
  LIVE_REFRESH_MS,
  MAX_TOP_EMOTES,
  SPARKLINE_MAX_POINTS,
  deriveLiveStats,
  trendArrowGlyph,
  type LiveStats,
  type LiveStatsInput,
  type TrendDirection,
} from '../../utils/liveStats.ts'

/**
 * LiveStatsBand renders the compact real-time activity band on a live stream's
 * analytics page (Requirement 18). It refreshes every 15 seconds, animates
 * number changes with a short transform (no layout shift), draws a 60-point
 * chat-rate sparkline on a canvas, respects prefers-reduced-motion, and on
 * error/timeout retains the last successfully received values behind a
 * stale-data indicator while retrying on the next cycle.
 *
 * All derivation lives in ../../utils/liveStats.ts so it can be unit-tested
 * without rendering (task 15.2).
 */

export interface LiveStatsBandProps {
  login: string
  /** When provided, skip fetching and render from this live detail payload. */
  detail?: AnalyticsStreamDetail
  detailLoading?: boolean
  /** Disable polling when the band is off-screen or the stream is not live. */
  enabled?: boolean
  className?: string
}

/** Tracks the user's prefers-reduced-motion preference (Req 18.4). */
function useReducedMotion(): boolean {
  const [reduced, setReduced] = useState<boolean>(() =>
    typeof window !== 'undefined' && typeof window.matchMedia === 'function'
      ? window.matchMedia('(prefers-reduced-motion: reduce)').matches
      : false,
  )

  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    const onChange = () => setReduced(mq.matches)
    onChange()
    if (typeof mq.addEventListener === 'function') {
      mq.addEventListener('change', onChange)
      return () => mq.removeEventListener('change', onChange)
    }
    // Safari < 14 fallback.
    mq.addListener(onChange)
    return () => mq.removeListener(onChange)
  }, [])

  return reduced
}

function toLiveStatsInput(detail: AnalyticsStreamDetail): LiveStatsInput {
  return {
    state: detail.state,
    rollups: detail.rollups ?? [],
    topEmotes: detail.topEmotes ?? [],
    avgViewers: detail.stream?.avgViewers ?? 0,
    peakViewers: detail.stream?.peakViewers ?? 0,
  }
}

/**
 * Animates a numeric value with a brief transform on change (Req 18.2). Uses
 * tabular-nums + inline-block + transform-only transition so the surrounding
 * layout never shifts. Disabled when reduced motion is requested (Req 18.4).
 */
function AnimatedNumber({
  value,
  format,
  reducedMotion,
  className,
}: {
  value: number
  format?: (v: number) => string
  reducedMotion: boolean
  className?: string
}) {
  const [pulse, setPulse] = useState(false)
  const prev = useRef(value)

  useEffect(() => {
    if (value === prev.current) return
    prev.current = value
    if (reducedMotion) return
    setPulse(true)
    const t = setTimeout(() => setPulse(false), 280)
    return () => clearTimeout(t)
  }, [value, reducedMotion])

  return (
    <span
      className={`inline-block tabular-nums will-change-transform ${className ?? ''}`}
      style={{
        transitionProperty: 'transform, color',
        transitionDuration: reducedMotion ? '0ms' : '250ms',
        transform: pulse ? 'scale(1.12)' : 'scale(1)',
      }}
    >
      {format ? format(value) : value}
    </span>
  )
}

function TrendArrow({ trend }: { trend: TrendDirection }) {
  const color =
    trend === 'up' ? 'text-emerald-400' : trend === 'down' ? 'text-rose-400' : 'text-zinc-500'
  const label = trend === 'up' ? 'trending up' : trend === 'down' ? 'trending down' : 'stable'
  return (
    <span className={`text-xs font-black ${color}`} aria-label={label} title={label}>
      {trendArrowGlyph(trend)}
    </span>
  )
}

function formatSignedDelta(delta: number | null): string {
  if (delta === null) return '—'
  if (delta === 0) return '±0'
  return delta > 0 ? `+${delta.toLocaleString()}` : `−${Math.abs(delta).toLocaleString()}`
}

const CONFIDENCE_STYLES: Record<LiveStats['confidence'], string> = {
  Synced: 'bg-emerald-500/15 text-emerald-300 border-emerald-400/30',
  Collecting: 'bg-violet-500/15 text-violet-300 border-violet-400/30',
  'Waiting for first minute': 'bg-amber-500/15 text-amber-300 border-amber-400/30',
  'Stats only': 'bg-zinc-500/15 text-zinc-300 border-zinc-400/30',
}

/** 60-point chat-rate sparkline drawn on a canvas (Req 18.3). */
function Sparkline({
  series,
  reducedMotion,
}: {
  series: number[]
  reducedMotion: boolean
}) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null)

  useLayoutEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const parent = canvas.parentElement
    const cssWidth = Math.max(1, parent?.clientWidth ?? canvas.clientWidth ?? 160)
    const cssHeight = Math.max(1, canvas.clientHeight || 36)
    const dpr = typeof window !== 'undefined' ? window.devicePixelRatio || 1 : 1

    canvas.width = Math.floor(cssWidth * dpr)
    canvas.height = Math.floor(cssHeight * dpr)

    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.clearRect(0, 0, cssWidth, cssHeight)

    const points = series.slice(-SPARKLINE_MAX_POINTS)
    if (points.length < 2) return

    const max = Math.max(1, ...points)
    const stepX = cssWidth / (SPARKLINE_MAX_POINTS - 1)
    const pad = 2
    const usableH = cssHeight - pad * 2

    const offset = SPARKLINE_MAX_POINTS - points.length
    const coords = points.map((v, i) => {
      const x = (offset + i) * stepX
      const y = pad + usableH - (v / max) * usableH
      return [x, y] as const
    })

    // Area fill.
    ctx.beginPath()
    ctx.moveTo(coords[0][0], cssHeight)
    for (const [x, y] of coords) ctx.lineTo(x, y)
    ctx.lineTo(coords[coords.length - 1][0], cssHeight)
    ctx.closePath()
    ctx.fillStyle = 'rgba(139, 92, 246, 0.18)'
    ctx.fill()

    // Line.
    ctx.beginPath()
    coords.forEach(([x, y], i) => (i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y)))
    ctx.lineWidth = 1.5
    ctx.strokeStyle = 'rgba(167, 139, 250, 0.9)'
    ctx.lineJoin = 'round'
    ctx.stroke()
  }, [series])

  return (
    <canvas
      ref={canvasRef}
      className="h-9 w-full"
      style={{ transition: reducedMotion ? 'none' : 'opacity 250ms ease' }}
      role="img"
      aria-label="Chat activity over the last 60 minutes"
    />
  )
}

export function LiveStatsBand({ login, detail, detailLoading, enabled = true, className }: LiveStatsBandProps) {
  const reducedMotion = useReducedMotion()
  const selfFetch = detail === undefined

  const query = useAnalyticsLive(login, {
    enabled: Boolean(login) && enabled && selfFetch,
    refetchInterval: LIVE_REFRESH_MS,
    withTimeout: selfFetch,
  })

  const resolvedDetail = detail ?? query.data
  const stale = selfFetch && Boolean(query.error) && Boolean(query.data)

  if (!resolvedDetail) {
    if (selfFetch && query.isError) {
      return (
        <div
          className={
            className ??
            'rounded-xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-xs text-zinc-400'
          }
        >
          Live stats are temporarily unavailable. Retrying…
        </div>
      )
    }
    return (
      <div
        className={
          className ??
          'rounded-xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-xs text-zinc-500'
        }
      >
        {detailLoading || (selfFetch && query.isLoading) ? 'Loading live stats…' : 'Live stats unavailable'}
      </div>
    )
  }

  const stats = deriveLiveStats(toLiveStatsInput(resolvedDetail))

  return (
    <div
      className={
        className ??
        'flex flex-col gap-3 rounded-xl border border-white/10 bg-zinc-950/60 p-4'
      }
      aria-live="polite"
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <span className="text-xs font-black uppercase tracking-wide text-zinc-400">
            Live now
          </span>
          <span
            className={`rounded-full border px-2 py-0.5 text-[10px] font-black ${CONFIDENCE_STYLES[stats.confidence]}`}
          >
            {stats.confidence}
          </span>
        </div>
        {stale && (
          <span
            className="rounded-full border border-amber-400/30 bg-amber-500/10 px-2 py-0.5 text-[10px] font-black text-amber-300"
            title="Showing last received values; retrying on the next refresh."
          >
            Stale
          </span>
        )}
      </div>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        <div className="flex flex-col">
          <span className="text-[10px] font-black uppercase tracking-wide text-zinc-500">
            Viewers
          </span>
          <span className="text-lg font-black text-white">
            <AnimatedNumber
              value={stats.currentViewers}
              format={v => v.toLocaleString()}
              reducedMotion={reducedMotion}
            />
          </span>
          <span
            className={`text-[11px] font-semibold ${
              stats.viewerDelta5m === null
                ? 'text-zinc-500'
                : stats.viewerDelta5m > 0
                  ? 'text-emerald-400'
                  : stats.viewerDelta5m < 0
                    ? 'text-rose-400'
                    : 'text-zinc-500'
            }`}
          >
            {formatSignedDelta(stats.viewerDelta5m)} · 5m
          </span>
        </div>

        <div className="flex flex-col">
          <span className="text-[10px] font-black uppercase tracking-wide text-zinc-500">
            Chat / min
          </span>
          <span className="flex items-center gap-1 text-lg font-black text-white">
            <AnimatedNumber
              value={stats.chatPerMin1m}
              format={v => v.toLocaleString()}
              reducedMotion={reducedMotion}
            />
            <TrendArrow trend={stats.chatTrend} />
          </span>
          <span className="text-[11px] font-semibold text-zinc-500">
            {stats.chatPerMin5m.toLocaleString()} avg · 5m
          </span>
        </div>

        <div className="flex flex-col">
          <span className="text-[10px] font-black uppercase tracking-wide text-zinc-500">
            Emotes / min
          </span>
          <span className="text-lg font-black text-white">
            <AnimatedNumber
              value={stats.totalEmotePerMin}
              format={v => v.toLocaleString()}
              reducedMotion={reducedMotion}
            />
          </span>
          {stats.hasProviderSplit ? (
            <span className="flex flex-wrap gap-x-2 text-[11px] font-semibold text-zinc-500">
              {stats.emoteProviderRates.map(rate => (
                <span key={rate.provider}>
                  {rate.provider} {rate.perMinute.toLocaleString()}
                </span>
              ))}
            </span>
          ) : (
            <span className="text-[11px] font-semibold text-zinc-600">No provider data</span>
          )}
        </div>
      </div>

      {stats.topEmotes.length > 0 && (
        <div className="flex items-center gap-2">
          <span className="text-[10px] font-black uppercase tracking-wide text-zinc-500">
            Top
          </span>
          <div className="flex items-center gap-2">
            {stats.topEmotes.slice(0, MAX_TOP_EMOTES).map(emote => (
              <span
                key={emote.id ?? emote.key ?? emote.name}
                className="flex items-center gap-1 rounded-md bg-white/5 px-1.5 py-1"
                title={`${emote.name}${emote.provider ? ` · ${emote.provider}` : ''} · ${emote.count.toLocaleString()}`}
              >
                {emote.imageUrl ? (
                  <img
                    src={emote.imageUrl}
                    alt={emote.name}
                    width={24}
                    height={24}
                    className="h-6 w-6 object-contain"
                    loading="lazy"
                  />
                ) : (
                  <span className="text-xs font-semibold text-zinc-300">{emote.name}</span>
                )}
                {emote.provider && (
                  <span className="text-[9px] font-black uppercase text-zinc-500">
                    {emote.provider}
                  </span>
                )}
              </span>
            ))}
          </div>
        </div>
      )}

      <div className="min-h-[36px]">
        <span className="text-[10px] font-black uppercase tracking-wide text-zinc-500">
          Chat activity (last 60 min)
        </span>
        <Sparkline series={stats.sparkline} reducedMotion={reducedMotion} />
      </div>
    </div>
  )
}

export default LiveStatsBand
