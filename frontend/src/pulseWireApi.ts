import { METADATA, SETUP_CONTROL_TOKEN } from './config'
import { normalizeChannelSpreadResponse } from './utils/pulseWireSpread'

const API_BASE = METADATA

export class PulseWireApiError extends Error {
  status: number
  code?: string
  hint?: string

  constructor(message: string, status: number, code?: string, hint?: string) {
    super(message)
    this.name = 'PulseWireApiError'
    this.status = status
    this.code = code
    this.hint = hint
  }
}

async function pulseWireJson<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body = await res.json().catch(() => ({})) as { error?: string; hint?: string }
    throw new PulseWireApiError(
      body.error || `HTTP ${res.status}`,
      res.status,
      body.error,
      body.hint,
    )
  }
  return res.json() as Promise<T>
}

export type PulseWireWindow = 'today' | '24h' | '7d'
export type PulseWireCursor = number | string
export type PulseWireFeedSort = 'rank' | 'updated' | 'volatility'
export type PulseWireCommunitySort = 'hot' | 'new'
export type PulseWireRankModel = 'breaking' | 'daily_wire' | 'weekly_recap' | 'legacy' | string

export type PulseWireWatchEntry = {
  id: number
  kind: 'category' | 'keyword'
  value: string
  label?: string
  createdAt?: string
}

export type PulseWireScores = {
  trend: number | null
  volatility: number | null
  confidence: 'single_source' | 'corroborated' | 'widely_reported' | null
  sentiment: number | null
  updatedAt?: string
}

export type PulseWireWindowScores = {
  window?: PulseWireWindow
  since?: string
  evidenceCount?: number
  sourceCount?: number
  recentSourceDelta?: number
  velocityScore?: number | null
  credibilityScore?: number | null
  impactScore?: number | null
  momentumScore?: number | null
  freshnessScore?: number | null
  rankScore?: number | null
  dominantSource?: string | null
  computedAt?: string
  status?: 'ready' | 'fallback' | 'stale' | string
}

export type PulseWireReceipt = {
  sourceType: string
  risk?: string
  pct: number
  url?: string
  previewStatus?: string
  previewId?: number
  thumbnailUrl?: string
  label?: string
  occurredAt?: string
}

export type PulseWireTimelineStep = {
  at: string
  sourceType: string
  label: string
  sourceUrl?: string
}

export type PulseWireEvidencePreview = {
  id?: number
  canonicalUrl: string
  platform: string
  providerName?: string
  title?: string
  author?: string
  thumbnailUrl?: string
  embedUrl?: string
  embedHtml?: string
  createdAtSrc?: string
  fetchedAt?: string
  httpStatus?: number
  error?: string
  expiresAt?: string
  previewStatus: 'ready' | 'fallback' | 'error' | 'pending' | string
  matchKind?: string
  note?: string
}

export type PulseWireMatchExplanation = {
  sourceType: string
  matchedBy: string
  confidence: number
  author?: string
  factors?: string[]
  previewStatus?: string
  sourceUrl?: string
  previewId?: number
  evidenceId?: number
}

export type PulseWireSpreadBackfillState = 'idle' | 'warming' | 'ready' | string

export type PulseWireSpreadBackfillMeta = {
  state?: PulseWireSpreadBackfillState
  requestedAt?: string
}

export type PulseWireChannelSpreadMeta = {
  entityKnown?: boolean
  aliases?: string[]
  lastIngestAt?: string
  unresolvedMentionCount?: number
  backfill?: PulseWireSpreadBackfillMeta
}

export type PulseWireChannelSpreadResponse = {
  login: string
  items: PulseWireStory[]
  probableItems?: PulseWireStory[]
  meta?: PulseWireChannelSpreadMeta
}

export type PulseWireSourceHealth = Record<string, {
  mode: 'active' | 'link_only' | 'off' | 'error' | 'degraded' | 'deferred' | string
  healthy?: boolean
  last_poll_at?: string
  last_ok_at?: string
  last_err_at?: string
  last_error?: string
  last_items?: number
  last_evidence_at?: string
  hint?: string
  details?: Record<string, {
    healthy?: boolean
    last_poll_at?: string
    last_ok_at?: string
    last_err_at?: string
    last_error?: string
    last_items?: number
  }>
}>

