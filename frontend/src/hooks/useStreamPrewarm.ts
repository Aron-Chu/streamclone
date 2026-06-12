import { useCallback, useEffect, useRef } from 'react'
import { startStream } from '../api'
import { useUiSettings } from '../settings'
import { requestQuality } from '../streamQuality'

// Hover-intent delay before firing the relay start. Casual mouse passes are free.
const hoverIntentMs = 300
// Don't re-prewarm the same channel inside this window. The video service
// reaps listenerless sessions after STREAM_IDLE_TIMEOUT (60s default), so a
// shorter TTL keeps the relay warm across repeated hovers without stacking calls.
const prewarmTtlMs = 45_000

const recentPrewarms = new Map<string, number>()

/**
 * Prewarm the HLS relay for a channel when the user shows intent (hover/focus
 * on a card) so playback is near-instant when they actually navigate.
 *
 * Fire-and-forget: the prewarm session never sends keepalives, so if the user
 * never opens the channel the orchestrator's reaper kills the relay after the
 * idle timeout. If they do navigate, the channel page joins the warm relay.
 */
export function useStreamPrewarm() {
  const settings = useUiSettings(state => state.settings)
  const settingsRef = useRef(settings)
  settingsRef.current = settings
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => () => {
    if (timerRef.current) clearTimeout(timerRef.current)
  }, [])

  const cancelPrewarm = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }, [])

  const prewarm = useCallback((login: string, isLive: boolean) => {
    if (!isLive || !login) return
    cancelPrewarm()
    timerRef.current = setTimeout(() => {
      timerRef.current = null
      const last = recentPrewarms.get(login)
      const now = Date.now()
      if (last && now - last < prewarmTtlMs) return
      recentPrewarms.set(login, now)
      const { preferredQuality, playbackLatencyMode } = settingsRef.current
      startStream(login, requestQuality(preferredQuality || 'best'), playbackLatencyMode)
        .catch(() => {
          // Offline channel or relay cap reached — let the next hover retry after TTL.
          recentPrewarms.delete(login)
        })
    }, hoverIntentMs)
  }, [cancelPrewarm])

  return { prewarm, cancelPrewarm }
}
