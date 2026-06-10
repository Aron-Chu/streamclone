import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Category, getCategories, getCategoryStreams, getRandomStream, getStreams, keepaliveStream, search, startStream, stopStream, Stream } from '../api'
import { useAuth } from '../auth'
import { normalizeBrowserOriginUrl } from '../config'
import { useHlsPlayback } from '../playback'
import { useThemeEffect, useUiSettings } from '../settings'
import ChannelRail from './ChannelRail'
import LocalTokenImportButton from './LocalTokenImportButton'
import SettingsButton from './SettingsPanel'

const W = 440
const H = 248

function thumb(url: string | undefined, w = W, h = H) {
  return (url ?? '').replace('{width}', String(w)).replace('{height}', String(h))
}

function StreamCard({ stream, index }: { stream: Stream; index: number }) {
  const isLive = stream.isLive ?? Boolean(stream.thumbnailUrl && (stream.viewers ?? 0) > 0)
  const title = stream.title || stream.displayName || stream.login
  const previewUrl = stream.thumbnailUrl || stream.profileImageUrl
  return (
    <Link
      to={`/c/${stream.login}`}
      className="group block overflow-hidden rounded-lg border border-white/10 bg-white/[0.045] shadow-2xl shadow-black/20 transition duration-300 hover:-translate-y-1 hover:border-violet-400/60 hover:bg-white/[0.07]"
      style={{ animationDelay: `${Math.min(index, 10) * 35}ms` }}
    >
      <div className="relative aspect-video overflow-hidden bg-zinc-900">
        {previewUrl ? (
          isLive ? (
            <img
              src={thumb(stream.thumbnailUrl || previewUrl)}
              alt={title}
              className="h-full w-full object-cover transition duration-500 group-hover:scale-105"
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center bg-[linear-gradient(135deg,#202027,#050507)]">
              <img
                src={thumb(stream.profileImageUrl, 140, 140)}
                alt={stream.displayName || stream.login}
                className="h-24 w-24 rounded-full border border-white/10 object-cover shadow-lg"
              />
            </div>
          )
        ) : (
          <div className="h-full w-full bg-[linear-gradient(135deg,#202027,#050507)]" />
        )}
        <div className={`absolute left-3 top-3 rounded px-2 py-0.5 text-[11px] font-black uppercase tracking-wide text-white shadow-lg ${
          isLive ? 'bg-red-600' : 'bg-zinc-700'
        }`}>
          {isLive ? 'Live' : 'Offline'}
        </div>
        {isLive ? (
          <div className="absolute bottom-3 right-3 rounded bg-black/75 px-2 py-0.5 text-xs font-semibold text-zinc-100 backdrop-blur">
            {(stream.viewers ?? 0).toLocaleString()} viewers
          </div>
        ) : null}
      </div>
      <div className="space-y-1 p-3">
        <div className="line-clamp-2 min-h-10 text-sm font-bold leading-5 text-zinc-50">{title}</div>
        <div className="flex items-center justify-between gap-2 text-xs text-zinc-400">
          <span className="truncate font-semibold text-violet-200">{stream.displayName || stream.login}</span>
          <span className="truncate text-zinc-500">{isLive ? (stream.category || 'Live') : 'Offline'}</span>
        </div>
      </div>
    </Link>
  )
}

function CategoryPill({
  category,
  selected,
  onClick,
}: {
  category: Category
  selected: boolean
  onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      className={`group flex min-w-44 items-center gap-3 rounded-lg border px-3 py-2 text-left transition duration-300 ${
        selected
          ? 'border-violet-400 bg-violet-500/20 text-white shadow-lg shadow-violet-950/30'
          : 'border-white/10 bg-white/[0.045] text-zinc-300 hover:border-cyan-300/50 hover:bg-white/[0.08]'
      }`}
    >
      <div className="h-12 w-9 shrink-0 overflow-hidden rounded bg-zinc-800">
        {category.thumbnailUrl ? (
          <img src={thumb(category.thumbnailUrl, 72, 96)} alt={category.name} className="h-full w-full object-cover transition duration-300 group-hover:scale-110" />
        ) : null}
      </div>
      <span className="line-clamp-2 text-sm font-bold">{category.name}</span>
    </button>
  )
}

function SkeletonGrid() {
  return (
    <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 xl:grid-cols-4">
      {Array.from({ length: 8 }).map((_, i) => (
        <div key={i} className="overflow-hidden rounded-lg border border-white/10 bg-white/[0.04]">
          <div className="aspect-video animate-pulse bg-white/10" />
          <div className="space-y-2 p-3">
            <div className="h-4 w-5/6 animate-pulse rounded bg-white/10" />
            <div className="h-3 w-2/3 animate-pulse rounded bg-white/10" />
          </div>
        </div>
      ))}
    </div>
  )
}

function RandomLiveHero() {
  const videoRef = useRef<HTMLVideoElement>(null)
  const sessionRef = useRef<{ channel: string; sessionId?: string } | null>(null)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const settings = useUiSettings(s => s.settings)
  const updateSettings = useUiSettings(s => s.updateSettings)
  const [previewEnabled, setPreviewEnabled] = useState(settings.previewAutoplay)
  const [hlsUrl, setHlsUrl] = useState('')
  const [relayStatus, setRelayStatus] = useState<'idle' | 'loading' | 'playing' | 'error'>('idle')
  const [error, setError] = useState<string | null>(null)
  const playback = useHlsPlayback(videoRef, { src: hlsUrl, enabled: Boolean(hlsUrl && previewEnabled), muted: settings.previewMuted, autoPlay: true })

  const random = useQuery({
    queryKey: ['random-stream'],
    queryFn: () => getRandomStream(20000),
    staleTime: 0,
  })
  const stream = random.data?.stream

  const stopPreview = async () => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current)
      intervalRef.current = null
    }
    setHlsUrl('')
    const session = sessionRef.current
    sessionRef.current = null
    if (session) {
      await stopStream(session.channel, session.sessionId).catch(() => undefined)
    }
  }

  useEffect(() => {
    setPreviewEnabled(settings.previewAutoplay)
  }, [settings.previewAutoplay])

  useEffect(() => {
    let alive = true
    if (!stream?.login || !previewEnabled) {
      setRelayStatus('idle')
      stopPreview().catch(() => undefined)
      return () => {
        alive = false
      }
    }

    const start = async () => {
      setRelayStatus('loading')
      setError(null)
      await stopPreview()
      try {
        const response = await startStream(stream.login, '480p,360p,best')
        if (!alive) {
          await stopStream(stream.login, response.session_id).catch(() => undefined)
          return
        }
        sessionRef.current = { channel: stream.login, sessionId: response.session_id }
        setHlsUrl(normalizeBrowserOriginUrl(response.hlsUrl, ['/live/']))
        setRelayStatus('playing')
        intervalRef.current = setInterval(() => {
          keepaliveStream(stream.login, sessionRef.current?.sessionId).catch(() => undefined)
        }, 20000)
      } catch (err) {
        if (alive) {
          setRelayStatus('error')
          setError((err as Error).message || 'preview failed')
        }
      }
    }
    start()

    return () => {
      alive = false
      stopPreview().catch(() => undefined)
    }
  }, [stream?.login, previewEnabled, settings.previewMuted])

  const next = async () => {
    setPreviewEnabled(settings.previewAutoplay)
    await stopPreview()
    random.refetch().catch(() => undefined)
  }

  const title = stream?.title || stream?.displayName || stream?.login || 'Finding a live channel'
  const previewImage = stream?.thumbnailUrl ? thumb(stream.thumbnailUrl, 960, 540) : ''
  const status = error ? 'error' : hlsUrl ? playback.state : relayStatus
  const previewError = error || playback.error

  return (
    <section className="overflow-hidden rounded-lg border border-white/10 bg-white/[0.045] shadow-2xl shadow-black/30">
      <div className="grid min-h-[360px] lg:grid-cols-[minmax(0,1.55fr)_minmax(320px,.9fr)]">
        <div className="relative min-h-[260px] bg-black">
          {previewImage ? <img src={previewImage} alt="" className="absolute inset-0 h-full w-full object-cover opacity-30 blur-sm" /> : null}
          <video ref={videoRef} className={`relative z-10 h-full w-full bg-black object-contain ${previewError ? 'opacity-0' : ''}`} muted={settings.previewMuted} playsInline autoPlay poster={previewImage || undefined} />
          <div className="absolute left-4 top-4 z-20 flex flex-wrap items-center gap-2">
            <span className="rounded bg-red-600 px-2 py-1 text-[11px] font-black uppercase text-white">Random live</span>
            <span className="rounded bg-black/70 px-2 py-1 text-[11px] font-black uppercase text-zinc-200">{status}</span>
            {random.data?.poolSize ? <span className="rounded bg-black/70 px-2 py-1 text-[11px] font-black uppercase text-zinc-200">{random.data.poolSize.toLocaleString()} pool</span> : null}
          </div>
          {previewError || status !== 'playing' ? (
            <div className="absolute inset-0 z-10 grid place-items-center bg-black/45">
              {previewImage && previewError ? (
                <img src={previewImage} alt="" className="absolute inset-0 h-full w-full object-contain opacity-40" />
              ) : null}
              <div className="relative rounded border border-white/10 bg-zinc-950/85 px-5 py-4 text-center shadow-2xl">
                <div className="text-sm font-black text-white">{previewError ? 'Preview unavailable' : random.isLoading ? 'Finding stream' : previewEnabled ? 'Preview loading' : 'Preview paused'}</div>
                <div className="mt-1 max-w-xs text-xs font-semibold text-zinc-400">
                  {previewError ? `${previewError} — showing stream thumbnail instead of live video.` : (previewEnabled ? `Relay ${playback.metrics.hlsStage}` : 'Autoplay is off.')}
                </div>
              </div>
            </div>
          ) : null}
        </div>
        <div className="flex flex-col justify-between gap-6 p-5 sm:p-6">
          <div>
            <div className="mb-3 flex items-center gap-3">
              <div className="grid h-12 w-12 shrink-0 place-items-center overflow-hidden rounded-full bg-white/10 text-sm font-black text-violet-100">
                {previewImage ? <img src={previewImage} alt="" className="h-full w-full object-cover" /> : stream?.login?.slice(0, 1).toUpperCase()}
              </div>
              <div className="min-w-0">
                <div className="truncate text-lg font-black text-white">{stream?.displayName || stream?.login || 'Live channel'}</div>
                <div className="truncate text-sm font-semibold text-zinc-400">{stream?.category || 'Top live'}</div>
              </div>
            </div>
            <h2 className="line-clamp-2 text-2xl font-black leading-tight text-white sm:text-3xl">{title}</h2>
            <div className="mt-4 grid grid-cols-2 gap-2 text-sm font-bold text-zinc-300">
              <div className="rounded border border-white/10 bg-white/[0.05] px-3 py-2">
                <div className="text-[11px] uppercase text-zinc-500">Viewers</div>
                <div className="text-base text-white">{(stream?.viewers ?? 0).toLocaleString()}</div>
              </div>
              <div className="rounded border border-white/10 bg-white/[0.05] px-3 py-2">
                <div className="text-[11px] uppercase text-zinc-500">Relay</div>
                <div className="text-base text-white">{previewEnabled ? 'Local HLS' : 'Paused'}</div>
              </div>
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={() => setPreviewEnabled(value => !value)}
              className="rounded bg-violet-500 px-4 py-2 text-sm font-black text-white transition hover:bg-violet-400"
            >
              {previewEnabled ? 'Pause' : 'Start'}
            </button>
            <button
              type="button"
              onClick={() => updateSettings({ previewMuted: !settings.previewMuted })}
              className="rounded border border-white/10 bg-white/[0.06] px-4 py-2 text-sm font-black text-zinc-200 transition hover:bg-white/10"
            >
              {settings.previewMuted ? 'Unmute' : 'Mute'}
            </button>
            <button
              type="button"
              onClick={next}
              className="rounded border border-white/10 bg-white/[0.06] px-4 py-2 text-sm font-black text-zinc-200 transition hover:bg-white/10"
            >
              Next
            </button>
            {stream?.login ? (
              <Link to={`/c/${stream.login}`} className="rounded border border-cyan-300/30 bg-cyan-400/10 px-4 py-2 text-sm font-black text-cyan-100 transition hover:bg-cyan-400/20">
                Open channel
              </Link>
            ) : null}
          </div>
        </div>
      </div>
    </section>
  )
}

