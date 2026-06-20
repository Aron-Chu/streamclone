import type { PulseWireRankModel, PulseWireWindow } from '../../pulseWireApi'
import { formatSince, pulseWireModeSubtitle, rankModelLabel, windowEditionTitle, windowTagline } from '../../utils/pulseWireFormat'
import { WINDOW_OPTIONS } from './PulseWireFilters'

type Props = {
  mode?: 'trending' | 'wire'
  window: PulseWireWindow
  since?: string
  rankModel?: PulseWireRankModel | null
  refreshing?: boolean
  loading?: boolean
  disabled?: boolean
  onWindowChange: (next: PulseWireWindow) => void
  onRefresh: () => void
}

export default function PulseWireEditionHeader({
  mode = 'wire',
  window,
  since,
  rankModel,
  refreshing = false,
  loading = false,
  disabled = false,
  onWindowChange,
  onRefresh,
}: Props) {
  return (
    <header className="mb-4 border-b border-[#202027] pb-4">
      <div className="flex flex-col gap-3 xl:flex-row xl:items-end xl:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-black text-[#F7F7F8]">Pulse Wire</h1>
            <span className="text-[11px] font-semibold text-[#7A7A85]">{pulseWireModeSubtitle(mode)}</span>
          </div>
          <p className="mt-1 text-sm font-semibold text-[#EFEFF1]">{windowEditionTitle(window, mode)}</p>
          <p className="mt-0.5 max-w-3xl text-xs leading-relaxed text-[#7A7A85]">{windowTagline(window, mode)}</p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <div className="flex flex-wrap gap-1.5" role="group" aria-label="Pulse Wire time window">
            {WINDOW_OPTIONS.map(option => {
              const active = window === option.id
              return (
                <button
                  key={option.id}
                  type="button"
                  onClick={() => onWindowChange(option.id)}
                  aria-pressed={active}
                  className={`rounded-lg border px-3 py-1.5 text-xs font-semibold transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] ${
                    active
                      ? 'border-[#A970FF] bg-[#221534] text-[#EFEFF1]'
                      : 'border-[#2A2A2E] bg-[#111116] text-[#ADADB8] hover:border-[#3A3A40] hover:text-[#EFEFF1]'
                  }`}
                >
                  {option.label}
                </button>
              )
            })}
          </div>
          <button
            type="button"
            onClick={onRefresh}
            disabled={refreshing || loading || disabled}
            className="rounded-lg border border-[#2A2A2E] bg-[#111116] px-3 py-1.5 text-xs font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/40 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] disabled:cursor-not-allowed disabled:opacity-50"
          >
            {refreshing ? 'Refreshing...' : 'Refresh'}
          </button>
          <span className="rounded-lg border border-[#2A2A2E] bg-[#111116] px-2.5 py-1.5 text-[11px] font-semibold text-[#7A7A85]">
            {formatSince(since, window)}
          </span>
          <span className="rounded-lg border border-[#2A2A2E] bg-[#111116] px-2.5 py-1.5 text-[11px] font-semibold text-[#7A7A85]">
            {rankModelLabel(rankModel)}
          </span>
        </div>
      </div>
    </header>
  )
}