export type PulseWireTrendingStreamer = {
  login: string
  displayName: string
  storyCount: number
  evidenceCount?: number
  sourceDiversity?: number
  volatility?: number | null
  lastSeen: string
}

export type PulseWireCommunityFlair = {
  flair: string
  count?: number
}

export type PulseWireCommunityPost = {
  id: number
  externalId?: string
  title: string
  url: string
  permalink?: string
  source: string
  subreddit?: string
  score: number
  comments: number
  thumbnailUrl?: string
  displayThumbnailUrl?: string
  previewKind?: 'reddit' | 'twitch' | 'youtube' | 'fallback' | 'none' | string
  previewUrl?: string
  selfText?: string
  embedUrl?: string
  embedHtml?: string
  linkedPlatform?: string
  streamerLogin?: string
  streamerDisplayName?: string
  flair?: string
  category?: string
  postedAt?: string
}

export type PulseWireUnlinkedEvidence = {
  id: number
  title: string
  url: string
  source: string
  category?: string
  score?: number
  comments?: number
  viewCount?: number
  previewKind?: string
  previewUrl?: string
  thumbnailUrl?: string
  displayThumbnailUrl?: string
  streamerLogin?: string
  streamerDisplayName?: string
  postedAt?: string
}

export type PulseWireBanEvent = {
  id: number
  streamerLogin: string
  streamerDisplayName?: string
  eventType: string
  platform?: string
  source: string
  headline: string
  sourceUrl?: string
  occurredAt?: string
  confidence?: number
  previewKind?: string
  previewUrl?: string
  thumbnailUrl?: string
  displayThumbnailUrl?: string
}

export type PulseWireTopClip = {
  id: number
  externalId?: string
  title: string
  url: string
  viewCount: number
  durationSeconds?: number
  thumbnailUrl?: string
  displayThumbnailUrl?: string
  streamerLogin?: string
  streamerDisplayName?: string
  postedAt?: string
}

export type PulseWireOriginEmote = {
  id?: string
  name: string
  provider?: string
  count?: number
}

export type PulseWireOriginSpikePoint = {
  offsetS: number
  relativeS: number
  chatCount: number
  totalEmoteCount: number
  sevenTvEmoteCount?: number
  viewerMax?: number
}

export type PulseWireStory = {
  story: {
    id: number
    title?: string
    summary?: string
    category?: string
    storyClass?: string
    state: string
    originSearchStatus?: 'matched' | 'searched_missing' | 'searched_no_fingerprints' | string
    originCheckedAt?: string
    createdAt?: string
    updatedAt?: string
  }
  entity?: { id?: number; login?: string; displayName?: string; avatarUrl?: string }
  origin?: {
    id?: number
    streamId: string
    vodOffsetS: number
    vodId?: string
    quotes?: string[]
    topEmotes?: PulseWireOriginEmote[]
    chatSpikeSummary?: string
    originConfidence?: number
    originSpikePoints?: PulseWireOriginSpikePoint[]
  }
  scores: PulseWireScores
  windowScores?: PulseWireWindowScores
  receipts?: PulseWireReceipt[]
  windowReceipts?: PulseWireReceipt[]
  timeline?: PulseWireTimelineStep[]
  windowTimeline?: PulseWireTimelineStep[]
  firstSeenAt?: string
  lastSeenAt?: string
  evidenceGallery?: PulseWireEvidencePreview[]
  matchExplanation?: PulseWireMatchExplanation[]
  operatorActions?: PulseWireOperatorAction[]
  tracked?: boolean
  matchTier?: 'probable' | string
}

export type PulseWireOperatorAction = {
  id: number
  clusterId: number
  action: string
  operator: string
  note?: string
  beforeData?: Record<string, unknown>
  afterData?: Record<string, unknown>
  createdAt?: string
}

export type PulseWireFeedResponse = {
  items: PulseWireStory[]
  cursor?: PulseWireCursor | null
  nextCursor?: PulseWireCursor | null
  window?: PulseWireWindow
  since?: string
  sort?: PulseWireFeedSort
  rankModel?: PulseWireRankModel
}

export type PulseWireViewerPoint = {
  at?: string
  sampledAt?: string
  viewers: number
  rank?: number | null
}

export type PulseWireStoryLink = {
  id: number
  title?: string
  category?: string | null
  state?: string
}

