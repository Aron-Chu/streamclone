import type { Category } from '../../api'
import { formatCategoryViewers } from '../../utils/categorySort'

function thumb(url: string | undefined, w = 188, h = 250) {
  return (url ?? '').replace('{width}', String(w)).replace('{height}', String(h))
}

interface CategoryBrowseGridProps {
  categories: Category[]
  onSelect: (category: Category) => void
}

export function CategoryBrowseGrid({ categories, onSelect }: CategoryBrowseGridProps) {
  if (!categories.length) return null

  return (
    <div className="grid grid-cols-2 gap-x-3 gap-y-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7">
      {categories.map(category => (
        <button
          key={`${category.id}-${category.name}`}
          type="button"
          onClick={() => onSelect(category)}
          className="group min-w-0 text-left"
        >
          <div className="aspect-[3/4] overflow-hidden rounded-md bg-[#18181b]">
            {category.thumbnailUrl ? (
              <img
                src={thumb(category.thumbnailUrl)}
                alt={category.name}
                className="h-full w-full object-cover transition duration-300 group-hover:scale-105 group-hover:brightness-110"
              />
            ) : (
              <div className="grid h-full w-full place-items-center bg-[#26262c] text-sm font-black text-zinc-500">
                {category.name.slice(0, 1).toUpperCase()}
              </div>
            )}
          </div>
          <div className="mt-2 min-w-0">
            <div className="truncate text-sm font-bold text-zinc-100 group-hover:text-white">{category.name}</div>
            <div className="truncate text-sm font-semibold text-zinc-400">
              {formatCategoryViewers(category.viewers)}
            </div>
          </div>
        </button>
      ))}
    </div>
  )
}
