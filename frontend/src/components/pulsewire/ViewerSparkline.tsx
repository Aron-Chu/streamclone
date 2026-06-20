import type { PulseWireViewerPoint } from '../../pulseWireApi'

type Props = {
  points?: PulseWireViewerPoint[] | null
  width?: number
  height?: number
  className?: string
  stroke?: string
  fill?: string
  ariaLabel?: string
}

// Static SVG line chart. No animation, so it is inherently safe for
// prefers-reduced-motion users. Pass className="w-full" with a fixed height for
// a responsive sparkline (viewBox + non-uniform preserveAspectRatio stretch it).
export default function ViewerSparkline({
  points,
  width = 120,
  height = 32,
  className = '',
  stroke = '#A970FF',
  fill = 'rgba(169,112,255,0.16)',
  ariaLabel,
}: Props) {
  const values = (points ?? [])
    .map(point => point.viewers)
    .filter((value): value is number => typeof value === 'number' && Number.isFinite(value))

  if (values.length < 2) {
    return <span className={`text-[11px] text-[#7A7A85] ${className}`} aria-label="Not enough samples yet">—</span>
  }

  const max = Math.max(...values)
  const min = Math.min(...values)
  const span = max - min || 1
  const stepX = width / (values.length - 1)
  const padY = 1.5

  const coords = values.map((value, index) => {
    const x = index * stepX
    const y = height - padY - ((value - min) / span) * (height - padY * 2)
    return [x, y] as const
  })

  const line = coords
    .map(([x, y], index) => `${index === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`)
    .join(' ')
  const area = `${line} L${width.toFixed(1)},${height.toFixed(1)} L0,${height.toFixed(1)} Z`
  const [lastX, lastY] = coords[coords.length - 1]
  const trendingUp = values[values.length - 1] >= values[0]

  return (
    <svg
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      className={className}
      role="img"
      aria-label={ariaLabel ?? `Viewer trend across ${values.length} samples, ${trendingUp ? 'up' : 'down'}`}
      preserveAspectRatio="none"
    >
      <path d={area} fill={fill} stroke="none" />
      <path d={line} fill="none" stroke={stroke} strokeWidth={1.5} strokeLinejoin="round" strokeLinecap="round" />
      <circle cx={lastX} cy={lastY} r={2} fill={stroke} />
    </svg>
  )
}
