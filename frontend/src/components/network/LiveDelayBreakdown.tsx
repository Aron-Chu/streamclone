import type { LiveDelayBreakdown } from '../../playbackMath'
import { formatLiveDelayTooltip } from '../../playbackMath'

function fmtSec(value: number | null | undefined) {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  return `${value.toFixed(1)}s`
}

export interface LiveDelayBreakdownProps {
  breakdown: LiveDelayBreakdown
  latencyMode?: string
  title?: string
  compact?: boolean
}

export default function LiveDelayBreakdownPanel({
  breakdown,
  latencyMode,
  title = 'Live delay breakdown',
  compact = false,
}: LiveDelayBreakdownProps) {
  const relay = breakdown.relayDelaySec ?? 0
  const buffer = breakdown.bufferDelaySec ?? 0
  const sync = breakdown.syncDriftSec ?? 0
  const total = Math.max(relay + buffer + sync, breakdown.displayDelaySec ?? 0, 0.01)
  const segments = [
    { key: 'relay', label: 'Relay segments', value: relay, className: 'bg-violet-500/80' },
    { key: 'buffer', label: 'HLS target buffer', value: buffer, className: 'bg-cyan-500/70' },
    { key: 'sync', label: 'Behind sync', value: sync, className: 'bg-amber-400/70' },
  ]

  return (
    <section className={`rounded-xl border border-white/10 bg-[#0e0e10] ${compact ? 'p-3' : 'p-4'}`}>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div>
          <div className="text-[11px] font-black uppercase tracking-wide text-zinc-500">{title}</div>
          <div className="mt-1 text-xl font-black text-white">{fmtSec(breakdown.displayDelaySec)} end-to-end</div>
        </div>
        {latencyMode ? (
          <span className="rounded border border-violet-400/30 bg-violet-500/10 px-2 py-1 text-[10px] font-black uppercase text-violet-100">
            {latencyMode}
          </span>
        ) : null}
      </div>
      <div className="mb-2 flex h-3 overflow-hidden rounded-full bg-white/10">
        {segments.map(segment => (
          segment.value > 0 ? (
            <div
              key={segment.key}
              className={`${segment.className} transition-all`}
              style={{ width: `${Math.max(4, (segment.value / total) * 100)}%` }}
              title={`${segment.label}: ${fmtSec(segment.value)}`}
            />
          ) : null
        ))}
      </div>
      <div className="grid gap-2 sm:grid-cols-3">
        {segments.map(segment => (
          <div key={segment.key} className="rounded border border-white/10 bg-white/[0.03] px-3 py-2">
            <div className="flex items-center gap-2">
              <span className={`h-2 w-2 rounded-full ${segment.className}`} />
              <span className="text-[10px] font-black uppercase text-zinc-500">{segment.label}</span>
            </div>
            <div className="mt-1 text-sm font-black text-white">{fmtSec(segment.value)}</div>
          </div>
        ))}
      </div>
      <p className="mt-3 text-xs font-semibold text-zinc-500">{formatLiveDelayTooltip(breakdown)}</p>
    </section>
  )
}
