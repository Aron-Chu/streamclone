import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { getChannelDetails } from '../../api'
import type { Message } from '../../chatStore'
import {
  messagePlainText,
  normalizeChatUserLogin,
  selectRecentUserMessages,
} from '../../utils/chatUserCard'
import type { ChatUserFilter } from './ChatUserMenu'

export interface ChatUserCardProps {
  displayName: string
  login?: string
  senderHash?: string
  color?: string
  canMention?: boolean
  onMention?: (login: string) => void
  onFilterUser?: (filter: ChatUserFilter) => void
  recentMessages?: Message[]
  onClose: () => void
}

function formatTime(ts: number): string {
  if (!Number.isFinite(ts)) return ''
  return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

export default function ChatUserCard({
  displayName,
  login,
  senderHash,
  color,
  canMention = false,
  onMention,
  onFilterUser,
  recentMessages = [],
  onClose,
}: ChatUserCardProps) {
  const resolvedLogin = normalizeChatUserLogin(displayName, login)
  const hasLogin = Boolean(resolvedLogin)
  const copyLabel = hasLogin ? resolvedLogin! : displayName
  const recent = selectRecentUserMessages(recentMessages, displayName, login)

  const detailsQuery = useQuery({
    queryKey: ['chat-user-card', resolvedLogin],
    queryFn: () => getChannelDetails(resolvedLogin!),
    enabled: hasLogin,
    staleTime: 60_000,
  })

  const profileImage = detailsQuery.data?.profileImage
  const profileDisplayName = detailsQuery.data?.displayName || displayName
  const isLive = detailsQuery.data?.isLive

  const copyLogin = async () => {
    try {
      await navigator.clipboard.writeText(copyLabel)
    } catch {
      return
    }
    onClose()
  }

  const mention = () => {
    if (!canMention || !onMention || !hasLogin) return
    onMention(resolvedLogin!)
    onClose()
  }

  const filterUser = () => {
    if (!onFilterUser) return
    onFilterUser({ displayName, login: resolvedLogin, senderHash })
    onClose()
  }

  return (
    <div
      data-testid="chat-user-card"
      className="w-[min(20rem,calc(100vw-1rem))] overflow-hidden rounded-xl border border-white/10 bg-[#181820] text-left text-xs font-semibold text-zinc-200 shadow-2xl shadow-black/60"
    >
      <div className="border-b border-white/5 bg-[#14141c] px-4 py-4">
        <div className="flex items-start gap-3">
          {profileImage ? (
            <img
              src={profileImage}
              alt={profileDisplayName}
              className="h-14 w-14 shrink-0 rounded-full border border-white/10 object-cover"
              loading="lazy"
            />
          ) : (
            <div
              className="grid h-14 w-14 shrink-0 place-items-center rounded-full border border-white/10 bg-zinc-800 text-lg font-black text-violet-200"
              style={color ? { color } : undefined}
            >
              {profileDisplayName.slice(0, 1).toUpperCase()}
            </div>
          )}
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-black text-zinc-100">{profileDisplayName}</div>
            {hasLogin ? (
              <div className="truncate font-mono text-[11px] text-zinc-500">@{resolvedLogin}</div>
            ) : (
              <div className="text-[11px] text-zinc-500">Login unknown</div>
            )}
            {isLive ? (
              <div className="mt-1 inline-flex items-center gap-1 rounded bg-red-500/15 px-2 py-0.5 text-[10px] font-black uppercase text-red-200">
                <span className="h-1.5 w-1.5 rounded-full bg-red-400" />
                Live
              </div>
            ) : null}
          </div>
        </div>
      </div>

      {recent.length ? (
        <div className="border-b border-white/5 px-4 py-3">
          <div className="mb-2 text-[10px] font-black uppercase tracking-wide text-zinc-500">Recent messages</div>
          <ul className="space-y-2">
            {recent.map(msg => (
              <li key={msg.clientMsgId ?? msg.id} className="rounded border border-white/5 bg-white/[0.03] px-2 py-1.5">
                <div className="mb-0.5 text-[10px] font-bold text-zinc-500">{formatTime(msg.ts)}</div>
                <div className="line-clamp-2 text-[11px] leading-4 text-zinc-300">{messagePlainText(msg) || '…'}</div>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <div className="py-1">
        {hasLogin ? (
          <Link
            role="menuitem"
            to={`/c/${encodeURIComponent(resolvedLogin!)}`}
            className="block px-4 py-2 transition hover:bg-white/10"
            onClick={onClose}
          >
            Open in Streamclone
          </Link>
        ) : null}
        {hasLogin ? (
          <>
            <a
              role="menuitem"
              href={`https://www.twitch.tv/${encodeURIComponent(resolvedLogin!)}`}
              target="_blank"
              rel="noopener noreferrer"
              className="block px-4 py-2 transition hover:bg-white/10"
              onClick={onClose}
            >
              Open on Twitch
            </a>
            <a
              role="menuitem"
              href={`https://www.twitch.tv/messages/${encodeURIComponent(resolvedLogin!)}`}
              target="_blank"
              rel="noopener noreferrer"
              className="block px-4 py-2 transition hover:bg-white/10"
              onClick={onClose}
            >
              Message on Twitch
            </a>
          </>
        ) : null}
        <button type="button" role="menuitem" onClick={copyLogin} className="block w-full px-4 py-2 text-left transition hover:bg-white/10">
          Copy {hasLogin ? 'login' : 'name'}
        </button>
        {canMention && hasLogin && onMention ? (
          <button type="button" role="menuitem" onClick={mention} className="block w-full px-4 py-2 text-left transition hover:bg-white/10">
            Mention in chat
          </button>
        ) : null}
        {onFilterUser ? (
          <button type="button" role="menuitem" onClick={filterUser} className="block w-full px-4 py-2 text-left transition hover:bg-white/10">
            Show all from this user
          </button>
        ) : null}
      </div>
    </div>
  )
}