export type PulseWireRisingStreamer = {
  login: string
  displayName?: string
  avatarUrl?: string
  category?: string | null
  viewersNow?: number | null
  viewersPrev?: number | null
  statsSource?: 'directory_sample' | 'metadata_fallback' | 'none' | string
  lastSampleAt?: string | null
  viewerDeltaPct?: number | null
  rankNow?: number | null
  rankPrev?: number | null
  rankDelta?: number | null
  newEntrant?: boolean
  clipVelocity?: number | null
  risingScore?: number | null
  sparkline?: PulseWireViewerPoint[]
  viewerSeries?: PulseWireViewerPoint[]
  topStory?: PulseWireStoryLink | null
  topStoryId?: number | null
  topStoryTitle?: string | null
}

export type PulseWireRisingResponse = {
  items: PulseWireRisingStreamer[]
  window?: PulseWireWindow
  since?: string
  sampleStatus?: string
  lastSampleAt?: string
}

export type PulseWireStreamerProfile = {
  login: string
  displayName?: string
  avatarUrl?: string
  category?: string | null
  isLive?: boolean
  rankNow?: number | null
  rankPrev?: number | null
  rankDelta?: number | null
  viewersNow?: number | null
  viewersPrev?: number | null
  viewerDeltaPct?: number | null
  followersNow?: number | null
  followerDelta?: number | null
  followerSampled?: boolean
  clipVelocity?: number | null
  risingScore?: number | null
  newEntrant?: boolean
  viewerSeries?: PulseWireViewerPoint[]
  recentStories?: PulseWireStory[]
  rising?: PulseWireRisingStreamer
  window?: PulseWireWindow
  since?: string
  updatedAt?: string
}

export type PulseWireStreamerProfileResponse = {
  profile: PulseWireStreamerProfile
  window?: PulseWireWindow
  since?: string
}

export type PulseWireEditionKpi = {
  label: string
  value: string
  hint?: string
}

export type PulseWireEditionSection = {
  id: string
  title: string
  subtitle?: string
  stories?: PulseWireStory[]
  movers?: PulseWireRisingStreamer[]
  kpis?: PulseWireEditionKpi[]
}

export type PulseWireEditionResponse = {
  window?: PulseWireWindow
  since?: string
  rankModel?: PulseWireRankModel
  generatedAt?: string
  date?: string
  totalLive?: number | null
  totalViewers?: number | null
  sections?: PulseWireEditionSection[]
  topGainers?: PulseWireRisingStreamer[]
  topDroppers?: PulseWireRisingStreamer[]
  newEntrants?: PulseWireRisingStreamer[]
  bansOfTheDay?: PulseWireStory[]
  bans?: PulseWireStory[]
  topStories?: PulseWireStory[]
}

function normalizeViewerSeries(points?: PulseWireViewerPoint[]): PulseWireViewerPoint[] | undefined {
  if (!points?.length) return points
  return points.map(point => ({
    ...point,
    at: point.at ?? point.sampledAt,
  }))
}

function normalizeRisingStreamer(row: PulseWireRisingStreamer): PulseWireRisingStreamer {
  const sparkline = normalizeViewerSeries(row.sparkline ?? row.viewerSeries)
  const topStory = row.topStory ?? (row.topStoryId
    ? { id: row.topStoryId, title: row.topStoryTitle ?? undefined }
    : null)
  return { ...row, sparkline, topStory }
}

function normalizeRisingResponse(res: PulseWireRisingResponse): PulseWireRisingResponse {
  return {
    ...res,
    items: (res.items ?? []).map(normalizeRisingStreamer),
  }
}

function normalizeStreamerProfile(
  raw: PulseWireStreamerProfile,
  meta?: { window?: PulseWireWindow; since?: string },
): PulseWireStreamerProfile {
  const rising = raw.rising
  return {
    ...raw,
    window: meta?.window ?? raw.window,
    since: meta?.since ?? raw.since,
    viewersPrev: raw.viewersPrev ?? rising?.viewersPrev ?? null,
    viewerDeltaPct: raw.viewerDeltaPct ?? rising?.viewerDeltaPct ?? null,
    rankPrev: raw.rankPrev ?? rising?.rankPrev ?? null,
    rankDelta: raw.rankDelta ?? rising?.rankDelta ?? null,
    newEntrant: raw.newEntrant ?? rising?.newEntrant,
    clipVelocity: raw.clipVelocity ?? rising?.clipVelocity ?? null,
    risingScore: raw.risingScore ?? rising?.risingScore ?? null,
    viewerSeries: normalizeViewerSeries(raw.viewerSeries),
    followerSampled: raw.followerSampled ?? raw.followersNow != null,
  }
}

