import { memo, type ReactNode } from 'react'
import type { ChatBadge } from '../../api'
import type { Fragment, Message } from '../../chatStore'
import { normalizeBrowserOriginUrl } from '../../config'
import { linkifyText } from '../../utils/linkifyText'
import { resolveEmoteImageUrl } from '../../utils/emoteImageUrl'
import ChatUserMenu, { mentionLoginFromFragment, type ChatUserFilter } from './ChatUserMenu'
import { ChatEmoteImage, ChatEmoteStack } from './ChatEmoteTooltipLayer'

function normalizeMentionToken(value: string) {
  return value.trim().replace(/^@+/, '').toLowerCase()
}

interface FragProps {
  f: Fragment
  selfMention?: boolean
  mentionColor?: string
  canMention?: boolean
  onMention?: (login: string) => void
  onFilterUser?: (filter: ChatUserFilter) => void
  recentMessages?: Message[]
  fragKey: string
}

function resolveFragmentEmoteUrl(f: Fragment): string {
  return resolveEmoteImageUrl({
    provider: f.provider,
    id: f.id,
    imageUrl: f.u,
    scale: '1x',
  })
}

function Frag({ f, selfMention, mentionColor, canMention, onMention, onFilterUser, recentMessages, fragKey }: FragProps) {
  if (f.t === 'emote') {
    return (
      <ChatEmoteImage
        src={normalizeBrowserOriginUrl(resolveFragmentEmoteUrl(f), ['/emotes/'])}
        name={f.c}
        provider={f.provider}
        fallbackId={f.id}
        className="inline-block align-middle drop-shadow"
        style={{ height: '1.65em', width: f.zw ? '1.65em' : undefined }}
        decoding="async"
      />
    )
  }
  if (f.t === 'mention') {
    const mentionLogin = mentionLoginFromFragment(f.c)
    const body = selfMention ? (
      <span className="whitespace-pre-wrap break-words rounded bg-violet-400/15 px-1 py-0.5 font-black text-violet-100">{f.c}</span>
    ) : (
      <span style={{ color: mentionColor || '#c4b5fd' }} className="whitespace-pre-wrap break-words font-black">{f.c}</span>
    )
    return (
      <ChatUserMenu
        displayName={f.c.replace(/^@+/, '')}
        login={mentionLogin}
        color={mentionColor}
        canMention={canMention}
        onMention={onMention}
        onFilterUser={onFilterUser}
        recentMessages={recentMessages}
        className="whitespace-pre-wrap break-words font-black hover:underline"
      >
        {body}
      </ChatUserMenu>
    )
  }
  return <span className="whitespace-pre-wrap break-words">{linkifyText(f.c, fragKey)}</span>
}

function badgeText(raw: string) {
  const [set, version] = raw.split('/')
  if (!set) return raw
  return version ? `${set} ${version}` : set
}

function BadgeStrip({ rawBadges, badges }: { rawBadges: string[]; badges: Record<string, ChatBadge> }) {
  const visible = rawBadges.filter(Boolean)
  if (!visible.length) return null
  return (
    <span className="mr-1 inline-flex translate-y-[2px] items-center gap-0.5 align-baseline">
      {visible.slice(0, 5).map(raw => {
        const badge = badges[raw]
        const title = badge?.title || badgeText(raw)
        const src = badge?.imageUrl1x || badge?.imageUrl2x || badge?.imageUrl4x
        if (src) {
          return <img key={raw} src={src} alt={title} title={title} className="h-4 w-4 rounded-sm object-contain" loading="lazy" />
        }
        return (
          <span key={raw} title={title} className="rounded bg-white/10 px-1 py-0.5 text-[9px] font-black uppercase leading-none text-zinc-300">
            {badgeText(raw).slice(0, 4)}
          </span>
        )
      })}
    </span>
  )
}

export function renderMessageFragments(
  fragments: Fragment[],
  mentionNames: Set<string>,
  options?: {
    mentionColor?: string
    canMention?: boolean
    onMention?: (login: string) => void
    onFilterUser?: (filter: ChatUserFilter) => void
    recentMessages?: Message[]
  },
) {
  const nodes: ReactNode[] = []
  let index = 0
  while (index < fragments.length) {
    const fragment = fragments[index]
    if (fragment.t === 'emote' && !fragment.zw) {
      const overlays: Fragment[] = []
      let next = index + 1
      while (next < fragments.length && fragments[next].t === 'emote' && fragments[next].zw) {
        overlays.push(fragments[next])
        next++
      }
      nodes.push(
        <ChatEmoteStack
          key={`stack-${index}-${fragment.c}`}
          baseName={fragment.c}
          baseUrl={normalizeBrowserOriginUrl(resolveFragmentEmoteUrl(fragment), ['/emotes/'])}
          baseProvider={fragment.provider}
          baseId={fragment.id}
          overlays={overlays.map(overlay => ({
            name: overlay.c,
            url: normalizeBrowserOriginUrl(resolveFragmentEmoteUrl(overlay), ['/emotes/']),
            provider: overlay.provider,
            id: overlay.id,
          }))}
        />,
      )
      index = next
      continue
    }
    nodes.push(
      <Frag
        key={`frag-${index}-${fragment.c}`}
        fragKey={`frag-${index}`}
        f={fragment}
        selfMention={fragment.t === 'mention' ? mentionNames.has(normalizeMentionToken(fragment.c)) : false}
        mentionColor={options?.mentionColor}
        canMention={options?.canMention}
        onMention={options?.onMention}
        onFilterUser={options?.onFilterUser}
        recentMessages={options?.recentMessages}
      />,
    )
    index++
  }
  return nodes
}

