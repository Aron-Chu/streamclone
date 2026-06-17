import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Link } from 'react-router-dom'

export interface ChatUserFilter {
  displayName: string
  login?: string
  senderHash?: string
}

export interface ChatUserMenuProps {
  displayName: string
  login?: string
  senderHash?: string
  color?: string
  canMention?: boolean
  onMention?: (login: string) => void
  onFilterUser?: (filter: ChatUserFilter) => void
  className?: string
  children: ReactNode
}

type MenuPosition = {
  top: number
  left: number
  flipAbove: boolean
}

function effectiveLogin(displayName: string, login?: string): string | undefined {
  const trimmed = login?.trim()
  if (trimmed) return trimmed.toLowerCase()
  const guess = displayName.trim().toLowerCase().replace(/\s+/g, '')
  return guess || undefined
}

function findScrollParent(el: HTMLElement | null): HTMLElement | Window {
  let node = el?.parentElement ?? null
  while (node) {
    const style = window.getComputedStyle(node)
    const scrollable = /(auto|scroll|overlay)/.test(style.overflowY) && node.scrollHeight > node.clientHeight
    if (scrollable) return node
    node = node.parentElement
  }
  return window
}

export default function ChatUserMenu({
  displayName,
  login,
  senderHash,
  color,
  canMention = false,
  onMention,
  onFilterUser,
  className,
  children,
}: ChatUserMenuProps) {
  const [open, setOpen] = useState(false)
  const [position, setPosition] = useState<MenuPosition | null>(null)
  const rootRef = useRef<HTMLSpanElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const resolvedLogin = effectiveLogin(displayName, login)
  const hasLogin = Boolean(resolvedLogin)

  const recomputePosition = useCallback(() => {
    const trigger = triggerRef.current
    const menu = menuRef.current
    if (!trigger) return
    const rect = trigger.getBoundingClientRect()
    const menuHeight = menu?.offsetHeight ?? 240
    const gap = 4
    const spaceBelow = window.innerHeight - rect.bottom - gap
    const spaceAbove = rect.top - gap
    const flipAbove = spaceBelow < menuHeight && spaceAbove > spaceBelow
    const top = flipAbove ? rect.top - menuHeight - gap : rect.bottom + gap
    setPosition({ top: Math.max(8, top), left: Math.max(8, rect.left), flipAbove })
  }, [])

  useLayoutEffect(() => {
    if (!open) {
      setPosition(null)
      return
    }
    recomputePosition()
  }, [open, recomputePosition])

  useEffect(() => {
    if (!open) return
    const scrollParent = findScrollParent(rootRef.current)
    const onScrollOrResize = () => recomputePosition()
    scrollParent.addEventListener('scroll', onScrollOrResize, { passive: true })
    window.addEventListener('resize', onScrollOrResize)
    return () => {
      scrollParent.removeEventListener('scroll', onScrollOrResize)
      window.removeEventListener('resize', onScrollOrResize)
    }
  }, [open, recomputePosition])

  useEffect(() => {
    if (!open) return
    const onPointer = (event: MouseEvent) => {
      const target = event.target as Node
      if (rootRef.current?.contains(target) || menuRef.current?.contains(target)) return
      setOpen(false)
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onPointer)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onPointer)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const copyLabel = hasLogin ? resolvedLogin! : displayName

  const copyLogin = async () => {
    try {
      await navigator.clipboard.writeText(copyLabel)
    } catch {
      return
    }
    setOpen(false)
  }

  const mention = () => {
    if (!canMention || !onMention || !hasLogin) return
    onMention(resolvedLogin!)
    setOpen(false)
  }

  const filterUser = () => {
    if (!onFilterUser) return
    onFilterUser({ displayName, login: resolvedLogin, senderHash })
    setOpen(false)
  }

  const menu = open && position ? (
    <>
      <div
        className="fixed inset-0 z-[99]"
        aria-hidden
        onMouseDown={() => setOpen(false)}
      />
      <div
        ref={menuRef}
        role="menu"
        style={{ top: position.top, left: position.left }}
        className="fixed z-[100] min-w-[12rem] overflow-hidden rounded-lg border border-white/10 bg-[#181820] py-1 text-left text-xs font-semibold text-zinc-200 shadow-2xl shadow-black/60"
      >
        <div className="border-b border-white/5 px-3 py-2">
          <div className="truncate font-black text-zinc-100">{displayName}</div>
          {hasLogin && resolvedLogin !== displayName.toLowerCase() ? (
            <div className="truncate font-mono text-[10px] text-zinc-500">{resolvedLogin}</div>
          ) : null}
        </div>
        {hasLogin ? (
          <Link
            role="menuitem"
            to={`/c/${encodeURIComponent(resolvedLogin!)}`}
            className="block px-3 py-2 transition hover:bg-white/10"
            onClick={() => setOpen(false)}
          >
            Open in Streamclone
          </Link>
        ) : (
          <span className="block px-3 py-2 text-zinc-500">Login unknown</span>
        )}
        {hasLogin ? (
          <>
            <a
              role="menuitem"
              href={`https://www.twitch.tv/${encodeURIComponent(resolvedLogin!)}`}
              target="_blank"
              rel="noopener noreferrer"
              className="block px-3 py-2 transition hover:bg-white/10"
              onClick={() => setOpen(false)}
            >
              Open on Twitch
            </a>
            <a
              role="menuitem"
              href={`https://www.twitch.tv/messages/${encodeURIComponent(resolvedLogin!)}`}
              target="_blank"
              rel="noopener noreferrer"
              className="block px-3 py-2 transition hover:bg-white/10"
              onClick={() => setOpen(false)}
            >
              Message on Twitch
            </a>
          </>
        ) : null}
        <button type="button" role="menuitem" onClick={copyLogin} className="block w-full px-3 py-2 text-left transition hover:bg-white/10">
          Copy {hasLogin ? 'login' : 'name'}
        </button>
        {canMention && hasLogin && onMention ? (
          <button type="button" role="menuitem" onClick={mention} className="block w-full px-3 py-2 text-left transition hover:bg-white/10">
            Mention in chat
          </button>
        ) : null}
        {onFilterUser ? (
          <button type="button" role="menuitem" onClick={filterUser} className="block w-full px-3 py-2 text-left transition hover:bg-white/10">
            Show all from this user
          </button>
        ) : null}
      </div>
    </>
  ) : null

  return (
    <span ref={rootRef} className={`inline ${open ? 'relative z-10' : ''}`}>
      <button
        ref={triggerRef}
        type="button"
        onClick={() => setOpen(value => !value)}
        style={color ? { color } : undefined}
        className={className ?? 'font-black hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-violet-400'}
        aria-haspopup="menu"
        aria-expanded={open}
      >
        {children}
      </button>
      {menu ? createPortal(menu, document.body) : null}
    </span>
  )
}

export function mentionLoginFromFragment(text: string): string | undefined {
  const token = text.trim().replace(/^@+/, '').toLowerCase()
  return token || undefined
}
