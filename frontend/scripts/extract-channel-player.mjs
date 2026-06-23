import fs from 'node:fs'
import path from 'node:path'

const root = path.resolve(import.meta.dirname, '..')
const channelPath = path.join(root, 'src/components/Channel.tsx')
const outPath = path.join(root, 'src/components/channel/ChannelPlayerSurface.tsx')
const lines = fs.readFileSync(channelPath, 'utf8').split(/\r?\n/)

const helperStart = lines.findIndex(l => l.startsWith('function fmtMetricSec('))
const helperEnd = lines.findIndex((l, i) => i > helperStart && l.startsWith('function FollowButton('))
const playerStart = lines.findIndex(l => l.includes('className={playerViewportClass}'))
const playerEnd = lines.findIndex((l, i) => i > playerStart && l.trim() === '</div>' && lines[i + 1]?.trim() === '</div>' && lines[i + 2]?.trim() === '</div>')

if (helperStart < 0 || helperEnd < 0 || playerStart < 0 || playerEnd < 0) {
  console.error('markers', { helperStart, helperEnd, playerStart, playerEnd })
  process.exit(1)
}

const header = `import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { Dispatch, RefObject, SetStateAction } from 'react'

import type {
  ChannelDetails,
  StartResponse,
  StreamDiagnostics,
  VodStartResponse,
} from '../../api'
import { normalizeBrowserOriginUrl } from '../../config'
import { useHlsPlayback, type PlaybackState } from '../../playback'
import { computeEndToEndLiveDelaySec } from '../../playbackMath'
import {
  autoHighStableQuality,
  defaultQualityOptions,
  requestQuality,
} from '../../streamQuality'
import type {
  BottomDensityMode,
  PlaybackLatencyMode,
  VideoFitMode,
} from '../../settings'
import { buildVodSeekTarget } from '@streamclone/pulse-core'
import { needsVodRelayRestart, readVideoSeekableRanges, vodRelativeSeekSeconds } from '../../utils/vodSeek'
import { usePlayheadStore } from '../../stores/playheadStore'
import { PLAYHEAD_SYNC_INTERVAL_MS } from '../../utils/chartCursorSync'
import TwitchVodEmbed, { type TwitchVodPlayerHandle } from './TwitchVodEmbed'
import VodModeControls from './VodModeControls'
import VodErrorState from './VodErrorState'
import type { VodErrorInput } from './vodError'
import PlayerHeatmap from './PlayerHeatmap'
import VodSeekBar from './VodSeekBar'
import PlaybackDiagnostics from '../PlaybackDiagnostics'
import { useRegisterChannelPlayback } from './channelPlaybackContext'
import type { AnalyticsMinuteRollup } from '../../api'

type QualityMenuOption = { value: string; label: string }
type RenditionOption = NonNullable<StartResponse['renditions']>[number]

function qualityLabel(value: string) {
  if (value === autoHighStableQuality) return '720p fast'
  if (value === 'best') return 'Best / source'
  return value
}

function renditionFrameRateSuffix(frameRate: number | undefined) {
  return frameRate && frameRate >= 59.5 ? '60' : ''
}

function qualityOptions(renditions: StartResponse['renditions'] | undefined) {
  if (!renditions?.length) {
    return defaultQualityOptions.map(value => ({ value, label: qualityLabel(value) }))
  }
  const seen = new Set<string>()
  const options: QualityMenuOption[] = [autoHighStableQuality, 'best'].map(value => {
    seen.add(value)
    return { value, label: qualityLabel(value) }
  })
  for (const rendition of renditions) {
    const group = rendition.group?.trim().toLowerCase() || ''
    const name = rendition.name?.trim().toLowerCase() || ''
    const value = group === 'chunked' || name.includes('source')
      ? 'best'
      : group || (rendition.height ? \`\${rendition.height}p\${renditionFrameRateSuffix(rendition.frameRate)}\` : name || 'best')
    if (!value || seen.has(value)) continue
    seen.add(value)
    options.push({ value, label: rendition.name || value })
  }
  return options.length ? options : defaultQualityOptions.map(value => ({ value, label: qualityLabel(value) }))
}

function qualityOptionDetail(value: string, renditions: StartResponse['renditions'] | undefined) {
  if (!renditions?.length) return value === autoHighStableQuality ? 'Fast high stable relay preset' : 'Relay quality request'
  const match = renditions.find(r => (r.group || r.name || '').toLowerCase().includes(value.toLowerCase()))
  if (!match) return 'Discovered rendition'
  const parts = [match.name, match.width && match.height ? \`\${match.width}x\${match.height}\` : null, match.frameRate ? \`\${Math.round(match.frameRate)}fps\` : null].filter(Boolean)
  return parts.join(' · ') || 'Discovered rendition'
}

`

