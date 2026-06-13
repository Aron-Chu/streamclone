import { useQuery } from '@tanstack/react-query'
import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { search, type Category, type Stream } from '../api'

const MIN_QUERY = 2
const DEBOUNCE_MS = 250

function thumb(url: string | undefined, w = 44, h = 44) {
  return (url ?? '').replace('{width}', String(w)).replace('{height}', String(h))
}

function streamIsLive(stream: Stream) {
  return stream.isLive ?? Boolean(stream.thumbnailUrl && (stream.viewers ?? 0) > 0)
}

type SearchRow =
  | { kind: 'channel'; stream: Stream }
  | { kind: 'category'; category: Category }

interface ChannelSearchInputProps {
  className?: string
  placeholder?: string
  onNavigate?: () => void
}

export default function ChannelSearchInput({
  className = '',
  placeholder = 'Search channels (live or offline)',
  onNavigate,
}: ChannelSearchInputProps) {
  const listId = useId()
  const rootRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')
  const [open, setOpen] = useState(false)
  const [activeIndex, setActiveIndex] = useState(-1)

  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedQuery(query.trim()), DEBOUNCE_MS)
    return () => window.clearTimeout(timer)
  }, [query])

  useEffect(() => {
    setActiveIndex(-1)
  }, [debouncedQuery])

  const searchResults = useQuery({
    queryKey: ['search', debouncedQuery],
    queryFn: () => search(debouncedQuery),
    enabled: debouncedQuery.length >= MIN_QUERY,
    staleTime: 30_000,
  })

  const rows = useMemo(() => {
    const streams = searchResults.data?.streams ?? []
    const live = streams.filter(streamIsLive)
    const offline = streams.filter(s => !streamIsLive(s))
    const items: SearchRow[] = [
      ...live.map(stream => ({ kind: 'channel' as const, stream })),
      ...offline.map(stream => ({ kind: 'channel' as const, stream })),
      ...(searchResults.data?.categories ?? []).map(category => ({ kind: 'category' as const, category })),
    ]
    return items
  }, [searchResults.data?.categories, searchResults.data?.streams])

  useEffect(() => {
    if (!open) return
    const onPointerDown = (event: MouseEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', onPointerDown)
    return () => document.removeEventListener('mousedown', onPointerDown)
  }, [open])

  const close = () => {
    setOpen(false)
    setActiveIndex(-1)
  }

  const goToChannel = (login: string) => {
    close()
    setQuery('')
    setDebouncedQuery('')
    onNavigate?.()
    navigate(`/c/${login}`)
  }

  const goToBrowseSearch = () => {
    const term = debouncedQuery || query.trim()
    close()
    setQuery('')
    setDebouncedQuery('')
    onNavigate?.()
    navigate('/', { state: { directorySearch: term } })
  }

  const activateRow = (row: SearchRow) => {
    if (row.kind === 'channel') {
      goToChannel(row.stream.login)
      return
    }
    close()
    setQuery('')
    setDebouncedQuery('')
    onNavigate?.()
    navigate('/', { state: { directoryCategoryId: row.category.id, directoryCategoryName: row.category.name } })
  }

  const showPanel = open && query.trim().length >= MIN_QUERY

  return (
    <div ref={rootRef} className={`relative ${className}`}>
      <div className="relative">
        <input
          ref={inputRef}
          type="search"
          value={query}
          spellCheck={false}
          autoCorrect="off"
          autoCapitalize="off"
          aria-autocomplete="list"
          aria-controls={listId}
          aria-expanded={showPanel}
          placeholder={placeholder}
          className="w-full rounded-lg border border-white/10 bg-white/[0.07] px-3 py-2 text-sm font-semibold text-white outline-none transition placeholder:text-zinc-500 focus:border-violet-300 focus:bg-white/[0.1] focus:ring-4 focus:ring-violet-500/15 lg:px-4 lg:py-2.5"
          onFocus={() => setOpen(true)}
          onChange={event => {
            setQuery(event.target.value)
            setOpen(true)
          }}
          onKeyDown={event => {
            if (event.key === 'Escape') {
              close()
              inputRef.current?.blur()
              return
            }
            if (event.key === 'ArrowDown') {
              event.preventDefault()
              if (!rows.length) return
              setOpen(true)
              setActiveIndex(index => (index + 1) % rows.length)
              return
            }
            if (event.key === 'ArrowUp') {
              event.preventDefault()
              if (!rows.length) return
              setOpen(true)
              setActiveIndex(index => (index <= 0 ? rows.length - 1 : index - 1))
              return
            }
            if (event.key === 'Enter') {
              if (activeIndex >= 0 && rows[activeIndex]) {
                event.preventDefault()
                activateRow(rows[activeIndex])
                return
              }
              if (query.trim().length >= MIN_QUERY) {
                event.preventDefault()
                goToBrowseSearch()
              }
            }
          }}
        />
        {query ? (
          <button
            type="button"
            onClick={() => {
              setQuery('')
              setDebouncedQuery('')
              inputRef.current?.focus()
            }}
            className="absolute right-2 top-1/2 -translate-y-1/2 rounded px-2 py-1 text-xs font-bold text-zinc-300 transition hover:bg-white/10 hover:text-white"
          >
            Clear
          </button>
        ) : null}
      </div>

      {showPanel ? (
        <div
          id={listId}
          role="listbox"
          className="absolute left-0 right-0 top-[calc(100%+0.35rem)] z-50 overflow-hidden rounded-xl border border-white/10 bg-[#0d0d12]/95 shadow-2xl shadow-black/50 backdrop-blur-xl"
        >
          {searchResults.isLoading ? (
            <div className="px-4 py-3 text-sm font-semibold text-zinc-400">Searching…</div>
          ) : searchResults.isError ? (
            <div className="px-4 py-3 text-sm font-semibold text-red-200">Search unavailable right now.</div>
          ) : rows.length ? (
            <ul className="max-h-[min(24rem,55vh)] overflow-y-auto py-1">
              {rows.map((row, index) => {
                if (row.kind === 'category') {
                  const { category } = row
                  return (
                    <li key={`cat-${category.id}`}>
                      <button
                        type="button"
                        role="option"
                        aria-selected={activeIndex === index}
                        onMouseEnter={() => setActiveIndex(index)}
                        onClick={() => activateRow(row)}
                        className={`flex w-full items-center gap-3 px-3 py-2.5 text-left transition ${
                          activeIndex === index ? 'bg-violet-500/20' : 'hover:bg-white/[0.06]'
                        }`}
                      >
                        <div className="h-10 w-8 shrink-0 overflow-hidden rounded bg-zinc-800">
                          {category.thumbnailUrl ? (
                            <img src={thumb(category.thumbnailUrl, 32, 40)} alt="" className="h-full w-full object-cover" />
                          ) : null}
                        </div>
                        <div className="min-w-0">
                          <div className="truncate text-sm font-bold text-white">{category.name}</div>
                          <div className="text-xs font-semibold text-zinc-500">Category</div>
                        </div>
                      </button>
                    </li>
                  )
                }

                const { stream } = row
                const live = streamIsLive(stream)
                return (
                  <li key={`ch-${stream.login}`}>
                    <button
                      type="button"
                      role="option"
                      aria-selected={activeIndex === index}
                      onMouseEnter={() => setActiveIndex(index)}
                      onClick={() => activateRow(row)}
                      className={`flex w-full items-center gap-3 px-3 py-2.5 text-left transition ${
                        activeIndex === index ? 'bg-violet-500/20' : 'hover:bg-white/[0.06]'
                      }`}
                    >
                      <div className="relative h-10 w-10 shrink-0 overflow-hidden rounded-full bg-zinc-800">
                        {stream.profileImageUrl ? (
                          <img src={thumb(stream.profileImageUrl, 80, 80)} alt="" className="h-full w-full object-cover" />
                        ) : null}
                        <span className={`absolute bottom-0 right-0 h-2.5 w-2.5 rounded-full ring-2 ring-[#0d0d12] ${live ? 'bg-red-500' : 'bg-zinc-500'}`} />
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-sm font-bold text-white">{stream.displayName || stream.login}</div>
                        <div className="truncate text-xs font-semibold text-zinc-500">
                          {live
                            ? `${(stream.viewers ?? 0).toLocaleString()} viewers · ${stream.category || 'Live'}`
                            : 'Offline'}
                        </div>
                      </div>
                      <span className={`shrink-0 rounded px-2 py-0.5 text-[10px] font-black uppercase tracking-wide ${
                        live ? 'bg-red-600/90 text-white' : 'bg-zinc-700 text-zinc-200'
                      }`}>
                        {live ? 'Live' : 'Offline'}
                      </span>
                    </button>
                  </li>
                )
              })}
            </ul>
          ) : (
            <div className="px-4 py-3 text-sm font-semibold text-zinc-400">No channels match &quot;{debouncedQuery}&quot;.</div>
          )}

          <div className="border-t border-white/10 px-3 py-2">
            <button
              type="button"
              onClick={goToBrowseSearch}
              className="w-full rounded-lg px-2 py-2 text-left text-xs font-bold text-violet-200 transition hover:bg-white/[0.06]"
            >
              Browse all results for &quot;{debouncedQuery || query.trim()}&quot;
            </button>
          </div>
        </div>
      ) : null}
    </div>
  )
}

export function DirectorySearchField({
  value,
  onChange,
  className = '',
}: {
  value: string
  onChange: (value: string) => void
  className?: string
}) {
  return (
    <div className={`relative min-w-0 flex-1 ${className}`}>
      <input
        className="w-full rounded-lg border border-white/10 bg-white/[0.07] px-4 py-3 text-sm font-semibold text-white outline-none transition placeholder:text-zinc-500 focus:border-violet-300 focus:bg-white/[0.1] focus:ring-4 focus:ring-violet-500/15"
        placeholder="Search channels or categories"
        spellCheck={false}
        autoCorrect="off"
        autoCapitalize="off"
        value={value}
        onChange={event => onChange(event.target.value)}
      />
      {value.trim() ? (
        <button
          type="button"
          onClick={() => onChange('')}
          className="absolute right-2 top-1/2 -translate-y-1/2 rounded px-2 py-1 text-xs font-bold text-zinc-300 transition hover:bg-white/10 hover:text-white"
        >
          Clear
        </button>
      ) : null}
    </div>
  )
}
