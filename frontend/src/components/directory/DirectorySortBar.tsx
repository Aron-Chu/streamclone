import type { DirectorySort } from '../../utils/directorySort'
import { isLocalSort, SORT_OPTIONS } from '../../utils/directorySort'

interface DirectorySortBarProps {
  sort: DirectorySort
  onSortChange: (sort: DirectorySort) => void
}

export function DirectorySortBar({ sort, onSortChange }: DirectorySortBarProps) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-end sm:gap-4">
      {isLocalSort(sort) ? (
        <p className="text-xs text-zinc-500 sm:order-first sm:mr-auto">
          Sorted locally within loaded results
        </p>
      ) : null}
      <label className="flex items-center gap-2 text-sm text-zinc-400">
        <span className="shrink-0 font-semibold">Sort by</span>
        <select
          value={sort}
          onChange={e => onSortChange(e.target.value as DirectorySort)}
          className="rounded border border-[#3a3a3d] bg-[#18181b] px-3 py-1.5 text-sm font-semibold text-zinc-100 outline-none transition hover:border-[#53535a] focus:border-violet-400/60"
        >
          {SORT_OPTIONS.map(option => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </label>
    </div>
  )
}
