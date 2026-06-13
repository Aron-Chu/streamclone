import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { Link, useParams } from 'react-router-dom'
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
  getAnalyticsLive,
  getFollowedChannels,
  getLocalFollowedChannels,
  getStreamDiagnostics,
  keepaliveStream,
  startStream,
  stopStream,
  unfollowChannel,
  watchAnalyticsChannel,
} from '../api'
import type { AnalyticsMinuteRollup, AnalyticsTopEmote, ChannelDetails, ChannelEmote, ChannelInsights, ClipCard, EmoteProvider, SourceStatus, StartResponse, StartupBreakdown, StreamDiagnostics, StatsTimelinePoint, StreamStat } from '../api'
import { useAuth } from '../auth'
import { useChatStore } from '../chatStore'
import { normalizeBrowserOriginUrl } from '../config'
import { useHlsPlayback, type PlaybackMetrics, type PlaybackState } from '../playback'
import { useThemeEffect, useUiSettings, type BottomDensityMode, type ClipPeriod, type PlaybackLatencyMode, type StatsPeriod, type VideoFitMode } from '../settings'
import { autoHighStableQuality, defaultQualityOptions, requestQuality } from '../streamQuality'
import { emoteLoadPercent, formatEmoteProviderProgress, sortChannelEmotesByUsage } from '../emoteUtils'
import BrandLogo from './BrandLogo'
import Chat, { type ChatEmoteStatus } from './Chat'
import ChannelRail from './ChannelRail'
import LocalTokenImportButton from './LocalTokenImportButton'
import PlaybackDiagnostics from './PlaybackDiagnostics'
import SettingsButton from './SettingsPanel'

