import { useEffect, useState } from 'react'
import {
  fetchPulseWireCommunity,
  PulseWireApiError,
  type PulseWireCommunityPost,
  type PulseWireCommunitySort,
  type PulseWireWindow,
} from '../../../pulseWireApi'
import { windowShortLabel } from '../../../utils/pulseWireFormat'
import CommunityPostCard from './CommunityPostCard'

type Props = {
  window: PulseWireWindow
  sort: PulseWireCommunitySort
  category?: string
  refreshKey?: number
  className?: string
}

export default function CommunityFeed({ window, sort, category, refreshKey = 0, className = '' }: Props) {
  const [items, setItems] = useState<PulseWireCommunityPost[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError('')
    fetchPulseWireCommunity({ window, sort, category, limit: 30 })
      .then(res => {
        if (!cancelled) setItems(res.items ?? [])
      })
      .catch(e => {
        if (cancelled) return
        if (e instanceof PulseWireApiError && e.code === 'pulse_wire_disabled') {
          setError(e.hint ?? 'Pulse Wire is disabled on this install.')
          return
        }
        setError(e instanceof Error ? e.message : 'Community feed unavailable')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [window, sort, category, refreshKey])

  if (loading) {
    return (
      <div className={`space-y-3 animate-pulse ${className}`}>
        <div className="h-28 rounded-[14px] border border-[#2A2A2E] bg-[#121217]" />
        <div className="h-28 rounded-[14px] border border-[#2A2A2E] bg-[#121217]" />
      </div>
    )
  }

  if (error) {
    return (
      <div className={`rounded-2xl border border-amber-400/30 bg-amber-500/10 p-4 text-sm text-amber-100 ${className}`}>
        {error}
      </div>
    )
  }

  if (!items.length) {
    const emptyHint = category === 'bans'
      ? 'Ban headlines appear after StreamerBans ingest runs — set STREAMERBANS_INGEST_ENABLED=true if this stays empty.'
      : category === 'funny' || category === 'drama'
        ? `No ${category}-tagged threads in ${windowShortLabel(window)} — clips and Wire stories above may still match the moment.`
        : category
          ? `No ${category} threads in ${windowShortLabel(window)} yet.`
          : `No Reddit threads in ${windowShortLabel(window)} yet.`
    return (
      <div className={`rounded-2xl border border-[#2A2A2E] bg-[#121217] p-5 text-sm text-[#ADADB8] ${className}`}>
        <p>{emptyHint}</p>
        {!category ? (
          <p className="mt-2 text-xs text-[#7A7A85]">
            LSF and r/Twitch posts appear here as ingest runs — check Reddit/scraper health if this stays empty.
          </p>
        ) : null}
      </div>
    )
  }

  return (
    <div className={`space-y-3 ${className}`}>
      {items.map(post => (
        <CommunityPostCard key={post.id} post={post} />
      ))}
    </div>
  )
}
