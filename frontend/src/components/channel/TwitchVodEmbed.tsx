import { useEffect, useId, useRef } from 'react'

import { formatTwitchEmbedTime, loadTwitchEmbedScript, type TwitchPlayerInstance } from '../../utils/twitchEmbed'

export interface TwitchVodPlayerHandle {
  seek: (seconds: number) => void
  getCurrentTime: () => number | null
  getDuration: () => number | null
  setMuted: (muted: boolean) => void
}

export interface TwitchVodEmbedProps {
  vodId: string
  offsetSeconds: number
  muted: boolean
  onReady?: () => void
  onError?: (message: string) => void
  playerRef?: React.MutableRefObject<TwitchVodPlayerHandle | null>
  className?: string
}

export function TwitchVodEmbed({
  vodId,
  offsetSeconds,
  muted,
  onReady,
  onError,
  playerRef,
  className,
}: TwitchVodEmbedProps) {
  const reactId = useId()
  const containerId = `streamclone-vod-embed-${vodId}-${reactId.replace(/:/g, '')}`
  const playerInstanceRef = useRef<TwitchPlayerInstance | null>(null)

  useEffect(() => {
    let alive = true
    let player: TwitchPlayerInstance | null = null

    loadTwitchEmbedScript()
      .then(() => {
        if (!alive) return
        if (!window.Twitch?.Player) throw new Error('Twitch embed player unavailable')
        player = new window.Twitch.Player(containerId, {
          video: vodId,
          muted,
          autoplay: true,
          width: '100%',
          height: '100%',
          parent: [window.location.hostname],
          time: formatTwitchEmbedTime(offsetSeconds),
        })
        playerInstanceRef.current = player
        if (playerRef) {
          playerRef.current = {
            seek: (seconds: number) => {
              player?.seek?.(Math.max(0, Math.floor(seconds)))
            },
            getCurrentTime: () => {
              const value = player?.getCurrentTime?.()
              return Number.isFinite(value) ? Number(value) : null
            },
            getDuration: () => {
              const value = player?.getDuration?.()
              return Number.isFinite(value) ? Number(value) : null
            },
            setMuted: (nextMuted: boolean) => {
              player?.setMuted?.(nextMuted)
            },
          }
        }
        player.setMuted?.(muted)
        onReady?.()
      })
      .catch(err => {
        if (!alive) return
        onError?.((err as Error).message || 'Twitch embed failed')
      })

    return () => {
      alive = false
      player?.pause?.()
      playerInstanceRef.current = null
      if (playerRef) playerRef.current = null
    }
  }, [containerId, muted, offsetSeconds, onError, onReady, playerRef, vodId])

  useEffect(() => {
    playerInstanceRef.current?.setMuted?.(muted)
  }, [muted])

  return (
    <div
      id={containerId}
      className={className ?? 'absolute inset-0 z-[2] h-full w-full bg-black'}
      aria-label={`Twitch VOD ${vodId}`}
    />
  )
}

export default TwitchVodEmbed
