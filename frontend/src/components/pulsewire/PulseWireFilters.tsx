import type { PulseWireFeedSort, PulseWireWindow } from '../../pulseWireApi'

export type PulseWireFilterChip =
  | 'all'
  | 'live_now'
  | 'drama'
  | 'funny'
  | 'bans'
  | 'records'
  | 'esports'
  | 'unverified'
  | 'high_volatility'
  | 'saved'

export const CHIP_OPTIONS: ReadonlyArray<{ id: PulseWireFilterChip; label: string }> = [
  { id: 'all', label: 'All' },
  { id: 'live_now', label: 'Live now' },
  { id: 'drama', label: 'Drama' },
  { id: 'funny', label: 'Funny' },
  { id: 'bans', label: 'Bans' },
  { id: 'records', label: 'Records' },
  { id: 'esports', label: 'Esports' },
  { id: 'unverified', label: 'Unverified' },
  { id: 'high_volatility', label: 'High volatility' },
  { id: 'saved', label: 'Saved' },
]

export const WINDOW_OPTIONS: ReadonlyArray<{ id: PulseWireWindow; label: string }> = [
  { id: 'today', label: 'Today' },
  { id: '24h', label: '24h' },
  { id: '7d', label: '7d' },
]

export function chipLabel(chip: PulseWireFilterChip): string {
  return CHIP_OPTIONS.find(option => option.id === chip)?.label ?? chip
}

export function chipToFeedParams(chip: PulseWireFilterChip, sort: PulseWireFeedSort) {
  const params: { state?: string; category?: string; sort: PulseWireFeedSort } = { sort }
  switch (chip) {
    case 'live_now':
      params.state = 'published'
      break
    case 'unverified':
      params.state = 'unverified'
      break
    case 'high_volatility':
      params.sort = 'volatility'
      break
    case 'saved':
      break
    case 'drama':
    case 'funny':
    case 'bans':
    case 'records':
    case 'esports':
      params.category = chip
      break
    default:
      break
  }
  if (sort === 'volatility' && chip !== 'high_volatility') {
    params.sort = 'volatility'
  }
  return params
}

export default function PulseWireFilters({
  chip,
  sort,
  activeLogin,
  searchQuery,
  onChipChange,
  onSortChange,
  onClearLogin,
  onSearchChange,
  wireFriendly = false,
}: {
  chip: PulseWireFilterChip
  sort: PulseWireFeedSort
  activeLogin?: string
  searchQuery?: string
  onChipChange: (next: PulseWireFilterChip) => void
  onSortChange: (next: PulseWireFeedSort) => void
  onClearLogin?: () => void
  onSearchChange?: (next: string) => void
  wireFriendly?: boolean
}) {
  return (
    <div className="mb-6 space-y-3">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-[11px] font-bold uppercase tracking-[0.08em] text-[#A970FF]">
          {wireFriendly ? 'Wire stories' : 'Headlines'}
        </p>
        <label className="flex shrink-0 items-center gap-2 text-xs font-semibold text-[#ADADB8]">
          <span>Sort</span>
          <select
            value={sort}
            onChange={event => onSortChange(event.target.value as PulseWireFeedSort)}
            className="rounded-lg border border-[#2A2A2E] bg-[#1B1B1F] px-3 py-1.5 text-xs font-semibold text-[#EFEFF1] outline-none focus-visible:border-[#A970FF] focus-visible:ring-2 focus-visible:ring-[#A970FF]/40"
          >
            <option value="rank">{wireFriendly ? 'Hot' : 'Rank'}</option>
            <option value="updated">{wireFriendly ? 'Newest' : 'Updated'}</option>
            <option value="volatility">{wireFriendly ? 'Spreading' : 'Volatility'}</option>
          </select>
        </label>
      </div>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
        <label className="min-w-0 flex-1">
          <span className="sr-only">Search streamer, topic, or story title</span>
          <input
            type="search"
            value={searchQuery ?? ''}
            onChange={event => onSearchChange?.(event.target.value)}
            placeholder="Search streamers, topics, or titles"
            className="w-full rounded-lg border border-[#2A2A2E] bg-[#121217] px-3 py-2 text-sm text-[#EFEFF1] outline-none placeholder:text-[#6F6F78] focus-visible:border-[#A970FF] focus-visible:ring-2 focus-visible:ring-[#A970FF]/40"
          />
        </label>
        {searchQuery ? (
          <button
            type="button"
            onClick={() => onSearchChange?.('')}
            className="rounded-lg border border-[#2A2A2E] bg-[#1B1B1F] px-3 py-2 text-xs font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
          >
            Clear search
          </button>
        ) : null}
      </div>
      {activeLogin ? (
        <div className="flex flex-wrap items-center gap-2 text-xs text-[#ADADB8]">
          <span>
            Filtering streamer{' '}
            <span className="font-semibold text-[#EFEFF1]">{activeLogin}</span>
          </span>
          <button
            type="button"
            onClick={onClearLogin}
            className="rounded-full border border-[#2A2A2E] px-2 py-0.5 font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
          >
            Clear
          </button>
        </div>
      ) : null}
      <div className="flex flex-wrap gap-2" role="group" aria-label="Category filters">
        {CHIP_OPTIONS.map(option => {
          const active = chip === option.id
          return (
            <button
              key={option.id}
              type="button"
              onClick={() => onChipChange(option.id)}
              aria-pressed={active}
              className={`rounded-full border px-3 py-1.5 text-xs font-semibold transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] ${
                active
                  ? 'border-[#A970FF] bg-[#9147FF]/20 text-[#EFEFF1]'
                  : 'border-[#2A2A2E] bg-[#1B1B1F] text-[#ADADB8] hover:border-[#3A3A40] hover:text-[#EFEFF1]'
              }`}
            >
              {option.label}
            </button>
          )
        })}
      </div>
    </div>
  )
}
