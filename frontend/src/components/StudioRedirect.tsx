import { useEffect } from 'react'
import { useParams } from 'react-router-dom'
import { REPLAYFORGE_UI } from '../config'
import { replayforgeStudioUrl } from '../utils/studioLink'

export default function StudioRedirect() {
  const { jobId } = useParams<{ jobId?: string }>()

  useEffect(() => {
    window.location.replace(replayforgeStudioUrl(REPLAYFORGE_UI, jobId))
  }, [jobId])

  return (
    <main className="flex min-h-screen items-center justify-center bg-[#07070a] text-zinc-100">
      <p className="text-sm font-semibold text-zinc-400">Opening ReplayForge Clip Studio…</p>
    </main>
  )
}
