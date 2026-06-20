import { useEffect, useRef } from 'react'

import { useHlsPlayback, type PlaybackState } from '../../playback'
import { playbackActionsRef, useRegisterChannelPlayback } from './channelPlaybackContext'

export type ChannelPlaybackHostProps = {
  videoRef: React.RefObject<HTMLVideoElement | null>
  options: Parameters<typeof useHlsPlayback>[1]
  showTwitchEmbed: boolean
  relayState: PlaybackState
  hlsUrl: string
  seekVodAbsoluteOffset: (absoluteOffset: number) => void
  onCoarsePlaybackStateChange?: (state: PlaybackState) => void
  onFirstFrameMs?: (ms: number) => void
}

export function ChannelPlaybackHost({
  videoRef,
  options,
  showTwitchEmbed,
  relayState,
  hlsUrl,
  seekVodAbsoluteOffset,
  onCoarsePlaybackStateChange,
  onFirstFrameMs,
}: ChannelPlaybackHostProps) {
  const playback = useHlsPlayback(videoRef as React.RefObject<HTMLVideoElement>, options)
  const registerPlayback = useRegisterChannelPlayback()
  const prevCoarseState = useRef<PlaybackState | null>(null)
  const playbackState = showTwitchEmbed ? relayState : (hlsUrl ? playback.state : relayState)

  playbackActionsRef.current = {
    seekVodRelay: playback.seekVodRelay,
    jumpLive: () => playback.jumpLive(),
    getError: () => playback.error,
    getHlsStage: () => playback.metrics.hlsStage,
    getState: () => playback.state,
  }

  useEffect(() => {
    if (prevCoarseState.current === playbackState) return
    prevCoarseState.current = playbackState
    onCoarsePlaybackStateChange?.(playbackState)
  }, [onCoarsePlaybackStateChange, playbackState])

  useEffect(() => {
    if (playback.metrics.firstFrameMs === null) return
    onFirstFrameMs?.(playback.metrics.firstFrameMs)
  }, [onFirstFrameMs, playback.metrics.firstFrameMs])

  useEffect(() => {
    registerPlayback({
      metrics: playback.metrics,
      playbackState,
      state: playback.state,
      error: playback.error,
      effectiveLatencyMode: playback.effectiveLatencyMode,
      jumpLive: () => playback.jumpLive(),
      seekVodRelay: (relativeSec: number) => playback.seekVodRelay(relativeSec),
      seekVodAbsoluteOffset,
      videoRef,
    })
  }, [
    registerPlayback,
    playback,
    playback.metrics,
    playback.state,
    playback.error,
    playback.effectiveLatencyMode,
    playbackState,
    seekVodAbsoluteOffset,
    videoRef,
  ])

  return null
}