function unwrapStreamerPayload(
  data: PulseWireStreamerProfile | PulseWireStreamerProfileResponse,
): PulseWireStreamerProfile {
  if ('profile' in data && data.profile) {
    return normalizeStreamerProfile(data.profile, {
      window: data.window,
      since: data.since,
    })
  }
  return normalizeStreamerProfile(data as PulseWireStreamerProfile)
}

function defaultRankModel(window: PulseWireWindow): PulseWireRankModel {
  switch (window) {
    case 'today':
      return 'breaking'
    case '7d':
      return 'weekly_recap'
    default:
      return 'daily_wire'
  }
}

function dailyToEdition(daily: PulseWireEditionResponse, window: PulseWireWindow): PulseWireEditionResponse {
  const gainers = (daily.topGainers ?? []).map(normalizeRisingStreamer)
  const droppers = (daily.topDroppers ?? []).map(normalizeRisingStreamer)
  const entrants = (daily.newEntrants ?? []).map(normalizeRisingStreamer)
  const bans = daily.bansOfTheDay ?? []
  const topStories = daily.topStories ?? []

  const sections: PulseWireEditionSection[] = []
  if (window === 'today') {
    sections.push(
      {
        id: 'breaking',
        title: 'Breaking now',
        subtitle: 'Fresh evidence and velocity',
        stories: topStories.slice(0, 3),
      },
      {
        id: 'biggest_gainer',
        title: 'Biggest gainer',
        movers: gainers.slice(0, 1),
      },
      {
        id: 'new_entrants',
        title: 'New entrants',
        movers: entrants.slice(0, 3),
      },
    )
  } else if (window === '7d') {
    sections.push(
      {
        id: 'weekly_risers',
        title: 'Weekly risers',
        movers: gainers.slice(0, 5),
      },
      {
        id: 'most_covered',
        title: 'Most covered',
        stories: topStories.slice(0, 3),
      },
      {
        id: 'resolved',
        title: 'Settled stories',
        stories: topStories.filter(item => item.story.state === 'settled').slice(0, 3),
      },
    )
  } else {
    sections.push(
      {
        id: 'top_corroborated',
        title: 'Top corroborated',
        stories: topStories.filter(item => item.scores.confidence !== 'single_source').slice(0, 3),
      },
      {
        id: 'fastest_spreading',
        title: 'Fastest spreading',
        stories: [...topStories]
          .sort((a, b) => (b.scores.volatility ?? 0) - (a.scores.volatility ?? 0))
          .slice(0, 3),
      },
      {
        id: 'bans',
        title: 'Bans & moderation',
        stories: bans.slice(0, 3),
      },
    )
  }

  return {
    ...daily,
    window,
    rankModel: defaultRankModel(window),
    sections,
    topGainers: gainers,
    topDroppers: droppers,
    newEntrants: entrants,
  }
}

function wireEditionToSections(data: PulseWireEditionResponse, window: PulseWireWindow): PulseWireEditionSection[] | undefined {
  const raw = data as PulseWireEditionResponse & {
    breaking?: PulseWireStory[]
    topCorroborated?: PulseWireStory[]
    biggestMover?: PulseWireRisingStreamer
    bans?: PulseWireStory[]
    fastestSpreading?: PulseWireStory[]
    weeklyRecap?: PulseWireStory[]
  }
  if (!raw.breaking?.length && !raw.topCorroborated?.length && !raw.bans?.length && !raw.fastestSpreading?.length && !raw.weeklyRecap?.length && !raw.biggestMover && !raw.newEntrants?.length && !raw.topGainers?.length) {
    return undefined
  }
  const gainers = (raw.topGainers ?? []).map(normalizeRisingStreamer)
  const entrants = (raw.newEntrants ?? []).map(normalizeRisingStreamer)
  if (window === 'today') {
    return [
      { id: 'breaking', title: 'Breaking now', subtitle: 'Fresh evidence and velocity', stories: raw.breaking?.slice(0, 3) },
      { id: 'biggest_gainer', title: 'Biggest gainer', movers: raw.biggestMover ? [normalizeRisingStreamer(raw.biggestMover)] : gainers.slice(0, 1) },
      { id: 'new_entrants', title: 'New entrants', movers: entrants.slice(0, 3) },
    ]
  }
  if (window === '7d') {
    return [
      { id: 'weekly_risers', title: 'Weekly risers', movers: gainers.slice(0, 5) },
      { id: 'most_covered', title: 'Most covered', stories: raw.topCorroborated?.slice(0, 3) ?? raw.breaking?.slice(0, 3) },
      { id: 'resolved', title: 'Settled stories', stories: raw.weeklyRecap?.slice(0, 3) },
    ]
  }
  return [
    { id: 'top_corroborated', title: 'Top corroborated', stories: raw.topCorroborated?.slice(0, 3) },
    { id: 'fastest_spreading', title: 'Fastest spreading', stories: raw.fastestSpreading?.slice(0, 3) },
    { id: 'bans', title: 'Bans & moderation', stories: raw.bans?.slice(0, 3) },
  ]
}

