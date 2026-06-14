import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { getRandomStream, keepaliveStream, startStream, stopStream } from '../../api'
import { normalizeBrowserOriginUrl } from '../../config'
import { useHlsPlayback } from '../../playback'
import { useUiSettings } from '../../settings'

function thumb(url: string | undefined, w = 440, h = 248) {
  return (url ?? '').replace('{width}', String(w)).replace('{height}', String(h))
}

export function RandomLiveHero() {
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
