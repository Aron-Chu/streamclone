import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'

import {
  fetchChannelSpread,
  fetchSourceHealth,
  isSpreadBackfillWarming,
  PulseWireApiError,
  requestChannelSpreadBackfill,
  type PulseWireChannelSpreadMeta,
  type PulseWireSourceHealth,
  type PulseWireStory,
} from '../../pulseWireApi'
import { formatRelativeTime } from '../../utils/pulseWireFormat'
import StoryCompactCard from '../pulsewire/StoryCompactCard'

const SPREAD_POLL_MS = 10_000
const SPREAD_POLL_MAX_MS = 90_000

const COMPACT_SOURCE_KEYS: { key: string; label: string }[] = [
  { key: 'reddit', label: 'Reddit' },
  { key: 'twitchclips', label: 'Clips' },
  { key: 'youtube', label: 'YouTube' },
]

const SOURCE_MODE_CLASS: Record<string, string> = {
  active: 'text-[#3FCB7E]',
  link_only: 'text-[#68B7FF]',
  off: 'text-[#ADADB8]',
  error: 'text-[#FF5C57]',
  degraded: 'text-[#FFD166]',
  deferred: 'text-[#FFB02E]',
}

function SpreadLoadingSkeleton({ subtitle }: { subtitle: string }) {
  return (
    <div className="rounded-lg border border-violet-400/20 bg-white/[0.035] px-4 py-5">
      <div className="flex items-center justify-center gap-2">
        <span className="relative flex h-2.5 w-2.5">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-violet-400/70 opacity-75" />
          <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-violet-500" />
        </span>
        <div className="text-sm font-black text-violet-200">Social spread loading</div>
      </div>
      <p className="mt-2 text-center text-xs font-semibold leading-relaxed text-zinc-500">{subtitle}</p>
      <div className="mt-4 space-y-3">
        {[0, 1].map(key => (
          <div key={key} className="flex gap-3 rounded-lg border border-white/5 bg-black/20 p-3">
            <div className="h-14 w-14 shrink-0 animate-pulse rounded bg-gradient-to-br from-violet-500/20 to-zinc-800/80" />
            <div className="min-w-0 flex-1 space-y-2">
              <div className="h-3 w-16 animate-pulse rounded bg-violet-500/20" />
              <div className="h-4 w-full animate-pulse rounded bg-white/10" />
              <div className="h-4 w-4/5 animate-pulse rounded bg-white/10" />
              <div className="h-3 w-24 animate-pulse rounded bg-white/5" />
            </div>
          </div>
        ))}
      </div>
      <p className="mt-3 text-center text-[11px] font-semibold text-zinc-600">Checking again automatically…</p>
    </div>
  )
}

function CompactSpreadSourceHealth({ sources }: { sources?: PulseWireSourceHealth }) {
  if (!sources) return null
  const rows = COMPACT_SOURCE_KEYS.filter(({ key }) => sources[key])
  if (!rows.length) {
    return (
      <p className="text-[11px] text-[#7A7A85]">Source health will appear after the first ingest poll.</p>
    )
  }
  return (
    <div className="flex flex-wrap gap-2">
      {rows.map(({ key, label }) => {
        const source = sources[key]
        const mode = source.mode ?? 'off'
        return (
          <span
            key={key}
            className="inline-flex items-center gap-1.5 rounded-md border border-[#2A2A2E] bg-[#101014] px-2 py-1 text-[11px] font-semibold text-[#ADADB8]"
          >
            <span>{label}</span>
            <span className={SOURCE_MODE_CLASS[mode] ?? SOURCE_MODE_CLASS.off}>
              {mode.replace(/_/g, ' ')}
            </span>
          </span>
        )
      })}
    </div>
  )
}

