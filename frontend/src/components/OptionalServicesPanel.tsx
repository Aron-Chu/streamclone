import { Link } from 'react-router-dom'
import type { ReactNode } from 'react'
import { SETUP_CONTROL_WAKE_ENABLED } from '../config'
import { useOptionalServices } from '../hooks/useOptionalServices'

type ServiceStatus = 'ready' | 'offline' | 'checking'

function ServiceCard({
  title,
  detail,
  status,
  children,
}: {
  title: string
  detail: string
  status: ServiceStatus
  children?: ReactNode
}) {
  const good = status === 'ready'
  const checking = status === 'checking'
  return (
    <div className="rounded-lg border border-white/10 bg-white/[0.035] p-4">
      <div className="mb-2 flex items-center justify-between gap-3">
        <div className="text-sm font-black text-white">{title}</div>
        <span className={`rounded px-2 py-0.5 text-[10px] font-black uppercase tracking-wide ${
          checking ? 'bg-zinc-500/20 text-zinc-300' : good ? 'bg-emerald-500/15 text-emerald-100' : 'bg-amber-500/15 text-amber-100'
        }`}>
          {checking ? 'Checking' : good ? 'Ready' : 'Offline'}
        </span>
      </div>
      <p className="mb-3 text-xs font-semibold leading-5 text-zinc-400">{detail}</p>
      {children}
    </div>
  )
}

type OptionalServicesPanelProps = {
  variant: 'overlay' | 'banner'
  onDismiss?: () => void
  onBrowse?: () => void
}

export default function OptionalServicesPanel({
  variant,
  onDismiss,
  onBrowse,
}: OptionalServicesPanelProps) {
  const {
    hasServiceSnapshot,
    statusLoading,
    profile,
    refreshStatus,
  } = useOptionalServices({ probeControl: true, pollActive: variant === 'overlay' })

  const coreStatus: ServiceStatus = statusLoading ? 'checking' : hasServiceSnapshot ? 'ready' : 'offline'

  if (variant === 'banner') {
    return null
  }

  return (
    <div className="space-y-5 text-zinc-100">
      <div className="space-y-2">
        <div className="text-xs font-black uppercase tracking-[0.2em] text-violet-300">Welcome to Streamclone</div>
        <h1 className="text-2xl font-black text-white">What is running right now</h1>
        <p className="text-sm font-semibold leading-6 text-zinc-400">
          Live checks for the core watch stack — directory, playback, chat, and emotes.
        </p>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <span className="rounded border border-white/10 bg-white/[0.06] px-2.5 py-1 text-[11px] font-black uppercase text-zinc-300">
          Profile {profile}
        </span>
        <span className={`rounded px-2.5 py-1 text-[11px] font-black uppercase ${
          SETUP_CONTROL_WAKE_ENABLED ? 'bg-cyan-500/15 text-cyan-100' : 'bg-zinc-500/20 text-zinc-400'
        }`}>
          {SETUP_CONTROL_WAKE_ENABLED ? 'Launcher control available' : 'Launcher control offline'}
        </span>
        <button
          type="button"
          onClick={() => void refreshStatus()}
          className="rounded border border-white/10 bg-white/[0.06] px-3 py-1.5 text-xs font-black text-zinc-200 transition hover:bg-white/10"
        >
          Refresh now
        </button>
      </div>

      <div className="mx-auto grid w-full max-w-5xl justify-center gap-3 [grid-template-columns:repeat(auto-fit,minmax(min(100%,18rem),1fr))]">
        <ServiceCard
          title="Core app"
          detail="Directory, playback, chat, and emotes."
          status={coreStatus}
        />
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <button
          type="button"
          onClick={onBrowse}
          className="rounded-lg border border-violet-400/50 bg-violet-500/20 px-5 py-3 text-sm font-black text-violet-50 shadow-lg shadow-violet-950/30 transition hover:bg-violet-500/30"
        >
          Browse live streams
        </button>
        {onDismiss ? (
          <button
            type="button"
            onClick={onDismiss}
            className="rounded border border-white/10 px-3 py-2 text-xs font-black text-zinc-400 transition hover:bg-white/10 hover:text-zinc-200"
          >
            Not now
          </button>
        ) : null}
        <Link
          to="/"
          className="rounded border border-white/10 bg-white/[0.06] px-3 py-2 text-xs font-black text-zinc-300 transition hover:bg-white/10"
        >
          Live directory
        </Link>
      </div>
    </div>
  )
}