export interface ChatLogRowProps {
  msg: Message
  badges?: Record<string, ChatBadge>
  mentionNames?: Set<string>
  canMention?: boolean
  onMention?: (login: string) => void
  onFilterUser?: (filter: ChatUserFilter) => void
  recentMessages?: Message[]
  showAck?: boolean
}

function formatMs(value: number | null | undefined) {
  if (value === null || value === undefined) return '-'
  if (value < 1000) return `${value}ms`
  return `${(value / 1000).toFixed(1)}s`
}

function moderationSuffix(msg: Message): string | null {
  if (msg.moderation === 'timeout') {
    return msg.moderationDurationSec ? `Timed out (${msg.moderationDurationSec}s)` : 'Timed out'
  }
  if (msg.moderation === 'ban') return 'Banned'
  return null
}

export const ChatLogRow = memo(function ChatLogRow({
  msg,
  badges = {},
  mentionNames = new Set<string>(),
  canMention = false,
  onMention,
  onFilterUser,
  recentMessages,
  showAck = true,
}: ChatLogRowProps) {
  if (msg.kind === 'notice') {
    return (
      <div className="chat-row px-3 py-1 text-center text-sm leading-snug text-zinc-400 transition hover:bg-white/[0.03]">
        <span className="text-[11px] font-semibold">{msg.modText ?? msg.fragments.map(f => f.c).join('')}</span>
      </div>
    )
  }

  if (msg.kind === 'mod_event') {
    return (
      <div className="chat-row px-3 py-1 text-sm leading-snug text-amber-200/90 transition hover:bg-white/[0.03]">
        <span className="mr-2 inline-block w-10 align-baseline text-[11px] font-semibold text-amber-400/80">
          {Number.isFinite(msg.ts) ? new Date(msg.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : ''}
        </span>
        <span className="italic">{msg.modText ?? msg.user}</span>
      </div>
    )
  }

  if (msg.deleted || msg.moderation === 'deleted') {
    return (
      <div className="chat-row px-3 py-1 text-sm leading-snug text-zinc-600 italic transition hover:bg-white/[0.03]">
        <span className="mr-2 inline-block w-10 align-baseline text-[11px] font-semibold">{Number.isFinite(msg.ts) ? new Date(msg.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : ''}</span>
        <span>Message deleted</span>
      </div>
    )
  }

  const isModerated = msg.moderation === 'timeout' || msg.moderation === 'ban'
  const suffix = isModerated ? moderationSuffix(msg) : null
  const time = Number.isFinite(msg.ts) ? new Date(msg.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : ''
  const tone = msg.ackState === 'error'
    ? 'text-red-300'
    : msg.pending
      ? 'text-zinc-500'
      : 'text-zinc-600'

  return (
    <div className={`chat-row px-3 py-1 text-sm leading-snug transition hover:bg-white/[0.045] ${msg.pending ? 'opacity-70' : ''} ${isModerated ? 'opacity-45 line-through decoration-zinc-500/70' : ''}`}>
      <span className={`mr-2 inline-block w-10 align-baseline text-[11px] font-semibold ${tone}`}>{time}</span>
      <BadgeStrip rawBadges={msg.badges ?? []} badges={badges} />
      <ChatUserMenu
        displayName={msg.user || 'viewer'}
        login={msg.login}
        color={msg.color || '#c4b5fd'}
        canMention={canMention}
        onMention={onMention}
        onFilterUser={onFilterUser}
        recentMessages={recentMessages}
        className="mr-1 font-black hover:underline"
      >
        {msg.user || 'viewer'}:
      </ChatUserMenu>
      <span className="align-baseline">
        {renderMessageFragments(msg.fragments, mentionNames, {
          mentionColor: msg.color,
          canMention,
          onMention,
          onFilterUser,
          recentMessages,
        })}
      </span>
      {suffix ? (
        <span className="ml-2 align-baseline text-[11px] font-semibold not-italic no-underline text-zinc-500">
          {suffix}
        </span>
      ) : null}
      {showAck && msg.ackState && msg.source === 'local' ? (
        <span className={`ml-2 align-baseline text-[11px] font-bold ${msg.ackState === 'error' ? 'text-red-300' : msg.ackState === 'live' ? 'text-emerald-300' : 'text-zinc-500'}`}>
          {msg.ackState === 'live' ? `live ${formatMs(msg.echoLatencyMs)}` : msg.error ?? msg.ackState}
        </span>
      ) : null}
    </div>
  )
})

export default ChatLogRow
