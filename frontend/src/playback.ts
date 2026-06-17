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
const stallDowngradeThreshold = 3
const rebufferDowngradeMs = 4000

/** VOD relays publish a short live HLS window; clamp when duration is known. */
function clampVodRelaySeek(video: HTMLVideoElement, seekTarget: number): number {
  if (!Number.isFinite(seekTarget) || seekTarget <= 0) return 0
  const duration = video.duration
  if (!Number.isFinite(duration) || duration <= 0) return seekTarget
  return Math.min(seekTarget, Math.max(0, duration - 0.5))
}

function applyVodRelaySeek(video: HTMLVideoElement, seekTarget: number): boolean {
  if (!Number.isFinite(seekTarget) || seekTarget <= 0) return true
  const clamped = clampVodRelaySeek(video, seekTarget)
  if (clamped <= 0) return false
  video.currentTime = clamped
  return true
}

/** Resume VOD relay playback without snapping to the live edge of the sliding window. */
function vodResumePosition(video: HTMLVideoElement, seekTarget: number): number {
  if (Number.isFinite(video.currentTime) && video.currentTime > 0.5) return video.currentTime
  if (seekTarget > 0) return seekTarget
  return 0
}

function nextLatencyMode(mode: PlaybackLatencyMode): PlaybackLatencyMode | null {
  if (mode === 'instant') return 'fast'
  if (mode === 'fast') return 'stable'
  return null
}

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
  mode?: 'live' | 'vod'
  /** Seconds into the relayed stream to seek once playback starts (VOD mode). */
  seekOnStart?: number
  /** Bumps when the VOD relay repositions so HLS re-attaches even if the URL is unchanged. */
  relayEpoch?: number
  /** Softer 401 / LevelLoadError recovery while the relay is repositioning. */
  vodRepositioning?: boolean
  onLatencyDowngrade?: (mode: PlaybackLatencyMode) => void
  /** Fired after repeated 401s on HLS playlists — usually stale session or dead relay. */
  onUnauthorizedHls?: () => void
  onVodRelayStale?: () => void
}

