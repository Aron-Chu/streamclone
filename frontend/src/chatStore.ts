import { create } from 'zustand'
import { CHAT_WS, MAX_RETAINED_MESSAGES } from './config'
import type { AuthUser, ChannelEmote } from './api'
import { buildEmoteLookup, splitZeroWidthSuffix } from './emoteText'

export interface Fragment {
  t: 'text' | 'emote' | 'mention'
  c: string
  u?: string
  zw?: boolean
}

export interface Message {
  id: string
  user: string
  color: string
  badges: string[]
  ts: number
  server_received_ts?: number
  fragments: Fragment[]
  receivedAt?: number
  source?: 'remote' | 'local'
  pending?: boolean
  clientMsgId?: string
  clientSentTs?: number
  ackState?: 'pending' | 'queued' | 'sent' | 'live' | 'error'
  error?: string
  echoLatencyMs?: number
}

export interface LatencySummary {
  last: number | null
  avg: number | null
  p50: number | null
  p95: number | null
  max: number | null
  samples: number
}

type ConnectionState = 'connecting' | 'open' | 'reconnecting' | 'closed'

interface ChatState {
  messages: Message[]
  restoredFromCache: boolean
  connectionState: ConnectionState
  channelState: string
  activeChannel: string | null
  lastError: string | null
  emoteRevision: number
  chatLatencyMs: number | null
  sourceLatencyMs: number | null
  relayLatencyMs: number | null
  browserLatencyMs: number | null
  lastEchoLatencyMs: number | null
  latencyStats: Record<'total' | 'source' | 'relay' | 'browser' | 'echo', LatencySummary>
  subscribe: (channel: string) => void
  unsubscribe: (channel: string) => void
  sendMessage: (channel: string, text: string, user?: AuthUser, loadedEmotes?: ChannelEmote[]) => void
}

let ws: WebSocket | null = null
let retryDelay = 1000
let retryTimer: ReturnType<typeof setTimeout> | null = null
const CHAT_CACHE_PREFIX = 'streamclone:chat-cache:v1:'
const CHAT_CACHE_TTL_MS = 30 * 60 * 1000
const latencyWindows: Record<'total' | 'source' | 'relay' | 'browser' | 'echo', number[]> = {
  total: [],
  source: [],
  relay: [],
  browser: [],
  echo: [],
}
const emptyLatencySummary: LatencySummary = { last: null, avg: null, p50: null, p95: null, max: null, samples: 0 }
const emptyLatencyStats: ChatState['latencyStats'] = {
  total: emptyLatencySummary,
  source: emptyLatencySummary,
  relay: emptyLatencySummary,
  browser: emptyLatencySummary,
  echo: emptyLatencySummary,
}

function send(frame: unknown) {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(frame))
  }
}

function trimMessages(messages: Message[]) {
  const cap = Number.isFinite(MAX_RETAINED_MESSAGES) && MAX_RETAINED_MESSAGES > 0 ? MAX_RETAINED_MESSAGES : 250
  return messages.length > cap ? messages.slice(messages.length - cap) : messages
}

function cacheKey(channel: string) {
  return `${CHAT_CACHE_PREFIX}${channel.trim().toLowerCase()}`
}

function cacheableMessages(messages: Message[]) {
  return trimMessages(messages.filter(message => message.ackState !== 'pending'))
}

function loadCachedMessages(channel: string) {
  if (typeof localStorage === 'undefined') return []
  try {
    const parsed = JSON.parse(localStorage.getItem(cacheKey(channel)) || 'null') as { savedAt?: number; messages?: Message[] } | null
    if (!parsed || !Array.isArray(parsed.messages)) return []
    if (Number.isFinite(parsed.savedAt) && Date.now() - Number(parsed.savedAt) > CHAT_CACHE_TTL_MS) {
      localStorage.removeItem(cacheKey(channel))
      return []
    }
    return cacheableMessages(parsed.messages)
  } catch {
    return []
  }
}

