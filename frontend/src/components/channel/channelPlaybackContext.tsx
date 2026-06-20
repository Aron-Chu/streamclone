import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type RefObject,
} from 'react'

import type { PlaybackMetrics, PlaybackState } from '../../playback'
import { emptyMetrics } from '../../playback'
import type { PlaybackLatencyMode } from '../../settings'

export type PlaybackActionsRef = {
  seekVodRelay: (relative: number) => void
  jumpLive: () => void
  getError: () => string | null
  getHlsStage: () => string
  getState: () => PlaybackState
}

/** Mutable bridge so the channel shell can seek without subscribing to 1 Hz metrics. */
export const playbackActionsRef: { current: PlaybackActionsRef | null } = { current: null }

export type ChannelPlaybackContextValue = {
  metrics: PlaybackMetrics
  playbackState: PlaybackState
  state: PlaybackState
  error: string | null
  effectiveLatencyMode: PlaybackLatencyMode
  jumpLive: () => void
  seekVodRelay: (relativeSec: number) => void
  seekVodAbsoluteOffset: (absoluteOffset: number) => void
  videoRef: RefObject<HTMLVideoElement | null>
}

const defaultValue: ChannelPlaybackContextValue = {
  metrics: emptyMetrics,
  playbackState: 'starting',
  state: 'starting',
  error: null,
  effectiveLatencyMode: 'stable',
  jumpLive: () => undefined,
  seekVodRelay: () => undefined,
  seekVodAbsoluteOffset: () => undefined,
  videoRef: { current: null },
}

const ChannelPlaybackContext = createContext<ChannelPlaybackContextValue>(defaultValue)
const RegisterPlaybackContext = createContext<((value: ChannelPlaybackContextValue) => void) | null>(null)

export function ChannelPlaybackProvider({ children }: { children: React.ReactNode }) {
  const [value, setValue] = useState<ChannelPlaybackContextValue>(defaultValue)
  const register = useCallback((next: ChannelPlaybackContextValue) => {
    setValue(next)
  }, [])
  const registerCtx = useMemo(() => register, [register])
  return (
    <RegisterPlaybackContext.Provider value={registerCtx}>
      <ChannelPlaybackContext.Provider value={value}>
        {children}
      </ChannelPlaybackContext.Provider>
    </RegisterPlaybackContext.Provider>
  )
}

export function useRegisterChannelPlayback() {
  const register = useContext(RegisterPlaybackContext)
  if (!register) {
    throw new Error('useRegisterChannelPlayback must be used within ChannelPlaybackProvider')
  }
  return register
}

export function useChannelPlayback() {
  return useContext(ChannelPlaybackContext)
}
