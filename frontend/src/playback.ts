import { RefObject, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type Hls from 'hls.js'
import { calculateLiveEdge } from './playbackMath'
import type { PlaybackLatencyMode } from './settings'

export type PlaybackState = 'idle' | 'starting' | 'buffering' | 'playing' | 'retrying' | 'error'

export interface PlaybackMetrics {
  downloadResolution: string
  renderResolution: string
  viewportResolution: string
  downloadBitrateKbps: number | null
  bandwidthEstimateKbps: number | null
  fps: number | null
  skippedFrames: number | null
  totalFrames: number | null
  bufferSizeSec: number | null
  latencyToLiveSec: number | null
  targetLatencySec: number | null
  currentTimeSec: number | null
  liveSyncPositionSec: number | null
  seekableEndSec: number | null
  behindLiveSec: number | null
  actualTwitchLatencySec: number | null
  delayVsTwitchSec: number | null
  canJumpLive: boolean
  codecs: string
  protocol: string
  latencyMode: string
  renderSurface: string
  hlsStage: string
  recoveryAttempts: number
  stalls: number
  firstFrameMs: number | null
}

const emptyMetrics: PlaybackMetrics = {
  downloadResolution: '-',
  renderResolution: '-',
  viewportResolution: '-',
  downloadBitrateKbps: null,
  bandwidthEstimateKbps: null,
  fps: null,
  skippedFrames: null,
  totalFrames: null,
  bufferSizeSec: null,
  latencyToLiveSec: null,
  targetLatencySec: null,
  currentTimeSec: null,
  liveSyncPositionSec: null,
  seekableEndSec: null,
  behindLiveSec: null,
  actualTwitchLatencySec: null,
  delayVsTwitchSec: null,
  canJumpLive: false,
  codecs: '-',
  protocol: 'HLS',
  latencyMode: 'Stable HLS',
  renderSurface: 'video',
  hlsStage: 'idle',
  recoveryAttempts: 0,
  stalls: 0,
  firstFrameMs: null,
}

const maxFatalNetworkRecoveries = 3
const maxFatalMediaRecoveries = 2

function normalizePlaybackError(message: string | null | undefined) {
  const fallback = message?.trim() || 'media playback error'
  const lower = fallback.toLowerCase()
  const isElectron = typeof navigator !== 'undefined' && /electron|code\//i.test(navigator.userAgent)

  if (lower.includes('decoder_error_not_supported') || lower.includes('unsupportedconfig')) {
    if (isElectron) {
      return 'This VS Code browser runtime could not initialize the stream decoder for the H.264/AAC feed. Open Streamclone in Chrome or Edge and try playback there.'
    }
    return 'This browser could not initialize the decoder for the current H.264/AAC stream.'
  }

  if (lower.includes('media_err_decode')) {
    return 'The browser hit a media decode failure while starting the stream.'
  }

  return fallback
}

interface UseHlsPlaybackOptions {
  src?: string
  enabled: boolean
  muted?: boolean
  autoPlay?: boolean
  latencyMode?: PlaybackLatencyMode
}

function hlsLatencyConfig(latencyMode: PlaybackLatencyMode) {
  switch (latencyMode) {
    case 'instant':
      return {
        lowLatencyMode: true,
        liveSyncDurationCount: 1,
        liveMaxLatencyDurationCount: 2,
        maxBufferLength: 2,
        backBufferLength: 4,
        maxLiveSyncPlaybackRate: 1.3,
      }
    case 'fast':
      return {
        lowLatencyMode: true,
        liveSyncDurationCount: 1.5,
        liveMaxLatencyDurationCount: 3,
        maxBufferLength: 4,
        backBufferLength: 8,
        maxLiveSyncPlaybackRate: 1.2,
      }
    default:
      return {
        lowLatencyMode: false,
        liveSyncDurationCount: 5,
        liveMaxLatencyDurationCount: 9,
        maxBufferLength: 15,
        backBufferLength: 30,
        maxLiveSyncPlaybackRate: 1,
      }
  }
}

function latencyModeLabel(mode: PlaybackLatencyMode) {
  if (mode === 'instant') return 'Instant HLS'
  if (mode === 'fast') return 'Fast HLS'
  return 'Stable HLS'
}

export function useHlsPlayback(videoRef: RefObject<HTMLVideoElement>, options: UseHlsPlaybackOptions) {
  const [state, setState] = useState<PlaybackState>('idle')
  const [error, setError] = useState<string | null>(null)
  const [metrics, setMetrics] = useState<PlaybackMetrics>(emptyMetrics)
  const hlsRef = useRef<Hls | null>(null)
  const recoveryRef = useRef(0)
  const stallsRef = useRef(0)
  const stageRef = useRef('idle')
  const firstFrameRef = useRef<number | null>(null)
  const startedAtRef = useRef<number>(0)
  const lastFragRef = useRef<{ bytes: number; duration: number } | null>(null)
  const fpsRef = useRef<{ startedAt: number; frames: number; fps: number | null }>({ startedAt: 0, frames: 0, fps: null })

  useEffect(() => {
    const video = videoRef.current
    if (video) video.muted = Boolean(options.muted)
  }, [options.muted, videoRef])

  const jumpLive = useCallback(() => {
    const video = videoRef.current
    if (!video) return false
    const metrics = getLiveEdgeMetrics(video, hlsRef.current)
    if (metrics.jumpTargetSec === null) return false
    video.currentTime = metrics.jumpTargetSec
    video.play().catch(() => undefined)
    return true
  }, [videoRef])

  useEffect(() => {
    const video = videoRef.current
    if (!video || !options.enabled || !options.src) {
      if (hlsRef.current) {
        hlsRef.current.destroy()
        hlsRef.current = null
      }
      if (video) {
        video.pause()
        video.removeAttribute('src')
        video.load()
      }
      stageRef.current = 'idle'
      setState('idle')
      setError(null)
      setMetrics(emptyMetrics)
      return
    }

    let alive = true
    let intervalId: ReturnType<typeof setInterval> | null = null
    let frameCallbackId: number | null = null
    const src = options.src
    recoveryRef.current = 0
    stallsRef.current = 0
    firstFrameRef.current = null
    startedAtRef.current = performance.now()
    fpsRef.current = { startedAt: performance.now(), frames: 0, fps: null }
    stageRef.current = 'starting'
    setState('starting')
    setError(null)
    const latencyMode = options.latencyMode ?? 'stable'
    setMetrics({ ...emptyMetrics, hlsStage: 'starting', latencyMode: latencyMode === 'fast' ? 'Fast HLS' : 'Stable HLS' })
    video.muted = Boolean(options.muted)

    const updateMetrics = () => {
      if (!alive) return
      setMetrics(readPlaybackMetrics(video, hlsRef.current, stageRef.current, {
        recoveryAttempts: recoveryRef.current,
        stalls: stallsRef.current,
        firstFrameMs: firstFrameRef.current === null ? null : Math.max(0, Math.round(firstFrameRef.current - startedAtRef.current)),
        lastFragment: lastFragRef.current,
        fps: fpsRef.current.fps,
        latencyMode,
      }))
    }

    const markPlaying = () => {
      if (!alive) return
      if (firstFrameRef.current === null) {
        firstFrameRef.current = performance.now()
      }
      stageRef.current = 'first-frame'
      setState('playing')
      updateMetrics()
    }

    const onPlaying = () => markPlaying()
    const onWaiting = () => {
      stallsRef.current += 1
      stageRef.current = 'buffering'
      setState(current => current === 'playing' ? 'buffering' : current)
      updateMetrics()
    }
    const onError = () => {
      stageRef.current = 'media-error'
      setState('error')
      setError(normalizePlaybackError(video.error?.message))
      updateMetrics()
    }

    video.addEventListener('playing', onPlaying)
    video.addEventListener('waiting', onWaiting)
    video.addEventListener('error', onError)

    const frameLoop = () => {
      if (!alive) return
      const now = performance.now()
      const frameStats = fpsRef.current
      frameStats.frames += 1
      if (firstFrameRef.current === null) {
        markPlaying()
      }
      if (now - frameStats.startedAt >= 1000) {
        frameStats.fps = Math.round((frameStats.frames * 1000) / (now - frameStats.startedAt))
        frameStats.startedAt = now
        frameStats.frames = 0
        updateMetrics()
      }
      if ('requestVideoFrameCallback' in video) {
        frameCallbackId = video.requestVideoFrameCallback(frameLoop)
      }
    }

    const attach = async () => {
      const { default: HlsPlayer } = await import('hls.js')
      if (!alive) return
      if (HlsPlayer.isSupported()) {
        const latency = hlsLatencyConfig(latencyMode)
        const hls = new HlsPlayer({
          ...latency,
          capLevelToPlayerSize: true,
          maxBufferHole: 0.5,
          enableWorker: true,
          xhrSetup: xhr => {
            xhr.withCredentials = true
          },
        })
        hlsRef.current = hls
        hls.on(HlsPlayer.Events.MEDIA_ATTACHED, () => {
          stageRef.current = 'media-attached'
          updateMetrics()
        })
        hls.on(HlsPlayer.Events.MANIFEST_PARSED, () => {
          stageRef.current = 'manifest-parsed'
          updateMetrics()
          if (options.autoPlay !== false) video.play().catch(() => undefined)
        })
        hls.on(HlsPlayer.Events.BUFFER_APPENDED, () => {
          if (stageRef.current !== 'first-frame') stageRef.current = 'buffered'
          updateMetrics()
        })
        hls.on(HlsPlayer.Events.FRAG_LOADED, (_, data) => {
          const eventData = data as unknown as { frag?: { stats?: { total?: number; loaded?: number }; duration?: number }; stats?: { total?: number; loaded?: number } }
          const stats = eventData.frag?.stats ?? eventData.stats
          const bytes = Number(stats?.total ?? stats?.loaded ?? 0)
          const duration = Number(eventData.frag?.duration ?? 0)
          if (bytes > 0 && duration > 0) {
            lastFragRef.current = { bytes, duration }
          }
          updateMetrics()
        })
        hls.on(HlsPlayer.Events.ERROR, (_, data) => {
          if (!data?.fatal) return
          recoveryRef.current += 1
          stageRef.current = data.details || 'hls-error'
          updateMetrics()
          if (data.type === HlsPlayer.ErrorTypes.NETWORK_ERROR) {
            if (recoveryRef.current > maxFatalNetworkRecoveries) {
              setState('error')
              setError(normalizePlaybackError(`HLS ${data.details || 'network error'}`))
              return
            }
            setState('retrying')
            hls.startLoad()
            return
          }
          if (data.type === HlsPlayer.ErrorTypes.MEDIA_ERROR) {
            if (recoveryRef.current > maxFatalMediaRecoveries) {
              setState('error')
              setError(normalizePlaybackError(`HLS ${data.details || 'media error'}`))
              return
            }
            setState('retrying')
            hls.recoverMediaError()
            return
          }
          setState('error')
          setError(normalizePlaybackError(`HLS ${data.details || 'fatal playback error'}`))
        })
        hls.loadSource(src)
        hls.attachMedia(video)
      } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
        stageRef.current = 'native-hls'
        video.src = src
        if (options.autoPlay !== false) video.play().catch(() => undefined)
      } else {
        stageRef.current = 'unsupported'
        setState('error')
        setError('HLS playback is not supported in this browser')
      }
    }

    attach().catch(err => {
      if (!alive) return
      stageRef.current = 'attach-error'
      setState('error')
      setError(normalizePlaybackError((err as Error).message || 'failed to attach HLS player'))
    })

    if ('requestVideoFrameCallback' in video) {
      frameCallbackId = video.requestVideoFrameCallback(frameLoop)
    }
    intervalId = setInterval(updateMetrics, 1000)

    return () => {
      alive = false
      if (intervalId) clearInterval(intervalId)
      if (frameCallbackId !== null && 'cancelVideoFrameCallback' in video) {
        video.cancelVideoFrameCallback(frameCallbackId)
      }
      video.removeEventListener('playing', onPlaying)
      video.removeEventListener('waiting', onWaiting)
      video.removeEventListener('error', onError)
      if (hlsRef.current) {
        hlsRef.current.destroy()
        hlsRef.current = null
      }
      video.pause()
      video.removeAttribute('src')
      video.load()
    }
  }, [options.src, options.enabled, options.autoPlay, options.latencyMode, videoRef])

  return useMemo(() => ({ state, error, metrics, jumpLive }), [state, error, metrics, jumpLive])
}

