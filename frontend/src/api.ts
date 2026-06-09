import { ANALYTICS, CHAT_HTTP, EMOTE, METADATA, VIDEO, CLIPPER, CLIPPER_TOKEN } from './config'
import type { ClipPeriod, StatsPeriod } from './settings'

export class ApiError extends Error {
  status: number
  code?: string
  retryable?: boolean

  constructor(message: string, status: number, code?: string, retryable?: boolean) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.retryable = retryable
  }
}

export interface Stream {
  id?: string
  login: string
  displayName?: string
  title: string
  category: string
  viewers: number
  thumbnailUrl: string
  isLive?: boolean
  profileImageUrl?: string
}

export interface Category {
  id: string
  name: string
  thumbnailUrl: string
}

export interface ChannelInfo {
  id: string
  login: string
  displayName: string
}

export interface SourceStatus {
  source: string
  state: 'ready' | 'fallback' | 'unavailable' | 'blocked' | 'error' | 'limited'
  message?: string
  provider?: string
  backoffUntil?: number
}

export interface AnalyticsWatchResponse {
  channel: string
  tracking: boolean
  active: number
  max: number
  message?: string
  sources: SourceStatus[]
}

export interface AnalyticsStream {
  streamId: string
  broadcasterId: string
  login: string
  displayName?: string
  profileImageUrl?: string
  description?: string
  title?: string
  category?: string
  tags: string[]
  language?: string
  thumbnailUrl?: string
  startedAt: string
  endedAt?: string
  lastSeenAt: string
  currentViewers: number
  avgViewers: number
  peakViewers: number
  viewerSamples: number
  chatMessages: number
  totalEmoteUses: number
  seventvEmoteUses: number
}

export interface AnalyticsMinuteRollup {
  minuteTs: string
  viewerAvg: number
  viewerMax: number
  viewerLatest: number
  viewerSamples: number
  chatCount: number
  totalEmoteCount: number
  seventvEmoteCount: number
  emotes: Record<string, number>
  missing?: boolean
}

export interface AnalyticsTopEmote {
  key: string
  name: string
  id?: string
  provider?: string
  imageUrl?: string
  count: number
}

export interface AnalyticsStreamDetail {
  channel: string
  state: 'live' | 'historical' | 'not_collected'
  stream?: AnalyticsStream
  rollups: AnalyticsMinuteRollup[]
  topEmotes: AnalyticsTopEmote[]
  sources: SourceStatus[]
  updatedAt: number
  vodId?: string
}

export interface AnalyticsStreamsResponse {
  channel: string
  items: AnalyticsStream[]
  sources: SourceStatus[]
  updatedAt: number
}

export interface ChatBadge {
  setId: string
  versionId: string
  title?: string
  description?: string
  clickUrl?: string
  imageUrl1x?: string
  imageUrl2x?: string
  imageUrl4x?: string
}

export interface ChatBadgeCatalog {
  channel: string
  badges: Record<string, ChatBadge>
  sources: SourceStatus[]
  updatedAt: number
}

export interface ChannelDetails {
  id: string
  login: string
  displayName: string
  profileImage?: string
  description?: string
  createdAt?: string
  aboutPanels?: AboutPanel[]
  socialLinks?: SocialLink[]
  isLive: boolean
  streamId?: string
  streamTitle?: string
  category?: string
  viewers?: number
  thumbnailUrl?: string
  startedAt?: string
  updatedAt: number
  sources?: SourceStatus[]
}

export interface AboutPanel {
  id?: string
  type?: string
  title?: string
  description?: string
  imageUrl?: string
  linkUrl?: string
}

export interface SocialLink {
  id?: string
  title?: string
  url?: string
}

export interface ClipCard {
  id: string
  title: string
  url: string
  embedUrl?: string
  thumbnailUrl?: string
  broadcasterName?: string
  creatorName?: string
  viewCount?: number
  createdAt?: string
  durationSeconds?: number
}

export interface StatsTimelinePoint {
  label: string
  avgViewers: number
  peakViewers: number
  hoursWatched?: number
}

