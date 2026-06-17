import { useEffect, useMemo, useRef, useState } from 'react'
import NetworkSparkline from './NetworkSparkline'

const MAX_SAMPLES = 60

function formatRate(bytesPerSec: number) {
  if (!Number.isFinite(bytesPerSec) || bytesPerSec <= 0) return '0 B/s'
  const units = ['B/s', 'KB/s', 'MB/s', 'GB/s']
  let size = bytesPerSec
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit += 1
  }
  return `${size >= 10 || unit === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[unit]}`
}

export interface StackThroughputSample {
  at: number
  rxBps: number
  txBps: number
}

export interface LiveStackThroughputPanelProps {
  rxBytes: number
  txBytes: number
  hasBytes: boolean
  loading?: boolean
}

export default function LiveStackThroughputPanel({
  rxBytes,
  txBytes,
  hasBytes,
  loading = false,
}: LiveStackThroughputPanelProps) {
  const [samples, setSamples] = useState<StackThroughputSample[]>([])
  const prevRef = useRef<{ rxBytes: number; txBytes: number; at: number } | null>(null)

  useEffect(() => {
    if (!hasBytes) return
    const now = Date.now()
    const prev = prevRef.current
    prevRef.current = { rxBytes, txBytes, at: now }
    if (!prev) return
    const elapsedSec = Math.max((now - prev.at) / 1000, 0.001)
    const rxBps = Math.max(0, (rxBytes - prev.rxBytes) / elapsedSec)
    const txBps = Math.max(0, (txBytes - prev.txBytes) / elapsedSec)
    setSamples(current => [...current, { at: now, rxBps, txBps }].slice(-MAX_SAMPLES))
  }, [hasBytes, rxBytes, txBytes])

  const latest = samples.length ? samples[samples.length - 1] : undefined
  const rxSeries = useMemo(() => samples.map(s => s.rxBps), [samples])
  const txSeries = useMemo(() => samples.map(s => s.txBps), [samples])

  return (
    <section className="rounded-xl border border-white/10 bg-[#0e0e10] p-4">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div className="text-[11px] font-black uppercase tracking-wide text-zinc-500">Stack throughput</div>
        {samples.length >= 2 ? (
          <span className="rounded border border-emerald-400/20 bg-emerald-500/10 px-2 py-0.5 text-[10px] font-black uppercase text-emerald-100">
            Live docker rates
          </span>
        ) : null}
      </div>

      {!hasBytes ? (
        <div className="text-sm font-semibold text-zinc-500">
          {loading ? 'Waiting for Docker stats…' : 'Start setup-control to stream container rx/tx rates.'}
        </div>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          <article className="rounded-lg border border-white/10 bg-white/[0.03] p-3">
            <div className="text-[10px] font-black uppercase text-zinc-500">Download rate</div>
            <div className="mt-1 text-lg font-black text-white">{latest ? formatRate(latest.rxBps) : '—'}</div>
            <div className="mt-2">
              <NetworkSparkline series={rxSeries} color="#60a5fa" fill="rgba(96,165,250,0.12)" />
            </div>
          </article>
          <article className="rounded-lg border border-white/10 bg-white/[0.03] p-3">
            <div className="text-[10px] font-black uppercase text-zinc-500">Upload rate</div>
            <div className="mt-1 text-lg font-black text-white">{latest ? formatRate(latest.txBps) : '—'}</div>
            <div className="mt-2">
              <NetworkSparkline series={txSeries} color="#fbbf24" fill="rgba(251,191,36,0.12)" />
            </div>
          </article>
        </div>
      )}
    </section>
  )
}