function HeaderAuth() {
  const auth = useAuth()
  if (auth.isAuthenticated) {
    return (
      <div className="flex shrink-0 items-center gap-2">
        <div className="hidden min-w-0 text-right sm:block">
          <div className="max-w-32 truncate text-xs font-black text-white">{auth.user?.displayName || auth.user?.display_name || auth.user?.login}</div>
          <div className="text-[11px] font-semibold text-emerald-300">Connected</div>
        </div>
        <button onClick={auth.logout} className="rounded border border-white/10 bg-white/[0.06] px-3 py-2 text-xs font-black text-zinc-200 transition hover:bg-white/10">
          Log out
        </button>
      </div>
    )
  }
  return (
    <div className="flex shrink-0 items-center gap-2">
      <LocalTokenImportButton compact />
    </div>
  )
}

export default function Directory() {
  const [q, setQ] = useState('')
  const [selectedCategory, setSelectedCategory] = useState<Category | null>(null)
  const [mobileRailOpen, setMobileRailOpen] = useState(false)
  const [railCollapsed, setRailCollapsed] = useState(false)
  const settings = useUiSettings(s => s.settings)
  const query = q.trim()
  useThemeEffect(settings.theme)

  const streams = useQuery<Stream[]>({
    queryKey: ['streams'],
    queryFn: getStreams,
  })

  const categories = useQuery<Category[]>({
    queryKey: ['categories'],
    queryFn: getCategories,
  })

  const categoryStreams = useQuery<Stream[]>({
    queryKey: ['category-streams', selectedCategory?.id],
    queryFn: () => getCategoryStreams(selectedCategory!.id),
    enabled: Boolean(selectedCategory && !query),
  })

  const searchResults = useQuery({
    queryKey: ['search', query],
    queryFn: () => search(query),
    enabled: query.length > 0,
  })

  const shownCategories = useMemo(() => {
    if (query) return searchResults.data?.categories ?? []
    return categories.data ?? []
  }, [categories.data, query, searchResults.data?.categories])

  const shownStreams = useMemo(() => {
    if (query) return searchResults.data?.streams ?? []
    if (selectedCategory) return categoryStreams.data ?? []
    return streams.data ?? []
  }, [categoryStreams.data, query, searchResults.data?.streams, selectedCategory, streams.data])

  const loading = streams.isLoading || categories.isLoading || categoryStreams.isLoading || searchResults.isLoading
  const error = streams.error || categories.error || categoryStreams.error || searchResults.error

  return (
    <main className="min-h-screen overflow-hidden bg-[#07070a] text-zinc-100">
      <div className="pointer-events-none fixed inset-0 bg-[linear-gradient(135deg,rgba(139,92,246,.16),transparent_28%),linear-gradient(180deg,rgba(255,255,255,.045),transparent_34%)]" />
      <div className="relative flex min-h-screen">
        <ChannelRail
          collapsed={railCollapsed}
          mobileOpen={mobileRailOpen}
          onToggleCollapsed={() => setRailCollapsed(v => !v)}
          onCloseMobile={() => setMobileRailOpen(false)}
        />
        <div className="min-w-0 flex-1 overflow-hidden">
      <div className="mx-auto flex min-h-screen w-full max-w-7xl flex-col px-4 py-5 sm:px-6 lg:px-8">
        <header className="sticky top-0 z-20 -mx-4 border-b border-white/10 bg-[#07070a]/85 px-4 py-4 backdrop-blur-xl sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
            <div className="flex items-center justify-between gap-3">
              <div className="flex min-w-0 items-center gap-3">
              <button onClick={() => setMobileRailOpen(true)} className="rounded border border-white/10 bg-white/[0.06] px-3 py-2 text-sm font-black text-white lg:hidden">
                Menu
              </button>
              <Link to="/" className="flex items-center gap-3">
                <span className="grid h-10 w-10 place-items-center rounded-lg bg-violet-500 text-lg font-black text-white shadow-lg shadow-violet-950/40">7</span>
                <div>
                  <div className="text-lg font-black tracking-tight">Streamclone</div>
                  <div className="text-xs font-semibold uppercase tracking-[0.18em] text-cyan-200/80">Live directory</div>
                </div>
              </Link>
              </div>
              <div className="flex items-center gap-2 lg:hidden">
                <SettingsButton />
                <HeaderAuth />
              </div>
            </div>
            <div className="flex w-full items-center gap-3 lg:max-w-3xl">
            <div className="relative min-w-0 flex-1">
              <input
                className="w-full rounded-lg border border-white/10 bg-white/[0.07] px-4 py-3 text-sm font-semibold text-white outline-none transition placeholder:text-zinc-500 focus:border-violet-300 focus:bg-white/[0.1] focus:ring-4 focus:ring-violet-500/15"
                placeholder="Search channels or categories"
                spellCheck={false}
                autoCorrect="off"
                autoCapitalize="off"
                value={q}
                onChange={e => {
                  setQ(e.target.value)
                  if (e.target.value.trim()) setSelectedCategory(null)
                }}
              />
              {query ? (
                <button
                  onClick={() => setQ('')}
                  className="absolute right-2 top-1/2 -translate-y-1/2 rounded px-2 py-1 text-xs font-bold text-zinc-300 transition hover:bg-white/10 hover:text-white"
                >
                  Clear
                </button>
              ) : null}
            </div>
            <div className="hidden lg:block">
              <div className="flex items-center gap-2">
                <SettingsButton />
                <HeaderAuth />
              </div>
            </div>
            </div>
          </div>
        </header>

        <section className="flex flex-1 flex-col gap-6 py-6">
          {!query && !selectedCategory ? <RandomLiveHero /> : null}

          <div className="flex flex-col gap-3">
            <div className="flex items-end justify-between gap-4">
              <div>
                <h1 className="text-3xl font-black tracking-tight sm:text-4xl">{query ? `Search: ${query}` : selectedCategory ? selectedCategory.name : 'Live channels'}</h1>
                <p className="mt-1 text-sm font-medium text-zinc-400">{selectedCategory && !query ? 'Category streams' : 'Streams, games, and chat-ready channels'}</p>
              </div>
              {selectedCategory && !query ? (
                <button onClick={() => setSelectedCategory(null)} className="rounded-lg border border-white/10 bg-white/[0.06] px-3 py-2 text-sm font-bold text-zinc-200 transition hover:border-violet-300 hover:bg-white/[0.1]">
                  All live
                </button>
              ) : null}
            </div>
          </div>

          {shownCategories.length ? (
            <div className="flex gap-3 overflow-x-auto pb-2">
              {shownCategories.map(category => (
                <CategoryPill
                  key={category.id}
                  category={category}
                  selected={selectedCategory?.id === category.id}
                  onClick={() => {
                    setQ('')
                    setSelectedCategory(category)
                  }}
                />
              ))}
            </div>
          ) : null}

          {error ? (
            <div className="rounded-lg border border-red-400/30 bg-red-500/10 p-5 text-sm font-semibold text-red-100">
              Metadata service is not responding yet.
            </div>
          ) : loading ? (
            <SkeletonGrid />
          ) : shownStreams.length ? (
            <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 xl:grid-cols-4">
              {shownStreams.map((s, i) => (
                <StreamCard key={`${s.login}-${s.id ?? i}`} stream={s} index={i} />
              ))}
            </div>
          ) : (
            <div className="grid min-h-72 place-items-center rounded-lg border border-white/10 bg-white/[0.04] text-center">
              <div>
                <div className="text-lg font-black text-white">Nothing live here yet</div>
                <div className="mt-1 text-sm text-zinc-400">Try a different search or category.</div>
              </div>
            </div>
          )}
        </section>
      </div>
        </div>
      </div>
    </main>
  )
}