export interface StreamStat {
  id: string
  title: string
  category?: string
  startedAt?: string
  endedAt?: string
  durationMinutes?: number
  avgViewers: number
  peakViewers: number
  hoursWatched?: number
}

export interface StatsDerived {
  hoursStreamed?: number
  viewerHoursPerStreamHour?: number
  peakToAverageRatio?: number
  followersPerStreamHour?: number
  clipsLoaded: number
  lsfPostsLoaded: number
  hasRealStreamHistory: boolean
}

export interface ExternalStatsSnapshot {
  rank: number
  minutes_streamed: number
  avg_viewers: number
  max_viewers: number
  hours_watched: number
  followers: number
  followers_total: number
}

export interface InsightCard {
  id: string
  title: string
  url: string
  permalink: string
  thumbnail?: string
  author?: string
  score: number
  comments: number
  createdUtc: number
  subreddit?: string
  flairText?: string
  streamerTags?: string[]
}

export interface ClipsResponse {
  items: ClipCard[]
  sources: SourceStatus[]
  period: ClipPeriod
  cursor?: string
  updatedAt: number
}

export interface ChannelInsights {
  channel: string
  period: StatsPeriod
  clipPeriod: ClipPeriod
  lsfPeriod: ClipPeriod
  stats?: ExternalStatsSnapshot
  statsDerived?: StatsDerived
  statsTimeline?: StatsTimelinePoint[]
  streamHistory?: StreamStat[]
  clips: ClipCard[]
  lsf: InsightCard[]
  sources: SourceStatus[]
  updatedAt: number
}

export interface RandomStreamResponse {
  stream: Stream
  poolSize: number
  stale: boolean
  updatedAt: number
}

export interface LSFResponse {
  items: InsightCard[]
  sources: SourceStatus[]
  period: ClipPeriod
  sort: 'top' | 'hot' | 'new'
  updatedAt: number
}

export interface AuthDebug {
  ready: boolean
  clientIdConfigured: boolean
  clientSecretConfigured: boolean
  redirectUrl: string
  frontendUrl: string
  apiOrigin: string
  requestOrigin?: string
  redirectOrigin?: string
  frontendOrigin?: string
  cookieName: string
  cookieSameSite: string
  cookieSecureOnThisOrigin: boolean
  callbackMatchesApi: boolean
  frontendMatchesRequest: boolean
  warnings: string[]
}

export interface DevTokenImportRequest {
  accessToken: string
  refreshToken?: string
}

export interface DevTokenImportResponse {
  authenticated: boolean
  user: AuthUser
  scopes: string[]
}

export interface DevDeviceAuthStartResponse {
  requestId: string
  userCode: string
  verificationUri: string
  expiresInSeconds: number
  pollIntervalSeconds: number
}

export interface DevDeviceAuthPendingResponse {
  status: 'pending'
}

export interface DevDeviceAuthAuthenticatedResponse extends DevTokenImportResponse {
  status: 'complete'
}

export type DevDeviceAuthPollResponse = DevDeviceAuthPendingResponse | DevDeviceAuthAuthenticatedResponse

export interface StartupBreakdown {
  upstreamFetchMs?: number
  workerSpawnMs?: number
  hlsReadyMs?: number
  totalMs?: number
}

export interface StartResponse {
  hlsUrl: string
  session_id: string
  quality?: string
  listeners: number
  renditions?: Array<{ name: string; width?: number; height?: number; frameRate?: number; bandwidth?: number; group?: string }>
  selectedRendition?: { name: string; width?: number; height?: number; frameRate?: number; bandwidth?: number; group?: string }
  workerBackend?: string
  startupMs?: number
  startupBreakdown?: StartupBreakdown
  fallbackAttempted?: boolean
  qualityRestarted?: boolean
}

export interface HLSProbe {
  url?: string
  ready: boolean
  statusCode?: number
  durationMs?: number
  contentType?: string
  targetDuration?: string
  partTarget?: string
  mediaSequence?: string
  error?: string
  playlistSummary?: string
}