const helpers = lines.slice(helperStart, helperEnd).join('\n')
const playerBlock = lines.slice(playerStart, playerEnd + 1).join('\n')

const component = `

export type ChannelPlayerSurfaceProps = {
  channelLogin: string
  isVodPlayback: boolean
  vodPlaybackId: string
  vodOffsetSeconds: number
  vodAnalyticsStreamId: string
  vodFromAnalytics: boolean
  vodAnalyticsHref: string | null
  showAnalyticsActivityWaveform: boolean
  showTwitchEmbed: boolean
  vodEmbedFallback: boolean
  onVodEmbedFallbackChange: Dispatch<SetStateAction<boolean>>
  hlsUrl: string
  relayState: PlaybackState
  setRelayState: Dispatch<SetStateAction<PlaybackState>>
  streamSession: StartResponse | null
  diagnostics?: StreamDiagnostics | null
  details?: ChannelDetails
  isChannelOffline: boolean
  error: string | null
  setError: Dispatch<SetStateAction<string | null>>
  vodRelayError: VodErrorInput | null
  setVodRelayError: Dispatch<SetStateAction<VodErrorInput | null>>
  vodSeekPending: boolean
  setVodSeekPending: Dispatch<SetStateAction<boolean>>
  vodSeekOnStart: number
  vodRelayEpoch: number
  vodRelayBaseSeconds: number
  muted: boolean
  setMuted: Dispatch<SetStateAction<boolean>>
  playerVolume: number
  preferredQuality: string
  playbackLatencyMode: PlaybackLatencyMode
  videoFit: VideoFitMode
  bottomDensity: BottomDensityMode
  isTheater: boolean
  setIsTheater: Dispatch<SetStateAction<boolean>>
  detailsExpanded: boolean
  setDetailsExpanded: Dispatch<SetStateAction<boolean>>
  startupStartedAt: number
  startupBenchmarks: Array<{ sessionId: string; attempt: number; backend: string; relayStartupMs: number | null; firstFrameMs: number | null; fallbackUsed: boolean }>
  streamPoster?: string
  playerViewportClass: string
  videoObjectFitClass: string
  activityRollups: AnalyticsMinuteRollup[] | null
  vodRollupsLoading: boolean
  vodRollupsError: boolean
  vodAnalyticsDurationSec: number | null
  onRetry: () => void
  onCoarsePlaybackStateChange?: (state: PlaybackState) => void
  onSetSearchParamsOffset: (offset: number) => void
  onHotRestartVodRelay: (absoluteOffset: number) => void
  onUnauthorizedHls: () => void
  onVodRelayStale: () => void
  onBackToAnalytics?: () => void
  onVodResync?: () => void
  onSetActiveTabDiagnostics: () => void
  onTogglePlay: () => void
  onToggleFullscreen: () => void
  onSetPlayerVolume: (value: number) => void
  onSetPreferredQuality: (value: string) => void
  onSetPlaybackLatencyMode: (mode: PlaybackLatencyMode) => void
  onSetVideoFit: (mode: VideoFitMode) => void
  onSetBottomDensity: (mode: BottomDensityMode) => void
  hotRestartVodRelayRef: RefObject<(absoluteOffset: number) => void>
  internalVodSeekRef: RefObject<boolean>
  pendingFarSeekRef: RefObject<number | null>
  farSeekTimerRef: RefObject<ReturnType<typeof setTimeout> | null>
}

export default function ChannelPlayerSurface(props: ChannelPlayerSurfaceProps) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const playerFrameRef = useRef<HTMLDivElement>(null)
  const twitchEmbedRef = useRef<TwitchVodPlayerHandle | null>(null)
  const registerPlayback = useRegisterChannelPlayback()
  const [startupNow, setStartupNow] = useState(() => Date.now())
  const [embedMountReady, setEmbedMountReady] = useState(false)
  const [embedMetrics, setEmbedMetrics] = useState<{ current: number | null; duration: number | null }>({ current: null, duration: null })
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [vodVideoIsPlaying, setVodVideoIsPlaying] = useState(false)
  const [vodHasFirstFrame, setVodHasFirstFrame] = useState(false)
  const prevCoarseState = useRef<PlaybackState | null>(null)

  const playback = useHlsPlayback(videoRef, {
    src: props.hlsUrl,
    enabled: Boolean(props.hlsUrl),
    muted: props.muted,
    autoPlay: true,
    mode: props.isVodPlayback ? 'vod' : 'live',
    seekOnStart: props.isVodPlayback ? props.vodSeekOnStart : undefined,
    relayEpoch: props.isVodPlayback ? props.vodRelayEpoch : undefined,
    vodRepositioning: props.isVodPlayback && props.vodSeekPending,
    latencyMode: props.isVodPlayback ? 'stable' : props.playbackLatencyMode,
    onUnauthorizedHls: props.onUnauthorizedHls,
    onVodRelayStale: props.isVodPlayback ? props.onVodRelayStale : undefined,
  })

  const seekVodAbsoluteOffset = useCallback((absoluteOffset: number) => {
    const safe = Math.max(0, Math.floor(absoluteOffset))
    props.internalVodSeekRef.current = true
    props.onSetSearchParamsOffset(safe)
    requestAnimationFrame(() => { props.internalVodSeekRef.current = false })

    if (props.showTwitchEmbed) {
      twitchEmbedRef.current?.seek(safe)
      return
    }

    const video = videoRef.current
    const relative = vodRelativeSeekSeconds(safe, props.vodRelayBaseSeconds)
    const seekableRanges = video ? readVideoSeekableRanges(video) : []
    const duration = video && Number.isFinite(video.duration) ? video.duration : null
    const restartNeeded = needsVodRelayRestart(safe, props.vodRelayBaseSeconds, seekableRanges, 30, duration)

    if (!restartNeeded && video) {
      props.pendingFarSeekRef.current = null
      if (props.farSeekTimerRef.current) {
        clearTimeout(props.farSeekTimerRef.current)
        props.farSeekTimerRef.current = null
      }
      props.setVodSeekPending(false)
      video.currentTime = relative
      playback.seekVodRelay(relative)
      return
    }

    props.pendingFarSeekRef.current = safe
    props.setVodSeekPending(true)
    if (props.farSeekTimerRef.current) clearTimeout(props.farSeekTimerRef.current)
    props.farSeekTimerRef.current = window.setTimeout(() => {
      const target = props.pendingFarSeekRef.current
      props.pendingFarSeekRef.current = null
      props.farSeekTimerRef.current = null
      if (target != null) props.onHotRestartVodRelay(target)
      else props.setVodSeekPending(false)
    }, 300)
  }, [playback, props])

  props.hotRestartVodRelayRef.current = props.onHotRestartVodRelay

  const vodPlayheadStreamId = props.vodAnalyticsStreamId || props.vodPlaybackId
  useEffect(() => {
    if (!props.isVodPlayback || !vodPlayheadStreamId) {
      usePlayheadStore.getState().reset()
      return
    }
    const { setPlayhead, setPlaying } = usePlayheadStore.getState()
    const publish = () => {
      let absoluteSec = 0
      if (props.showTwitchEmbed) {
        absoluteSec = twitchEmbedRef.current?.getCurrentTime() ?? props.vodOffsetSeconds
        setPlaying(props.relayState === 'playing')
      } else {
        const video = videoRef.current
        const current = video && Number.isFinite(video.currentTime)
          ? video.currentTime
          : playback.metrics.currentTimeSec ?? 0
        absoluteSec = props.vodRelayBaseSeconds + Math.max(0, current)
        setPlaying(playback.state === 'playing')
      }
      setPlayhead(vodPlayheadStreamId, absoluteSec, props.vodPlaybackId)
    }
    publish()
    const intervalId = window.setInterval(publish, PLAYHEAD_SYNC_INTERVAL_MS)
    return () => {
      window.clearInterval(intervalId)
      usePlayheadStore.getState().reset()
    }
  }, [props.isVodPlayback, vodPlayheadStreamId, props.vodPlaybackId, props.vodRelayBaseSeconds, playback.state, props.showTwitchEmbed, props.relayState, props.vodOffsetSeconds])

  useEffect(() => {
    if (!props.showTwitchEmbed) {
      setEmbedMountReady(false)
      return
    }
    const frame = requestAnimationFrame(() => setEmbedMountReady(true))
    return () => cancelAnimationFrame(frame)
  }, [props.showTwitchEmbed])

  useEffect(() => {
    if (!props.showTwitchEmbed || props.relayState !== 'playing') {
      setEmbedMetrics({ current: null, duration: null })
      return
    }
    const publish = () => {
      setEmbedMetrics({
        current: twitchEmbedRef.current?.getCurrentTime() ?? null,
        duration: twitchEmbedRef.current?.getDuration() ?? null,
      })
    }
    publish()
    const intervalId = window.setInterval(publish, 1000)
    return () => window.clearInterval(intervalId)
  }, [props.relayState, props.showTwitchEmbed])

  const vodBannerCurrentSec = useMemo(() => {
    if (props.showTwitchEmbed) return embedMetrics.current ?? props.vodOffsetSeconds
    const rel = playback.metrics.currentTimeSec
    if (rel == null || !Number.isFinite(rel)) return props.vodOffsetSeconds
    return props.vodRelayBaseSeconds + Math.max(0, rel)
  }, [props.showTwitchEmbed, embedMetrics.current, props.vodOffsetSeconds, playback.metrics.currentTimeSec, props.vodRelayBaseSeconds])

  const vodBannerTotalSec = useMemo(() => {
    if (props.showTwitchEmbed) return embedMetrics.duration ?? props.vodAnalyticsDurationSec
    return props.vodAnalyticsDurationSec ?? playback.metrics.seekableEndSec
  }, [props.showTwitchEmbed, embedMetrics.duration, props.vodAnalyticsDurationSec, playback.metrics.seekableEndSec])

  const playbackState = props.showTwitchEmbed ? props.relayState : (props.hlsUrl ? playback.state : props.relayState)

  useEffect(() => {
    if (prevCoarseState.current === playbackState) return
    prevCoarseState.current = playbackState
    props.onCoarsePlaybackStateChange?.(playbackState)
  }, [playbackState, props.onCoarsePlaybackStateChange])

  useEffect(() => {
    registerPlayback({
      metrics: playback.metrics,
      playbackState,
      jumpLive: () => playback.jumpLive(),
      seekVodAbsoluteOffset,
      videoRef,
    })
  }, [registerPlayback, playback, playback.metrics, playbackState, seekVodAbsoluteOffset])

  useEffect(() => {
    if (playbackState === 'playing' || props.error || playback.error) return
    const timer = window.setInterval(() => setStartupNow(Date.now()), 250)
    return () => window.clearInterval(timer)
  }, [props.error, props.hlsUrl, playback.error, playback.state, props.relayState, props.showTwitchEmbed, playbackState])

  const playbackError = props.error || playback.error
  const hasVodStructuredError = props.isVodPlayback && props.vodRelayError !== null && !props.showTwitchEmbed
  const showStructuredVodError = hasVodStructuredError && (playbackState === 'error' || playbackState === 'retrying')
  const showStartupOverlay = !props.isChannelOffline
    && !props.showTwitchEmbed
    && !props.vodSeekPending
    && playbackState !== 'playing'
    && playbackState !== 'buffering'
    && (playbackState === 'error' || playbackState === 'retrying' || playback.metrics.firstFrameMs === null)
    && !(props.isVodPlayback && vodHasFirstFrame && playbackState === 'retrying')
    && !(playbackError && props.details && !props.details.isLive && !props.isVodPlayback && !showStructuredVodError)
  const overlayState = startupOverlayState({
    playbackError,
    relayState: props.relayState,
    hlsUrl: props.hlsUrl,
    hlsStage: playback.metrics.hlsStage,
  })
  const startupElapsedMs = playback.metrics.firstFrameMs ?? Math.max(0, startupNow - props.startupStartedAt)
  const playerControlsVisible = playbackState === 'playing' || playbackState === 'buffering' || props.detailsExpanded
  const activeRenditions = props.streamSession?.renditions ?? props.diagnostics?.renditions
  const activeSelectedRendition = props.streamSession?.selectedRendition ?? props.diagnostics?.selectedRendition
  const requestedQuality = props.preferredQuality
  const loadedQuality = props.streamSession?.selectedRendition?.name || props.diagnostics?.selectedRendition?.name || props.preferredQuality
  const hasActivityRollups = (props.activityRollups?.length ?? 0) > 0
  const relayBreakdown = props.streamSession?.startupBreakdown ?? props.diagnostics?.startupBreakdown

  return (
    PLAYER_BLOCK_PLACEHOLDER
  )
}
`

