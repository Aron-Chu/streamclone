import { useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { isOnboardingDismissed, markOnboardingDismissed } from '../onboardingStorage'
import { OPEN_STACK_STATUS_EVENT } from '../stackStatusEvents'
import OptionalServicesPanel from './OptionalServicesPanel'

export default function WelcomeOverlay() {
  const location = useLocation()
  const navigate = useNavigate()
  const [forcedOpen, setForcedOpen] = useState(false)
  const [sessionDismissed, setSessionDismissed] = useState(false)

  useEffect(() => {
    const open = () => setForcedOpen(true)
    window.addEventListener(OPEN_STACK_STATUS_EVENT, open)
    return () => window.removeEventListener(OPEN_STACK_STATUS_EVENT, open)
  }, [])

  const showOnboarding = Boolean((location.state as { showOnboarding?: boolean } | null)?.showOnboarding)

  useEffect(() => {
    if (!showOnboarding) return
    navigate('.', { replace: true, state: {} })
  }, [showOnboarding, navigate])

  const autoShow = location.pathname === '/' && !sessionDismissed && (showOnboarding || !isOnboardingDismissed())
  const open = forcedOpen || autoShow

  if (!open) return null

  const handleDismiss = () => {
    markOnboardingDismissed()
    setSessionDismissed(true)
    setForcedOpen(false)
  }

  return (
    <div className="fixed inset-0 z-[70] flex items-center justify-center bg-black/60 p-4 backdrop-blur-md">
      <div className="max-h-[90vh] w-full max-w-3xl overflow-y-auto rounded-xl border border-white/10 bg-[#0d0d12]/95 p-5 shadow-2xl shadow-black/60">
        <div className="mb-4 flex items-center justify-between gap-3">
          <div className="text-sm font-black text-zinc-200">Stack status</div>
        </div>
        <OptionalServicesPanel
          variant="overlay"
          onDismiss={handleDismiss}
          onBrowse={handleDismiss}
        />
      </div>
    </div>
  )
}