function normalizeEdition(data: PulseWireEditionResponse, window: PulseWireWindow): PulseWireEditionResponse {
  if (data.sections?.length) {
    return {
      ...data,
      window: data.window ?? window,
      rankModel: data.rankModel ?? defaultRankModel(window),
      sections: data.sections.map(section => ({
        ...section,
        stories: section.stories,
        movers: section.movers?.map(normalizeRisingStreamer),
      })),
      topGainers: data.topGainers?.map(normalizeRisingStreamer),
      topDroppers: data.topDroppers?.map(normalizeRisingStreamer),
      newEntrants: data.newEntrants?.map(normalizeRisingStreamer),
    }
  }
  const wireSections = wireEditionToSections(data, window)
  if (wireSections?.length) {
    return {
      ...data,
      window: data.window ?? window,
      rankModel: data.rankModel ?? defaultRankModel(window),
      sections: wireSections,
      topGainers: data.topGainers?.map(normalizeRisingStreamer),
      topDroppers: data.topDroppers?.map(normalizeRisingStreamer),
      newEntrants: data.newEntrants?.map(normalizeRisingStreamer),
    }
  }
  return dailyToEdition(data, window)
}

export function storyReceipts(story: PulseWireStory): PulseWireReceipt[] | undefined {
  return story.windowReceipts?.length ? story.windowReceipts : story.receipts
}

export function storyTimeline(story: PulseWireStory): PulseWireTimelineStep[] | undefined {
  return story.windowTimeline?.length ? story.windowTimeline : story.timeline
}

export function storyUpdatedAt(story: PulseWireStory): string | undefined {
  return story.lastSeenAt ?? story.story.updatedAt ?? story.scores.updatedAt ?? story.windowScores?.computedAt
}

export function effectiveScores(story: PulseWireStory): PulseWireScores {
  const ws = story.windowScores
  if (!ws) return story.scores
  return {
    ...story.scores,
    trend: ws.rankScore ?? story.scores.trend,
    volatility: ws.velocityScore ?? story.scores.volatility,
  }
}

export {
  formatChannelMatchReason,
  isSpreadBackfillWarming,
  matchConfidenceLabel,
  normalizeChannelSpreadResponse,
} from './utils/pulseWireSpread'

export async function fetchPulseWireFeed(params?: {
  state?: string
  category?: string
  login?: string
  cursor?: PulseWireCursor | null
  sort?: PulseWireFeedSort
  window?: PulseWireWindow
  signal?: AbortSignal
}) {
  const q = new URLSearchParams()
  if (params?.state) q.set('state', params.state)
  if (params?.category) q.set('category', params.category)
  if (params?.login) q.set('login', params.login)
  if (params?.cursor != null && params.cursor !== '') q.set('cursor', String(params.cursor))
  if (params?.window) q.set('window', params.window)
  if (params?.sort) q.set('sort', params.sort)
  const res = await fetch(`${API_BASE}/v1/pulse-wire/feed?${q}`, { signal: params?.signal })
  const data = await pulseWireJson<PulseWireFeedResponse>(res)
  return {
    ...data,
    rankModel: data.rankModel ?? defaultRankModel(data.window ?? params?.window ?? '24h'),
  }
}

