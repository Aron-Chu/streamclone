import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { PULSE_WIRE_ENABLED } from '../../config'
import {
  addWatchEntry,
  deleteWatchEntry,
  fetchDeveloping,
  fetchPulseWireBans,
  fetchPulseWireCommunity,
  fetchPulseWireCommunityFlairs,
  fetchPulseWireFeed,
  fetchPulseWireStory,
  fetchPulseWireUnlinkedEvidence,
  fetchSourceHealth,
  fetchTopClips,
  fetchTrendingStreamers,
  fetchWatchEntries,
  PulseWireApiError,
  type PulseWireCursor,
  type PulseWireBanEvent,
  type PulseWireCommunityFlair,
  type PulseWireCommunityPost,
  type PulseWireFeedSort,
  type PulseWireRankModel,
  type PulseWireSourceHealth,
  type PulseWireStory,
  type PulseWireTopClip,
  type PulseWireTrendingStreamer,
  type PulseWireUnlinkedEvidence,
  type PulseWireWatchEntry,
  type PulseWireWindow,
} from '../../pulseWireApi'
import { formatCompactCount, formatEngagementCount, formatRelativeTime, hasEngagementCounts, windowEditionTitle, windowShortLabel, windowTagline } from '../../utils/pulseWireFormat'
import { readAnalystMode, writeAnalystMode } from '../../utils/pulseWireAnalystMode'
import { isCrossPlatformStory, toWireStoryView } from '../../utils/pulseWireStoryView'
import { pulseWireDisplayThumbnail } from '../../utils/twitchClipThumb'
import { DirectoryLayout } from '../directory/DirectoryLayout'
import DevelopingPanel from './DevelopingPanel'
import SourceHealthPanel from './SourceHealthPanel'
import PulseWireStoryDetail from './PulseWireStoryDetail'
import PulseWireFilters, { chipLabel, chipToFeedParams, type PulseWireFilterChip } from './PulseWireFilters'
import PulseWireEditionHeader from './PulseWireEditionHeader'
import NewsSection from './NewsSection'
import ClipThumbnail from './community/ClipThumbnail'
import CommunityPostCard from './community/CommunityPostCard'
import WireUnlinkedPanel from './WireUnlinkedPanel'
import LeadStoryDesk from './LeadStoryDesk'
import WireStoryLanes from './WireStoryLanes'
import WireReaderRail from './WireReaderRail'

const WARM_POLL_MS = 30_000
const WARM_CAP_MS = 5 * 60_000

type PulseWireTab = 'trending' | 'wire'

function parseWindow(raw: string | null): PulseWireWindow {
  if (raw === 'today') return 'today'
  if (raw === '7d') return '7d'
  return '24h'
}

function parseSort(raw: string | null): PulseWireFeedSort {
  if (raw === 'updated' || raw === 'volatility') return raw
  return 'rank'
}

function parseTab(raw: string | null): PulseWireTab {
  return raw === 'wire' ? 'wire' : 'trending'
}

function parseTrendingFlair(raw: string | null | undefined) {
  const trimmed = (raw ?? '').trim()
  return trimmed || null
}

function parseChip(params: URLSearchParams): PulseWireFilterChip {
  const filter = params.get('filter')
  if (filter === 'high_volatility') return 'high_volatility'
  if (filter === 'saved') return 'saved'
  const state = params.get('state')
  if (state === 'published') return 'live_now'
  if (state === 'unverified') return 'unverified'
  const category = params.get('category')
  if (category === 'drama' || category === 'funny' || category === 'bans' || category === 'records' || category === 'esports') {
    return category
  }
  return 'all'
}

function writeChipParams(params: URLSearchParams, chip: PulseWireFilterChip) {
  params.delete('filter')
  params.delete('state')
  params.delete('category')
  switch (chip) {
    case 'live_now':
      params.set('state', 'published')
      break
    case 'unverified':
      params.set('state', 'unverified')
      break
    case 'high_volatility':
      params.set('filter', 'high_volatility')
      break
    case 'saved':
      params.set('filter', 'saved')
      break
    case 'drama':
    case 'funny':
    case 'bans':
    case 'records':
    case 'esports':
      params.set('category', chip)
      break
    default:
      break
  }
}

function responseCursor(res: { cursor?: PulseWireCursor | null; nextCursor?: PulseWireCursor | null }) {
  return res.nextCursor ?? res.cursor ?? null
}

function hasCursor(cursor: PulseWireCursor | null) {
  return cursor != null && cursor !== '' && cursor !== 0 && cursor !== '0'
}

function FeedSkeleton() {
  return (
    <div className="space-y-4 animate-pulse">
      <div className="h-56 rounded-2xl border border-[#2A2A2E] bg-[#121217]" />
      <div className="h-28 rounded-xl border border-[#2A2A2E] bg-[#121217]" />
      <div className="h-28 rounded-xl border border-[#2A2A2E] bg-[#121217]" />
    </div>
  )
}

function sortFeed(items: PulseWireStory[], sort: PulseWireFeedSort) {
  if (sort === 'rank' || sort === 'updated') return items
  return [...items].sort((a, b) => (b.scores.volatility ?? -1) - (a.scores.volatility ?? -1))
}

function normalizeSearchQuery(value: string | null | undefined) {
  return (value ?? '').trim().toLowerCase()
}

function searchableTopic(value: string | null | undefined) {
  return normalizeSearchQuery(value).replace(/[_-]+/g, ' ')
}

function searchMatchRank(story: PulseWireStory, query: string) {
  if (!query) return 0
  const login = story.entity?.login?.toLowerCase() ?? ''
  const displayName = story.entity?.displayName?.toLowerCase() ?? ''
  if (login.includes(query) || displayName.includes(query)) return 3
  if ((story.story.title ?? '').toLowerCase().includes(query)) return 2
  const category = searchableTopic(story.story.category)
  const storyClass = searchableTopic(story.story.storyClass)
  if (category.includes(query) || storyClass.includes(query)) return 1
  return 0
}

function filterStoriesBySearch(items: PulseWireStory[], rawQuery: string) {
  const query = normalizeSearchQuery(rawQuery)
  if (!query) return items
  return items
    .map((item, index) => ({ item, index, rank: searchMatchRank(item, query) }))
    .filter(result => result.rank > 0)
    .sort((a, b) => b.rank - a.rank || a.index - b.index)
    .map(result => result.item)
}

const TRENDING_SOURCE_KEYS = ['reddit', 'youtube', 'twitchclips', 'streamerbans'] as const

function sourceDisplayName(key: string) {
  switch (key) {
    case 'reddit':
      return 'Reddit'
    case 'youtube':
      return 'YouTube'
    case 'twitchclips':
      return 'Clips'
    case 'streamerbans':
      return 'StreamerBans'
    default:
      return key
  }
}

function sourceModeLabel(mode: string | undefined) {
  switch (mode) {
    case 'link_only':
      return 'link only'
    case 'deferred':
      return 'deferred'
    case 'degraded':
      return 'degraded'
    case 'active':
      return 'active'
    case 'off':
      return 'off'
    case 'error':
      return 'error'
    default:
      return mode || 'unknown'
  }
}

function sourceModeClasses(mode: string | undefined) {
  switch (mode) {
    case 'active':
      return 'border-emerald-400/35 bg-emerald-500/10 text-emerald-100'
    case 'degraded':
      return 'border-amber-400/40 bg-amber-500/10 text-amber-100'
    case 'error':
      return 'border-red-400/35 bg-red-500/10 text-red-100'
    case 'off':
    case 'deferred':
      return 'border-[#3A3A40] bg-[#18181C] text-[#ADADB8]'
    default:
      return 'border-[#2A2A2E] bg-[#141418] text-[#C8C8D0]'
  }
}