function hlsVodRelayConfig() {
  return {
    lowLatencyMode: false,
    // VOD relays publish a short live HLS window — stay at buffered position, not live edge.
    liveSyncMode: 'buffered' as const,
    liveSyncDurationCount: 2,
    liveMaxLatencyDurationCount: 5,
    maxBufferLength: 30,
    backBufferLength: 0,
    liveBackBufferLength: 0,
    maxLiveSyncPlaybackRate: 1,
    startFragPrefetch: true,
  }
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
  const [effectiveLatencyMode, setEffectiveLatencyMode] = useState<PlaybackLatencyMode>(options.latencyMode ?? 'stable')
  const hlsRef = useRef<Hls | null>(null)
  const recoveryRef = useRef(0)
  const stallsRef = useRef(0)
  const rebufferStartedRef = useRef<number | null>(null)
  const stageRef = useRef('idle')
  const firstFrameRef = useRef<number | null>(null)
  const startedAtRef = useRef<number>(0)
  const lastFragRef = useRef<{ bytes: number; duration: number } | null>(null)
  const fpsRef = useRef<{ startedAt: number; frames: number; fps: number | null }>({ startedAt: 0, frames: 0, fps: null })
  const seekOnStartRef = useRef(0)
  seekOnStartRef.current = Math.max(0, options.seekOnStart ?? 0)

  useEffect(() => {
    const video = videoRef.current
    if (options.mode !== 'vod' || !options.enabled || !options.src || !video) return
    const target = seekOnStartRef.current
    if (target <= 0) return
    applyVodRelaySeek(video, target)
  }, [options.enabled, options.mode, options.seekOnStart, options.src, videoRef])

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

  const seekVodRelay = useCallback((relativeSec: number) => {
    const hls = hlsRef.current
    const video = videoRef.current
    if (!hls || !video || options.mode !== 'vod') return false
    const clamped = clampVodRelaySeek(video, relativeSec)
    if (clamped > 0) {
      video.currentTime = clamped
    }
    hls.startLoad(clamped > 0 ? clamped : undefined)
    return true
  }, [options.mode, videoRef])

  useEffect(() => {
    setEffectiveLatencyMode(options.latencyMode ?? 'stable')
  }, [options.latencyMode])

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
    let unauthorizedReloadCount = 0
    let unauthorizedEscalated = false
    stallsRef.current = 0
    rebufferStartedRef.current = null
    firstFrameRef.current = null
    startedAtRef.current = performance.now()
    fpsRef.current = { startedAt: performance.now(), frames: 0, fps: null }
    stageRef.current = 'starting'
    setState('starting')
    setError(null)
    const playbackMode = options.mode ?? 'live'
    const latencyMode = playbackMode === 'vod' ? 'stable' : effectiveLatencyMode
    const seekTarget = playbackMode === 'vod' ? seekOnStartRef.current : 0
    const isVodRepositioning = playbackMode === 'vod' && Boolean(options.vodRepositioning)
    let seekApplied = seekTarget <= 0
    setMetrics({ ...emptyMetrics, hlsStage: 'starting', latencyMode: latencyModeLabel(latencyMode) })
    video.muted = Boolean(options.muted)

    const downgradeLatency = () => {
      const next = nextLatencyMode(latencyMode)
      if (!next || !alive) return
      setEffectiveLatencyMode(next)
      options.onLatencyDowngrade?.(next)
    }

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
      if (!seekApplied && seekTarget > 0) {
        seekApplied = applyVodRelaySeek(video, seekTarget)
      }
      if (rebufferStartedRef.current !== null) {
        const rebufferMs = performance.now() - rebufferStartedRef.current
        if (rebufferMs >= rebufferDowngradeMs) {
          downgradeLatency()
        }
        rebufferStartedRef.current = null
      }
      if (firstFrameRef.current === null) {
        firstFrameRef.current = performance.now()
      }
      stageRef.current = 'first-frame'
      setState('playing')
      updateMetrics()
    }

    const vodHoldPosition = () => vodResumePosition(video, seekTarget)

    const setVodRecoveryState = () => {
      if (isVodRepositioning || firstFrameRef.current !== null) {
        setState('buffering')
        return
      }
      setState('retrying')
    }

    const onPlaying = () => markPlaying()
    const onWaiting = () => {
      stallsRef.current += 1
      if (rebufferStartedRef.current === null) {
        rebufferStartedRef.current = performance.now()
      }
      if (stallsRef.current >= stallDowngradeThreshold && nextLatencyMode(latencyMode)) {
        downgradeLatency()
        stallsRef.current = 0
      }
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
    const onDurationReady = () => {
      if (!alive || seekApplied || seekTarget <= 0) return
      seekApplied = applyVodRelaySeek(video, seekTarget)
    }
    video.addEventListener('loadedmetadata', onDurationReady)
    video.addEventListener('durationchange', onDurationReady)

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
        const latency = playbackMode === 'vod' ? hlsVodRelayConfig() : hlsLatencyConfig(latencyMode)
        const maxNetworkRecoveries = playbackMode === 'vod' ? 6 : maxFatalNetworkRecoveries
        const hls = new HlsPlayer({
          ...latency,
          ...(playbackMode === 'vod' && seekTarget > 0 ? { startPosition: seekTarget } : {}),
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
          if (!seekApplied && seekTarget > 0) {
            seekApplied = applyVodRelaySeek(video, seekTarget)
          }
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
          const details = `${data.details || ''}`.toLowerCase()
          const responseCode = (data as { response?: { code?: number } }).response?.code
          const isUnauthorizedPlaylist =
            responseCode === 401 &&
            (details.includes('level') || details.includes('manifest') || details.includes('frag'))

          if (isUnauthorizedPlaylist) {
            unauthorizedReloadCount += 1
            stageRef.current = data.details || 'hls-unauthorized'
            updateMetrics()
            const maxUnauthorizedReloads = isVodRepositioning ? 12 : playbackMode === 'vod' ? 5 : 2
            if (unauthorizedReloadCount <= maxUnauthorizedReloads) {
              seekApplied = false
              if (playbackMode === 'vod') {
                setVodRecoveryState()
                applyVodRelaySeek(video, vodHoldPosition())
                hls.loadSource(src)
              } else {
                setState('retrying')
                hls.loadSource(src)
              }
              return
            }
            hls.stopLoad()
            if (!unauthorizedEscalated && playbackMode !== 'vod') {
              unauthorizedEscalated = true
              setState('retrying')
              options.onUnauthorizedHls?.()
              return
            }
            if (playbackMode === 'vod') {
              seekApplied = false
              applyVodRelaySeek(video, vodHoldPosition())
              if (isVodRepositioning) {
                setState('buffering')
                hls.startLoad(vodHoldPosition())
                return
              }
              hls.startLoad(vodHoldPosition())
              return
            }
            setState('error')
            setError(normalizePlaybackError('HLS stream unauthorized — relay may be down. Try Retry.'))
            return
          }

          if (!data?.fatal) {
            if (
              playbackMode === 'vod'
              && responseCode === 404
              && details.includes('frag')
            ) {
              stageRef.current = 'frag-404-recover'
              hls.startLoad(vodHoldPosition())
              updateMetrics()
            }
            return
          }
          recoveryRef.current += 1
          stageRef.current = data.details || 'hls-error'
          updateMetrics()
          if (data.type === HlsPlayer.ErrorTypes.NETWORK_ERROR) {
            const isLevelOrManifestLoad =
              responseCode === 401 || details.includes('levelload') || details.includes('manifestload')
            if (isVodRepositioning && isLevelOrManifestLoad) {
              recoveryRef.current = Math.max(1, Math.floor(recoveryRef.current / 2))
              seekApplied = false
              setState('buffering')
              applyVodRelaySeek(video, vodHoldPosition())
              hls.loadSource(src)
              updateMetrics()
              return
            }
            if (recoveryRef.current > maxNetworkRecoveries) {
              if (playbackMode === 'vod') {
                const hold = vodHoldPosition()
                recoveryRef.current = Math.max(1, Math.floor(maxNetworkRecoveries / 2))
                setState('buffering')
                hls.startLoad(hold)
                updateMetrics()
                return
              }
              if (options.onVodRelayStale) {
                setState('retrying')
                options.onVodRelayStale()
                return
              }
              setState('error')
              setError(normalizePlaybackError(`HLS ${data.details || 'network error'}`))
              return
            }
            if (playbackMode === 'vod') {
              setVodRecoveryState()
            } else {
              setState('retrying')
            }
            if (responseCode === 401 || details.includes('levelload') || details.includes('manifestload')) {
              seekApplied = false
              if (playbackMode === 'vod') {
                applyVodRelaySeek(video, vodHoldPosition())
              }
              hls.loadSource(src)
            } else if (playbackMode === 'vod') {
              hls.startLoad(vodHoldPosition())
            } else {
              hls.startLoad()
            }
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
      video.removeEventListener('loadedmetadata', onDurationReady)
      video.removeEventListener('durationchange', onDurationReady)
      if (hlsRef.current) {
        hlsRef.current.destroy()
        hlsRef.current = null
      }
      video.pause()
      video.removeAttribute('src')
      video.load()
    }
  }, [effectiveLatencyMode, options.autoPlay, options.enabled, options.mode, options.muted, options.onLatencyDowngrade, options.onUnauthorizedHls, options.onVodRelayStale, options.relayEpoch, options.seekOnStart, options.src, options.vodRepositioning, videoRef])

  return useMemo(() => ({ state, error, metrics, jumpLive, seekVodRelay, effectiveLatencyMode }), [state, error, metrics, jumpLive, seekVodRelay, effectiveLatencyMode])
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