export interface StreamDiagnostics {
  channel: string
  active: boolean
  hlsUrl?: string
  quality?: string
  listeners?: number
  lastSeen?: number
  startedAt?: number
  uptimeMs?: number
  workerStartedAt?: number
  workerUptimeMs?: number
  restarts: number
  maxRestarts: number
  lastRestartAt?: number
  lastWorkerError?: string
  lastStartError?: string
  stopped: boolean
  backendVersion: string
  latencyMode: string
  protocol: string
  renditions?: Array<{ name: string; width?: number; height?: number; frameRate?: number; bandwidth?: number; group?: string }>
  selectedRendition?: { name: string; width?: number; height?: number; frameRate?: number; bandwidth?: number; group?: string }
  workerBackend?: string
  startupMs?: number
  startupBreakdown?: StartupBreakdown
  fallbackAttempts?: number
  hlsProbe: HLSProbe
  updatedAt: number
}

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const fallback = res.statusText || `HTTP ${res.status}`
    try {
      const body = await res.json() as { error?: string; code?: string; retryable?: boolean }
      throw new ApiError(body.error || fallback, res.status, body.code, body.retryable)
    } catch (err) {
      if (err instanceof ApiError) throw err
      throw new ApiError(fallback, res.status)
    }
  }
  return res.json() as Promise<T>
}

interface Page<T> {
  items: T[]
  cursor: string
}

interface SearchResult {
  streams: Stream[]
  categories: Category[]
}

export interface AuthUser {
  id: string
  login: string
  display_name?: string
  displayName?: string
  profile_image_url?: string
  profileImageUrl?: string
}

export interface MeResponse {
  authenticated: boolean
  canImportLocalToken?: boolean
  user?: AuthUser
  scopes?: string[]
}

export interface FollowedChannel {
  id: string
  login: string
  displayName: string
  profileImage?: string
  isLive: boolean
  title?: string
  category?: string
  viewers?: number
  thumbnailUrl?: string
}

export const getStreams = (): Promise<Stream[]> =>
  fetch(`${METADATA}/v1/streams`).then(r => json<Page<Stream>>(r)).then(p => p.items ?? [])

export const getRandomStream = (pool = 20000): Promise<RandomStreamResponse> =>
  fetch(`${METADATA}/v1/streams/random?pool=${encodeURIComponent(pool)}`).then(r => json<RandomStreamResponse>(r))

export const getCategories = (): Promise<Category[]> =>
  fetch(`${METADATA}/v1/categories?limit=12`).then(r => json<Page<Category>>(r)).then(p => p.items ?? [])

export const getCategoryStreams = (categoryId: string): Promise<Stream[]> =>
  fetch(`${METADATA}/v1/categories/${encodeURIComponent(categoryId)}/streams?limit=24`).then(r => json<Page<Stream>>(r)).then(p => p.items ?? [])

export const search = (q: string): Promise<SearchResult> =>
  fetch(`${METADATA}/v1/search?q=${encodeURIComponent(q)}&limit=24`).then(r => json<SearchResult>(r))

export const getChannel = (login: string): Promise<ChannelInfo> =>
  fetch(`${METADATA}/v1/channels/${encodeURIComponent(login)}`).then(r => json<ChannelInfo>(r))

export const getChannelBadges = (login: string): Promise<ChatBadgeCatalog> =>
  fetch(`${METADATA}/v1/channels/${encodeURIComponent(login)}/badges`).then(r => json<ChatBadgeCatalog>(r))

export const getChannelDetails = (login: string): Promise<ChannelDetails> =>
  fetch(`${METADATA}/v1/channels/${encodeURIComponent(login)}/details`).then(r => json<ChannelDetails>(r))

export const getChannelInsights = (login: string, period: StatsPeriod = '7d', clipPeriod: ClipPeriod = period, lsfPeriod: ClipPeriod = period, lsfSort: 'top' | 'hot' | 'new' = 'top'): Promise<ChannelInsights> =>
  fetch(`${METADATA}/v1/channels/${encodeURIComponent(login)}/insights?period=${encodeURIComponent(period)}&clipPeriod=${encodeURIComponent(clipPeriod)}&lsfPeriod=${encodeURIComponent(lsfPeriod)}&lsfSort=${encodeURIComponent(lsfSort)}`).then(r => json<ChannelInsights>(r))