function SpreadEmptyState({
  login,
  meta,
  sourceHealth,
  backfillPending,
  backfillError,
  onBackfill,
}: {
  login: string
  meta?: PulseWireChannelSpreadMeta
  sourceHealth?: PulseWireSourceHealth
  backfillPending: boolean
  backfillError: string
  onBackfill: () => void
}) {
  const aliases = meta?.aliases?.filter(Boolean) ?? []
  const warming = backfillPending || isSpreadBackfillWarming(meta)

  if (warming) {
    return (
      <SpreadLoadingSkeleton
        subtitle={`Searching Reddit, clips, and unresolved mentions for ${login}…`}
      />
    )
  }

  return (
    <div className="rounded-lg border border-[#2A2A2E] bg-[#141418] p-4 space-y-3">
      <div>
        <h3 className="text-sm font-bold uppercase tracking-wider text-[#A970FF]">Social spread</h3>
        <p className="mt-1 text-xs text-[#7A7A85]">
          Cross-platform Pulse Wire stories linked to this streamer. LSF threads above stay the reliable Reddit source until parity is signed off.
        </p>
      </div>

      <CompactSpreadSourceHealth sources={sourceHealth} />

      <dl className="grid gap-1 text-[11px] text-[#ADADB8]">
        {meta?.lastIngestAt ? (
          <div className="flex flex-wrap gap-x-2">
            <dt className="text-[#7A7A85]">Last ingest</dt>
            <dd>{formatRelativeTime(meta.lastIngestAt)}</dd>
          </div>
        ) : null}
        {meta?.unresolvedMentionCount != null && meta.unresolvedMentionCount > 0 ? (
          <div className="flex flex-wrap gap-x-2">
            <dt className="text-[#7A7A85]">Unresolved mentions (48h)</dt>
            <dd>{meta.unresolvedMentionCount}</dd>
          </div>
        ) : null}
        {aliases.length ? (
          <div className="flex flex-wrap gap-x-2">
            <dt className="text-[#7A7A85]">Known aliases</dt>
            <dd>{aliases.join(', ')}</dd>
          </div>
        ) : meta?.entityKnown === false ? (
          <div className="text-[#7A7A85]">Entity not indexed yet — backfill may learn aliases from flair.</div>
        ) : null}
      </dl>

      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={() => onBackfill()}
          disabled={backfillPending}
          className="rounded-lg bg-[#9147FF] px-3 py-2 text-xs font-semibold text-white hover:bg-[#A970FF] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] disabled:cursor-not-allowed disabled:opacity-50"
        >
          Check for stories
        </button>
        <Link
          to="/pulse-wire"
          className="rounded-lg border border-[#33333A] bg-[#1F1F23] px-3 py-2 text-xs font-semibold text-[#ADADB8] hover:border-[#A970FF]/40 hover:text-[#EFEFF1] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
        >
          Open Pulse Wire
        </Link>
      </div>
      {backfillError ? <p className="text-xs text-red-300">{backfillError}</p> : null}
    </div>
  )
}

