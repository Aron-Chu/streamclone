import { useEffect, useMemo, useRef, useState } from 'react'
import type { FormEvent, KeyboardEvent } from 'react'
import type { AuthUser, ChannelEmote, ChatBadge, EmoteBenchmark, EmoteProviderStatus } from '../api'
import { LatencySummary, useChatStore } from '../chatStore'
import { normalizeBrowserOriginUrl } from '../config'
import ChatLogRow from './chat/ChatLogRow'

function normalizeMentionToken(value: string) {
  return value.trim().replace(/^@+/, '').toLowerCase()
}

function mentionAliases(user?: AuthUser) {
  return new Set([
    user?.login,
    user?.displayName,
    user?.display_name,
  ].filter((value): value is string => Boolean(value && value.trim())).map(normalizeMentionToken))
}

function formatMs(value: number | null | undefined) {
  if (value === null || value === undefined) return '-'
  if (value < 1000) return `${value}ms`
  return `${(value / 1000).toFixed(1)}s`
}

function StatusPill({ label, tone }: { label: string; tone: 'ok' | 'warn' | 'err' | 'idle' }) {
  const classes = {
    ok: 'bg-emerald-400/15 text-emerald-200',
    warn: 'bg-amber-400/15 text-amber-100',
    err: 'bg-red-400/15 text-red-200',
    idle: 'bg-white/10 text-zinc-300',
  }[tone]
  return <span className={`rounded px-2 py-1 text-[11px] font-black uppercase ${classes}`}>{label}</span>
}

function StatLine({ label, stat }: { label: string; stat: LatencySummary }) {
  return (
    <div className="grid grid-cols-[70px_repeat(5,minmax(0,1fr))] gap-2 rounded border border-white/10 bg-white/[0.035] px-2 py-1.5 text-[11px] font-bold text-zinc-400">
      <span className="font-black uppercase text-zinc-300">{label}</span>
      <span>last {formatMs(stat.last)}</span>
      <span>p50 {formatMs(stat.p50)}</span>
      <span>p95 {formatMs(stat.p95)}</span>
      <span>max {formatMs(stat.max)}</span>
      <span>n {stat.samples}</span>
    </div>
  )
}

export interface ChatEmoteStatus {
  state: 'idle' | 'loading' | 'ready' | 'processing' | 'failed'
  count: number
  pending: number
  total?: number
  percent?: number
  error?: string
  providers?: EmoteProviderStatus[]
  benchmark?: EmoteBenchmark
}

function providerLabel(provider: string) {
  if (provider === 'seventv') return '7TV'
  if (provider === 'twitch') return 'Twitch'
  if (provider === 'ffz') return 'FFZ'
  return provider
}

function emoteLabel(status: ChatEmoteStatus | undefined, revision: number) {
  if (!status) return revision > 0 ? `emotes updated ${revision}` : 'emotes ready'
  if (status.providers?.length) {
    return status.providers.map(provider => {
      if (provider.state === 'processing') return `${providerLabel(provider.provider)} ${provider.percent ?? 0}%`
      if (provider.state === 'failed') return `${providerLabel(provider.provider)} failed`
      return `${providerLabel(provider.provider)} ready`
    }).join(' · ')
  }
  if (status.state === 'idle') return 'emotes idle'
  if (status.state === 'loading') return 'emotes loading'
  if (status.state === 'failed') return 'emotes unavailable'
  if (status.state === 'processing') return `emotes ${status.percent ?? 0}%`
  if (revision > 0) return `emotes ready ${status.count} +${revision}`
  return `emotes ready ${status.count}`
}

interface ChatProps {
  channel: string
  user?: AuthUser
  isAuthenticated: boolean
  emotes?: ChatEmoteStatus
  badgeCatalog?: Record<string, ChatBadge>
  loadedEmotes?: ChannelEmote[]
}

type CompletionState = {
  base: string
  start: number
  end: number
  index: number
}

function currentToken(value: string, cursor: number) {
  const prefix = value.slice(0, cursor)
  const match = /(^|\s)(\S*)$/.exec(prefix)
  if (!match) return { token: '', start: cursor, end: cursor }
  const token = match[2] ?? ''
  return { token, start: cursor - token.length, end: cursor }
}