export async function fetchTrendingStreamers(params?: { window?: PulseWireWindow; limit?: number; signal?: AbortSignal }) {
  const q = new URLSearchParams()
  if (params?.window) q.set('window', params.window)
  if (params?.limit) q.set('limit', String(params.limit))
  const suffix = q.toString() ? `?${q}` : ''
  const res = await fetch(`${API_BASE}/v1/pulse-wire/trending-streamers${suffix}`, { signal: params?.signal })
  return pulseWireJson<{ items: PulseWireTrendingStreamer[]; window?: PulseWireWindow; since?: string }>(res)
}

export async function fetchPulseWireCommunity(params?: {
  window?: PulseWireWindow
  sort?: PulseWireCommunitySort
  category?: string
  flair?: string
  limit?: number
  signal?: AbortSignal
}) {
  const q = new URLSearchParams()
  if (params?.window) q.set('window', params.window)
  if (params?.sort) q.set('sort', params.sort)
  if (params?.category) q.set('category', params.category)
  if (params?.flair) q.set('flair', params.flair)
  if (params?.limit) q.set('limit', String(params.limit))
  const suffix = q.toString() ? `?${q}` : ''
  const res = await fetch(`${API_BASE}/v1/pulse-wire/community${suffix}`, { signal: params?.signal })
  return pulseWireJson<{
    items: PulseWireCommunityPost[]
    window?: PulseWireWindow
    since?: string
    sort?: PulseWireCommunitySort
    flair?: string
  }>(res)
}

export async function fetchPulseWireCommunityFlairs(params?: {
  window?: PulseWireWindow
  limit?: number
  signal?: AbortSignal
}) {
  const q = new URLSearchParams()
  if (params?.window) q.set('window', params.window)
  if (params?.limit) q.set('limit', String(params.limit))
  const suffix = q.toString() ? `?${q}` : ''
  const res = await fetch(`${API_BASE}/v1/pulse-wire/community/flairs${suffix}`, { signal: params?.signal })
  return pulseWireJson<{
    items: PulseWireCommunityFlair[]
    window?: PulseWireWindow
  }>(res)
}

export async function fetchPulseWireUnlinkedEvidence(params?: { window?: PulseWireWindow; limit?: number; signal?: AbortSignal }) {
  const q = new URLSearchParams()
  if (params?.window) q.set('window', params.window)
  if (params?.limit) q.set('limit', String(params.limit))
  const suffix = q.toString() ? `?${q}` : ''
  const res = await fetch(`${API_BASE}/v1/pulse-wire/evidence/unlinked${suffix}`, { signal: params?.signal })
  return pulseWireJson<{
    items: PulseWireUnlinkedEvidence[]
    window?: PulseWireWindow
    since?: string
  }>(res)
}

export async function fetchPulseWireBans(params?: { window?: PulseWireWindow; limit?: number; signal?: AbortSignal }) {
  const q = new URLSearchParams()
  if (params?.window) q.set('window', params.window)
  if (params?.limit) q.set('limit', String(params.limit))
  const suffix = q.toString() ? `?${q}` : ''
  const res = await fetch(`${API_BASE}/v1/pulse-wire/bans${suffix}`, { signal: params?.signal })
  return pulseWireJson<{
    items: PulseWireBanEvent[]
    window?: PulseWireWindow
    since?: string
  }>(res)
}

export async function fetchTopClips(params?: { window?: PulseWireWindow; limit?: number }) {
  const q = new URLSearchParams()
  if (params?.window) q.set('window', params.window)
  if (params?.limit) q.set('limit', String(params.limit))
  const suffix = q.toString() ? `?${q}` : ''
  const res = await fetch(`${API_BASE}/v1/pulse-wire/clips/top${suffix}`)
  return pulseWireJson<{ items: PulseWireTopClip[]; window?: PulseWireWindow; since?: string }>(res)
}

export async function fetchPulseWireStory(id: number, params?: { window?: PulseWireWindow; signal?: AbortSignal }) {
  const suffix = params?.window ? `?window=${params.window}` : ''
  const res = await fetch(`${API_BASE}/v1/pulse-wire/stories/${id}${suffix}`, { signal: params?.signal })
  return pulseWireJson<PulseWireStory>(res)
}

export async function fetchRising(window: PulseWireWindow = '24h') {
  const res = await fetch(`${API_BASE}/v1/pulse-wire/rising?window=${window}`)
  return pulseWireJson(res)
}