export function getLiveEdgeMetrics(video: HTMLVideoElement, hls: Hls | null) {
  const hlsAny = hls as unknown as {
    liveSyncPosition?: number
    latency?: number
    targetLatency?: number
  } | null
  const currentTimeSec = Number.isFinite(video.currentTime) ? Number(video.currentTime.toFixed(2)) : null
  const liveSyncPositionSec = Number.isFinite(hlsAny?.liveSyncPosition) ? Number((hlsAny?.liveSyncPosition ?? 0).toFixed(2)) : null
  const seekableEndSec = seekableEnd(video)
  const targetLatencySec = Number.isFinite(hlsAny?.targetLatency) ? Number((hlsAny?.targetLatency ?? 0).toFixed(2)) : null
  const liveEdge = calculateLiveEdge({ currentTimeSec, liveSyncPositionSec, seekableEndSec, targetLatencySec })
  return {
    currentTimeSec,
    liveSyncPositionSec,
    seekableEndSec,
    behindLiveSec: liveEdge.behindLiveSec,
    targetLatencySec,
    canJumpLive: liveEdge.canJumpLive,
    jumpTargetSec: liveEdge.jumpTargetSec,
  }
}

function readPlaybackMetrics(
  video: HTMLVideoElement,
  hls: Hls | null,
  stage: string,
  refs: {
    recoveryAttempts: number
    stalls: number
    firstFrameMs: number | null
    lastFragment: { bytes: number; duration: number } | null
    fps: number | null
    latencyMode: PlaybackLatencyMode
  },
): PlaybackMetrics {
  const rect = video.getBoundingClientRect()
  const quality = video.getVideoPlaybackQuality?.()
  const bufferSizeSec = bufferedAhead(video)
  const hlsAny = hls as unknown as {
    bandwidthEstimate?: number
    latency?: number
    targetLatency?: number
    currentLevel?: number
    loadLevel?: number
    liveSyncPosition?: number
    levels?: Array<{ width?: number; height?: number; bitrate?: number; videoCodec?: string; audioCodec?: string; attrs?: { CODECS?: string } }>
  } | null
  const levels = hlsAny?.levels ?? []
  const liveEdge = getLiveEdgeMetrics(video, hls)
  const levelIndex = Number.isFinite(hlsAny?.currentLevel) && (hlsAny?.currentLevel ?? -1) >= 0 ? hlsAny?.currentLevel : hlsAny?.loadLevel
  const level = typeof levelIndex === 'number' && levelIndex >= 0 ? levels[levelIndex] : levels.find(item => item.width || item.height) ?? null
  const downloadBitrate = refs.lastFragment && refs.lastFragment.duration > 0
    ? Math.round((refs.lastFragment.bytes * 8) / refs.lastFragment.duration / 1000)
    : level?.bitrate ? Math.round(level.bitrate / 1000) : null
  const codecs = level?.attrs?.CODECS || [level?.videoCodec, level?.audioCodec].filter(Boolean).join(',') || '-'

  return {
    downloadResolution: level?.width && level?.height ? `${level.width}x${level.height}` : video.videoWidth && video.videoHeight ? `${video.videoWidth}x${video.videoHeight}` : '-',
    renderResolution: video.videoWidth && video.videoHeight ? `${video.videoWidth}x${video.videoHeight}` : '-',
    viewportResolution: rect.width && rect.height ? `${Math.round(rect.width)}x${Math.round(rect.height)}` : '-',
    downloadBitrateKbps: downloadBitrate,
    bandwidthEstimateKbps: hlsAny?.bandwidthEstimate ? Math.round(hlsAny.bandwidthEstimate / 1000) : null,
    fps: refs.fps,
    skippedFrames: quality?.droppedVideoFrames ?? null,
    totalFrames: quality?.totalVideoFrames ?? null,
    bufferSizeSec,
    latencyToLiveSec: Number.isFinite(hlsAny?.latency) ? Number((hlsAny?.latency ?? 0).toFixed(2)) : null,
    targetLatencySec: liveEdge.targetLatencySec,
    currentTimeSec: liveEdge.currentTimeSec,
    liveSyncPositionSec: liveEdge.liveSyncPositionSec,
    seekableEndSec: liveEdge.seekableEndSec,
    behindLiveSec: liveEdge.behindLiveSec,
    actualTwitchLatencySec: null,
    delayVsTwitchSec: null,
    canJumpLive: liveEdge.canJumpLive,
    codecs,
    protocol: 'HLS',
    latencyMode: latencyModeLabel(refs.latencyMode),
    renderSurface: 'video',
    hlsStage: stage,
    recoveryAttempts: refs.recoveryAttempts,
    stalls: refs.stalls,
    firstFrameMs: refs.firstFrameMs,
  }
}

function seekableEnd(video: HTMLVideoElement) {
  if (!video.seekable?.length) return null
  const end = video.seekable.end(video.seekable.length - 1)
  return Number.isFinite(end) ? Number(end.toFixed(2)) : null
}

function bufferedAhead(video: HTMLVideoElement) {
  if (!video.buffered?.length || !Number.isFinite(video.currentTime)) return null
  for (let i = video.buffered.length - 1; i >= 0; i -= 1) {
    const start = video.buffered.start(i)
    const end = video.buffered.end(i)
    if (video.currentTime >= start && video.currentTime <= end) {
      return Number(Math.max(0, end - video.currentTime).toFixed(2))
    }
  }
  return null
}
