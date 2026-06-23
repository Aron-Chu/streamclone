import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ApiError,
  ensureChannelEmotes,
  followChannel,
  getChannel,
  getChannelBadges,
  getChannelDetails,
  getChannelEmotes,
  getChannelInsights,
  getAlwaysTracked,
  getAnalyticsStream,
  getAnalyticsStreams,
  getFollowedChannels,
  getLocalFollowedChannels,
  getReplayHeatmapDetail,
  getStreamDiagnostics,
  getSyncStatus,
  keepaliveStream,
  startStream,
  setAlwaysTracked,
  startHistoricalSync,
  startVodPlayback,
  stopStream,
  unfollowChannel,
  vodSessionKey,
  watchAnalyticsChannel,
} from '../api'
import { useAnalyticsLive, analyticsLiveQueryKey } from '../hooks/useAnalyticsLive'
import type { AnalyticsStream, AnalyticsTopEmote, AboutPanel, ChannelDetails, ChannelEmote, ChannelInsights, ClipCard, EmoteProvider, SourceStatus, StartResponse, StartupBreakdown, StreamDiagnostics, StatsTimelinePoint, StreamStat, VodStartResponse } from '../api'
import { useAuth } from '../auth'
import { useChatStore } from '../chatStore'
import { normalizeBrowserOriginUrl } from '../config'
import { type PlaybackMetrics, type PlaybackState } from '../playback'
import { computeEndToEndLiveDelaySec } from '../playbackMath'
import { useThemeEffect, useUiSettings, type BottomDensityMode, type ClipPeriod, type PlaybackLatencyMode, type StatsPeriod, type VideoFitMode } from '../settings'
import { autoHighStableQuality, defaultQualityOptions, requestQuality } from '../streamQuality'
import { emoteLoadPercent, formatEmoteProviderProgress, sortChannelEmotesByUsage } from '../emoteUtils'
import { normalizeVodId } from '../utils/vodId'
import { buildVodDeepLink, buildVodSeekTarget, estimateVodPlayerSeekTarget, parseVodAnalyticsContext, preferTwitchEmbedReview } from '@streamclone/pulse-core'
import {
  defaultAnalyticsVodSidebarTab,
  isEmbedAnalyticsVodReview,
  resolveVodDetailDurationSec,
  resolveVodTotalDurationSec,
} from '../utils/vodReviewLayout'
import { shouldUseTwitchEmbedFallback } from '../utils/vodEmbedFallback'
import { needsVodRelayRestart, readVideoSeekableRanges, vodRelativeSeekSeconds } from '../utils/vodSeek'
import { buildTwitchVodUrl } from '../utils/twitchVodUrl'
import VodChatReplayPanel from './analytics/VodChatReplayPanel'
import ChannelTabShell from './channel/ChannelTabShell'
import ChannelVodsPanel from './channel/ChannelVodsPanel'
import TwitchVodEmbed, { type TwitchVodPlayerHandle } from './channel/TwitchVodEmbed'
import VodStreamPulsePanel from './channel/VodStreamPulsePanel'
import VodSeekBar from './channel/VodSeekBar'
import BrandLogo from './BrandLogo'
import ChannelSearchInput from './ChannelSearchInput'
import Chat, { type ChatEmoteStatus } from './Chat'
import ChannelRail from './ChannelRail'
import LocalTokenImportButton from './LocalTokenImportButton'
import PlaybackDiagnostics from './PlaybackDiagnostics'
import SettingsButton from './SettingsPanel'
import VodModeControls, { formatVodTimestamp } from './channel/VodModeControls'
import VodErrorState from './channel/VodErrorState'
import type { VodErrorInput } from './channel/vodError'
import { HLS_NOT_READY_MAX_AUTO_RETRIES } from './channel/vodError'
import PlayerHeatmap from './channel/PlayerHeatmap'
import VodActivityGraphPanel from './channel/VodActivityGraphPanel'
import ChannelPlayerSurface, { ChannelPlayerChrome, ChannelStartupClock, ChannelVodPlayheadPublisher } from './channel/ChannelPlayerSurface'
import { ChannelPlaybackProvider, playbackActionsRef, useChannelPlayback } from './channel/channelPlaybackContext'
import StreamPulsePanel from './channel/StreamPulsePanel'
import TrackAnalyticsToggle from './channel/TrackAnalyticsToggle'
import SocialLinkChip from './channel/SocialLinkChip'
import { isLsfWarming } from '../utils/pulseEmptyState.ts'
import { usePlayheadStore } from '../stores/playheadStore'

type ChannelTab = 'about' | 'stats' | 'clips' | 'vods' | 'diagnostics' | 'emotes'
type ChatSidebarTab = 'chat' | 'pulse'
type MobileChannelPane = 'watch' | 'chat' | 'workspace'

const mobileChannelPanes: Array<{ id: MobileChannelPane; label: string }> = [
  { id: 'watch', label: 'Watch' },
  { id: 'chat', label: 'Chat' },
  { id: 'workspace', label: 'Workspace' },
]

const emoteProviderOptions: Array<{ id: EmoteProvider; label: string }> = [
  { id: 'seventv', label: '7TV' },
  { id: 'twitch', label: 'Twitch' },
  { id: 'ffz', label: 'FFZ' },
  { id: 'bttv', label: 'BTTV' },
]

const periodOptions: Array<{ id: ClipPeriod; label: string }> = [
  { id: '24h', label: '24h' },
  { id: '7d', label: '7d' },
  { id: '30d', label: '30d' },
  { id: '365d', label: 'Year' },
  { id: 'all', label: 'All' },
]


type QualityMenuOption = {
  value: string
  label: string
}

type RenditionOption = NonNullable<StartResponse['renditions']>[number]

function qualityLabel(value: string) {
  if (value === autoHighStableQuality) return '720p fast'
  if (value === 'best') return 'Best / source'
  return value
}

async function resolvePlayableHlsUrl(rawUrl: string) {
  const normalized = normalizeBrowserOriginUrl(rawUrl, ['/live/'])
  return normalized
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
    const value = renditionRequestValue(rendition)
    if (!value || seen.has(value)) continue
    seen.add(value)
    options.push({ value, label: renditionLabel(rendition, value) })
  }
  return options.length ? options : defaultQualityOptions.map(value => ({ value, label: qualityLabel(value) }))
}

function renditionRequestValue(rendition: RenditionOption) {
  const group = rendition.group?.trim().toLowerCase() || ''
  const name = rendition.name?.trim().toLowerCase() || ''
  if (group === 'chunked' || name.includes('source')) return 'best'
  if (group) return group
  if (rendition.height) {
    return `${rendition.height}p${renditionFrameRateSuffix(rendition.frameRate)}`
  }
  return name || 'best'
}

function renditionLabel(rendition: RenditionOption, value: string) {
  if (rendition.name) return rendition.name
  if (rendition.height) return `${rendition.height}p${renditionFrameRateSuffix(rendition.frameRate)}`
  return qualityLabel(value)
}

function qualityOptionDetail(value: string, renditions: StartResponse['renditions'] | undefined) {
  if (value === autoHighStableQuality) return 'Starts at 720p60/720p for faster relay, then tries source.'
  if (value === 'best') return 'Lets the backend choose the source rendition.'
  const rendition = renditions?.find(item => renditionRequestValue(item) === value)
  if (!rendition) return 'Preset request sent to the relay.'
  const size = rendition.width && rendition.height ? `${rendition.width}x${rendition.height}` : rendition.height ? `${rendition.height}p` : ''
  const fps = rendition.frameRate ? `${Math.round(rendition.frameRate)}fps` : ''
  const bandwidth = rendition.bandwidth ? `${Math.round(rendition.bandwidth / 1000)} kbps` : ''
  return [size, fps, bandwidth].filter(Boolean).join(' · ') || 'Discovered by the backend.'
}

function resolveRequestedQuality(
  renditions: StartResponse['renditions'] | undefined,
  preferredQuality: string | undefined,
  selectedRendition: StartResponse['selectedRendition'] | StreamDiagnostics['selectedRendition'],
) {
  const options = qualityOptions(renditions)
  const preferred = preferredQuality?.trim() || ''
  if (preferred && options.some(option => option.value === preferred)) {
    return preferred
  }
  const selectedValue = selectedRendition ? renditionRequestValue(selectedRendition) : ''
  if (selectedValue && options.some(option => option.value === selectedValue)) {
    return selectedValue
  }
  return options[0]?.value || preferred || autoHighStableQuality
}

function selectedRenditionText(session: StartResponse | null, diagnostics?: StreamDiagnostics) {
  const r = session?.selectedRendition ?? diagnostics?.selectedRendition
  if (!r) return session?.quality || diagnostics?.quality || 'best'
  const size = r.width && r.height ? `${r.width}x${r.height}` : r.height ? `${r.height}p` : ''
  const fps = r.frameRate ? `${Math.round(r.frameRate)}fps` : ''
  return [r.name, size, fps].filter(Boolean).join(' · ')
}

function count(value: number | null | undefined) {
  if (value === null || value === undefined) return '-'
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return value.toLocaleString()
}

function fullCount(value: number | null | undefined) {
  return value === null || value === undefined ? '-' : value.toLocaleString()
}

function relativeTime(value?: string | number) {
  if (!value) return ''
  const ts = typeof value === 'number' ? value * 1000 : Date.parse(value)
  if (!Number.isFinite(ts)) return ''
  const diff = Date.now() - ts
  const minutes = Math.max(1, Math.round(diff / 60000))
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.round(minutes / 60)
  if (hours < 48) return `${hours}h ago`
  return `${Math.round(hours / 24)}d ago`
}

function calendarDate(value?: string | number) {
  if (!value) return '-'
  const ts = typeof value === 'number' ? value * 1000 : Date.parse(value)
  if (!Number.isFinite(ts)) return '-'
  return new Date(ts).toLocaleDateString([], { year: 'numeric', month: 'short', day: 'numeric' })
}

function compactUrl(value?: string) {
  if (!value) return ''
  try {
    const url = new URL(value)
    const path = url.pathname && url.pathname !== '/' ? url.pathname : ''
    return `${url.host}${path}`
  } catch {
    return value
  }
}

function normalizePanelText(value: string | undefined, fallback: string) {
  const trimmed = value?.trim()
  if (!trimmed || trimmed.toLowerCase() === 'default') return fallback
  return trimmed
}

function sourceTone(state: SourceStatus['state']) {
  if (state === 'ready') return 'border-emerald-400/20 bg-emerald-400/10 text-emerald-100'
  if (state === 'fallback') return 'border-cyan-300/20 bg-cyan-400/10 text-cyan-100'
  if (state === 'blocked' || state === 'unavailable') return 'border-amber-300/20 bg-amber-400/10 text-amber-100'
  return 'border-red-400/20 bg-red-500/10 text-red-100'
}

function sourceStateLabel(source: SourceStatus) {
  if (source.source === 'stream_history' && source.message === 'stream-by-stream history source not configured') return 'Summary only'
  if (source.state === 'ready') return 'Ready'
  if (source.state === 'fallback') return 'Fallback'
  if (source.state === 'blocked') return 'Blocked'
  if (source.state === 'unavailable') return 'Unavailable'
  return 'Error'
}

