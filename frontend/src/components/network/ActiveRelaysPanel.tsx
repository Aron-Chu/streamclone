import { Link } from 'react-router-dom'
import type { OpsActiveStream } from '../../api'

function fmtMs(value: number | null | undefined) {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  if (value >= 1000) return `${(value / 1000).toFixed(1)}s`
  return `${Math.round(value)}ms`
}

export interface ActiveRelaysPanelProps {
  streams: OpsActiveStream[]
  highlightChannel?: string
  loading?: boolean
}

export default function ActiveRelaysPanel({
  streams,
  highlightChannel = '',
  loading = false,
}: ActiveRelaysPanelProps) {
  return (
    <section className="rounded-xl border border-white/10 bg-[#0e0e10] p-4">
      <div className="mb-3 flex items-center justify-between gap-2">
        <div className="text-[11px] font-black uppercase tracking-wide text-zinc-500">Active relays</div>
        <div className="text-xs font-semibold text-zinc-500">{streams.length} live</div>
      </div>
      {loading && !streams.length ? (
        <div className="text-sm font-semibold text-zinc-500">Loading relay snapshot…</div>
      ) : !streams.length ? (
        <div className="rounded-lg border border-dashed border-white/10 bg-white/[0.02] p-4 text-sm font-semibold text-zinc-500">
          No active HLS relays. Open a live channel to start a worker.
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="min-w-full text-left text-xs">
            <thead>
              <tr className="text-zinc-500">
                <th className="pb-2 pr-4 font-black uppercase">Channel</th>
                <th className="pb-2 pr-4 font-black uppercase">Listeners</th>
                <th className="pb-2 pr-4 font-black uppercase">Quality</th>
                <th className="pb-2 pr-4 font-black uppercase">Live edge</th>
                <th className="pb-2 pr-4 font-black uppercase">HLS probe</th>
                <th className="pb-2 font-black uppercase">Backend</th>
              </tr>
            </thead>
            <tbody>
              {streams.map(stream => {
                const highlighted = highlightChannel && stream.channel.toLowerCase() === highlightChannel
                return (
                  <tr
                    key={stream.channel}
                    className={`border-t border-white/5 ${highlighted ? 'bg-violet-500/10 text-violet-50' : 'text-zinc-300'}`}
                  >
                    <td className="py-2 pr-4">
                      <Link to={`/c/${encodeURIComponent(stream.channel)}`} className="font-black text-white hover:text-violet-200">
                        {stream.channel}
                      </Link>
                    </td>
                    <td className="py-2 pr-4">{stream.listeners ?? '—'}</td>
                    <td className="py-2 pr-4">{stream.quality || '—'}</td>
                    <td className="py-2 pr-4">{stream.liveEdge ?? '—'}</td>
                    <td className="py-2 pr-4">
                      {stream.hlsProbeDurationMs != null ? fmtMs(stream.hlsProbeDurationMs) : 'pending'}
                    </td>
                    <td className="py-2">{stream.workerBackend || '—'}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
