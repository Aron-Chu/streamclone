import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import type { Message } from '../../chatStore'
import ChatUserCard from './ChatUserCard'

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
  recentMessages?: Message[]
  className?: string
  children: ReactNode
}

type MenuPosition = {
  top: number
  left: number
  flipAbove: boolean
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
  recentMessages,
  className,
  children,
}: ChatUserMenuProps) {
  const [open, setOpen] = useState(false)
  const [position, setPosition] = useState<MenuPosition | null>(null)
  const rootRef = useRef<HTMLSpanElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)

  const recomputePosition = useCallback(() => {
    const trigger = triggerRef.current
    const menu = menuRef.current
    if (!trigger) return
    const rect = trigger.getBoundingClientRect()
    const menuHeight = menu?.offsetHeight ?? 360
    const menuWidth = menu?.offsetWidth ?? 320
    const gap = 4
    const spaceBelow = window.innerHeight - rect.bottom - gap
    const spaceAbove = rect.top - gap
    const flipAbove = spaceBelow < menuHeight && spaceAbove > spaceBelow
    const top = flipAbove ? rect.top - menuHeight - gap : rect.bottom + gap
    const left = Math.min(Math.max(8, rect.left), Math.max(8, window.innerWidth - menuWidth - 8))
    setPosition({ top: Math.max(8, top), left, flipAbove })
  }, [])

  useLayoutEffect(() => {
    if (!open) {
      setPosition(null)
      return
    }
    recomputePosition()
  }, [open, recomputePosition, recentMessages?.length])

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
        className="fixed z-[100]"
      >
        <ChatUserCard
          displayName={displayName}
          login={login}
          senderHash={senderHash}
          color={color}
          canMention={canMention}
          onMention={onMention}
          onFilterUser={onFilterUser}
          recentMessages={recentMessages}
          onClose={() => setOpen(false)}
        />
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