export interface StreamHistoryResponse {
  channel: string
  period: StatsPeriod
  items: StreamStat[]
  sources: SourceStatus[]
  updatedAt: number
}

export const getChannelStreamHistory = (login: string, period: StatsPeriod = '30d'): Promise<StreamHistoryResponse> =>
  fetch(`${METADATA}/v1/channels/${encodeURIComponent(login)}/streams/history?period=${encodeURIComponent(period)}`).then(r => json<StreamHistoryResponse>(r))

export const watchAnalyticsChannel = (login: string): Promise<AnalyticsWatchResponse> =>
  fetch(`${ANALYTICS}/v1/analytics/channels/${encodeURIComponent(login)}/watch`, { method: 'POST' }).then(r => json<AnalyticsWatchResponse>(r))

export const getAnalyticsLive = (login: string): Promise<AnalyticsStreamDetail> =>
  fetch(`${ANALYTICS}/v1/analytics/channels/${encodeURIComponent(login)}/live`).then(r => json<AnalyticsStreamDetail>(r))

export const getAnalyticsStreams = (login: string, limit = 20): Promise<AnalyticsStreamsResponse> =>
  fetch(`${ANALYTICS}/v1/analytics/channels/${encodeURIComponent(login)}/streams?limit=${encodeURIComponent(limit)}`).then(r => json<AnalyticsStreamsResponse>(r))

export interface AlwaysTrackedResponse {
  channels: string[]
}

export const getAlwaysTracked = (): Promise<AlwaysTrackedResponse> =>
  fetch(`${ANALYTICS}/v1/analytics/always-tracked`).then(r => json<AlwaysTrackedResponse>(r))

export const setAlwaysTracked = (channel: string, track: boolean): Promise<AnalyticsWatchResponse> =>
  fetch(`${ANALYTICS}/v1/analytics/always-tracked`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ channel, track }),
  }).then(r => json<AnalyticsWatchResponse>(r))

export const getAnalyticsStream = async (streamId: string, opts?: { sparse?: boolean }): Promise<AnalyticsStreamDetail | null> => {
  const sparse = opts?.sparse !== false
  const res = await fetch(`${ANALYTICS}/v1/analytics/streams/${encodeURIComponent(streamId)}?sparse=${sparse ? 'true' : 'false'}`)
  if (res.status === 404) return null
  return json<AnalyticsStreamDetail>(res)
}

export interface GameSegment {
  id: number
  streamId: string
  gameName: string
  boxArtUrl: string
  offsetSeconds: number
  durationSeconds: number
  createdAt: string
}

export type SyncPhase =
  | 'starting'
  | 'scraping_tracker'
  | 'parsing_tracker'
  | 'resolving_vod'
  | 'fetching_comments'
  | 'writing_rollups'
  | 'completed'
  | 'failed'

export interface SyncStatus {
  streamId: string
  phase: SyncPhase
  message?: string
  startedAt: string
  updatedAt: string
  commentsFetched?: number
  rollupsWritten?: number
  resultMessage?: string
  error?: string
  viewersOnly?: boolean
}

export interface StartSyncResponse {
  accepted: boolean
  status?: SyncStatus
}

export const startHistoricalSync = async (
  streamId: string,
  login = '',
  options?: { viewersOnly?: boolean },
): Promise<StartSyncResponse> => {
  const params = new URLSearchParams()
  if (login) params.set('channel', login)
  if (options?.viewersOnly) params.set('viewers_only', 'true')
  const res = await fetch(`${ANALYTICS}/v1/analytics/streams/${encodeURIComponent(streamId)}/sync?${params}`, { method: 'POST' })
  return json<StartSyncResponse>(res)
}

export const getSyncStatus = async (streamId: string): Promise<SyncStatus | null> => {
  const res = await fetch(`${ANALYTICS}/v1/analytics/streams/${encodeURIComponent(streamId)}/sync/status`)
  if (res.status === 404) return null
  return json<SyncStatus>(res)
}

