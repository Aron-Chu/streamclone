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
  getAnalyticsLive,
  getAnalyticsStream,
  getAnalyticsStreams,
  getFollowedChannels,
  getLocalFollowedChannels,
  getStreamDiagnostics,
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
import type { AnalyticsStream, AnalyticsTopEmote, ChannelDetails, ChannelEmote, ChannelInsights, ClipCard, EmoteProvider, SourceStatus, StartResponse, StartupBreakdown, StreamDiagnostics, StatsTimelinePoint, StreamStat, VodStartResponse } from '../api'
import { useAuth } from '../auth'
import { useChatStore } from '../chatStore'
import { normalizeBrowserOriginUrl } from '../config'
import { useHlsPlayback, type PlaybackMetrics, type PlaybackState } from '../playback'
import { useThemeEffect, useUiSettings, type BottomDensityMode, type ClipPeriod, type PlaybackLatencyMode, type StatsPeriod, type VideoFitMode } from '../settings'
import { autoHighStableQuality, defaultQualityOptions, requestQuality } from '../streamQuality'
import { emoteLoadPercent, formatEmoteProviderProgress, sortChannelEmotesByUsage } from '../emoteUtils'
import { normalizeVodId } from '../utils/vodId'
import { buildVodDeepLink, buildVodSeekTarget, parseVodAnalyticsContext } from '../utils/vodDeepLink'
import { buildTwitchVodUrl } from '../utils/twitchVodUrl'
import ActivityWaveform from './analytics/ActivityWaveform'
import VodChatReplayPanel from './analytics/VodChatReplayPanel'
import ChannelVodsPanel from './channel/ChannelVodsPanel'
import TwitchVodEmbed, { type TwitchVodPlayerHandle } from './channel/TwitchVodEmbed'
import { usePlayheadStore } from '../stores/playheadStore'
import { PLAYHEAD_SYNC_INTERVAL_MS } from '../utils/chartCursorSync'
import BrandLogo from './BrandLogo'
import ChannelSearchInput from './ChannelSearchInput'
import Chat, { type ChatEmoteStatus } from './Chat'
import ChannelRail from './ChannelRail'
import LocalTokenImportButton from './LocalTokenImportButton'
import PlaybackDiagnostics from './PlaybackDiagnostics'
import SettingsButton from './SettingsPanel'
import VodModeControls from './channel/VodModeControls'
import VodErrorState from './channel/VodErrorState'
import type { VodErrorInput } from './channel/vodError'
import { HLS_NOT_READY_MAX_AUTO_RETRIES } from './channel/vodError'
import PlayerHeatmap from './channel/PlayerHeatmap'

type ChannelTab = 'about' | 'stats' | 'clips' | 'vods' | 'diagnostics' | 'emotes'
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
  return normalizeBrowserOriginUrl(rawUrl, ['/live/'])
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
      if (provider.state === 'processing') return `${label} ${provider.percent ?? 0}%`
      if (provider.state === 'failed') return `${label} failed`
      return `${label} ready`
    }).join(' · ')
  }
  if (status.state === 'idle') return 'idle'
  if (status.state === 'loading') return 'loading'
  if (status.state === 'processing') return `processing ${status.percent ?? 0}%`
  if (status.state === 'failed') return 'failed'
  return `ready ${status.count}`
}

function providerStatusTone(status: ChatEmoteStatus) {
  if (status.state === 'failed' || status.providers?.some(provider => provider.state === 'failed')) return 'text-red-200 bg-red-500/10 border-red-400/20'
  if (status.state === 'processing' || status.state === 'loading') return 'text-amber-100 bg-amber-400/10 border-amber-300/20'
  if (status.state === 'ready') return 'text-emerald-100 bg-emerald-400/10 border-emerald-400/20'
  return 'text-zinc-300 bg-white/[0.045] border-white/10'
}