type ChannelTab = 'about' | 'stats' | 'clips' | 'vods' | 'diagnostics' | 'emotes'

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
  backend,
  startupMs,
  fallbackAttempted,
  detailsExpanded,
  onTogglePlay,
  onMuted,
  onVolume,
  onToggleFullscreen,
  onJumpLive,
  onQuality,
  onLatencyMode,
  onVideoFit,
  onBottomDensity,
  onDetailsExpanded,
}: {
  playbackState: PlaybackState
  metrics: PlaybackMetrics
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
  backend?: string
  startupMs?: number
  fallbackAttempted?: boolean
  detailsExpanded: boolean
  onTogglePlay: () => void
  onMuted: (muted: boolean) => void
  onVolume: (volume: number) => void
  onToggleFullscreen: () => void
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
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={onTogglePlay}
          aria-label="Play or pause"
          className="grid h-9 w-9 place-items-center rounded border border-white/10 bg-white/[0.08] text-xs font-black text-white transition hover:bg-white/15"
        >
          {playbackState === 'playing' ? '❚❚' : '▶'}
        </button>
        <button
          type="button"
          onClick={() => onMuted(!muted)}
          aria-label={muted ? 'Unmute' : 'Mute'}
          className="grid h-9 w-9 place-items-center rounded border border-white/10 bg-white/[0.08] text-xs font-black text-white transition hover:bg-white/15"
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
          className="h-1.5 w-24 cursor-pointer accent-violet-400"
        />
        <div className="relative">
          <button
            type="button"
            aria-haspopup="listbox"
            aria-expanded={qualityOpen}
            onClick={() => setQualityOpen(open => !open)}
            className="flex h-9 min-w-[10rem] items-center justify-between gap-2 rounded border border-white/10 bg-white/[0.08] px-3 text-left text-xs font-black text-white transition hover:bg-white/15"
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
          onClick={onToggleFullscreen}
          aria-label={isFullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
          className="h-9 rounded border border-white/10 bg-white/[0.08] px-3 text-xs font-black text-white transition hover:bg-white/15"
        >
          {isFullscreen ? 'Exit' : 'Fullscreen'}
        </button>
        {detailsExpanded ? (
          <>
            <span className="h-9 rounded border border-white/10 bg-white/[0.045] px-3 py-2 text-xs font-black uppercase text-zinc-300">
              {playbackState}
            </span>
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
              {(['fit', 'fill'] as const).map(mode => (
                <button
                  key={mode}
                  type="button"
                  onClick={() => onVideoFit(mode)}
                  className={`rounded px-3 py-1.5 text-xs font-black uppercase transition ${videoFit === mode ? 'bg-white text-zinc-950' : 'text-zinc-400 hover:bg-white/10 hover:text-white'}`}
                >
                  {mode}
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
            <div className="flex flex-wrap gap-2 text-[11px] font-black uppercase text-zinc-500">
              <span className="rounded bg-white/[0.045] px-2 py-1">Delay {fmtMetricSec(metrics.behindLiveSec)}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1">Buffered {fmtMetricSec(metrics.bufferSizeSec)}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1">Target {fmtMetricSec(metrics.targetLatencySec)}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1">Stage {formatPlaybackStage(metrics.hlsStage)}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1">Backend {backend || '-'}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1">Startup {startupMs ? `${startupMs}ms` : '-'}</span>
              <span className="rounded bg-white/[0.045] px-2 py-1">First frame {fmtMs(metrics.firstFrameMs)}</span>
              {fallbackAttempted ? <span className="rounded bg-cyan-400/10 px-2 py-1 text-cyan-100">Fallback used</span> : null}
            </div>
          </>
        ) : null}
        <button
          type="button"
          onClick={() => onDetailsExpanded(!detailsExpanded)}
          className={`h-9 rounded border px-3 text-xs font-black uppercase transition ${detailsExpanded ? 'border-violet-300/40 bg-violet-400/20 text-violet-100' : 'border-white/10 bg-white/[0.06] text-zinc-200 hover:bg-white/10'}`}
        >
          {detailsExpanded ? 'Less' : 'More'}
        </button>
      </div>
    </section>
  )
}

function MiniViewerSparkline({ values }: { values: number[] }) {
  if (values.length < 2) return null
  const max = Math.max(...values, 1)
  const coords = values.map((value, index) => {
    const x = values.length === 1 ? 0 : (index / (values.length - 1)) * 100
    const y = 24 - (value / max) * 20
    return `${x.toFixed(1)},${y.toFixed(1)}`
  }).join(' ')
  return (
    <svg viewBox="0 0 100 26" className="mt-1 h-6 w-full max-w-[120px]" aria-hidden>
      <polyline fill="none" stroke="rgba(34,211,238,.35)" strokeWidth="6" strokeLinecap="round" points={coords} />
      <polyline fill="none" stroke="rgb(34,211,238)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" points={coords} />
    </svg>
  )
}

function viewerSparklineValues(rollups: AnalyticsMinuteRollup[] | undefined) {
  if (!rollups?.length) return []
  return rollups.slice(-20).map(rollup => rollup.viewerMax || rollup.viewerLatest || rollup.viewerAvg || 0)
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

function ChannelMeta({
  login,
  details,
  detailsLoading,
  quality,
  listeners,
  dense,
  viewerTrend,
}: {
  login: string
  details?: ChannelDetails
  detailsLoading: boolean
  quality: string
  listeners: number | null
  dense: boolean
  viewerTrend?: number[]
}) {
  if (detailsLoading && !details) {
    return <ChannelMetaSkeleton dense={dense} />
  }
  const display = details?.displayName || login
  const title = details?.streamTitle || (detailsLoading ? 'Loading stream details' : `${display}'s channel`)
  const avatar = details?.profileImage
  return (
    <section className={`border-t border-white/10 bg-[#0d0d12] ${dense ? 'px-3 py-3 lg:px-4' : 'px-4 py-4 lg:px-6'}`}>
      <div className={`flex flex-col ${dense ? 'gap-3' : 'gap-4'} 2xl:flex-row 2xl:items-start 2xl:justify-between`}>
        <div className="min-w-0 flex-1">
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <span className={`rounded px-2 py-0.5 text-[11px] font-black uppercase tracking-wide text-white ${details?.isLive ? 'bg-red-600' : 'bg-zinc-600'}`}>
              {details?.isLive ? 'Live' : 'Offline'}
            </span>
            <FollowButton login={login} />
            {details?.category ? <span className="rounded border border-white/10 bg-white/[0.06] px-2 py-0.5 text-xs font-bold text-zinc-200">{details.category}</span> : null}
            {details?.startedAt ? <span className="text-xs font-semibold text-zinc-500">Started {relativeTime(details.startedAt)}</span> : null}
          </div>
          <h1 title={title} className="line-clamp-2 text-xl font-black leading-tight tracking-tight text-white sm:text-2xl">{title}</h1>
          <div className={`mt-3 flex items-start ${dense ? 'gap-2' : 'gap-3'}`}>
            <div className={`grid shrink-0 place-items-center overflow-hidden rounded bg-white/10 text-sm font-black text-violet-100 ${dense ? 'h-10 w-10' : 'h-12 w-12'}`}>
              {avatar ? <img src={avatar} alt={display} className="h-full w-full object-cover" /> : display.slice(0, 1).toUpperCase()}
            </div>
            <div className="min-w-0">
              <div className="font-black text-zinc-100">{display}</div>
              {details?.description ? <p className={`mt-1 max-w-3xl font-medium text-zinc-300 ${dense ? 'line-clamp-1 text-[13px] leading-5' : 'line-clamp-2 text-sm leading-6'}`}>{details.description}</p> : null}
              <div className="mt-2">
                <SourcePills sources={details?.sources} />
              </div>
            </div>
          </div>
        </div>
        <div className={`grid grid-cols-2 text-xs font-bold text-zinc-300 sm:grid-cols-4 ${dense ? 'gap-1.5 2xl:w-[460px]' : 'gap-2 2xl:w-[520px]'}`}>
          <div className={`rounded border border-white/10 bg-white/[0.055] ${dense ? 'px-2.5 py-2' : 'px-3 py-2'}`} title="Live viewer count from Twitch metadata (sidebar uses the same value when this channel is open)">
            <div className="text-[11px] uppercase text-zinc-500">Viewers</div>
            <div className="mt-0.5 text-sm text-white">{fullCount(details?.viewers)}</div>
            {details?.isLive ? <MiniViewerSparkline values={viewerTrend ?? []} /> : null}
          </div>
          <div className={`rounded border border-white/10 bg-white/[0.055] ${dense ? 'px-2.5 py-2' : 'px-3 py-2'}`}>
            <div className="text-[11px] uppercase text-zinc-500">Relay</div>
            <div className="mt-0.5 text-sm text-white">{quality}</div>
          </div>
          <div className={`rounded border border-white/10 bg-white/[0.055] ${dense ? 'px-2.5 py-2' : 'px-3 py-2'}`}>
            <div className="text-[11px] uppercase text-zinc-500">Local</div>
            <div className="mt-0.5 text-sm text-white">{fullCount(listeners)} listeners</div>
          </div>
          <div className={`rounded border border-white/10 bg-white/[0.055] ${dense ? 'px-2.5 py-2' : 'px-3 py-2'}`}>
            <div className="text-[11px] uppercase text-zinc-500">Updated</div>
            <div className="mt-0.5 text-sm text-white">{details?.updatedAt ? relativeTime(details.updatedAt / 1000) : '-'}</div>
          </div>
          <Link to={`/analytics/${encodeURIComponent(login)}`} className="col-span-2 rounded border border-cyan-300/20 bg-cyan-400/10 px-3 py-2 text-center text-xs font-black uppercase text-cyan-100 transition hover:border-cyan-200/60 hover:bg-cyan-400/20 sm:col-span-4">
            Analytics
          </Link>
        </div>
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
        {points.slice(0, 5).map(point => <span key={point.label} className="truncate">{point.label}</span>)}
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
      <div className="grid min-w-[620px] grid-cols-[minmax(0,1.6fr)_120px_120px_120px] gap-3 border-b border-white/10 px-3 py-2 text-[11px] font-black uppercase text-zinc-500">
        <span>Stream</span>
        <span>Average</span>
        <span>Peak</span>
        <span>Watched</span>
      </div>
      {rows.map(row => (
        <Link
          key={row.id}
          to={`/analytics/${encodeURIComponent(channel)}/${encodeURIComponent(row.id)}`}
          className="grid min-w-[620px] grid-cols-[minmax(0,1.6fr)_120px_120px_120px] gap-3 border-b border-white/5 px-3 py-3 text-sm font-bold text-zinc-300 transition last:border-b-0 hover:bg-white/[0.05]"
        >
          <div className="min-w-0">
            <div className="truncate font-black text-white">{row.title}</div>
            <div className="mt-0.5 truncate text-xs text-zinc-500">{row.category || `${fullCount(row.durationMinutes)} minutes`}</div>
          </div>
          <span>{fullCount(row.avgViewers)}</span>
          <span>{fullCount(row.peakViewers)}</span>
          <span>{count(row.hoursWatched)}</span>
        </Link>
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
  loading,
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
}: {
  activeTab: ChannelTab
  onTab: (tab: ChannelTab) => void
  insights?: ChannelInsights
  loading: boolean
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
}) {
  const statsSources = (insights?.sources ?? []).filter(source => sourceMatchesGroup(source, 'stats'))
  const clipSources = (insights?.sources ?? []).filter(source => sourceMatchesGroup(source, 'clips'))
  const tabs: Array<{ id: ChannelTab; label: string }> = [
    { id: 'about', label: 'About' },
    { id: 'stats', label: 'Stats' },
    { id: 'clips', label: 'Clips' },
    { id: 'vods', label: 'VODs' },
    { id: 'diagnostics', label: 'Diagnostics' },
    { id: 'emotes', label: 'Emotes' },
  ]
  return (
    <section className={`border-t border-white/10 bg-[#09090d] ${dense ? 'px-3 py-3 lg:px-4' : 'px-4 py-4 lg:px-6'}`}>
      <div className={`sticky top-0 z-10 -mx-3 bg-[#09090d]/95 px-3 py-2 backdrop-blur-sm lg:-mx-4 lg:px-4 ${dense ? 'mb-2' : 'mb-3'}`}>
        <div className={`flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between`}>
          <div>
            <h2 className="text-base font-black text-white">Channel workspace</h2>
            <div className="mt-1 text-xs font-semibold text-zinc-500">{loading ? 'Loading sources' : insights?.updatedAt ? `${statsPeriod} · updated ${relativeTime(insights.updatedAt / 1000)}` : 'Sources pending'}</div>
          </div>
          <div className="flex flex-wrap rounded border border-white/10 bg-white/[0.045] p-1">
            {tabs.map(tab => (
              <button
                key={tab.id}
                onClick={() => onTab(tab.id)}
                className={`rounded px-3 py-1.5 text-xs font-black transition ${activeTab === tab.id ? 'bg-white text-zinc-950' : 'text-zinc-400 hover:bg-white/10 hover:text-white'}`}
              >
                {tab.label}
              </button>
            ))}
          </div>
        </div>
      </div>

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
          <StreamHistoryTable rows={insights?.streamHistory} sources={statsSources} channel={channel} />
        </div>
      ) : null}
      {activeTab === 'diagnostics' ? (
        <PlaybackDiagnostics channel={channel} metrics={playbackMetrics} diagnostics={diagnostics} sessionId={streamSession?.session_id} onJumpLive={onJumpLive} />
      ) : null}
      {activeTab === 'emotes' ? emotePanel : null}
    </section>
  )
}

function ChannelDetailSections({ details, dense }: { details?: ChannelDetails; dense: boolean }) {
  const displayName = details?.displayName || details?.login || 'Channel'
  const panels = details?.aboutPanels ?? []
  const socialLinks = details?.socialLinks ?? []
  const aboutSources = (details?.sources ?? []).filter(source => source.source === 'twitch_gql_about_panels')
  const aboutIssue = aboutSources.find(source => source.state !== 'ready')
  const factCards = [
    {
      label: 'Status',
      value: details?.isLive ? 'Live now' : 'Offline',
      detail: details?.startedAt ? `Started ${relativeTime(details.startedAt)}` : 'Waiting for the next broadcast.',
    },
    {
      label: 'Category',
      value: details?.category || 'Unlisted',
      detail: details?.streamId ? `Stream ${details.streamId}` : 'No active stream ID right now.',
    },
    {
      label: 'Viewers',
      value: fullCount(details?.viewers),
      detail: details?.isLive ? 'Current live viewer count.' : 'Viewer count appears when the stream is live.',
    },
    {
      label: 'Created',
      value: calendarDate(details?.createdAt),
      detail: details?.updatedAt ? `Metadata refreshed ${relativeTime(details.updatedAt / 1000)}` : 'Metadata refresh time unavailable.',
    },
  ]

  return (
    <div className={dense ? 'space-y-4' : 'space-y-5'}>
      <div className={`grid ${dense ? 'gap-3' : 'gap-4'} xl:grid-cols-[minmax(0,1.75fr)_minmax(300px,.95fr)]`}>
        <section className={`overflow-hidden rounded-2xl border border-white/10 bg-[linear-gradient(180deg,rgba(255,255,255,0.08),rgba(255,255,255,0.03))] shadow-2xl shadow-black/20 ${dense ? 'p-4' : 'p-5'}`}>
          <div className="flex flex-wrap items-center gap-2">
            <span className={`rounded px-2 py-0.5 text-[11px] font-black uppercase tracking-wide text-white ${details?.isLive ? 'bg-red-600' : 'bg-zinc-600'}`}>
              {details?.isLive ? 'Live' : 'Offline'}
            </span>
            {details?.category ? <span className="rounded border border-white/10 bg-white/[0.06] px-2 py-0.5 text-xs font-bold text-zinc-200">{details.category}</span> : null}
            {details?.startedAt ? <span className="text-xs font-semibold text-zinc-400">Started {relativeTime(details.startedAt)}</span> : null}
          </div>
          <div className="mt-4 max-w-4xl">
            <div className="text-[11px] font-black uppercase tracking-[0.18em] text-zinc-500">About {displayName}</div>
            <p className="mt-3 text-base font-medium leading-7 text-zinc-200">
              {details?.description || 'No channel description is available from the current metadata source yet.'}
            </p>
          </div>
          <div className="mt-5">
            <div className="text-[11px] font-black uppercase tracking-[0.18em] text-zinc-500">Metadata sources</div>
            <div className="mt-2">
              <SourcePills sources={details?.sources} />
            </div>
          </div>
          {socialLinks.length ? (
            <div className="mt-5">
              <div className="text-[11px] font-black uppercase tracking-[0.18em] text-zinc-500">Links</div>
              <div className={`mt-3 grid ${dense ? 'gap-2 sm:grid-cols-2 xl:grid-cols-3' : 'gap-3 sm:grid-cols-2 xl:grid-cols-3'}`}>
                {socialLinks.map((link, index) => (
                  <a
                    key={link.id || link.url || index}
                    href={link.url}
                    target="_blank"
                    rel="noreferrer"
                    className="group flex items-center justify-between gap-3 rounded-xl border border-white/10 bg-white/[0.045] px-4 py-3 transition duration-300 hover:-translate-y-0.5 hover:border-violet-300/45 hover:bg-white/[0.07]"
                  >
                    <div className="min-w-0">
                      <div className="truncate text-sm font-black text-white">{link.title || 'Social link'}</div>
                      <div className="mt-1 truncate text-xs font-semibold text-zinc-400">{compactUrl(link.url)}</div>
                    </div>
                    <span className="shrink-0 text-[11px] font-black uppercase tracking-wide text-cyan-100">Open</span>
                  </a>
                ))}
              </div>
            </div>
          ) : null}
        </section>
        <aside className={`grid ${dense ? 'gap-2.5' : 'gap-3'} sm:grid-cols-2 xl:grid-cols-1`}>
          {factCards.map(card => (
            <div key={card.label} className={`rounded-2xl border border-white/10 bg-white/[0.04] shadow-xl shadow-black/10 ${dense ? 'p-3.5' : 'p-4'}`}>
              <div className="text-[11px] font-black uppercase tracking-[0.18em] text-zinc-500">{card.label}</div>
              <div className="mt-2 text-lg font-black text-white">{card.value}</div>
              <div className="mt-1 text-sm font-medium leading-6 text-zinc-400">{card.detail}</div>
            </div>
          ))}
        </aside>
      </div>
      <div>
        <div className="mb-3">
          <div className="text-[11px] font-black uppercase tracking-[0.18em] text-zinc-500">Twitch panels</div>
          <div className="mt-1 text-sm font-semibold text-zinc-400">Images, links, and custom cards pulled from the channel&apos;s About section.</div>
        </div>
        {panels.length ? (
          <div className={`grid ${dense ? 'gap-3 md:grid-cols-2 xl:grid-cols-3' : 'gap-4 md:grid-cols-2'}`}>
            {panels.map((panel, index) => {
              const panelBadge = normalizePanelText(titleCase(panel.type || ''), panel.linkUrl ? 'Link panel' : 'Custom panel')
              const panelTitle = normalizePanelText(panel.title, panel.linkUrl ? compactUrl(panel.linkUrl) : panelBadge)
              const panelBody = panel.description || (panel.linkUrl ? `Opens ${compactUrl(panel.linkUrl)}.` : 'No panel description was provided for this card.')
              const body = (
                <div className="group h-full overflow-hidden rounded-2xl border border-white/10 bg-white/[0.045] shadow-2xl shadow-black/20 transition duration-300 hover:-translate-y-1 hover:border-violet-300/45 hover:bg-white/[0.07]">
                  <div className="relative aspect-[16/9] overflow-hidden bg-[linear-gradient(135deg,#181826,#07070b)]">
                    {panel.imageUrl ? (
                      <img src={panel.imageUrl} alt={panelTitle} className="h-full w-full object-cover transition duration-500 group-hover:scale-105" />
                    ) : (
                      <div className="grid h-full w-full place-items-center text-xs font-black uppercase tracking-[0.2em] text-zinc-500">Twitch panel</div>
                    )}
                  </div>
                  <div className={`space-y-2 ${dense ? 'p-3.5' : 'p-4'}`}>
                    <div className="text-[11px] font-black uppercase tracking-[0.18em] text-zinc-500">{panelBadge}</div>
                    <div className="text-base font-black text-white">{panelTitle}</div>
                    <div className="whitespace-pre-line text-sm font-medium leading-6 text-zinc-300">{panelBody}</div>
                  </div>
                </div>
              )
              if (panel.linkUrl) {
                return (
                  <a key={panel.id || panel.linkUrl || index} href={panel.linkUrl} target="_blank" rel="noreferrer">
                    {body}
                  </a>
                )
              }
              return <div key={panel.id || index}>{body}</div>
            })}
          </div>
        ) : (
          <div className="space-y-3">
            <EmptyPanel title="No custom panels" detail={aboutIssue ? sourceMessageText(aboutIssue) : 'This channel did not expose Twitch About panels from the current metadata sources.'} />
            <SourceDiagnostics sources={aboutSources} />
          </div>
        )}
      </div>
    </div>
  )
}

export default function Channel() {
  const { login } = useParams<{ login: string }>()
  const channelLogin = login ?? ''
  const videoRef = useRef<HTMLVideoElement>(null)
  const sessionIdRef = useRef<string | undefined>()
  const [error, setError] = useState<string | null>(null)
  const [retryKey, setRetryKey] = useState(0)
  const [relayState, setRelayState] = useState<PlaybackState>('starting')
  const [hlsUrl, setHlsUrl] = useState('')
  const [streamSession, setStreamSession] = useState<StartResponse | null>(null)
  const [listeners, setListeners] = useState<number | null>(null)
  const [mobileRailOpen, setMobileRailOpen] = useState(false)
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
    if (autoRetryAttemptsRef.current >= 2) return
    autoRetryAttemptsRef.current += 1
    retryStream()
  }, [retryStream])
  const playback = useHlsPlayback(videoRef, {
    src: hlsUrl,
    enabled: Boolean(hlsUrl),
    muted,
    autoPlay: true,
    latencyMode: settings.playbackLatencyMode,
    onUnauthorizedHls: handleUnauthorizedHls,
  })

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
    enabled: Boolean(channelLogin && hlsUrl),
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
    staleTime: 30_000,
    refetchInterval: query => (query.state.data?.state === 'live' ? 30_000 : 60_000),
  })

  useEffect(() => {
    if (!channelLogin) return
    watchAnalyticsChannel(channelLogin).catch(() => undefined)
  }, [channelLogin])

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
    let alive = true
    let intervalId: ReturnType<typeof setInterval> | null = null
    sessionIdRef.current = undefined
    setError(null)
    setIsChannelOffline(false)
    setStartupStartedAt(Date.now())
    setRelayState(retryKey > 0 ? 'retrying' : 'starting')
    setHlsUrl('')
    setStreamSession(null)
    setListeners(null)

    subscribe(channelLogin)

    const start = async (attempt = 0) => {
      try {
        const requestedQuality = requestQuality(settings.preferredQuality || 'best')
        const response = await startStream(channelLogin, requestedQuality, settings.playbackLatencyMode)
        if (!alive) {
          await stopStream(channelLogin, response.session_id).catch(() => undefined)
          return
        }
        sessionIdRef.current = response.session_id
        setStreamSession(response)
        setListeners(response.listeners ?? null)
        const playableUrl = await resolvePlayableHlsUrl(response.hlsUrl)
        if (!alive) {
          await stopStream(channelLogin, response.session_id).catch(() => undefined)
          return
        }
        setHlsUrl(playableUrl)
        setRelayState('playing')

        intervalId = setInterval(() => {
          keepaliveStream(channelLogin, sessionIdRef.current).catch(() => undefined)
        }, 20000)
      } catch (e) {
        if (alive && e instanceof ApiError && e.code === 'hls_not_ready' && e.retryable && attempt < 1) {
          setRelayState('retrying')
          await start(attempt + 1)
          return
        }
        if (alive) {
          const isOffline = e instanceof ApiError && e.code === 'channel_offline'
          setIsChannelOffline(isOffline)
          setRelayState('error')
          setError(isOffline ? 'This channel is currently offline.' : ((e as Error).message || 'stream start failed'))
        }
      }
    }
    start()

    return () => {
      alive = false
      if (intervalId) clearInterval(intervalId)
      setHlsUrl('')
      unsubscribe(channelLogin)
      stopStream(channelLogin, sessionIdRef.current).catch(() => undefined)
    }
  }, [channelLogin, retryKey, settings.playbackLatencyMode, settings.preferredQuality, subscribe, unsubscribe])

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
    updateSettings({ preferredQuality: value })
    setError(null)
  }

  const setPlaybackLatencyMode = (value: PlaybackLatencyMode) => {
    updateSettings({ playbackLatencyMode: value })
  }

  const setVideoFit = (value: VideoFitMode) => {
    updateSettings({ videoFit: value })
  }

  const setBottomDensity = (value: BottomDensityMode) => {
    updateSettings({ bottomDensity: value })
  }

  const togglePlay = () => {
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
  }

  const toggleFullscreen = async () => {
    const video = videoRef.current
    if (!video) return
    try {
      if (document.fullscreenElement) {
        await document.exitFullscreen()
      } else {
        await video.requestFullscreen()
      }
    } catch {
      return
    }
  }

  const jumpLive = () => {
    playback.jumpLive()
  }

  const retry = retryStream

  useEffect(() => {
    const video = videoRef.current
    if (!video) return
    video.volume = settings.playerVolume
  }, [settings.playerVolume, hlsUrl])

  useEffect(() => {
    const onFullscreenChange = () => setIsFullscreen(document.fullscreenElement === videoRef.current)
    document.addEventListener('fullscreenchange', onFullscreenChange)
    return () => document.removeEventListener('fullscreenchange', onFullscreenChange)
  }, [])

  useEffect(() => {
    autoRetryAttemptsRef.current = 0
  }, [channelLogin])

  useEffect(() => {
    if ((hlsUrl ? playback.state : relayState) === 'playing') {
      autoRetryAttemptsRef.current = 0
    }
  }, [hlsUrl, playback.state, relayState])

  useEffect(() => {
    if (!hlsUrl || error || !playback.error || playback.state !== 'error') return
    const stage = playback.metrics.hlsStage.toLowerCase()
    const retryableStage = ['levelloaderror', 'levelloadtimeout', 'manifestloaderror', 'manifestloadtimeout', 'fragloaderror', 'fragloadtimeout']
      .some(token => stage.includes(token))
    if (!retryableStage || autoRetryAttemptsRef.current >= 2) return
    const timer = window.setTimeout(() => {
      autoRetryAttemptsRef.current += 1
      retry()
    }, 1500)
    return () => window.clearTimeout(timer)
  }, [error, hlsUrl, playback.error, playback.metrics.hlsStage, playback.state])

  useEffect(() => {
    const activePlaybackState = hlsUrl ? playback.state : relayState
    if (activePlaybackState === 'playing' || error || playback.error) return
    const timer = window.setInterval(() => setStartupNow(Date.now()), 250)
    return () => window.clearInterval(timer)
  }, [error, hlsUrl, playback.error, playback.state, relayState])

  const sortedLoadedEmotes = useMemo(
    () => sortChannelEmotesByUsage(emotePreview.data ?? [], liveAnalytics.data?.topEmotes),
    [emotePreview.data, liveAnalytics.data?.topEmotes],
  )
  const headerTitle = useMemo(() => details.data?.displayName || channelLogin || 'Channel', [channelLogin, details.data?.displayName])
  const viewerTrend = useMemo(
    () => (details.data?.isLive ? viewerSparklineValues(liveAnalytics.data?.rollups) : []),
    [details.data?.isLive, liveAnalytics.data?.rollups],
  )
  const railViewerOverrides = useMemo(() => {
    if (!channelLogin || details.data?.viewers == null) return undefined
    return { [channelLogin]: details.data.viewers }
  }, [channelLogin, details.data?.viewers])
  const streamPoster = details.data?.thumbnailUrl?.replace('{width}', '960').replace('{height}', '540')
  const playbackState = hlsUrl ? playback.state : relayState
  const playbackError = error || playback.error
  const activeRenditions = streamSession?.renditions ?? diagnostics.data?.renditions
  const activeSelectedRendition = streamSession?.selectedRendition ?? diagnostics.data?.selectedRendition
  const requestedQuality = resolveRequestedQuality(activeRenditions, settings.preferredQuality, activeSelectedRendition)
  const loadedQuality = selectedRenditionText(streamSession, diagnostics.data)
  const isDenseBottom = settings.bottomDensity === 'dense'
  const compactPlayerHeightClass = detailsExpanded
    ? isDenseBottom ? 'h-[clamp(130px,22vh,26vh)]' : 'h-[clamp(170px,30vh,34vh)]'
    : isDenseBottom ? 'h-[clamp(220px,42vh,48vh)]' : 'h-[clamp(240px,48vh,54vh)]'
  const playerViewportClass = isTheater
    ? 'grid min-h-[180px] flex-1 place-items-center bg-black transition-[flex,height] duration-200'
    : `grid min-h-[170px] shrink-0 place-items-center bg-black transition-[flex,height] duration-200 ${compactPlayerHeightClass}`
  const playerFrameClass = settings.videoFit === 'fill' || isTheater
    ? 'group relative h-full w-full min-h-0 overflow-hidden bg-black'
    : 'group relative aspect-video h-full max-h-full max-w-full overflow-hidden bg-black'
  const lastLiveAgo = details.data?.startedAt
    ? relativeTime(details.data.startedAt)
    : details.data?.updatedAt
      ? relativeTime(details.data.updatedAt / 1000)
      : ''
  const showBottomPanel = !isTheater || detailsExpanded
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
    <main className="h-screen overflow-hidden bg-[#050507] text-zinc-100">
      <div className="relative flex h-screen min-h-0 overflow-hidden bg-[linear-gradient(135deg,rgba(139,92,246,.14),rgba(5,5,7,0)_32%),linear-gradient(180deg,#07070a,#050507)]">
        <ChannelRail
          collapsed={railCollapsed}
          mobileOpen={mobileRailOpen}
          onToggleCollapsed={() => setRailCollapsed(v => !v)}
          onCloseMobile={() => setMobileRailOpen(false)}
          viewerOverrides={railViewerOverrides}
        />
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          <header className="relative z-30 flex min-h-16 items-center justify-between gap-3 border-b border-white/10 bg-black/45 px-3 py-3 backdrop-blur-xl lg:px-5">
            <div className="flex min-w-0 items-center gap-2">
              <button onClick={() => setMobileRailOpen(true)} className="rounded border border-white/10 bg-white/[0.06] px-3 py-2 text-sm font-black text-white lg:hidden">
                Menu
              </button>
              <Link to="/" className="flex shrink-0 items-center gap-3 rounded px-2 py-1 transition hover:bg-white/10">
                <BrandLogo size="sm" showText />
                <span className="hidden rounded bg-white/10 px-2 py-0.5 text-xs font-bold text-zinc-300 sm:inline">Browse</span>
              </Link>
              <div className="hidden min-w-0 border-l border-white/10 pl-3 md:block">
                <div className="truncate text-sm font-black text-white">{headerTitle}</div>
                <div className="truncate text-xs font-semibold text-zinc-500">{details.data?.streamTitle || details.data?.category || 'Channel workspace'}</div>
              </div>
            </div>
            <div className="flex shrink-0 items-center gap-2">
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

          <div className="flex min-h-0 flex-1 flex-col overflow-hidden lg:flex-row">
            <section className="flex min-h-0 flex-1 flex-col overflow-hidden bg-black">
              <div className={`flex min-h-0 flex-col ${isTheater ? (detailsExpanded ? 'min-h-0 flex-[3]' : 'min-h-0 flex-1') : 'shrink-0'}`}>
                <div className={playerViewportClass}>
                  <div className={playerFrameClass}>
                    <video ref={videoRef} className={`h-full w-full bg-black ${settings.videoFit === 'fill' ? 'object-cover' : 'object-contain'}`} autoPlay muted={muted} playsInline poster={streamPoster || undefined} />

                    {/* Stream thumbnail poster until first frame */}
                    {!isChannelOffline && playback.metrics.firstFrameMs === null && streamPoster && !(playbackError && details.data && !details.data.isLive) ? (
                      <img
                        src={streamPoster}
                        alt=""
                        className="pointer-events-none absolute inset-0 z-[1] h-full w-full object-contain"
                        aria-hidden
                      />
                    ) : null}

                    {/* Offline background */}
                    {(isChannelOffline || (playbackError && details.data && !details.data.isLive)) ? (
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
                    {!isChannelOffline && playbackState !== 'playing' && playbackState !== 'buffering' && (playbackState === 'error' || playbackState === 'retrying' || playback.metrics.firstFrameMs === null) && !(playbackError && details.data && !details.data.isLive) ? (
                      <div className="absolute inset-0 z-10">
                        {/* Blurred profile background during startup */}
                        {details.data?.profileImage ? (
                          <img
                            src={details.data.profileImage}
                            alt=""
                            className="absolute inset-0 h-full w-full object-cover opacity-15 blur-3xl scale-110"
                          />
                        ) : null}
                        <div className="absolute inset-0 bg-black/60" />

                        {/* Compact startup status — bottom left */}
                        <div className="absolute bottom-4 left-4 z-20 max-w-md">
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
                                  onClick={() => setActiveTab('diagnostics')}
                                  className="rounded border border-white/15 bg-white/[0.06] px-3 py-1.5 text-xs font-black text-zinc-200 transition hover:bg-white/10"
                                >
                                  Diagnostics
                                </button>
                              </div>
                            ) : null}
                          </div>
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
                    {playbackState === 'playing' && hlsUrl ? (
                      <div className="absolute bottom-3 left-3 z-20 flex items-center gap-2 rounded-full border border-white/10 bg-black/70 px-3 py-1.5 text-[11px] font-black uppercase text-zinc-300 shadow-lg shadow-black/40 backdrop-blur-sm transition-opacity hover:opacity-100 opacity-60">
                        <span className="h-2 w-2 rounded-full bg-emerald-400 shadow-sm shadow-emerald-400/50" />
                        <span>HLS relay</span>
                        {playback.metrics.behindLiveSec !== null ? <span className="text-zinc-500">+{fmtMetricSec(playback.metrics.behindLiveSec)}</span> : null}
                      </div>
                    ) : null}
                    <button
                      type="button"
                      onClick={() => setIsTheater(value => !value)}
                      title={isTheater ? 'Exit theater mode' : 'Theater mode'}
                      aria-label={isTheater ? 'Exit theater mode' : 'Theater mode'}
                      className="absolute bottom-3 right-3 z-20 rounded border border-white/15 bg-black/75 px-3 py-2 text-xs font-black uppercase text-white opacity-0 shadow-xl shadow-black/40 transition hover:border-violet-200/60 hover:bg-violet-500/30 group-hover:opacity-100"
                    >
                      {isTheater ? 'Shrink' : 'Theater'}
                    </button>
                    <div className="pointer-events-none absolute inset-x-0 bottom-0 z-30 opacity-0 transition-opacity duration-200 group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100">
                      <LivePlayerControls
                        playbackState={playbackState}
                        metrics={playback.metrics}
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
                        backend={streamSession?.workerBackend ?? diagnostics.data?.workerBackend}
                        startupMs={streamSession?.startupMs ?? diagnostics.data?.startupMs}
                        fallbackAttempted={streamSession?.fallbackAttempted || Boolean(diagnostics.data?.fallbackAttempts)}
                        detailsExpanded={detailsExpanded}
                        onTogglePlay={togglePlay}
                        onMuted={setMuted}
                        onVolume={setPlayerVolume}
                        onToggleFullscreen={() => void toggleFullscreen()}
                        onJumpLive={jumpLive}
                        onQuality={setPreferredQuality}
                        onLatencyMode={setPlaybackLatencyMode}
                        onVideoFit={setVideoFit}
                        onBottomDensity={setBottomDensity}
                        onDetailsExpanded={setDetailsExpanded}
                      />
                    </div>
                  </div>
                </div>
              </div>
              <div className={`min-h-0 overflow-y-auto transition-[flex,max-height,opacity] duration-200 ${showBottomPanel ? (isTheater && detailsExpanded ? 'min-h-0 flex-1' : 'flex-1') : 'max-h-0 flex-none overflow-hidden opacity-0'}`}>
                <ChannelMeta
                  login={channelLogin}
                  details={details.data}
                  detailsLoading={details.isLoading}
                  quality={loadedQuality}
                  listeners={listeners}
                  dense={isDenseBottom}
                  viewerTrend={viewerTrend}
                />
                <ChannelTabs
                  activeTab={activeTab}
                  onTab={setActiveTab}
                  insights={insights.data}
                  loading={insights.isLoading}
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
                />
              </div>
            </section>
            <aside className="flex h-[44vh] shrink-0 flex-col border-t border-white/10 bg-[#111117] lg:h-auto lg:w-[400px] lg:border-l lg:border-t-0">
              <div className="min-h-0 flex-1">
                <Chat
                  channel={channelLogin}
                  user={auth.user}
                  isAuthenticated={auth.isAuthenticated}
                  emotes={emoteStatus}
                  badgeCatalog={badgeCatalog.data?.badges ?? {}}
                  loadedEmotes={sortedLoadedEmotes}
                />
              </div>
            </aside>
          </div>
        </div>
      </div>
    </main>
  )
}