/** @deprecated Use startHistoricalSync + getSyncStatus polling */
export const syncHistoricalStream = (
  streamId: string,
  login = '',
  options?: { viewersOnly?: boolean },
): Promise<{ success: boolean; message: string }> => {
  const params = new URLSearchParams()
  if (login) params.set('channel', login)
  if (options?.viewersOnly) params.set('viewers_only', 'true')
  return fetch(`${ANALYTICS}/v1/analytics/streams/${encodeURIComponent(streamId)}/sync?${params}`, { method: 'POST' }).then(r => json<{ success: boolean; message: string }>(r))
}

export const getStreamGameSegments = (streamId: string): Promise<GameSegment[]> =>
  fetch(`${ANALYTICS}/v1/analytics/streams/${encodeURIComponent(streamId)}/games`).then(r => json<GameSegment[]>(r))

export const getTwitchDayClips = (login: string, startedAt: string, endedAt: string, cursor = ''): Promise<ClipsResponse> => {
  const params = new URLSearchParams({ startedAt, endedAt, cursor })
  return fetch(`${METADATA}/v1/channels/${encodeURIComponent(login)}/clips?${params}`).then(r => json<ClipsResponse>(r))
}


export const startStream = (channel: string, quality?: string): Promise<StartResponse> =>
  fetch(`${VIDEO}/v1/stream/start`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ channel, quality }),
  }).then(r => json<StartResponse>(r))

export const getStreamDiagnostics = (channel: string): Promise<StreamDiagnostics> =>
  fetch(`${VIDEO}/v1/stream/diagnostics?channel=${encodeURIComponent(channel)}`).then(r => json<StreamDiagnostics>(r))

export const keepaliveStream = (channel: string, sessionId?: string): Promise<void> =>
  fetch(`${VIDEO}/v1/stream/keepalive`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ channel, session_id: sessionId }),
  }).then(r => { if (!r.ok) throw new Error(r.statusText) })

export const stopStream = (channel: string, sessionId?: string): Promise<void> =>
  fetch(`${VIDEO}/v1/stream/stop`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ channel, session_id: sessionId }),
  }).then(r => { if (!r.ok) throw new Error(r.statusText) })

export type EmoteProvider = 'seventv' | 'twitch' | 'ffz'

export interface EmoteProviderStatus {
  provider: EmoteProvider
  state: 'ready' | 'processing' | 'failed'
  count: number
  pending: number
  failed?: number
  total?: number
  percent?: number
  error?: string
}

export interface EmoteEnsureResponse {
  state: 'ready' | 'processing'
  count: number
  pending: number
  total?: number
  percent?: number
  providers?: EmoteProviderStatus[]
  benchmark?: EmoteBenchmark
}

export interface EmoteBenchmark {
  ensureMs: number
  seedMs: number
  dictionaryMs: number
  cacheHit: boolean
  providers: Array<{ provider: EmoteProvider; state: string; count: number; pending: number; failed?: number; total?: number; percent?: number; durationMs: number }>
}

export interface ChannelEmote {
  name: string
  emote_id: string
  url: string
  zw: boolean
  provider?: EmoteProvider | 'custom'
}

export const ensureChannelEmotes = (login: string, twitchId: string, providers: EmoteProvider[] = ['seventv']): Promise<EmoteEnsureResponse> =>
  fetch(`${EMOTE}/v1/channels/${encodeURIComponent(login)}/emotes/ensure`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ twitch_id: twitchId, providers }),
  }).then(r => json<EmoteEnsureResponse>(r))

export const getChannelEmotes = (login: string): Promise<ChannelEmote[]> =>
  fetch(`${EMOTE}/v1/channels/${encodeURIComponent(login)}/emotes`).then(r => json<ChannelEmote[]>(r))

export const getAuthDebug = (): Promise<AuthDebug> =>
  fetch(`${CHAT_HTTP}/v1/auth/debug`, { credentials: 'include' }).then(r => json<AuthDebug>(r))

export const getMe = (): Promise<MeResponse> =>
  fetch(`${CHAT_HTTP}/v1/me`, { credentials: 'include' }).then(r => json<MeResponse>(r))