const body = component.replace('PLAYER_BLOCK_PLACEHOLDER', playerBlock
  .replace('className={playerViewportClass}', 'className={props.playerViewportClass}')
  .replace(/\bplayback\.metrics\b/g, 'playback.metrics')
  .replace(/\bplaybackState\b/g, 'playbackState')
  .replace(/\bplaybackError\b/g, 'playbackError')
  .replace(/\bshowStartupOverlay\b/g, 'showStartupOverlay')
  .replace(/\bshowStructuredVodError\b/g, 'showStructuredVodError')
  .replace(/\bplayerControlsVisible\b/g, 'playerControlsVisible')
  .replace(/\bstreamPoster\b/g, 'props.streamPoster')
  .replace(/\bchannelLogin\b/g, 'props.channelLogin')
  .replace(/\bisVodPlayback\b/g, 'props.isVodPlayback')
  .replace(/\bvodPlaybackId\b/g, 'props.vodPlaybackId')
  .replace(/\bvodOffsetSeconds\b/g, 'props.vodOffsetSeconds')
  .replace(/\bvodAnalyticsHref\b/g, 'props.vodAnalyticsHref')
  .replace(/\bvodFromAnalytics\b/g, 'props.vodFromAnalytics')
  .replace(/\bvodAnalyticsStreamId\b/g, 'props.vodAnalyticsStreamId')
  .replace(/\bshowTwitchEmbed\b/g, 'props.showTwitchEmbed')
  .replace(/\bembedMountReady\b/g, 'embedMountReady')
  .replace(/\bmuted\b/g, 'props.muted')
  .replace(/\bretry\b/g, 'props.onRetry')
  .replace(/\bhandleBackToAnalytics\b/g, 'props.onBackToAnalytics')
  .replace(/\bhandleVodResync\b/g, 'props.onVodResync')
  .replace(/\bsetActiveTab\('diagnostics'\)/g, 'props.onSetActiveTabDiagnostics()')
  .replace(/\bsetMobilePane\('workspace'\)/g, '')
  .replace(/\bvodRelayError\b/g, 'props.vodRelayError')
  .replace(/\bvodSeekPending\b/g, 'props.vodSeekPending')
  .replace(/\bstartupBenchmarks\b/g, 'props.startupBenchmarks')
  .replace(/\bstartupElapsedMs\b/g, 'startupElapsedMs')
  .replace(/\boverlayState\b/g, 'overlayState')
  .replace(/\brelayBreakdown\b/g, 'relayBreakdown')
  .replace(/\bstreamSession\b/g, 'props.streamSession')
  .replace(/\bdiagnostics\.data\b/g, 'props.diagnostics')
  .replace(/\bdiagnostics\b/g, 'props.diagnostics')
  .replace(/\bvideoObjectFitClass\b/g, 'props.videoObjectFitClass')
  .replace(/\bvodBannerCurrentSec\b/g, 'vodBannerCurrentSec')
  .replace(/\bvodBannerTotalSec\b/g, 'vodBannerTotalSec')
  .replace(/\bshowAnalyticsActivityWaveform\b/g, 'props.showAnalyticsActivityWaveform')
  .replace(/\bhasActivityRollups\b/g, 'hasActivityRollups')
  .replace(/\bactivityRollups\b/g, 'props.activityRollups')
  .replace(/\bvodRollupsQuery\.isLoading\b/g, 'props.vodRollupsLoading')
  .replace(/\bvodRollupsQuery\.isError\b/g, 'props.vodRollupsError')
  .replace(/\bseekVodAbsoluteOffset\b/g, 'seekVodAbsoluteOffset')
  .replace(/\bsettings\.playerVolume\b/g, 'props.playerVolume')
  .replace(/\bsettings\.preferredQuality\b/g, 'props.preferredQuality')
  .replace(/\bsettings\.playbackLatencyMode\b/g, 'props.playbackLatencyMode')
  .replace(/\bsettings\.videoFit\b/g, 'props.videoFit')
  .replace(/\bsettings\.bottomDensity\b/g, 'props.bottomDensity')
  .replace(/\bisTheater\b/g, 'props.isTheater')
  .replace(/\bisFullscreen\b/g, 'isFullscreen')
  .replace(/\bdetailsExpanded\b/g, 'props.detailsExpanded')
  .replace(/\brequestedQuality\b/g, 'requestedQuality')
  .replace(/\bloadedQuality\b/g, 'loadedQuality')
  .replace(/\bactiveRenditions\b/g, 'activeRenditions')
  .replace(/\btogglePlay\b/g, 'props.onTogglePlay')
  .replace(/\btoggleFullscreen\b/g, 'props.onToggleFullscreen')
  .replace(/\bsetPlayerVolume\b/g, 'props.onSetPlayerVolume')
  .replace(/\bsetPreferredQuality\b/g, 'props.onSetPreferredQuality')
  .replace(/\bsetPlaybackLatencyMode\b/g, 'props.onSetPlaybackLatencyMode')
  .replace(/\bsetVideoFit\b/g, 'props.onSetVideoFit')
  .replace(/\bsetBottomDensity\b/g, 'props.onSetBottomDensity')
  .replace(/\bsetMuted\b/g, 'props.setMuted')
  .replace(/\bsetIsTheater\b/g, 'props.setIsTheater')
  .replace(/\bsetDetailsExpanded\b/g, 'props.setDetailsExpanded')
  .replace(/\bjumpLive\b/g, '() => playback.jumpLive()')
  .replace(/\bisChannelOffline\b/g, 'props.isChannelOffline')
  .replace(/\bdetails\.data\b/g, 'props.details')
)

fs.writeFileSync(outPath, header + helpers + body)
console.log('wrote', outPath)
