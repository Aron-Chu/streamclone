import type { OpsNetworkPromMetric, OpsNetworkPrometheus } from '../../api'

const METRIC_ENTRIES: Array<{ key: keyof OpsNetworkPrometheus; label: string; duration?: boolean }> = [
  { key: 'httpRequestsPerSec', label: 'HTTP req/s' },
  { key: 'chatConnections', label: 'Chat WS clients' },
  { key: 'streamListeners', label: 'Stream listeners' },
  { key: 'chatMessagesOutPerSec', label: 'Chat msgs/s' },
  { key: 'upstreamP95Sec', label: 'Upstream p95', duration: true },
]

function formatMetricValue(metric: OpsNetworkPromMetric | undefined, duration = false) {
  if (!metric) return '—'
  const value = metric.value
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  if (duration) return `${value.toFixed(2)}s`
  if (value >= 100) return value.toFixed(0)
  if (value >= 10) return value.toFixed(1)
  return value.toFixed(2)
}

function seriesLabel(labels?: Record<string, string>) {
  if (!labels) return 'total'
  return labels.service ?? labels.job ?? labels.instance ?? Object.values(labels)[0] ?? 'total'
}

export interface ServiceThroughputPanelProps {
  prometheus?: OpsNetworkPrometheus
  pulseReady: boolean
  loading?: boolean
  onStartPulse?: () => void
  startingPulse?: boolean
}

export default function ServiceThroughputPanel({
  prometheus,
  pulseReady,
  loading = false,
  onStartPulse,
  startingPulse = false,
}: ServiceThroughputPanelProps) {
  return (
    <section className="rounded-xl border border-white/10 bg-[#0e0e10] p-4">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div className="text-[11px] font-black uppercase tracking-wide text-zinc-500">Service throughput</div>
        {!pulseReady ? (
          <span className="rounded border border-amber-300/20 bg-amber-400/10 px-2 py-1 text-[10px] font-black uppercase text-amber-100">
            Pulse offline
          </span>
        ) : null}
      </div>
      {!pulseReady ? (
        <div className="mb-4 rounded-lg border border-amber-300/20 bg-amber-500/10 p-3 text-sm font-semibold text-amber-50">
          Start Pulse dashboards for historical rates and Grafana ops dashboards.
          {onStartPulse ? (
            <button
              type="button"
              onClick={onStartPulse}
              disabled={startingPulse}
              className="ml-2 rounded border border-violet-400/40 bg-violet-500/15 px-2 py-1 text-[11px] font-black uppercase text-violet-100 transition hover:bg-violet-500/25 disabled:opacity-50"
            >
              {startingPulse ? 'Starting…' : 'Start Pulse dashboards'}
            </button>
          ) : null}
        </div>
      ) : null}
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {METRIC_ENTRIES.map(entry => {
          const metric = prometheus?.[entry.key]
          const series = metric?.series ?? []
          return (
            <article key={entry.key} className="rounded-lg border border-white/10 bg-white/[0.03] p-3">
              <div className="text-[10px] font-black uppercase text-zinc-500">{entry.label}</div>
              <div className="mt-1 text-lg font-black text-white">
                {loading && !metric ? '—' : formatMetricValue(metric, entry.duration)}
              </div>
              {series.length ? (
                <div className="mt-2 space-y-1">
                  {series.slice(0, 4).map(row => (
                    <div key={`${entry.key}-${seriesLabel(row.labels)}-${row.value}`} className="flex justify-between gap-2 text-[11px] font-semibold text-zinc-500">
                      <span className="truncate">{seriesLabel(row.labels)}</span>
                      <span className="font-mono text-zinc-300">{formatMetricValue({ query: entry.key, value: row.value }, entry.duration)}</span>
                    </div>
                  ))}
                </div>
              ) : null}
            </article>
          )
        })}
      </div>
    </section>
  )
}
