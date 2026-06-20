import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  fetchPulseWireDaily,
  PulseWireApiError,
  type PulseWireEditionResponse,
  type PulseWireRisingStreamer,
} from '../../pulseWireApi'
import { deltaTone, formatDeltaPct, formatViewers } from '../../utils/pulseWireFormat'

type Props = {
  refreshKey?: number
  className?: string
}

function KpiTile({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="min-w-[120px] flex-1 rounded-xl border border-[#26262C] bg-[#161619] px-4 py-3">
      <p className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">{label}</p>
      <p className="mt-1 text-xl font-black text-[#F7F7F8]">{value}</p>
      {hint ? <p className="text-[11px] text-[#7A7A85]">{hint}</p> : null}
    </div>
  )
}

function MoverTile({ label, mover, accent }: { label: string; mover?: PulseWireRisingStreamer; accent: string }) {
  if (!mover) {
    return (
      <div className="min-w-[160px] flex-1 rounded-xl border border-[#26262C] bg-[#161619] px-4 py-3">
        <p className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">{label}</p>
        <p className="mt-1 text-sm text-[#7A7A85]">Gathering data</p>
      </div>
    )
  }
  const name = mover.displayName || mover.login
  return (
    <Link
      to={`/pulse-wire/streamer/${encodeURIComponent(mover.login)}`}
      className="group min-w-[160px] flex-1 rounded-xl border border-[#26262C] bg-[#161619] px-4 py-3 transition hover:border-[#A970FF]/50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
    >
      <p className="text-[11px] font-bold uppercase tracking-[0.06em]" style={{ color: accent }}>{label}</p>
      <div className="mt-1 flex items-center gap-2">
        {mover.avatarUrl ? (
          <img src={mover.avatarUrl} alt="" className="h-7 w-7 shrink-0 rounded-full object-cover" loading="lazy" />
        ) : (
          <span className="grid h-7 w-7 shrink-0 place-items-center rounded-full bg-[#1B1B1F] text-xs font-bold text-[#ADADB8]">
            {name.slice(0, 1).toUpperCase()}
          </span>
        )}
        <span className="min-w-0 truncate text-sm font-bold text-[#F7F7F8] group-hover:text-white">{name}</span>
      </div>
      <p className={`mt-1 text-sm font-bold ${deltaTone(mover.viewerDeltaPct)}`}>
        {formatDeltaPct(mover.viewerDeltaPct)} · {formatViewers(mover.viewersNow)} viewers
      </p>
    </Link>
  )
}

export default function DailyMoversStrip({ refreshKey = 0, className = '' }: Props) {
  const [edition, setEdition] = useState<PulseWireEditionResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [disabled, setDisabled] = useState(false)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError('')
    fetchPulseWireDaily()
      .then(res => {
        if (cancelled) return
        setEdition(res)
        setDisabled(false)
      })
      .catch(err => {
        if (cancelled) return
        setEdition(null)
        if (err instanceof PulseWireApiError && err.code === 'pulse_wire_disabled') {
          setDisabled(true)
          return
        }
        setError(err instanceof Error ? err.message : 'Daily edition unavailable')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [refreshKey])

  if (disabled) return null

  const topGainer = edition?.topGainers?.[0]
  const topDropper = edition?.topDroppers?.[0]

  return (
    <section className={className} aria-label="Today on Twitch">
      <div className="mb-3 flex items-center gap-2">
        <h2 className="text-[18px] font-bold text-[#F7F7F8]">Today on Twitch</h2>
        {edition?.date ? <span className="text-[11px] font-semibold text-[#7A7A85]">{edition.date}</span> : null}
      </div>
      {loading ? (
        <div className="flex flex-wrap gap-3">
          {Array.from({ length: 5 }).map((_, index) => (
            <div key={index} className="h-[74px] min-w-[120px] flex-1 animate-pulse rounded-xl border border-[#26262C] bg-[#161619]" />
          ))}
        </div>
      ) : error || !edition ? (
        <p className="rounded-xl border border-[#2A2A2E] bg-[#121217] p-4 text-xs text-[#7A7A85]">
          Daily movers are gathering data — the directory sampler populates this strip after a few sampling runs.
        </p>
      ) : (
        <div className="flex flex-wrap gap-3">
          <KpiTile label="Live channels" value={formatViewers(edition.totalLive)} hint="sampled" />
          <KpiTile label="Total viewers" value={formatViewers(edition.totalViewers)} />
          <KpiTile
            label="New entrants"
            value={edition.newEntrants?.length != null ? String(edition.newEntrants.length) : '—'}
          />
          <KpiTile
            label="Bans today"
            value={edition.bansOfTheDay?.length != null ? String(edition.bansOfTheDay.length) : '—'}
          />
          <MoverTile label="Top gainer" mover={topGainer} accent="#3FCB7E" />
          <MoverTile label="Top dropper" mover={topDropper} accent="#FF5C57" />
        </div>
      )}
    </section>
  )
}
