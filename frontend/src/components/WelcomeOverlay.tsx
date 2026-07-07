import { useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { STREAMCLONE_INSTALL_ID } from '../config'
import { isOnboardingDismissed, markOnboardingDismissed } from '../onboardingStorage'
import { OPEN_STACK_STATUS_EVENT } from '../stackStatusEvents'
import SystemHealthPanel from './SystemHealthPanel'

export default function WelcomeOverlay() {
  const location = useLocation()
  const navigate = useNavigate()
  const [forcedOpen, setForcedOpen] = useState(false)
  const [sessionDismissed, setSessionDismissed] = useState(false)
  const showOnboarding = Boolean((location.state as { showOnboarding?: boolean } | null)?.showOnboarding)
  const installId = STREAMCLONE_INSTALL_ID
  const autoShow = location.pathname === '/' && !sessionDismissed && (showOnboarding || !isOnboardingDismissed(installId))
  const open = forcedOpen || autoShow

  useEffect(() => {
    const onOpen = () => setForcedOpen(true)
    window.addEventListener(OPEN_STACK_STATUS_EVENT, onOpen)
    return () => window.removeEventListener(OPEN_STACK_STATUS_EVENT, onOpen)
  }, [])

  useEffect(() => {
    const params = new URLSearchParams(location.search)
    if (params.get('welcome') === '1') {
      setForcedOpen(true)
      params.delete('welcome')
      const nextSearch = params.toString()
      navigate(
        { pathname: location.pathname, search: nextSearch ? `?${nextSearch}` : '' },
        { replace: true, state: location.state },
      )
    }
  }, [location.pathname, location.search, location.state, navigate])

  useEffect(() => {
    if (!showOnboarding) return
    setForcedOpen(true)
    navigate('.', { replace: true, state: {} })
  }, [showOnboarding, navigate])

  if (!open) return null

  const handleDismiss = () => {
    markOnboardingDismissed(installId)
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
