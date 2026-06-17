import { useLayoutEffect, useRef } from 'react'

export interface NetworkSparklineProps {
  series: number[]
  color?: string
  fill?: string
  height?: number
  className?: string
}

export default function NetworkSparkline({
  series,
  color = '#a78bfa',
  fill = 'rgba(167,139,250,0.15)',
  height = 48,
  className = '',
}: NetworkSparklineProps) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null)

  useLayoutEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const parent = canvas.parentElement
    const cssWidth = Math.max(1, parent?.clientWidth ?? canvas.clientWidth ?? 160)
    const cssHeight = Math.max(1, height)
    const dpr = typeof window !== 'undefined' ? window.devicePixelRatio || 1 : 1

    canvas.width = Math.floor(cssWidth * dpr)
    canvas.height = Math.floor(cssHeight * dpr)
    canvas.style.height = `${cssHeight}px`

    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.clearRect(0, 0, cssWidth, cssHeight)

    if (series.length < 2) {
      ctx.strokeStyle = 'rgba(255,255,255,0.08)'
      ctx.beginPath()
      ctx.moveTo(0, cssHeight / 2)
      ctx.lineTo(cssWidth, cssHeight / 2)
      ctx.stroke()
      return
    }

    const max = Math.max(...series, 0.001)
    const min = Math.min(...series, 0)
    const span = Math.max(max - min, 0.001)
    const step = cssWidth / (series.length - 1)

    const points = series.map((value, index) => ({
      x: index * step,
      y: cssHeight - ((value - min) / span) * (cssHeight - 4) - 2,
    }))

    ctx.beginPath()
    ctx.moveTo(points[0].x, points[0].y)
    for (let i = 1; i < points.length; i += 1) ctx.lineTo(points[i].x, points[i].y)
    const lastPoint = points[points.length - 1]
    ctx.lineTo(lastPoint.x, cssHeight)
    ctx.lineTo(points[0].x, cssHeight)
    ctx.closePath()
    ctx.fillStyle = fill
    ctx.fill()

    ctx.beginPath()
    ctx.moveTo(points[0].x, points[0].y)
    for (let i = 1; i < points.length; i += 1) ctx.lineTo(points[i].x, points[i].y)
    ctx.strokeStyle = color
    ctx.lineWidth = 1.5
    ctx.stroke()
  }, [series, color, fill, height])

  return (
    <canvas
      ref={canvasRef}
      className={`block w-full ${className}`}
      aria-hidden="true"
    />
  )
}
