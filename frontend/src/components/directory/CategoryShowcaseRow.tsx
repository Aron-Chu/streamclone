import { Link } from 'react-router-dom'
import type { Category } from '../../api'
import { formatCategoryViewers } from '../../utils/categorySort'
import { HorizontalScrollRow } from './HorizontalScrollRow'

const W = 188
const H = 250

function thumb(url: string | undefined, w = W, h = H) {
  return (url ?? '').replace('{width}', String(w)).replace('{height}', String(h))
}

interface CategoryShowcaseRowProps {
  categories: Category[]
}

export function CategoryShowcaseRow({ categories }: CategoryShowcaseRowProps) {
  if (!categories.length) return null

  return (
    <HorizontalScrollRow gapClassName="gap-3">
      {categories.map(category => (
        <Link
          key={category.id}
          to={`/browse/category/${category.id}?name=${encodeURIComponent(category.name)}`}
          className="group flex w-36 shrink-0 flex-col gap-2 text-left transition hover:opacity-90"
        >
          <div className="aspect-[3/4] overflow-hidden rounded-md bg-[#18181b]">
            {category.thumbnailUrl ? (
              <img
                src={thumb(category.thumbnailUrl)}
                alt={category.name}
                className="h-full w-full object-cover transition duration-300 group-hover:scale-105"
              />
            ) : (
              <div className="grid h-full w-full place-items-center text-2xl text-zinc-600">🎮</div>
            )}
          </div>
          <span className="line-clamp-2 text-sm font-semibold text-zinc-200 group-hover:text-white">
            {category.name}
          </span>
          <span className="text-xs font-semibold text-zinc-500">
            {formatCategoryViewers(category.viewers)}
          </span>
        </Link>
      ))}
    </HorizontalScrollRow>
  )
}