export const logout = (): Promise<void> =>
  fetch(`${CHAT_HTTP}/v1/logout`, {
    method: 'POST',
    credentials: 'include',
  }).then(r => { if (!r.ok) throw new Error(r.statusText) })

export const importDevTwitchToken = ({ accessToken, refreshToken }: DevTokenImportRequest): Promise<DevTokenImportResponse> =>
  fetch(`${CHAT_HTTP}/v1/auth/dev/import`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      access_token: accessToken,
      refresh_token: refreshToken?.trim() || undefined,
    }),
  }).then(r => json<DevTokenImportResponse>(r))

export const claimPreparedDevTwitchToken = (): Promise<DevTokenImportResponse> =>
  fetch(`${CHAT_HTTP}/v1/auth/dev/claim`, {
    method: 'POST',
    credentials: 'include',
  }).then(r => json<DevTokenImportResponse>(r))

export const startDevTwitchDeviceAuth = (): Promise<DevDeviceAuthStartResponse> =>
  fetch(`${CHAT_HTTP}/v1/auth/dev/device/start`, {
    method: 'POST',
    credentials: 'include',
  }).then(r => json<DevDeviceAuthStartResponse>(r))

export const pollDevTwitchDeviceAuth = (requestId: string): Promise<DevDeviceAuthPollResponse> =>
  fetch(`${CHAT_HTTP}/v1/auth/dev/device/poll`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ request_id: requestId }),
  }).then(r => json<DevDeviceAuthPollResponse>(r))

export const getFollowedChannels = (): Promise<FollowedChannel[]> =>
  fetch(`${CHAT_HTTP}/v1/followed`, { credentials: 'include' })
    .then(r => json<{ channels: FollowedChannel[] }>(r))
    .then(r => r.channels ?? [])

const browserOrigin = typeof window === 'undefined' ? '' : window.location.origin
const CLIPPER_BASE = CLIPPER === browserOrigin ? `${CLIPPER}/v1/clipper` : `${CLIPPER}/v1`

export interface ClipperJob {
  id: string
  channel: string
  broadcaster_id?: string
  trigger_type: string
  reason?: string
  title?: string
  requested_duration?: number
  source_duration: number
  final_duration: number
  event_latency_offset: number
  trigger_detected_at: number
  peak_chat_ts?: number
  message_count?: number
  twitch_clip_id?: string
  twitch_edit_url?: string
  twitch_clip_url?: string
  twitch_clip_duration?: number
  raw_path?: string
  captions_path?: string
  final_path?: string
  state: 'queued' | 'creating_clip' | 'waiting_for_clip' | 'downloading' | 'transcribing' | 'rendering' | 'ready' | 'failed' | 'purged'
  failure_code?: string
  error_message?: string
  warnings: string[]
  suppressed_count: number
  artifact_available: number
  created_at: number
  updated_at: number
  started_at?: number
  finished_at?: number
}

export interface ClipperJobsResponse {
  items: ClipperJob[]
}

export interface ClipperJobResponse {
  job: ClipperJob
  events: Array<{ id: number; job_id: string; state: string; message: string; created_at: number }>
}

export interface CaptionWordTiming {
  text: string
  start: number
  end: number
}

export interface CaptionWord {
  start: number
  end: number
  text: string
  words?: CaptionWordTiming[]
}

export interface ClipperTemplate {
  id: string
  name: string
  description: string
  format_preset: string
  caption_preset: string
  max_words_per_line: number
  has_intro_zoom: boolean
  has_vignette: boolean
  noise_reduce: string | null
}

export interface ClipperTemplatesResponse {
  items: ClipperTemplate[]
}

function clipperHeaders(extra?: Record<string, string>): Record<string, string> {
  const headers: Record<string, string> = { ...extra }
  if (CLIPPER_TOKEN) {
    headers.Authorization = `Bearer ${CLIPPER_TOKEN}`
  }
  return headers
}

export interface ClipperCaptionsResponse {
  captions: CaptionWord[]
}

export const getClipperJobs = (limit = 100, channel = ''): Promise<ClipperJobsResponse> => {
  const params = new URLSearchParams({ limit: String(limit) })
  if (channel) params.set('channel', channel)
  return fetch(`${CLIPPER_BASE}/jobs?${params}`).then(r => json<ClipperJobsResponse>(r))
}

