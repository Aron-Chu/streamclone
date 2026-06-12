import { Link } from 'react-router-dom'
import { profileNeedsScraper, SCRAPER_SETUP_DOC_URL } from '../setupProfile'
import { useOptionalServices, type ServiceStartProgress } from '../hooks/useOptionalServices'

type ServiceStatus = 'ready' | 'offline' | 'checking'

export function CoreMinuteChartsNotice({ compact = false }: { compact?: boolean }) {
  const { controlReady, starting, startService } = useOptionalServices()

  return (
    <div className={compact ? 'mt-2 text-left' : 'max-w-md'}>
      <div className={`font-black text-zinc-100 ${compact ? 'text-[11px]' : 'text-base'}`}>
        Minute charts need Analytics tier
      </div>
      <p className={`mt-1 font-semibold text-zinc-500 ${compact ? 'text-[10px] leading-4' : 'text-sm'}`}>
        Core Watch includes Helix/VOD stream lists and TwitchTracker summary stats (avg/peak).
        Per-minute viewer, chat, and emote charts require the optional scraper profile.
      </p>
      <div className={`mt-2 flex flex-wrap items-center gap-2 ${compact ? 'text-[10px]' : 'text-xs'}`}>
        {controlReady ? (
          <button
            type="button"
            onClick={() => void startService('scraper')}
            disabled={starting === 'scraper'}
            className={`rounded border border-violet-400/40 bg-violet-500/15 px-2.5 py-1 font-black text-violet-100 transition hover:bg-violet-500/25 disabled:opacity-50 ${compact ? 'text-[10px]' : 'text-xs'}`}
          >
            {starting === 'scraper' ? 'Starting…' : 'Start Analytics'}
          </button>
        ) : null}
        <Link
          to="/"
          className="font-bold text-zinc-300 underline decoration-zinc-500/40 underline-offset-2 transition hover:text-white"
        >
          Live directory
        </Link>
        <a
          href={SCRAPER_SETUP_DOC_URL}
          target="_blank"
          rel="noreferrer"
          className="font-bold text-violet-300 underline decoration-violet-400/30 underline-offset-2 transition hover:text-violet-200"
        >
          Scraper setup guide →
        </a>
      </div>
    </div>
  )
}

function ServiceStartProgressBar({ progress }: { progress: ServiceStartProgress }) {
  const width = Math.max(0, Math.min(100, progress.percent))
  return (
    <div className="mb-3 rounded-lg border border-violet-400/25 bg-violet-500/10 p-3">
      <div className="mb-2 flex items-center justify-between gap-2 text-[11px] font-black uppercase tracking-wide text-violet-100">
        <span>{progress.phase}</span>
        <span>{width}%</span>
      </div>
      <div className="h-2 overflow-hidden rounded-full bg-black/40">
        <div
          className="h-full rounded-full bg-gradient-to-r from-violet-500 to-cyan-400 transition-all duration-500 ease-out"
          style={{ width: `${width}%` }}
        />
      </div>
      <p className="mt-2 text-[11px] font-semibold leading-4 text-violet-100/80">{progress.detail}</p>
    </div>
  )
}

function ServiceCard({
  title,
  detail,
  status,
  actionLabel,
  onAction,
  busy,
  error,
  progress,
}: {
  title: string
  detail: string
  status: ServiceStatus
  actionLabel?: string
  onAction?: () => void
  busy?: boolean
  error?: string | null
  progress?: ServiceStartProgress | null
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
      {progress ? <ServiceStartProgressBar progress={progress} /> : null}
      {error ? (
        <div className="mb-3 rounded border border-red-300/20 bg-red-500/10 px-2 py-1.5 text-xs font-semibold text-red-100">
          {error}
        </div>
      ) : null}
      {actionLabel && onAction && status !== 'ready' ? (
        <button
          type="button"
          onClick={onAction}
          disabled={busy}
          className="rounded border border-violet-400/40 bg-violet-500/15 px-3 py-2 text-xs font-black text-violet-100 transition hover:bg-violet-500/25 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {busy ? 'Starting…' : actionLabel}
        </button>
      ) : null}
    </div>
  )
}

type OptionalServicesPanelProps = {
  variant: 'overlay' | 'banner'
  focus?: 'scraper' | 'clipper' | 'all'
  onDismiss?: () => void
  onBrowse?: () => void
  channelLogin?: string
}

