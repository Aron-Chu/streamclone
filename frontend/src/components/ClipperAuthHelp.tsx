import { useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'

import {
  pollDevTwitchDeviceAuth,
  startDevTwitchDeviceAuth,
  syncClipperAuthFromSignIn,
  type DevDeviceAuthAuthenticatedResponse,
  type MeResponse,
} from '../api'
import { useAuth } from '../auth'

type ClipperAuthHelpProps = {
  compact?: boolean
  onSynced?: () => void
}

export default function ClipperAuthHelp({ compact = false, onSynced }: ClipperAuthHelpProps) {
  const auth = useAuth()
  const queryClient = useQueryClient()
  const twitchTabRef = useRef<Window | null>(null)
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState<string | null>(null)
  const [deviceRequestId, setDeviceRequestId] = useState<string | null>(null)
  const [pollIntervalMs, setPollIntervalMs] = useState(3000)

  useEffect(() => {
    if (!deviceRequestId) return

    let cancelled = false

    async function poll() {
      const requestId = deviceRequestId
      if (!requestId) return
      try {
        const result = await pollDevTwitchDeviceAuth(requestId)
        if (cancelled || result.status === 'pending') return
        applyAuthenticatedDeviceLogin(queryClient, result)
        const tab = twitchTabRef.current
        if (tab && !tab.closed) tab.close()
        twitchTabRef.current = null
        setDeviceRequestId(null)
        setStatus('Twitch connected — clipper token synced. Retry the export.')
        onSynced?.()
      } catch (error) {
        if (cancelled) return
        setDeviceRequestId(null)
        setStatus(error instanceof Error ? error.message : 'Twitch login failed.')
      }
    }

    void poll()
    const timer = window.setInterval(() => {
      void poll()
    }, pollIntervalMs)

    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [deviceRequestId, pollIntervalMs, onSynced, queryClient])

  async function handleReconnect() {
    setBusy(true)
    setStatus(null)
    try {
      if (auth.isAuthenticated) {
        await syncClipperAuthFromSignIn()
        setStatus('Clipper token synced from your Streamclone login. Retry the export.')
        onSynced?.()
        return
      }
      const started = await startDevTwitchDeviceAuth()
      const tab = window.open(started.verificationUri, '_blank')
      if (tab && !tab.closed) {
        twitchTabRef.current = tab
      }
      setPollIntervalMs(Math.max(started.pollIntervalSeconds, 1) * 1000)
      setDeviceRequestId(started.requestId)
      setStatus('Complete Twitch login in the opened tab — this page will sync automatically.')
    } catch {
      setStatus('Could not start Twitch login. Check that the stack is running on localhost:8090.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className={compact ? 'mt-1.5 space-y-1' : 'mt-2 space-y-2'}>
      <button
        type="button"
        onClick={() => void handleReconnect()}
        disabled={busy || Boolean(deviceRequestId)}
        className={`rounded border border-cyan-400/30 bg-cyan-500/10 font-black uppercase text-cyan-100 transition hover:bg-cyan-500/20 disabled:opacity-50 ${
          compact ? 'px-2 py-1 text-[9px]' : 'px-3 py-1.5 text-[10px]'
        }`}
      >
        {busy
          ? 'Connecting…'
          : deviceRequestId
            ? 'Waiting for Twitch…'
            : auth.isAuthenticated
              ? 'Sync clipper token'
              : 'Reconnect Twitch'}
      </button>
      {status ? (
        <p className={`font-semibold text-cyan-200/90 ${compact ? 'text-[9px]' : 'text-[10px]'}`}>{status}</p>
      ) : null}
    </div>
  )
}

function applyAuthenticatedDeviceLogin(
  queryClient: ReturnType<typeof useQueryClient>,
  result: DevDeviceAuthAuthenticatedResponse,
) {
  queryClient.setQueryData(['me'], (current: MeResponse | undefined) => ({
    ...current,
    authenticated: true,
    canImportLocalToken: true,
    user: result.user,
    scopes: result.scopes,
  }))
  queryClient.invalidateQueries({ queryKey: ['me'] })
  void syncClipperAuthFromSignIn().catch(() => {})
}