export async function fetchDeveloping(signal?: AbortSignal) {
  const res = await fetch(`${API_BASE}/v1/pulse-wire/developing`, { signal })
  return pulseWireJson(res)
}

export async function fetchSourceMix(window: PulseWireWindow = '24h', signal?: AbortSignal) {
  const res = await fetch(`${API_BASE}/v1/pulse-wire/source-mix?window=${window}`, { signal })
  return pulseWireJson<{ mix?: Record<string, number>; window?: PulseWireWindow; since?: string }>(res)
}

export async function fetchSourceHealth(signal?: AbortSignal) {
  const res = await fetch(`${API_BASE}/v1/pulse-wire/source-health`, { signal })
  return pulseWireJson<{ sources: PulseWireSourceHealth }>(res)
}

export async function fetchChannelSpread(login: string, signal?: AbortSignal) {
  const res = await fetch(`${API_BASE}/v1/channels/${encodeURIComponent(login)}/spread`, { signal })
  const data = await pulseWireJson<PulseWireChannelSpreadResponse>(res)
  return normalizeChannelSpreadResponse(data)
}

export async function requestChannelSpreadBackfill(login: string) {
  const res = await fetch(`${API_BASE}/v1/channels/${encodeURIComponent(login)}/spread/backfill`, { method: 'POST' })
  if (res.status === 202) {
    const body = await res.json().catch(() => ({})) as { state?: string }
    return { state: (body.state ?? 'warming') as PulseWireSpreadBackfillState }
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({})) as { error?: string; hint?: string }
    throw new PulseWireApiError(body.error || `backfill ${res.status}`, res.status, body.error, body.hint)
  }
  return res.json() as Promise<{ state: PulseWireSpreadBackfillState }>
}

export async function fetchRisingStreamers(params?: {
  window?: PulseWireWindow
  category?: string
  limit?: number
}) {
  const q = new URLSearchParams()
  if (params?.window) q.set('window', params.window)
  if (params?.category) q.set('category', params.category)
  if (params?.limit) q.set('limit', String(params.limit))
  const suffix = q.toString() ? `?${q}` : ''
  const res = await fetch(`${API_BASE}/v1/pulse-wire/rising-streamers${suffix}`)
  return normalizeRisingResponse(await pulseWireJson<PulseWireRisingResponse>(res))
}

export async function fetchPulseWireStreamer(login: string, params?: { window?: PulseWireWindow }) {
  const q = new URLSearchParams()
  if (params?.window) q.set('window', params.window)
  const suffix = q.toString() ? `?${q}` : ''
  const res = await fetch(`${API_BASE}/v1/pulse-wire/streamers/${encodeURIComponent(login)}${suffix}`)
  const data = await pulseWireJson<PulseWireStreamerProfile | PulseWireStreamerProfileResponse>(res)
  return unwrapStreamerPayload(data)
}

/** @deprecated Use fetchPulseWireStreamer */
export async function fetchStreamerProfile(login: string, params?: { window?: PulseWireWindow }) {
  return fetchPulseWireStreamer(login, params)
}

export async function fetchPulseWireEdition(window: PulseWireWindow = '24h') {
  const res = await fetch(`${API_BASE}/v1/pulse-wire/edition?window=${window}`)
  if (res.status === 404) {
    const dailyRes = await fetch(`${API_BASE}/v1/pulse-wire/daily`)
    if (!dailyRes.ok) {
      throw new PulseWireApiError('edition unavailable', dailyRes.status)
    }
    const daily = await dailyRes.json() as PulseWireEditionResponse
    return normalizeEdition(daily, window)
  }
  const data = await pulseWireJson<PulseWireEditionResponse>(res)
  return normalizeEdition(data, window)
}

/** @deprecated Use fetchPulseWireEdition for window-aware editions */
export async function fetchPulseWireDaily(date?: string) {
  const suffix = date ? `?date=${encodeURIComponent(date)}` : ''
  const res = await fetch(`${API_BASE}/v1/pulse-wire/daily${suffix}`)
  return pulseWireJson<PulseWireEditionResponse>(res)
}

export async function followStory(id: number) {
  const res = await fetch(`${API_BASE}/v1/pulse-wire/stories/${id}/follow`, { method: 'POST' })
  if (!res.ok) throw new Error(`follow ${res.status}`)
  return res.json()
}

