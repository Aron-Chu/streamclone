import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  fetchRisingStreamers,
  PulseWireApiError,
  type PulseWireRisingStreamer,
  type PulseWireWindow,
} from '../../pulseWireApi'
import { deltaTone, formatDeltaPct, formatRankDelta, formatViewers } from '../../utils/pulseWireFormat'
import ViewerSparkline from './ViewerSparkline'

type Props = {
  window: PulseWireWindow
  category?: string
  limit?: number
  refreshKey?: number
  className?: string
}

const WINDOW_LABEL: Record<PulseWireWindow, string> = {
  today: 'today',
  '24h': '24h',
  '7d': '7d',
}

function RowSkeleton() {
  return (
    <div className="flex items-center gap-3 rounded-xl border border-[#26262C] bg-[#161619] px-4 py-3">
      <div className="h-8 w-8 shrink-0 animate-pulse rounded-full bg-[#1B1B1F]" />
      <div className="min-w-0 flex-1 space-y-2">
        <div className="h-3 w-28 animate-pulse rounded bg-[#1B1B1F]" />
        <div className="h-2.5 w-20 animate-pulse rounded bg-[#1B1B1F]" />
      </div>
      <div className="h-6 w-16 animate-pulse rounded bg-[#1B1B1F]" />
    </div>
  )
}

function RisingRow({ rank, row, window }: { rank: number; row: PulseWireRisingStreamer; window: PulseWireWindow }) {
  const name = row.displayName || row.login
  const initial = name.slice(0, 1).toUpperCase()
  return (
    <Link
      to={`/pulse-wire/streamer/${encodeURIComponent(row.login)}?window=${window}`}
      className="group flex items-center gap-3 rounded-xl border border-[#26262C] bg-[#161619] px-4 py-3 transition hover:border-[#A970FF]/50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
    >
      <span className="w-5 shrink-0 text-center text-sm font-black text-[#7A7A85]">{rank}</span>
      {row.avatarUrl ? (
        <img src={row.avatarUrl} alt="" className="h-9 w-9 shrink-0 rounded-full object-cover" loading="lazy" />
      ) : (
        <span className="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-gradient-to-br from-[#9147FF] to-[#5A2BAE] text-sm font-bold text-white">
          {initial}
        </span>
      )}
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <p className="truncate text-[15px] font-bold text-[#F7F7F8] group-hover:text-white">{name}</p>
          {row.newEntrant ? (
            <span className="rounded-full bg-[#16321F] px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wide text-[#3FCB7E]">
              New
            </span>
          ) : null}
        </div>
        <p className="truncate text-xs text-[#ADADB8]">
          {row.category || 'Live'} · {formatViewers(row.viewersNow)} viewers
        </p>
      </div>
      <div className="hidden shrink-0 sm:block">
        <ViewerSparkline points={row.sparkline} width={96} height={28} ariaLabel={`${name} viewer trend`} />
      </div>
      <div className="w-16 shrink-0 text-right">
        <p className={`text-sm font-bold ${deltaTone(row.viewerDeltaPct)}`}>{formatDeltaPct(row.viewerDeltaPct)}</p>
        <p className={`text-[11px] font-semibold ${deltaTone(row.rankDelta)}`}>{formatRankDelta(row.rankDelta)}</p>
      </div>
    </Link>
  )
}

export default function RisingLeaderboard({ window, category, limit = 8, refreshKey = 0, className = '' }: Props) {
  const [rows, setRows] = useState<PulseWireRisingStreamer[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [disabled, setDisabled] = useState(false)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError('')
    fetchRisingStreamers({ window, category: category || undefined, limit })
      .then(res => {
        if (cancelled) return
        setRows(res.items ?? [])
        setDisabled(false)
      })
      .catch(err => {
        if (cancelled) return
        setRows([])
        if (err instanceof PulseWireApiError && err.code === 'pulse_wire_disabled') {
          setDisabled(true)
          return
        }
        setError(err instanceof Error ? err.message : 'Rising leaderboard unavailable')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [window, category, limit, refreshKey])

  if (disabled) return null

  return (
    <section className={className}>
      <div className="mb-3 flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <h2 className="text-[18px] font-bold text-[#F7F7F8]">Rising leaderboard</h2>
          <span className="rounded-full border border-[#A970FF]/40 bg-[#9147FF]/15 px-2 py-0.5 text-[10px] font-bold uppercase tracking-[0.06em] text-[#A970FF]">
            {WINDOW_LABEL[window]}
          </span>
        </div>
        <span className="text-[11px] font-semibold text-[#7A7A85]">Viewer · rank momentum</span>
      </div>
      {loading ? (
        <div className="space-y-2">
          <RowSkeleton />
          <RowSkeleton />
          <RowSkeleton />
        </div>
      ) : error ? (
        <p className="rounded-xl border border-[#2A2A2E] bg-[#121217] p-4 text-xs text-[#7A7A85]">
          Rising stats are gathering data — {error}
        </p>
      ) : rows.length === 0 ? (
        <p className="rounded-xl border border-[#2A2A2E] bg-[#121217] p-4 text-xs text-[#7A7A85]">
          No rising streamers in {WINDOW_LABEL[window]} yet. The directory sampler builds this leaderboard as it collects viewer history.
        </p>
      ) : (
        <div className="pulse-wire-stagger space-y-2">
          {rows.map((row, index) => (
            <RisingRow
              key={row.login}
              rank={row.rankNow != null && row.rankNow > 0 ? row.rankNow : index + 1}
              row={row}
              window={window}
            />
          ))}
        </div>
      )}
    </section>
  )
}
