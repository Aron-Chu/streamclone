interface EmptyStreamsProps {
  query?: string
  categoryName?: string | null
}

export function EmptyStreams({ query, categoryName }: EmptyStreamsProps) {
  return (
    <div className="grid min-h-80 place-items-center rounded-md bg-[#18181b] px-6 text-center">
      <div className="max-w-md">
        <div className="mx-auto mb-4 grid h-16 w-16 place-items-center rounded-full bg-[#26262c] text-3xl">
          📡
        </div>
        <div className="text-lg font-bold text-white">No channels match right now</div>
        <div className="mt-2 text-sm leading-6 text-zinc-400">
          {query
            ? `Nothing turned up for "${query}". Try another spelling or browse a category.`
            : categoryName
              ? `No live streams in ${categoryName} at the moment — check back soon or pick another category.`
              : 'Live channels will appear here when metadata finishes loading.'}
        </div>
      </div>
    </div>
  )
}
