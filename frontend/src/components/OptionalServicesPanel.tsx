import { Link } from 'react-router-dom'
import type { ReactNode } from 'react'
import { SETUP_CONTROL_WAKE_ENABLED, REPLAYFORGE_UI } from '../config'
import { useOptionalServices } from '../hooks/useOptionalServices'

type ServiceStatus = 'ready' | 'offline' | 'checking'

function ServiceCard({
  title,
  detail,
  status,
  readyHref,
  readyLabel,
  onReadyOpen,
  offlineHref,
  offlineLabel,
  children,
}: {
  title: string
  detail: string
  status: ServiceStatus
  readyHref?: string
  readyLabel?: string
  onReadyOpen?: () => void
  offlineHref?: string
  offlineLabel?: string
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
      {offlineHref && status !== 'ready' ? (
        <a
          href={offlineHref}
          target="_blank"
          rel="noreferrer"
          className="inline-flex rounded border border-violet-400/40 bg-violet-500/15 px-3 py-2 text-xs font-black text-violet-100 transition hover:bg-violet-500/25"
        >
          {offlineLabel ?? 'Open'}
        </a>
      ) : null}
      {readyHref && status === 'ready' ? (
        <a
          href={readyHref}
          target="_blank"
          rel="noreferrer"
          onClick={onReadyOpen}
          className="inline-flex rounded border border-emerald-400/40 bg-emerald-500/15 px-3 py-2 text-xs font-black text-emerald-100 transition hover:bg-emerald-500/25"
        >
          {readyLabel ?? 'Open'}
        </a>
      ) : null}
      {children}
    </div>
  )
}

type OptionalServicesPanelProps = {
  variant: 'overlay' | 'banner'
  focus?: 'clipper' | 'all'
  onDismiss?: () => void
  onBrowse?: () => void
  channelLogin?: string
}

export default function OptionalServicesPanel({
  variant,
  focus = 'all',
  onDismiss,
  onBrowse,
}: OptionalServicesPanelProps) {
  const {
    hasServiceSnapshot,
    statusLoading,
    profile,
    clipperOffline,
    clipperReady,
    refreshStatus,
  } = useOptionalServices({ probeControl: true, pollActive: variant === 'overlay' })

  const coreStatus: ServiceStatus = statusLoading ? 'checking' : hasServiceSnapshot ? 'ready' : 'offline'
  const clipperStatus: ServiceStatus = statusLoading ? 'checking' : clipperReady ? 'ready' : 'offline'
  const replayforgeUi = REPLAYFORGE_UI.replace(/\/$/, '')

  const showClipper = focus === 'all' || focus === 'clipper'

  if (variant === 'banner') {
    const clipperBanner = showClipper && clipperOffline
    if (!clipperBanner) return null

    return (
      <div className="rounded-lg border border-amber-300/20 bg-amber-400/10 px-3 py-2.5 sm:px-4">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-xs font-semibold leading-5 text-amber-50/90 sm:text-sm">
            ReplayForge is offline — start ReplayForge to edit clips.
          </p>
          <div className="flex shrink-0 flex-wrap items-center gap-2">
            <a
              href={replayforgeUi}
              target="_blank"
              rel="noreferrer"
              className="rounded border border-amber-200/30 bg-amber-300/15 px-2.5 py-1 text-[11px] font-black uppercase tracking-wide text-amber-50 transition hover:bg-amber-300/25"
            >
              Start ReplayForge
            </a>
            {clipperReady ? (
              <a
                href={replayforgeUi}
                target="_blank"
                rel="noreferrer"
                className="rounded border border-emerald-200/30 bg-emerald-300/15 px-2.5 py-1 text-[11px] font-black uppercase tracking-wide text-emerald-50 transition hover:bg-emerald-300/25"
              >
                Open ReplayForge
              </a>
            ) : null}
            <Link
              to="/"
              className="rounded border border-white/10 bg-white/[0.06] px-2.5 py-1 text-[11px] font-black uppercase tracking-wide text-zinc-200 transition hover:bg-white/10"
            >
              Live directory
            </Link>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-5 text-zinc-100">
      <div className="space-y-2">
        <div className="text-xs font-black uppercase tracking-[0.2em] text-violet-300">Welcome to Streamclone</div>
        <h1 className="text-2xl font-black text-white">What is running right now</h1>
        <p className="text-sm font-semibold leading-6 text-zinc-400">
          Live checks for the core stack and optional ReplayForge.
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
        {showClipper ? (
          <ServiceCard
            title="ReplayForge"
            detail="Clip Studio jobs and rendered clips. Install and run ReplayForge separately (API :8095, UI :8096)."
            status={clipperStatus}
            offlineHref={replayforgeUi}
            offlineLabel="Start ReplayForge"
            readyHref={replayforgeUi}
            readyLabel="Open ReplayForge"
          />
        ) : null}
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
      </div>
    </div>
  )
}
