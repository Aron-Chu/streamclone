import {
  CATEGORY_SORT_OPTIONS,
  type CategorySort,
} from '../../utils/categorySort'

interface CategorySortBarProps {
  sort: CategorySort
  onSortChange: (sort: CategorySort) => void
}

export function CategorySortBar({ sort, onSortChange }: CategorySortBarProps) {
  return (
    <label className="flex items-center gap-2 text-sm text-zinc-400">
      <span className="shrink-0 font-semibold">Sort by</span>
      <select
        value={sort}
        onChange={e => onSortChange(e.target.value as CategorySort)}
        className="rounded border border-[#3a3a3d] bg-[#18181b] px-3 py-1.5 text-sm font-semibold text-zinc-100 outline-none transition hover:border-[#53535a] focus:border-violet-400/60"
      >
        {CATEGORY_SORT_OPTIONS.map(option => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  )
}
