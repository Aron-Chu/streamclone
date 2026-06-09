import { lazy, Suspense, useEffect, useState } from 'react'
import { Routes, Route } from 'react-router-dom'

const Directory = lazy(() => import('./components/Directory'))
const Channel = lazy(() => import('./components/Channel'))
const Analytics = lazy(() => import('./components/Analytics'))
const ClipStudio = lazy(() => import('./components/ClipStudio'))

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
      <Suspense fallback={<main className="grid min-h-screen place-items-center bg-[#07070a] text-sm font-bold text-zinc-300">Loading</main>}>
        <Routes>
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
