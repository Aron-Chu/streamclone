import { useMemo } from 'react'
import type { ChannelEmote, EmoteProvider } from '../../api'
import { normalizeBrowserOriginUrl } from '../../config'

const PROVIDER_ROWS: { id: EmoteProvider; label: string }[] = [
  { id: 'seventv', label: '7TV' },
  { id: 'twitch', label: 'Twitch' },
  { id: 'ffz', label: 'FFZ' },
]

interface EmoteScrollPickerProps {
  emotes: ChannelEmote[]
  onPick: (emoteName: string) => void
}

export function EmoteScrollPicker({ emotes, onPick }: EmoteScrollPickerProps) {
  const grouped = useMemo(() => {
    const buckets: Record<EmoteProvider, ChannelEmote[]> = {
      seventv: [],
      twitch: [],
      ffz: [],
    }
    for (const emote of emotes) {
      const provider = emote.provider
      if (provider === 'seventv' || provider === 'twitch' || provider === 'ffz') {
        buckets[provider].push(emote)
      }
    }
    for (const key of Object.keys(buckets) as EmoteProvider[]) {
      buckets[key].sort((a, b) => a.name.localeCompare(b.name))
    }
    return buckets
  }, [emotes])

  if (emotes.length === 0) {
    return (
      <p className="clip-studio-caption-hint emote-scroll-empty">
        Channel emotes loading — names typed in captions still preview when available.
      </p>
    )
  }

  return (
    <div className="emote-scroll-picker">
      {PROVIDER_ROWS.map(row => {
        const items = grouped[row.id]
        if (items.length === 0) return null
        return (
          <div key={row.id} className="emote-scroll-row">
            <span className="emote-scroll-row-label">{row.label}</span>
            <div className="emote-scroll-track" role="list">
              {items.map(emote => (
                <button
                  key={`${row.id}-${emote.emote_id}-${emote.name}`}
                  type="button"
                  className="emote-scroll-chip"
                  title={emote.name}
                  onClick={() => onPick(emote.name)}
                >
                  <img
                    src={normalizeBrowserOriginUrl(emote.url, ['/emotes/'])}
                    alt={emote.name}
                    loading="lazy"
                  />
                </button>
              ))}
            </div>
          </div>
        )
      })}
    </div>
  )
}
