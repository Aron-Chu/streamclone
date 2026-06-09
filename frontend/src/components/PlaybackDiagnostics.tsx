import { useEffect, useMemo, useState } from 'react'
import type { StreamDiagnostics } from '../api'
import type { PlaybackMetrics } from '../playback'

type TwitchPlayerInstance = {
  getPlaybackStats?: () => Record<string, unknown>
  setMuted?: (muted: boolean) => void
  pause?: () => void
}

declare global {
  interface Window {
    Twitch?: {
      Player: new (target: string, options: Record<string, unknown>) => TwitchPlayerInstance
    }
  }
}

interface TwitchStatsInput {
  downloadBitrateKbps: string
  bandwidthEstimateKbps: string
  fps: string
  skippedFrames: string
  bufferSizeSec: string
  latencyToBroadcasterSec: string
}

const blankStats: TwitchStatsInput = {
  downloadBitrateKbps: '',
  bandwidthEstimateKbps: '',
  fps: '',
  skippedFrames: '',
  bufferSizeSec: '',
  latencyToBroadcasterSec: '',
}

let twitchEmbedScriptPromise: Promise<void> | null = null

function fmt(value: number | null | undefined, unit = '') {
  if (value === null || value === undefined || Number.isNaN(value)) return '-'
  return `${value}${unit}`
}

function fmtSec(value: number | null | undefined) {
  if (value === null || value === undefined || Number.isNaN(value)) return '-'
  return `${value.toFixed(2)}s`
}

function parseNum(value: string) {
  const n = Number(value)
  return Number.isFinite(n) ? n : null
}

function delta(local: number | null | undefined, remote: string, suffix = '') {
  const remoteValue = parseNum(remote)
  if (local === null || local === undefined || remoteValue === null) return '-'
  const diff = local - remoteValue
  const sign = diff > 0 ? '+' : ''
  return `${sign}${diff.toFixed(Math.abs(diff) < 10 ? 2 : 0)}${suffix}`
}

function delayDelta(local: number | null | undefined, twitch: number | null | undefined) {
  if (local === null || local === undefined || twitch === null || twitch === undefined) return '-'
  const diff = local - twitch
  const sign = diff > 0 ? '+' : ''
  return `${sign}${diff.toFixed(Math.abs(diff) < 10 ? 2 : 0)}s`
}

function loadTwitchEmbedScript() {
  if (typeof window === 'undefined') return Promise.reject(new Error('browser required'))
  if (window.Twitch?.Player) return Promise.resolve()
  if (twitchEmbedScriptPromise) return twitchEmbedScriptPromise
  twitchEmbedScriptPromise = new Promise((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>('script[data-streamclone-twitch-embed]')
    if (existing) {
      existing.addEventListener('load', () => resolve(), { once: true })
      existing.addEventListener('error', () => reject(new Error('twitch embed script failed')), { once: true })
      return
    }
    const script = document.createElement('script')
    script.src = 'https://player.twitch.tv/js/embed/v1.js'
    script.async = true
    script.dataset.streamcloneTwitchEmbed = 'true'
    script.onload = () => resolve()
    script.onerror = () => reject(new Error('twitch embed script failed'))
    document.head.appendChild(script)
  })
  return twitchEmbedScriptPromise
}

function MetricCell({ label, value, title }: { label: string; value: string; title?: string }) {
  return (
    <div title={title} className="rounded border border-white/10 bg-white/[0.045] px-3 py-2">
      <div className="text-[10px] font-black uppercase text-zinc-500">{label}</div>
      <div className="mt-1 truncate text-sm font-black text-white">{value}</div>
    </div>
  )
}