export async function unfollowStory(id: number) {
  const res = await fetch(`${API_BASE}/v1/pulse-wire/stories/${id}/follow`, { method: 'DELETE' })
  if (!res.ok) throw new Error(`unfollow ${res.status}`)
  return res.json()
}

export async function fetchWatchEntries(signal?: AbortSignal) {
  const res = await fetch(`${API_BASE}/v1/pulse-wire/watch-entries`, { signal })
  return pulseWireJson<{ items?: PulseWireWatchEntry[] }>(res)
}

export async function addWatchEntry(kind: PulseWireWatchEntry['kind'], value: string, label?: string) {
  const res = await fetch(`${API_BASE}/v1/pulse-wire/watch-entries`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ kind, value, label }),
  })
  return pulseWireJson<{ status: string; item: PulseWireWatchEntry }>(res)
}

export async function deleteWatchEntry(id: number) {
  const res = await fetch(`${API_BASE}/v1/pulse-wire/watch-entries/${id}`, { method: 'DELETE' })
  return pulseWireJson<{ status: string }>(res)
}

export async function createClipStory(id: number) {
  const headers: Record<string, string> = {}
  if (SETUP_CONTROL_TOKEN) headers['X-Streamclone-Setup-Token'] = SETUP_CONTROL_TOKEN
  const res = await fetch(`${API_BASE}/v1/pulse-wire/stories/${id}/clip`, { method: 'POST', headers })
  if (!res.ok) throw new Error(`clip ${res.status}`)
  return res.json() as Promise<{ status?: string; clipUrl?: string }>
}

export async function addEvidenceStory(id: number, url: string, note?: string) {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (SETUP_CONTROL_TOKEN) headers['X-Streamclone-Setup-Token'] = SETUP_CONTROL_TOKEN
  const res = await fetch(`${API_BASE}/v1/pulse-wire/stories/${id}/evidence`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ url, note }),
  })
  const body = await res.json().catch(() => ({})) as {
    error?: string
    hint?: string
    status?: string
    preview?: PulseWireEvidencePreview
  }
  if (!res.ok) {
    throw new PulseWireApiError(body.error || `add evidence ${res.status}`, res.status, body.error, body.hint)
  }
  return body as { status: string; preview?: PulseWireEvidencePreview }
}

export type PulseWireOperatorActionName =
  | 'mark_not_news'
  | 'mark_community_meta'
  | 'mark_debunked'
  | 'manual_suppress'
  | 'confirm_streamer_entity'
  | 'confirm_origin_moment'
  | 'merge_duplicate_story'
  | 'split_unrelated_evidence'

export async function markPulseWireStory(
  id: number,
  action: PulseWireOperatorActionName,
  note?: string,
  options?: { entityId?: number; momentFpId?: number; targetClusterId?: number; evidenceIds?: number[]; title?: string },
) {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (SETUP_CONTROL_TOKEN) headers['X-Streamclone-Setup-Token'] = SETUP_CONTROL_TOKEN
  const res = await fetch(`${API_BASE}/v1/pulse-wire/stories/${id}/operator-action`, {
    method: 'POST',
    headers,
    body: JSON.stringify({
      action,
      note,
      entityId: options?.entityId,
      momentFpId: options?.momentFpId,
      targetClusterId: options?.targetClusterId,
      evidenceIds: options?.evidenceIds,
      title: options?.title,
    }),
  })
  const body = await res.json().catch(() => ({})) as {
    error?: string
    hint?: string
    status?: string
    action?: PulseWireOperatorAction
    story?: PulseWireStory
  }
  if (!res.ok) {
    throw new PulseWireApiError(body.error || `operator action ${res.status}`, res.status, body.error, body.hint)
  }
  return body as { status: string; action: PulseWireOperatorAction; story?: PulseWireStory }
}

export async function confirmDevelopingStory(id: number, action: 'confirm' | 'merge' | 'reject' = 'confirm') {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (SETUP_CONTROL_TOKEN) headers['X-Streamclone-Setup-Token'] = SETUP_CONTROL_TOKEN
  const res = await fetch(`${API_BASE}/v1/pulse-wire/developing/${id}/confirm`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ action }),
  })
  if (!res.ok) throw new Error(`confirm ${res.status}`)
  return res.json() as Promise<{ status: string; action: string }>
}
