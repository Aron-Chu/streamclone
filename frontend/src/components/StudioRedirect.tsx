import { useEffect } from 'react'
import { useParams } from 'react-router-dom'
import { REPLAYFORGE_UI } from '../config'

export default function StudioRedirect() {
  const { jobId } = useParams<{ jobId?: string }>()

  useEffect(() => {
    const base = REPLAYFORGE_UI.replace(/\/$/, '')
    const target = jobId
      ? `${base}/studio/${encodeURIComponent(jobId)}`
      : `${base}/studio`
    window.location.replace(target)
  }, [jobId])

  return (
    <main className="flex min-h-screen items-center justify-center bg-[#07070a] text-zinc-100">
      <p className="text-sm font-semibold text-zinc-400">Opening ReplayForge Clip Studio…</p>
    </main>
  )
}