export default function SocialSpreadPanel({ login }: { login: string }) {
  const [items, setItems] = useState<PulseWireStory[]>([])
  const [probableItems, setProbableItems] = useState<PulseWireStory[]>([])
  const [meta, setMeta] = useState<PulseWireChannelSpreadMeta | undefined>()
  const [sourceHealth, setSourceHealth] = useState<PulseWireSourceHealth | undefined>()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [backfillPending, setBackfillPending] = useState(false)
  const [backfillError, setBackfillError] = useState('')
  const pollStartedAtRef = useRef<number | null>(null)

  const applySpread = useCallback((res: Awaited<ReturnType<typeof fetchChannelSpread>>) => {
    setItems(res.items ?? [])
    setProbableItems(res.probableItems ?? [])
    setMeta(res.meta)
    if (!isSpreadBackfillWarming(res.meta)) {
      setBackfillPending(false)
      pollStartedAtRef.current = null
    }
  }, [])

  const loadSpread = useCallback(async (signal?: AbortSignal) => {
    const res = await fetchChannelSpread(login, signal)
    applySpread(res)
    return res
  }, [applySpread, login])

  useEffect(() => {
    const controller = new AbortController()
    let cancelled = false
    setLoading(true)
    setError('')

    Promise.all([
      loadSpread(controller.signal),
      fetchSourceHealth(controller.signal).catch(() => null),
    ])
      .then(([, health]) => {
        if (cancelled) return
        if (health?.sources) setSourceHealth(health.sources)
      })
      .catch(e => {
        if (!cancelled && !controller.signal.aborted) {
          setError(e instanceof Error ? e.message : 'Spread unavailable')
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
      controller.abort()
    }
  }, [loadSpread])

  const warming = backfillPending || isSpreadBackfillWarming(meta)

  useEffect(() => {
    if (!warming) return undefined

    if (pollStartedAtRef.current == null) {
      pollStartedAtRef.current = Date.now()
    }

    const interval = window.setInterval(() => {
      const startedAt = pollStartedAtRef.current ?? Date.now()
      if (Date.now() - startedAt > SPREAD_POLL_MAX_MS) {
        setBackfillPending(false)
        pollStartedAtRef.current = null
        return
      }
      loadSpread().catch(() => undefined)
    }, SPREAD_POLL_MS)

    return () => window.clearInterval(interval)
  }, [warming, loadSpread])

  async function handleBackfill() {
    setBackfillError('')
    setBackfillPending(true)
    pollStartedAtRef.current = Date.now()
    setMeta(prev => ({
      ...prev,
      backfill: { state: 'warming', requestedAt: new Date().toISOString() },
    }))
    try {
      await requestChannelSpreadBackfill(login)
      await loadSpread()
    } catch (e) {
      setBackfillPending(false)
      pollStartedAtRef.current = null
      if (e instanceof PulseWireApiError) {
        setBackfillError(e.hint ? `${e.message} — ${e.hint}` : e.message)
      } else {
        setBackfillError(e instanceof Error ? e.message : 'Backfill failed')
      }
    }
  }

  if (loading) {
    return <SpreadLoadingSkeleton subtitle={`Loading cross-platform spread for ${login}…`} />
  }

  if (error && !items.length && !probableItems.length) {
    return (
      <div className="rounded-lg border border-[#2A2A2E] bg-[#141418] p-3 space-y-2">
        <p className="text-xs text-[#7A7A85]">Social spread: {error}</p>
        <button
          type="button"
          onClick={() => void handleBackfill()}
          className="rounded-lg border border-[#33333A] bg-[#1F1F23] px-3 py-2 text-xs font-semibold text-[#ADADB8] hover:border-[#A970FF]/40"
        >
          Check for stories
        </button>
      </div>
    )
  }

  const hasPrimary = items.length > 0
  const hasProbable = probableItems.length > 0

  if (!hasPrimary && !hasProbable) {
    return (
      <SpreadEmptyState
        login={login}
        meta={meta}
        sourceHealth={sourceHealth}
        backfillPending={backfillPending}
        backfillError={backfillError}
        onBackfill={() => void handleBackfill()}
      />
    )
  }

  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-sm font-bold uppercase tracking-wider text-[#A970FF]">Social spread</h3>
        <p className="mt-1 text-xs text-[#7A7A85]">Channel-scoped Pulse Wire stories tied to this streamer.</p>
      </div>

      {warming ? (
        <SpreadLoadingSkeleton subtitle={`Refreshing spread for ${login}…`} />
      ) : null}

      {hasPrimary ? (
        <div className="space-y-3">
          {items.map(item => (
            <StoryCompactCard key={item.story.id} story={item} variant="channel" />
          ))}
        </div>
      ) : null}

      {hasProbable ? (
        <section className="space-y-3 rounded-lg border border-[#2A2A2E]/80 bg-[#101014]/60 p-3">
          <div>
            <h4 className="text-xs font-bold uppercase tracking-wider text-[#7A7A85]">Possible matches</h4>
            <p className="mt-1 text-[11px] text-[#5A5A65]">
              Title or flair matches — not confirmed entity links.
            </p>
          </div>
          <div className="space-y-3 opacity-90">
            {probableItems.map(item => (
              <StoryCompactCard key={item.story.id} story={item} variant="channel" subdued />
            ))}
          </div>
        </section>
      ) : null}

      {!warming && !hasPrimary && hasProbable ? (
        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={() => void handleBackfill()}
            disabled={backfillPending}
            className="rounded-lg border border-[#33333A] bg-[#1F1F23] px-3 py-2 text-xs font-semibold text-[#ADADB8] hover:border-[#A970FF]/40"
          >
            Check for confirmed links
          </button>
          <Link
            to="/pulse-wire"
            className="text-xs font-semibold text-[#A970FF] hover:text-[#CDB4FF]"
          >
            Open Pulse Wire
          </Link>
        </div>
      ) : null}
    </div>
  )
}
