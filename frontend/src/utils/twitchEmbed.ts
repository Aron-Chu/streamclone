let twitchEmbedScriptPromise: Promise<void> | null = null

export type TwitchPlayerInstance = {
  seek?: (timestamp: number) => void
  getCurrentTime?: () => number
  getDuration?: () => number
  setMuted?: (muted: boolean) => void
  pause?: () => void
  play?: () => void
  getPlaybackStats?: () => Record<string, unknown>
}

declare global {
  interface Window {
    Twitch?: {
      Player: new (target: string, options: Record<string, unknown>) => TwitchPlayerInstance
    }
  }
}

export function loadTwitchEmbedScript(): Promise<void> {
  if (typeof window === 'undefined') return Promise.reject(new Error('browser required'))
  if (window.Twitch?.Player) return Promise.resolve()
  if (twitchEmbedScriptPromise) return twitchEmbedScriptPromise
  twitchEmbedScriptPromise = new Promise((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>('script[data-streamclone-twitch-embed]')
    if (existing) {
      existing.addEventListener('load', () => resolve(), { once: true })
      existing.addEventListener('error', () => reject(new Error('twitch embed script failed')), { once: true })
      return
    }
    const script = document.createElement('script')
    script.src = 'https://player.twitch.tv/js/embed/v1.js'
    script.async = true
    script.dataset.streamcloneTwitchEmbed = '1'
    script.onload = () => resolve()
    script.onerror = () => reject(new Error('twitch embed script failed'))
    document.head.appendChild(script)
  })
  return twitchEmbedScriptPromise
}

export function formatTwitchEmbedTime(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  return `${h}h${m}m${sec}s`
}

/** Twitch clip/player iframes require parent=<host> matching the embedding page. */
export function withTwitchEmbedParent(embedUrl: string): string {
  if (!embedUrl || typeof window === 'undefined') return embedUrl
  try {
    const url = new URL(embedUrl, window.location.origin)
    if (!url.hostname.includes('twitch.tv')) return embedUrl
    const parent = window.location.hostname || 'localhost'
    if (!url.searchParams.getAll('parent').includes(parent)) {
      url.searchParams.append('parent', parent)
    }
    return url.toString()
  } catch {
    return embedUrl
  }
}
