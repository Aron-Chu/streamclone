import { useLayoutEffect, useRef } from 'react'
import {
  BANDWIDTH_SERIES_META,
  type BandwidthSeriesKey,
} from '../../utils/networkActivityModel'

export interface NetworkBandwidthOverviewProps {
  categorySeries: Record<BandwidthSeriesKey, number[]>
  latestRates: Record<BandwidthSeriesKey, number>
  selectedSeries?: BandwidthSeriesKey | null
  onSelectSeries?: (series: BandwidthSeriesKey | null) => void
  height?: number
  loading?: boolean
}

const SERIES_ORDER: BandwidthSeriesKey[] = ['hls', 'analytics', 'chat', 'core', 'browser']

function formatLegendRate(bytesPerSec: number) {
  if (!Number.isFinite(bytesPerSec) || bytesPerSec <= 0) return '0 B/s'
  if (bytesPerSec >= 1_000_000) return `${(bytesPerSec / 1_000_000).toFixed(1)} MB/s`
  if (bytesPerSec >= 1_000) return `${(bytesPerSec / 1_000).toFixed(0)} KB/s`
  return `${bytesPerSec.toFixed(0)} B/s`
}

export default function NetworkBandwidthOverview({
  categorySeries,
  latestRates,
  selectedSeries = null,
  onSelectSeries,
  height = 160,
  loading = false,
}: NetworkBandwidthOverviewProps) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null)

  useLayoutEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const parent = canvas.parentElement
    const cssWidth = Math.max(1, parent?.clientWidth ?? canvas.clientWidth ?? 640)
    const cssHeight = Math.max(1, height)
    const dpr = typeof window !== 'undefined' ? window.devicePixelRatio || 1 : 1

    canvas.width = Math.floor(cssWidth * dpr)
    canvas.height = Math.floor(cssHeight * dpr)
    canvas.style.height = `${cssHeight}px`

    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.clearRect(0, 0, cssWidth, cssHeight)

    const pointCount = Math.max(...SERIES_ORDER.map(key => categorySeries[key]?.length ?? 0), 0)
    if (pointCount < 2) {
      ctx.strokeStyle = 'rgba(255,255,255,0.08)'
      ctx.beginPath()
      ctx.moveTo(0, cssHeight / 2)
      ctx.lineTo(cssWidth, cssHeight / 2)
      ctx.stroke()
      return
    }

    const stackedTotals: number[] = []
    for (let i = 0; i < pointCount; i += 1) {
      let total = 0
      for (const key of SERIES_ORDER) {
        if (selectedSeries && selectedSeries !== key) continue
        total += categorySeries[key]?.[i] ?? 0
      }
      stackedTotals.push(total)
    }

    const max = Math.max(...stackedTotals, 0.001)
    const step = cssWidth / Math.max(pointCount - 1, 1)

    const cumulative: number[][] = SERIES_ORDER.map(() => [])
    for (let i = 0; i < pointCount; i += 1) {
      let running = 0
      SERIES_ORDER.forEach((key, seriesIndex) => {
        if (selectedSeries && selectedSeries !== key) {
          cumulative[seriesIndex]!.push(running)
          return
        }
        running += categorySeries[key]?.[i] ?? 0
        cumulative[seriesIndex]!.push(running)
      })
    }

    for (let seriesIndex = SERIES_ORDER.length - 1; seriesIndex >= 0; seriesIndex -= 1) {
      const key = SERIES_ORDER[seriesIndex]!
      if (selectedSeries && selectedSeries !== key) continue
      const meta = BANDWIDTH_SERIES_META[key]
      const top = cumulative[seriesIndex]!
      const bottom = seriesIndex === 0
        ? new Array(pointCount).fill(0)
        : cumulative[seriesIndex - 1]!

      ctx.beginPath()
      for (let i = 0; i < pointCount; i += 1) {
        const x = i * step
        const yTop = cssHeight - (top[i]! / max) * (cssHeight - 8) - 4
        if (i === 0) ctx.moveTo(x, yTop)
        else ctx.lineTo(x, yTop)
      }
      for (let i = pointCount - 1; i >= 0; i -= 1) {
        const x = i * step
        const yBottom = cssHeight - (bottom[i]! / max) * (cssHeight - 8) - 4
        ctx.lineTo(x, yBottom)
      }
      ctx.closePath()
      ctx.fillStyle = meta.fill
      ctx.fill()
      ctx.strokeStyle = meta.color
      ctx.lineWidth = 1
      ctx.stroke()
    }
  }, [categorySeries, height, selectedSeries])

  return (
    <section className="rounded-xl border border-white/10 bg-[#0e0e10] p-4">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div>
          <div className="text-[11px] font-black uppercase tracking-wide text-zinc-500">Bandwidth overview</div>
          <p className="mt-1 text-xs font-semibold text-zinc-500">Stacked activity by category · ~2 min history</p>
        </div>
        {loading ? (
          <span className="text-[10px] font-black uppercase text-zinc-600">Sampling…</span>
        ) : null}
      </div>

      <canvas ref={canvasRef} className="block w-full" aria-hidden="true" />

      <div className="mt-3 flex flex-wrap gap-2">
        <button
          type="button"
          onClick={() => onSelectSeries?.(null)}
          className={`rounded border px-2 py-1 text-[10px] font-black uppercase transition ${
            selectedSeries == null
              ? 'border-white/20 bg-white/10 text-white'
              : 'border-white/10 text-zinc-500 hover:bg-white/5'
          }`}
        >
          All
        </button>
        {SERIES_ORDER.map(key => {
          const meta = BANDWIDTH_SERIES_META[key]
          const active = selectedSeries === key
          return (
            <button
              key={key}
              type="button"
              onClick={() => onSelectSeries?.(active ? null : key)}
              className={`rounded border px-2 py-1 text-[10px] font-black uppercase transition ${
                active ? 'border-white/20 bg-white/10 text-white' : 'border-white/10 text-zinc-400 hover:bg-white/5'
              }`}
              style={{ borderColor: active ? meta.color : undefined }}
            >
              <span className="mr-1.5 inline-block h-2 w-2 rounded-full" style={{ backgroundColor: meta.color }} />
              {meta.label}
              <span className="ml-1.5 font-mono text-[9px] text-zinc-500">
                {formatLegendRate(latestRates[key] ?? 0)}
              </span>
            </button>
          )
        })}
      </div>
    </section>
  )
}