export default function OptionalServicesPanel({
  variant,
  focus = 'all',
  onDismiss,
  onBrowse,
  channelLogin,
}: OptionalServicesPanelProps) {
  const {
    setup,
    profile,
    services,
    controlReady,
    scraperOffline,
    clipperOffline,
    starting,
    startProgress,
    actionError,
    startService,
    refreshStatus,
  } = useOptionalServices()

  const coreStatus: ServiceStatus = setup.isLoading ? 'checking' : setup.data ? 'ready' : 'offline'
  const scraperStatus: ServiceStatus = setup.isLoading ? 'checking' : services?.scraper ?? 'offline'
  const clipperStatus: ServiceStatus = setup.isLoading ? 'checking' : services?.clipper ?? 'offline'

  const showScraper = focus === 'all' || focus === 'scraper'
  const showClipper = focus === 'all' || focus === 'clipper'

  if (variant === 'banner') {
    const scraperBanner = showScraper && scraperOffline && (profile === 'core' || profileNeedsScraper(profile))
    const clipperBanner = showClipper && clipperOffline

    if (!scraperBanner && !clipperBanner) return null

    const isCoreInfo = profile === 'core' && scraperBanner && !clipperBanner

    return (
      <div className={`rounded-lg border px-3 py-2.5 sm:px-4 ${
        isCoreInfo ? 'border-cyan-300/20 bg-cyan-400/10' : 'border-amber-300/20 bg-amber-400/10'
      }`}>
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p className={`text-xs font-semibold leading-5 sm:text-sm ${
            isCoreInfo ? 'text-cyan-50/90' : 'text-amber-50/90'
          }`}>
            {clipperBanner && !scraperBanner
              ? 'Clip Studio is offline — start the clipper service to edit clips.'
              : isCoreInfo
                ? 'Viewer charts need Analytics setup — optional charts and VOD chat load from a second profile.'
                : 'Viewer charts are paused — Analytics is not running.'}
          </p>
          <div className="flex shrink-0 flex-wrap items-center gap-2">
            {scraperBanner ? (
              <a
                href={SCRAPER_SETUP_DOC_URL}
                target="_blank"
                rel="noreferrer"
                className={`rounded border px-2.5 py-1 text-[11px] font-black uppercase tracking-wide transition ${
                  isCoreInfo
                    ? 'border-cyan-200/30 bg-cyan-300/15 text-cyan-50 hover:bg-cyan-300/25'
                    : 'border-amber-200/30 bg-amber-300/15 text-amber-50 hover:bg-amber-300/25'
                }`}
              >
                {isCoreInfo ? 'Analytics setup' : 'Setup guide'}
              </a>
            ) : null}
            {scraperBanner && controlReady ? (
              <button
                type="button"
                onClick={() => void startService('scraper')}
                disabled={starting === 'scraper'}
                className="rounded border border-amber-200/30 bg-amber-300/15 px-2.5 py-1 text-[11px] font-black uppercase tracking-wide text-amber-50 transition hover:bg-amber-300/25 disabled:opacity-50"
              >
                {starting === 'scraper' ? 'Starting…' : 'Start Analytics'}
              </button>
            ) : null}
            {clipperBanner && controlReady ? (
              <button
                type="button"
                onClick={() => void startService('clipper')}
                disabled={starting === 'clipper'}
                className="rounded border border-amber-200/30 bg-amber-300/15 px-2.5 py-1 text-[11px] font-black uppercase tracking-wide text-amber-50 transition hover:bg-amber-300/25 disabled:opacity-50"
              >
                {starting === 'clipper' ? 'Starting…' : 'Start Clip Studio'}
              </button>
            ) : null}
            <Link
              to="/"
              className="rounded border border-white/10 bg-white/[0.06] px-2.5 py-1 text-[11px] font-black uppercase tracking-wide text-zinc-200 transition hover:bg-white/10"
            >
              Live directory
            </Link>
            {channelLogin ? (
              <Link
                to={`/analytics/${encodeURIComponent(channelLogin)}`}
                className="rounded border border-white/10 bg-white/[0.06] px-2.5 py-1 text-[11px] font-black uppercase tracking-wide text-zinc-200 transition hover:bg-white/10"
              >
                Back to Analytics
              </Link>
            ) : null}
          </div>
        </div>
        {actionError ? (
          <p className="mt-2 text-[11px] font-semibold text-amber-100/80">{actionError}</p>
        ) : null}
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-3xl space-y-5 text-zinc-100">
      <div className="space-y-2">
        <div className="text-xs font-black uppercase tracking-[0.2em] text-violet-300">Welcome to Streamclone</div>
        <h1 className="text-2xl font-black text-white">What is running right now</h1>
        <p className="text-sm font-semibold leading-6 text-zinc-400">
          Live checks for the core stack and optional Analytics and Clip Studio services.
        </p>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <span className="rounded border border-white/10 bg-white/[0.06] px-2.5 py-1 text-[11px] font-black uppercase text-zinc-300">
          Profile {profile}
        </span>
        <span className={`rounded px-2.5 py-1 text-[11px] font-black uppercase ${
          controlReady ? 'bg-emerald-500/15 text-emerald-100' : 'bg-zinc-500/20 text-zinc-400'
        }`}>
          {controlReady ? 'One-click start available' : 'Launcher control offline'}
        </span>
        <button
          type="button"
          onClick={() => void refreshStatus()}
          className="rounded border border-white/10 bg-white/[0.06] px-3 py-1.5 text-xs font-black text-zinc-200 transition hover:bg-white/10"
        >
          Refresh now
        </button>
      </div>

      {startProgress ? (
        <ServiceStartProgressBar progress={startProgress} />
      ) : null}

      {actionError ? (
        <div className="rounded border border-amber-300/20 bg-amber-400/10 p-3 text-xs font-semibold text-amber-100">
          {actionError}
        </div>
      ) : null}

      <div className="grid gap-3 md:grid-cols-3">
        <ServiceCard
          title="Core app"
          detail="Directory, playback, chat, emotes, and analytics API."
          status={coreStatus}
        />
        {showScraper ? (
          <ServiceCard
            title="Analytics (scraper)"
            detail="TwitchTracker viewer charts for analytics sync."
            status={scraperStatus}
            actionLabel="Start Analytics"
            onAction={() => void startService('scraper')}
            busy={starting === 'scraper'}
            progress={startProgress?.service === 'scraper' ? startProgress : null}
          />
        ) : null}
        {showClipper ? (
          <ServiceCard
            title="Clip Studio (clipper)"
            detail="Clip Studio jobs and rendered clips."
            status={clipperStatus}
            actionLabel="Start Clip Studio"
            onAction={() => void startService('clipper')}
            busy={starting === 'clipper'}
            progress={startProgress?.service === 'clipper' ? startProgress : null}
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
