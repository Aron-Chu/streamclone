import type { ReactNode } from 'react'
import { resolveSocialLinkMeta, type SocialPlatform } from '../../utils/socialLinkMeta'

function IconTwitch() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden className="h-4 w-4 shrink-0" fill="currentColor">
      <path d="M4 3l-2 4v12h5v4l4-4h4l7-7V3H4zm15 10-3 3h-4l-3 3v-3H6V5h13v8z" />
      <path d="M14 7h2v5h-2V7zm-4 0h2v5h-2V7z" />
    </svg>
  )
}

function IconDiscord() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden className="h-4 w-4 shrink-0" fill="currentColor">
      <path d="M18.9 5.5A16.4 16.4 0 0 0 14.7 4a12 12 0 0 0-.5 1 8.6 8.6 0 0 0-3.8 0A11.8 11.8 0 0 0 9.9 4 16.2 16.2 0 0 0 5.7 5.5 17.2 17.2 0 0 0 2 16.4a16.5 16.5 0 0 0 5 2.5 12.4 12.4 0 0 0 1.1-1.8 10.7 10.7 0 0 1-1.7-.8l.4-.3a12 12 0 0 0 10.2 0l.4.3a10.5 10.5 0 0 1-1.7.8 12.4 12.4 0 0 0 1.1 1.8 16.5 16.5 0 0 0 5-2.5 17.1 17.1 0 0 0-3.7-10.9zM9.7 14.2c-.9 0-1.6-.8-1.6-1.8s.7-1.8 1.6-1.8 1.6.8 1.6 1.8-.7 1.8-1.6 1.8zm4.6 0c-.9 0-1.6-.8-1.6-1.8s.7-1.8 1.6-1.8 1.6.8 1.6 1.8-.7 1.8-1.6 1.8z" />
    </svg>
  )
}

function IconX() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden className="h-4 w-4 shrink-0" fill="currentColor">
      <path d="M16.6 3h3.1l-6.8 7.8L21 21h-6.2l-4.9-6.4L4.2 21H1.1l7.3-8.3L3 3h6.3l4.4 5.8L16.6 3zm-1.1 16.2h1.7L7.8 4.8H6l9.5 14.4z" />
    </svg>
  )
}

function IconYouTube() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden className="h-4 w-4 shrink-0" fill="currentColor">
      <path d="M21.6 7.2a2.8 2.8 0 0 0-2-2C17.8 4.6 12 4.6 12 4.6s-5.8 0-7.6.6a2.8 2.8 0 0 0-2 2A29.4 29.4 0 0 0 2 12a29.4 29.4 0 0 0 .4 4.8 2.8 2.8 0 0 0 2 2c1.8.6 7.6.6 7.6.6s5.8 0 7.6-.6a2.8 2.8 0 0 0 2-2 29.4 29.4 0 0 0 .4-4.8 29.4 29.4 0 0 0-.4-4.8zM10 15.5v-7l6 3.5-6 3.5z" />
    </svg>
  )
}

function IconInstagram() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden className="h-4 w-4 shrink-0" fill="currentColor">
      <path d="M7 2h10a5 5 0 0 1 5 5v10a5 5 0 0 1-5 5H7a5 5 0 0 1-5-5V7a5 5 0 0 1 5-5zm0 2a3 3 0 0 0-3 3v10a3 3 0 0 0 3 3h10a3 3 0 0 0 3-3V7a3 3 0 0 0-3-3H7zm5 3.5A5.5 5.5 0 1 1 6.5 13 5.5 5.5 0 0 1 12 7.5zm0 2A3.5 3.5 0 1 0 15.5 13 3.5 3.5 0 0 0 12 9.5zM17.8 6.7a1.1 1.1 0 1 1-1.1 1.1 1.1 1.1 0 0 1 1.1-1.1z" />
    </svg>
  )
}

function IconTikTok() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden className="h-4 w-4 shrink-0" fill="currentColor">
      <path d="M16.5 3c.4 2.2 1.8 3.9 4 4.5v3.4c-1.5 0-2.9-.5-4-1.3v6.8a6.8 6.8 0 1 1-6.8-6.8c.3 0 .7 0 1 .1v3.6a3.2 3.2 0 1 0 2.3 3.1V3h3.5z" />
    </svg>
  )
}

function IconKick() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden className="h-4 w-4 shrink-0" fill="currentColor">
      <path d="M4 4h6l2 3 2-3h6v16h-6l-2-3-2 3H4V4zm10 3.5-1.5 2.3L15 12l-2.5 2.2L14 16.5 18 12l-4-4.5z" />
    </svg>
  )
}

function IconLink() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden className="h-4 w-4 shrink-0" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="M10 14a3.5 3.5 0 0 0 5 0l2-2a3.5 3.5 0 0 0-5-5l-1 1" />
      <path d="M14 10a3.5 3.5 0 0 0-5 0l-2 2a3.5 3.5 0 0 0 5 5l1-1" />
    </svg>
  )
}

function IconExternal() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden className="h-3 w-3 shrink-0 opacity-70" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="M14 5h5v5" />
      <path d="M10 14 19 5" />
      <path d="M19 14v5H5V5h5" />
    </svg>
  )
}

const ICONS: Record<SocialPlatform, () => ReactNode> = {
  twitch: IconTwitch,
  discord: IconDiscord,
  x: IconX,
  youtube: IconYouTube,
  instagram: IconInstagram,
  tiktok: IconTikTok,
  kick: IconKick,
  generic: IconLink,
}

export interface SocialLinkChipProps {
  url: string
  title?: string
}

export default function SocialLinkChip({ url, title }: SocialLinkChipProps) {
  const meta = resolveSocialLinkMeta(url, title)
  const Icon = ICONS[meta.platform]
  const tooltip = title?.trim() || url

  return (
    <a
      href={url}
      target="_blank"
      rel="noreferrer"
      title={tooltip}
      className="group inline-flex max-w-full items-center gap-2 rounded-full border border-white/10 bg-white/[0.04] px-3 py-1.5 text-sm font-semibold text-[#bf94ff] transition hover:border-violet-400/50 hover:bg-violet-500/10 hover:text-[#d8b7ff] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-violet-400"
    >
      <Icon />
      <span className="truncate">{meta.label}</span>
      <IconExternal />
    </a>
  )
}
