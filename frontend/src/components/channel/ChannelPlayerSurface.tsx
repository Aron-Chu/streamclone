import { useEffect, useRef } from 'react'

import type { PlaybackState } from '../../playback'
import { usePlayheadStore } from '../../stores/playheadStore'
import { ChannelPlaybackHost } from './ChannelPlaybackHost'
import { useChannelPlayback } from './channelPlaybackContext'

type ChannelPlayerSurfaceProps = {
  videoRef: React.RefObject<HTMLVideoElement | null>
  playbackOptions: Parameters<typeof ChannelPlaybackHost>[0]['options']
  showTwitchEmbed: boolean
  relayState: PlaybackState
  hlsUrl: string
  seekVodAbsoluteOffset: (absoluteOffset: number) => void
  onCoarsePlaybackStateChange?: (state: PlaybackState) => void
  onFirstFrameMs?: (ms: number) => void
  children: React.ReactNode
}

export default function ChannelPlayerSurface({
  videoRef,
  playbackOptions,
  showTwitchEmbed,
  relayState,
  hlsUrl,
  seekVodAbsoluteOffset,
  onCoarsePlaybackStateChange,
  onFirstFrameMs,
  children,
}: ChannelPlayerSurfaceProps) {
  return (
    <>
      <ChannelPlaybackHost
        videoRef={videoRef}
        options={playbackOptions}
        showTwitchEmbed={showTwitchEmbed}
        relayState={relayState}
        hlsUrl={hlsUrl}
        seekVodAbsoluteOffset={seekVodAbsoluteOffset}
        onCoarsePlaybackStateChange={onCoarsePlaybackStateChange}
        onFirstFrameMs={onFirstFrameMs}
      />
      {children}
    </>
  )
}

/** Subscribes to playback metrics for player chrome only (controls/overlays). */
export function ChannelPlayerChrome({ children }: { children: (ctx: ReturnType<typeof useChannelPlayback>) => React.ReactNode }) {
  const ctx = useChannelPlayback()
  return <>{children(ctx)}</>
}

/** Publishes VOD playhead from playback metrics without re-rendering the channel shell. */
export function ChannelVodPlayheadPublisher({
  enabled,
  streamId,
  vodId,
  vodRelayBaseSeconds,
  showTwitchEmbed,
  relayState,
  vodOffsetSeconds,
  twitchGetTime,
}: {
  enabled: boolean
  streamId: string
  vodId: string
  vodRelayBaseSeconds: number
  showTwitchEmbed: boolean
  relayState: PlaybackState
  vodOffsetSeconds: number
  twitchGetTime: () => number | undefined
}) {
  const { metrics, playbackState, videoRef } = useChannelPlayback()
  useEffect(() => {
    if (!enabled || !streamId) {
      usePlayheadStore.getState().reset()
      return
    }
    const { setPlayhead, setPlaying } = usePlayheadStore.getState()
    const publish = () => {
      let absoluteSec = 0
      if (showTwitchEmbed) {
        absoluteSec = twitchGetTime() ?? vodOffsetSeconds
        setPlaying(relayState === 'playing')
      } else {
        const video = videoRef.current
        const current = video && Number.isFinite(video.currentTime)
          ? video.currentTime
          : metrics.currentTimeSec ?? 0
        absoluteSec = vodRelayBaseSeconds + Math.max(0, current)
        setPlaying(playbackState === 'playing')
      }
      setPlayhead(streamId, absoluteSec, vodId)
    }
    publish()
    const intervalMs = showTwitchEmbed ? 250 : 1000
    const intervalId = window.setInterval(publish, intervalMs)
    return () => {
      window.clearInterval(intervalId)
      usePlayheadStore.getState().reset()
    }
  }, [enabled, metrics.currentTimeSec, playbackState, relayState, showTwitchEmbed, streamId, videoRef, vodId, vodOffsetSeconds, vodRelayBaseSeconds, twitchGetTime])
  return null
}

export function ChannelStartupClock({
  onTick,
}: {
  onTick: (now: number) => void
}) {
  const { playbackState } = useChannelPlayback()
  const prev = useRef(playbackState)
  useEffect(() => {
    if (playbackState === 'playing') return
    const timer = window.setInterval(() => onTick(Date.now()), 250)
    return () => window.clearInterval(timer)
  }, [onTick, playbackState])
  useEffect(() => {
    prev.current = playbackState
  }, [playbackState])
  return null
}
