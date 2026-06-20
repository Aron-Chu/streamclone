import { Link } from 'react-router-dom'
import type { PulseWireTrendingStreamer, PulseWireWindow } from '../../pulseWireApi'
import { formatRelativeTime, windowShortLabel } from '../../utils/pulseWireFormat'

function volatilityTone(value: number | null | undefined) {
  if (value == null) return 'text-[#7A7A85]'
  if (value >= 70) return 'text-[#FF8C1A]'
  if (value >= 50) return 'text-[#FFB02E]'
  if (value >= 30) return 'text-[#D6D6DE]'
  return 'text-[#3FCB7E]'
}

type Props = {
  items: PulseWireTrendingStreamer[]
  window?: PulseWireWindow
  activeLogin?: string
  onSelect?: (login: string) => void
}

export default function RisingNowPanel({ items, window = '24h', activeLogin, onSelect }: Props) {
  const hasVolatility = items.some(item => item.volatility != null)
  return (
    <div className="rounded-xl border border-[#2A2A2E] bg-[#161619] p-4">
      <div className="mb-3 flex items-center justify-between gap-2">
        <div>
          <h3 className="text-[15px] font-bold text-[#F7F7F8]">Rising now</h3>
          <p className="text-[11px] font-semibold text-[#7A7A85]">Story volatility · {windowShortLabel(window)}</p>
        </div>
        <span className="text-[11px] font-semibold text-[#7A7A85]">Volatility</span>
      </div>
      {!items.length ? (
        <p className="text-xs text-[#7A7A85]">No trending streamers in {windowShortLabel(window)} yet</p>
      ) : null}
      {items.length > 0 && !hasVolatility ? (
        <p className="mb-3 text-xs text-[#7A7A85]">Volatility scores are still gathering data for these streamers.</p>
      ) : null}
      <ul className="space-y-2">
        {items.slice(0, 5).map((row, i) => {
          const active = activeLogin === row.login
          const rowContent = (
            <>
              <span className="truncate text-[#D6D6DE]">{i + 1}. {row.displayName || row.login}</span>
              <span className={`font-semibold ${volatilityTone(row.volatility)}`}>
                {row.volatility != null ? Math.round(row.volatility) : '—'}
              </span>
            </>
          )
          if (onSelect) {
            return (
              <li key={`${row.login}-${i}`}>
                <button
                  type="button"
                  onClick={() => onSelect(row.login)}
                  className={`flex w-full items-center justify-between gap-3 rounded-lg px-2 py-1.5 text-sm transition hover:bg-[#1B1B1F] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] ${
                    active ? 'bg-[#9147FF]/10 text-[#EFEFF1]' : ''
                  }`}
                >
                  {rowContent}
                </button>
              </li>
            )
          }
          return (
            <li key={`${row.login}-${i}`}>
              <Link
                to={`/pulse-wire/streamer/${encodeURIComponent(row.login)}?window=${window}`}
                className="flex items-center justify-between gap-3 rounded-lg px-2 py-1.5 text-sm transition hover:bg-[#1B1B1F] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
              >
                {rowContent}
              </Link>
            </li>
          )
        })}
      </ul>
      {items.some(item => item.lastSeen) ? (
        <p className="mt-3 text-[10px] text-[#7A7A85]">
          Last seen {formatRelativeTime(items[0]?.lastSeen)}
        </p>
      ) : null}
    </div>
  )
}
