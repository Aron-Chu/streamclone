import { useEffect, useState } from 'react'

import { useQueryClient } from '@tanstack/react-query'

import {
  ApiError,
  type DevDeviceAuthAuthenticatedResponse,
  type MeResponse,
  pollDevTwitchDeviceAuth,
  startDevTwitchDeviceAuth,
} from '../api'
import { useAuth } from '../auth'

type LocalTokenImportButtonProps = {
  compact?: boolean
}

type DeviceAuthModalState = {
  requestId: string
  userCode: string
  verificationUri: string
  expiresAt: number
  pollIntervalMs: number
}

export default function LocalTokenImportButton({ compact = false }: LocalTokenImportButtonProps) {
  const auth = useAuth()
  const queryClient = useQueryClient()
  const [opening, setOpening] = useState(false)
  const [status, setStatus] = useState<string | null>(null)
  const [deviceAuth, setDeviceAuth] = useState<DeviceAuthModalState | null>(null)
  const [now, setNow] = useState(() => Date.now())

  if (auth.isAuthenticated) {
    return null
  }

  useEffect(() => {
    if (!deviceAuth) return
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [deviceAuth])

  useEffect(() => {
    if (!deviceAuth) return

    const activeAuth = deviceAuth

    let cancelled = false

    async function poll() {
      try {
        const result = await pollDevTwitchDeviceAuth(activeAuth.requestId)
        if (cancelled || result.status === 'pending') {
          return
        }
        applyAuthenticatedDeviceLogin(queryClient, result)
        setDeviceAuth(null)
        setStatus(null)
      } catch (error) {
        if (cancelled) {
          return
        }
        const message = error instanceof Error ? error.message : 'Unable to complete Twitch verification.'
        setDeviceAuth(null)
        setStatus(message)
      }
    }

    void poll()
    const timer = window.setInterval(() => {
      void poll()
    }, activeAuth.pollIntervalMs)

    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [deviceAuth, queryClient])

  useEffect(() => {
    if (!deviceAuth) return
    if (deviceAuth.expiresAt > now) return
    setDeviceAuth(null)
    setStatus('Twitch verification expired. Click Use local token to start again.')
  }, [deviceAuth, now])

  async function beginDeviceAuth() {
    const started = await startDevTwitchDeviceAuth()
    setNow(Date.now())
    setDeviceAuth({
      requestId: started.requestId,
      userCode: started.userCode,
      verificationUri: started.verificationUri,
      expiresAt: Date.now() + started.expiresInSeconds * 1000,
      pollIntervalMs: Math.max(started.pollIntervalSeconds, 1) * 1000,
    })
  }

  async function useLocalToken() {
    setStatus(null)
    setOpening(true)
    if (auth.isLoading) {
      setStatus('Still checking local auth availability. Try again in a moment.')
      setOpening(false)
      return
    }
    if (auth.error) {
      setStatus('Local auth backend is unavailable. Start the backend or local proxy, then try again.')
      setOpening(false)
      return
    }
    if (!auth.canImportLocalToken) {
      setStatus('Local token auth is only available on localhost through the local proxy.')
      setOpening(false)
      return
    }
    try {
      await auth.claimPreparedLocalToken()
      setStatus(null)
    } catch (error) {
      const statusCode = error instanceof ApiError
        ? error.status
        : typeof error === 'object' && error !== null && 'status' in error && typeof error.status === 'number'
          ? error.status
          : undefined
      const message = error instanceof Error
        ? error.message
        : typeof error === 'object' && error !== null && 'message' in error && typeof error.message === 'string'
          ? error.message
          : ''

      if (statusCode === 404 || message === 'no prepared local token is waiting') {
        try {
          await beginDeviceAuth()
        } catch (deviceError) {
          setStatus(deviceError instanceof Error ? deviceError.message : 'Unable to start Twitch verification.')
        }
        return
      }
      setStatus(message || 'Unable to claim the local token.')
    } finally {
      setOpening(false)
    }
  }

  return (
    <div className="flex flex-col items-end gap-2">
      <button
        type="button"
        onClick={useLocalToken}
        disabled={opening || auth.isClaimingPreparedLocalToken}
        className={`${compact ? 'px-3' : 'px-4'} rounded border border-cyan-300/30 bg-cyan-400/10 py-2 text-xs font-black text-cyan-100 transition hover:bg-cyan-400/20 disabled:cursor-not-allowed disabled:opacity-70`}
      >
        {opening || auth.isClaimingPreparedLocalToken ? 'Opening local token...' : 'Use local token'}
      </button>
      {status ? (
        <div className="max-w-64 text-right text-[11px] font-semibold text-amber-100">
          {status}
        </div>
      ) : null}
      {deviceAuth ? (
        <DeviceAuthModal
          auth={deviceAuth}
          remainingSeconds={Math.max(0, Math.ceil((deviceAuth.expiresAt - now) / 1000))}
          onClose={() => setDeviceAuth(null)}
        />
      ) : null}
    </div>
  )
}

function DeviceAuthModal({
  auth,
  remainingSeconds,
  onClose,
}: {
  auth: DeviceAuthModalState
  remainingSeconds: number
  onClose: () => void
}) {
  return (
    <div className="fixed inset-0 z-[70] flex items-center justify-center bg-[#04050a]/80 p-4 backdrop-blur-sm">
      <div className="w-full max-w-md rounded border border-cyan-300/20 bg-[#111117] p-5 text-left text-zinc-100 shadow-2xl shadow-black/60">
        <div className="flex items-start justify-between gap-4">
          <div>
            <div className="text-base font-black text-white">Finish Twitch verification</div>
            <div className="mt-1 text-xs font-semibold text-zinc-400">
              Leave this dialog open. Streamclone will sign you in automatically after Twitch approval.
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded px-2 py-1 text-xs font-black text-zinc-400 transition hover:bg-white/10 hover:text-white"
          >
            Close
          </button>
        </div>

        <a
          href={auth.verificationUri}
          target="_blank"
          rel="noreferrer"
          className="mt-4 inline-flex rounded border border-cyan-300/30 bg-cyan-400/10 px-4 py-2 text-xs font-black text-cyan-100 transition hover:bg-cyan-400/20"
        >
          Open Twitch verification
        </a>

        <div className="mt-4 rounded border border-white/10 bg-white/[0.04] p-4">
          <div className="text-[11px] font-black uppercase tracking-[0.24em] text-zinc-500">Code</div>
          <div className="mt-2 font-mono text-2xl font-black tracking-[0.35em] text-white">{auth.userCode}</div>
        </div>

        <div className="mt-4 flex items-center justify-between text-xs font-semibold text-zinc-400">
          <span>{remainingSeconds > 0 ? `Expires in ${formatRemaining(remainingSeconds)}` : 'Verification expired'}</span>
          <span>Waiting for Twitch approval...</span>
        </div>
      </div>
    </div>
  )
}

function applyAuthenticatedDeviceLogin(queryClient: ReturnType<typeof useQueryClient>, result: DevDeviceAuthAuthenticatedResponse) {
  queryClient.setQueryData(['me'], (current: MeResponse | undefined) => ({
    ...current,
    authenticated: true,
    canImportLocalToken: true,
    user: result.user,
    scopes: result.scopes,
  }))
  queryClient.invalidateQueries({ queryKey: ['me'] })
  queryClient.invalidateQueries({ queryKey: ['followed'] })
}

function formatRemaining(totalSeconds: number) {
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  return `${minutes}:${seconds.toString().padStart(2, '0')}`
}
