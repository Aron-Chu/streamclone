import type { NetworkActivityNode } from '../../utils/networkActivityModel'
import { formatBytes, formatRate } from '../../utils/networkActivityModel'
import NetworkSparkline from './NetworkSparkline'

const PHASE_STYLES: Record<string, string> = {
  scraping_tracker: 'border-amber-300/30 bg-amber-500/10 text-amber-50',
  fetching_comments: 'border-sky-400/30 bg-sky-500/10 text-sky-100',
  preloading_emotes: 'border-violet-400/30 bg-violet-500/10 text-violet-100',
}

function phaseBadgeClass(phase?: string) {
  if (!phase) return 'border-white/10 bg-white/[0.03] text-zinc-400'
  for (const [key, style] of Object.entries(PHASE_STYLES)) {
    if (phase.includes(key)) return style
  }
  return 'border-white/10 bg-white/[0.03] text-zinc-400'
}

export interface AnalyticsSyncBreakdownRowProps {
  parent: NetworkActivityNode
  children: NetworkActivityNode[]
  expanded: boolean
  onToggle: () => void
  sparklineColor?: string
}

export default function AnalyticsSyncBreakdownRow({
  parent,
  children,
  expanded,
  onToggle,
  sparklineColor = '#f472b6',
}: AnalyticsSyncBreakdownRowProps) {
  const childBytes = children.reduce((sum, child) => sum + (child.bytesTotal ?? 0), 0)
  const measuredShare = parent.bytesTotal && childBytes
    ? Math.min(100, Math.round((childBytes / parent.bytesTotal) * 100))
    : null

  return (
    <div className="space-y-1">
      <div className="flex h-2 overflow-hidden rounded-full bg-white/10">
        {children.map(child => {
          const width = parent.bytesTotal && child.bytesTotal
            ? Math.max(4, (child.bytesTotal / parent.bytesTotal) * 100)
            : 100 / Math.max(children.length, 1)
          return (
            <div
              key={child.id}
              className="bg-fuchsia-500/70 transition-all first:rounded-l-full last:rounded-r-full"
              style={{ width: `${width}%` }}
              title={`${child.name}: ${formatBytes(child.bytesTotal)}`}
            />
          )
        })}
      </div>

      <div className="flex flex-wrap items-center justify-between gap-2 text-[10px] font-semibold text-zinc-500">
        <button
          type="button"
          onClick={onToggle}
          className="font-black uppercase text-zinc-400 transition hover:text-zinc-200"
        >
          {expanded ? '▾' : '▸'} {children.length} measured ops
        </button>
        {measuredShare != null ? (
          <span>{measuredShare}% attributed to sub-steps</span>
        ) : null}
        {parent.phase ? (
          <span className={`rounded border px-1.5 py-0.5 text-[10px] font-black uppercase ${phaseBadgeClass(parent.phase)}`}>
            {parent.phase}
          </span>
        ) : null}
      </div>

      {expanded ? (
        <div className="space-y-1 border-l border-white/10 pl-3">
          {children.map(child => (
            <div key={child.id} className="flex flex-wrap items-center justify-between gap-2 rounded border border-white/5 bg-white/[0.02] px-2 py-1.5">
              <div className="min-w-0">
                <div className="text-xs font-black text-zinc-200">{child.name}</div>
                {child.phase ? (
                  <span className={`mt-0.5 inline-block rounded border px-1 py-0.5 text-[9px] font-black uppercase ${phaseBadgeClass(child.phase)}`}>
                    {child.phase}
                  </span>
                ) : null}
              </div>
              <div className="flex items-center gap-3">
                <div className="w-24">
                  <NetworkSparkline
                    series={child.sparkline ?? []}
                    color={sparklineColor}
                    fill="rgba(244,114,182,0.12)"
                    height={28}
                  />
                </div>
                <div className="text-right font-mono text-[11px] text-zinc-300">
                  <div>{formatBytes(child.bytesTotal)}</div>
                  <div className="text-zinc-500">{formatRate(child.bytesPerSec)}</div>
                </div>
              </div>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  )
}