export default function PlaybackDiagnostics({
  channel,
  metrics,
  diagnostics,
  sessionId,
  onJumpLive,
}: {
  channel: string
  metrics: PlaybackMetrics
  diagnostics?: StreamDiagnostics
  sessionId?: string
  onJumpLive?: () => void
}) {
  const storageKey = `streamclone:twitch-stats:${channel}`
  const [open, setOpen] = useState(false)
  const [referenceEnabled, setReferenceEnabled] = useState(false)
  const [referenceStatus, setReferenceStatus] = useState('idle')
  const [referenceLatency, setReferenceLatency] = useState<number | null>(null)
  const [input, setInput] = useState<TwitchStatsInput>(() => {
    try {
      return { ...blankStats, ...JSON.parse(localStorage.getItem(storageKey) || '{}') }
    } catch {
      return blankStats
    }
  })

  useEffect(() => {
    try {
      localStorage.setItem(storageKey, JSON.stringify(input))
    } catch {
      return
    }
  }, [input, storageKey])

  const referenceId = `streamclone-twitch-ref-${channel.replace(/[^a-z0-9_-]/gi, '-') || 'channel'}`

  useEffect(() => {
    if (!referenceEnabled) return
    let alive = true
    let intervalId: ReturnType<typeof setInterval> | null = null
    let player: TwitchPlayerInstance | null = null
    setReferenceStatus('loading')
    loadTwitchEmbedScript()
      .then(() => {
        if (!alive) return
        if (!window.Twitch?.Player) throw new Error('twitch player unavailable')
        player = new window.Twitch.Player(referenceId, {
          channel,
          muted: true,
          autoplay: true,
          width: 1,
          height: 1,
          parent: [window.location.hostname],
        })
        player.setMuted?.(true)
        setReferenceStatus('waiting for stats')
        intervalId = setInterval(() => {
          const stats = player?.getPlaybackStats?.()
          const latency = Number(stats?.hlsLatencyBroadcaster)
          if (Number.isFinite(latency)) {
            setReferenceLatency(Number(latency.toFixed(2)))
            setReferenceStatus('ready')
          }
        }, 2000)
      })
      .catch(err => {
        if (!alive) return
        setReferenceStatus((err as Error).message || 'reference unavailable')
      })
    return () => {
      alive = false
      if (intervalId) clearInterval(intervalId)
      player?.pause?.()
    }
  }, [channel, referenceEnabled, referenceId])

  const manualLatency = parseNum(input.latencyToBroadcasterSec)
  const actualTwitchLatencySec = referenceLatency ?? manualLatency
  const localLiveLatencySec = metrics.latencyToLiveSec ?? metrics.behindLiveSec

  const benchmark = useMemo(() => ({
    channel,
    sessionId,
    local: metrics,
    relay: diagnostics,
    twitch: {
      manual: input,
      referenceLatencySec: referenceLatency,
      actualTwitchLatencySec,
      delayVsTwitchSec: localLiveLatencySec === null || localLiveLatencySec === undefined || actualTwitchLatencySec === null ? null : Number((localLiveLatencySec - actualTwitchLatencySec).toFixed(2)),
    },
    capturedAt: new Date().toISOString(),
  }), [actualTwitchLatencySec, channel, diagnostics, input, localLiveLatencySec, metrics, referenceLatency, sessionId])

  const copyBenchmark = () => {
    navigator.clipboard?.writeText(JSON.stringify(benchmark, null, 2)).catch(() => undefined)
  }

  return (
    <section className="border-t border-white/10 bg-[#08080b] px-4 py-3 lg:px-6">
      <div className="flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
        <div className="grid flex-1 grid-cols-2 gap-2 md:grid-cols-4 2xl:grid-cols-6">
          <MetricCell label="Download" value={metrics.downloadResolution} />
          <MetricCell label="Render" value={metrics.renderResolution} />
          <MetricCell label="Viewport" value={metrics.viewportResolution} />
          <MetricCell label="Bitrate" value={fmt(metrics.downloadBitrateKbps, ' Kbps')} />
          <MetricCell label="Bandwidth" value={fmt(metrics.bandwidthEstimateKbps, ' Kbps')} />
          <MetricCell label="FPS" value={fmt(metrics.fps)} />
          <MetricCell label="Skipped" value={`${fmt(metrics.skippedFrames)} / ${fmt(metrics.totalFrames)}`} />
          <MetricCell label="Buffer" value={fmtSec(metrics.bufferSizeSec)} />
          <MetricCell label="Live latency" value={fmtSec(metrics.latencyToLiveSec)} />
          <MetricCell label="Target" value={fmtSec(metrics.targetLatencySec)} />
          <MetricCell label="Behind live" value={fmtSec(metrics.behindLiveSec)} />
          <MetricCell label="Seekable end" value={fmtSec(metrics.seekableEndSec)} />
          <MetricCell label="Live sync" value={fmtSec(metrics.liveSyncPositionSec)} />
          <MetricCell label="Twitch delay" value={fmtSec(actualTwitchLatencySec)} title={referenceLatency !== null ? 'Read from Twitch embed hlsLatencyBroadcaster' : 'Manual comparison value'} />
          <MetricCell label="Local minus Twitch" value={delayDelta(localLiveLatencySec, actualTwitchLatencySec)} />
          <MetricCell label="Relay restarts" value={String(diagnostics?.restarts ?? 0)} title={diagnostics?.lastWorkerError} />
          <MetricCell label="Backend" value={diagnostics?.workerBackend || '-'} title={diagnostics?.lastStartError} />
          <MetricCell label="Rendition" value={diagnostics?.selectedRendition?.name || diagnostics?.quality || '-'} />
          <MetricCell label="Startup" value={diagnostics?.startupMs ? `${diagnostics.startupMs}ms` : '-'} />
          <MetricCell label="Upstream" value={diagnostics?.startupBreakdown?.upstreamFetchMs ? `${diagnostics.startupBreakdown.upstreamFetchMs}ms` : '-'} />
          <MetricCell label="Spawn" value={diagnostics?.startupBreakdown?.workerSpawnMs ? `${diagnostics.startupBreakdown.workerSpawnMs}ms` : '-'} />
          <MetricCell label="HLS ready" value={diagnostics?.startupBreakdown?.hlsReadyMs ? `${diagnostics.startupBreakdown.hlsReadyMs}ms` : '-'} />
          <MetricCell label="Fallbacks" value={String(diagnostics?.fallbackAttempts ?? 0)} />
          <MetricCell label="Recovery" value={String(metrics.recoveryAttempts)} />
          <MetricCell label="Stalls" value={String(metrics.stalls)} />
          <MetricCell label="First frame" value={metrics.firstFrameMs === null ? '-' : `${metrics.firstFrameMs}ms`} />
          <MetricCell label="Session" value={sessionId?.slice(0, 8) || '-'} />
        </div>
        <div className="flex shrink-0 flex-wrap gap-2">
          <button
            type="button"
            onClick={() => setOpen(value => !value)}
            className="rounded border border-white/10 bg-white/[0.06] px-3 py-2 text-xs font-black text-zinc-200 transition hover:bg-white/10"
          >
            Compare Twitch
          </button>
          <button
            type="button"
            onClick={onJumpLive}
            disabled={!onJumpLive || !metrics.canJumpLive}
            className="rounded border border-cyan-300/30 bg-cyan-400/10 px-3 py-2 text-xs font-black text-cyan-100 transition hover:bg-cyan-400/20 disabled:cursor-not-allowed disabled:border-white/10 disabled:bg-white/[0.04] disabled:text-zinc-500"
          >
            Jump Live
          </button>
          <button
            type="button"
            onClick={copyBenchmark}
            className="rounded border border-cyan-300/30 bg-cyan-400/10 px-3 py-2 text-xs font-black text-cyan-100 transition hover:bg-cyan-400/20"
          >
            Copy JSON
          </button>
        </div>
      </div>
      <div className="mt-2 flex flex-wrap gap-2 text-[11px] font-bold text-zinc-500">
        <span>Stage {metrics.hlsStage}</span>
        <span>Codecs {metrics.codecs}</span>
        <span>Protocol {diagnostics?.protocol || metrics.protocol}</span>
        <span>Latency {diagnostics?.latencyMode || metrics.latencyMode}</span>
        <span>Backend {diagnostics?.backendVersion || '-'}</span>
        <span>Probe {diagnostics?.hlsProbe?.ready ? 'ready' : diagnostics?.hlsProbe?.error || diagnostics?.hlsProbe?.statusCode || '-'}</span>
      </div>
      {open ? (
        <div className="mt-3 rounded border border-white/10 bg-white/[0.035] p-3">
          <div className="mb-3 flex items-center justify-between gap-3">
            <div>
              <div className="text-sm font-black text-white">Actual Twitch comparison</div>
              <div className="mt-1 text-xs font-semibold text-zinc-500">Use a hidden Twitch reference player when embeds allow it, or paste official Twitch stats values.</div>
            </div>
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                onClick={() => setReferenceEnabled(value => !value)}
                className="rounded border border-white/10 bg-white/[0.06] px-2 py-1 text-xs font-black text-zinc-300 transition hover:bg-white/10 hover:text-white"
              >
                {referenceEnabled ? 'Stop ref' : 'Start ref'}
              </button>
              <button
                type="button"
                onClick={() => setInput(blankStats)}
                className="rounded px-2 py-1 text-xs font-black text-zinc-400 transition hover:bg-white/10 hover:text-white"
              >
                Clear
              </button>
            </div>
          </div>
          <div className="mb-3 flex flex-wrap gap-2 text-[11px] font-bold text-zinc-500">
            <span className="rounded bg-white/[0.045] px-2 py-1">Reference {referenceStatus}</span>
            <span className="rounded bg-white/[0.045] px-2 py-1">Twitch {fmtSec(referenceLatency)}</span>
            <span className="rounded bg-white/[0.045] px-2 py-1">Delay {delayDelta(localLiveLatencySec, actualTwitchLatencySec)}</span>
          </div>
          {referenceEnabled ? <div id={referenceId} className="pointer-events-none absolute h-px w-px overflow-hidden opacity-0" /> : null}
          <div className="grid gap-2 md:grid-cols-3 xl:grid-cols-6">
            <CompareInput label="Bitrate Kbps" value={input.downloadBitrateKbps} local={fmt(metrics.downloadBitrateKbps)} diff={delta(metrics.downloadBitrateKbps, input.downloadBitrateKbps, ' Kbps')} onChange={value => setInput(current => ({ ...current, downloadBitrateKbps: value }))} />
            <CompareInput label="Bandwidth Kbps" value={input.bandwidthEstimateKbps} local={fmt(metrics.bandwidthEstimateKbps)} diff={delta(metrics.bandwidthEstimateKbps, input.bandwidthEstimateKbps, ' Kbps')} onChange={value => setInput(current => ({ ...current, bandwidthEstimateKbps: value }))} />
            <CompareInput label="FPS" value={input.fps} local={fmt(metrics.fps)} diff={delta(metrics.fps, input.fps)} onChange={value => setInput(current => ({ ...current, fps: value }))} />
            <CompareInput label="Skipped" value={input.skippedFrames} local={fmt(metrics.skippedFrames)} diff={delta(metrics.skippedFrames, input.skippedFrames)} onChange={value => setInput(current => ({ ...current, skippedFrames: value }))} />
            <CompareInput label="Buffer sec" value={input.bufferSizeSec} local={fmtSec(metrics.bufferSizeSec)} diff={delta(metrics.bufferSizeSec, input.bufferSizeSec, 's')} onChange={value => setInput(current => ({ ...current, bufferSizeSec: value }))} />
            <CompareInput label="Latency sec" value={input.latencyToBroadcasterSec} local={fmtSec(metrics.latencyToLiveSec)} diff={delta(metrics.latencyToLiveSec, input.latencyToBroadcasterSec, 's')} onChange={value => setInput(current => ({ ...current, latencyToBroadcasterSec: value }))} />
          </div>
        </div>
      ) : null}
    </section>
  )
}

function CompareInput({
  label,
  value,
  local,
  diff,
  onChange,
}: {
  label: string
  value: string
  local: string
  diff: string
  onChange: (value: string) => void
}) {
  return (
    <label className="block rounded border border-white/10 bg-black/20 p-2">
      <span className="text-[10px] font-black uppercase text-zinc-500">{label}</span>
      <input
        value={value}
        onChange={event => onChange(event.target.value)}
        inputMode="decimal"
        className="mt-1 w-full rounded border border-white/10 bg-white/[0.06] px-2 py-1.5 text-sm font-bold text-white outline-none focus:border-violet-300"
        placeholder="Twitch"
      />
      <span className="mt-1 flex items-center justify-between gap-2 text-[11px] font-bold text-zinc-500">
        <span>local {local}</span>
        <span className="text-zinc-300">{diff}</span>
      </span>
    </label>
  )
}
