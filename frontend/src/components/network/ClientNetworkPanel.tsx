import { Link } from 'react-router-dom'
import type { ClientNetworkAnalysis, ClientNetworkSample } from '../../utils/clientNetworkProbe'
import { formatMbps } from '../../utils/clientNetworkProbe'
import NetworkSparkline from './NetworkSparkline'

const WARNING_STYLES = {
  info: 'border-sky-400/20 bg-sky-500/10 text-sky-100',
  warn: 'border-amber-300/20 bg-amber-500/10 text-amber-50',
  critical: 'border-rose-400/30 bg-rose-500/10 text-rose-50',
} as const

export interface ClientNetworkPanelProps {
  samples: ClientNetworkSample[]
  analysis: ClientNetworkAnalysis
  paused?: boolean
}

export default function ClientNetworkPanel({ samples, analysis, paused = false }: ClientNetworkPanelProps) {
  const latest = samples.length ? samples[samples.length - 1] : undefined
  const hasConnectionApi = latest != null || samples.length > 0

  const downlinkSeries = samples.map(s => s.downlinkMbps ?? 0)
  const rttSeries = samples.map(s => s.rttMs ?? 0)
  const probeSeries = samples.map(s => s.probeMbps ?? 0)

  return (
    <section className="rounded-xl border border-white/10 bg-[#0e0e10] p-4">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div className="text-[11px] font-black uppercase tracking-wide text-zinc-500">Client network</div>
        {samples.length >= 2 && !paused ? (
          <span className="rounded border border-emerald-400/20 bg-emerald-500/10 px-2 py-0.5 text-[10px] font-black uppercase text-emerald-100">
            Live · {samples.length} samples
          </span>
        ) : paused ? (
          <span className="text-[10px] font-black uppercase text-zinc-600">Paused</span>
        ) : (
          <span className="text-[10px] font-semibold text-zinc-600">Sampling…</span>
        )}
      </div>

      {!hasConnectionApi ? (
        <div className="rounded-lg border border-dashed border-white/10 bg-white/[0.02] p-4 text-sm font-semibold text-zinc-500">
          Browser network API unavailable. Throughput probes still run from this tab.
          <div className="mt-3">
            <Link to="/browse/live" className="text-xs font-black uppercase text-violet-300 transition hover:text-violet-100">
              Browse live channels →
            </Link>
          </div>
        </div>
      ) : null}

      {analysis.warnings.length ? (
        <div className="mb-4 space-y-2">
          {analysis.warnings.map(warning => (
            <div
              key={`${warning.kind}-${warning.message}`}
              className={`rounded-lg border px-3 py-2 text-xs font-semibold ${WARNING_STYLES[warning.severity]}`}
            >
              {warning.message}
            </div>
          ))}
        </div>
      ) : null}

      <div className="grid gap-4">
        <article className="rounded-lg border border-white/10 bg-white/[0.03] p-3">
          <div className="flex flex-wrap items-end justify-between gap-2">
            <div>
              <div className="text-[10px] font-black uppercase text-zinc-500">Measured throughput</div>
              <div className="mt-1 text-lg font-black text-white">{formatMbps(analysis.latestProbeMbps)}</div>
              <div className="text-[11px] font-semibold text-zinc-500">Same-origin probe · updates every 2s</div>
            </div>
            <div className="text-right">
              <div className="text-[10px] font-black uppercase text-zinc-500">Link estimate</div>
              <div className="mt-1 text-sm font-black text-zinc-200">{formatMbps(analysis.latestDownlinkMbps)}</div>
            </div>
          </div>
          <div className="mt-3">
            <NetworkSparkline series={probeSeries} color="#34d399" fill="rgba(52,211,153,0.12)" />
          </div>
          {analysis.utilizationPct != null ? (
            <div className="mt-3">
              <div className="mb-1 flex justify-between text-[10px] font-black uppercase text-zinc-500">
                <span>Bandwidth use</span>
                <span className={analysis.utilizationPct >= 85 ? 'text-amber-200' : 'text-zinc-400'}>
                  {analysis.utilizationPct.toFixed(0)}% of link estimate
                </span>
              </div>
              <div className="h-2 overflow-hidden rounded-full bg-white/5">
                <div
                  className={`h-full rounded-full transition-all duration-500 ${
                    analysis.utilizationPct >= 95
                      ? 'bg-rose-400'
                      : analysis.utilizationPct >= 75
                        ? 'bg-amber-300'
                        : 'bg-emerald-400'
                  }`}
                  style={{ width: `${Math.min(100, analysis.utilizationPct)}%` }}
                />
              </div>
            </div>
          ) : null}
        </article>

        <div className="grid gap-3 sm:grid-cols-2">
          <article className="rounded-lg border border-white/10 bg-white/[0.03] p-3">
            <div className="text-[10px] font-black uppercase text-zinc-500">Downlink estimate</div>
            <div className="mt-1 text-sm font-black text-white">{formatMbps(latest?.downlinkMbps)}</div>
            <div className="mt-2">
              <NetworkSparkline series={downlinkSeries} color="#a78bfa" />
            </div>
          </article>
          <article className="rounded-lg border border-white/10 bg-white/[0.03] p-3">
            <div className="text-[10px] font-black uppercase text-zinc-500">RTT</div>
            <div className="mt-1 text-sm font-black text-white">
              {latest?.rttMs != null ? `${latest.rttMs} ms` : '—'}
              {analysis.rttJitterMs != null ? (
                <span className="ml-2 text-[11px] font-semibold text-zinc-500">±{analysis.rttJitterMs.toFixed(0)} ms</span>
              ) : null}
            </div>
            <div className="mt-2">
              <NetworkSparkline series={rttSeries} color="#f472b6" fill="rgba(244,114,182,0.12)" />
            </div>
          </article>
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <Metric label="Effective type" value={latest?.effectiveType || '—'} />
          <Metric label="Save data" value={latest?.saveData ? 'On' : 'Off'} />
        </div>
      </div>
    </section>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-white/10 bg-white/[0.03] px-3 py-2">
      <div className="text-[10px] font-black uppercase text-zinc-500">{label}</div>
      <div className="mt-1 text-sm font-black text-white">{value}</div>
    </div>
  )
}
