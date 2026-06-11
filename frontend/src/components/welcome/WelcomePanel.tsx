import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { getSetupControlHealth, getSetupWelcome, startSetupService, type SetupWelcome } from '../../api'

const WELCOME_DISMISSED_KEY = 'streamclone-welcome-dismissed'
const WELCOME_SEEN_KEY = 'streamclone-welcome-seen'

export function isWelcomeDismissed() {
  return typeof window !== 'undefined' && window.localStorage.getItem(WELCOME_DISMISSED_KEY) === '1'
}

export function dismissWelcome() {
  window.localStorage.setItem(WELCOME_DISMISSED_KEY, '1')
  window.localStorage.setItem(WELCOME_SEEN_KEY, '1')
}

export function markWelcomeSeen() {
  window.localStorage.setItem(WELCOME_SEEN_KEY, '1')
}

export function shouldPromptWelcome(setup: SetupWelcome | undefined) {
  if (!setup || isWelcomeDismissed()) return false
  if (setup.incomplete) return true
  if (!window.localStorage.getItem(WELCOME_SEEN_KEY) && setup.showWelcome) return true
  return false
}

function ServiceCard({
  title,
  detail,
  status,
  actionLabel,
  onAction,
  busy,
  error,
}: {
  title: string
  detail: string
  status: 'ready' | 'offline' | 'checking'
  actionLabel?: string
  onAction?: () => void
  busy?: boolean
  error?: string | null
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

type WelcomePanelProps = {
  onDismiss?: () => void
  compact?: boolean
}

export default function WelcomePanel({ onDismiss, compact = false }: WelcomePanelProps) {
  const queryClient = useQueryClient()
  const [starting, setStarting] = useState<'scraper' | 'clipper' | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  const setup = useQuery({
    queryKey: ['setup-welcome'],
    queryFn: getSetupWelcome,
    staleTime: 5_000,
    refetchInterval: 10_000,
  })
  const control = useQuery({
    queryKey: ['setup-control-health'],
    queryFn: getSetupControlHealth,
    staleTime: 10_000,
    retry: false,
  })

  const profile = setup.data?.profile ?? 'core'
  const services = setup.data?.services
  const controlReady = Boolean(control.data?.ok)

  const refreshStatus = async () => {
    setActionError(null)
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['setup-welcome'] }),
      queryClient.invalidateQueries({ queryKey: ['setup-control-health'] }),
    ])
  }

  const handleStart = async (service: 'scraper' | 'clipper') => {
    setActionError(null)
    if (!controlReady) {
      setActionError('One-click start needs the local launcher. Run scripts/start-streamclone.ps1, then try again.')
      return
    }
    setStarting(service)
    try {
      await startSetupService(service)
      for (let attempt = 0; attempt < 15; attempt++) {
        await new Promise(resolve => window.setTimeout(resolve, 2000))
        await queryClient.invalidateQueries({ queryKey: ['setup-welcome'] })
        const latest = queryClient.getQueryData<SetupWelcome>(['setup-welcome'])
        if (latest?.services[service] === 'ready') break
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to start service.'
      setActionError(message)
    } finally {
      setStarting(null)
    }
  }

  const coreStatus = setup.isLoading ? 'checking' : setup.data ? 'ready' : 'offline'

  return (
    <div className={`${compact ? '' : 'mx-auto max-w-3xl'} space-y-5 text-zinc-100`}>
      <div className="space-y-2">
        <div className="text-xs font-black uppercase tracking-[0.2em] text-violet-300">Streamclone status</div>
        <h1 className="text-2xl font-black text-white">What is running right now</h1>
        <p className="text-sm font-semibold leading-6 text-zinc-400">
          Live checks for the core stack and optional scraper / clipper services. Status refreshes every 10 seconds.
        </p>
      </div>

      <section className="rounded-lg border border-violet-400/25 bg-violet-500/10 p-4">
        <div className="text-sm font-black text-violet-50">Prefer not to use a terminal?</div>
        <p className="mt-2 text-xs font-semibold leading-5 text-violet-100/85">
          After you clone or extract the repo, you do not need to type commands. Use the double-click launchers in the repo folder
          (or Desktop shortcuts after install).
        </p>
        <ul className="mt-3 space-y-2 text-xs font-semibold text-violet-50/90">
          {typeof navigator !== 'undefined' && /win/i.test(navigator.userAgent) ? (
            <>
              <li><span className="font-black text-white">First time:</span> double-click <code className="rounded bg-black/25 px-1 py-0.5">Install Streamclone.cmd</code> — creates Desktop shortcuts and runs setup.</li>
              <li><span className="font-black text-white">Every day:</span> double-click <code className="rounded bg-black/25 px-1 py-0.5">Start Streamclone.cmd</code> — starts Docker and opens this page.</li>
              <li><span className="font-black text-white">Stop:</span> double-click <code className="rounded bg-black/25 px-1 py-0.5">Stop Streamclone.cmd</code> or the Desktop shortcut.</li>
            </>
          ) : (
            <>
              <li><span className="font-black text-white">First time:</span> open <code className="rounded bg-black/25 px-1 py-0.5">launchers/Install Streamclone.command</code></li>
              <li><span className="font-black text-white">Every day:</span> double-click <code className="rounded bg-black/25 px-1 py-0.5">launchers/Start Streamclone.command</code></li>
            </>
          )}
        </ul>
        {setup.data?.setupGuideUrl ? (
          <a
            href={setup.data.setupGuideUrl}
            target="_blank"
            rel="noreferrer"
            className="mt-3 inline-flex rounded border border-violet-300/30 bg-violet-400/10 px-3 py-2 text-xs font-black text-violet-100 transition hover:bg-violet-400/20"
          >
            Desktop install guide
          </a>
        ) : null}
      </section>

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
        <ServiceCard
          title="Scraper"
          detail="TwitchTracker viewer charts for analytics sync."
          status={setup.isLoading ? 'checking' : services?.scraper ?? 'offline'}
          actionLabel="Start scraper"
          onAction={() => void handleStart('scraper')}
          busy={starting === 'scraper'}
        />
        <ServiceCard
          title="Clipper"
          detail="Clip Studio jobs and rendered clips."
          status={setup.isLoading ? 'checking' : services?.clipper ?? 'offline'}
          actionLabel="Start clipper"
          onAction={() => void handleStart('clipper')}
          busy={starting === 'clipper'}
        />
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <Link
          to="/"
          className="rounded border border-white/10 bg-white/[0.06] px-3 py-2 text-xs font-black text-zinc-200 transition hover:bg-white/10"
        >
          Go to directory
        </Link>
        {onDismiss ? (
          <button
            type="button"
            onClick={onDismiss}
            className="rounded border border-white/10 px-3 py-2 text-xs font-black text-zinc-400 transition hover:bg-white/10 hover:text-zinc-200"
          >
            Dismiss
          </button>
        ) : null}
      </div>
    </div>
  )
}