function sourceOffHint(key: string, source: { last_error?: string; hint?: string }) {
  if (source.last_error) return source.last_error
  if (source.hint) return source.hint
  switch (key) {
    case 'reddit':
      return 'Set REDDIT_COMMERCIAL_OK=true in .env and recreate storygraph.'
    case 'streamerbans':
      return 'Set STREAMERBANS_INGEST_ENABLED=true in .env and recreate storygraph.'
    default:
      return ''
  }
}

function sourceHealthNote(key: string, source: { mode?: string; last_error?: string; hint?: string }) {
  const note = sourceOffHint(key, source) || source.last_error || source.hint || ''
  if (source.mode === 'off' && note.includes('recreate storygraph')) {
    return `${note} Run make reload-env-if-stale if .env is already correct.`
  }
  return note
}

function TrendingSourceHealthRow({ sources }: { sources: PulseWireSourceHealth }) {
  const items = TRENDING_SOURCE_KEYS.map(key => ({ key, source: sources[key] })).filter(item => item.source)
  if (!items.length) return null
  return (
    <div
      className="mb-4 rounded-lg border border-[#232329] bg-[#111116] px-3 py-2"
      data-testid="trending-source-health"
      aria-label="Trending source health"
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="mr-1 text-[11px] font-semibold uppercase tracking-wide text-[#7A7A85]">Sources</span>
        {items.map(({ key, source }) => {
          const mode = sourceModeLabel(source.mode)
          const note = sourceHealthNote(key, source)
          return (
            <span
              key={key}
              className={`inline-flex max-w-full items-center gap-1 rounded-full border px-2 py-1 text-[11px] font-semibold ${sourceModeClasses(source.mode)}`}
              title={note || `${sourceDisplayName(key)} ${mode}`}
            >
              <span>{sourceDisplayName(key)}</span>
              <span className="text-current/70">{mode}</span>
              {note && (source.mode === 'degraded' || source.mode === 'error' || source.mode === 'off') ? (
                <span className="hidden max-w-[280px] truncate text-current/65 sm:inline">{note}</span>
              ) : null}
            </span>
          )
        })}
      </div>
    </div>
  )
}

type TrendingNewsState = {
  community: PulseWireCommunityPost[]
  clips: PulseWireTopClip[]
  bans: PulseWireBanEvent[]
}

type TrendingLeadPick =
  | { kind: 'community'; post: PulseWireCommunityPost }
  | { kind: 'clip'; clip: PulseWireTopClip }
  | { kind: 'ban'; ban: PulseWireBanEvent }

function communityHasPreview(post: PulseWireCommunityPost) {
  const kind = post.previewKind ?? (post.displayThumbnailUrl ? 'fallback' : 'none')
  return kind !== 'none' && Boolean(post.displayThumbnailUrl)
}

function pickTrendingLead(data: TrendingNewsState): TrendingLeadPick | null {
  const community = data.community[0]
  const clip = data.clips[0]
  const ban = data.bans[0]

  if (community && communityHasPreview(community)) return { kind: 'community', post: community }
  if (clip?.displayThumbnailUrl) return { kind: 'clip', clip }
  if (community) return { kind: 'community', post: community }
  if (clip) return { kind: 'clip', clip }
  if (ban) return { kind: 'ban', ban }
  return null
}

function TrendingClipsScroller({ clips }: { clips: PulseWireTopClip[] }) {
  if (!clips.length) {
    return (
      <p className="rounded-lg border border-[#2A2A2E] bg-[#121217] p-4 text-sm text-[#7A7A85]">
        No top clips in this window yet.
      </p>
    )
  }
  return (
    <div
      data-testid="trending-clips-row"
      className="-mx-1 flex gap-3 overflow-x-auto pb-1 snap-x snap-mandatory [scrollbar-width:thin]"
    >
      {clips.map(clip => (
        <div key={clip.id} className="w-[min(260px,78vw)] shrink-0 snap-start">
          <ClipNewsCard clip={clip} compact />
        </div>
      ))}
    </div>
  )
}

function TrendingLeadSkeleton() {
  return <div className="h-56 animate-pulse rounded-xl border border-[#2A2A2E] bg-[#121217]" />
}

function TrendingPageSkeleton() {
  return (
    <div className="space-y-6">
      <div>
        <div className="mb-3 h-4 w-24 animate-pulse rounded bg-[#1B1B1F]" />
        <TrendingLeadSkeleton />
      </div>
      <div>
        <div className="mb-3 h-4 w-20 animate-pulse rounded bg-[#1B1B1F]" />
        <div className="flex gap-3 overflow-hidden">
          {[0, 1, 2].map(key => (
            <div key={key} className="h-40 w-64 shrink-0 animate-pulse rounded-xl border border-[#2A2A2E] bg-[#121217]" />
          ))}
        </div>
      </div>
      <div>
        <div className="mb-3 h-4 w-32 animate-pulse rounded bg-[#1B1B1F]" />
        <div className="grid gap-3 md:grid-cols-2">
          {[0, 1].map(key => (
            <div key={key} className="h-36 animate-pulse rounded-xl border border-[#2A2A2E] bg-[#121217]" />
          ))}
        </div>
      </div>
    </div>
  )
}

function TrendingFlairChips({
  flairs,
  activeFlair,
  loading,
  onSelect,
}: {
  flairs: PulseWireCommunityFlair[]
  activeFlair: string | null
  loading: boolean
  onSelect: (flair: string | null) => void
}) {
  const chips = flairs.filter(item => item.flair.trim())
  if (!loading && !chips.length && !activeFlair) return null
  return (
    <div
      className="mb-4 flex flex-wrap gap-2"
      role="group"
      aria-label="LSF flair filters"
      data-testid="trending-flair-chips"
    >
      <button
        type="button"
        aria-pressed={!activeFlair}
        onClick={() => onSelect(null)}
        className={`rounded-full border px-3 py-1.5 text-xs font-semibold transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] ${
          !activeFlair
            ? 'border-[#A970FF] bg-[#9147FF]/20 text-[#EFEFF1]'
            : 'border-[#2A2A2E] bg-[#1B1B1F] text-[#ADADB8] hover:border-[#3A3A40] hover:text-[#EFEFF1]'
        }`}
      >
        All
      </button>
      {loading && !chips.length ? (
        <span className="self-center text-xs text-[#7A7A85]">Loading flairs…</span>
      ) : null}
      {chips.map(item => {
        const active = activeFlair === item.flair
        return (
          <button
            key={item.flair}
            type="button"
            aria-pressed={active}
            onClick={() => onSelect(item.flair)}
            className={`rounded-full border px-3 py-1.5 text-xs font-semibold transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] ${
              active
                ? 'border-[#FF7447]/50 bg-[#2A1710] text-[#FFB199]'
                : 'border-[#2A2A2E] bg-[#1B1B1F] text-[#ADADB8] hover:border-[#3A3A40] hover:text-[#EFEFF1]'
            }`}
          >
            {item.flair}
            {item.count != null ? (
              <span className="ml-1 text-current/65">{formatCompactCount(item.count)}</span>
            ) : null}
          </button>
        )
      })}
    </div>
  )
}

function sourceTone(source: string) {
  switch (source) {
    case 'LSF hot':
      return 'border-[#FF7447]/35 bg-[#2A1710] text-[#FFB199]'
    case 'top clip':
      return 'border-[#A970FF]/35 bg-[#201633] text-[#D6C2FF]'
    case 'ban event':
      return 'border-[#FF5C57]/35 bg-[#2A1515] text-[#FFB5B2]'
    default:
      return 'border-[#2A2A2E] bg-[#1B1B1F] text-[#D6D6DE]'
  }
}