function persistMessages(channel: string, messages: Message[]) {
  if (!channel || typeof localStorage === 'undefined') return
  const cached = cacheableMessages(messages)
  try {
    if (!cached.length) {
      localStorage.removeItem(cacheKey(channel))
      return
    }
    localStorage.setItem(cacheKey(channel), JSON.stringify({ savedAt: Date.now(), messages: cached }))
  } catch {
    return
  }
}

function textOf(msg: Message) {
  return msg.fragments.map(f => f.c).join('')
}

function userLabel(user?: AuthUser) {
  return user?.displayName || user?.display_name || user?.login || 'viewer'
}

function userLogin(user?: AuthUser) {
  return (user?.login || userLabel(user)).toLowerCase()
}

function emoteFragment(name: string, emote: ChannelEmote): Fragment {
  return { t: 'emote', c: name, u: emote.url, zw: emote.zw }
}

function tokenizeClientSide(text: string, loadedEmotes?: ChannelEmote[]): Fragment[] {
  if (!loadedEmotes || !loadedEmotes.length) return [{ t: 'text', c: text }]

  const emoteMap = buildEmoteLookup(loadedEmotes)

  const fragments: Fragment[] = []
  const runes = [...text]
  let index = 0

  const flushText = (value: string) => {
    if (value) fragments.push({ t: 'text', c: value })
  }

  let pending = ''
  while (index < runes.length) {
    if (runes[index] === ' ') {
      let end = index
      while (end < runes.length && runes[end] === ' ') end++
      pending += runes.slice(index, end).join('')
      index = end
      continue
    }

    let end = index
    while (end < runes.length && runes[end] !== ' ') end++
    const word = runes.slice(index, end).join('')

    const emote = emoteMap.get(word)
    if (emote) {
      if (emote.zw && fragments.length > 0 && fragments[fragments.length - 1].t === 'emote' && !pending.trim()) {
        pending = ''
      }
      flushText(pending)
      pending = ''
      fragments.push(emoteFragment(word, emote))
    } else {
      const split = splitZeroWidthSuffix(emoteMap, word)
      if (split) {
        flushText(pending)
        pending = ''
        fragments.push(emoteFragment(split.base, emoteMap.get(split.base)!))
        for (const overlay of split.overlays) {
          fragments.push(emoteFragment(overlay.name, overlay.emote))
        }
      } else {
        pending += word
      }
    }
    index = end
  }

  flushText(pending)
  return fragments.length ? fragments : [{ t: 'text', c: text }]
}

function smooth(prev: number | null, next: number | null) {
  if (next === null) return prev
  return prev === null ? next : Math.round(prev * 0.75 + next * 0.25)
}

function average(values: number[]) {
  return values.length ? Math.round(values.reduce((sum, v) => sum + v, 0) / values.length) : null
}

function resetLatencyWindows() {
  Object.values(latencyWindows).forEach(values => values.splice(0, values.length))
}

function pushLatency(kind: keyof typeof latencyWindows, values: number[]) {
  if (!values.length) return
  const bucket = latencyWindows[kind]
  bucket.push(...values.filter(value => Number.isFinite(value) && value >= 0))
  if (bucket.length > 180) {
    bucket.splice(0, bucket.length - 180)
  }
}

function summarize(values: number[]): LatencySummary {
  if (!values.length) return emptyLatencySummary
  const sorted = [...values].sort((a, b) => a - b)
  const pick = (q: number) => sorted[Math.min(sorted.length - 1, Math.max(0, Math.ceil(sorted.length * q) - 1))]
  return {
    last: Math.round(values[values.length - 1]),
    avg: average(values),
    p50: Math.round(pick(0.5)),
    p95: Math.round(pick(0.95)),
    max: Math.round(sorted[sorted.length - 1]),
    samples: values.length,
  }
}

function currentLatencyStats(): ChatState['latencyStats'] {
  return {
    total: summarize(latencyWindows.total),
    source: summarize(latencyWindows.source),
    relay: summarize(latencyWindows.relay),
    browser: summarize(latencyWindows.browser),
    echo: summarize(latencyWindows.echo),
  }
}

