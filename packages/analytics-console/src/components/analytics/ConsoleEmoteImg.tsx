import { useState } from 'react'

function emoteInitial(name: string): string {
  const trimmed = name.trim()
  return trimmed ? trimmed.charAt(0).toUpperCase() : '?'
}

export function ConsoleEmoteImg({
  src,
  name,
  className,
  fallbackClassName,
  width,
  height,
}: {
  src?: string
  name: string
  className?: string
  fallbackClassName?: string
  width?: number
  height?: number
}) {
  const [failed, setFailed] = useState(false)
  if (!src?.trim() || failed) {
    return (
      <span
        className={fallbackClassName ?? 'inline-flex shrink-0 items-center justify-center rounded bg-white/[0.06] text-[10px] font-black text-zinc-500'}
        aria-hidden="true"
      >
        {emoteInitial(name)}
      </span>
    )
  }
  return (
    <img
      src={src}
      alt=""
      aria-hidden
      className={className}
      width={width}
      height={height}
      loading="lazy"
      decoding="async"
      referrerPolicy="no-referrer"
      onError={() => setFailed(true)}
    />
  )
}