function ClipNewsCard({ clip, compact = false }: { clip: PulseWireTopClip; compact?: boolean }) {
  return (
    <a
      href={clip.url}
      target="_blank"
      rel="noreferrer"
      className={`group block overflow-hidden rounded-lg border border-[#2A2A2E] bg-[#121217] transition hover:border-[#A970FF]/40 ${compact ? '' : 'h-full'}`}
    >
      <div className="relative aspect-video bg-[#0C0C0F]">
        <ClipThumbnail
          displayThumbnailUrl={clip.displayThumbnailUrl}
          title={clip.title}
          className="h-full w-full object-cover transition duration-300 group-hover:scale-105"
        />
        <span className="absolute left-2 top-2 rounded-full border border-[#A970FF]/35 bg-black/70 px-2 py-0.5 text-[10px] font-bold uppercase text-[#D6C2FF]">
          Top clip
        </span>
      </div>
      <div className="p-3">
        <h4 className={`font-semibold leading-5 text-[#F7F7F8] ${compact ? 'line-clamp-2 text-sm' : 'line-clamp-3 text-base'}`}>{clip.title}</h4>
        <p className="mt-2 text-[11px] font-semibold text-[#7A7A85]">
          {formatCompactCount(clip.viewCount)} views
          {clip.streamerLogin ? ` · ${clip.streamerDisplayName || clip.streamerLogin}` : ''}
        </p>
      </div>
    </a>
  )
}

function BanEventCard({ ban }: { ban: PulseWireBanEvent }) {
  const avatarSrc = pulseWireDisplayThumbnail(ban.displayThumbnailUrl ?? ban.previewUrl)
  const initials = (ban.streamerDisplayName || ban.streamerLogin || '?').slice(0, 1).toUpperCase()
  return (
    <a
      href={ban.sourceUrl || '#'}
      target={ban.sourceUrl ? '_blank' : undefined}
      rel={ban.sourceUrl ? 'noreferrer' : undefined}
      className="flex gap-3 rounded-lg border border-[#3A2426] bg-[#160F11] p-3 transition hover:border-[#FF5C57]/45"
    >
      {avatarSrc ? (
        <img src={avatarSrc} alt="" className="h-11 w-11 shrink-0 rounded-full object-cover ring-2 ring-[#FF5C57]/25" loading="lazy" />
      ) : (
        <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full bg-[#2A1515] text-sm font-bold text-[#FFB5B2] ring-2 ring-[#FF5C57]/25">
          {initials}
        </div>
      )}
      <div className="min-w-0 flex-1">
        <div className="flex items-center justify-between gap-2">
          <span className="rounded-full border border-[#FF5C57]/35 bg-[#2A1515] px-2 py-0.5 text-[10px] font-bold uppercase text-[#FFB5B2]">
            Ban event
          </span>
          <span className="text-[11px] text-[#7A7A85]">{formatRelativeTime(ban.occurredAt)}</span>
        </div>
        <h4 className="mt-2 line-clamp-2 text-sm font-semibold leading-5 text-[#F7F7F8]">{ban.headline}</h4>
        <p className="mt-2 text-[11px] font-semibold text-[#7A7A85]">
          {ban.streamerDisplayName || ban.streamerLogin} · {ban.source}
        </p>
      </div>
    </a>
  )
}

function TrendingLead({
  data,
  window,
  flair,
  lead,
}: {
  data: TrendingNewsState
  window: PulseWireWindow
  flair: string | null
  lead?: TrendingLeadPick | null
}) {
  const picked = lead ?? pickTrendingLead(data)

  if (!picked) {
    return (
      <div className="rounded-xl border border-[#2A2A2E] bg-[#121217] p-5">
        <p className="text-sm text-[#ADADB8]">
          {flair
            ? `No ${flair} threads in ${windowShortLabel(window)} yet.`
            : `No headlines in ${windowShortLabel(window)} yet. Trending fills from Reddit threads, top clips, and ban news as sources publish.`}
        </p>
      </div>
    )
  }

  if (picked.kind === 'ban') {
    const banLead = picked.ban
    const banAvatar = pulseWireDisplayThumbnail(banLead.displayThumbnailUrl ?? banLead.previewUrl)
    return (
      <article className="rounded-xl border border-[#3A2426] bg-[#160F11] p-5">
        <div className="flex items-start gap-4">
          {banAvatar ? (
            <img src={banAvatar} alt="" className="h-16 w-16 shrink-0 rounded-full object-cover ring-2 ring-[#FF5C57]/30" loading="lazy" />
          ) : (
            <div className="flex h-16 w-16 shrink-0 items-center justify-center rounded-full bg-[#2A1515] text-xl font-black text-[#FFB5B2] ring-2 ring-[#FF5C57]/30">
              {(banLead.streamerDisplayName || banLead.streamerLogin || '?').slice(0, 1).toUpperCase()}
            </div>
          )}
          <div className="min-w-0 flex-1">
            <span className={`inline-flex rounded-full border px-2 py-1 text-[11px] font-bold uppercase ${sourceTone('ban event')}`}>Ban event</span>
            <h2 className="mt-3 text-2xl font-black leading-tight text-[#F7F7F8]">{banLead.headline}</h2>
            <p className="mt-3 text-sm text-[#ADADB8]">{banLead.streamerDisplayName || banLead.streamerLogin} · {formatRelativeTime(banLead.occurredAt)}</p>
          </div>
        </div>
      </article>
    )
  }

  if (picked.kind === 'clip') {
    return <ClipNewsCard clip={picked.clip} />
  }

  const communityLead = picked.post
  const previewKind = communityLead.previewKind ?? (communityLead.displayThumbnailUrl ? 'fallback' : 'none')
  const heroSrc = communityLead.displayThumbnailUrl ? pulseWireDisplayThumbnail(communityLead.displayThumbnailUrl) : undefined
  const subreddit = communityLead.subreddit ? `r/${communityLead.subreddit.replace(/^r\//, '')}` : 'LivestreamFail'
  const threadUrl = communityLead.permalink || communityLead.url
  return (
    <a
      data-testid="trending-lead"
      href={threadUrl}
      target="_blank"
      rel="noreferrer"
      className="group block overflow-hidden rounded-xl border border-[#2A2A2E] bg-[#121217] transition hover:border-[#A970FF]/40"
    >
      {heroSrc ? (
        <div className="relative aspect-[21/9] bg-[#1B1B1F]">
          <img src={heroSrc} alt="" className="h-full w-full object-cover transition duration-300 group-hover:scale-105" loading="lazy" />
          <span className={`absolute left-4 top-4 inline-flex rounded-full border px-2 py-1 text-[11px] font-bold uppercase ${sourceTone('LSF hot')}`}>LSF hot</span>
        </div>
      ) : null}
      <div className="p-5">
        {!heroSrc ? (
          <span className={`inline-flex rounded-full border px-2 py-1 text-[11px] font-bold uppercase ${sourceTone('LSF hot')}`}>LSF hot</span>
        ) : null}
        {!heroSrc && previewKind === 'fallback' ? (
          <p className="mt-3 text-xs font-semibold uppercase tracking-wide text-[#7A7A85]">{subreddit} · Text post</p>
        ) : null}
        <h2 className={`${heroSrc ? 'mt-0' : 'mt-3'} text-2xl font-black leading-tight text-[#F7F7F8]`}>{communityLead.title}</h2>
        {hasEngagementCounts(communityLead.score, communityLead.comments) ? (
          <p className="mt-3 text-sm text-[#ADADB8]">
            {formatEngagementCount(communityLead.score)} upvotes · {formatEngagementCount(communityLead.comments)} comments
            {communityLead.streamerLogin ? ` · ${communityLead.streamerDisplayName || communityLead.streamerLogin}` : ''}
          </p>
        ) : communityLead.streamerLogin ? (
          <p className="mt-3 text-sm text-[#ADADB8]">{communityLead.streamerDisplayName || communityLead.streamerLogin}</p>
        ) : null}
      </div>
    </a>
  )
}

function TrendingNewsPage({
  window,
  flair,
  refreshKey,
  sourceHealth,
}: {
  window: PulseWireWindow
  flair: string | null
  refreshKey: number
  sourceHealth: PulseWireSourceHealth
}) {
  const [data, setData] = useState<TrendingNewsState>({ community: [], clips: [], bans: [] })
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError('')
    Promise.allSettled([
      fetchPulseWireCommunity({ window, sort: 'hot', flair: flair ?? undefined, limit: 18 }),
      fetchTopClips({ window, limit: 12 }),
      fetchPulseWireBans({ window, limit: 8 }),
    ]).then(([communityRes, clipsRes, bansRes]) => {
      if (cancelled) return
      setData({
        community: communityRes.status === 'fulfilled' ? communityRes.value.items ?? [] : [],
        clips: clipsRes.status === 'fulfilled' ? clipsRes.value.items ?? [] : [],
        bans: bansRes.status === 'fulfilled' ? bansRes.value.items ?? [] : [],
      })
      const failures = [communityRes, clipsRes, bansRes].filter(res => res.status === 'rejected')
      setError(failures.length === 3 ? 'Trending sources are unavailable right now.' : '')
    }).finally(() => {
      if (!cancelled) setLoading(false)
    })
    return () => { cancelled = true }
  }, [window, flair, refreshKey])

  const lead = pickTrendingLead(data)
  const usedCommunityId = lead?.kind === 'community' ? lead.post.id : null
  const usedClipId = lead?.kind === 'clip' ? lead.clip.id : null
  const moreCommunity = data.community.filter(post => post.id !== usedCommunityId).slice(0, 8)
  const clipRow = data.clips.filter(clip => clip.id !== usedClipId).slice(0, 8)
  const communityEmptyHint = flair
    ? `No ${flair} threads in ${windowShortLabel(window)} yet.`
    : `No Reddit threads in ${windowShortLabel(window)} yet.`

  if (loading) {
    return <TrendingPageSkeleton />
  }

  if (error) {
    return <div className="rounded-xl border border-amber-400/30 bg-amber-500/10 p-4 text-sm text-amber-100">{error}</div>
  }

  const leadTitle = lead?.kind === 'clip'
    ? 'Top clip'
    : lead?.kind === 'ban'
      ? 'Ban story'
      : flair
        ? `${flair} on Reddit`
        : 'Hot on Reddit'
  const leadSubtitle = flair
    ? `Top ${flair} threads in ${windowShortLabel(window)}.`
    : `Lead story for ${windowShortLabel(window)} — thread, clip, or ban with the strongest signal.`

  return (
    <div className="space-y-6">
      <NewsSection eyebrow="Trending" title={leadTitle} subtitle={leadSubtitle}>
        <TrendingLead data={data} window={window} flair={flair} lead={lead} />
      </NewsSection>

      <NewsSection
        eyebrow="Clips"
        title="Top clips"
        subtitle={`Most-viewed Twitch clips in ${windowShortLabel(window)}.`}
      >
        <TrendingClipsScroller clips={clipRow} />
      </NewsSection>

      {data.bans.length ? (
        <NewsSection title="Latest partner bans" subtitle="Official Twitch partner bans from StreamerBans.">
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {data.bans.slice(0, 3).map(ban => <BanEventCard key={ban.id} ban={ban} />)}
          </div>
        </NewsSection>
      ) : sourceHealth.streamerbans?.mode === 'off' ? (
        <div className="rounded-lg border border-[#3A2A12] bg-[#151208] p-3 text-sm text-[#FFE0A3]">
          StreamerBans ingest is off — enable STREAMERBANS_INGEST_ENABLED to load partner bans from streamerbans.com.
        </div>
      ) : null}

      <NewsSection
        eyebrow="Reddit"
        title="More community threads"
        subtitle={flair ? `${flair} threads ranked by heat and recency.` : 'Reddit discussion ranked by heat and recency.'}
      >
        <div className="grid gap-3 md:grid-cols-2">
          {moreCommunity.length ? moreCommunity.map(post => <CommunityPostCard key={post.id} post={post} />) : (
            <p className="rounded-lg border border-[#2A2A2E] bg-[#121217] p-4 text-sm text-[#ADADB8]">
              {communityEmptyHint}
            </p>
          )}
        </div>
      </NewsSection>
    </div>
  )
}

