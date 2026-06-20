import { useEffect, useState } from 'react'
import {
  fetchTopClips,
  type PulseWireTopClip,
  type PulseWireWindow,
} from '../../../pulseWireApi'
import { formatCompactCount } from '../../../utils/pulseWireFormat'
import ClipThumbnail from './ClipThumbnail'

function formatDuration(seconds?: number) {
  if (seconds == null || !Number.isFinite(seconds)) return null
  const total = Math.max(0, Math.floor(seconds))
  const mm = Math.floor(total / 60)
  const ss = total % 60
  return `${mm}:${ss.toString().padStart(2, '0')}`
}

type Props = {
  window: PulseWireWindow
  refreshKey?: number
  expanded?: boolean
  className?: string
}

export default function TopClipsStrip({ window, refreshKey = 0, expanded = false, className = '' }: Props) {
  const [items, setItems] = useState<PulseWireTopClip[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    fetchTopClips({ window, limit: expanded ? 24 : 12 })
      .then(res => { if (!cancelled) setItems(res.items ?? []) })
      .catch(() => { if (!cancelled) setItems([]) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [window, refreshKey, expanded])

  if (loading) {
    return (
      <div className={`grid gap-3 ${expanded ? 'sm:grid-cols-2 lg:grid-cols-3' : 'grid-cols-2'} ${className}`}>
        {[0, 1, 2].map(key => (
          <div key={key} className="aspect-video animate-pulse rounded-xl border border-[#2A2A2E] bg-[#121217]" />
        ))}
      </div>
    )
  }

  if (!items.length) {
    return (
      <div className={`rounded-xl border border-[#2A2A2E] bg-[#121217] p-4 text-xs text-[#7A7A85] ${className}`}>
        No top clips in this window yet.
      </div>
    )
  }

  return (
    <div className={`grid gap-3 ${expanded ? 'sm:grid-cols-2 lg:grid-cols-3' : 'grid-cols-2'} ${className}`}>
      {items.map(clip => {
        const duration = formatDuration(clip.durationSeconds)
        return (
          <a
            key={clip.id}
            href={clip.url}
            target="_blank"
            rel="noreferrer"
            className="group block overflow-hidden rounded-xl border border-[#2A2A2E] bg-[#121217] transition hover:border-[#A970FF]/40"
          >
            <div className="relative aspect-video bg-[#0C0C0F]">
              <ClipThumbnail
                displayThumbnailUrl={clip.displayThumbnailUrl}
                title={clip.title}
                className="h-full w-full object-cover transition duration-300 group-hover:scale-105"
              />
              {duration ? (
                <div className="absolute bottom-2 right-2 rounded bg-black/75 px-2 py-0.5 text-[11px] font-semibold text-[#EFEFF1]">
                  {duration}
                </div>
              ) : null}
            </div>
            <div className="p-3">
              <div className="line-clamp-2 text-sm font-semibold leading-5 text-[#F7F7F8]">{clip.title}</div>
              <div className="mt-2 text-[11px] font-semibold text-[#7A7A85]">
                {formatCompactCount(clip.viewCount)} views
                {clip.streamerLogin ? ` · ${clip.streamerDisplayName || clip.streamerLogin}` : ''}
              </div>
            </div>
          </a>
        )
      })}
    </div>
  )
}
