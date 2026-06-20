import {
  createContext,
  useCallback,
  useContext,
  useState,
  type ImgHTMLAttributes,
  type ReactNode,
} from 'react'

import {
  formatEmoteStackTooltipLabel,
  formatEmoteTooltipLabel,
} from '../../utils/emoteTooltip'

type TooltipState = { label: string; x: number; y: number } | null

type ChatEmoteTooltipContextValue = {
  showTooltip: (label: string, anchor: HTMLElement) => void
  hideTooltip: () => void
}

const ChatEmoteTooltipContext = createContext<ChatEmoteTooltipContextValue | null>(null)

export function ChatEmoteTooltipProvider({ children }: { children: ReactNode }) {
  const [tooltip, setTooltip] = useState<TooltipState>(null)

  const showTooltip = useCallback((label: string, anchor: HTMLElement) => {
    const rect = anchor.getBoundingClientRect()
    setTooltip({
      label,
      x: rect.left + rect.width / 2,
      y: rect.top,
    })
  }, [])

  const hideTooltip = useCallback(() => setTooltip(null), [])

  return (
    <ChatEmoteTooltipContext.Provider value={{ showTooltip, hideTooltip }}>
      {children}
      {tooltip ? (
        <div
          role="tooltip"
          className="pointer-events-none fixed z-[9999] -translate-x-1/2 -translate-y-full rounded border border-white/15 bg-zinc-950/95 px-2.5 py-1 text-[11px] font-semibold text-white shadow-lg shadow-black/50 backdrop-blur-sm"
          style={{ left: tooltip.x, top: tooltip.y - 6 }}
        >
          {tooltip.label}
        </div>
      ) : null}
    </ChatEmoteTooltipContext.Provider>
  )
}

function useChatEmoteTooltip() {
  return useContext(ChatEmoteTooltipContext)
}

type ChatEmoteImageProps = Omit<ImgHTMLAttributes<HTMLImageElement>, 'alt' | 'title'> & {
  name?: string
  provider?: string
  fallbackId?: string
  alt?: string
  disableFloatingTooltip?: boolean
}

export function ChatEmoteImage({
  name,
  provider,
  fallbackId,
  alt,
  disableFloatingTooltip = false,
  onMouseEnter,
  onMouseLeave,
  onFocus,
  onBlur,
  ...props
}: ChatEmoteImageProps) {
  const tooltip = useChatEmoteTooltip()
  const label = formatEmoteTooltipLabel(name, provider, fallbackId)

  return (
    <img
      {...props}
      alt={alt ?? name ?? 'emote'}
      title={label}
      aria-label={label}
      onMouseEnter={event => {
        if (!disableFloatingTooltip) {
          tooltip?.showTooltip(label, event.currentTarget)
        }
        onMouseEnter?.(event)
      }}
      onMouseLeave={event => {
        if (!disableFloatingTooltip) {
          tooltip?.hideTooltip()
        }
        onMouseLeave?.(event)
      }}
      onFocus={event => {
        if (!disableFloatingTooltip) {
          tooltip?.showTooltip(label, event.currentTarget)
        }
        onFocus?.(event)
      }}
      onBlur={event => {
        if (!disableFloatingTooltip) {
          tooltip?.hideTooltip()
        }
        onBlur?.(event)
      }}
    />
  )
}

type EmoteStackProps = {
  baseName: string
  baseUrl: string
  baseProvider?: string
  baseId?: string
  overlays: Array<{ name: string; url: string; provider?: string; id?: string }>
}

export function ChatEmoteStack({
  baseName,
  baseUrl,
  baseProvider,
  baseId,
  overlays,
}: EmoteStackProps) {
  const tooltip = useChatEmoteTooltip()
  const label = formatEmoteStackTooltipLabel(baseName, baseProvider, overlays)

  return (
    <span
      className="relative inline-block align-middle"
      style={{ height: '1.65em', lineHeight: 0 }}
      title={label}
      aria-label={label}
      onMouseEnter={event => tooltip?.showTooltip(label, event.currentTarget)}
      onMouseLeave={() => tooltip?.hideTooltip()}
    >
      <ChatEmoteImage
        src={baseUrl}
        name={baseName}
        provider={baseProvider}
        fallbackId={baseId}
        disableFloatingTooltip
        className="inline-block h-full w-auto max-w-none align-middle drop-shadow"
        decoding="async"
        loading="lazy"
      />
      {overlays.map((overlay, index) => (
        <ChatEmoteImage
          key={`${overlay.name}-${index}`}
          src={overlay.url}
          name={overlay.name}
          provider={overlay.provider}
          fallbackId={overlay.id}
          disableFloatingTooltip
          className="pointer-events-none absolute inset-0 z-10 h-full w-full object-contain drop-shadow"
          decoding="async"
          loading="lazy"
        />
      ))}
    </span>
  )
}