function providerCardTone(state: string) {
  if (state === 'failed') return 'border-red-400/25 bg-red-500/10 text-red-100'
  if (state === 'ready') return 'border-emerald-400/25 bg-emerald-400/10 text-emerald-100'
  if (state === 'processing') return 'border-amber-300/25 bg-amber-400/10 text-amber-100'
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
          <div className="text-sm font-black text-white">Chat emotes</div>
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
  isVod = false,
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
  isVod?: boolean
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
  const [qualityOpen, setQualityOpen] = useState(false)
  const qOptions = qualityOptions(renditions)
  const selectedQuality = qOptions.find(option => option.value === requestedQuality) ?? qOptions[0]
  const discoveredCount = renditions?.length ?? 0
  const liveTone = behind !== null && behind > Math.max((metrics.targetLatencySec ?? 0) + 4, 10)
    ? 'border-amber-300/35 bg-amber-400/15 text-amber-100'
    : 'border-red-400/35 bg-red-500/15 text-red-100'
  return (
    <section className="bg-gradient-to-t from-black/90 via-black/70 to-transparent px-3 py-3 lg:px-5">
      <div className="flex flex-nowrap items-center gap-2">
        <button
          type="button"
          onClick={onTogglePlay}
          aria-label="Play or pause"
          className="grid h-9 w-9 shrink-0 place-items-center rounded border border-white/10 bg-white/[0.08] text-xs font-black text-white transition hover:bg-white/15"
        >
          {playbackState === 'playing' ? '❚❚' : '▶'}
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
                <span className={`h-9 rounded border px-3 py-2 text-xs font-black uppercase ${liveTone}`}>
                  LIVE {metrics.behindLiveSec === null ? '' : `+${fmtMetricSec(metrics.behindLiveSec)}`}
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
              <span className="rounded bg-white/[0.045] px-2 py-1">Delay {fmtMetricSec(metrics.behindLiveSec)}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1">Buffered {fmtMetricSec(metrics.bufferSizeSec)}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1">Target {fmtMetricSec(metrics.targetLatencySec)}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1">Stage {formatPlaybackStage(metrics.hlsStage)}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1">Backend {backend || '-'}</span>
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
    <section className={`border-t border-white/10 bg-[#0d0d12] ${dense ? 'px-3 py-3 lg:px-4' : 'px-4 py-4 lg:px-6'}`}>
      <div className={`flex flex-col ${dense ? 'gap-3' : 'gap-4'} 2xl:flex-row 2xl:items-start 2xl:justify-between`}>
        <div className="min-w-0 flex-1 space-y-3">
          <div className="flex gap-2">
            <div className={`animate-pulse rounded bg-white/10 ${dense ? 'h-5 w-14' : 'h-5 w-16'}`} />
            <div className="h-5 w-24 animate-pulse rounded bg-white/10" />
          </div>
          <div className={`animate-pulse rounded bg-white/10 ${dense ? 'h-7 w-4/5' : 'h-8 w-3/4'}`} />
          <div className={`flex items-start ${dense ? 'gap-2' : 'gap-3'}`}>
            <div className={`shrink-0 animate-pulse rounded bg-white/10 ${dense ? 'h-10 w-10' : 'h-12 w-12'}`} />
            <div className="min-w-0 flex-1 space-y-2">
              <div className="h-4 w-32 animate-pulse rounded bg-white/10" />
              <div className="h-3 w-full max-w-md animate-pulse rounded bg-white/10" />
            </div>
          </div>
        </div>
        <div className={`grid grid-cols-2 sm:grid-cols-4 ${dense ? 'gap-1.5 2xl:w-[460px]' : 'gap-2 2xl:w-[520px]'}`}>
          {Array.from({ length: 4 }).map((_, index) => (
            <div key={index} className={`rounded border border-white/10 bg-white/[0.04] ${dense ? 'px-2.5 py-2' : 'px-3 py-2'}`}>
              <div className="h-3 w-12 animate-pulse rounded bg-white/10" />
              <div className={`mt-1.5 animate-pulse rounded bg-white/10 ${dense ? 'h-4 w-16' : 'h-5 w-20'}`} />
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

function TrackAnalyticsToggle({
  tracked,
  pending,
  onToggle,
}: {
  tracked: boolean
  pending: boolean
  onToggle: (next: boolean) => void
}) {
  return (
    <button
      type="button"
      disabled={pending}
      onClick={() => onToggle(!tracked)}
      className={`rounded px-2.5 py-0.5 text-[11px] font-black uppercase tracking-wide transition disabled:opacity-60 ${
        tracked
          ? 'bg-violet-600 text-white hover:bg-violet-500'
          : 'border border-violet-400/30 bg-violet-500/10 text-violet-200 hover:border-violet-300/50'
      }`}
      title={tracked ? 'Minute-level chat and emote rollups are collected for this live stream.' : 'Enable live analytics collection and the activity chart below the player.'}
    >
      {pending ? 'Saving…' : tracked ? 'Tracking analytics' : 'Track analytics'}
    </button>
  )
}

function ChannelMeta({
  login,
  details,
  detailsLoading,
  quality,
  listeners,
  dense,
  trackLiveAnalytics,
  trackAnalyticsPending,
  onTrackAnalytics,
}: {
  login: string
  details?: ChannelDetails
  detailsLoading: boolean
  quality: string
  listeners: number | null
  dense: boolean
  trackLiveAnalytics?: boolean
  trackAnalyticsPending?: boolean
  onTrackAnalytics?: (track: boolean) => void
}) {
  if (detailsLoading && !details) {
    return <ChannelMetaSkeleton dense={dense} />
  }
  const display = details?.displayName || login
  const title = details?.streamTitle || (detailsLoading ? 'Loading stream details' : `${display}'s channel`)
  const avatar = details?.profileImage
  return (
    <section className={`shrink-0 border-b border-white/10 bg-[#0e0e10] ${dense ? 'px-3 py-3 lg:px-4' : 'px-4 py-4 lg:px-5'}`}>
      <div className="mb-2 flex flex-wrap items-center gap-x-2 gap-y-1">
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
      <div className={`mt-3 flex flex-wrap items-center gap-3 ${dense ? 'gap-2' : ''}`}>
        <div className={`grid shrink-0 place-items-center overflow-hidden rounded-full bg-zinc-800 text-sm font-black text-violet-100 ${dense ? 'h-10 w-10' : 'h-12 w-12'}`}>
          {avatar ? <img src={avatar} alt={display} className="h-full w-full object-cover" /> : display.slice(0, 1).toUpperCase()}
        </div>
        <div className="min-w-0 flex-1">
          <div className="text-base font-semibold text-white">{display}</div>
          {!dense && details?.description ? (
            <p className="mt-1 line-clamp-2 text-sm leading-5 text-zinc-400">{details.description}</p>
          ) : null}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <FollowButton login={login} />
          {details?.isLive && onTrackAnalytics ? (
            <TrackAnalyticsToggle
              tracked={Boolean(trackLiveAnalytics)}
              pending={Boolean(trackAnalyticsPending)}
              onToggle={onTrackAnalytics}
            />
          ) : null}
        </div>
      </div>
      {listeners != null && listeners > 0 ? (
        <div className="mt-2 text-xs font-medium text-zinc-500">{fullCount(listeners)} watching on this relay · {quality}</div>
      ) : null}
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
        <div className="mt-1 text-xs font-semibold text-zinc-500">Restarts {diagnostics?.restarts ?? 0} / backend {diagnostics?.workerBackend || '-'}</div>
      </div>
      <div className="rounded border border-white/10 bg-white/[0.035] p-3">
        <div className="text-[11px] font-black uppercase text-zinc-500">Context loaded</div>
        <div className="mt-1 text-sm font-black text-white">{fullCount(insights?.clips?.length ?? 0)} clips</div>
        <div className="mt-1 text-xs font-semibold text-zinc-500">These counts are loaded items for the selected periods, not all-time totals.</div>
      </div>
    </div>
  )
}

function ChannelTabs({
  activeTab,
  onTab,
  insights,
  channel,
  details,
  diagnostics,
  playbackMetrics,
  onJumpLive,
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
}: {
  activeTab: ChannelTab
  onTab: (tab: ChannelTab) => void
  insights?: ChannelInsights
  channel: string
  details?: ChannelDetails
  diagnostics?: StreamDiagnostics
  playbackMetrics: PlaybackMetrics
  onJumpLive: () => void
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
}) {
  const statsSources = (insights?.sources ?? []).filter(source => sourceMatchesGroup(source, 'stats'))
  const clipSources = (insights?.sources ?? []).filter(source => sourceMatchesGroup(source, 'clips'))
  const tabs: Array<{ id: ChannelTab; label: string }> = [
    { id: 'about', label: 'About' },
    { id: 'stats', label: 'Stats' },
    { id: 'clips', label: 'Clips' },
    { id: 'vods', label: 'Videos' },
    { id: 'emotes', label: 'Emotes' },
  ]
  return (
    <section className="shrink-0 bg-[#0e0e10]">
      <div className={`sticky top-0 z-10 border-b border-white/10 bg-[#0e0e10]/95 backdrop-blur-sm ${dense ? 'px-2' : 'px-3 lg:px-4'}`}>
        <div className="flex items-center gap-1 overflow-x-auto">
          {tabs.map(tab => (
            <button
              key={tab.id}
              type="button"
              onClick={() => onTab(tab.id)}
              className={`shrink-0 border-b-2 px-3 py-3 text-sm font-semibold transition ${activeTab === tab.id ? 'border-[#bf94ff] text-white' : 'border-transparent text-zinc-400 hover:border-zinc-600 hover:text-zinc-200'}`}
            >
              {tab.label}
            </button>
          ))}
          <button
            type="button"
            onClick={() => onTab('diagnostics')}
            className={`shrink-0 border-b-2 px-3 py-3 text-sm font-semibold transition ${
              activeTab === 'diagnostics'
                ? 'border-amber-300 text-amber-100'
                : 'border-transparent text-zinc-500 hover:border-zinc-600 hover:text-zinc-300'
            }`}
          >
            Advanced
          </button>
        </div>
      </div>

      <div className={dense ? 'px-3 py-3 lg:px-4' : 'px-4 py-4 lg:px-5'}>
      {activeTab === 'diagnostics' ? (
        <p className={`text-xs font-semibold leading-relaxed text-zinc-500 ${dense ? 'mb-3' : 'mb-4'}`}>
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
            <div className="text-xs font-black uppercase text-zinc-500">Stream History & VODs</div>
            <a
              href={`/analytics/${encodeURIComponent(channel)}`}
              className="rounded bg-violet-600 px-4 py-2 text-xs font-black text-white transition hover:bg-violet-500"
            >
              Open Full Analytics →
            </a>
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
        <PlaybackDiagnostics channel={channel} metrics={playbackMetrics} diagnostics={diagnostics} sessionId={streamSession?.session_id} onJumpLive={onJumpLive} isVod={isVod} />
      ) : null}
      {activeTab === 'emotes' ? emotePanel : null}
      </div>
    </section>
  )
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
        <section>
          <h4 className="text-sm font-semibold text-white">Links</h4>
          <div className="mt-2 flex flex-wrap gap-x-4 gap-y-2">
            {socialLinks.map((link, index) => (
              <a
                key={link.id || link.url || index}
                href={link.url}
                target="_blank"
                rel="noreferrer"
                className="text-sm font-semibold text-[#bf94ff] transition hover:text-[#d8b7ff] hover:underline"
              >
                {link.title || compactUrl(link.url)}
              </a>
            ))}
          </div>
        </section>
      ) : null}

      {panels.length ? (
        <section>
          <h4 className="mb-3 text-sm font-semibold text-white">Panels</h4>
          <div className="flex w-full max-w-[340px] flex-col gap-4">
            {panels.map((panel, index) => {
              const panelTitle = normalizePanelText(panel.title, panel.linkUrl ? compactUrl(panel.linkUrl) : 'Panel')
              const panelBody = panel.description?.trim()
              const body = (
                <div className="overflow-hidden rounded-md bg-[#18181b]">
                  {panel.imageUrl ? (
                    <img src={panel.imageUrl} alt={panelTitle} className="block w-full" loading="lazy" />
                  ) : null}
                  {panelBody ? (
                    <div className={`text-sm leading-6 text-zinc-300 ${panel.imageUrl ? 'px-3 py-3' : 'px-3 py-4'}`}>
                      <div className="whitespace-pre-wrap">{panelBody}</div>
                    </div>
                  ) : null}
                </div>
              )
              if (panel.linkUrl) {
                return (
                  <a key={panel.id || panel.linkUrl || index} href={panel.linkUrl} target="_blank" rel="noreferrer" className="block transition hover:opacity-90">
                    {body}
                  </a>
                )
              }
              return <div key={panel.id || index}>{body}</div>
            })}
          </div>
        </section>
      ) : (
        <section className="space-y-3">
          <EmptyPanel title="No panels yet" detail={aboutIssue ? sourceMessageText(aboutIssue) : 'Custom About panels from Twitch will appear here when metadata is available.'} />
          {aboutIssue ? <SourceDiagnostics sources={aboutSources} /> : null}
        </section>
      )}
    </div>
  )
}

export default function Channel() {
  const { login } = useParams<{ login: string }>()
  const [searchParams] = useSearchParams()
  const queryClient = useQueryClient()
  const channelLogin = login ?? ''
  const rawVodParam = searchParams.get('vod')
  const vodParamPresent = (rawVodParam?.trim().length ?? 0) > 0
  const vodPlaybackId = normalizeVodId(rawVodParam) ?? ''
  const isVodPlayback = vodPlaybackId.length > 0
  const vodIdInvalid = vodParamPresent && !isVodPlayback
  const vodOffsetSeconds = Math.max(0, Number.parseInt(searchParams.get('offset') || '0', 10) || 0)
  const vodAnalyticsContext = parseVodAnalyticsContext(searchParams, channelLogin, isVodPlayback)
  const { fromAnalytics: vodFromAnalytics, streamId: vodAnalyticsStreamId, analyticsHref: vodAnalyticsHref } = vodAnalyticsContext
  const showAnalyticsActivityWaveform = isVodPlayback && vodFromAnalytics && Boolean(vodAnalyticsStreamId)
  const useAnalyticsEmbedFirst = false
  const relaySessionKey = isVodPlayback ? vodSessionKey(vodPlaybackId) : channelLogin
  const twitchEmbedRef = useRef<TwitchVodPlayerHandle | null>(null)
  const [vodEmbedFallback, setVodEmbedFallback] = useState(false)
  const [embedMountReady, setEmbedMountReady] = useState(false)
  const showTwitchEmbed = isVodPlayback && vodEmbedFallback
  const [vodSeekOnStart, setVodSeekOnStart] = useState(0)
  const videoRef = useRef<HTMLVideoElement>(null)
  const playerFrameRef = useRef<HTMLDivElement>(null)
  const sessionIdRef = useRef<string | undefined>()
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
  const [emoteStatus, setEmoteStatus] = useState<ChatEmoteStatus>({ state: 'idle', count: 0, pending: 0 })
  const [emoteLoadRequest, setEmoteLoadRequest] = useState<{ providers: EmoteProvider[]; token: number } | null>(null)
  const [muted, setMuted] = useState(true)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [isTheater, setIsTheater] = useState(false)
  const [isChannelOffline, setIsChannelOffline] = useState(false)
  const [startupStartedAt, setStartupStartedAt] = useState(() => Date.now())
  const [startupNow, setStartupNow] = useState(() => Date.now())
  const [startupBenchmarks, setStartupBenchmarks] = useState<StartupBenchmarkEntry[]>([])
  const autoRetryAttemptsRef = useRef(0)
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
  const retryStream = useCallback(() => {
    setError(null)
    setHlsUrl('')
    setRetryKey(k => k + 1)
  }, [])
  const handleUnauthorizedHls = useCallback(() => {
    if (autoRetryAttemptsRef.current >= 2) {
      setRelayState('error')
      if (isVodPlayback) {
        setVodRelayError({ code: 'hls_proxy_auth', retryable: true })
        setError(null)
      } else {
        setError('Video relay unavailable. Try Retry in a moment or switch channels.')
      }
      setHlsUrl('')
      return
    }
    autoRetryAttemptsRef.current += 1
    retryStream()
  }, [isVodPlayback, retryStream])
  const handleVodRelayStale = useCallback(() => {
    if (autoRetryAttemptsRef.current >= 3) {
      setRelayState('error')
      setVodRelayError({ code: 'hls_not_ready', retryable: true })
      setError(null)
      return
    }
    autoRetryAttemptsRef.current += 1
    retryStream()
  }, [retryStream])
  const playback = useHlsPlayback(videoRef, {
    src: hlsUrl,
    enabled: Boolean(hlsUrl),
    muted,
    autoPlay: true,
    mode: isVodPlayback ? 'vod' : 'live',
    seekOnStart: isVodPlayback ? vodSeekOnStart : undefined,
    latencyMode: isVodPlayback ? 'stable' : settings.playbackLatencyMode,
    onUnauthorizedHls: handleUnauthorizedHls,
    onVodRelayStale: isVodPlayback ? handleVodRelayStale : undefined,
  })

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
  const vodRelayBaseSeconds = useMemo(() => {
    if (!isVodPlayback) return 0
    const resp = streamSession as VodStartResponse | null
    if (!resp || typeof resp.offset_seconds !== 'number') return 0
    return Math.max(0, resp.offset_seconds - buildVodSeekTarget(resp.offset_seconds, resp.seek_seconds))
  }, [isVodPlayback, streamSession])

  useEffect(() => {
    if (useAnalyticsEmbedFirst) return
    setVodEmbedFallback(false)
  }, [channelLogin, vodPlaybackId, retryKey, useAnalyticsEmbedFirst])

  useEffect(() => {
    if (!isVodPlayback || !vodPlayheadStreamId) {
      usePlayheadStore.getState().reset()
      return
    }
    const { setPlayhead, setPlaying } = usePlayheadStore.getState()
    const publish = () => {
      let absoluteSec = 0
      if (showTwitchEmbed) {
        absoluteSec = twitchEmbedRef.current?.getCurrentTime() ?? vodOffsetSeconds
        setPlaying(relayState === 'playing')
      } else {
        const video = videoRef.current
        const current = video && Number.isFinite(video.currentTime)
          ? video.currentTime
          : playback.metrics.currentTimeSec ?? 0
        absoluteSec = vodRelayBaseSeconds + Math.max(0, current)
        setPlaying(playback.state === 'playing')
      }
      setPlayhead(vodPlayheadStreamId, absoluteSec, vodPlaybackId)
    }
    publish()
    const intervalId = window.setInterval(publish, PLAYHEAD_SYNC_INTERVAL_MS)
    return () => {
      window.clearInterval(intervalId)
      usePlayheadStore.getState().reset()
    }
    // playback.metrics.currentTimeSec and embed playhead are read live inside the
    // interval rather than tracked as a dep to keep a steady 1 Hz cadence.
  }, [isVodPlayback, vodPlayheadStreamId, vodPlaybackId, vodRelayBaseSeconds, playback.state, showTwitchEmbed, relayState, vodOffsetSeconds])

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
    queryKey: ['stream-diagnostics', channelLogin, hlsUrl, playback.state],
    queryFn: () => getStreamDiagnostics(channelLogin),
    enabled: Boolean(channelLogin && hlsUrl && !isVodPlayback),
    refetchInterval: hlsUrl ? 5000 : false,
  })
  const emotePreview = useQuery({
    queryKey: ['channel-emotes', channelLogin, emoteStatus.state, emoteStatus.count],
    queryFn: () => getChannelEmotes(channelLogin),
    enabled: Boolean(channelLogin),
    staleTime: 5000,
    refetchInterval: emoteStatus.state === 'loading' || emoteStatus.state === 'processing' ? 5000 : false,
  })
  const liveAnalytics = useQuery({
    queryKey: ['channel-live-analytics', channelLogin],
    queryFn: () => getAnalyticsLive(channelLogin),
    enabled: Boolean(channelLogin),
    staleTime: 15_000,
    refetchInterval: query => (query.state.data?.state === 'live' ? 15_000 : 60_000),
  })

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
      void queryClient.invalidateQueries({ queryKey: ['channel-live-analytics', channelLogin] })
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

  const liveActivityRollups = useMemo(
    () => (liveAnalytics.data?.rollups ?? []).filter(rollup => !rollup.missing),
    [liveAnalytics.data?.rollups],
  )
  const showLiveActivityWaveform = Boolean(
    !isVodPlayback
    && details.data?.isLive
    && trackLiveAnalytics
    && liveActivityRollups.length > 0,
  )

  const vodRollupsQuery = useQuery({
    queryKey: ['vod-activity-rollups', vodAnalyticsStreamId, channelLogin],
    queryFn: () => getAnalyticsStream(vodAnalyticsStreamId, { channel: channelLogin }),
    enabled: Boolean(isVodPlayback && vodFromAnalytics && vodAnalyticsStreamId),
    staleTime: 120_000,
    retry: 1,
  })
  const activityRollups = vodRollupsQuery.data?.rollups ?? null
  const hasActivityRollups = (activityRollups?.length ?? 0) > 0

  useEffect(() => {
    if (!channelLogin || !trackLiveAnalytics) return
    watchAnalyticsChannel(channelLogin).catch(() => undefined)
  }, [channelLogin, trackLiveAnalytics])

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

  useEffect(() => {
    if (!streamSession?.session_id || playback.metrics.firstFrameMs === null) return
    setStartupBenchmarks(current => current.map(entry => (
      entry.sessionId === streamSession.session_id
        ? { ...entry, firstFrameMs: playback.metrics.firstFrameMs }
        : entry
    )))
  }, [playback.metrics.firstFrameMs, streamSession?.session_id])

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
    setVodSeekOnStart(0)

    if (!isVodPlayback) {
      subscribe(channelLogin)
    }

    const start = async () => {
      try {
        const vodStartQuality = isVodPlayback && vodFromAnalytics && settings.preferredQuality === 'best'
          ? autoHighStableQuality
          : settings.preferredQuality
        const requestedQuality = requestQuality(vodStartQuality)
        const response: StartResponse | VodStartResponse = isVodPlayback
          ? await startVodPlayback(vodPlaybackId, vodOffsetSeconds, requestedQuality, 'stable')
          : await startStream(channelLogin, requestedQuality, settings.playbackLatencyMode)
        if (!alive) {
          await stopStream(relaySessionKey, response.session_id).catch(() => undefined)
          return
        }
        sessionIdRef.current = response.session_id
        setStreamSession(response)
        setListeners(response.listeners ?? null)
        if (isVodPlayback) {
          const vodResponse = response as VodStartResponse
          const seekTarget = buildVodSeekTarget(vodResponse.offset_seconds, vodResponse.seek_seconds)
          setVodSeekOnStart(seekTarget)
        }
        const playableUrl = await resolvePlayableHlsUrl(response.hlsUrl)
        if (!alive) {
          await stopStream(relaySessionKey, response.session_id).catch(() => undefined)
          return
        }
        setHlsUrl(playableUrl)
        setRelayState('playing')
        setVodRelayError(null)

        intervalId = setInterval(() => {
          keepaliveStream(relaySessionKey, sessionIdRef.current).catch(() => undefined)
        }, 20000)
      } catch (e) {
        if (!alive) return
        const isOffline = !isVodPlayback && e instanceof ApiError && e.code === 'channel_offline'
        setIsChannelOffline(isOffline)
        setRelayState('error')
        if (isVodPlayback) {
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
      setHlsUrl('')
      if (!isVodPlayback) unsubscribe(channelLogin)
      stopStream(relaySessionKey, sessionIdRef.current).catch(() => undefined)
    }
  }, [channelLogin, isVodPlayback, relaySessionKey, retryKey, settings.playbackLatencyMode, settings.preferredQuality, subscribe, unsubscribe, vodFromAnalytics, vodIdInvalid, vodOffsetSeconds, vodPlaybackId])

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
        if (result.state === 'processing') {
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

  const jumpLive = () => {
    playback.jumpLive()
  }

  const retry = retryStream

  const handleBackToAnalytics = useCallback(() => {
    if (vodAnalyticsHref) window.location.assign(vodAnalyticsHref)
  }, [vodAnalyticsHref])

  const [vodResyncPending, setVodResyncPending] = useState(false)

  const handleVodResync = useCallback(async () => {
    if (!channelLogin) return
    setVodResyncPending(true)
    if (vodAnalyticsStreamId) {
      try {
        await startHistoricalSync(vodAnalyticsStreamId, channelLogin, { vodId: vodPlaybackId })
      } catch {
        // Still navigate so the user can watch sync progress on analytics.
      }
    }
    setVodResyncPending(false)
    if (vodAnalyticsHref) window.location.assign(vodAnalyticsHref)
  }, [channelLogin, vodAnalyticsHref, vodAnalyticsStreamId, vodPlaybackId])

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
  const vodBannerCurrentSec = useMemo(() => {
    if (showTwitchEmbed) return embedMetrics.current ?? vodOffsetSeconds
    const rel = playback.metrics.currentTimeSec
    if (rel == null || !Number.isFinite(rel)) return vodOffsetSeconds
    return vodRelayBaseSeconds + Math.max(0, rel)
  }, [showTwitchEmbed, embedMetrics.current, vodOffsetSeconds, playback.metrics.currentTimeSec, vodRelayBaseSeconds])
  const vodBannerTotalSec = useMemo(() => {
    if (showTwitchEmbed) return embedMetrics.duration ?? vodAnalyticsDurationSec
    return vodAnalyticsDurationSec ?? playback.metrics.seekableEndSec
  }, [showTwitchEmbed, embedMetrics.duration, vodAnalyticsDurationSec, playback.metrics.seekableEndSec])

  const playbackState = showTwitchEmbed ? relayState : (hlsUrl ? playback.state : relayState)
  const hasVodStructuredError = isVodPlayback && vodRelayError !== null && !showTwitchEmbed
  const showStructuredVodError = hasVodStructuredError && (playbackState === 'error' || playbackState === 'retrying')

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
    if (playbackState === 'playing') {
      autoRetryAttemptsRef.current = 0
    }
  }, [playbackState])

  useEffect(() => {
    if (!hlsUrl || error || !playback.error || playback.state !== 'error') return
    const stage = playback.metrics.hlsStage.toLowerCase()
    const retryableStage = ['levelloaderror', 'levelloadtimeout', 'manifestloaderror', 'manifestloadtimeout', 'fragloaderror', 'fragloadtimeout']
      .some(token => stage.includes(token))
    if (!retryableStage || autoRetryAttemptsRef.current >= (isVodPlayback ? 3 : 2)) return
    const timer = window.setTimeout(() => {
      autoRetryAttemptsRef.current += 1
      retry()
    }, 1500)
    return () => window.clearTimeout(timer)
  }, [error, hlsUrl, isVodPlayback, playback.error, playback.metrics.hlsStage, playback.state, retry])

  useEffect(() => {
    const activePlaybackState = showTwitchEmbed ? relayState : (hlsUrl ? playback.state : relayState)
    if (activePlaybackState === 'playing' || error || playback.error) return
    const timer = window.setInterval(() => setStartupNow(Date.now()), 250)
    return () => window.clearInterval(timer)
  }, [error, hlsUrl, playback.error, playback.state, relayState, showTwitchEmbed])

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
  const playbackError = error || playback.error
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
  const playerControlsVisible = playbackState === 'playing' || playbackState === 'buffering' || detailsExpanded
  const lastLiveAgo = details.data?.startedAt
    ? relativeTime(details.data.startedAt)
    : details.data?.updatedAt
      ? relativeTime(details.data.updatedAt / 1000)
      : ''
  const overlayState = startupOverlayState({
    playbackError,
    relayState,
    hlsUrl,
    hlsStage: playback.metrics.hlsStage,
  })
  const startupElapsedMs = playback.metrics.firstFrameMs ?? Math.max(0, startupNow - startupStartedAt)
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
                <span className={`rounded px-2 py-1 ${playbackState === 'playing' ? 'bg-emerald-400/15 text-emerald-200' : playbackState === 'error' ? 'bg-red-400/15 text-red-200' : 'bg-amber-400/15 text-amber-100'}`}>
                  {playbackState}
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
              className={`${mobilePane === 'chat' ? 'hidden' : 'flex'} min-h-0 min-w-0 flex-1 flex-col overflow-y-auto overscroll-y-contain bg-[#0e0e10] lg:flex`}
            >
                <div className={`${mobilePane === 'watch' ? 'block' : 'hidden'} shrink-0 lg:block`}>
                  <div className={playerViewportClass}>
                    <div ref={playerFrameRef} className="group absolute inset-0 overflow-hidden bg-black">
                    <video ref={videoRef} className={`absolute inset-0 h-full w-full bg-black ${showTwitchEmbed ? 'hidden' : ''} ${videoObjectFitClass}`} autoPlay muted={muted} playsInline poster={streamPoster || undefined} />

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
                          setRelayState('error')
                          setVodRelayError({ message, code: 'upstream_token_failed', retryable: true })
                        }}
                      />
                    ) : null}

                    {/* VOD review-mode banner (Req 1.1, 20.x) */}
                    {isVodPlayback ? (
                      <div className="pointer-events-none absolute inset-x-0 top-0 z-30 flex justify-center p-3">
                        <VodModeControls
                          vodId={vodPlaybackId}
                          offsetSeconds={vodOffsetSeconds}
                          channelLogin={channelLogin}
                          currentTimeSec={vodBannerCurrentSec}
                          totalDurationSec={vodBannerTotalSec}
                          analyticsHref={vodAnalyticsHref}
                        />
                      </div>
                    ) : null}

                    {/* Stream thumbnail poster until first frame */}
                    {!isChannelOffline && playback.metrics.firstFrameMs === null && streamPoster && !(playbackError && details.data && !details.data.isLive && !isVodPlayback) ? (
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
                    {playbackState === 'buffering' && playback.metrics.firstFrameMs !== null ? (
                      <div className="absolute inset-0 z-10 grid place-items-center bg-black/30 backdrop-blur-md transition-opacity duration-300">
                        <div className="flex items-center gap-3 rounded-full border border-white/10 bg-black/60 px-5 py-2.5 shadow-2xl shadow-black/50">
                          <div className="h-4 w-4 animate-spin rounded-full border-2 border-violet-300/30 border-t-violet-300" />
                          <span className="text-sm font-black text-white">Buffering</span>
                        </div>
                      </div>
                    ) : null}

                    {/* Startup overlay — subtler bottom-left bar instead of large centered modal */}
                    {!isChannelOffline && !showTwitchEmbed && playbackState !== 'playing' && playbackState !== 'buffering' && (playbackState === 'error' || playbackState === 'retrying' || playback.metrics.firstFrameMs === null) && !(playbackError && details.data && !details.data.isLive && !isVodPlayback && !showStructuredVodError) ? (
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

                        {/* Compact startup status — bottom left; lift above control bar when VOD error actions show */}
                        <div className={`absolute left-4 z-20 max-w-md ${showStructuredVodError ? 'bottom-24 sm:bottom-6' : 'bottom-4'}`}>
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
                              {playback.metrics.firstFrameMs !== null ? (
                                <span className="rounded bg-emerald-400/10 px-1.5 py-0.5 text-emerald-200">Frame {fmtMs(playback.metrics.firstFrameMs)}</span>
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
                        <span>{useAnalyticsEmbedFirst ? 'Twitch player' : 'Twitch embed fallback'}</span>
                        {vodFromAnalytics && vodAnalyticsStreamId ? <span className="text-zinc-500">+ activity graph</span> : null}
                      </div>
                    ) : null}
                    {playbackState === 'playing' && hlsUrl ? (
                      <div className={`absolute left-3 z-20 flex items-center gap-2 rounded-full border border-white/10 bg-black/70 px-3 py-1.5 text-[11px] font-black uppercase text-zinc-300 shadow-lg shadow-black/40 backdrop-blur-sm transition-opacity hover:opacity-100 ${playerControlsVisible ? 'bottom-16 opacity-80' : 'bottom-3 opacity-60'}`}>
                        <span className="h-2 w-2 rounded-full bg-emerald-400 shadow-sm shadow-emerald-400/50" />
                        <span>HLS relay</span>
                        {playback.metrics.behindLiveSec !== null ? <span className="text-zinc-500">+{fmtMetricSec(playback.metrics.behindLiveSec)}</span> : null}
                      </div>
                    ) : null}
                    {!showStructuredVodError ? (
                    <div className={`absolute inset-x-0 bottom-0 z-50 transition-opacity duration-200 ${playerControlsVisible ? 'pointer-events-auto opacity-100' : 'pointer-events-none opacity-0 group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100'} ${showTwitchEmbed ? 'pb-2' : ''}`}>
                      {showAnalyticsActivityWaveform && hasActivityRollups ? (
                        <div className="pointer-events-none px-3 pb-1 lg:px-5 group-hover:pointer-events-auto focus-within:pointer-events-auto">
                          <PlayerHeatmap
                            rollups={activityRollups}
                            totalDurationSec={vodBannerTotalSec ?? ((activityRollups?.length ?? 0) * 60)}
                            isLoading={vodRollupsQuery.isLoading}
                            isError={vodRollupsQuery.isError}
                            onSeek={(offsetSec) => {
                              if (showTwitchEmbed) {
                                twitchEmbedRef.current?.seek(offsetSec)
                                return
                              }
                              const video = videoRef.current
                              if (video) video.currentTime = Math.max(0, offsetSec - vodRelayBaseSeconds)
                            }}
                          />
                        </div>
                      ) : showAnalyticsActivityWaveform && !vodRollupsQuery.isLoading ? (
                        <div className="px-3 pb-1 text-[10px] font-semibold text-zinc-500 lg:px-5">
                          Activity graph needs synced analytics data for this stream.
                        </div>
                      ) : null}
                      <LivePlayerControls
                        playbackState={playbackState}
                        metrics={playback.metrics}
                        isVod={isVodPlayback}
                        requestedQuality={requestedQuality}
                        loadedQuality={loadedQuality}
                        renditions={activeRenditions}
                        latencyMode={playback.effectiveLatencyMode}
                        latencyModeAuto={playback.effectiveLatencyMode !== settings.playbackLatencyMode}
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
                        onJumpLive={jumpLive}
                        onQuality={setPreferredQuality}
                        onLatencyMode={setPlaybackLatencyMode}
                        onVideoFit={setVideoFit}
                        onBottomDensity={setBottomDensity}
                        onDetailsExpanded={setDetailsExpanded}
                      />
                    </div>
                    ) : null}
                    </div>
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
                  trackLiveAnalytics={trackLiveAnalytics}
                  trackAnalyticsPending={trackAnalyticsMutation.isPending}
                  onTrackAnalytics={track => trackAnalyticsMutation.mutate(track)}
                />
                <ChannelTabs
                  activeTab={activeTab}
                  onTab={setActiveTab}
                  insights={insights.data}
                  channel={channelLogin}
                  details={details.data}
                  diagnostics={diagnostics.data}
                  playbackMetrics={playback.metrics}
                  onJumpLive={jumpLive}
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
                />
                {showLiveActivityWaveform ? (
                  <div className="border-t border-white/10 bg-[#0a0a0e] px-3 py-2 lg:px-5">
                    <div className="mb-1.5 flex flex-wrap items-center justify-between gap-2">
                      <span className="text-[10px] font-black uppercase tracking-wide text-zinc-500">Live chat activity</span>
                      <span className="text-[10px] font-semibold text-zinc-600">{liveActivityRollups.length} min collected</span>
                    </div>
                    <ActivityWaveform
                      rollups={liveActivityRollups}
                      totalDurationSec={Math.max(liveActivityRollups.length * 60, 60)}
                      variant="player"
                      showLayerToggles
                    />
                  </div>
                ) : details.data?.isLive && trackLiveAnalytics && !liveAnalytics.isLoading && liveActivityRollups.length === 0 ? (
                  <div className="border-t border-white/10 bg-[#0a0a0e] px-3 py-2 text-[11px] font-semibold text-zinc-500 lg:px-5">
                    Analytics tracking is on — the activity chart will appear after the first minute of rollups.
                  </div>
                ) : null}
                </div>
            </div>
            <aside className={`${mobilePane === 'chat' ? 'flex' : 'hidden'} min-h-0 shrink-0 flex-col overflow-hidden border-t border-white/10 bg-[#111117] lg:flex lg:w-[400px] lg:border-l lg:border-t-0`}>
              <div className="min-h-0 flex-1 overflow-y-auto">
                {isVodPlayback && vodAnalyticsStreamId ? (
                  <VodChatReplayPanel
                    streamId={vodAnalyticsStreamId}
                    currentOffsetSeconds={vodBannerCurrentSec ?? vodOffsetSeconds}
                    isSyncing={vodResyncPending}
                    onSync={handleVodResync}
                    className="flex h-full flex-col border-0 bg-transparent"
                  />
                ) : isVodPlayback ? (
                  <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-center">
                    <p className="text-sm font-semibold text-zinc-300">VOD chat replay</p>
                    <p className="text-xs leading-relaxed text-zinc-500">
                      Sync chat and emotes for this VOD in Analytics first so the URL includes <code className="text-violet-200">sid=</code> and synced chat can load.
                    </p>
                  </div>
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