function OperatorDrawer({
  id,
  developing,
  sourceHealth,
  stories,
  analystMode,
  onConfirmed,
  open,
  onOpenChange,
}: {
  id: string
  developing: PulseWireStory[]
  sourceHealth: PulseWireSourceHealth
  stories: PulseWireStory[]
  analystMode: boolean
  onConfirmed: () => void
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const views = stories.map(story => toWireStoryView(story, sourceHealth, { analystMode }))
  const waitingForOrigin = views.filter(view => view.readerStatus === 'needs_origin' || view.missingEvidence.some(item => /origin/i.test(item))).length
  const missingKeySources = views.filter(view => view.missingEvidence.length > 0).length
  const lowConfidence = views.filter(view => view.confidenceLabel === 'Low' || view.confidenceLabel === 'Insufficient data').length

  return (
    <div id={id} className="rounded-xl border border-[#2A2A2E] bg-[#121217]">
      <button
        type="button"
        onClick={() => onOpenChange(!open)}
        className="flex w-full items-center justify-between gap-2 px-4 py-3 text-left text-sm font-semibold text-[#ADADB8] transition hover:text-[#EFEFF1] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
        aria-expanded={open}
      >
        <span>Operator tools</span>
        <span className="text-xs text-[#7A7A85]">{open ? 'Hide' : 'Show'}</span>
      </button>
      {open ? (
        <div className="space-y-4 border-t border-[#2A2A2E] p-4">
          <DevelopingPanel items={developing} onConfirmed={onConfirmed} />
          <SourceHealthPanel sources={sourceHealth} />
          <div className="rounded-lg border border-[#24242B] bg-[#101014] p-3">
            <p className="text-[11px] font-black uppercase tracking-[0.06em] text-[#7A7A85]">Missing evidence queue</p>
            <div className="mt-2 space-y-2">
              {[
                { label: 'Stories waiting for origin match', value: waitingForOrigin },
                { label: 'Stories missing key sources', value: missingKeySources },
                { label: 'Low confidence items', value: lowConfidence },
              ].map(row => (
                <div key={row.label} className="flex items-center justify-between gap-2 text-xs">
                  <span className="text-[#ADADB8]">{row.label}</span>
                  <span className="font-bold text-[#F7F7F8]">{row.value}</span>
                </div>
              ))}
            </div>
            <p className="mt-3 text-[11px] leading-relaxed text-[#7A7A85]">
              Analyst queue counts from visible cross-platform stories.
            </p>
          </div>
        </div>
      ) : null}
    </div>
  )
}

export default function PulseWirePage() {
  const location = useLocation()
  const navigate = useNavigate()
  const { storyId } = useParams()
  const [searchParams, setSearchParams] = useSearchParams()
  const windowRange = parseWindow(searchParams.get('window'))
  const activeLogin = searchParams.get('login') ?? ''
  const activeSearch = searchParams.get('q') ?? ''
  const activeTab = storyId ? 'wire' : parseTab(searchParams.get('tab'))
  const activeFlair = parseTrendingFlair(searchParams.get('flair'))
  const trendingWindow: PulseWireWindow = activeFlair ? '7d' : windowRange
  const analystMode = readAnalystMode(searchParams)
  const [analystModeEnabled, setAnalystModeEnabled] = useState(analystMode)
  const pulseWireSearch = searchParams.toString()
  const pulseWireSearchSuffix = pulseWireSearch ? `?${pulseWireSearch}` : ''

  const [feed, setFeed] = useState<PulseWireStory[]>([])
  const [hero, setHero] = useState<PulseWireStory | null>(null)
  const [trending, setTrending] = useState<PulseWireTrendingStreamer[]>([])
  const [developing, setDeveloping] = useState<PulseWireStory[]>([])
  const [bans, setBans] = useState<PulseWireBanEvent[]>([])
  const [banError, setBanError] = useState('')
  const [sourceHealth, setSourceHealth] = useState<PulseWireSourceHealth>({})
  const [feedSince, setFeedSince] = useState<string | undefined>()
  const [rankModel, setRankModel] = useState<PulseWireRankModel | undefined>()
  const [chip, setChip] = useState<PulseWireFilterChip>(() => parseChip(searchParams))
  const [sort, setSort] = useState<PulseWireFeedSort>(() => parseSort(searchParams.get('sort')))
  const [nextCursor, setNextCursor] = useState<PulseWireCursor | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [warming, setWarming] = useState(false)
  const [disabledHint, setDisabledHint] = useState('')
  const [error, setError] = useState('')
  const [refreshNonce, setRefreshNonce] = useState(0)
  const [refreshing, setRefreshing] = useState(false)
  const [trendingFlairs, setTrendingFlairs] = useState<PulseWireCommunityFlair[]>([])
  const [trendingFlairsLoading, setTrendingFlairsLoading] = useState(false)
  const [operatorOpen, setOperatorOpen] = useState(false)
  const [allFeedCount, setAllFeedCount] = useState<number | null>(null)
  const [unlinked, setUnlinked] = useState<PulseWireUnlinkedEvidence[]>([])
  const [watchEntries, setWatchEntries] = useState<PulseWireWatchEntry[]>([])
  const [pendingMissingEvidenceFocus, setPendingMissingEvidenceFocus] = useState<number | null>(null)
  const prevLoadDeps = useRef({ storyId, feedParamsKey: '', refreshNonce: 0, windowRange, activeLogin })

  const feedParams = useMemo(
    () => ({
      ...chipToFeedParams(chip, sort),
      login: activeLogin || undefined,
      window: windowRange,
    }),
    [chip, sort, activeLogin, windowRange],
  )
  const feedParamsKey = useMemo(() => JSON.stringify(feedParams), [feedParams])
  const searchActive = Boolean(normalizeSearchQuery(activeSearch))
  const filtersActive = chip !== 'all' || sort !== 'rank' || windowRange !== '24h' || Boolean(activeLogin) || searchActive

  useEffect(() => {
    const params = new URLSearchParams(searchParams)
    let changed = false
    if (!params.get('window')) {
      params.set('window', '24h')
      changed = true
    }
    if (params.get('view')) {
      params.delete('view')
      changed = true
    }
    if (changed) {
      setSearchParams(params, { replace: true })
    }
  }, [searchParams, setSearchParams])

  useEffect(() => {
    const nextChip = parseChip(searchParams)
    const nextSort = parseSort(searchParams.get('sort'))
    setChip(prev => (prev === nextChip ? prev : nextChip))
    setSort(prev => (prev === nextSort ? prev : nextSort))
  }, [searchParams])

  useEffect(() => {
    setSearchParams(prev => {
      const params = new URLSearchParams(prev)
      writeChipParams(params, chip)
      if (sort === 'rank') params.delete('sort')
      else params.set('sort', sort)
      if (!params.get('window')) params.set('window', '24h')
      return params
    }, { replace: true })
  }, [chip, sort, setSearchParams])

  const setWindow = useCallback((next: PulseWireWindow) => {
    setSearchParams(prev => {
      const params = new URLSearchParams(prev)
      if (next === '24h') params.delete('window')
      else params.set('window', next)
      return params
    })
  }, [setSearchParams])

  const setLoginFilter = useCallback((login: string) => {
    setSearchParams(prev => {
      const params = new URLSearchParams(prev)
      if (!login) params.delete('login')
      else params.set('login', login)
      return params
    })
  }, [setSearchParams])

  const setTab = useCallback((tab: PulseWireTab) => {
    setSearchParams(prev => {
      const params = new URLSearchParams(prev)
      if (tab === 'wire') params.set('tab', 'wire')
      else params.delete('tab')
      return params
    })
  }, [setSearchParams])

  const setTrendingFlair = useCallback((flair: string | null) => {
    setSearchParams(prev => {
      const params = new URLSearchParams(prev)
      if (flair) {
        params.set('flair', flair)
        params.set('window', '7d')
      } else {
        params.delete('flair')
      }
      return params
    })
  }, [setSearchParams])

  const setSearchFilter = useCallback((query: string) => {
    setSearchParams(prev => {
      const params = new URLSearchParams(prev)
      const trimmed = query.trim()
      if (trimmed) params.set('q', trimmed)
      else params.delete('q')
      params.set('tab', 'wire')
      if (!params.get('window')) params.set('window', '24h')
      return params
    })
  }, [setSearchParams])

  const setSavedFilter = useCallback(() => {
    setChip('saved')
    setSearchParams(prev => {
      const params = new URLSearchParams(prev)
      writeChipParams(params, 'saved')
      if (!params.get('window')) params.set('window', '24h')
      params.set('tab', 'wire')
      return params
    })
  }, [setSearchParams])

  const saveWatchEntry = useCallback(async (kind: PulseWireWatchEntry['kind'], value: string, label?: string) => {
    const result = await addWatchEntry(kind, value, label)
    setWatchEntries(prev => {
      const next = prev.filter(item => !(item.kind === result.item.kind && item.value === result.item.value))
      return [result.item, ...next]
    })
  }, [])

  const removeWatchEntry = useCallback(async (id: number) => {
    await deleteWatchEntry(id)
    setWatchEntries(prev => prev.filter(item => item.id !== id))
  }, [])

  const applyWatchEntry = useCallback((entry: PulseWireWatchEntry) => {
    if (entry.kind === 'category') {
      setChip(entry.value as PulseWireFilterChip)
      setSearchParams(prev => {
        const params = new URLSearchParams(prev)
        writeChipParams(params, entry.value as PulseWireFilterChip)
        params.set('tab', 'wire')
        params.delete('q')
        if (!params.get('window')) params.set('window', '24h')
        return params
      })
      return
    }
    setSearchFilter(entry.value)
  }, [setSearchFilter, setSearchParams])

  const markStoryTracked = useCallback((storyID: number, tracked: boolean) => {
    const apply = (item: PulseWireStory) => (
      item.story.id === storyID ? { ...item, tracked } : item
    )
    setFeed(prev => prev.map(apply))
    setHero(prev => (prev ? apply(prev) : prev))
  }, [])

  async function reloadStoryDetail() {
    if (!storyId) return
    const story = await fetchPulseWireStory(Number(storyId), { window: windowRange })
    setHero(story)
  }

  useEffect(() => {
    setAnalystModeEnabled(readAnalystMode(searchParams))
  }, [searchParams])

  const setAnalystMode = useCallback((enabled: boolean) => {
    writeAnalystMode(enabled)
    setAnalystModeEnabled(enabled)
    setSearchParams(prev => {
      const params = new URLSearchParams(prev)
      if (enabled) params.set('analyst', '1')
      else params.delete('analyst')
      return params
    }, { replace: true })
  }, [setSearchParams])

  const hasCrossPlatformStories = useMemo(
    () => feed.some(item => isCrossPlatformStory(item, sourceHealth)),
    [feed, sourceHealth],
  )
  const showCrossPlatformTab = hasCrossPlatformStories || activeTab === 'wire' || Boolean(storyId)

  const rankedFeed = useMemo(() => {
    let items = sortFeed(feed, feedParams.sort)
    if (chip === 'high_volatility') {
      items = items.filter(item => (item.scores.volatility ?? 0) >= 50)
    } else if (chip === 'saved') {
      items = items.filter(item => item.tracked)
    }
    items = filterStoriesBySearch(items, activeSearch)
    if (activeTab === 'wire' && !storyId) {
      items = items.filter(item => isCrossPlatformStory(item, sourceHealth))
    }
    return items
  }, [feed, feedParams.sort, chip, activeSearch, activeTab, storyId, sourceHealth])

  const visibleHero = useMemo(() => {
    if (storyId) return hero
    if (chip === 'saved' || searchActive || activeTab === 'wire') return rankedFeed[0] ?? null
    return hero
  }, [storyId, hero, chip, searchActive, rankedFeed, activeTab])

  const feedItems = useMemo(() => {
    if (!visibleHero) return rankedFeed
    return rankedFeed.filter(item => item.story.id !== visibleHero.story.id)
  }, [rankedFeed, visibleHero])
  const railStories = useMemo(() => (visibleHero ? [visibleHero, ...feedItems] : feedItems), [visibleHero, feedItems])
  const followedStories = useMemo(() => {
    const seen = new Set<number>()
    const stories: PulseWireStory[] = []
    for (const item of [hero, ...feed]) {
      if (!item?.tracked || seen.has(item.story.id)) continue
      seen.add(item.story.id)
      stories.push(item)
    }
    return sortFeed(stories, feedParams.sort)
  }, [hero, feed, feedParams.sort])
  const activeWatchCategory = useMemo(() => {
    return ['drama', 'funny', 'bans', 'records', 'esports', 'unverified'].includes(chip) ? chip : ''
  }, [chip])

  const showWireFeed = activeTab === 'wire' || Boolean(storyId)
  useEffect(() => {
    const wantsMissingEvidence = location.hash === '#missing-evidence' || pendingMissingEvidenceFocus === hero?.story.id
    if (!storyId || !hero || typeof window === 'undefined' || !wantsMissingEvidence) return
    const scrollIntoReadingPosition = (target: HTMLElement) => {
      target.scrollIntoView({ block: 'start' })
      const stickyHeaderOffset = 96
      const top = target.getBoundingClientRect().top + window.scrollY - stickyHeaderOffset
      window.scrollTo({ top: Math.max(0, top), behavior: 'auto' })

      let parent = target.parentElement
      while (parent && parent !== document.body) {
        const style = window.getComputedStyle(parent)
        const canScroll = /(auto|scroll|overlay)/.test(style.overflowY) && parent.scrollHeight > parent.clientHeight
        if (canScroll) {
          const parentRect = parent.getBoundingClientRect()
          const targetRect = target.getBoundingClientRect()
          parent.scrollTop += targetRect.top - parentRect.top - 16
          break
        }
        parent = parent.parentElement
      }
    }
    const focusTarget = () => {
      const target = document.getElementById('missing-evidence')
      if (!target) return
      if (window.location.hash !== '#missing-evidence') {
        window.history.replaceState(null, '', `${window.location.pathname}${window.location.search}#missing-evidence`)
      }
      scrollIntoReadingPosition(target)
      target.focus({ preventScroll: true })
    }
    const first = window.setTimeout(focusTarget, 50)
    const second = window.setTimeout(() => {
      focusTarget()
      setPendingMissingEvidenceFocus(null)
    }, 200)
    const third = window.setTimeout(focusTarget, 500)
    return () => {
      window.clearTimeout(first)
      window.clearTimeout(second)
      window.clearTimeout(third)
    }
  }, [storyId, hero, location.hash, pendingMissingEvidenceFocus])

  useEffect(() => {
    setNextCursor(null)
  }, [feedParamsKey, windowRange, activeLogin])

  useEffect(() => {
    if (!showWireFeed) setLoading(false)
  }, [showWireFeed])

  useEffect(() => {
    if (activeTab !== 'trending' || storyId) return
    const controller = new AbortController()
    fetchSourceHealth(controller.signal)
      .then(health => {
        if (!controller.signal.aborted) {
          setSourceHealth(health.sources ?? {})
        }
      })
      .catch(err => {
        if (!(err instanceof DOMException && err.name === 'AbortError')) {
          setSourceHealth({})
        }
      })
    return () => controller.abort()
  }, [activeTab, storyId, refreshNonce])

  useEffect(() => {
    if (activeTab !== 'trending' || storyId) return
    const controller = new AbortController()
    setTrendingFlairsLoading(true)
    fetchPulseWireCommunityFlairs({ window: '7d', limit: 20, signal: controller.signal })
      .then(res => {
        if (!controller.signal.aborted) {
          setTrendingFlairs(res.items ?? [])
        }
      })
      .catch(err => {
        if (!(err instanceof DOMException && err.name === 'AbortError')) {
          setTrendingFlairs([])
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setTrendingFlairsLoading(false)
        }
      })
    return () => controller.abort()
  }, [activeTab, storyId, refreshNonce])

  useEffect(() => {
    const abortController = new AbortController()
    const { signal } = abortController
    let cancelled = false
    let pollTimer: ReturnType<typeof setInterval> | null = null
    let warmStartedAt = 0

    const prev = prevLoadDeps.current
    const paramsChanged =
      prev.storyId !== storyId ||
      prev.feedParamsKey !== feedParamsKey ||
      prev.windowRange !== windowRange ||
      prev.activeLogin !== activeLogin
    const refreshTriggered = prev.refreshNonce !== refreshNonce
    prevLoadDeps.current = { storyId, feedParamsKey, refreshNonce, windowRange, activeLogin }

    const isRefreshOnly = refreshTriggered && !paramsChanged
    const showInitialLoading = paramsChanged || !refreshTriggered

    const clearPoll = () => {
      if (pollTimer) {
        clearInterval(pollTimer)
        pollTimer = null
      }
    }

    const isStale = () => cancelled || signal.aborted

    function applyFeedResponse(feedRes: Awaited<ReturnType<typeof fetchPulseWireFeed>>, append: boolean) {
      const nextFeed = feedRes.items ?? []
      setFeed(prev => (append ? [...prev, ...nextFeed] : nextFeed))
      setNextCursor(responseCursor(feedRes))
      setFeedSince(typeof feedRes.since === 'string' ? feedRes.since : feedRes.since != null ? String(feedRes.since) : undefined)
      setRankModel(feedRes.rankModel)
      return nextFeed
    }

    async function loadFeedOnly() {
      try {
        const feedRes = await fetchPulseWireFeed({ ...feedParams, signal })
        if (isStale()) return
        const nextFeed = applyFeedResponse(feedRes, false)
        if (isStale()) return
        if (!storyId) {
          setHero(sortFeed(nextFeed, feedParams.sort)[0] ?? null)
        }
        const shouldWarm = !filtersActive && nextFeed.length === 0
        setWarming(shouldWarm)
        if (!shouldWarm) clearPoll()
      } catch (e) {
        if (isStale()) return
        if (e instanceof DOMException && e.name === 'AbortError') return
        setError(e instanceof Error ? e.message : 'Failed to load Pulse Wire')
        setWarming(false)
        clearPoll()
      }
    }

    const scheduleWarmPoll = () => {
      clearPoll()
      warmStartedAt = Date.now()
      pollTimer = setInterval(() => {
        if (Date.now() - warmStartedAt >= WARM_CAP_MS) {
          clearPoll()
          return
        }
        void loadFeedOnly()
      }, WARM_POLL_MS)
    }

    async function load(isInitial: boolean, pageCursor: PulseWireCursor | null, append: boolean) {
      if (isInitial) {
        setLoading(true)
        setError('')
        setDisabledHint('')
      } else if (append) {
        setLoadingMore(true)
      } else if (refreshNonce > 0) {
        setRefreshing(true)
      }
      try {
        const [feedRes, devRes, healthRes, streamerRes, watchRes, bansRes] = await Promise.all([
          fetchPulseWireFeed({ ...feedParams, cursor: pageCursor, signal }),
          fetchDeveloping(signal),
          fetchSourceHealth(signal),
          fetchTrendingStreamers({ window: windowRange, limit: 10, signal }),
          fetchWatchEntries(signal),
          fetchPulseWireBans({ window: windowRange, limit: 5, signal })
            .then(res => {
              setBanError('')
              return res
            })
            .catch(err => {
              if (err instanceof DOMException && err.name === 'AbortError') throw err
              setBanError(err instanceof Error ? err.message : 'Ban feed unavailable')
              return { items: [] as PulseWireBanEvent[] }
            }),
        ])
        if (isStale()) return
        const nextFeed = applyFeedResponse(feedRes, append)
        if (filtersActive && !append && nextFeed.length === 0) {
          const allRes = await fetchPulseWireFeed({ sort: feedParams.sort, window: windowRange, signal })
          if (!isStale()) setAllFeedCount(allRes.items?.length ?? 0)
        } else if (!isStale() && !append) {
          setAllFeedCount(null)
        }
        if (!filtersActive && !append && nextFeed.length === 0) {
          const unlinkedRes = await fetchPulseWireUnlinkedEvidence({ window: windowRange, limit: 12, signal })
          if (!isStale()) setUnlinked(unlinkedRes.items ?? [])
        } else if (!isStale() && !append) {
          setUnlinked([])
        }
        setTrending(streamerRes.items ?? [])
        setWatchEntries(watchRes.items ?? [])
        setDeveloping((devRes as { items?: PulseWireStory[] }).items ?? [])
        setBans(bansRes.items ?? [])
        setSourceHealth(healthRes.sources ?? {})
        if (storyId) {
          const story = await fetchPulseWireStory(Number(storyId), { window: windowRange, signal })
          if (!isStale()) setHero(story)
        } else if (!append) {
          setHero(sortFeed(nextFeed, feedParams.sort)[0] ?? null)
        }
        const shouldWarm = !filtersActive && nextFeed.length === 0 && !append
        setWarming(shouldWarm)
        if (shouldWarm && !pollTimer) {
          scheduleWarmPoll()
        } else if (!shouldWarm) {
          clearPoll()
        }
      } catch (e) {
        if (isStale()) return
        if (e instanceof DOMException && e.name === 'AbortError') return
        if (e instanceof PulseWireApiError && e.code === 'pulse_wire_disabled') {
          setDisabledHint(e.hint ?? 'Set PULSE_WIRE_ENABLED=true in .env and restart Streamclone.')
          setError('')
          setWarming(false)
          clearPoll()
          return
        }
        setError(e instanceof Error ? e.message : 'Failed to load Pulse Wire')
        setWarming(false)
        clearPoll()
      } finally {
        if (!isStale()) {
          if (isInitial) setLoading(false)
          if (append) setLoadingMore(false)
          if (isRefreshOnly && !append) setRefreshing(false)
        }
      }
    }

    if (isRefreshOnly) {
      setRefreshing(true)
      void load(false, null, false)
    } else if (showInitialLoading && showWireFeed) {
      void load(true, null, false)
    }

    return () => {
      cancelled = true
      abortController.abort()
      clearPoll()
    }
  }, [storyId, feedParams, feedParamsKey, refreshNonce, filtersActive, windowRange, activeLogin, activeTab, showWireFeed])

  async function loadMore() {
    if (!hasCursor(nextCursor) || loadingMore) return
    setLoadingMore(true)
    try {
      const feedRes = await fetchPulseWireFeed({ ...feedParams, cursor: nextCursor })
      setFeed(prev => [...prev, ...(feedRes.items ?? [])])
      setNextCursor(responseCursor(feedRes))
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load more stories')
    } finally {
      setLoadingMore(false)
    }
  }

  return (
    <DirectoryLayout
      headerSubtitle="Pulse Wire"
      showBrowseLink
      showPulseWireLink={PULSE_WIRE_ENABLED}
      pulseWireActive
    >
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_280px]">
        <section className="min-w-0">
          <div className="mb-4 flex flex-col gap-4 border-b border-[#202027] sm:flex-row sm:items-center sm:justify-between">
            <div className="flex flex-wrap gap-2" role="tablist" aria-label="Pulse Wire views">
              <button
                type="button"
                role="tab"
                aria-selected={activeTab === 'trending'}
                onClick={() => setTab('trending')}
                className={`border-b-2 px-3 pb-3 pt-1 text-sm font-semibold transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] ${
                  activeTab === 'trending'
                    ? 'border-[#A970FF] text-[#EFEFF1]'
                    : 'border-transparent text-[#ADADB8] hover:border-[#3A3A40] hover:text-[#EFEFF1]'
                }`}
              >
                Trending
              </button>
              {showCrossPlatformTab ? (
                <button
                  type="button"
                  role="tab"
                  aria-selected={activeTab === 'wire'}
                  aria-label="Cross-platform stories (Advanced)"
                  onClick={() => setTab('wire')}
                  className={`border-b-2 px-3 pb-3 pt-1 text-sm font-semibold transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] ${
                    activeTab === 'wire'
                      ? 'border-[#A970FF] text-[#EFEFF1]'
                      : 'border-transparent text-[#ADADB8] hover:border-[#3A3A40] hover:text-[#EFEFF1]'
                  }`}
                >
                  Cross-platform
                  <span className="ml-1.5 text-[10px] font-semibold uppercase tracking-wide text-[#7A7A85]">Advanced</span>
                </button>
              ) : null}
            </div>
          </div>
          {activeTab === 'trending' && !storyId ? (
            <>
              {disabledHint ? (
                <p className="mb-4 rounded-lg border border-amber-400/30 bg-amber-500/10 p-3 text-sm text-amber-100">
                  Pulse Wire is disabled on this install. {disabledHint}
                </p>
              ) : null}
              <TrendingSourceHealthRow sources={sourceHealth} />
              <header className="mb-4 rounded-xl border border-[#24242B] bg-[#101014] p-4">
                <h2 className="text-lg font-black text-[#F7F7F8]">{windowEditionTitle(trendingWindow, 'trending')}</h2>
                <p className="mt-1 text-sm text-[#ADADB8]">{windowTagline(trendingWindow, 'trending')}</p>
              </header>
              <TrendingFlairChips
                flairs={trendingFlairs}
                activeFlair={activeFlair}
                loading={trendingFlairsLoading}
                onSelect={setTrendingFlair}
              />
              <TrendingNewsPage
                window={trendingWindow}
                flair={activeFlair}
                refreshKey={refreshNonce}
                sourceHealth={sourceHealth}
              />
            </>
          ) : (
            <>
          <PulseWireEditionHeader
            mode="wire"
            window={windowRange}
            since={feedSince}
            rankModel={rankModel}
            refreshing={refreshing}
            loading={loading}
            disabled={Boolean(disabledHint)}
            onWindowChange={setWindow}
            onRefresh={() => setRefreshNonce(value => value + 1)}
          />
          <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
            <p className="text-xs text-[#7A7A85]">Stories with two or more attached sources in this window.</p>
            <button
              type="button"
              aria-pressed={analystModeEnabled}
              onClick={() => setAnalystMode(!analystModeEnabled)}
              className={`rounded-lg border px-3 py-1.5 text-xs font-semibold transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] ${
                analystModeEnabled
                  ? 'border-[#FFE0A3]/50 bg-[#21190D] text-[#FFE0A3]'
                  : 'border-[#2A2A2E] bg-[#111116] text-[#ADADB8] hover:border-[#3A3A40] hover:text-[#EFEFF1]'
              }`}
            >
              {analystModeEnabled ? 'Analyst gaps on' : 'Show analyst gaps'}
            </button>
          </div>
          <PulseWireFilters
            chip={chip}
            sort={sort}
            activeLogin={activeLogin}
            searchQuery={activeSearch}
            wireFriendly
            onChipChange={next => {
              setChip(next)
              if (next === 'high_volatility') setSort('volatility')
            }}
            onSortChange={setSort}
            onClearLogin={() => setLoginFilter('')}
            onSearchChange={setSearchFilter}
          />
          {activeTab === 'wire' || storyId ? (
            <details className="mb-4 rounded-xl border border-[#2A2A2E] bg-[#0C0C0F] lg:hidden">
              <summary className="cursor-pointer px-4 py-3 text-sm font-semibold text-[#EFEFF1] marker:text-[#A970FF]">
                Reader rail
              </summary>
              <div className="space-y-4 border-t border-[#202027] p-3">
                <WireReaderRail
                  sourceHealth={sourceHealth}
                  trending={trending}
                  stories={railStories}
                  followedStories={followedStories}
                  watchEntries={watchEntries}
                  bans={bans}
                  banError={banError}
                  window={windowRange}
                  activeLogin={activeLogin}
                  activeCategory={activeWatchCategory}
                  searchQuery={activeSearch}
                  onSelectLogin={setLoginFilter}
                  onSelectWatchEntry={applyWatchEntry}
                  onAddWatchEntry={saveWatchEntry}
                  onDeleteWatchEntry={removeWatchEntry}
                  savedOnly={chip === 'saved'}
                  onShowSaved={setSavedFilter}
                  onOpenOperatorTools={() => setOperatorOpen(true)}
                />
                <OperatorDrawer
                  id="pulse-wire-operator-tools-mobile"
                  developing={developing}
                  sourceHealth={sourceHealth}
                  stories={railStories}
                  analystMode={analystModeEnabled}
                  open={operatorOpen}
                  onOpenChange={setOperatorOpen}
                  onConfirmed={() => setRefreshNonce(value => value + 1)}
                />
              </div>
            </details>
          ) : null}
          {disabledHint ? (
            <p className="mb-4 rounded-lg border border-amber-400/30 bg-amber-500/10 p-3 text-sm text-amber-100">
              Pulse Wire is disabled on this install. {disabledHint}
            </p>
          ) : null}
          {error ? <p className="mb-4 rounded-lg border border-red-400/30 bg-red-500/10 p-3 text-sm">{error}</p> : null}
          {loading ? <FeedSkeleton /> : null}
          {!loading && visibleHero && !storyId ? (
            <div className={`pulse-wire-card-enter mb-6 transition-opacity ${refreshing ? 'opacity-60' : ''}`}>
              <LeadStoryDesk
                story={visibleHero}
                sourceHealth={sourceHealth}
                analystMode={analystModeEnabled}
                onOpen={() => navigate(`/pulse-wire/${visibleHero.story.id}${pulseWireSearchSuffix}`)}
                missingEvidenceHref={`/pulse-wire/${visibleHero.story.id}${pulseWireSearchSuffix}#missing-evidence`}
                onReviewMissingEvidence={() => {
                  setPendingMissingEvidenceFocus(visibleHero.story.id)
                  navigate(`/pulse-wire/${visibleHero.story.id}${pulseWireSearchSuffix}#missing-evidence`)
                }}
                onTrackedChange={tracked => markStoryTracked(visibleHero.story.id, tracked)}
              />
            </div>
          ) : null}
          {!loading && storyId && hero ? (
            <div className={`mb-6 transition-opacity ${refreshing ? 'opacity-60' : ''}`}>
              <PulseWireStoryDetail
                story={hero}
                sourceHealth={sourceHealth}
                analystMode={analystModeEnabled}
                onAnalystModeChange={setAnalystMode}
                onAdded={() => void reloadStoryDetail()}
                onCollapse={() => navigate(`/pulse-wire${pulseWireSearchSuffix}`)}
              />
            </div>
          ) : null}
          {!loading && !error && !disabledHint && !visibleHero && !feedItems.length && !unlinked.length ? (
            <div className="rounded-2xl border border-[#2A2A2E] bg-[#121217] p-5 text-sm text-[#ADADB8]">
              {warming && !filtersActive ? (
                <p>
                  Warming Pulse Wire — first stories usually appear within a minute after startup.
                  Checking again every 30 seconds…
                </p>
              ) : filtersActive ? (
                <div className="space-y-2">
                  {allFeedCount != null && allFeedCount > 0 ? (
                    <p>
                      Stories exist on the wire, but none match{' '}
                      <span className="font-semibold text-[#EFEFF1]">{chipLabel(chip)}</span>
                      {activeSearch ? (
                        <>
                          {' '}for <span className="font-semibold text-[#EFEFF1]">"{activeSearch}"</span>
                        </>
                      ) : null}
                      {activeLogin ? (
                        <>
                          {' '}for <span className="font-semibold text-[#EFEFF1]">{activeLogin}</span>
                        </>
                      ) : null}{' '}
                      in {windowShortLabel(windowRange)} yet.
                    </p>
                  ) : (
                    <p>
                      No stories match your filters in {windowShortLabel(windowRange)} yet.
                    </p>
                  )}
                  <p className="text-xs text-[#7A7A85]">
                    Try <span className="font-semibold text-[#ADADB8]">All</span>, widen the window, or tap{' '}
                    <span className="font-semibold text-[#ADADB8]">Refresh</span> after ingest runs.
                  </p>
                </div>
              ) : activeTab === 'wire' && !storyId && !hasCrossPlatformStories ? (
                <div className="space-y-3">
                  <p>
                    No cross-platform stories in {windowShortLabel(windowRange)} yet. Browse Trending for hot threads and clips.
                  </p>
                  <button
                    type="button"
                    onClick={() => setTab('trending')}
                    className="rounded-lg border border-[#2A2A2E] bg-[#1B1B1F] px-3 py-2 text-xs font-semibold text-[#EFEFF1] hover:border-[#A970FF]/40"
                  >
                    Back to Trending
                  </button>
                </div>
              ) : (
                <p>No cross-platform stories in {windowShortLabel(windowRange)} yet. They appear when ingest clusters evidence from multiple sources.</p>
              )}
            </div>
          ) : null}
          {!loading && !error && !disabledHint && !visibleHero && !feedItems.length && unlinked.length > 0 ? (
            <WireUnlinkedPanel items={unlinked} className="mb-6" />
          ) : null}
          {!loading ? (
            <div className={`pulse-wire-stagger transition-opacity ${refreshing ? 'opacity-60' : ''}`}>
              <WireStoryLanes stories={feedItems} sourceHealth={sourceHealth} detailSearch={pulseWireSearchSuffix} analystMode={analystModeEnabled} />
            </div>
          ) : null}
          {!loading && hasCursor(nextCursor) ? (
            <div className="mt-6 flex justify-center">
              <button
                type="button"
                onClick={() => void loadMore()}
                disabled={loadingMore}
                className="rounded-lg border border-[#2A2A2E] bg-[#1B1B1F] px-4 py-2 text-xs font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/40 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] disabled:opacity-50"
              >
                {loadingMore ? 'Loading…' : 'Load more'}
              </button>
            </div>
          ) : null}
            </>
          )}
        </section>
        {activeTab === 'wire' || storyId ? (
        <aside className={`hidden space-y-4 transition-opacity lg:block lg:bg-[#0C0C0F] lg:pl-1 ${refreshing ? 'opacity-60' : ''}`}>
          <WireReaderRail
            sourceHealth={sourceHealth}
            trending={trending}
            stories={railStories}
            followedStories={followedStories}
            watchEntries={watchEntries}
            bans={bans}
            banError={banError}
            window={windowRange}
            activeLogin={activeLogin}
            activeCategory={activeWatchCategory}
            searchQuery={activeSearch}
            onSelectLogin={setLoginFilter}
            onSelectWatchEntry={applyWatchEntry}
            onAddWatchEntry={saveWatchEntry}
            onDeleteWatchEntry={removeWatchEntry}
            savedOnly={chip === 'saved'}
            onShowSaved={setSavedFilter}
            onOpenOperatorTools={() => setOperatorOpen(true)}
          />
          <OperatorDrawer
            id="pulse-wire-operator-tools-desktop"
            developing={developing}
            sourceHealth={sourceHealth}
            stories={railStories}
            analystMode={analystModeEnabled}
            open={operatorOpen}
            onOpenChange={setOperatorOpen}
            onConfirmed={() => setRefreshNonce(value => value + 1)}
          />
        </aside>
        ) : null}
      </div>
    </DirectoryLayout>
  )
}
