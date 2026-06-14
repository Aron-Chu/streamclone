import type { Stream } from '../../api'
import { DirectoryStreamCard } from './DirectoryStreamCard'
import { HorizontalScrollRow } from './HorizontalScrollRow'

export function StreamGrid({ streams }: { streams: Stream[] }) {
  return (
    <div className="grid grid-cols-2 gap-4 md:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
      {streams.map((s, i) => (
        <DirectoryStreamCard key={`${s.login}-${s.id ?? i}`} stream={s} index={i} />
      ))}
    </div>
  )
}

export function StreamShelf({ streams }: { streams: Stream[] }) {
  if (!streams.length) return null

  return (
    <HorizontalScrollRow>
      {streams.map((s, i) => (
        <div key={`${s.login}-${s.id ?? i}`} className="w-[280px] shrink-0 sm:w-[320px]">
          <DirectoryStreamCard stream={s} index={i} />
        </div>
      ))}
    </HorizontalScrollRow>
  )
}

export function StreamShelfSkeleton() {
  return (
    <HorizontalScrollRow>
      {Array.from({ length: 5 }).map((_, i) => (
        <div key={i} className="w-[280px] shrink-0 overflow-hidden rounded-md bg-[#18181b] sm:w-[320px]">
          <div className="aspect-video animate-pulse bg-[#26262c]" />
          <div className="flex gap-2 p-2">
            <div className="h-9 w-9 shrink-0 animate-pulse rounded-full bg-[#26262c]" />
            <div className="flex-1 space-y-2 py-1">
              <div className="h-3 w-5/6 animate-pulse rounded bg-[#26262c]" />
              <div className="h-2.5 w-2/3 animate-pulse rounded bg-[#26262c]" />
            </div>
          </div>
        </div>
      ))}
    </HorizontalScrollRow>
  )
}

export function SkeletonGrid() {
  return (
    <div className="grid grid-cols-2 gap-4 md:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
      {Array.from({ length: 10 }).map((_, i) => (
        <div key={i} className="overflow-hidden rounded-md bg-[#18181b]">
          <div className="aspect-video animate-pulse bg-[#26262c]" />
          <div className="flex gap-2 p-2">
            <div className="h-9 w-9 shrink-0 animate-pulse rounded-full bg-[#26262c]" />
            <div className="flex-1 space-y-2 py-1">
              <div className="h-3 w-5/6 animate-pulse rounded bg-[#26262c]" />
              <div className="h-2.5 w-2/3 animate-pulse rounded bg-[#26262c]" />
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

export function CategoryGridSkeleton() {
  return (
    <div className="grid grid-cols-2 gap-x-3 gap-y-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7">
      {Array.from({ length: 14 }).map((_, i) => (
        <div key={i} className="min-w-0">
          <div className="aspect-[3/4] animate-pulse rounded-md bg-[#18181b]" />
          <div className="mt-2 h-4 w-5/6 animate-pulse rounded bg-[#18181b]" />
          <div className="mt-1 h-3 w-2/3 animate-pulse rounded bg-[#18181b]" />
        </div>
      ))}
    </div>
  )
}
