import { lazy, Suspense, useEffect, useState } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import WelcomeOverlay from './components/WelcomeOverlay'

const Directory = lazy(() => import('./components/Directory'))
const Channel = lazy(() => import('./components/Channel'))
const Analytics = lazy(() => import('./components/Analytics'))
const ClipStudio = lazy(() => import('./components/ClipStudio'))

function RouteLoadingSkeleton() {
  return (
    <main className="min-h-screen overflow-hidden bg-[#07070a] text-zinc-100">
      <div className="pointer-events-none fixed inset-0 bg-[linear-gradient(135deg,rgba(139,92,246,.16),transparent_28%),linear-gradient(180deg,rgba(255,255,255,.045),transparent_34%)]" />
      <div className="relative mx-auto flex min-h-screen w-full max-w-7xl flex-col px-4 py-5 sm:px-6 lg:px-8">
        <div className="mb-6 flex items-center justify-between gap-4">
          <div className="h-10 w-40 animate-pulse rounded-lg bg-white/10" />
          <div className="h-11 w-full max-w-xl animate-pulse rounded-lg bg-white/10" />
        </div>
        <div className="mb-6 h-72 animate-pulse rounded-lg border border-white/10 bg-white/[0.04]" />
        <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 xl:grid-cols-4">
          {Array.from({ length: 8 }).map((_, index) => (
            <div key={index} className="overflow-hidden rounded-lg border border-white/10 bg-white/[0.04]">
              <div className="aspect-video animate-pulse bg-white/10" />
              <div className="space-y-2 p-3">
                <div className="h-4 w-5/6 animate-pulse rounded bg-white/10" />
                <div className="h-3 w-2/3 animate-pulse rounded bg-white/10" />
              </div>
            </div>
          ))}
        </div>
      </div>
    </main>
  )
}

export default function App() {
  const [authNotice, setAuthNotice] = useState<{ tone: 'success' | 'error'; message: string } | null>(null)

  useEffect(() => {
    const url = new URL(window.location.href)
    const status = url.searchParams.get('auth')
    if (status !== 'success' && status !== 'error') return
    const message = url.searchParams.get('auth_message') || (status === 'success' ? 'Twitch connected.' : 'Twitch login failed.')
    setAuthNotice({ tone: status, message })
    url.searchParams.delete('auth')
    url.searchParams.delete('auth_code')
    url.searchParams.delete('auth_message')
    window.history.replaceState({}, '', `${url.pathname}${url.search}${url.hash}`)
    const timer = window.setTimeout(() => setAuthNotice(null), 7000)
    return () => window.clearTimeout(timer)
  }, [])

  return (
    <>
      {authNotice ? (
        <div className={`fixed left-1/2 top-4 z-[60] w-[calc(100%-2rem)] max-w-xl -translate-x-1/2 rounded border px-4 py-3 text-sm font-bold shadow-2xl ${authNotice.tone === 'success' ? 'border-emerald-300/30 bg-emerald-500/15 text-emerald-100' : 'border-red-300/30 bg-red-500/15 text-red-100'}`}>
          {authNotice.message}
        </div>
      ) : null}
      <WelcomeOverlay />
      <Suspense fallback={<RouteLoadingSkeleton />}>
        <Routes>
          <Route path="/welcome" element={<Navigate to="/" replace state={{ showOnboarding: true }} />} />
          <Route path="/" element={<Directory />} />
          <Route path="/c/:login" element={<Channel />} />
          <Route path="/analytics/:login" element={<Analytics />} />
          <Route path="/analytics/:login/:streamId" element={<Analytics />} />
          <Route path="/studio/:jobId" element={<ClipStudio />} />
        </Routes>
      </Suspense>
    </>
  )
}
