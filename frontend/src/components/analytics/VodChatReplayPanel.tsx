import { useEffect, useRef, useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'

import { ANALYTICS } from '../../config.ts'
import { computeBucketStart, computeBucketEnd } from '../../utils/vodChatReplay.ts'
import { linkifyText } from '../../utils/linkifyText'
import ChatUserMenu, { type ChatUserFilter } from '../chat/ChatUserMenu'

export interface EmoteFrag {
  name: string
  id: string
  provider: string
  imageUrl: string
}

export interface VODChatMessage {
  id: number
  streamId: string
  minuteTs: string
  messageId: string
  displayName: string
  commenterLogin?: string
  senderHash: string
  text: string
  emoteFrags?: EmoteFrag[]
  offsetSeconds: number
  syncedAt: string
}

export interface ChatReplayResponse {
  messages: VODChatMessage[]
  nextCursor: string
  unavailable: boolean
}

export interface VodChatReplayPanelProps {
  streamId: string
  currentOffsetSeconds: number
  isSyncing?: boolean
  onSync?: () => void
  /** Stream has minute rollups but no persisted chat-replay rows yet. */
  needsChatReplayResync?: boolean
  className?: string
}

async function fetchChatReplay(
  streamId: string,
  offsetStart: number,
  offsetEnd: number,
  limit = 200,
  cursor?: string,
): Promise<ChatReplayResponse> {
  const params = new URLSearchParams({
    offsetStart: String(offsetStart),
    offsetEnd: String(offsetEnd),
    limit: String(limit),
  })
  if (cursor) params.set('cursor', cursor)
  const res = await fetch(
    `${ANALYTICS}/v1/analytics/streams/${encodeURIComponent(streamId)}/chat-replay?${params}`,
  )
  if (!res.ok) {
    throw new Error(`Chat replay request failed: ${res.status}`)
  }
  return res.json() as Promise<ChatReplayResponse>
}

function renderMessageText(
  text: string,
  emoteFrags?: EmoteFrag[],
  onFilterUser?: (filter: ChatUserFilter) => void,
): React.ReactNode {
  if (!emoteFrags || emoteFrags.length === 0) {
    return <span>{linkifyText(text, 'vod')}</span>
  }

  const parts: React.ReactNode[] = []
  const words = text.split(' ')
  const emoteMap = new Map<string, EmoteFrag>()
  for (const frag of emoteFrags) {
    emoteMap.set(frag.name, frag)
  }

  for (let i = 0; i < words.length; i++) {
    const word = words[i]
    const emote = emoteMap.get(word)
    if (emote) {
      parts.push(
        <img
          key={`${emote.id}-${i}`}
          src={emote.imageUrl}
          alt={emote.name}
          title={emote.name}
          className="mx-0.5 inline-block h-7 max-w-[2rem] align-middle object-contain"
          loading="lazy"
          decoding="async"
        />,
      )
    } else if (word.startsWith('@') && word.length > 1) {
      const login = word.replace(/^@+/, '').toLowerCase()
      parts.push(
        <ChatUserMenu
          key={`m-${i}`}
          displayName={word.replace(/^@+/, '')}
          login={login}
          onFilterUser={onFilterUser}
          className="font-black text-violet-300 hover:underline"
        >
          {word}
        </ChatUserMenu>,
      )
    } else {
      parts.push(<span key={i}>{linkifyText(word, `w-${i}`)}</span>)
    }
    if (i < words.length - 1) {
      parts.push(<span key={`sp-${i}`}> </span>)
    }
  }

  return <>{parts}</>
}

export function VodChatReplayPanel({
  streamId,
  currentOffsetSeconds,
  isSyncing = false,
  onSync,
  needsChatReplayResync = false,
  className,
}: VodChatReplayPanelProps) {
  const bucketStart = computeBucketStart(currentOffsetSeconds)
  const bucketEnd = computeBucketEnd(bucketStart)
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const [prevBucket, setPrevBucket] = useState(bucketStart)
  const [shouldAutoScroll, setShouldAutoScroll] = useState(true)

  const { data, isLoading, isError } = useQuery({
    queryKey: ['vod-chat-replay', streamId, bucketStart],
    queryFn: () => fetchChatReplay(streamId, bucketStart, bucketEnd),
    enabled: Boolean(streamId),
    staleTime: 30_000,
    refetchOnWindowFocus: false,
  })

  useEffect(() => {
    if (bucketStart !== prevBucket) {
      setPrevBucket(bucketStart)
      setShouldAutoScroll(true)
    }
  }, [bucketStart, prevBucket])

  useEffect(() => {
    if (shouldAutoScroll && scrollRef.current && data?.messages?.length) {
      const el = scrollRef.current
      el.scrollTop = el.scrollHeight
    }
  }, [data?.messages, shouldAutoScroll])

  const handleScroll = useCallback(() => {
    if (!scrollRef.current) return
    const el = scrollRef.current
    const isAtBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 32
    setShouldAutoScroll(isAtBottom)
  }, [])

  if (data?.unavailable) {
    return (
      <div
        className={
          className ??
          'flex flex-col items-center justify-center gap-3 rounded-xl border border-white/10 bg-zinc-950/60 p-6 text-center'
        }
      >
        <svg
          className="h-8 w-8 text-zinc-500"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={1.5}
          aria-hidden
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M8.625 12a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0Zm0 0H8.25m4.125 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0Zm0 0H12m4.125 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0Zm0 0h-.375M21 12c0 4.556-4.03 8.25-9 8.25a9.764 9.764 0 0 1-2.555-.337A5.972 5.972 0 0 1 5.41 20.97a5.969 5.969 0 0 1-.474-.065 4.48 4.48 0 0 0 .978-2.025c.09-.457-.133-.901-.467-1.226C3.93 16.178 3 14.189 3 12c0-4.556 4.03-8.25 9-8.25s9 3.694 9 8.25Z"
          />
        </svg>
        <p className="text-sm font-semibold text-zinc-300">
          Chat replay not available
        </p>
        <p className="text-xs leading-relaxed text-zinc-500">
          {needsChatReplayResync
            ? 'Rollups exist but chat replay was not stored for this sync. Re-sync chat in Analytics to backfill messages.'
            : 'Sync this stream to enable chat replay.'}
        </p>
        <button
          type="button"
          onClick={onSync}
          disabled={isSyncing || !onSync}
          className="mt-1 rounded-md border border-violet-400/30 bg-violet-500/15 px-4 py-2 text-xs font-black text-violet-200 transition hover:bg-violet-500/25 focus:outline-none focus-visible:ring-2 focus-visible:ring-violet-400 disabled:opacity-50"
          aria-label="Sync stream chat"
        >
          {isSyncing ? 'Syncing…' : 'Sync chat'}
        </button>
      </div>
    )
  }

  const messages = data?.messages ?? []
  const hasMessages = messages.length > 0
  const isEmptyMinute = !isLoading && !isError && data && !data.unavailable && messages.length === 0

  return (
    <div
      className={
        className ??
        'flex h-full min-h-0 flex-col overflow-hidden rounded-xl border border-white/10 bg-zinc-950/60'
      }
    >
      <div className="flex shrink-0 items-center justify-between border-b border-white/10 px-3 py-2.5">
        <div>
          <div className="text-sm font-black text-zinc-200">Chat replay</div>
          <div className="text-[11px] font-semibold text-zinc-500">
            Minute bucket · {formatMinuteLabel(bucketStart)}
          </div>
        </div>
        {isSyncing ? (
          <span className="flex items-center gap-1.5 text-[11px] font-semibold text-amber-300">
            <span className="relative flex h-2 w-2">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-amber-400 opacity-75" />
              <span className="relative inline-flex h-2 w-2 rounded-full bg-amber-400" />
            </span>
            Syncing…
          </span>
        ) : null}
      </div>

      <div className="relative min-h-0 flex-1">
        {isLoading ? (
          <div className="flex h-full items-center justify-center py-8">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-zinc-600 border-t-violet-400" />
          </div>
        ) : null}

        {isError ? (
          <div className="flex h-full items-center justify-center px-4 py-8">
            <p className="text-sm text-zinc-500">
              Failed to load chat replay. Retrying…
            </p>
          </div>
        ) : null}

        {isEmptyMinute ? (
          <div className="flex h-full items-center justify-center px-4 py-8">
            <p className="text-sm italic text-zinc-500">
              No messages in this minute
            </p>
          </div>
        ) : null}

        {hasMessages ? (
          <div
            ref={scrollRef}
            onScroll={handleScroll}
            className="h-full overflow-y-auto py-2"
            role="log"
            aria-label="VOD chat messages"
            aria-live="polite"
          >
            {messages.map(msg => (
              <div
                key={msg.id}
                className="chat-row px-3 py-1 text-sm leading-snug transition hover:bg-white/[0.045]"
              >
                <ChatUserMenu
                  displayName={msg.displayName}
                  login={msg.commenterLogin}
                  senderHash={msg.senderHash}
                  color={hashToColor(msg.senderHash)}
                  className="mr-1 font-black hover:underline"
                >
                  {msg.displayName}:
                </ChatUserMenu>
                <span className="break-words text-zinc-200">
                  {renderMessageText(msg.text, msg.emoteFrags)}
                </span>
              </div>
            ))}
          </div>
        ) : null}
      </div>
    </div>
  )
}

function formatMinuteLabel(bucketStart: number): string {
  const mm = Math.floor(bucketStart / 60)
  const hh = Math.floor(mm / 60)
  const remainMm = mm % 60
  const ss = bucketStart % 60
  return `${hh.toString().padStart(2, '0')}:${remainMm.toString().padStart(2, '0')}:${ss.toString().padStart(2, '0')}`
}

function hashToColor(senderHash: string): string {
  const chatColors = [
    '#FF6B6B', '#FF8E53', '#FFC93C', '#6BCB77', '#4D96FF',
    '#9B59B6', '#E91E63', '#00BCD4', '#FF5722', '#8BC34A',
    '#FF9800', '#3F51B5', '#009688', '#F44336', '#2196F3',
  ]
  let hash = 0
  for (let i = 0; i < senderHash.length; i++) {
    hash = (hash * 31 + senderHash.charCodeAt(i)) | 0
  }
  return chatColors[Math.abs(hash) % chatColors.length]
}

export default VodChatReplayPanel
