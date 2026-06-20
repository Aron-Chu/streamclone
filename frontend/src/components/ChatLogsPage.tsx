import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'

import {
  fetchChannelChatLogMessages,
  fetchChannelChatLogs,
  type ChannelChatLogStream,
  type UnifiedLogEntry,
} from '../api'
import ChatUserMenu, { type ChatUserFilter } from './chat/ChatUserMenu'
import LogMessageBody from './chat/LogMessageBody'

const ALL_STREAMS = 'all'
const PAGE_LIMIT = 500

type VirtualLogRow =
  | { kind: 'segment'; key: string; entry: UnifiedLogEntry }
  | { kind: 'log'; key: string; entry: UnifiedLogEntry }

function formatTs(ts: string) {
  const date = new Date(ts)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function formatStreamLabel(stream: ChannelChatLogStream) {
  const start = new Date(stream.startedAt)
  const label = Number.isNaN(start.getTime())
    ? stream.streamId
    : start.toLocaleString([], { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' })
  return stream.title ? `${label} — ${stream.title}` : label
}

function formatSegmentHeader(entry: UnifiedLogEntry) {
  const started = entry.streamStartedAt ? new Date(entry.streamStartedAt) : null
  const dateLabel = started && !Number.isNaN(started.getTime())
    ? started.toLocaleString([], { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' })
    : entry.streamId || 'Stream'
  return entry.streamTitle ? `${dateLabel} — ${entry.streamTitle}` : dateLabel
}

function useDebouncedValue<T>(value: T, delayMs: number) {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delayMs)
    return () => window.clearTimeout(timer)
  }, [value, delayMs])
  return debounced
}

function StreamSegmentHeader({ entry, login }: { entry: UnifiedLogEntry; login: string }) {
  const label = formatSegmentHeader(entry)
  return (
    <div className="sticky top-0 z-[5] border-b border-violet-400/20 bg-[#12121a]/95 px-4 py-2 backdrop-blur-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="text-xs font-black uppercase tracking-wide text-violet-200">{label}</div>
        {entry.streamId ? (
          <Link
            to={`/analytics/${encodeURIComponent(login)}?stream=${encodeURIComponent(entry.streamId)}`}
            className="text-[11px] font-bold text-violet-300/90 transition hover:text-violet-100"
          >
            Open in Analytics
          </Link>
        ) : null}
      </div>
    </div>
  )
}

function LogRow({
  entry,
  onFilterUser,
}: {
  entry: UnifiedLogEntry
  onFilterUser: (filter: ChatUserFilter) => void
}) {
  if (entry.kind === 'mod_event') {
    return (
      <div className="grid grid-cols-[9rem_1fr] gap-3 border-b border-white/5 px-4 py-2 text-sm text-amber-200/90">
        <time className="font-mono text-[11px] text-zinc-500">{formatTs(entry.ts)}</time>
        <div className="italic">{entry.modText}</div>
      </div>
    )
  }

  return (
    <div className="grid grid-cols-[9rem_10rem_1fr] gap-3 border-b border-white/5 px-4 py-2 text-sm">
      <time className="font-mono text-[11px] text-zinc-500">{formatTs(entry.ts)}</time>
      <ChatUserMenu
        displayName={entry.displayName ?? 'viewer'}
        login={entry.login}
        senderHash={entry.senderHash}
        onFilterUser={onFilterUser}
        className="truncate font-black text-violet-300 hover:underline"
      >
        {entry.displayName ?? 'viewer'}
      </ChatUserMenu>
      <div className="min-w-0 break-words text-zinc-200">
        <LogMessageBody text={entry.text} emoteFrags={entry.emoteFrags} keyPrefix={`e-${entry.source}-${entry.id}`} />
      </div>
    </div>
  )
}

export default function ChatLogsPage() {
  const { login = '', streamId: routeStreamId = '' } = useParams<{ login: string; streamId?: string }>()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const [query, setQuery] = useState(searchParams.get('q') ?? '')
  const [userFilter, setUserFilter] = useState(searchParams.get('user') ?? '')
  const [senderHash, setSenderHash] = useState(searchParams.get('senderHash') ?? '')
  const [streamId, setStreamId] = useState(routeStreamId || searchParams.get('streamId') || ALL_STREAMS)
  const scrollRef = useRef<HTMLDivElement>(null)

  const debouncedQuery = useDebouncedValue(query, 300)
  const debouncedUserFilter = useDebouncedValue(userFilter, 300)
  const debouncedSenderHash = useDebouncedValue(senderHash, 300)
  const isSearching = query !== debouncedQuery || userFilter !== debouncedUserFilter || senderHash !== debouncedSenderHash

  useEffect(() => {
    if (routeStreamId) setStreamId(routeStreamId)
  }, [routeStreamId])

  useEffect(() => {
    const next = new URLSearchParams()
    if (debouncedQuery.trim()) next.set('q', debouncedQuery.trim())
    if (debouncedUserFilter.trim()) next.set('user', debouncedUserFilter.trim())
    if (debouncedSenderHash.trim()) next.set('senderHash', debouncedSenderHash.trim())
    if (streamId.trim() && streamId !== ALL_STREAMS) next.set('streamId', streamId.trim())
    setSearchParams(next, { replace: true })
  }, [debouncedQuery, debouncedUserFilter, debouncedSenderHash, streamId, setSearchParams])

  const streamsQuery = useQuery({
    queryKey: ['chat-logs-streams', login],
    queryFn: () => fetchChannelChatLogs(login),
    enabled: Boolean(login),
  })

  const messagesQuery = useInfiniteQuery({
    queryKey: ['chat-logs-messages', login, streamId, debouncedQuery, debouncedUserFilter, debouncedSenderHash],
    queryFn: ({ pageParam }) => fetchChannelChatLogMessages(login, {
      streamId: streamId || ALL_STREAMS,
      q: debouncedQuery || undefined,
      user: debouncedUserFilter || undefined,
      senderHash: debouncedSenderHash || undefined,
      cursor: pageParam,
      limit: PAGE_LIMIT,
    }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: lastPage => lastPage.nextCursor || undefined,
    enabled: Boolean(login),
  })

  const entries = useMemo(
    () => messagesQuery.data?.pages.flatMap(page => page.entries) ?? [],
    [messagesQuery.data?.pages],
  )

  const onFilterUser = useCallback((filter: ChatUserFilter) => {
    if (filter.senderHash) {
      setSenderHash(filter.senderHash)
      setUserFilter('')
    } else {
      setUserFilter(filter.login || filter.displayName)
      setSenderHash('')
    }
  }, [])

  const streams = streamsQuery.data?.streams ?? []
  const liveCount = streamsQuery.data?.liveMessageCount ?? 0
  const syncedTotal = useMemo(
    () => streams.reduce((sum, stream) => sum + stream.messageCount, 0),
    [streams],
  )
  const selectedStream = streams.find(s => s.streamId === streamId)
  const showSegments = streamId === ALL_STREAMS || streamId === ''
  const totalForMode = streamId === 'live'
    ? liveCount
    : streamId === ALL_STREAMS || streamId === ''
      ? syncedTotal
      : selectedStream?.messageCount ?? 0

  const virtualRows = useMemo(() => {
    const rows: VirtualLogRow[] = []
    let previousStreamId = ''
    for (const entry of entries) {
      if (showSegments && entry.streamId && entry.streamId !== previousStreamId) {
        rows.push({ kind: 'segment', key: `seg-${entry.streamId}-${entry.id}`, entry })
        previousStreamId = entry.streamId
      }
      rows.push({ kind: 'log', key: `${entry.source}-${entry.kind}-${entry.id}`, entry })
    }
    return rows
  }, [entries, showSegments])

  const rowVirtualizer = useVirtualizer({
    count: virtualRows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: index => (virtualRows[index]?.kind === 'segment' ? 44 : 40),
    overscan: 12,
    getItemKey: index => virtualRows[index]?.key ?? index,
  })

  const { hasNextPage, isFetchingNextPage, fetchNextPage } = messagesQuery

  const onScroll = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    if (hasNextPage && !isFetchingNextPage && el.scrollHeight - el.scrollTop - el.clientHeight < 120) {
      void fetchNextPage()
    }
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])

  return (
    <main className="min-h-screen bg-[#07070a] text-zinc-100">
      <div className="mx-auto flex w-full max-w-6xl flex-col gap-5 px-4 py-6 sm:px-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div className="text-xs font-black uppercase tracking-wide text-zinc-500">Chat logs</div>
            <h1 className="text-2xl font-black text-white">{login}</h1>
          </div>
          <div className="flex flex-wrap gap-2">
            <Link to={`/c/${encodeURIComponent(login)}`} className="rounded border border-white/10 px-3 py-2 text-xs font-black text-zinc-300 transition hover:bg-white/10">
              Open channel
            </Link>
            <Link to={`/analytics/${encodeURIComponent(login)}`} className="rounded border border-white/10 px-3 py-2 text-xs font-black text-zinc-300 transition hover:bg-white/10">
              Analytics
            </Link>
          </div>
        </div>

        <section className="grid gap-3 rounded-xl border border-white/10 bg-white/[0.03] p-4 md:grid-cols-[1fr_16rem]">
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="grid gap-1 text-xs font-bold text-zinc-400">
              Stream
              <select
                value={streamId}
                onChange={event => {
                  const next = event.target.value
                  setStreamId(next)
                  if (next && next !== ALL_STREAMS) {
                    navigate(`/logs/${encodeURIComponent(login)}/${encodeURIComponent(next)}`, { replace: true })
                  } else {
                    navigate(`/logs/${encodeURIComponent(login)}`, { replace: true })
                  }
                }}
                className="rounded border border-white/10 bg-[#111117] px-3 py-2 text-sm font-semibold text-white"
              >
                <option value={ALL_STREAMS}>All synced streams ({syncedTotal || '0'})</option>
                <option value="live">Live archive {liveCount ? `(${liveCount})` : ''}</option>
                {streams.map(stream => (
                  <option key={stream.streamId} value={stream.streamId}>
                    {formatStreamLabel(stream)} ({stream.messageCount})
                  </option>
                ))}
              </select>
            </label>
            <label className="grid gap-1 text-xs font-bold text-zinc-400">
              User filter
              <input
                value={userFilter}
                onChange={event => setUserFilter(event.target.value)}
                onKeyDown={event => {
                  if (event.key === 'Enter') event.currentTarget.blur()
                }}
                placeholder="display name or login"
                className="rounded border border-white/10 bg-[#111117] px-3 py-2 text-sm text-white"
              />
            </label>
            <label className="grid gap-1 text-xs font-bold text-zinc-400 sm:col-span-2">
              Search
              <input
                value={query}
                onChange={event => setQuery(event.target.value)}
                onKeyDown={event => {
                  if (event.key === 'Enter') event.currentTarget.blur()
                }}
                placeholder="message text"
                className="rounded border border-white/10 bg-[#111117] px-3 py-2 text-sm text-white"
              />
            </label>
          </div>
          <aside className="rounded-lg border border-white/10 bg-black/20 p-3 text-xs text-zinc-400">
            <div className="font-black uppercase text-zinc-500">Stats</div>
            <div className="mt-2 space-y-1">
              <div>
                Loaded {entries.length.toLocaleString()} of {totalForMode ? totalForMode.toLocaleString() : '—'} synced messages
              </div>
              {isSearching || messagesQuery.isFetching ? (
                <div className="text-violet-300/90">Searching…</div>
              ) : null}
              {selectedStream ? (
                <div>Coverage: {selectedStream.firstOffsetSeconds}s–{selectedStream.lastOffsetSeconds}s</div>
              ) : null}
              {!streams.length && !liveCount ? (
                <p className="pt-2 text-amber-200/90">No synced chat yet. Run chat sync from Analytics for VOD history, or enable live persistence.</p>
              ) : null}
            </div>
          </aside>
        </section>

        <section className="overflow-hidden rounded-xl border border-white/10 bg-[#0b0b10]">
          <div className="grid grid-cols-[9rem_10rem_1fr] gap-3 border-b border-white/10 px-4 py-2 text-[10px] font-black uppercase tracking-wide text-zinc-500">
            <span>Time</span>
            <span>User</span>
            <span>Message</span>
          </div>
          <div ref={scrollRef} onScroll={onScroll} className="max-h-[70vh] overflow-y-auto">
            {messagesQuery.isLoading ? (
              <div className="px-4 py-10 text-center text-sm text-zinc-500">Loading chat log…</div>
            ) : messagesQuery.isError ? (
              <div className="px-4 py-10 text-center text-sm text-red-300">Failed to load chat log.</div>
            ) : entries.length === 0 ? (
              <div className="px-4 py-10 text-center text-sm text-zinc-500">No messages match these filters.</div>
            ) : (
              <div
                className="relative w-full"
                style={{ height: `${rowVirtualizer.getTotalSize()}px` }}
              >
                {rowVirtualizer.getVirtualItems().map(virtualRow => {
                  const row = virtualRows[virtualRow.index]
                  if (!row) return null
                  return (
                    <div
                      key={row.key}
                      data-index={virtualRow.index}
                      ref={rowVirtualizer.measureElement}
                      className="absolute left-0 top-0 w-full"
                      style={{ transform: `translateY(${virtualRow.start}px)` }}
                    >
                      {row.kind === 'segment' ? (
                        <StreamSegmentHeader entry={row.entry} login={login} />
                      ) : (
                        <LogRow entry={row.entry} onFilterUser={onFilterUser} />
                      )}
                    </div>
                  )
                })}
              </div>
            )}
            {messagesQuery.isFetchingNextPage ? (
              <div className="px-4 py-3 text-center text-xs text-zinc-500">Loading more…</div>
            ) : null}
          </div>
        </section>
      </div>
    </main>
  )
}
