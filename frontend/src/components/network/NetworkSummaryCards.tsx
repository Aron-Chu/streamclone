function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = value
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit += 1
  }
  return `${size >= 10 || unit === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[unit]}`
}

function formatDelay(value: number | null | undefined) {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  return `${value >= 10 ? value.toFixed(0) : value.toFixed(1)}s`
}

export interface NetworkSummaryCardsProps {
  rxBytes: number
  txBytes: number
  hasBytes: boolean
  relayCount: number
  chatConnections: number | null
  medianDelay: number | null
  loading?: boolean
}

export default function NetworkSummaryCards({
  rxBytes,
  txBytes,
  hasBytes,
  relayCount,
  chatConnections,
  medianDelay,
  loading = false,
}: NetworkSummaryCardsProps) {
  const cards = [
    {
      label: 'Container net I/O',
      value: hasBytes ? `${formatBytes(rxBytes)} ↓ / ${formatBytes(txBytes)} ↑` : loading ? 'Loading…' : 'Unavailable',
      detail: 'Docker rx / tx totals',
    },
    {
      label: 'Active relays',
      value: loading ? '—' : String(relayCount),
      detail: 'Live HLS workers',
    },
    {
      label: 'Chat WS clients',
      value: chatConnections === null ? (loading ? '—' : 'N/A') : String(Math.round(chatConnections)),
      detail: 'Prometheus chat_connections',
    },
    {
      label: 'Median relay delay',
      value: formatDelay(medianDelay),
      detail: 'liveEdge × target duration est.',
    },
  ]

  return (
    <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      {cards.map(card => (
        <article key={card.label} className="rounded-xl border border-white/10 bg-[#0e0e10] p-4">
          <div className="text-[11px] font-black uppercase tracking-wide text-zinc-500">{card.label}</div>
          <div className="mt-2 text-lg font-black text-white">{card.value}</div>
          <div className="mt-1 text-xs font-semibold text-zinc-500">{card.detail}</div>
        </article>
      ))}
    </section>
  )
}
