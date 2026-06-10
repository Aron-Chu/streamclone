import { useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { getSetupWelcome } from '../../api'
import WelcomePanel, { dismissWelcome, markWelcomeSeen, shouldPromptWelcome } from './WelcomePanel'

export default function WelcomeGate() {
  const location = useLocation()
  const [open, setOpen] = useState(false)
  const setup = useQuery({
    queryKey: ['setup-welcome'],
    queryFn: getSetupWelcome,
    staleTime: 15_000,
  })

  useEffect(() => {
    if (location.pathname === '/welcome') {
      setOpen(false)
      return
    }
    setOpen(shouldPromptWelcome(setup.data))
    if (shouldPromptWelcome(setup.data)) {
      markWelcomeSeen()
    }
  }, [location.pathname, setup.data])

  if (!open || location.pathname === '/welcome') {
    return null
  }

  return (
    <div className="fixed inset-0 z-[70] flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div className="max-h-[90vh] w-full max-w-3xl overflow-y-auto rounded-xl border border-white/10 bg-[#0d0d12] p-5 shadow-2xl shadow-black/60">
        <div className="mb-4 flex items-center justify-between gap-3">
          <div className="text-sm font-black text-zinc-200">Stack status</div>
          <Link to="/welcome" className="text-xs font-black text-violet-300 hover:text-violet-200">
            Open full page
          </Link>
        </div>
        <WelcomePanel
          compact
          onDismiss={() => {
            dismissWelcome()
            setOpen(false)
          }}
        />
      </div>
    </div>
  )
}