function mergeIncoming(state: ChatState, incoming: Message[], frameServerSentTs?: number): Partial<ChatState> {
  const now = Date.now()
  const messages = [...state.messages]
  let echoLatency = state.lastEchoLatencyMs
  const latencies: number[] = []
  const sourceLatencies: number[] = []
  const relayLatencies: number[] = []

  for (const raw of incoming) {
    const msg: Message = { ...raw, receivedAt: now, source: 'remote' }
    if (Number.isFinite(msg.ts) && msg.ts > 0) {
      latencies.push(Math.max(0, now - msg.ts))
    }
    if (Number.isFinite(msg.server_received_ts) && Number.isFinite(msg.ts) && msg.ts > 0) {
      sourceLatencies.push(Math.max(0, (msg.server_received_ts ?? now) - msg.ts))
    }
    if (Number.isFinite(frameServerSentTs) && Number.isFinite(msg.server_received_ts)) {
      relayLatencies.push(Math.max(0, (frameServerSentTs ?? now) - (msg.server_received_ts ?? now)))
    }

    const text = textOf(msg).trim()
    const idx = messages.findIndex(candidate => {
      if (!candidate.pending || candidate.ackState === 'error') return false
      if (candidate.clientSentTs && now - candidate.clientSentTs > 60_000) return false
      return candidate.user.toLowerCase() === msg.user.toLowerCase() && textOf(candidate).trim() === text
    })

    if (idx >= 0) {
      const pending = messages[idx]
      const latency = pending.clientSentTs ? now - pending.clientSentTs : undefined
      messages[idx] = {
        ...msg,
        clientMsgId: pending.clientMsgId,
        clientSentTs: pending.clientSentTs,
        pending: false,
        ackState: 'live',
        echoLatencyMs: latency,
      }
      if (latency !== undefined) echoLatency = latency
      if (latency !== undefined) pushLatency('echo', [latency])
    } else {
      messages.push(msg)
    }
  }

  const averageLatency = average(latencies)
  const averageSourceLatency = average(sourceLatencies)
  const averageRelayLatency = average(relayLatencies)
  const browserLatency = Number.isFinite(frameServerSentTs) ? Math.max(0, now - (frameServerSentTs ?? now)) : null
  pushLatency('total', latencies)
  pushLatency('source', sourceLatencies)
  pushLatency('relay', relayLatencies)
  if (browserLatency !== null) pushLatency('browser', [browserLatency])

  const nextMessages = trimMessages(messages)
  if (state.activeChannel) {
    persistMessages(state.activeChannel, nextMessages)
  }

  return {
    messages: nextMessages,
    restoredFromCache: false,
    chatLatencyMs: smooth(state.chatLatencyMs, averageLatency),
    sourceLatencyMs: smooth(state.sourceLatencyMs, averageSourceLatency),
    relayLatencyMs: smooth(state.relayLatencyMs, averageRelayLatency),
    browserLatencyMs: smooth(state.browserLatencyMs, browserLatency),
    lastEchoLatencyMs: echoLatency,
    latencyStats: currentLatencyStats(),
  }
}

function applyAck(messages: Message[], clientMsgId: string, state: 'queued' | 'sent'): Message[] {
  return messages.map(msg => msg.clientMsgId === clientMsgId ? { ...msg, ackState: state } : msg)
}

function applyError(messages: Message[], clientMsgId: string, error: string): Message[] {
  return messages.map(msg => msg.clientMsgId === clientMsgId ? { ...msg, pending: false, ackState: 'error' as const, error } : msg)
}

