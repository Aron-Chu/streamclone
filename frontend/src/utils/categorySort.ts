import type { Category } from '../api'

export type CategorySort = 'viewers' | 'name'

export const CATEGORY_SORT_OPTIONS: { value: CategorySort; label: string }[] = [
  { value: 'viewers', label: 'Viewers (High to Low)' },
  { value: 'name', label: 'Name (A-Z)' },
]

export function sortCategories(categories: Category[], sort: CategorySort): Category[] {
  const copy = [...categories]
  if (sort === 'name') {
    copy.sort((a, b) => a.name.toLowerCase().localeCompare(b.name.toLowerCase()))
    return copy
  }
  copy.sort((a, b) => (b.viewers ?? 0) - (a.viewers ?? 0) || a.name.toLowerCase().localeCompare(b.name.toLowerCase()))
  return copy
}

export function formatCategoryViewers(count: number | undefined): string {
  const value = Math.max(0, count ?? 0)
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1).replace(/\.0$/, '')}M viewers`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1).replace(/\.0$/, '')}K viewers`
  return `${value.toLocaleString()} viewers`
}
