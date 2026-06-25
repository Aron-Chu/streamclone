import { useLayoutEffect, useRef } from 'react'
import {
  BANDWIDTH_SERIES_META,
  type BandwidthSeriesKey,
} from '../../utils/networkActivityModel'

export interface NetworkStackedAreaChartProps {
  categorySeries: Record<BandwidthSeriesKey, number[]>
  seriesOrder: BandwidthSeriesKey[]
  selectedSeries?: BandwidthSeriesKey | null
  height?: number
  testId?: string
}

export default function NetworkStackedAreaChart({
  categorySeries,
  seriesOrder,
  selectedSeries = null,
  height = 140,
  testId,
}: NetworkStackedAreaChartProps) {
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

    const pointCount = Math.max(...seriesOrder.map(key => categorySeries[key]?.length ?? 0), 0)
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
      for (const key of seriesOrder) {
        if (selectedSeries && selectedSeries !== key) continue
        total += categorySeries[key]?.[i] ?? 0
      }
      stackedTotals.push(total)
    }

    const max = Math.max(...stackedTotals, 0.001)
    const step = cssWidth / Math.max(pointCount - 1, 1)

    const cumulative: number[][] = seriesOrder.map(() => [])
    for (let i = 0; i < pointCount; i += 1) {
      let running = 0
      seriesOrder.forEach((key, seriesIndex) => {
        if (selectedSeries && selectedSeries !== key) {
          cumulative[seriesIndex]!.push(running)
          return
        }
        running += categorySeries[key]?.[i] ?? 0
        cumulative[seriesIndex]!.push(running)
      })
    }

    for (let seriesIndex = seriesOrder.length - 1; seriesIndex >= 0; seriesIndex -= 1) {
      const key = seriesOrder[seriesIndex]!
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
  }, [categorySeries, height, selectedSeries, seriesOrder])

  return (
    <div data-testid={testId}>
      <canvas ref={canvasRef} className="block w-full" aria-hidden="true" />
    </div>
  )
}