export const useChatStore = create<ChatState>((set, get) => {
  const connect = () => {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return
    set({ connectionState: retryDelay === 1000 ? 'connecting' : 'reconnecting' })
    ws = new WebSocket(CHAT_WS)

    ws.onopen = () => {
      retryDelay = 1000
      set({ connectionState: 'open', lastError: null })
      const channel = get().activeChannel
      if (channel) {
        send({ op: 'subscribe', channel })
      }
    }

    ws.onmessage = (ev: MessageEvent) => {
      const data = JSON.parse(ev.data as string) as {
        type: string
        channel?: string
        state?: string
        message?: string
        messages?: Message[]
        server_sent_ts?: number
        client_msg_id?: string
      }
      if (data.type === 'batch' && data.messages) {
        set(state => mergeIncoming(state, data.messages!, data.server_sent_ts))
      }
      if (data.type === 'status') {
        set({ channelState: data.state ?? 'connected', lastError: null })
      }
      if (data.type === 'error') {
        set({ channelState: 'error', lastError: data.message ?? 'chat error' })
      }
      if (data.type === 'message_ack' && data.client_msg_id) {
        const ack = data.state === 'sent' ? 'sent' : 'queued'
        set(state => {
          const messages = applyAck(state.messages, data.client_msg_id!, ack)
          if (state.activeChannel) {
            persistMessages(state.activeChannel, messages)
          }
          return { messages }
        })
      }
      if (data.type === 'message_error' && data.client_msg_id) {
        set(state => {
          const messages = applyError(state.messages, data.client_msg_id!, data.message ?? 'send failed')
          if (state.activeChannel) {
            persistMessages(state.activeChannel, messages)
          }
          return {
            messages,
            lastError: data.message === 'auth_required' ? 'Connect Twitch to send messages.' : state.lastError,
          }
        })
      }
      if (data.type === 'emote_delta') {
        set(state => ({ emoteRevision: state.emoteRevision + 1 }))
      }
    }

    ws.onclose = () => {
      ws = null
      set({ connectionState: get().activeChannel ? 'reconnecting' : 'closed' })
      if (retryTimer) clearTimeout(retryTimer)
      retryTimer = setTimeout(() => {
        retryDelay = Math.min(retryDelay * 2, 30000)
        connect()
      }, retryDelay)
    }

    ws.onerror = () => {
      set({ lastError: 'chat socket error' })
    }
  }

  connect()

  return {
    messages: [],
    restoredFromCache: false,
    connectionState: 'connecting',
    channelState: 'idle',
    activeChannel: null,
    lastError: null,
    emoteRevision: 0,
    chatLatencyMs: null,
    sourceLatencyMs: null,
    relayLatencyMs: null,
    browserLatencyMs: null,
    lastEchoLatencyMs: null,
    latencyStats: emptyLatencyStats,
    subscribe(channel: string) {
      const prev = get().activeChannel
      if (prev && prev !== channel) {
        send({ op: 'unsubscribe', channel: prev })
      }
      const cachedMessages = loadCachedMessages(channel)
      set({
        activeChannel: channel,
        channelState: 'subscribing',
        messages: cachedMessages,
        restoredFromCache: cachedMessages.length > 0,
        lastError: null,
        chatLatencyMs: null,
        sourceLatencyMs: null,
        relayLatencyMs: null,
        browserLatencyMs: null,
        lastEchoLatencyMs: null,
        latencyStats: emptyLatencyStats,
      })
      resetLatencyWindows()
      connect()
      send({ op: 'subscribe', channel })
    },
    unsubscribe(channel: string) {
      if (get().activeChannel === channel) {
        set({ activeChannel: null, channelState: 'idle', restoredFromCache: false })
      }
      send({ op: 'unsubscribe', channel })
    },
    sendMessage(channel: string, text: string, user?: AuthUser, loadedEmotes?: ChannelEmote[]) {
      const clean = text.trim()
      if (!clean) return
      const now = Date.now()
      const clientMsgId = `local-${now}-${Math.random().toString(36).slice(2)}`
      const pending: Message = {
        id: clientMsgId,
        user: userLabel(user),
        color: '#bf94ff',
        badges: ['self'],
        ts: now,
        fragments: tokenizeClientSide(clean, loadedEmotes),
        source: 'local',
        pending: true,
        clientMsgId,
        clientSentTs: now,
        ackState: 'pending',
      }
      set(state => {
        const messages = trimMessages([...state.messages, pending])
        persistMessages(state.activeChannel ?? channel, messages)
        return { messages, restoredFromCache: false }
      })
      connect()
      send({
        op: 'send_message',
        channel,
        text: clean,
        client_msg_id: clientMsgId,
        client_sent_ts: now,
        user: userLogin(user),
      })
    },
  }
})
