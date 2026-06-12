import { useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { isOnboardingDismissed, markOnboardingDismissed } from '../onboardingStorage'
import { OPEN_STACK_STATUS_EVENT, dispatchOpenStackStatus } from '../stackStatusEvents'
import { useOptionalServices } from '../hooks/useOptionalServices'
import SystemHealthPanel from './SystemHealthPanel'

export default function WelcomeOverlay() {
  const location = useLocation()
  const navigate = useNavigate()
  const [forcedOpen, setForcedOpen] = useState(false)
  const [sessionDismissed, setSessionDismissed] = useState(false)
  const offlinePromptedRef = useRef(false)
  const { setup, scraperOffline, clipperOffline } = useOptionalServices()

  useEffect(() => {
    const open = () => setForcedOpen(true)
    window.addEventListener(OPEN_STACK_STATUS_EVENT, open)
    return () => window.removeEventListener(OPEN_STACK_STATUS_EVENT, open)
  }, [])

  const showOnboarding = Boolean((location.state as { showOnboarding?: boolean } | null)?.showOnboarding)

  useEffect(() => {
    if (!showOnboarding) return
    setForcedOpen(true)
    navigate('.', { replace: true, state: {} })
  }, [showOnboarding, navigate])

  useEffect(() => {
    if (!setup.data?.incomplete) return
    if (offlinePromptedRef.current) return
    if (!isOnboardingDismissed() && location.pathname === '/') return
    if (scraperOffline || clipperOffline) {
      offlinePromptedRef.current = true
      dispatchOpenStackStatus()
    }
  }, [setup.data?.incomplete, scraperOffline, clipperOffline, location.pathname])

  const autoShow = location.pathname === '/' && !sessionDismissed && (showOnboarding || !isOnboardingDismissed())
  const open = forcedOpen || autoShow

  if (!open) return null

  const handleDismiss = () => {
    markOnboardingDismissed()
    setSessionDismissed(true)
    setForcedOpen(false)
  }

  return (
    <div className="fixed inset-0 z-[70] flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div className="max-h-[90vh] w-full max-w-3xl overflow-y-auto rounded-xl border border-white/10 bg-[#0d0d12] p-5 shadow-2xl shadow-black/60">
        <div className="mb-4 flex items-center justify-between gap-3">
          <div className="text-sm font-black text-zinc-200">Stack status</div>
          <button
            type="button"
            onClick={handleDismiss}
            className="rounded px-2 py-1 text-xs font-black text-zinc-400 transition hover:bg-white/10 hover:text-white"
          >
            Close
          </button>
        </div>
        <SystemHealthPanel
          onDismiss={handleDismiss}
          onBrowse={() => {
            handleDismiss()
            if (location.pathname !== '/') navigate('/')
          }}
        />
      </div>
    </div>
  )
}