export const getClipperJob = (jobId: string): Promise<ClipperJobResponse> =>
  fetch(`${CLIPPER_BASE}/jobs/${encodeURIComponent(jobId)}`).then(r => json<ClipperJobResponse>(r))

export const getClipperJobCaptions = (jobId: string): Promise<ClipperCaptionsResponse> =>
  fetch(`${CLIPPER_BASE}/jobs/${encodeURIComponent(jobId)}/captions`).then(r => json<ClipperCaptionsResponse>(r))

export const getClipperTemplates = (): Promise<ClipperTemplatesResponse> =>
  fetch(`${CLIPPER_BASE}/templates`).then(r => json<ClipperTemplatesResponse>(r))

export const updateClipperJobCaptions = (jobId: string, captions: CaptionWord[]): Promise<{ status: string }> =>
  fetch(`${CLIPPER_BASE}/jobs/${encodeURIComponent(jobId)}/captions`, {
    method: 'PUT',
    headers: clipperHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ captions }),
  }).then(r => json<{ status: string }>(r))

export const triggerClipperManual = (channel: string, title?: string, duration?: number, finalDuration?: number): Promise<{ status: string; job_id: string }> =>
  fetch(`${CLIPPER_BASE}/triggers/manual`, {
    method: 'POST',
    headers: clipperHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ channel, title, duration, final_duration: finalDuration }),
  }).then(r => json<{ status: string; job_id: string }>(r))

export type CaptionPreset =
  | 'default'
  | 'tiktok_pop'
  | 'karaoke_pop'
  | 'subtitle_bar'
  | 'gaming'
  | 'none'

export const renderClipperJob = (
  jobId: string,
  options: {
    trim_start?: number
    trim_duration?: number
    format_preset?: 'tiktok' | 'youtube' | 'youtube_short' | 'instagram_reel' | 'twitter'
    caption_preset?: CaptionPreset
    template_id?: string
  }
): Promise<{ status: string; job_id: string }> =>
  fetch(`${CLIPPER_BASE}/jobs/${encodeURIComponent(jobId)}/render`, {
    method: 'POST',
    headers: clipperHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(options),
  }).then(r => json<{ status: string; job_id: string }>(r))

export const transcribeClipperJob = (
  jobId: string,
  options: {
    trim_start?: number
    trim_duration?: number
    max_words_per_line?: number
  }
): Promise<{ status: string; job_id: string }> =>
  fetch(`${CLIPPER_BASE}/jobs/${encodeURIComponent(jobId)}/transcribe`, {
    method: 'POST',
    headers: clipperHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(options),
  }).then(r => json<{ status: string; job_id: string }>(r))

export function describeClipperFailure(job: Pick<ClipperJob, 'failure_code' | 'error_message'>): string {
  switch (job.failure_code) {
    case 'missing_scope':
      return 'Twitch token is missing the clips:edit scope. Run make twitch-local-auth, update .env, and restart the clipper container.'
    case 'twitch_not_configured':
      return 'Clipper Twitch credentials are not configured. Set CLIPPER_TWITCH_CLIENT_ID and CLIPPER_TWITCH_USER_ACCESS_TOKEN in .env, then restart clipper.'
    case 'invalid_token':
      return 'Clipper Twitch token is invalid or expired. Refresh CLIPPER_TWITCH_USER_ACCESS_TOKEN and restart clipper.'
    case 'clip_restricted':
      return 'Twitch rejected clip creation for this channel (clips may be disabled or restricted).'
    case 'offline':
      return 'The channel was offline when clip creation was attempted.'
    default:
      return job.error_message || 'Clip processing failed before a source video was available.'
  }
}

export const getClipperSourceVideoUrl = (jobId: string): string =>
  `${CLIPPER_BASE}/jobs/${encodeURIComponent(jobId)}/source.mp4`

export const getClipperFinalVideoUrl = (jobId: string): string =>
  `${CLIPPER_BASE}/jobs/${encodeURIComponent(jobId)}/final.mp4`