function emoteMatches(emotes: ChannelEmote[], token: string) {
  const q = token.trim().toLowerCase()
  if (!q) return []
  const seen = new Set<string>()
  const rows = emotes
    .filter(emote => emote.name && emote.name.toLowerCase().includes(q))
    .sort((left, right) => {
      const l = left.name.toLowerCase()
      const r = right.name.toLowerCase()
      const lPrefix = l.startsWith(q) ? 0 : 1
      const rPrefix = r.startsWith(q) ? 0 : 1
      return lPrefix - rPrefix || l.length - r.length || l.localeCompare(r)
    })
  return rows.filter(emote => {
    const key = emote.name.toLowerCase()
    if (seen.has(key)) return false
    seen.add(key)
    return true
  }).slice(0, 12)
}

export default function Chat({ channel, user, isAuthenticated, emotes, badgeCatalog = {}, loadedEmotes = [] }: ChatProps) {
  const messages = useChatStore(s => s.messages)
  const restoredFromCache = useChatStore(s => s.restoredFromCache)
  const connectionState = useChatStore(s => s.connectionState)
  const channelState = useChatStore(s => s.channelState)
  const lastError = useChatStore(s => s.lastError)
  const emoteRevision = useChatStore(s => s.emoteRevision)
  const latencyStats = useChatStore(s => s.latencyStats)
  const sendMessage = useChatStore(s => s.sendMessage)
  const scrollRef = useRef<HTMLDivElement>(null)
  const contentRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const lastCountRef = useRef(0)
  const lastScrollTopRef = useRef(0)
  const programmaticScrollRef = useRef(false)
  const programmaticScrollTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [isPinned, setIsPinned] = useState(true)
  const [newMessages, setNewMessages] = useState(0)
  const [draft, setDraft] = useState('')
  const [statsOpen, setStatsOpen] = useState(false)
  const [focused, setFocused] = useState(false)
  const [completion, setCompletion] = useState<CompletionState | null>(null)
  const mentionNames = useMemo(() => mentionAliases(user), [user?.displayName, user?.display_name, user?.login])
  const token = currentToken(draft, inputRef.current?.selectionStart ?? draft.length)
  const activeBase = completion?.base || token.token
  const suggestions = useMemo(() => emoteMatches(loadedEmotes, activeBase), [loadedEmotes, activeBase])

  const prefillMention = (login: string) => {
    setDraft(`@${login} `)
    requestAnimationFrame(() => {
      inputRef.current?.focus()
      const cursor = (`@${login} `).length
      inputRef.current?.setSelectionRange(cursor, cursor)
    })
  }

  const scrollToBottom = () => {
    const el = scrollRef.current
    if (!el) return
    programmaticScrollRef.current = true
    if (programmaticScrollTimer.current) clearTimeout(programmaticScrollTimer.current)
    programmaticScrollTimer.current = setTimeout(() => { programmaticScrollRef.current = false }, 150)
    el.scrollTop = el.scrollHeight
    lastScrollTopRef.current = el.scrollTop
    setIsPinned(true)
    setNewMessages(0)
    requestAnimationFrame(() => {
      if (!scrollRef.current) return
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
      lastScrollTopRef.current = scrollRef.current.scrollTop
    })
  }

  const jumpLabel = newMessages > 0 ? `↓ ${newMessages} new message${newMessages === 1 ? '' : 's'}` : 'Jump to bottom'

  useEffect(() => {
    const prev = lastCountRef.current
    lastCountRef.current = messages.length
    if (messages.length === 0) {
      setNewMessages(0)
      return
    }
    if (isPinned) {
      if (document.visibilityState === 'visible') {
        requestAnimationFrame(scrollToBottom)
      }
    } else if (messages.length > prev) {
      setNewMessages(count => count + messages.length - prev)
    }
  }, [messages.length, isPinned])

  useEffect(() => {
    const onVisible = () => {
      if (document.visibilityState === 'visible' && isPinned) {
        requestAnimationFrame(() => {
          requestAnimationFrame(scrollToBottom)
        })
      }
    }
    document.addEventListener('visibilitychange', onVisible)
    return () => document.removeEventListener('visibilitychange', onVisible)
  }, [isPinned])

  useEffect(() => {
    const content = contentRef.current
    if (!content || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(() => {
      if (!isPinned) return
      requestAnimationFrame(scrollToBottom)
    })
    observer.observe(content)
    return () => observer.disconnect()
  }, [isPinned])

  const onScroll = () => {
    const el = scrollRef.current
    if (!el) return
    if (programmaticScrollRef.current) return
    const scrollTop = el.scrollTop
    const distanceFromBottom = el.scrollHeight - scrollTop - el.clientHeight
    const atBottom = distanceFromBottom < 32
    const userScrolledUp = scrollTop < lastScrollTopRef.current - 2
    lastScrollTopRef.current = scrollTop

    if (atBottom) {
      setIsPinned(true)
      setNewMessages(0)
      return
    }

    // Scrolled away from bottom: pause auto-follow until Jump to bottom.
    if (isPinned || userScrolledUp || distanceFromBottom > 32) {
      setIsPinned(false)
    }
  }

  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (!isAuthenticated) {
      return
    }
    const text = draft.trim()
    if (!text) return
    sendMessage(channel, text, user, loadedEmotes)
    setDraft('')
    setCompletion(null)
    requestAnimationFrame(scrollToBottom)
  }

  const insertCompletion = (emote: ChannelEmote, state: CompletionState | null, baseToken = token.token) => {
    const input = inputRef.current
    const cursor = input?.selectionStart ?? draft.length
    const start = state?.start ?? token.start
    const end = state?.end && state.end >= start ? cursor : token.end
    const suffix = draft.slice(end).replace(/^\s+/, '')
    const next = `${draft.slice(0, start)}${emote.name} ${suffix}`
    const nextCursor = start + emote.name.length + 1
    setDraft(next)
    setCompletion({ base: baseToken, start, end: nextCursor, index: Math.max(0, suggestions.findIndex(item => item.name === emote.name)) })
    requestAnimationFrame(() => inputRef.current?.setSelectionRange(nextCursor, nextCursor))
  }

  const completeEmote = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Escape') {
      setCompletion(null)
      return
    }
    if (event.key !== 'Tab') return
    const input = inputRef.current
    const cursor = input?.selectionStart ?? draft.length
    const active = completion && completion.start <= cursor
    const base = active ? completion.base : currentToken(draft, cursor).token
    const matches = emoteMatches(loadedEmotes, base)
    if (!matches.length) return
    event.preventDefault()
    const index = active ? (completion.index + 1) % matches.length : 0
    const start = active ? completion.start : currentToken(draft, cursor).start
    const replaceEnd = active ? cursor : currentToken(draft, cursor).end
    const suffix = draft.slice(replaceEnd).replace(/^\s+/, '')
    const next = `${draft.slice(0, start)}${matches[index].name} ${suffix}`
    const nextCursor = start + matches[index].name.length + 1
    setDraft(next)
    setCompletion({ base, start, end: nextCursor, index })
    requestAnimationFrame(() => inputRef.current?.setSelectionRange(nextCursor, nextCursor))
  }

  const tone = lastError ? 'err' : connectionState === 'open' && channelState === 'subscribed' ? 'ok' : connectionState === 'reconnecting' ? 'warn' : 'idle'
  const copyStats = () => {
    navigator.clipboard?.writeText(JSON.stringify({ channel, latencyStats, capturedAt: new Date().toISOString() }, null, 2)).catch(() => undefined)
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-[#111117] text-white">
      <div className="flex items-center justify-between border-b border-white/10 px-3 py-2.5">
        <div>
          <div className="text-sm font-black">Live Chat</div>
          <div className="mt-1 text-[11px] font-semibold text-zinc-500">
            <span title={emotes?.error}>{emoteLabel(emotes, emoteRevision)}</span>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            aria-label="Chat delay stats"
            aria-expanded={statsOpen}
            onClick={() => setStatsOpen(value => !value)}
            className="rounded border border-white/10 bg-white/[0.05] px-2 py-1 text-[11px] font-black uppercase text-zinc-300 transition hover:bg-white/10 hover:text-white"
          >
            Stats
          </button>
          <StatusPill label={lastError ? 'error' : channelState || connectionState} tone={tone} />
        </div>
      </div>
      {statsOpen ? (
        <div className="space-y-1.5 border-b border-white/10 bg-[#0b0b10] p-3">
          <div className="mb-2 flex items-center justify-between">
            <div className="text-xs font-black uppercase text-zinc-400">Rolling delay window</div>
            <button type="button" onClick={copyStats} className="rounded px-2 py-1 text-[11px] font-black text-zinc-400 transition hover:bg-white/10 hover:text-white">
              Copy JSON
            </button>
          </div>
          <StatLine label="total" stat={latencyStats.total} />
          <StatLine label="source" stat={latencyStats.source} />
          <StatLine label="relay" stat={latencyStats.relay} />
          <StatLine label="browser" stat={latencyStats.browser} />
          <StatLine label="echo" stat={latencyStats.echo} />
        </div>
      ) : null}
      {lastError ? (
        <div className="border-b border-red-400/20 bg-red-500/10 px-3 py-2 text-xs font-semibold text-red-100">{lastError}</div>
      ) : null}
      {restoredFromCache && messages.length > 0 ? (
        <div className="border-b border-amber-400/20 bg-amber-500/10 px-3 py-2 text-xs font-semibold text-amber-100">
          {connectionState === 'open'
            ? 'Showing recent cached chat from before reload. New live messages will append here.'
            : 'Showing recent cached chat while the live connection comes back.'}
        </div>
      ) : null}
      <div className="relative min-h-0 flex-1">
        <div ref={scrollRef} onScroll={onScroll} className="scrollbar-hidden h-full overflow-y-auto py-2">
          <div ref={contentRef}>
          {messages.length === 0 ? (
            <div className="grid h-full place-items-center px-6 text-center">
              <div>
                <div className="text-sm font-black text-zinc-200">No messages yet</div>
                <div className="mt-1 text-xs font-medium leading-5 text-zinc-500">
                  {connectionState === 'open'
                    ? 'Listening live — new messages appear here. Chat does not backfill earlier history.'
                    : 'Connecting to live chat…'}
                </div>
                {!isAuthenticated ? (
                  <div className="mt-3 text-xs font-semibold text-violet-200">
                    Log in from the header to send messages in this channel.
                  </div>
                ) : null}
              </div>
            </div>
          ) : (
            messages.map(msg => (
              <ChatLogRow
                key={msg.clientMsgId ?? msg.id}
                msg={msg}
                badges={badgeCatalog}
                mentionNames={mentionNames}
                canMention={isAuthenticated}
                onMention={prefillMention}
              />
            ))
          )}
          </div>
        </div>
        {!isPinned ? (
          <button
            onClick={scrollToBottom}
            className="absolute bottom-3 left-1/2 z-10 -translate-x-1/2 rounded-full border border-violet-200/30 bg-violet-500 px-4 py-2 text-xs font-black text-white shadow-xl shadow-black/50 transition hover:bg-violet-400"
          >
            {jumpLabel}
          </button>
        ) : null}
      </div>
      <form onSubmit={submit} className="border-t border-white/10 bg-[#0f0f15] p-3">
        {isAuthenticated ? (
          <div className="relative flex items-center gap-2">
            {focused && suggestions.length ? (
              <div className="absolute bottom-full left-0 right-12 z-20 mb-2 max-h-64 overflow-y-auto rounded border border-white/10 bg-[#181820] p-1 shadow-2xl shadow-black/50">
                {suggestions.slice(0, 8).map((emote, index) => (
                  <button
                    key={`${emote.provider ?? 'custom'}-${emote.emote_id}-${emote.name}`}
                    type="button"
                    onMouseDown={event => {
                      event.preventDefault()
                      insertCompletion(emote, completion, activeBase)
                    }}
                    className={`flex w-full items-center gap-2 rounded px-2 py-1.5 text-left transition ${completion?.index === index ? 'bg-violet-400/20 text-white' : 'text-zinc-300 hover:bg-white/10 hover:text-white'}`}
                  >
                    <span className="grid h-7 w-7 shrink-0 place-items-center rounded bg-black/30 p-1">
                      <img src={normalizeBrowserOriginUrl(emote.url, ['/emotes/'])} alt={emote.name} className="max-h-full max-w-full object-contain" loading="lazy" />
                    </span>
                    <span className="min-w-0 flex-1 truncate text-sm font-black">{emote.name}</span>
                    <span className="text-[10px] font-black uppercase text-zinc-500">{providerLabel(emote.provider ?? 'custom')}</span>
                  </button>
                ))}
              </div>
            ) : null}
            <input
              ref={inputRef}
              value={draft}
              onChange={event => {
                setDraft(event.target.value)
                setCompletion(null)
              }}
              onKeyDown={completeEmote}
              onFocus={() => setFocused(true)}
              onBlur={() => setFocused(false)}
              maxLength={500}
              className="min-w-0 flex-1 rounded border border-white/10 bg-white/[0.07] px-3 py-2 text-sm font-medium text-white outline-none transition placeholder:text-zinc-500 focus:border-violet-300 focus:bg-white/[0.1] focus:ring-4 focus:ring-violet-500/15"
              placeholder={`Send a message to ${channel}`}
            />
            <button
              disabled={!draft.trim()}
              className="rounded bg-violet-500 px-3 py-2 text-sm font-black text-white transition hover:bg-violet-400 disabled:cursor-not-allowed disabled:bg-white/10 disabled:text-zinc-500"
            >
              Chat
            </button>
          </div>
        ) : (
          <div className="rounded border border-white/10 bg-white/[0.04] px-3 py-2 text-xs font-bold text-zinc-400">
            Log in from the header to send chat messages.
          </div>
        )}
      </form>
    </div>
  )
}