function titleCase(value: string) {
  return value
    .split(/[_\s-]+/)
    .filter(Boolean)
    .map(part => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function sourceDisplayName(source: SourceStatus) {
  if (source.source === 'twitchtracker') return 'TwitchTracker'
  if (source.source === 'twitch_helix_clips') return 'Twitch clips'
  if (source.source === 'twitch_helix') return 'Twitch Helix'
  if (source.source === 'twitch_gql') return 'Twitch GQL'
  if (source.source === 'twitch_gql_about_panels') return 'About panels'
  if (source.source === 'stream_history') return 'Stream history'
  return titleCase(source.provider || source.source)
}

function sourceMessageText(source: SourceStatus) {
  const message = source.message?.trim()
  if (!message) {
    if (source.state === 'ready') return 'Loaded successfully.'
    if (source.state === 'fallback') return 'Using fallback data for this section.'
    if (source.state === 'blocked') return 'The upstream provider blocked this request.'
    if (source.state === 'unavailable') return 'This source is not available for the current setup.'
    return 'This source returned an unexpected error.'
  }
  if (message === 'stream-by-stream history source not configured') {
    return 'The current metadata stack only has summary stats; no stream-by-stream history provider is configured yet.'
  }
  if (source.source === 'stream_history' && message === 'twitchtracker streams page did not contain parseable rows') {
    return 'TwitchTracker summary stats loaded, but the streams page did not expose parseable per-stream rows.'
  }
  if (source.source === 'stream_history' && source.provider === 'helix') {
    return 'Stream list loaded from Twitch VOD archives. Avg/peak viewers still come from TwitchTracker when its scrape succeeds.'
  }
  if (/^status \d+$/i.test(message)) {
    return `${sourceDisplayName(source)} returned ${message}.`
  }
  return message.charAt(0).toUpperCase() + message.slice(1)
}

function sourceMatchesGroup(source: SourceStatus, group: 'stats' | 'clips') {
  if (group === 'stats') return source.source.includes('twitchtracker') || source.source === 'stream_history'
  return source.source.includes('clips')
}

function summarizeClipEmptyState(sources: SourceStatus[] | undefined) {
  const rows = (sources ?? []).filter(source => source.source.includes('clips'))
  const unavailable = rows.find(source => source.state === 'unavailable')
  if (unavailable) return sourceMessageText(unavailable)
  const failed = rows.find(source => source.state === 'error' || source.state === 'blocked')
  if (failed) return sourceMessageText(failed)
  if (rows.some(source => source.state === 'fallback')) return 'Using fallback clip metadata for this section.'
  if (rows.some(source => source.state === 'ready')) return 'No clips matched the selected period for this channel.'
  return 'Clip metadata is still loading for this channel.'
}

function SourcePills({ sources }: { sources: SourceStatus[] | undefined }) {
  if (!sources?.length) return null
  return (
    <div className="flex flex-wrap gap-1.5">
      {sources.map((item, index) => (
        <span
          key={`${item.source}-${item.state}-${index}`}
          title={sourceMessageText(item)}
          className={`rounded border px-2 py-1 text-[11px] font-black uppercase ${sourceTone(item.state)}`}
        >
          {sourceDisplayName(item)} {sourceStateLabel(item)}
        </span>
      ))}
    </div>
  )
}

function SourceDiagnostics({ sources }: { sources: SourceStatus[] | undefined }) {
  const rows = sources ?? []
  if (!rows.length) return null
  return (
    <div className="rounded border border-white/10 bg-white/[0.035] p-3">
      <div className="mb-2 text-[11px] font-black uppercase text-zinc-500">Source diagnostics</div>
      <div className="space-y-2">
        {rows.map((source, index) => (
          <div key={`${source.source}-${source.provider}-${index}`} className="rounded border border-white/10 bg-black/20 p-3">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0 flex-1">
                <div className="text-xs font-black uppercase text-zinc-200">{sourceDisplayName(source)}</div>
                <div className="mt-1 text-xs font-semibold leading-5 text-zinc-400">{sourceMessageText(source)}</div>
              </div>
              <span className={`shrink-0 rounded border px-2 py-1 text-[10px] font-black uppercase ${sourceTone(source.state)}`}>
                {sourceStateLabel(source)}
              </span>
            </div>
            {source.backoffUntil ? <div className="mt-2 text-[11px] font-semibold text-amber-200">Backoff until {new Date(source.backoffUntil).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</div> : null}
          </div>
        ))}
      </div>
    </div>
  )
}

function providerStatusText(status: ChatEmoteStatus) {
  if (status.providers?.length) {
    return status.providers.map(provider => {
      const label = emoteProviderOptions.find(option => option.id === provider.provider)?.label ?? provider.provider
      if (provider.state === 'processing' || provider.state === 'partial') return `${label} ${provider.percent ?? 0}%`
      if (provider.state === 'failed') return `${label} failed`
      return `${label} ready`
    }).join(' · ')
  }
  if (status.state === 'idle') return 'idle'
  if (status.state === 'loading') return 'loading'
  if (status.state === 'processing' || status.state === 'partial') return `processing ${status.percent ?? 0}%`
  if (status.state === 'failed') return 'failed'
  return `ready ${status.count}`
}

function providerStatusTone(status: ChatEmoteStatus) {
  if (status.state === 'failed' || status.providers?.some(provider => provider.state === 'failed')) return 'text-red-200 bg-red-500/10 border-red-400/20'
  if (status.state === 'processing' || status.state === 'partial' || status.state === 'loading') return 'text-amber-100 bg-amber-400/10 border-amber-300/20'
  if (status.state === 'ready') return 'text-emerald-100 bg-emerald-400/10 border-emerald-400/20'
  return 'text-zinc-300 bg-white/[0.045] border-white/10'
}

function providerCardTone(state: string) {
  if (state === 'failed') return 'border-red-400/25 bg-red-500/10 text-red-100'
  if (state === 'ready') return 'border-emerald-400/25 bg-emerald-400/10 text-emerald-100'
  if (state === 'processing' || state === 'partial') return 'border-amber-300/25 bg-amber-400/10 text-amber-100'
  return 'border-white/10 bg-white/[0.035] text-zinc-300'
}

function providerProgress(state: string, active: boolean, count: number, total: number, percent?: number) {
  if (total > 0) return emoteLoadPercent(count, total, percent)
  if (typeof percent === 'number') return Math.max(0, Math.min(100, percent))
  if (state === 'ready') return 100
  if (state === 'processing') return 55
  return active ? 20 : 8
}

function EmoteProviderPanel({
  selected,
  status,
  autoLoad,
  disabled,
  loadedEmotes,
  emotesLoading,
  channelLabel,
  channelCategory,
  topEmotes,
  onToggle,
  onLoad,
  onAutoLoad,
}: {
  selected: EmoteProvider[]
  status: ChatEmoteStatus
  autoLoad: boolean
  disabled: boolean
  loadedEmotes: ChannelEmote[]
  emotesLoading: boolean
  channelLabel?: string
  channelCategory?: string
  topEmotes?: AnalyticsTopEmote[]
  onToggle: (provider: EmoteProvider) => void
  onLoad: () => void
  onAutoLoad: (value: boolean) => void
}) {
  const [expandedProviders, setExpandedProviders] = useState<Partial<Record<EmoteProvider, boolean>>>({})
  const busy = status.state === 'loading' || status.state === 'processing'
  const previewLimit = 16
  const providerRows = emoteProviderOptions.map(option => {
    const row = status.providers?.find(provider => provider.provider === option.id)
    const emotes = sortChannelEmotesByUsage(
      loadedEmotes.filter(emote => emote.provider === option.id),
      topEmotes,
    )
    const active = selected.includes(option.id)
    const count = row?.count ?? 0
    const total = row?.total ?? 0
    const state = row?.state ?? (total > 0 && count < total ? 'processing' : emotes.length ? 'ready' : active && busy ? 'processing' : 'idle')
    const pending = row?.pending ?? (state === 'processing' ? status.pending : 0)
    const expanded = Boolean(expandedProviders[option.id])
    const percent = emoteLoadPercent(count, total, row?.percent)
    return {
      ...option,
      active,
      expanded,
      state,
      pending,
      failed: row?.failed ?? 0,
      total,
      percent,
      count,
      previewCount: Math.min(previewLimit, emotes.length),
      visibleEmotes: expanded ? emotes : emotes.slice(0, previewLimit),
      emotes,
      error: row?.error,
    }
  })
  const copyBenchmark = () => {
    if (!status.benchmark) return
    navigator.clipboard?.writeText(JSON.stringify(status.benchmark, null, 2)).catch(() => undefined)
  }
  return (
    <div className="rounded border border-white/10 bg-white/[0.035] p-3">
      <div className="mb-2 flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <div className="text-sm font-semibold text-white">Chat emotes</div>
          {status.benchmark ? (
            <span className="rounded border border-white/10 bg-black/25 px-2 py-1 text-[10px] font-black uppercase text-zinc-300">
              {status.benchmark.cacheHit ? 'Warm cache' : 'Cold load'}
            </span>
          ) : null}
        </div>
        <span title={status.error} className={`rounded border px-2 py-1 text-[11px] font-black uppercase ${providerStatusTone(status)}`}>
          {providerStatusText(status)}
        </span>
      </div>
      {channelCategory ? (
        <div className="mb-2 text-xs font-semibold text-zinc-400">
          {channelLabel || 'Channel'} is playing <span className="font-black text-zinc-200">{channelCategory}</span>
        </div>
      ) : null}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex rounded border border-white/10 bg-white/[0.045] p-1">
          {emoteProviderOptions.map(option => {
            const active = selected.includes(option.id)
            return (
              <button
                key={option.id}
                type="button"
                aria-pressed={active}
                onClick={() => onToggle(option.id)}
                className={`rounded px-3 py-1.5 text-xs font-black transition ${active ? 'bg-white text-zinc-950' : 'text-zinc-400 hover:bg-white/10 hover:text-white'}`}
              >
                {option.label}
              </button>
            )
          })}
        </div>
        <button
          type="button"
          onClick={onLoad}
          disabled={disabled || selected.length === 0 || busy}
          className="rounded bg-violet-500 px-3 py-2 text-xs font-black text-white transition hover:bg-violet-400 disabled:cursor-not-allowed disabled:bg-white/10 disabled:text-zinc-500"
        >
          {busy ? 'Loading' : 'Load'}
        </button>
        <label className="flex items-center gap-2 text-xs font-bold text-zinc-400">
          <input
            type="checkbox"
            checked={autoLoad}
            onChange={event => onAutoLoad(event.target.checked)}
            className="h-4 w-4 accent-violet-400"
          />
          Auto-load
        </label>
      </div>
      <div className="mt-3 grid gap-3 md:grid-cols-2">
        {providerRows.map(provider => (
          <div key={provider.id} className={`rounded border p-3 ${providerCardTone(provider.state)}`}>
            <div className="flex items-start justify-between gap-3">
              <div>
                <div className="text-sm font-black text-white">{provider.label}</div>
                <div className="mt-1 text-xs font-semibold text-current/80">
                  {formatEmoteProviderProgress(provider)}
                </div>
                {provider.emotes.length > previewLimit ? (
                  <div className="mt-1 text-[11px] font-bold text-current/70">
                    {provider.expanded
                      ? `Showing all ${provider.emotes.length} loaded thumbnails.`
                      : `Previewing ${provider.previewCount} of ${provider.emotes.length} loaded thumbnails.`}
                  </div>
                ) : null}
              </div>
              <span title={provider.error} className="rounded bg-black/25 px-2 py-1 text-[10px] font-black uppercase">
                {provider.state}
              </span>
            </div>
            <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-black/35">
              <div className="h-full rounded-full bg-current transition-all" style={{ width: `${providerProgress(provider.state, provider.active, provider.count, provider.total, provider.percent)}%` }} />
            </div>
            <div className="mt-3 grid grid-cols-8 gap-1">
              {provider.visibleEmotes.map(emote => (
                <span key={`${provider.id}-${emote.emote_id}-${emote.name}`} title={emote.name} className="grid aspect-square place-items-center rounded bg-black/30 p-1">
                  <img src={normalizeBrowserOriginUrl(emote.url, ['/emotes/'])} alt={emote.name} className="max-h-full max-w-full object-contain" loading="lazy" />
                </span>
              ))}
              {!provider.emotes.length ? (
                <div className="col-span-8 rounded border border-dashed border-white/10 bg-black/20 px-2 py-3 text-center text-[11px] font-bold text-zinc-500">
                  {emotesLoading || provider.state === 'processing' ? 'Waiting for thumbnails' : 'No loaded thumbnails yet'}
                </div>
              ) : null}
            </div>
            {provider.emotes.length > previewLimit ? (
              <div className="mt-3 flex justify-end">
                <button
                  type="button"
                  onClick={() => setExpandedProviders(current => ({ ...current, [provider.id]: !provider.expanded }))}
                  className="rounded border border-white/10 bg-black/20 px-2 py-1 text-[11px] font-black text-current/90 transition hover:bg-black/30"
                >
                  {provider.expanded ? 'Show preview' : `Show all ${provider.emotes.length}`}
                </button>
              </div>
            ) : null}
          </div>
        ))}
      </div>
      {status.benchmark ? (
        <div className="mt-3 rounded border border-white/10 bg-black/20 p-3">
          <div className="mb-2 flex items-center justify-between gap-2">
            <div className="text-[11px] font-black uppercase text-zinc-500">Emote loading benchmark</div>
            <button type="button" onClick={copyBenchmark} className="rounded border border-cyan-300/30 bg-cyan-400/10 px-2 py-1 text-[11px] font-black text-cyan-100 transition hover:bg-cyan-400/20">
              Copy JSON
            </button>
          </div>
          <div className="grid grid-cols-2 gap-2 text-xs font-bold text-zinc-300 md:grid-cols-4">
            <span className="rounded bg-white/[0.045] px-2 py-1">Ensure {status.benchmark.ensureMs}ms</span>
            <span className="rounded bg-white/[0.045] px-2 py-1">Seed {status.benchmark.seedMs}ms</span>
            <span className="rounded bg-white/[0.045] px-2 py-1">Dictionary {status.benchmark.dictionaryMs}ms</span>
            <span className="rounded bg-white/[0.045] px-2 py-1">{status.benchmark.cacheHit ? 'Cache hit' : 'Cache miss'}</span>
          </div>
        </div>
      ) : null}
    </div>
  )
}

function AuthButton({ compact = false }: { compact?: boolean }) {
  const auth = useAuth()
  if (auth.isAuthenticated) {
    return (
      <div className="flex items-center gap-2">
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
    <div className="flex items-center gap-2">
      <LocalTokenImportButton compact={compact} />
    </div>
  )
}

function fmtMetricSec(value: number | null | undefined) {
  if (value === null || value === undefined || Number.isNaN(value)) return '-'
  if (value >= 60) return `${Math.round(value)}s`
  return `${value.toFixed(value >= 10 ? 0 : 1)}s`
}

function fmtMs(value: number | null | undefined) {
  if (value === null || value === undefined || Number.isNaN(value)) return '-'
  if (value >= 1000) return `${(value / 1000).toFixed(value >= 10_000 ? 0 : 1)}s`
  return `${Math.round(value)}ms`
}

function formatPlaybackStage(value: string | null | undefined) {
  const stage = value?.trim()
  if (!stage) return 'Starting'
  switch (stage) {
    case 'starting':
      return 'Attaching player'
    case 'media-attached':
      return 'Relay ready'
    case 'manifest-parsed':
      return 'Manifest parsed'
    case 'buffered':
      return 'Buffering first segment'
    case 'first-frame':
      return 'First frame rendered'
    case 'native-hls':
      return 'Native HLS ready'
    case 'media-error':
      return 'Media error'
    case 'attach-error':
      return 'Player attach failed'
    case 'hls-error':
      return 'HLS error'
    default:
      return stage
        .split(/[_-]+/)
        .filter(Boolean)
        .map(part => part.charAt(0).toUpperCase() + part.slice(1))
        .join(' ')
  }
}

function startupOverlayState({
  playbackError,
  relayState,
  hlsUrl,
  hlsStage,
}: {
  playbackError: string | null
  relayState: PlaybackState
  hlsUrl: string
  hlsStage: string
}) {
  if (playbackError) {
    return {
      title: 'Stream unavailable',
      detail: playbackError,
      stage: 'Error',
    }
  }

  if (!hlsUrl) {
    if (relayState === 'retrying') {
      return {
        title: 'Retrying relay',
        detail: 'Restarting the local HLS relay after a failed attempt.',
        stage: 'Relay retry',
      }
    }
    return {
      title: 'Starting relay',
      detail: 'Requesting stream access and waiting for the local HLS playlist.',
      stage: relayState === 'error' ? 'Relay error' : 'Relay bootstrap',
    }
  }

  return {
    title: 'Starting stream',
    detail: hlsStage === 'buffered' || hlsStage === 'first-frame'
      ? 'The browser is buffering the first playable segment.'
      : 'The browser is attaching the HLS player and parsing the live manifest.',
    stage: formatPlaybackStage(hlsStage),
  }
}

type StartupBenchmarkEntry = {
  sessionId: string
  attempt: number
  backend: string
  relayStartupMs: number | null
  firstFrameMs: number | null
  fallbackUsed: boolean
  startupBreakdown?: StartupBreakdown
}

function LivePlayerControls({
  playbackState,
  metrics,
  diagnostics,
  isVod = false,
  videoIsPlaying,
  requestedQuality,
  loadedQuality,
  renditions,
  latencyMode,
  latencyModeAuto,
  videoFit,
  bottomDensity,
  muted,
  volume,
  isFullscreen,
  isTheater,
  backend,
  startupMs,
  fallbackAttempted,
  detailsExpanded,
  onTogglePlay,
  onMuted,
  onVolume,
  onToggleFullscreen,
  onToggleTheater,
  onJumpLive,
  onQuality,
  onLatencyMode,
  onVideoFit,
  onBottomDensity,
  onDetailsExpanded,
}: {
  playbackState: PlaybackState
  metrics: PlaybackMetrics
  diagnostics?: StreamDiagnostics | null
  isVod?: boolean
  /** When set (VOD), drives play/pause icon from the video element instead of HLS state. */
  videoIsPlaying?: boolean
  requestedQuality: string
  loadedQuality: string
  renditions: StartResponse['renditions'] | undefined
  latencyMode: PlaybackLatencyMode
  latencyModeAuto?: boolean
  videoFit: VideoFitMode
  bottomDensity: BottomDensityMode
  muted: boolean
  volume: number
  isFullscreen: boolean
  isTheater: boolean
  backend?: string
  startupMs?: number
  fallbackAttempted?: boolean
  detailsExpanded: boolean
  onTogglePlay: () => void
  onMuted: (muted: boolean) => void
  onVolume: (volume: number) => void
  onToggleFullscreen: () => void
  onToggleTheater: () => void
  onJumpLive: () => void
  onQuality: (quality: string) => void
  onLatencyMode: (mode: PlaybackLatencyMode) => void
  onVideoFit: (mode: VideoFitMode) => void
  onBottomDensity: (mode: BottomDensityMode) => void
  onDetailsExpanded: (expanded: boolean) => void
}) {
  const behind = metrics.behindLiveSec
  const liveDelay = computeEndToEndLiveDelaySec(metrics, diagnostics)
  const displayDelay = liveDelay.displayDelaySec
  const liveDelayTooltip = liveDelay.tooltip
  const [qualityOpen, setQualityOpen] = useState(false)
  const qOptions = qualityOptions(renditions)
  const selectedQuality = qOptions.find(option => option.value === requestedQuality) ?? qOptions[0]
  const discoveredCount = renditions?.length ?? 0
  const liveTone = behind !== null && behind > Math.max((metrics.targetLatencySec ?? 0) + 4, 10)
    ? 'border-amber-300/35 bg-amber-400/15 text-amber-100'
    : 'border-red-400/35 bg-red-500/15 text-red-100'
  const showPlaying = videoIsPlaying ?? playbackState === 'playing'
  return (
    <section className="bg-gradient-to-t from-black/90 via-black/70 to-transparent px-3 py-3 lg:px-5">
      <div className="flex flex-nowrap items-center gap-2">
        <button
          type="button"
          onClick={onTogglePlay}
          aria-label="Play or pause"
          className="grid h-9 w-9 shrink-0 place-items-center rounded border border-white/10 bg-white/[0.08] text-xs font-black text-white transition hover:bg-white/15"
        >
          {showPlaying ? '❚❚' : '▶'}
        </button>
        <button
          type="button"
          onClick={() => onMuted(!muted)}
          aria-label={muted ? 'Unmute' : 'Mute'}
          className="grid h-9 w-9 shrink-0 place-items-center rounded border border-white/10 bg-white/[0.08] text-xs font-black text-white transition hover:bg-white/15"
        >
          {muted || volume === 0 ? '🔇' : volume < 0.5 ? '🔉' : '🔊'}
        </button>
        <input
          type="range"
          min={0}
          max={1}
          step={0.05}
          value={muted ? 0 : volume}
          onChange={event => {
            const next = Number(event.target.value)
            onVolume(next)
            if (next > 0 && muted) onMuted(false)
          }}
          aria-label="Volume"
          className="h-1.5 w-24 shrink-0 cursor-pointer accent-violet-400"
        />
        <div className="min-w-0 flex-1" />
        <div className="relative shrink-0">
          <button
            type="button"
            aria-haspopup="listbox"
            aria-expanded={qualityOpen}
            onClick={() => setQualityOpen(open => !open)}
            className="flex h-9 min-w-[8rem] items-center justify-between gap-2 rounded border border-white/10 bg-white/[0.08] px-3 text-left text-xs font-black text-white transition hover:bg-white/15 lg:min-w-[10rem]"
          >
            <span
              title={selectedQuality?.label === '720p fast' ? 'Fast high stable — starts at 720p60/720p for faster relay' : selectedQuality?.label ?? qualityLabel(requestedQuality)}
              className="min-w-0 flex-1 truncate text-sm"
            >
              {selectedQuality?.label ?? qualityLabel(requestedQuality)}
            </span>
            <span className="text-zinc-400">{qualityOpen ? '^' : 'v'}</span>
          </button>
          {qualityOpen ? (
            <div role="listbox" className="absolute bottom-10 left-0 z-40 w-80 max-w-[calc(100vw-2rem)] rounded border border-white/10 bg-[#181820] p-1 shadow-2xl shadow-black/60">
              <div className="px-2 py-1.5 text-[10px] font-black uppercase text-zinc-500">
                {discoveredCount ? `${discoveredCount} backend renditions discovered` : 'Preset requests until renditions load'}
              </div>
              {qOptions.map(option => (
                <button
                  key={option.value}
                  type="button"
                  role="option"
                  aria-selected={option.value === requestedQuality}
                  onClick={() => {
                    onQuality(option.value)
                    setQualityOpen(false)
                  }}
                  className={`w-full rounded px-2 py-2 text-left transition ${option.value === requestedQuality ? 'bg-violet-400/20 text-white' : 'text-zinc-300 hover:bg-white/10 hover:text-white'}`}
                >
                  <div className="flex items-center justify-between gap-3">
                    <span className="truncate text-sm font-black">{option.label}</span>
                    {option.value === requestedQuality ? <span className="text-[10px] font-black uppercase text-violet-200">Selected</span> : null}
                  </div>
                  <div className="mt-0.5 truncate text-[11px] font-semibold text-zinc-500">{qualityOptionDetail(option.value, renditions)}</div>
                </button>
              ))}
            </div>
          ) : null}
        </div>
        <button
          type="button"
          onClick={onToggleTheater}
          title={isTheater ? 'Exit theater mode' : 'Theater mode'}
          aria-label={isTheater ? 'Exit theater mode' : 'Theater mode'}
          className={`h-9 shrink-0 rounded border px-3 text-xs font-black uppercase transition ${isTheater ? 'border-violet-300/40 bg-violet-400/20 text-violet-100' : 'border-white/10 bg-white/[0.08] text-white hover:bg-white/15'}`}
        >
          {isTheater ? 'Shrink' : 'Theater'}
        </button>
        <button
          type="button"
          onClick={onToggleFullscreen}
          aria-label={isFullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
          className="h-9 shrink-0 rounded border border-white/10 bg-white/[0.08] px-3 text-xs font-black text-white transition hover:bg-white/15"
        >
          {isFullscreen ? 'Exit' : 'Fullscreen'}
        </button>
        <button
          type="button"
          aria-expanded={detailsExpanded}
          onClick={() => onDetailsExpanded(!detailsExpanded)}
          className={`h-9 shrink-0 rounded border px-3 text-xs font-black uppercase transition ${detailsExpanded ? 'border-violet-300/40 bg-violet-400/20 text-violet-100' : 'border-white/10 bg-white/[0.06] text-zinc-200 hover:bg-white/10'}`}
        >
          {detailsExpanded ? 'Hide' : 'Settings'}
        </button>
      </div>
      {detailsExpanded ? (
        <div className="mt-2 max-h-[min(40vh,280px)] overflow-y-auto border-t border-white/10 pt-2">
          <div className="flex flex-wrap items-center gap-2">
            <span className="h-9 rounded border border-white/10 bg-white/[0.045] px-3 py-2 text-xs font-black uppercase text-zinc-300">
              {playbackState}
            </span>
            {!isVod ? (
              <>
                <span
                  title={liveDelayTooltip}
                  className={`h-9 rounded border px-3 py-2 text-xs font-black uppercase ${liveTone}`}
                >
                  LIVE {displayDelay === null ? '' : `+${fmtMetricSec(displayDelay)}`}
                </span>
                <button
                  type="button"
                  onClick={onJumpLive}
                  disabled={!metrics.canJumpLive}
                  className="h-9 rounded border border-cyan-300/30 bg-cyan-400/10 px-3 text-xs font-black text-cyan-100 transition hover:bg-cyan-400/20 disabled:cursor-not-allowed disabled:border-white/10 disabled:bg-white/[0.04] disabled:text-zinc-500"
                >
                  Jump Live
                </button>
              </>
            ) : null}
            <div className="flex h-9 max-w-[22rem] items-center gap-2 rounded border border-white/10 bg-white/[0.045] px-3 text-xs font-black uppercase text-zinc-500">
              <span>Loaded</span>
              <span title={loadedQuality} className="truncate text-sm normal-case text-white">{loadedQuality}</span>
            </div>
            <div
              className="flex h-9 rounded border border-white/10 bg-white/[0.045] p-1"
              title={latencyModeAuto ? `Auto-switched to ${latencyMode} after buffering (change mode to override)` : undefined}
            >
              {(['instant', 'fast', 'stable'] as const).map(mode => (
                <button
                  key={mode}
                  type="button"
                  onClick={() => onLatencyMode(mode)}
                  className={`rounded px-3 py-1.5 text-xs font-black uppercase transition ${latencyMode === mode ? 'bg-white text-zinc-950' : 'text-zinc-400 hover:bg-white/10 hover:text-white'}`}
                >
                  {mode}
                </button>
              ))}
            </div>
            <div className="flex h-9 rounded border border-white/10 bg-white/[0.045] p-1">
              {([
                { id: 'fit', label: 'Fit', title: 'Show the whole video with letterboxing when needed' },
                { id: 'fill', label: 'Fill', title: 'Fill the player frame and crop edges when needed' },
              ] as const).map(mode => (
                <button
                  key={mode.id}
                  type="button"
                  onClick={() => onVideoFit(mode.id)}
                  title={mode.title}
                  aria-pressed={videoFit === mode.id}
                  className={`rounded px-3 py-1.5 text-xs font-black uppercase transition ${videoFit === mode.id ? 'bg-white text-zinc-950' : 'text-zinc-400 hover:bg-white/10 hover:text-white'}`}
                >
                  {mode.label}
                </button>
              ))}
            </div>
            <div className="flex h-9 rounded border border-white/10 bg-white/[0.045] p-1">
              {([
                { id: 'comfortable', label: 'Comfort' },
                { id: 'dense', label: 'Dense' },
              ] as const).map(mode => (
                <button
                  key={mode.id}
                  type="button"
                  onClick={() => onBottomDensity(mode.id)}
                  className={`rounded px-3 py-1.5 text-xs font-black uppercase transition ${bottomDensity === mode.id ? 'bg-white text-zinc-950' : 'text-zinc-400 hover:bg-white/10 hover:text-white'}`}
                >
                  {mode.label}
                </button>
              ))}
            </div>
            <div className="flex w-full flex-wrap gap-2 text-[11px] font-black uppercase text-zinc-500">
              <span className="rounded bg-white/[0.045] px-2 py-1" title={liveDelayTooltip}>Est. display {fmtMetricSec(displayDelay)}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1" title="hls.js latency to MediaMTX live edge">Player to origin {fmtMetricSec(metrics.latencyToLiveSec)}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1" title="Server measuredDelaySec at MediaMTX origin">Origin edge {fmtMetricSec(diagnostics?.measuredDelaySec)}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1">Behind sync {fmtMetricSec(metrics.behindLiveSec)}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1">Buffered {fmtMetricSec(metrics.bufferSizeSec)}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1">Target {fmtMetricSec(metrics.targetLatencySec)}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1">Stage {formatPlaybackStage(metrics.hlsStage)}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1">Backend {backend || '-'}</span>
              {diagnostics?.activeTransport ? <span className="rounded bg-white/[0.045] px-2 py-1">Transport {diagnostics.activeTransport}</span> : null}
              {latencyModeAuto ? <span className="rounded bg-amber-400/10 px-2 py-1 text-amber-100">Mode {latencyMode} (auto)</span> : <span className="rounded bg-white/[0.045] px-2 py-1">Mode {latencyMode}</span>}
              <span className="rounded bg-white/[0.045] px-2 py-1">Startup {startupMs ? `${startupMs}ms` : '-'}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1">First frame {fmtMs(metrics.firstFrameMs)}</span>
              {fallbackAttempted ? <span className="rounded bg-cyan-400/10 px-2 py-1 text-cyan-100">Fallback used</span> : null}
            </div>
          </div>
        </div>
      ) : null}
    </section>
  )
}

function FollowButton({ login }: { login: string }) {
  const queryClient = useQueryClient()
  const auth = useAuth()
  const followed = useQuery({
    queryKey: ['followed', auth.isAuthenticated],
    queryFn: () => getFollowedChannels(auth.isAuthenticated),
    retry: false,
    staleTime: 30_000,
  })
  const localFollowed = useQuery({
    queryKey: ['followed', 'local'],
    queryFn: getLocalFollowedChannels,
    retry: false,
    staleTime: 30_000,
  })
  const isLocalFollowing = localFollowed.data?.some(channel => channel.login === login) ?? false
  const isTwitchFollowing = followed.data?.some(channel => channel.login === login) ?? false
  const isFollowing = isLocalFollowing || isTwitchFollowing
  const twitchOnly = isTwitchFollowing && !isLocalFollowing
  const mutation = useMutation({
    mutationFn: () => (isLocalFollowing ? unfollowChannel(login) : followChannel(login)),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['followed'] })
      queryClient.invalidateQueries({ queryKey: ['followed', 'local'] })
    },
  })
  return (
    <button
      type="button"
      onClick={() => mutation.mutate()}
      disabled={mutation.isPending || twitchOnly}
      title={twitchOnly ? 'Following on Twitch — use Twitch to unfollow' : undefined}
      className={`rounded px-3 py-1 text-xs font-black uppercase tracking-wide transition disabled:opacity-60 ${isFollowing ? 'border border-white/15 bg-white/10 text-zinc-100 hover:bg-white/15' : 'bg-violet-500 text-white hover:bg-violet-400'}`}
    >
      {mutation.isPending ? 'Saving…' : isFollowing ? 'Following' : 'Follow'}
    </button>
  )
}

function ChannelMetaSkeleton({ dense }: { dense: boolean }) {
  return (
    <section className={`border-b border-white/10 bg-[#0e0e10] ${dense ? 'px-4 py-3 lg:px-6' : 'px-4 py-4 lg:px-6'}`}>
      <div className={dense ? 'space-y-3' : 'space-y-4'}>
        <div className="space-y-2">
          <div className="flex flex-wrap gap-2">
            <div className={`animate-pulse rounded bg-white/10 ${dense ? 'h-5 w-14' : 'h-5 w-16'}`} />
            <div className="h-5 w-24 animate-pulse rounded bg-white/10" />
          </div>
          <div className={`animate-pulse rounded bg-white/10 ${dense ? 'h-7 w-4/5' : 'h-8 w-3/4'}`} />
        </div>
        <div className={`flex flex-wrap items-center ${dense ? 'gap-2' : 'gap-3'}`}>
          <div className={`shrink-0 animate-pulse rounded-full bg-white/10 ${dense ? 'h-10 w-10' : 'h-12 w-12'}`} />
          <div className="min-w-0 flex-1 space-y-2">
            <div className="h-4 w-32 animate-pulse rounded bg-white/10" />
            <div className="h-3 w-full max-w-md animate-pulse rounded bg-white/10" />
          </div>
          <div className="flex shrink-0 gap-2">
            <div className="h-8 w-20 animate-pulse rounded bg-white/10" />
            <div className="h-8 w-28 animate-pulse rounded bg-white/10" />
          </div>
        </div>
      </div>
    </section>
  )
}

function OpenFullAnalyticsLink({ channel, prominent = false }: { channel: string; prominent?: boolean }) {
  return (
    <a
      href={`/analytics/${encodeURIComponent(channel)}`}
      className={`rounded bg-violet-600 font-black text-white transition hover:bg-violet-500 ${
        prominent ? 'px-4 py-2 text-xs' : 'px-3 py-1 text-xs uppercase tracking-wide'
      }`}
    >
      Open Full Analytics →
    </a>
  )
}

function ChannelMeta({
  login,
  details,
  detailsLoading,
  quality,
  listeners,
  dense,
}: {
  login: string
  details?: ChannelDetails
  detailsLoading: boolean
  quality: string
  listeners: number | null
  dense: boolean
}) {
  if (detailsLoading && !details) {
    return <ChannelMetaSkeleton dense={dense} />
  }
  const display = details?.displayName || login
  const title = details?.streamTitle || (detailsLoading ? 'Loading stream details' : `${display}'s channel`)
  const avatar = details?.profileImage
  return (
    <section className={`shrink-0 border-b border-white/10 bg-[#0e0e10] ${dense ? 'px-4 py-3 lg:px-6' : 'px-4 py-4 lg:px-6'}`}>
      <div className={dense ? 'space-y-3' : 'space-y-4'}>
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        {details?.isLive ? (
          <span className="rounded bg-red-600 px-2 py-0.5 text-[11px] font-bold uppercase tracking-wide text-white">Live</span>
        ) : (
          <span className="rounded bg-zinc-700 px-2 py-0.5 text-[11px] font-bold uppercase tracking-wide text-zinc-200">Offline</span>
        )}
        {details?.category ? <span className="text-sm font-semibold text-[#bf94ff]">{details.category}</span> : null}
        {details?.isLive && details.viewers != null ? (
          <span className="text-sm font-semibold text-zinc-300">{fullCount(details.viewers)} viewers</span>
        ) : null}
        {details?.startedAt ? <span className="text-sm text-zinc-500">· {relativeTime(details.startedAt)}</span> : null}
          </div>
          <h1 title={title} className={`font-semibold leading-snug text-white ${dense ? 'text-lg' : 'text-xl sm:text-2xl'}`}>{title}</h1>
        </div>
        <div className={`flex flex-wrap items-center gap-3 ${dense ? 'gap-2' : ''}`}>
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <div className={`grid shrink-0 place-items-center overflow-hidden rounded-full bg-zinc-800 text-sm font-black text-violet-100 ${dense ? 'h-10 w-10' : 'h-12 w-12'}`}>
            {avatar ? <img src={avatar} alt={display} className="h-full w-full object-cover" /> : display.slice(0, 1).toUpperCase()}
          </div>
          <div className="min-w-0">
            <div className="text-base font-semibold text-white">{display}</div>
            {!dense && details?.description ? (
              <p className="mt-1 line-clamp-2 text-sm leading-5 text-zinc-400">{details.description}</p>
            ) : null}
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          <FollowButton login={login} />
          <OpenFullAnalyticsLink channel={login} />
        </div>
      </div>
      {listeners != null && listeners > 0 ? (
        <div className="mt-3 border-t border-white/[0.06] pt-2 text-[11px] font-medium text-zinc-600">
          {fullCount(listeners)} watching on this relay · {quality}
        </div>
      ) : null}
      </div>
    </section>
  )
}

function StatCell({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-white/10 bg-white/[0.045] p-3">
      <div className="text-[11px] font-black uppercase text-zinc-500">{label}</div>
      <div className="mt-1 text-lg font-black text-white">{value}</div>
    </div>
  )
}

function PeriodPicker<T extends string>({ value, onChange }: { value: T; onChange: (value: T) => void }) {
  return (
    <div className="flex rounded border border-white/10 bg-white/[0.045] p-1">
      {periodOptions.map(option => (
        <button
          key={option.id}
          type="button"
          onClick={() => onChange(option.id as T)}
          className={`rounded px-3 py-1.5 text-xs font-black transition ${value === option.id ? 'bg-white text-zinc-950' : 'text-zinc-400 hover:bg-white/10 hover:text-white'}`}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}

function Sparkline({ points }: { points: StatsTimelinePoint[] | undefined }) {
  if (!points?.length) return null
  const values = points.map(point => point.avgViewers || point.peakViewers || 0)
  const max = Math.max(...values, 1)
  const coords = values.map((value, index) => {
    const x = values.length === 1 ? 0 : (index / (values.length - 1)) * 100
    const y = 48 - (value / max) * 42
    return `${x.toFixed(1)},${y.toFixed(1)}`
  }).join(' ')
  return (
    <div className="rounded border border-white/10 bg-white/[0.045] p-3">
      <div className="mb-2 flex items-center justify-between">
        <div className="text-[11px] font-black uppercase text-zinc-500">Viewer trend</div>
        <div className="text-xs font-bold text-zinc-400">{fullCount(max)} max</div>
      </div>
      <svg viewBox="0 0 100 52" className="h-24 w-full overflow-visible">
        <polyline fill="none" stroke="rgba(34,211,238,.35)" strokeWidth="8" strokeLinecap="round" points={coords} />
        <polyline fill="none" stroke="rgb(34,211,238)" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" points={coords} />
      </svg>
      <div className="mt-1 grid grid-cols-5 gap-1 text-[10px] font-bold uppercase text-zinc-500">
        {points.slice(0, 5).map((point, index) => <span key={`${point.label}-${index}`} className="truncate">{point.label}</span>)}
      </div>
    </div>
  )
}

function StreamHistoryTable({ rows, sources, channel }: { rows: StreamStat[] | undefined; sources?: SourceStatus[]; channel: string }) {
  if (!rows?.length) {
    const historySource = sources?.find(source => source.source === 'stream_history')
    return <EmptyPanel title="Summary-only history" detail={historySource ? sourceMessageText(historySource) : 'TwitchTracker summary stats loaded, but no stream-by-stream rows were returned.'} />
  }
  return (
    <div className="overflow-x-auto rounded border border-white/10 bg-white/[0.035]">
      <div className="grid min-w-[760px] grid-cols-[minmax(0,1.4fr)_100px_100px_100px_minmax(180px,1fr)] gap-3 border-b border-white/10 px-3 py-2 text-[11px] font-black uppercase text-zinc-500">
        <span>Stream</span>
        <span>Average</span>
        <span>Peak</span>
        <span>Watched</span>
        <span>Actions</span>
      </div>
      {rows.map(row => (
        <div
          key={row.id}
          className="grid min-w-[760px] grid-cols-[minmax(0,1.4fr)_100px_100px_100px_minmax(180px,1fr)] gap-3 border-b border-white/5 px-3 py-3 text-sm font-bold text-zinc-300 transition last:border-b-0 hover:bg-white/[0.05]"
        >
          <div className="min-w-0">
            <div className="truncate font-black text-white">{row.title}</div>
            <div className="mt-0.5 truncate text-xs text-zinc-500">{row.category || `${fullCount(row.durationMinutes)} minutes`}</div>
          </div>
          <span>{fullCount(row.avgViewers)}</span>
          <span>{fullCount(row.peakViewers)}</span>
          <span>{count(row.hoursWatched)}</span>
          <div className="flex flex-wrap items-center gap-2">
            <Link
              to={`/analytics/${encodeURIComponent(channel)}/${encodeURIComponent(row.id)}`}
              className="rounded border border-white/10 bg-white/[0.04] px-2 py-1 text-[11px] font-black uppercase text-cyan-300 hover:border-cyan-400/40"
            >
              Analytics
            </Link>
            {row.videoId ? (
              <Link
                to={buildVodDeepLink(channel, row.videoId, 0, row.id)}
                className="rounded border border-violet-400/20 bg-violet-500/10 px-2 py-1 text-[11px] font-black uppercase text-violet-200 hover:border-violet-300/40"
              >
                Play VOD
              </Link>
            ) : (
              <span className="text-[11px] font-semibold text-zinc-600">No VOD ID</span>
            )}
            {row.videoId ? (
              <a
                href={buildTwitchVodUrl(row.videoId)}
                target="_blank"
                rel="noopener noreferrer"
                className="rounded border border-white/10 bg-white/[0.04] px-2 py-1 text-[11px] font-black uppercase text-zinc-300 hover:border-violet-300/40"
              >
                Twitch
              </a>
            ) : null}
          </div>
        </div>
      ))}
    </div>
  )
}

function ClipGrid({ clips, sources }: { clips: ClipCard[]; sources?: SourceStatus[] }) {
  if (!clips.length) {
    return <EmptyPanel title="No clips loaded" detail={summarizeClipEmptyState(sources)} />
  }
  return (
    <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
      {clips.map(clip => (
        <a key={clip.id} href={clip.url} target="_blank" rel="noreferrer" className="group overflow-hidden rounded border border-white/10 bg-white/[0.045] transition hover:border-violet-300/60 hover:bg-white/[0.07]">
          <div className="relative aspect-video bg-zinc-900">
            {clip.thumbnailUrl ? <img src={clip.thumbnailUrl} alt={clip.title} className="h-full w-full object-cover transition duration-300 group-hover:scale-105" /> : null}
            <div className="absolute bottom-2 right-2 rounded bg-black/75 px-2 py-0.5 text-[11px] font-bold text-zinc-100">{fullCount(clip.viewCount)} views</div>
          </div>
          <div className="p-3">
            <div className="line-clamp-2 min-h-10 text-sm font-black leading-5 text-white">{clip.title}</div>
            <div className="mt-2 flex items-center justify-between gap-2 text-xs font-semibold text-zinc-500">
              <span className="truncate">{clip.creatorName || clip.broadcasterName || 'Twitch clip'}</span>
              <span>{relativeTime(clip.createdAt)}</span>
            </div>
          </div>
        </a>
      ))}
    </div>
  )
}



function EmptyPanel({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="grid min-h-32 place-items-center rounded border border-white/10 bg-white/[0.035] px-4 text-center">
      <div>
        <div className="text-sm font-black text-zinc-200">{title}</div>
        <div className="mt-1 text-xs font-semibold text-zinc-500">{detail}</div>
      </div>
    </div>
  )
}

function StatsGrid({ insights }: { insights?: ChannelInsights }) {
  const stats = insights?.stats
  if (!stats) {
    return <EmptyPanel title="No TwitchTracker summary" detail="The basic TwitchTracker API did not return a summary." />
  }
  return (
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <StatCell label="Tracker rank" value={stats.rank ? `#${fullCount(stats.rank)}` : '-'} />
      <StatCell label="Avg viewers" value={fullCount(stats.avg_viewers)} />
      <StatCell label="Peak viewers" value={fullCount(stats.max_viewers)} />
      <StatCell label="Hours watched" value={count(stats.hours_watched)} />
      <StatCell label="Hours streamed" value={fullCount(Math.round(stats.minutes_streamed / 60))} />
      <StatCell label="Followers gained" value={fullCount(stats.followers)} />
      <StatCell label="Followers total" value={count(stats.followers_total)} />
      <StatCell label="Updated" value={insights?.updatedAt ? relativeTime(insights.updatedAt / 1000) : '-'} />
    </div>
  )
}

function DerivedStatsGrid({ insights }: { insights?: ChannelInsights }) {
  const derived = insights?.statsDerived
  if (!derived) return null
  return (
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4 xl:grid-cols-5">
      <StatCell label="Streamed hours" value={derived.hoursStreamed === undefined ? '-' : fullCount(derived.hoursStreamed)} />
      <StatCell label="Viewer hrs / hr" value={derived.viewerHoursPerStreamHour === undefined ? '-' : fullCount(derived.viewerHoursPerStreamHour)} />
      <StatCell label="Peak / avg" value={derived.peakToAverageRatio === undefined ? '-' : `${derived.peakToAverageRatio.toFixed(2)}x`} />
      <StatCell label="Followers / hr" value={derived.followersPerStreamHour === undefined ? '-' : fullCount(derived.followersPerStreamHour)} />
      <StatCell label="Clips loaded" value={fullCount(derived.clipsLoaded)} />
    </div>
  )
}

function StatsContextPanel({ insights, diagnostics }: { insights?: ChannelInsights; diagnostics?: StreamDiagnostics }) {
  const hasHistory = Boolean(insights?.statsDerived?.hasRealStreamHistory)
  const hasStats = Boolean(insights?.stats)
  const historySource = insights?.sources?.find(source => source.source === 'stream_history')
  return (
    <div className="grid gap-3 xl:grid-cols-3">
      <div className="rounded border border-white/10 bg-white/[0.035] p-3">
        <div className="text-[11px] font-black uppercase text-zinc-500">History source</div>
        <div className="mt-1 text-sm font-black text-white">{hasHistory ? 'Stream rows loaded' : hasStats ? 'Summary stats loaded' : 'Waiting on stats source'}</div>
        <div className="mt-1 text-xs font-semibold text-zinc-500">{historySource ? sourceMessageText(historySource) : 'Per-stream rows will appear when TwitchTracker history is available.'}</div>
      </div>
      <div className="rounded border border-white/10 bg-white/[0.035] p-3">
        <div className="text-[11px] font-black uppercase text-zinc-500">Relay health</div>
        <div className="mt-1 text-sm font-black text-white">{diagnostics?.active ? 'Active' : '-'}</div>
        <div className="mt-1 text-xs font-semibold text-zinc-500">Restarts {diagnostics?.restarts ?? 0} / backend {diagnostics?.workerBackend || '-'}{diagnostics?.activeTransport ? ` / ${diagnostics.activeTransport}` : ''}</div>
      </div>
      <div className="rounded border border-white/10 bg-white/[0.035] p-3">
        <div className="text-[11px] font-black uppercase text-zinc-500">Context loaded</div>
        <div className="mt-1 text-sm font-black text-white">{fullCount(insights?.clips?.length ?? 0)} clips</div>
        <div className="mt-1 text-xs font-semibold text-zinc-500">These counts are loaded items for the selected periods, not all-time totals.</div>
      </div>
    </div>
  )
}

function ChannelDiagnosticsTab({
  channel,
  diagnostics,
  sessionId,
  isVod,
}: {
  channel: string
  diagnostics?: StreamDiagnostics
  sessionId?: string
  isVod?: boolean
}) {
  const { metrics, jumpLive, effectiveLatencyMode } = useChannelPlayback()
  return (
    <PlaybackDiagnostics
      channel={channel}
      metrics={metrics}
      diagnostics={diagnostics}
      sessionId={sessionId}
      onJumpLive={jumpLive}
      isVod={isVod}
      effectiveLatencyMode={effectiveLatencyMode}
    />
  )
}

function ChannelTabs({
  activeTab,
  onTab,
  insights,
  channel,
  details,
  diagnostics,
  streamSession,
  emotePanel,
  statsPeriod,
  clipPeriod,
  onStatsPeriod,
  onClipPeriod,
  dense,
  isVod = false,
  analyticsStreams,
  liveStreamId,
  trackLiveAnalytics,
  trackAnalyticsPending,
  onTrackAnalytics,
}: {
  activeTab: ChannelTab
  onTab: (tab: ChannelTab) => void
  insights?: ChannelInsights
  channel: string
  details?: ChannelDetails
  diagnostics?: StreamDiagnostics
  streamSession: StartResponse | null
  emotePanel: ReactNode
  statsPeriod: StatsPeriod
  clipPeriod: ClipPeriod
  onStatsPeriod: (period: StatsPeriod) => void
  onClipPeriod: (period: ClipPeriod) => void
  dense: boolean
  isVod?: boolean
  analyticsStreams?: AnalyticsStream[]
  liveStreamId?: string | null
  trackLiveAnalytics?: boolean
  trackAnalyticsPending?: boolean
  onTrackAnalytics?: (track: boolean) => void
}) {
  const statsSources = (insights?.sources ?? []).filter(source => sourceMatchesGroup(source, 'stats'))
  const clipSources = (insights?.sources ?? []).filter(source => sourceMatchesGroup(source, 'clips'))
  return (
    <ChannelTabShell activeTab={activeTab} onTab={onTab} dense={dense}>
      {activeTab === 'diagnostics' ? (
        <p className="text-xs font-semibold leading-relaxed text-zinc-500">
          Advanced playback tools for comparing local HLS relay latency against Twitch&apos;s embed player. Most viewers can stay on About or Stats — open this tab when tuning buffer, quality, or relay startup.
        </p>
      ) : null}

      {activeTab === 'about' ? <ChannelDetailSections details={details} dense={dense} /> : null}
      {activeTab === 'stats' ? (
        <div className={dense ? 'space-y-3' : 'space-y-4'}>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <PeriodPicker value={statsPeriod} onChange={onStatsPeriod} />
            <SourcePills sources={statsSources} />
          </div>
          <StatsGrid insights={insights} />
          <DerivedStatsGrid insights={insights} />
          <StatsContextPanel insights={insights} diagnostics={diagnostics} />
          <Sparkline points={insights?.statsTimeline} />
          <StreamHistoryTable rows={insights?.streamHistory} sources={statsSources} channel={channel} />
          <SourceDiagnostics sources={statsSources} />
        </div>
      ) : null}
      {activeTab === 'clips' ? (
        <div className={dense ? 'space-y-3' : 'space-y-4'}>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <PeriodPicker value={clipPeriod} onChange={onClipPeriod} />
            <SourcePills sources={clipSources} />
          </div>
          <ClipGrid clips={insights?.clips ?? []} sources={clipSources} />
          {clipSources.some(source => source.state !== 'ready') ? <SourceDiagnostics sources={clipSources} /> : null}
        </div>
      ) : null}
      {activeTab === 'vods' ? (
        <div className={dense ? 'space-y-3' : 'space-y-4'}>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h3 className="text-sm font-semibold text-white">Stream History & VODs</h3>
            {details?.isLive && onTrackAnalytics ? (
              <TrackAnalyticsToggle
                tracked={Boolean(trackLiveAnalytics)}
                pending={Boolean(trackAnalyticsPending)}
                onToggle={onTrackAnalytics}
                prominent
              />
            ) : null}
          </div>
          <ChannelVodsPanel
            rows={insights?.streamHistory}
            analyticsStreams={analyticsStreams}
            liveStreamId={liveStreamId}
            sources={statsSources}
            channel={channel}
          />
        </div>
      ) : null}
      {activeTab === 'diagnostics' ? (
        <ChannelDiagnosticsTab channel={channel} diagnostics={diagnostics} sessionId={streamSession?.session_id} isVod={isVod} />
      ) : null}
      {activeTab === 'emotes' ? emotePanel : null}
    </ChannelTabShell>
  )
}

type AboutPanelCardProps = { panel: AboutPanel; index: number }

function AboutPanelCard({ panel, index }: AboutPanelCardProps) {
  const [tall, setTall] = useState(false)
  const panelTitle = normalizePanelText(panel.title, panel.linkUrl ? compactUrl(panel.linkUrl) : 'Panel')
  const panelBody = panel.description?.trim()
  const body = (
    <div className="h-full overflow-hidden rounded-md border border-white/10 bg-[#18181b]">
      {panel.imageUrl ? (
        <img
          src={panel.imageUrl}
          alt={panelTitle}
          className="block w-full"
          loading="lazy"
          onLoad={event => {
            const img = event.currentTarget
            if (img.naturalHeight > img.naturalWidth * 1.5) setTall(true)
          }}
        />
      ) : null}
      {panelBody ? (
        <div className={`text-sm leading-6 text-zinc-300 ${panel.imageUrl ? 'px-3 py-3' : 'px-3 py-4'}`}>
          <div className="whitespace-pre-wrap">{panelBody}</div>
        </div>
      ) : null}
    </div>
  )
  const className = tall ? 'row-span-2' : ''
  if (panel.linkUrl) {
    return (
      <a
        key={panel.id || panel.linkUrl || index}
        href={panel.linkUrl}
        target="_blank"
        rel="noreferrer"
        className={`block transition hover:opacity-90 ${className}`}
      >
        {body}
      </a>
    )
  }
  return <div key={panel.id || index} className={className}>{body}</div>
}

function ChannelDetailSections({ details, dense }: { details?: ChannelDetails; dense: boolean }) {
  const displayName = details?.displayName || details?.login || 'Channel'
  const panels = details?.aboutPanels ?? []
  const socialLinks = details?.socialLinks ?? []
  const aboutSources = (details?.sources ?? []).filter(source => source.source === 'twitch_gql_about_panels')
  const aboutIssue = aboutSources.find(source => source.state !== 'ready')

  return (
    <div className={dense ? 'space-y-5' : 'space-y-6'}>
      <section>
        <h3 className="text-base font-semibold text-white">About {displayName}</h3>
        <p className="mt-3 max-w-3xl whitespace-pre-wrap text-sm leading-7 text-zinc-300">
          {details?.description || 'This channel has not set a profile description yet.'}
        </p>
        {details?.createdAt ? (
          <p className="mt-3 text-sm text-zinc-500">
            Channel created {calendarDate(details.createdAt)}
            {details.isLive && details.viewers != null ? ` · ${fullCount(details.viewers)} watching now` : ''}
          </p>
        ) : null}
      </section>

      {socialLinks.length ? (
        <section className="border-t border-white/10 pt-4">
          <h4 className="text-sm font-semibold text-white">Links</h4>
          <div className="mt-3 flex flex-wrap gap-2">
            {socialLinks.filter(link => link.url).map((link, index) => (
              <SocialLinkChip key={link.id || link.url || index} url={link.url!} title={link.title} />
            ))}
          </div>
        </section>
      ) : null}

      {panels.length ? (
        <section>
          <h4 className="mb-3 text-sm font-semibold text-white">Panels</h4>
          <div className="grid w-full auto-rows-min grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
            {panels.map((panel, index) => (
              <AboutPanelCard key={panel.id || panel.linkUrl || index} panel={panel} index={index} />
            ))}
          </div>
        </section>
      ) : (
        <section>
          <h4 className="mb-3 text-sm font-semibold text-white">Panels</h4>
          <div className="grid w-full auto-rows-min grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
            <div className="col-span-full space-y-3">
              <EmptyPanel title="No panels yet" detail={aboutIssue ? sourceMessageText(aboutIssue) : 'Custom About panels from Twitch will appear here when metadata is available.'} />
              {aboutIssue ? <SourceDiagnostics sources={aboutSources} /> : null}
            </div>
          </div>
        </section>
      )}
    </div>
  )
}

export default function Channel() {
  return (
    <ChannelPlaybackProvider>
      <ChannelPage />
    </ChannelPlaybackProvider>
  )
}

function ChannelPage() {
  const { login } = useParams<{ login: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const queryClient = useQueryClient()
  const channelLogin = login ?? ''
  const rawVodParam = searchParams.get('vod')
  const vodParamPresent = (rawVodParam?.trim().length ?? 0) > 0
  const vodPlaybackId = normalizeVodId(rawVodParam) ?? ''
  const isVodPlayback = vodPlaybackId.length > 0
  const vodIdInvalid = vodParamPresent && !isVodPlayback
  const vodOffsetSeconds = Math.max(0, Number.parseInt(searchParams.get('offset') || '0', 10) || 0)
  const [relayStartOffset, setRelayStartOffset] = useState(vodOffsetSeconds)
  const prevVodPlaybackIdRef = useRef('')
  const vodAnalyticsContext = parseVodAnalyticsContext(searchParams, channelLogin, isVodPlayback)
  const { fromAnalytics: vodFromAnalytics, streamId: vodAnalyticsStreamId, analyticsHref: vodAnalyticsHref } = vodAnalyticsContext
  const vodEmbedPrimary = preferTwitchEmbedReview(isVodPlayback, vodFromAnalytics, vodAnalyticsStreamId)
  const showAnalyticsActivityWaveform = isVodPlayback && vodFromAnalytics && Boolean(vodAnalyticsStreamId)
  const relaySessionKey = isVodPlayback ? vodSessionKey(vodPlaybackId) : channelLogin
  const twitchEmbedRef = useRef<TwitchVodPlayerHandle | null>(null)
  const [vodUseTwitchEmbed, setVodUseTwitchEmbed] = useState(vodEmbedPrimary)
  const [vodEmbedFallbackMode, setVodEmbedFallbackMode] = useState(false)
  const [embedMountReady, setEmbedMountReady] = useState(false)
  const showTwitchEmbed = isVodPlayback && vodUseTwitchEmbed
  const isEmbedAnalyticsReview = isEmbedAnalyticsVodReview(showTwitchEmbed, vodFromAnalytics)
  const [vodSeekOnStart, setVodSeekOnStart] = useState(0)
  const [vodRelayEpoch, setVodRelayEpoch] = useState(0)
  const [vodSeekPending, setVodSeekPending] = useState(false)
  const videoRef = useRef<HTMLVideoElement>(null)
  const playerFrameRef = useRef<HTMLDivElement>(null)
  const sessionIdRef = useRef<string | undefined>()
  const internalVodSeekRef = useRef(false)
  const vodHotRestartRef = useRef(false)
  const hotRestartVodRelayRef = useRef<(absoluteOffset: number) => void>(() => {})
  const pendingFarSeekRef = useRef<number | null>(null)
  const farSeekTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const keepaliveIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [vodRelayError, setVodRelayError] = useState<VodErrorInput | null>(null)
  const [retryKey, setRetryKey] = useState(0)
  const [relayState, setRelayState] = useState<PlaybackState>('starting')
  const [hlsUrl, setHlsUrl] = useState('')
  const [streamSession, setStreamSession] = useState<StartResponse | null>(null)
  const [listeners, setListeners] = useState<number | null>(null)
  const [mobileRailOpen, setMobileRailOpen] = useState(false)
  const [mobilePane, setMobilePane] = useState<MobileChannelPane>('watch')
  const [activeTab, setActiveTab] = useState<ChannelTab>('about')
  const [detailsExpanded, setDetailsExpanded] = useState(false)
  const [statsPeriod, setStatsPeriod] = useState<StatsPeriod>('7d')
  const [clipPeriod, setClipPeriod] = useState<ClipPeriod>('7d')
  const [chatSidebarTab, setChatSidebarTab] = useState<ChatSidebarTab>(() =>
    defaultAnalyticsVodSidebarTab(vodFromAnalytics, vodAnalyticsStreamId),
  )
  const [pulseAutoUpdate, setPulseAutoUpdate] = useState(true)
  const lsfRefreshRef = useRef(false)
  const [emoteStatus, setEmoteStatus] = useState<ChatEmoteStatus>({ state: 'idle', count: 0, pending: 0 })
  const [emoteLoadRequest, setEmoteLoadRequest] = useState<{ providers: EmoteProvider[]; token: number } | null>(null)
  const [muted, setMuted] = useState(true)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [isTheater, setIsTheater] = useState(false)
  const [isChannelOffline, setIsChannelOffline] = useState(false)
  const [startupStartedAt, setStartupStartedAt] = useState(() => Date.now())
  const [startupNow, setStartupNow] = useState(() => Date.now())
  const [coarsePlaybackState, setCoarsePlaybackState] = useState<PlaybackState>('starting')
  const [startupBenchmarks, setStartupBenchmarks] = useState<StartupBenchmarkEntry[]>([])
  const autoRetryAttemptsRef = useRef(0)
  const relayMountGenRef = useRef(0)
  const auth = useAuth()
  const settings = useUiSettings(s => s.settings)
  const updateSettings = useUiSettings(s => s.updateSettings)
  const [railCollapsed, setRailCollapsed] = useState(false)
  const toggleSavedEmoteProvider = useUiSettings(s => s.toggleEmoteProvider)
  const selectedEmoteProviders = settings.emoteProviders
  const autoLoadEmotes = settings.emoteAutoLoad
  useThemeEffect(settings.theme)
  const subscribe = useChatStore(s => s.subscribe)
  const unsubscribe = useChatStore(s => s.unsubscribe)
  const connectionState = useChatStore(s => s.connectionState)
  const emoteDeltaToken = useChatStore(s => s.emoteDeltaToken)
  const retryStream = useCallback(() => {
    setError(null)
    setHlsUrl('')
    setRetryKey(k => k + 1)
  }, [])
  const handleUnauthorizedHls = useCallback(() => {
    if (isVodPlayback) {
      // VOD: keep relay alive; playback.ts reloads the playlist and re-seeks in-player.
      return
    }
    if (autoRetryAttemptsRef.current >= 2) {
      setRelayState('error')
      setError('Video relay unavailable. Try Retry in a moment or switch channels.')
      setHlsUrl('')
      return
    }
    autoRetryAttemptsRef.current += 1
    retryStream()
  }, [isVodPlayback, retryStream])
  const vodRelayBaseSeconds = useMemo(() => {
    if (!isVodPlayback) return 0
    const resp = streamSession as VodStartResponse | null
    if (!resp || typeof resp.offset_seconds !== 'number') return 0
    return Math.max(0, resp.offset_seconds - buildVodSeekTarget(resp.offset_seconds, resp.seek_seconds))
  }, [isVodPlayback, streamSession])
  const handleVodRelayStale = useCallback(() => {
    if (!isVodPlayback || vodHotRestartRef.current) return
    const video = videoRef.current
    let absolute = vodOffsetSeconds
    if (video && Number.isFinite(video.currentTime)) {
      absolute = vodRelayBaseSeconds + Math.max(0, video.currentTime)
    }
    void hotRestartVodRelayRef.current(Math.floor(absolute))
  }, [isVodPlayback, vodOffsetSeconds, vodRelayBaseSeconds])
  const playbackOptions = useMemo(() => ({
    src: hlsUrl,
    enabled: Boolean(hlsUrl),
    muted,
    autoPlay: true,
    mode: (isVodPlayback ? 'vod' : 'live') as 'vod' | 'live',
    seekOnStart: isVodPlayback ? vodSeekOnStart : undefined,
    relayEpoch: isVodPlayback ? vodRelayEpoch : undefined,
    vodRepositioning: isVodPlayback && vodSeekPending,
    latencyMode: isVodPlayback ? 'stable' : settings.playbackLatencyMode,
    onUnauthorizedHls: handleUnauthorizedHls,
    onVodRelayStale: isVodPlayback ? handleVodRelayStale : undefined,
  }), [
    hlsUrl,
    muted,
    isVodPlayback,
    vodSeekOnStart,
    vodRelayEpoch,
    vodSeekPending,
    settings.playbackLatencyMode,
    handleUnauthorizedHls,
    handleVodRelayStale,
  ])

  // Same-page playhead sync (Req 22.1): while in VOD mode, publish the current
  // playback position into the shared Zustand store at >= 1 Hz so a co-located
  // analytics chart cursor can follow it. The stream id stored is the analytics
  // stream id from the deep link (`sid`) when present so the chart's
  // `chartStreamId` can match it; otherwise we fall back to the VOD id. The
  // relay timeline is offset from absolute VOD time, so we map the video
  // element's currentTime back to an absolute VOD offset using the relay's
  // start response (currentTime === seekTarget corresponds to the requested
  // offset_seconds).
  const vodPlayheadStreamId = vodAnalyticsStreamId || vodPlaybackId

  useEffect(() => {
    if (prevVodPlaybackIdRef.current === vodPlaybackId) return
    prevVodPlaybackIdRef.current = vodPlaybackId
    setRelayStartOffset(vodOffsetSeconds)
  }, [vodPlaybackId, vodOffsetSeconds])

  const hotRestartVodRelay = useCallback(async (absoluteOffset: number) => {
    if (!isVodPlayback || !vodPlaybackId || vodHotRestartRef.current) return
    const safe = Math.max(0, Math.floor(absoluteOffset))
    vodHotRestartRef.current = true
    setVodSeekPending(true)
    setHlsUrl('')
    const prevSession = sessionIdRef.current
    setRelayState('buffering')
    setVodRelayError(null)
    try {
      if (keepaliveIntervalRef.current) {
        clearInterval(keepaliveIntervalRef.current)
        keepaliveIntervalRef.current = null
      }
      await stopStream(relaySessionKey, prevSession).catch(() => undefined)
      const vodStartQuality = vodFromAnalytics && settings.preferredQuality === 'best'
        ? autoHighStableQuality
        : settings.preferredQuality
      const response = await startVodPlayback(
        vodPlaybackId,
        safe,
        requestQuality(vodStartQuality),
        'stable',
      )
      sessionIdRef.current = response.session_id
      setStreamSession(response)
      setListeners(response.listeners ?? null)
      const vodResponse = response as VodStartResponse
      const seekTarget = buildVodSeekTarget(vodResponse.offset_seconds, vodResponse.seek_seconds)
      setVodSeekOnStart(seekTarget)
      setVodRelayEpoch(epoch => epoch + 1)
      const playableUrl = await resolvePlayableHlsUrl(response.hlsUrl)
      setHlsUrl(playableUrl)
      setRelayState('playing')
      keepaliveIntervalRef.current = setInterval(() => {
        keepaliveStream(relaySessionKey, sessionIdRef.current).catch(() => undefined)
      }, 20000)
    } catch (e) {
      setRelayState('error')
      if (e instanceof ApiError) {
        setVodRelayError({
          code: e.code,
          message: e.message,
          retryable: e.retryable,
          reason: e.reason,
        })
      } else {
        setVodRelayError({
          message: (e as Error).message || 'VOD seek failed',
        })
      }
    } finally {
      vodHotRestartRef.current = false
      setVodSeekPending(false)
    }
  }, [isVodPlayback, relaySessionKey, settings.preferredQuality, vodFromAnalytics, vodPlaybackId])

  hotRestartVodRelayRef.current = (absoluteOffset: number) => {
    void hotRestartVodRelay(absoluteOffset)
  }

  const seekVodAbsoluteOffset = useCallback((absoluteOffset: number) => {
    const safe = Math.max(0, Math.floor(absoluteOffset))
    internalVodSeekRef.current = true
    setSearchParams(prev => {
      const next = new URLSearchParams(prev)
      next.set('offset', String(safe))
      return next
    }, { replace: true })

    const clearInternalSeek = () => {
      requestAnimationFrame(() => {
        internalVodSeekRef.current = false
      })
    }

    if (showTwitchEmbed) {
      twitchEmbedRef.current?.seek(safe)
      clearInternalSeek()
      return
    }

    const video = videoRef.current
    const relative = vodRelativeSeekSeconds(safe, vodRelayBaseSeconds)
    const seekableRanges = video ? readVideoSeekableRanges(video) : []
    const duration = video && Number.isFinite(video.duration) ? video.duration : null
    const restartNeeded = needsVodRelayRestart(safe, vodRelayBaseSeconds, seekableRanges, 30, duration)

    if (!restartNeeded && video) {
      pendingFarSeekRef.current = null
      if (farSeekTimerRef.current) {
        clearTimeout(farSeekTimerRef.current)
        farSeekTimerRef.current = null
      }
      setVodSeekPending(false)
      video.currentTime = relative
      playbackActionsRef.current?.seekVodRelay(relative)
      clearInternalSeek()
      return
    }

    pendingFarSeekRef.current = safe
    setVodSeekPending(true)
    if (farSeekTimerRef.current) {
      clearTimeout(farSeekTimerRef.current)
    }
    farSeekTimerRef.current = window.setTimeout(() => {
      const target = pendingFarSeekRef.current
      pendingFarSeekRef.current = null
      farSeekTimerRef.current = null
      if (target != null) {
        void hotRestartVodRelay(target)
      } else {
        setVodSeekPending(false)
      }
    }, 300)
    clearInternalSeek()
  }, [hotRestartVodRelay, setSearchParams, showTwitchEmbed, vodRelayBaseSeconds])

  useEffect(() => () => {
    if (farSeekTimerRef.current) {
      clearTimeout(farSeekTimerRef.current)
    }
  }, [])

  useEffect(() => {
    setVodUseTwitchEmbed(preferTwitchEmbedReview(isVodPlayback, vodFromAnalytics, vodAnalyticsStreamId))
    setVodEmbedFallbackMode(false)
  }, [channelLogin, isVodPlayback, retryKey, vodAnalyticsStreamId, vodFromAnalytics, vodPlaybackId])

  const details = useQuery({
    queryKey: ['channel-details', channelLogin],
    queryFn: () => getChannelDetails(channelLogin),
    enabled: Boolean(channelLogin),
    staleTime: 15_000,
  })
  const badgeCatalog = useQuery({
    queryKey: ['channel-badges', channelLogin],
    queryFn: () => getChannelBadges(channelLogin),
    enabled: Boolean(channelLogin),
    staleTime: 10 * 60_000,
  })
  const insights = useQuery({
    queryKey: ['channel-insights', channelLogin, statsPeriod, clipPeriod],
    queryFn: () => getChannelInsights(channelLogin, statsPeriod, clipPeriod),
    enabled: Boolean(channelLogin),
    staleTime: 60_000,
    retry: 2,
  })
  const diagnostics = useQuery({
    queryKey: ['stream-diagnostics', channelLogin, hlsUrl, coarsePlaybackState],
    queryFn: () => getStreamDiagnostics(channelLogin),
    enabled: Boolean(channelLogin && hlsUrl && !isVodPlayback),
    refetchInterval: hlsUrl ? 5000 : false,
  })
  const emotePreview = useQuery({
    queryKey: ['channel-emotes', channelLogin, emoteStatus.state, emoteStatus.count, emoteDeltaToken],
    queryFn: () => getChannelEmotes(channelLogin),
    enabled: Boolean(channelLogin),
    staleTime: 5000,
    refetchInterval: emoteStatus.state === 'loading' || emoteStatus.state === 'processing' || emoteStatus.state === 'partial' ? 5000 : false,
  })
  const isLiveStream = Boolean(details.data?.isLive && !isVodPlayback)
  const showPulseSidebarTab = isLiveStream || showAnalyticsActivityWaveform || (isVodPlayback && vodFromAnalytics)

  const liveAnalytics = useAnalyticsLive(channelLogin, {
    enabled: Boolean(channelLogin) && (isLiveStream || chatSidebarTab === 'pulse'),
  })

  const pulseInsights = useQuery({
    queryKey: ['channel-pulse-lsf', channelLogin, '7d', 'top'],
    queryFn: () => {
      const lsfRefresh = lsfRefreshRef.current
      lsfRefreshRef.current = false
      return getChannelInsights(channelLogin, '7d', '24h', '7d', 'top', { lsfRefresh })
    },
    enabled: Boolean(channelLogin && isLiveStream),
    refetchInterval: query => {
      const warming = isLsfWarming(query.state.data?.sources)
      const hasLsf = (query.state.data?.lsf?.length ?? 0) > 0
      if (warming && !hasLsf) return 15_000
      if (!pulseAutoUpdate) return false
      if (query.state.status !== 'success') return false
      return 60_000
    },
    staleTime: 15_000,
  })

  const handleLoadPulseLsf = useCallback(() => {
    lsfRefreshRef.current = true
    void pulseInsights.refetch()
  }, [pulseInsights])

  const alwaysTracked = useQuery({
    queryKey: ['analytics-always-tracked'],
    queryFn: getAlwaysTracked,
    staleTime: 60_000,
  })
  const trackLiveAnalytics = useMemo(() => {
    const channels = alwaysTracked.data?.channels ?? []
    return channels.some(entry => entry.toLowerCase() === channelLogin.toLowerCase())
  }, [alwaysTracked.data?.channels, channelLogin])

  const trackAnalyticsMutation = useMutation({
    mutationFn: (track: boolean) => setAlwaysTracked(channelLogin, track),
    onSuccess: (_data, track) => {
      void queryClient.invalidateQueries({ queryKey: ['analytics-always-tracked'] })
      void queryClient.invalidateQueries({ queryKey: analyticsLiveQueryKey(channelLogin) })
      if (track) {
        watchAnalyticsChannel(channelLogin).catch(() => undefined)
      }
    },
  })

  const analyticsStreamsQuery = useQuery({
    queryKey: ['channel-analytics-streams', channelLogin],
    queryFn: () => getAnalyticsStreams(channelLogin, 50),
    enabled: Boolean(channelLogin),
    staleTime: 60_000,
  })

  const vodRollupsQuery = useQuery({
    queryKey: ['vod-activity-rollups', vodAnalyticsStreamId, channelLogin],
    queryFn: () => getAnalyticsStream(vodAnalyticsStreamId, { channel: channelLogin }),
    enabled: Boolean(isVodPlayback && vodFromAnalytics && vodAnalyticsStreamId),
    staleTime: 120_000,
    retry: 1,
  })
  const vodHeatmapQuery = useQuery({
    queryKey: ['vod-replay-heatmap', vodAnalyticsStreamId, channelLogin],
    queryFn: () => getReplayHeatmapDetail(vodAnalyticsStreamId, 60, channelLogin),
    enabled: Boolean(isVodPlayback && vodFromAnalytics && vodAnalyticsStreamId),
    staleTime: 120_000,
    retry: 1,
  })
  const activityRollups = vodRollupsQuery.data?.rollups ?? null
  const hasActivityRollups = (activityRollups?.length ?? 0) > 0
  const vodDetailDurationSec = useMemo(
    () => resolveVodDetailDurationSec(vodRollupsQuery.data),
    [vodRollupsQuery.data],
  )

  useEffect(() => {
    if (isVodPlayback && vodFromAnalytics && vodAnalyticsStreamId) {
      setChatSidebarTab('pulse')
    }
  }, [isVodPlayback, vodPlaybackId, vodFromAnalytics, vodAnalyticsStreamId])

  useEffect(() => {
    if (!channelLogin || !trackLiveAnalytics) return
    watchAnalyticsChannel(channelLogin).catch(() => undefined)
  }, [channelLogin, trackLiveAnalytics])

  useEffect(() => {
    if (!showPulseSidebarTab && chatSidebarTab === 'pulse') {
      setChatSidebarTab('chat')
    }
  }, [showPulseSidebarTab, chatSidebarTab])

  useEffect(() => {
    if (!channelLogin) return
    setStartupBenchmarks([])
  }, [channelLogin])

  useEffect(() => {
    if (!streamSession?.session_id) return
    setStartupBenchmarks(current => {
      const remaining = current.filter(entry => entry.sessionId !== streamSession.session_id)
      return [{
        sessionId: streamSession.session_id,
        attempt: retryKey + 1,
        backend: streamSession.workerBackend || '-',
        relayStartupMs: streamSession.startupMs ?? null,
        firstFrameMs: null,
        fallbackUsed: Boolean(streamSession.fallbackAttempted),
        startupBreakdown: streamSession.startupBreakdown,
      }, ...remaining].slice(0, 4)
    })
  }, [retryKey, streamSession])

  const handleFirstFrameMs = useCallback((firstFrameMs: number) => {
    if (!streamSession?.session_id) return
    setStartupBenchmarks(current => current.map(entry => (
      entry.sessionId === streamSession.session_id
        ? { ...entry, firstFrameMs }
        : entry
    )))
  }, [streamSession?.session_id])

  useEffect(() => {
    if (!channelLogin) return
    if (vodIdInvalid) {
      setRelayState('error')
      setVodRelayError({ code: 'invalid_vod_id' })
      setError(null)
      setHlsUrl('')
      setStreamSession(null)
      return
    }
    let alive = true
    let intervalId: ReturnType<typeof setInterval> | null = null
    const mountGen = ++relayMountGenRef.current
    sessionIdRef.current = undefined
    setError(null)
    if (retryKey === 0) {
      setVodRelayError(null)
    }
    setIsChannelOffline(false)
    setStartupStartedAt(Date.now())
    setRelayState(retryKey > 0 ? 'retrying' : 'starting')
    setHlsUrl('')
    setStreamSession(null)
    setListeners(null)
    setVodSeekOnStart(isVodPlayback ? estimateVodPlayerSeekTarget(relayStartOffset) : 0)

    if (!isVodPlayback) {
      subscribe(channelLogin)
    }

    if (isVodPlayback && vodUseTwitchEmbed) {
      setRelayState('starting')
      return () => {
        alive = false
      }
    }

    const start = async () => {
      try {
        const vodStartQuality = isVodPlayback && vodFromAnalytics && settings.preferredQuality === 'best'
          ? autoHighStableQuality
          : settings.preferredQuality
        const requestedQuality = requestQuality(vodStartQuality)
        const response: StartResponse | VodStartResponse = isVodPlayback
          ? await startVodPlayback(vodPlaybackId, relayStartOffset, requestedQuality, 'stable')
          : await startStream(channelLogin, requestedQuality, settings.playbackLatencyMode)
        if (!alive) {
          await stopStream(relaySessionKey, response.session_id).catch(() => undefined)
          return
        }
        sessionIdRef.current = response.session_id
        setStreamSession(response)
        setListeners(response.listeners ?? null)
        const playableUrl = await resolvePlayableHlsUrl(response.hlsUrl)
        if (!alive) {
          await stopStream(relaySessionKey, response.session_id).catch(() => undefined)
          return
        }
        if (isVodPlayback) {
          const vodResponse = response as VodStartResponse
          const seekTarget = buildVodSeekTarget(vodResponse.offset_seconds, vodResponse.seek_seconds)
          setVodSeekOnStart(seekTarget)
        }
        setHlsUrl(playableUrl)
        setRelayState('playing')
        setVodRelayError(null)

        keepaliveIntervalRef.current = setInterval(() => {
          keepaliveStream(relaySessionKey, sessionIdRef.current).catch(() => undefined)
        }, 20000)
        intervalId = keepaliveIntervalRef.current
      } catch (e) {
        if (!alive) return
        const isOffline = !isVodPlayback && e instanceof ApiError && e.code === 'channel_offline'
        setIsChannelOffline(isOffline)
        setRelayState('error')
        if (isVodPlayback) {
          if (shouldUseTwitchEmbedFallback(e, { fromAnalytics: vodFromAnalytics }) && !vodUseTwitchEmbed) {
            setVodUseTwitchEmbed(true)
            setVodEmbedFallbackMode(true)
            setVodRelayError(null)
            setRelayState('starting')
            return
          }
          if (
            e instanceof ApiError
            && e.code === 'hls_not_ready'
            && retryKey < HLS_NOT_READY_MAX_AUTO_RETRIES
          ) {
            setRetryKey(k => k + 1)
            return
          }
          if (e instanceof ApiError) {
            setVodRelayError({
              code: e.code,
              message: e.message,
              retryable: e.retryable,
              reason: e.reason,
            })
          } else {
            setVodRelayError({
              message: (e as Error).message || 'VOD playback failed',
            })
          }
          setError(null)
        } else if (isOffline) {
          setError('This channel is currently offline.')
        } else {
          setError((e as Error).message || 'stream start failed')
        }
      }
    }
    start()

    return () => {
      alive = false
      if (intervalId) clearInterval(intervalId)
      if (keepaliveIntervalRef.current === intervalId) {
        keepaliveIntervalRef.current = null
      }
      if (!isVodPlayback) unsubscribe(channelLogin)
      const sessionToStop = sessionIdRef.current
      queueMicrotask(() => {
        if (relayMountGenRef.current !== mountGen) return
        stopStream(relaySessionKey, sessionToStop).catch(() => undefined)
      })
    }
  }, [channelLogin, isVodPlayback, relaySessionKey, relayStartOffset, retryKey, settings.preferredQuality, subscribe, unsubscribe, vodFromAnalytics, vodIdInvalid, vodPlaybackId, vodUseTwitchEmbed])

  useEffect(() => {
    setEmoteStatus({ state: 'idle', count: 0, pending: 0 })
    setEmoteLoadRequest(null)
    if (channelLogin && autoLoadEmotes) {
      setEmoteLoadRequest({ providers: [...selectedEmoteProviders], token: Date.now() })
    }
  }, [channelLogin])

  useEffect(() => {
    if (!channelLogin || !emoteLoadRequest || details.isLoading) return
    let alive = true
    let timer: ReturnType<typeof setTimeout> | null = null

    const refresh = async () => {
      try {
        const channelID = details.data?.id || (await getChannel(channelLogin)).id
        const result = await ensureChannelEmotes(channelLogin, channelID, emoteLoadRequest.providers)
        if (!alive) return
        setEmoteStatus({ state: result.state, count: result.count, pending: result.pending, total: result.total, percent: result.percent, providers: result.providers, benchmark: result.benchmark })
        if (result.state === 'processing' || result.state === 'partial') {
          timer = setTimeout(refresh, 5000)
        }
      } catch (e) {
        if (alive) {
          setEmoteStatus({ state: 'failed', count: 0, pending: 0, error: (e as Error).message || 'emote sync failed' })
        }
      }
    }

    setEmoteStatus({
      state: 'loading',
      count: 0,
      pending: 0,
      providers: emoteLoadRequest.providers.map(provider => ({ provider, state: 'processing', count: 0, pending: 0 })),
    })
    refresh()

    return () => {
      alive = false
      if (timer) clearTimeout(timer)
    }
  }, [channelLogin, details.data?.id, details.isLoading, emoteLoadRequest?.token])

  const toggleEmoteProvider = (provider: EmoteProvider) => {
    toggleSavedEmoteProvider(provider)
  }

  const loadSelectedEmotes = () => {
    setEmoteLoadRequest({ providers: [...selectedEmoteProviders], token: Date.now() })
  }

  const setAutoLoadPreference = (value: boolean) => {
    updateSettings({ emoteAutoLoad: value })
    if (value && channelLogin) {
      setEmoteLoadRequest({ providers: [...selectedEmoteProviders], token: Date.now() })
    }
  }

  const setPreferredQuality = (value: string) => {
    if (value === settings.preferredQuality) return
    updateSettings({ preferredQuality: value })
    setError(null)
    setVodRelayError(null)
    retryStream()
  }

  const setPlaybackLatencyMode = (value: PlaybackLatencyMode) => {
    if (value === settings.playbackLatencyMode) return
    updateSettings({ playbackLatencyMode: value })
    if (!isVodPlayback) retryStream()
  }

  const setVideoFit = (value: VideoFitMode) => {
    updateSettings({ videoFit: value })
  }

  const setBottomDensity = (value: BottomDensityMode) => {
    updateSettings({ bottomDensity: value })
  }

  const togglePlay = () => {
    if (showTwitchEmbed) return
    const video = videoRef.current
    if (!video) return
    if (video.paused) {
      video.play().catch(() => undefined)
    } else {
      video.pause()
    }
  }

  const setPlayerVolume = (value: number) => {
    const next = Math.max(0, Math.min(1, value))
    updateSettings({ playerVolume: next })
    const video = videoRef.current
    if (video) video.volume = next
    if (showTwitchEmbed && next === 0) twitchEmbedRef.current?.setMuted(true)
  }

  const toggleFullscreen = async () => {
    const target = showTwitchEmbed ? playerFrameRef.current : videoRef.current
    if (!target) return
    try {
      if (document.fullscreenElement) {
        await document.exitFullscreen()
      } else {
        await target.requestFullscreen()
      }
    } catch {
      return
    }
  }

  const retry = retryStream

  const handleBackToAnalytics = useCallback(() => {
    if (vodAnalyticsHref) window.location.assign(vodAnalyticsHref)
  }, [vodAnalyticsHref])

  const handleVodStreamLinked = useCallback((streamId: string) => {
    setSearchParams(prev => {
      const params = new URLSearchParams(prev)
      params.set('from', 'analytics')
      params.set('sid', streamId)
      return params
    }, { replace: true })
  }, [setSearchParams])

  const [vodResyncPending, setVodResyncPending] = useState(false)

  const handleVodResync = useCallback(async () => {
    if (!channelLogin || !vodAnalyticsStreamId) return
    setVodResyncPending(true)
    try {
      await startHistoricalSync(vodAnalyticsStreamId, channelLogin, {
        vodId: vodPlaybackId,
        forceChat: true,
      })
      for (let attempt = 0; attempt < 180; attempt += 1) {
        await new Promise(resolve => window.setTimeout(resolve, 2000))
        const status = await getSyncStatus(vodAnalyticsStreamId).catch(() => null)
        if (!status) break
        if (status.phase === 'completed' || status.phase === 'failed') break
      }
      await queryClient.invalidateQueries({ queryKey: ['vod-chat-replay', vodAnalyticsStreamId] })
      await queryClient.invalidateQueries({ queryKey: ['analytics-stream', vodAnalyticsStreamId] })
    } catch {
      // Keep the panel open; user can retry or open Analytics.
    } finally {
      setVodResyncPending(false)
    }
  }, [channelLogin, queryClient, vodAnalyticsStreamId, vodPlaybackId])

  const [embedMetrics, setEmbedMetrics] = useState<{ current: number | null; duration: number | null }>({
    current: null,
    duration: null,
  })

  useEffect(() => {
    if (!showTwitchEmbed) {
      setEmbedMountReady(false)
      return
    }
    const frame = requestAnimationFrame(() => setEmbedMountReady(true))
    return () => cancelAnimationFrame(frame)
  }, [showTwitchEmbed])

  useEffect(() => {
    if (!showTwitchEmbed || relayState !== 'playing') {
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
  }, [relayState, showTwitchEmbed])

  const vodAnalyticsDurationSec = useMemo(() => {
    if (!activityRollups?.length) return null
    return activityRollups.length * 60
  }, [activityRollups])
  const vodPulseTotalDurationSec = useMemo(() => resolveVodTotalDurationSec({
    rollupDurationSec: vodAnalyticsDurationSec,
    embedDurationSec: embedMetrics.duration,
    vodDetailDurationSec,
    relaySeekableEndSec: null,
  }), [
    embedMetrics.duration,
    vodAnalyticsDurationSec,
    vodDetailDurationSec,
  ])
  const vodPlayheadOffset = usePlayheadStore(s => (
    isVodPlayback && s.streamId === vodPlayheadStreamId ? s.offsetSeconds : null
  ))
  const sidebarCurrentOffsetSec = vodPlayheadOffset ?? vodOffsetSeconds

  const [vodVideoIsPlaying, setVodVideoIsPlaying] = useState(false)

  useEffect(() => {
    setVodVideoIsPlaying(false)
  }, [vodPlaybackId])

  useEffect(() => {
    const video = videoRef.current
    if (!video || !isVodPlayback || showTwitchEmbed || !hlsUrl) {
      setVodVideoIsPlaying(false)
      return
    }
    const sync = () => setVodVideoIsPlaying(!video.paused && !video.ended)
    video.addEventListener('play', sync)
    video.addEventListener('pause', sync)
    video.addEventListener('playing', sync)
    video.addEventListener('ended', sync)
    sync()
    return () => {
      video.removeEventListener('play', sync)
      video.removeEventListener('pause', sync)
      video.removeEventListener('playing', sync)
      video.removeEventListener('ended', sync)
    }
  }, [hlsUrl, isVodPlayback, showTwitchEmbed])

  const headerPlaybackState = coarsePlaybackState

  useEffect(() => {
    const video = videoRef.current
    if (!video) return
    video.volume = settings.playerVolume
  }, [settings.playerVolume, hlsUrl])

  useEffect(() => {
    const onFullscreenChange = () => {
      const active = document.fullscreenElement
      setIsFullscreen(active === videoRef.current || active === playerFrameRef.current)
    }
    document.addEventListener('fullscreenchange', onFullscreenChange)
    return () => document.removeEventListener('fullscreenchange', onFullscreenChange)
  }, [])

  useEffect(() => {
    autoRetryAttemptsRef.current = 0
  }, [channelLogin])

  useEffect(() => {
    if (coarsePlaybackState === 'playing') {
      autoRetryAttemptsRef.current = 0
    }
  }, [coarsePlaybackState])

  useEffect(() => {
    if (isVodPlayback) return
    if (!hlsUrl || error || coarsePlaybackState !== 'error') return
    const actions = playbackActionsRef.current
    if (!actions || actions.getError()) return
    const stage = actions.getHlsStage().toLowerCase()
    const retryableStage = ['levelloaderror', 'levelloadtimeout', 'manifestloaderror', 'manifestloadtimeout', 'fragloaderror', 'fragloadtimeout']
      .some(token => stage.includes(token))
    if (!retryableStage || autoRetryAttemptsRef.current >= 2) return
    const timer = window.setTimeout(() => {
      autoRetryAttemptsRef.current += 1
      retry()
    }, 1500)
    return () => window.clearTimeout(timer)
  }, [error, hlsUrl, isVodPlayback, coarsePlaybackState, retry])

  const sortedLoadedEmotes = useMemo(
    () => sortChannelEmotesByUsage(emotePreview.data ?? [], liveAnalytics.data?.topEmotes),
    [emotePreview.data, liveAnalytics.data?.topEmotes],
  )
  const headerTitle = useMemo(() => details.data?.displayName || channelLogin || 'Channel', [channelLogin, details.data?.displayName])
  const railViewerOverrides = useMemo(() => {
    if (!channelLogin || details.data?.viewers == null) return undefined
    return { [channelLogin]: details.data.viewers }
  }, [channelLogin, details.data?.viewers])
  const streamPoster = details.data?.thumbnailUrl?.replace('{width}', '960').replace('{height}', '540')
  const activeRenditions = streamSession?.renditions ?? diagnostics.data?.renditions
  const activeSelectedRendition = streamSession?.selectedRendition ?? diagnostics.data?.selectedRendition
  const requestedQuality = resolveRequestedQuality(activeRenditions, settings.preferredQuality, activeSelectedRendition)
  const loadedQuality = selectedRenditionText(streamSession, diagnostics.data)
  const isDenseBottom = settings.bottomDensity === 'dense'
  const theaterPlayerHeightClass = 'h-[clamp(320px,64vh,74vh)]'
  const playerViewportClass = isTheater
    ? `relative overflow-hidden w-full shrink-0 bg-black transition-[height] duration-200 ${theaterPlayerHeightClass}`
    : 'relative overflow-hidden w-full shrink-0 bg-black aspect-video'
  const videoObjectFitClass = settings.videoFit === 'fill' ? 'object-cover object-center' : 'object-contain object-center'
  const lastLiveAgo = details.data?.startedAt
    ? relativeTime(details.data.startedAt)
    : details.data?.updatedAt
      ? relativeTime(details.data.updatedAt / 1000)
      : ''
  const relayBreakdown = streamSession?.startupBreakdown ?? diagnostics.data?.startupBreakdown
  const emotePanel = (
    <EmoteProviderPanel
      selected={selectedEmoteProviders}
      status={emoteStatus}
      autoLoad={autoLoadEmotes}
      disabled={!channelLogin || details.isLoading}
      loadedEmotes={sortedLoadedEmotes}
      emotesLoading={emotePreview.isLoading || emotePreview.isFetching}
      channelLabel={details.data?.displayName || channelLogin}
      channelCategory={details.data?.category}
      topEmotes={liveAnalytics.data?.topEmotes}
      onToggle={toggleEmoteProvider}
      onLoad={loadSelectedEmotes}
      onAutoLoad={setAutoLoadPreference}
    />
  )

  return (
    <main className="flex h-dvh overflow-hidden bg-[#050507] text-zinc-100">
      <div className="relative flex h-dvh min-h-0 flex-1 overflow-hidden bg-[linear-gradient(135deg,rgba(139,92,246,.14),rgba(5,5,7,0)_32%),linear-gradient(180deg,#07070a,#050507)]">
        <ChannelRail
          collapsed={railCollapsed}
          mobileOpen={mobileRailOpen}
          onToggleCollapsed={() => setRailCollapsed(v => !v)}
          onCloseMobile={() => setMobileRailOpen(false)}
          viewerOverrides={railViewerOverrides}
        />
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          <header className="relative z-30 flex min-h-16 flex-wrap items-center gap-2 border-b border-white/10 bg-black/45 px-3 py-2 backdrop-blur-xl lg:gap-3 lg:px-5 lg:py-3">
            <div className="flex min-w-0 items-center gap-2">
              <button onClick={() => setMobileRailOpen(true)} className="rounded border border-white/10 bg-white/[0.06] px-3 py-2 text-sm font-black text-white lg:hidden">
                Menu
              </button>
              <Link to="/" className="flex shrink-0 items-center gap-3 rounded px-2 py-1 transition hover:bg-white/10">
                <BrandLogo size="sm" showText />
                <span className="hidden rounded bg-white/10 px-2 py-0.5 text-xs font-bold text-zinc-300 sm:inline">Browse</span>
              </Link>
              <div className="hidden min-w-0 max-w-[12rem] border-l border-white/10 pl-3 lg:block 2xl:max-w-xs">
                <div className="truncate text-sm font-black text-white">{headerTitle}</div>
                <div className="truncate text-xs font-semibold text-zinc-500">{details.data?.streamTitle || details.data?.category || 'Channel workspace'}</div>
              </div>
            </div>
            <div className="order-last w-full min-w-0 md:order-none md:flex-1 md:px-2 lg:max-w-xl xl:max-w-2xl">
              <ChannelSearchInput />
            </div>
            <div className="ml-auto flex shrink-0 items-center gap-2">
              <div className="hidden items-center gap-2 text-xs font-bold uppercase sm:flex">
                <span className={`rounded px-2 py-1 ${headerPlaybackState === 'playing' ? 'bg-emerald-400/15 text-emerald-200' : headerPlaybackState === 'error' ? 'bg-red-400/15 text-red-200' : 'bg-amber-400/15 text-amber-100'}`}>
                  {headerPlaybackState}
                </span>
                <span className="rounded bg-violet-400/15 px-2 py-1 text-violet-100">{connectionState}</span>
              </div>
              <SettingsButton />
              <AuthButton compact />
            </div>
          </header>

          <div className="border-b border-white/10 bg-[#08080c]/95 px-3 py-2 lg:hidden">
            <div className="grid grid-cols-3 gap-1 rounded border border-white/10 bg-white/[0.04] p-1 text-xs font-black uppercase">
              {mobileChannelPanes.map(pane => (
                <button
                  key={pane.id}
                  type="button"
                  onClick={() => setMobilePane(pane.id)}
                  className={`rounded px-3 py-2 transition ${
                    mobilePane === pane.id
                      ? 'bg-white text-zinc-950'
                      : 'text-zinc-500 hover:bg-white/10 hover:text-zinc-200'
                  }`}
                >
                  {pane.label}
                </button>
              ))}
            </div>
          </div>

          <div className="flex min-h-0 flex-1 flex-col overflow-hidden lg:flex-row">
            <div
              className={`${mobilePane === 'chat' ? 'hidden' : 'flex'} scrollbar-hidden min-h-0 min-w-0 flex-1 flex-col overflow-y-auto overscroll-y-contain bg-[#0e0e10] lg:flex`}
            >
                <div className={`${mobilePane === 'watch' ? 'block' : 'hidden'} shrink-0 lg:block`}>
                  <ChannelStartupClock onTick={setStartupNow} />
                  {isVodPlayback && vodPlayheadStreamId ? (
                    <ChannelVodPlayheadPublisher
                      enabled
                      streamId={vodPlayheadStreamId}
                      vodId={vodPlaybackId}
                      vodRelayBaseSeconds={vodRelayBaseSeconds}
                      showTwitchEmbed={showTwitchEmbed}
                      relayState={relayState}
                      vodOffsetSeconds={vodOffsetSeconds}
                      twitchGetTime={() => twitchEmbedRef.current?.getCurrentTime() ?? undefined}
                    />
                  ) : null}
                  <div className={playerViewportClass}>
                    <ChannelPlayerSurface
                      videoRef={videoRef}
                      playbackOptions={playbackOptions}
                      showTwitchEmbed={showTwitchEmbed}
                      relayState={relayState}
                      hlsUrl={hlsUrl}
                      seekVodAbsoluteOffset={seekVodAbsoluteOffset}
                      onCoarsePlaybackStateChange={setCoarsePlaybackState}
                      onFirstFrameMs={handleFirstFrameMs}
                    >
                    <div ref={playerFrameRef} className="group absolute inset-0 overflow-hidden bg-black">
                    <video key={isVodPlayback ? vodPlaybackId : channelLogin} ref={videoRef} className={`absolute inset-0 h-full w-full bg-black ${showTwitchEmbed ? 'hidden' : ''} ${videoObjectFitClass}`} autoPlay muted={muted} playsInline poster={streamPoster || undefined} />

                    {showTwitchEmbed && !embedMountReady ? (
                      <div className="absolute inset-0 z-20 grid place-items-center bg-black">
                        <div className="flex items-center gap-3 rounded-full border border-white/10 bg-black/70 px-5 py-2.5">
                          <div className="h-4 w-4 animate-spin rounded-full border-2 border-violet-300/30 border-t-violet-300" />
                          <span className="text-sm font-black text-white">Loading Twitch player…</span>
                        </div>
                      </div>
                    ) : null}
                    {showTwitchEmbed && embedMountReady ? (
                      <TwitchVodEmbed
                        vodId={vodPlaybackId}
                        offsetSeconds={vodOffsetSeconds}
                        muted={muted}
                        playerRef={twitchEmbedRef}
                        onReady={() => setRelayState('playing')}
                        onError={(message) => {
                          if (vodEmbedPrimary && vodUseTwitchEmbed && !vodEmbedFallbackMode) {
                            setVodUseTwitchEmbed(false)
                            setVodRelayError(null)
                            setRelayState('starting')
                            return
                          }
                          setRelayState('error')
                          setVodRelayError({ message, code: 'upstream_token_failed', retryable: true })
                        }}
                      />
                    ) : null}

                    <ChannelPlayerChrome>
                    {(pb) => {
                      const playbackState = showTwitchEmbed ? relayState : (hlsUrl ? pb.state : relayState)
                      const playbackError = error || pb.error
                      const vodBannerCurrentSec = showTwitchEmbed
                        ? (embedMetrics.current ?? sidebarCurrentOffsetSec)
                        : (vodPlayheadOffset ?? (pb.metrics.currentTimeSec == null || !Number.isFinite(pb.metrics.currentTimeSec)
                          ? vodOffsetSeconds
                          : vodRelayBaseSeconds + Math.max(0, pb.metrics.currentTimeSec)))
                      const vodBannerTotalSec = resolveVodTotalDurationSec({
                        rollupDurationSec: vodAnalyticsDurationSec,
                        embedDurationSec: embedMetrics.duration,
                        vodDetailDurationSec,
                        relaySeekableEndSec: showTwitchEmbed ? null : pb.metrics.seekableEndSec,
                      })
                      const hasVodStructuredError = isVodPlayback && vodRelayError !== null && !showTwitchEmbed
                      const showStructuredVodError = hasVodStructuredError && (playbackState === 'error' || playbackState === 'retrying')
                      const vodHasFirstFrame = pb.metrics.firstFrameMs !== null
                      const showStartupOverlay = !isChannelOffline
                        && !showTwitchEmbed
                        && !vodSeekPending
                        && playbackState !== 'playing'
                        && playbackState !== 'buffering'
                        && (playbackState === 'error' || playbackState === 'retrying' || pb.metrics.firstFrameMs === null)
                        && !(isVodPlayback && vodHasFirstFrame && playbackState === 'retrying')
                        && !(playbackError && details.data && !details.data.isLive && !isVodPlayback && !showStructuredVodError)
                      const playerControlsVisible = playbackState === 'playing' || playbackState === 'buffering' || detailsExpanded
                      const overlayState = startupOverlayState({
                        playbackError,
                        relayState,
                        hlsUrl,
                        hlsStage: pb.metrics.hlsStage,
                      })
                      const startupElapsedMs = pb.metrics.firstFrameMs ?? Math.max(0, startupNow - startupStartedAt)
                      return (
                    <>
                    {/* VOD review-mode banner (Req 1.1, 20.x) */}
                    {isVodPlayback ? (
                      <div className="pointer-events-none absolute inset-x-0 top-0 z-30 flex justify-center p-3">
                        <VodModeControls
                          vodId={vodPlaybackId}
                          offsetSeconds={vodOffsetSeconds}
                          channelLogin={channelLogin}
                          currentTimeSec={vodBannerCurrentSec}
                          totalDurationSec={vodBannerTotalSec}
                          seekPending={vodSeekPending}
                          analyticsHref={vodAnalyticsHref}
                          chatLogHref={
                            vodAnalyticsStreamId
                              ? `/logs/${encodeURIComponent(channelLogin)}/${encodeURIComponent(vodAnalyticsStreamId)}`
                              : null
                          }
                        />
                      </div>
                    ) : null}

                    {/* Stream thumbnail poster until first frame */}
                    {!isChannelOffline && pb.metrics.firstFrameMs === null && streamPoster && !(playbackError && details.data && !details.data.isLive && !isVodPlayback) ? (
                      <img
                        src={streamPoster}
                        alt=""
                        className="pointer-events-none absolute inset-0 z-[1] h-full w-full object-contain object-center"
                        aria-hidden
                      />
                    ) : null}

                    {/* Offline background */}
                    {(isChannelOffline || (playbackError && details.data && !details.data.isLive && !isVodPlayback)) ? (
                      <div className="absolute inset-0 z-10 flex flex-col items-center justify-center bg-black">
                        {details.data?.profileImage ? (
                          <img
                            src={details.data.profileImage}
                            alt=""
                            className="absolute inset-0 h-full w-full object-cover opacity-20 blur-3xl scale-110"
                          />
                        ) : null}
                        <div className="relative z-10 flex flex-col items-center gap-4">
                          {details.data?.profileImage ? (
                            <img
                              src={details.data.profileImage}
                              alt={details.data?.displayName || channelLogin}
                              className="h-20 w-20 rounded-full border-2 border-white/10 object-cover shadow-2xl shadow-black/60"
                            />
                          ) : (
                            <div className="grid h-20 w-20 place-items-center rounded-full border-2 border-white/10 bg-zinc-800 text-2xl font-black text-violet-200 shadow-2xl shadow-black/60">
                              {(details.data?.displayName || channelLogin).slice(0, 1).toUpperCase()}
                            </div>
                          )}
                          <div className="text-center">
                            <span className="rounded bg-zinc-700/80 px-3 py-1 text-sm font-black uppercase tracking-wide text-white shadow-lg">Offline</span>
                            <div className="mt-3 text-lg font-black text-white">{details.data?.displayName || channelLogin}</div>
                            {details.data?.category ? <div className="mt-1 text-sm font-semibold text-zinc-400">Last seen playing {details.data.category}</div> : null}
                            {lastLiveAgo ? <div className="mt-1 text-sm font-semibold text-zinc-500">Last live {lastLiveAgo}</div> : null}
                          </div>
                        </div>
                      </div>
                    ) : null}

                    {/* Mid-stream buffering overlay — frosted glass over last frame */}
                    {playbackState === 'buffering' && pb.metrics.firstFrameMs !== null ? (
                      <div className="absolute inset-0 z-10 grid place-items-center bg-black/30 backdrop-blur-md transition-opacity duration-300">
                        <div className="flex items-center gap-3 rounded-full border border-white/10 bg-black/60 px-5 py-2.5 shadow-2xl shadow-black/50">
                          <div className="h-4 w-4 animate-spin rounded-full border-2 border-violet-300/30 border-t-violet-300" />
                          <span className="text-sm font-black text-white">Buffering</span>
                        </div>
                      </div>
                    ) : null}

                    {/* Startup overlay — subtler top-left bar instead of large centered modal */}
                    {showStartupOverlay ? (
                      <div className={`absolute inset-0 ${showStructuredVodError ? 'z-40' : 'z-10'}`}>
                        {/* Blurred profile background during startup */}
                        {details.data?.profileImage ? (
                          <img
                            src={details.data.profileImage}
                            alt=""
                            className="absolute inset-0 h-full w-full object-cover opacity-15 blur-3xl scale-110"
                          />
                        ) : null}
                        <div className="absolute inset-0 bg-black/60" />

                        {/* Compact startup status — top left so it does not cover player controls */}
                        <div className={`absolute left-4 z-20 max-w-md ${showStructuredVodError ? 'bottom-24 sm:bottom-6' : 'top-4'}`}>
                          {showStructuredVodError && vodRelayError ? (
                            <VodErrorState
                              error={vodRelayError}
                              channelLogin={channelLogin}
                              vodId={vodPlaybackId}
                              fromAnalytics={vodFromAnalytics}
                              analyticsHref={vodAnalyticsHref}
                              analyticsStreamId={vodAnalyticsStreamId || null}
                              onRetry={retry}
                              onBackToAnalytics={vodAnalyticsHref ? handleBackToAnalytics : undefined}
                              onResync={vodAnalyticsStreamId ? handleVodResync : undefined}
                            />
                          ) : (
                          <div className="rounded-xl border border-white/10 bg-zinc-950/90 px-4 py-3 shadow-2xl shadow-black/60 backdrop-blur-xl">
                            <div className="flex items-center gap-3">
                              {!playbackError ? (
                                <div className="h-5 w-5 shrink-0 animate-spin rounded-full border-2 border-violet-300/30 border-t-violet-300" />
                              ) : (
                                <div className="h-5 w-5 shrink-0 rounded-full bg-red-400/80" />
                              )}
                              <div className="min-w-0">
                                <div className="text-sm font-black text-white">{overlayState.title}</div>
                                <div className="mt-0.5 truncate text-xs font-semibold text-zinc-400">{overlayState.detail}</div>
                              </div>
                            </div>
                            <div className="mt-2 flex flex-wrap items-center gap-1.5 text-[10px] font-black uppercase text-zinc-500">
                              <span className="rounded bg-white/[0.06] px-1.5 py-0.5">{overlayState.stage}</span>
                              <span className="rounded bg-white/[0.06] px-1.5 py-0.5">{fmtMs(startupElapsedMs)}</span>
                              {streamSession?.startupMs || diagnostics.data?.startupMs ? (
                                <span className="rounded bg-white/[0.06] px-1.5 py-0.5">Relay {fmtMs(streamSession?.startupMs ?? diagnostics.data?.startupMs)}</span>
                              ) : null}
                              {relayBreakdown ? (
                                <>
                                  <span className="rounded bg-white/[0.06] px-1.5 py-0.5">Up {fmtMs(relayBreakdown.upstreamFetchMs)}</span>
                                  <span className="rounded bg-white/[0.06] px-1.5 py-0.5">Spawn {fmtMs(relayBreakdown.workerSpawnMs)}</span>
                                  <span className="rounded bg-white/[0.06] px-1.5 py-0.5">HLS {fmtMs(relayBreakdown.hlsReadyMs)}</span>
                                </>
                              ) : null}
                              {pb.metrics.firstFrameMs !== null ? (
                                <span className="rounded bg-emerald-400/10 px-1.5 py-0.5 text-emerald-200">Frame {fmtMs(pb.metrics.firstFrameMs)}</span>
                              ) : null}
                            </div>
                            {playbackError ? (
                              <div className="mt-3 flex flex-wrap gap-2">
                                <button onClick={retry} className="rounded bg-violet-500 px-3 py-1.5 text-xs font-black text-white transition hover:bg-violet-400">
                                  Retry
                                </button>
                                <button
                                  type="button"
                                  onClick={() => {
                                    setActiveTab('diagnostics')
                                    setMobilePane('workspace')
                                  }}
                                  className="rounded border border-white/15 bg-white/[0.06] px-3 py-1.5 text-xs font-black text-zinc-200 transition hover:bg-white/10"
                                >
                                  Diagnostics
                                </button>
                              </div>
                            ) : null}
                          </div>
                          )}
                          {startupBenchmarks.length > 1 ? (
                            <div className="mt-2 rounded-xl border border-white/10 bg-zinc-950/80 px-4 py-2 shadow-xl backdrop-blur-xl">
                              <div className="mb-1 text-[10px] font-black uppercase tracking-wider text-zinc-600">Recent starts</div>
                              <div className="space-y-1">
                                {startupBenchmarks.slice(0, 3).map(entry => (
                                  <div key={entry.sessionId} className="flex flex-wrap gap-1.5 text-[10px] font-black uppercase text-zinc-400">
                                    <span className="rounded bg-white/[0.05] px-1.5 py-0.5">#{entry.attempt}</span>
                                    <span className="rounded bg-white/[0.05] px-1.5 py-0.5">Relay {fmtMs(entry.relayStartupMs)}</span>
                                    <span className="rounded bg-white/[0.05] px-1.5 py-0.5">Frame {fmtMs(entry.firstFrameMs)}</span>
                                    {entry.fallbackUsed ? <span className="rounded bg-cyan-400/10 px-1.5 py-0.5 text-cyan-200">Fallback</span> : null}
                                  </div>
                                ))}
                              </div>
                            </div>
                          ) : null}
                        </div>
                      </div>
                    ) : null}

                    {/* HLS relay status chip — compact bottom-left when playing */}
                    {playbackState === 'playing' && showTwitchEmbed ? (
                      <div className="absolute bottom-3 left-3 z-20 flex items-center gap-2 rounded-full border border-white/10 bg-black/70 px-3 py-1.5 text-[11px] font-black uppercase text-zinc-300 shadow-lg shadow-black/40 backdrop-blur-sm">
                        <span className="h-2 w-2 rounded-full bg-purple-400 shadow-sm shadow-purple-400/50" />
                        {isEmbedAnalyticsReview && !vodEmbedFallbackMode ? (
                          <span>Reviewing on Twitch · scrub in Pulse</span>
                        ) : (
                          <>
                            <span>{vodEmbedFallbackMode ? 'Twitch embed fallback' : 'Twitch embed'}</span>
                            {vodFromAnalytics && vodAnalyticsStreamId ? <span className="text-zinc-500">+ activity graph</span> : null}
                          </>
                        )}
                      </div>
                    ) : null}
                    {playbackState === 'playing' && hlsUrl ? (
                      <div className="pointer-events-none absolute left-3 top-3 z-30 flex items-center gap-2 rounded-full border border-white/10 bg-black/70 px-3 py-1.5 text-[11px] font-black uppercase text-zinc-300 opacity-90 shadow-lg shadow-black/40 backdrop-blur-sm">
                        <span className="h-2 w-2 rounded-full bg-emerald-400 shadow-sm shadow-emerald-400/50" />
                        <span>HLS relay</span>
                        {(() => {
                          const overlayDelay = computeEndToEndLiveDelaySec(pb.metrics, diagnostics.data).displayDelaySec
                          return overlayDelay !== null ? <span className="text-zinc-500">+{fmtMetricSec(overlayDelay)}</span> : null
                        })()}
                      </div>
                    ) : null}
                    {!showStructuredVodError ? (
                    isEmbedAnalyticsReview ? (
                      playbackState === 'playing' && (vodBannerCurrentSec != null || vodBannerTotalSec != null) ? (
                        <div className="pointer-events-none absolute bottom-3 right-3 z-20 rounded-full border border-white/10 bg-black/70 px-3 py-1.5 font-mono text-[11px] font-bold tabular-nums text-zinc-300 shadow-lg shadow-black/40 backdrop-blur-sm">
                          {formatVodTimestamp(vodBannerCurrentSec)} / {formatVodTimestamp(vodBannerTotalSec)}
                        </div>
                      ) : null
                    ) : (
                    <div className={`absolute inset-x-0 bottom-0 z-50 transition-opacity duration-200 ${playerControlsVisible ? 'pointer-events-auto opacity-100' : 'pointer-events-none opacity-0 group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100'} ${showTwitchEmbed ? 'pb-2' : ''}`}>
                      {showAnalyticsActivityWaveform && hasActivityRollups && !isEmbedAnalyticsReview ? (
                        <div className="pointer-events-none px-3 pb-1 lg:px-5 group-hover:pointer-events-auto focus-within:pointer-events-auto">
                          <PlayerHeatmap
                            rollups={activityRollups}
                            totalDurationSec={vodBannerTotalSec ?? ((activityRollups?.length ?? 0) * 60)}
                            isLoading={vodRollupsQuery.isLoading}
                            isError={vodRollupsQuery.isError}
                            currentOffsetSec={vodBannerCurrentSec ?? vodOffsetSeconds}
                            highlightOffsetSec={vodOffsetSeconds}
                            onSeek={seekVodAbsoluteOffset}
                          />
                        </div>
                      ) : isVodPlayback && vodFromAnalytics && !hasActivityRollups && !vodRollupsQuery.isLoading && !isEmbedAnalyticsReview ? (
                        <div className="px-3 pb-1 lg:px-5">
                          <VodActivityGraphPanel
                            channelLogin={channelLogin}
                            vodId={vodPlaybackId}
                            streamId={vodAnalyticsStreamId || null}
                            streams={analyticsStreamsQuery.data?.items ?? []}
                            onStreamLinked={handleVodStreamLinked}
                            onSyncComplete={() => {
                              void queryClient.invalidateQueries({ queryKey: ['vod-activity-rollups'] })
                              void queryClient.invalidateQueries({ queryKey: ['vod-chat-replay'] })
                            }}
                          />
                        </div>
                      ) : null}
                      {isVodPlayback && vodBannerTotalSec ? (
                        <VodSeekBar
                          currentSec={vodBannerCurrentSec ?? 0}
                          totalSec={vodBannerTotalSec}
                          onSeek={seekVodAbsoluteOffset}
                        />
                      ) : null}
                      <LivePlayerControls
                        playbackState={playbackState}
                        metrics={pb.metrics}
                        diagnostics={diagnostics.data}
                        isVod={isVodPlayback}
                        videoIsPlaying={isVodPlayback && !showTwitchEmbed ? vodVideoIsPlaying : undefined}
                        requestedQuality={requestedQuality}
                        loadedQuality={loadedQuality}
                        renditions={activeRenditions}
                        latencyMode={pb.effectiveLatencyMode}
                        latencyModeAuto={pb.effectiveLatencyMode !== settings.playbackLatencyMode}
                        videoFit={settings.videoFit}
                        bottomDensity={settings.bottomDensity}
                        muted={muted}
                        volume={settings.playerVolume}
                        isFullscreen={isFullscreen}
                        isTheater={isTheater}
                        backend={streamSession?.workerBackend ?? diagnostics.data?.workerBackend}
                        startupMs={streamSession?.startupMs ?? diagnostics.data?.startupMs}
                        fallbackAttempted={streamSession?.fallbackAttempted || Boolean(diagnostics.data?.fallbackAttempts)}
                        detailsExpanded={detailsExpanded}
                        onTogglePlay={togglePlay}
                        onMuted={(nextMuted) => {
                          setMuted(nextMuted)
                          if (showTwitchEmbed) twitchEmbedRef.current?.setMuted(nextMuted)
                        }}
                        onVolume={setPlayerVolume}
                        onToggleFullscreen={() => void toggleFullscreen()}
                        onToggleTheater={() => setIsTheater(value => !value)}
                        onJumpLive={pb.jumpLive}
                        onQuality={setPreferredQuality}
                        onLatencyMode={setPlaybackLatencyMode}
                        onVideoFit={setVideoFit}
                        onBottomDensity={setBottomDensity}
                        onDetailsExpanded={setDetailsExpanded}
                      />
                    </div>
                    )
                    ) : null}
                    </>
                      )
                    }}
                    </ChannelPlayerChrome>
                    </div>
                    </ChannelPlayerSurface>
                  </div>
                </div>
                <div className={`${mobilePane === 'workspace' ? 'block' : 'hidden'} lg:block`}>
                <ChannelMeta
                  login={channelLogin}
                  details={details.data}
                  detailsLoading={details.isLoading}
                  quality={loadedQuality}
                  listeners={listeners}
                  dense={isDenseBottom}
                />
                <ChannelTabs
                  activeTab={activeTab}
                  onTab={setActiveTab}
                  insights={insights.data}
                  channel={channelLogin}
                  details={details.data}
                  diagnostics={diagnostics.data}
                  streamSession={streamSession}
                  emotePanel={emotePanel}
                  statsPeriod={statsPeriod}
                  clipPeriod={clipPeriod}
                  onStatsPeriod={setStatsPeriod}
                  onClipPeriod={setClipPeriod}
                  dense={isDenseBottom}
                  isVod={isVodPlayback}
                  analyticsStreams={analyticsStreamsQuery.data?.items}
                  liveStreamId={liveAnalytics.data?.stream?.streamId ?? null}
                  trackLiveAnalytics={trackLiveAnalytics}
                  trackAnalyticsPending={trackAnalyticsMutation.isPending}
                  onTrackAnalytics={track => trackAnalyticsMutation.mutate(track)}
                />
                </div>
            </div>
            <aside className={`${mobilePane === 'chat' ? 'flex' : 'hidden'} min-h-0 shrink-0 flex-col overflow-hidden border-t border-white/10 bg-[#111117] lg:flex lg:w-[400px] lg:border-l lg:border-t-0`}>
              {showPulseSidebarTab ? (
                <div className="shrink-0 border-b border-white/10 px-3 py-2">
                  <div className="flex rounded border border-white/10 bg-black/25 p-0.5">
                    <button
                      type="button"
                      onClick={() => setChatSidebarTab('chat')}
                      className={`flex-1 rounded px-3 py-1.5 text-[11px] font-black uppercase tracking-wide transition ${
                        chatSidebarTab === 'chat'
                          ? 'bg-violet-600 text-white'
                          : 'text-zinc-400 hover:text-zinc-200'
                      }`}
                    >
                      Chat
                    </button>
                    <button
                      type="button"
                      onClick={() => setChatSidebarTab('pulse')}
                      className={`flex-1 rounded px-3 py-1.5 text-[11px] font-black uppercase tracking-wide transition ${
                        chatSidebarTab === 'pulse'
                          ? 'bg-violet-600 text-white'
                          : 'text-zinc-400 hover:text-zinc-200'
                      }`}
                    >
                      Pulse
                    </button>
                  </div>
                </div>
              ) : null}
              <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
                {isVodPlayback && vodAnalyticsStreamId ? (
                  chatSidebarTab === 'pulse' && (showAnalyticsActivityWaveform || (isVodPlayback && vodFromAnalytics)) ? (
                    <VodStreamPulsePanel
                      channelLogin={channelLogin}
                      streamId={vodAnalyticsStreamId}
                      vodId={vodPlaybackId}
                      detail={vodRollupsQuery.data}
                      rollups={activityRollups}
                      heatmapPoints={vodHeatmapQuery.data?.points}
                      totalDurationSec={vodPulseTotalDurationSec}
                      currentOffsetSec={sidebarCurrentOffsetSec}
                      highlightOffsetSec={vodOffsetSeconds}
                      onSeek={seekVodAbsoluteOffset}
                      onSeekMoment={seekVodAbsoluteOffset}
                      analyticsStreams={analyticsStreamsQuery.data?.items ?? []}
                      onStreamLinked={handleVodStreamLinked}
                      onSyncComplete={() => {
                        void queryClient.invalidateQueries({ queryKey: ['vod-activity-rollups'] })
                        void queryClient.invalidateQueries({ queryKey: ['vod-chat-replay'] })
                      }}
                      isLoading={vodRollupsQuery.isLoading}
                      isError={vodRollupsQuery.isError}
                      className="flex min-h-0 flex-1 flex-col overflow-hidden"
                    />
                  ) : (
                    <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
                      <div className="flex shrink-0 items-center justify-between border-b border-white/10 px-3 py-2.5">
                        <div className="text-[11px] font-semibold text-zinc-500">
                          Synced replay follows playback
                        </div>
                        <Link
                          to={`/logs/${encodeURIComponent(channelLogin)}/${encodeURIComponent(vodAnalyticsStreamId)}`}
                          className="rounded border border-cyan-400/25 bg-cyan-500/10 px-2.5 py-1 text-[10px] font-black uppercase text-cyan-200 transition hover:bg-cyan-500/20"
                        >
                          Chat log
                        </Link>
                      </div>
                      <VodChatReplayPanel
                        streamId={vodAnalyticsStreamId}
                        currentOffsetSeconds={sidebarCurrentOffsetSec}
                        isSyncing={vodResyncPending}
                        onSync={handleVodResync}
                        needsChatReplayResync={hasActivityRollups}
                        className="flex min-h-0 flex-1 flex-col overflow-hidden border-0 bg-transparent"
                      />
                    </div>
                  )
                ) : isVodPlayback ? (
                  <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-center">
                    <p className="text-sm font-semibold text-zinc-300">VOD chat replay</p>
                    <p className="text-xs leading-relaxed text-zinc-500">
                      Sync chat and emotes for this VOD in Analytics first so the URL includes <code className="text-violet-200">sid=</code>.
                      Streams synced before chat replay was enabled need one <strong className="font-bold text-zinc-400">Sync chat</strong> in Analytics or from the Chat tab after you open a synced VOD.
                    </p>
                  </div>
                ) : chatSidebarTab === 'pulse' && showPulseSidebarTab ? (
                  <StreamPulsePanel
                    channelLogin={channelLogin}
                    insights={pulseInsights.data}
                    insightsLoading={pulseInsights.isLoading}
                    insightsFetching={pulseInsights.isFetching}
                    insightsError={pulseInsights.isError}
                    lsfLoadPending={Boolean(pulseInsights.isFetching && isLsfWarming(pulseInsights.data?.sources))}
                    onLoadLsf={handleLoadPulseLsf}
                    liveAnalytics={liveAnalytics.data}
                    liveAnalyticsLoading={liveAnalytics.isLoading}
                    trackLiveAnalytics={trackLiveAnalytics}
                    trackAnalyticsPending={trackAnalyticsMutation.isPending}
                    onTrackAnalytics={track => trackAnalyticsMutation.mutate(track)}
                    autoUpdate={pulseAutoUpdate}
                    onAutoUpdateChange={setPulseAutoUpdate}
                  />
                ) : (
                  <Chat
                    channel={channelLogin}
                    user={auth.user}
                    isAuthenticated={auth.isAuthenticated}
                    emotes={emoteStatus}
                    badgeCatalog={badgeCatalog.data?.badges ?? {}}
                    loadedEmotes={sortedLoadedEmotes}
                  />
                )}
              </div>
            </aside>
          </div>
        </div>
      </div>
    </main>
  )
}
