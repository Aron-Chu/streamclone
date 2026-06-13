import { ANALYTICS, CHAT_HTTP, EMOTE, METADATA, VIDEO, CLIPPER, CLIPPER_TOKEN, SETUP_CONTROL_TOKEN } from './config'
import type { ClipPeriod, PlaybackLatencyMode, StatsPeriod } from './settings'

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

export interface ChatCoverageSummary {
  chatSpanMinutes: number
  streamSpanMinutes: number
  coveragePct: number
  partial: boolean
  vodDurationSec?: number
}

export interface AnalyticsStreamDetail {
  channel: string
  state: 'live' | 'historical' | 'not_collected' | 'syncing'
  stream?: AnalyticsStream
  rollups: AnalyticsMinuteRollup[]
  topEmotes: AnalyticsTopEmote[]
  sources: SourceStatus[]
  updatedAt: number
  vodId?: string
  syncPhase?: string
  chatCoveragePct?: number
  vodDurationSec?: number
  chatCoverage?: ChatCoverageSummary
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
  videoId?: string
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

export type SetupServiceState = 'ready' | 'offline'

export interface SetupWelcomeServices {
  scraper: SetupServiceState
  clipper: SetupServiceState
}

export interface SetupWelcome {
  profile: string
  services: SetupWelcomeServices
  incomplete: boolean
  showWelcome: boolean
  setupGuideUrl: string
}

export interface SetupControlHealth {
  ok: boolean
  service?: string
}

export interface SetupControlStartResponse {
  ok: boolean
  service?: string
  message?: string
  error?: string
  log?: string
  warmup?: string
}

export interface SetupControlStartStatus {
  ok: boolean
  service: 'scraper' | 'clipper'
  percent: number
  phase: string
  detail: string
  lines?: string[]
  warmup?: string
}

export interface HostDiagnosticsContainer {
  name: string
  status: string
}

export interface HostDiagnostics {
  ok: boolean
  healthy: boolean
  profile: string
  imageTag: string
  docker: string
  configReady: boolean
  webOk: boolean
  webUrl: string
  setupControl: boolean
  containers: HostDiagnosticsContainer[]
  optionalServices: SetupWelcomeServices
  scraperSibling: { path: string; present: boolean }
  recentStartLogs: { scraper: string; clipper: string }
  suggestions: string[]
}

export interface MetadataDiagnosticsServices {
  metadata: SetupServiceState
  chat: SetupServiceState
  video: SetupServiceState
  emote: SetupServiceState
  analytics: SetupServiceState
  scraper: SetupServiceState
  clipper: SetupServiceState
}

export interface MetadataDiagnostics {
  profile: string
  imageTag: string
  services: MetadataDiagnosticsServices
  healthy: boolean
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
  liveEdge?: number
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

function apiErrorMessage(body: Record<string, unknown>, fallback: string): string {
  if (typeof body.error === 'string' && body.error) return body.error
  const detail = body.detail
  if (typeof detail === 'string' && detail) return detail
  if (detail && typeof detail === 'object') {
    const nested = detail as { error?: string; message?: string }
    if (nested.error) return nested.error
    if (nested.message) return nested.message
  }
  return fallback
}

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const fallback = res.statusText || `HTTP ${res.status}`
    try {
      const body = await res.json() as Record<string, unknown>
      let message = apiErrorMessage(body, fallback)
      if (res.status === 401 && message.toLowerCase().includes('unauthorized')) {
        message = CLIPPER_TOKEN
          ? 'Clipper rejected the webhook token. Ensure VITE_CLIPPER_TOKEN matches CLIPPER_WEBHOOK_TOKEN in .env, then recreate the frontend container.'
          : 'Clipper webhook auth is not configured in the browser. Ensure VITE_CLIPPER_TOKEN matches CLIPPER_WEBHOOK_TOKEN in .env, then recreate the frontend container.'
      }
      throw new ApiError(
        message,
        res.status,
        typeof body.code === 'string' ? body.code : undefined,
        typeof body.retryable === 'boolean' ? body.retryable : undefined,
      )
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

export const prefetchAnalyticsTracker = (streamId: string, channel: string): Promise<{ status: string }> =>
  fetch(`${ANALYTICS}/v1/analytics/streams/${encodeURIComponent(streamId)}/prefetch-tracker?channel=${encodeURIComponent(channel)}`, {
    method: 'POST',
  }).then(r => json<{ status: string }>(r))

export const getAnalyticsStream = async (
  streamId: string,
  opts?: { sparse?: boolean; channel?: string },
): Promise<AnalyticsStreamDetail | null> => {
  const sparse = opts?.sparse !== false
  const params = new URLSearchParams({ sparse: sparse ? 'true' : 'false' })
  if (opts?.channel) params.set('channel', opts.channel)
  const res = await fetch(`${ANALYTICS}/v1/analytics/streams/${encodeURIComponent(streamId)}?${params}`)
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

export interface SyncPhaseTiming {
  trackerScrapeMs?: number
  vodResolveMs?: number
  gqlFetchMs?: number
  tokenizeMs?: number
  rollupWriteMs?: number
}

export type SyncChatIndexPhase = 'fetching' | 'tokenizing' | 'writing' | 'done'

export interface SyncChatProgress {
  active?: boolean
  vodId?: string
  fetchMode?: 'parallel' | 'serial' | string
  concurrency?: number
  segmentsTotal?: number
  segmentsDone?: number
  commentsFetched?: number
  timelineSec?: number
  vodDurationSec?: number
  streamDurationSec?: number
  rollupsExpected?: number
  indexPhase?: SyncChatIndexPhase | string
  gqlPages?: number
  throttled?: boolean
  message?: string
}

export interface SyncTrackerProgress {
  active?: boolean
  url?: string
  message?: string
}

export interface SyncStatus {
  streamId: string
  phase: SyncPhase
  message?: string
  startedAt: string
  updatedAt: string
  stale?: boolean
  commentsFetched?: number
  rollupsWritten?: number
  resultMessage?: string
  error?: string
  viewersOnly?: boolean
  viewerStatus?: string
  timing?: SyncPhaseTiming
  chat?: SyncChatProgress
  tracker?: SyncTrackerProgress
}

export interface StartSyncResponse {
  accepted: boolean
  status?: SyncStatus
}

export const startHistoricalSync = async (
  streamId: string,
  login = '',
  options?: { viewersOnly?: boolean; vodId?: string; forceChat?: boolean },
): Promise<StartSyncResponse> => {
  const params = new URLSearchParams()
  if (login) params.set('channel', login)
  if (options?.viewersOnly) params.set('viewers_only', 'true')
  if (options?.forceChat) params.set('force_chat', 'true')
  if (options?.vodId) params.set('vod_id', options.vodId)
  const res = await fetch(`${ANALYTICS}/v1/analytics/streams/${encodeURIComponent(streamId)}/sync?${params}`, { method: 'POST' })
  return json<StartSyncResponse>(res)
}

export const getSyncStatus = async (streamId: string): Promise<SyncStatus | null> => {
  const res = await fetch(`${ANALYTICS}/v1/analytics/streams/${encodeURIComponent(streamId)}/sync/status`)
  if (res.status === 404) return null
  if (res.status === 502 || res.status === 503 || res.status === 504) {
    throw new ApiError('Sync status upstream unavailable', res.status, 'upstream_unavailable', true)
  }
  const body: { phase?: string } & Record<string, unknown> = await json(res)
  // Backend returns 200 { phase: "idle" } when no Redis sync key — not an active sync.
  if (body.phase === 'idle') return null
  return body as unknown as SyncStatus
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


export const startStream = (channel: string, quality?: string, latencyMode?: PlaybackLatencyMode): Promise<StartResponse> =>
  fetch(`${VIDEO}/v1/stream/start`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ channel, quality, latency_mode: latencyMode }),
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

export type EmoteProvider = 'seventv' | 'twitch' | 'ffz' | 'bttv'

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

export const getSetupWelcome = (): Promise<SetupWelcome> =>
  fetch(`${METADATA}/v1/setup/welcome`).then(r => json<SetupWelcome>(r))

export const getSetupControlHealth = (): Promise<SetupControlHealth> =>
  fetch('/v1/setup-control/health').then(r => json<SetupControlHealth>(r))

function setupControlStartError(status: number, body: SetupControlStartResponse): ApiError {
  if (status === 401) {
    return new ApiError(
      'Setup token rejected. Close this tab, run Start Streamclone from Desktop, then try again.',
      status,
      body.error,
    )
  }
  if (status === 502 || status === 504) {
    return new ApiError(
      'Install helper timed out or is unavailable. Ensure Docker Desktop is running, run Start Streamclone.cmd, then retry.',
      status,
      body.error,
      true,
    )
  }
  return new ApiError(body.error || 'Unable to start service', status, body.error, status >= 500)
}

export const getSetupStartStatus = (service: 'scraper' | 'clipper'): Promise<SetupControlStartStatus> =>
  fetch(`/v1/setup-control/start/${service}/status`).then(r => json<SetupControlStartStatus>(r))

export const startSetupService = (service: 'scraper' | 'clipper'): Promise<SetupControlStartResponse> => {
  const headers: Record<string, string> = {}
  if (SETUP_CONTROL_TOKEN) {
    headers['X-Streamclone-Setup-Token'] = SETUP_CONTROL_TOKEN
  }
  return fetch(`/v1/setup-control/start/${service}`, { method: 'POST', headers }).then(async r => {
    const body = await r.json().catch(() => ({})) as SetupControlStartResponse
    if (!r.ok) {
      throw setupControlStartError(r.status, body)
    }
    return body
  })
}

export const syncClipperAuthFromSignIn = (): Promise<{ ok: boolean; merged?: boolean; recreated?: boolean; message?: string }> => {
  const headers: Record<string, string> = {}
  if (SETUP_CONTROL_TOKEN) {
    headers['X-Streamclone-Setup-Token'] = SETUP_CONTROL_TOKEN
  }
  return fetch('/v1/setup-control/sync-clipper-auth', { method: 'POST', headers }).then(async r => {
    const body = await r.json().catch(() => ({})) as { ok: boolean; merged?: boolean; recreated?: boolean; message?: string; error?: string }
    if (!r.ok) {
      throw new ApiError(body.error || r.statusText, r.status)
    }
    return body
  })
}

export const getHostDiagnostics = (): Promise<HostDiagnostics> =>
  fetch('/v1/setup-control/diagnostics').then(r => json<HostDiagnostics>(r))

export const getMetadataDiagnostics = (): Promise<MetadataDiagnostics> =>
  fetch(`${METADATA}/v1/setup/diagnostics`).then(r => json<MetadataDiagnostics>(r))

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

export const getLocalFollowedChannels = (): Promise<FollowedChannel[]> =>
  fetch(`${METADATA}/v1/followed`)
    .then(r => json<{ channels: FollowedChannel[] }>(r))
    .then(r => r.channels ?? [])

export const getTwitchFollowedChannels = (): Promise<FollowedChannel[]> =>
  fetch(`${CHAT_HTTP}/v1/me/followed`, { credentials: 'include' })
    .then(async r => {
      if (r.status === 401) return [] as FollowedChannel[]
      if (!r.ok) throw new Error(r.statusText)
      const body = await json<{ channels: FollowedChannel[] }>(r)
      return body.channels ?? []
    })

function mergeFollowedChannels(twitch: FollowedChannel[], local: FollowedChannel[]): FollowedChannel[] {
  const byLogin = new Map<string, FollowedChannel>()
  const order: string[] = []
  const add = (channel: FollowedChannel) => {
    const key = channel.login.toLowerCase()
    const existing = byLogin.get(key)
    if (existing) {
      byLogin.set(key, {
        ...existing,
        ...channel,
        isLive: existing.isLive || channel.isLive,
        viewers: Math.max(existing.viewers ?? 0, channel.viewers ?? 0) || channel.viewers || existing.viewers,
        profileImage: channel.profileImage || existing.profileImage,
        thumbnailUrl: channel.thumbnailUrl || existing.thumbnailUrl,
      })
      return
    }
    byLogin.set(key, channel)
    order.push(key)
  }
  for (const channel of twitch) add(channel)
  for (const channel of local) add(channel)
  return order.map(login => byLogin.get(login)!)
}

/** Twitch follows (when signed in) merged with Streamclone-local follows. */
export const getFollowedChannels = (): Promise<FollowedChannel[]> =>
  Promise.all([getTwitchFollowedChannels(), getLocalFollowedChannels()])
    .then(([twitch, local]) => mergeFollowedChannels(twitch, local))

export const followChannel = (login: string): Promise<{ ok: boolean; login: string; following: boolean }> =>
  fetch(`${METADATA}/v1/channels/${encodeURIComponent(login)}/follow`, { method: 'POST' })
    .then(r => json<{ ok: boolean; login: string; following: boolean }>(r))

export const unfollowChannel = (login: string): Promise<{ ok: boolean; login: string; following: boolean }> =>
  fetch(`${METADATA}/v1/channels/${encodeURIComponent(login)}/follow`, { method: 'DELETE' })
    .then(r => json<{ ok: boolean; login: string; following: boolean }>(r))

const browserOrigin = typeof window === 'undefined' ? '' : window.location.origin
const CLIPPER_BASE = CLIPPER === browserOrigin ? `${CLIPPER}/v1/clipper` : `${CLIPPER}/v1`

export interface ClipperMomentContext {
  stream_id?: string
  minute_ts?: string
  vod_offset_seconds?: number
  viewer_count?: number
  chat_per_min?: number
  emote_per_min?: number
  top_emotes?: Array<{ name: string; count: number; image_url?: string; imageUrl?: string }>
  chat_multiplier?: number
  pick_reason?: 'viewer_spike' | 'chat_spike' | 'emote_spike' | 'manual' | string
  moment_score?: number
}

export interface ClipperJob {
  id: string
  channel: string
  broadcaster_id?: string
  trigger_type: string
  reason?: string
  moment_context?: ClipperMomentContext | null
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

export interface CaptionTransform {
  x: number
  y: number
  rotation: number
  scale: number
}

export type CaptionEffect = 'none' | 'pop' | 'glow' | 'bounce' | 'shake'

export interface CaptionWord {
  start: number
  end: number
  text: string
  words?: CaptionWordTiming[]
  transform?: CaptionTransform
  effect?: CaptionEffect
}

export interface ClipperTemplate {
  id: string
  name: string
  description: string
  format_preset: string
  caption_preset: string
  max_words_per_line: number
  layout?: string
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

export interface ClipperTwitchStatus {
  ok: boolean
  failure_code?: string
  remediation?: string
  expires_in?: number
  has_clips_edit?: boolean
}

export const getClipperTwitchStatus = (): Promise<ClipperTwitchStatus> =>
  fetch(`${CLIPPER_BASE}/twitch/status`).then(r => json<ClipperTwitchStatus>(r))

export const updateClipperJobCaptions = (jobId: string, captions: CaptionWord[]): Promise<{ status: string }> =>
  fetch(`${CLIPPER_BASE}/jobs/${encodeURIComponent(jobId)}/captions`, {
    method: 'PUT',
    headers: clipperHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ captions }),
  }).then(r => json<{ status: string }>(r))

export interface ClipperManualTriggerOptions {
  title?: string
  duration?: number
  final_duration?: number
  reason?: string
  moment_context?: ClipperMomentContext
}

export const triggerClipperManual = (
  channel: string,
  options?: ClipperManualTriggerOptions | string,
  duration?: number,
  finalDuration?: number,
): Promise<{ status: string; job_id: string }> => {
  const payload: Record<string, unknown> = { channel }
  if (typeof options === 'string') {
    payload.title = options
    if (duration !== undefined) payload.duration = duration
    if (finalDuration !== undefined) payload.final_duration = finalDuration
  } else if (options) {
    Object.assign(payload, options)
  }
  return fetch(`${CLIPPER_BASE}/triggers/manual`, {
    method: 'POST',
    headers: clipperHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(payload),
  }).then(r => json<{ status: string; job_id: string }>(r))
}

export type CaptionPreset =
  | 'default'
  | 'tiktok_pop'
  | 'karaoke_pop'
  | 'subtitle_bar'
  | 'gaming'
  | 'none'

export type CaptionSize = 'sm' | 'md' | 'lg'
export type CaptionPosition = 'top' | 'center' | 'bottom'

export const renderClipperJob = (
  jobId: string,
  options: {
    trim_start?: number
    trim_duration?: number
    format_preset?: 'tiktok' | 'youtube' | 'youtube_short' | 'instagram_reel' | 'twitter'
    caption_preset?: CaptionPreset
    template_id?: string
    caption_size?: CaptionSize
    caption_position?: CaptionPosition
    layout?: string
    layout_split_ratio?: number
    emote_map?: Record<string, string>
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

export const retryClipperJob = (jobId: string): Promise<{ job: ClipperJob }> =>
  fetch(`${CLIPPER_BASE}/jobs/${encodeURIComponent(jobId)}/retry`, {
    method: 'POST',
    headers: clipperHeaders({ 'Content-Type': 'application/json' }),
  }).then(r => json<{ job: ClipperJob }>(r))

export interface ClipperEditorProject {
  trim_start?: number
  trim_end?: number
  format_preset?: string
  caption_preset?: CaptionPreset
  caption_size?: CaptionSize
  caption_position?: CaptionPosition
  layout?: string
  layout_split_ratio?: number
  selected_template_id?: string | null
}

export const getClipperJobProject = (jobId: string): Promise<{ project: ClipperEditorProject }> =>
  fetch(`${CLIPPER_BASE}/jobs/${encodeURIComponent(jobId)}/project`).then(r => json<{ project: ClipperEditorProject }>(r))

export const updateClipperJobProject = (jobId: string, project: ClipperEditorProject): Promise<{ status: string }> =>
  fetch(`${CLIPPER_BASE}/jobs/${encodeURIComponent(jobId)}/project`, {
    method: 'PUT',
    headers: clipperHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ project }),
  }).then(r => json<{ status: string }>(r))

export const previewClipperJob = (
  jobId: string,
  options: Parameters<typeof renderClipperJob>[1],
): Promise<{ status: string; job_id: string }> =>
  fetch(`${CLIPPER_BASE}/jobs/${encodeURIComponent(jobId)}/preview`, {
    method: 'POST',
    headers: clipperHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(options),
  }).then(r => json<{ status: string; job_id: string }>(r))

export const getClipperPreviewVideoUrl = (jobId: string): string =>
  `${CLIPPER_BASE}/jobs/${encodeURIComponent(jobId)}/preview.mp4`

export const batchQueueClipperMoments = (
  moments: ClipperManualTriggerOptions[],
): Promise<{ queued: string[]; suppressed: string[]; count: number }> =>
  fetch(`${CLIPPER_BASE}/jobs/batch`, {
    method: 'POST',
    headers: clipperHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ moments }),
  }).then(r => json<{ queued: string[]; suppressed: string[]; count: number }>(r))

export function describeClipperFailure(job: Pick<ClipperJob, 'failure_code' | 'error_message'>): string {
  switch (job.failure_code) {
    case 'missing_scope':
      return 'Twitch token is missing the clips:edit scope. Run make twitch-local-auth, update .env, and restart the clipper container.'
    case 'twitch_not_configured':
      return 'Clipper Twitch credentials are not configured. Set CLIPPER_TWITCH_CLIENT_ID and CLIPPER_TWITCH_USER_ACCESS_TOKEN in .env, then restart clipper.'
    case 'invalid_token':
      return 'Clipper Twitch token is expired or revoked. Run make twitch-local-auth, approve the Twitch login, then recreate the clipper service. Restarting clipper alone does not refresh the token.'
    case 'clip_restricted':
      return 'Twitch rejected clip creation for this channel (clips may be disabled or restricted).'
    case 'clip_not_ready':
      return 'Twitch created the clip but it was not ready before the poll timeout. Use Retry — the worker resumes from the existing clip ID without creating a duplicate.'
    case 'offline':
    case 'not_found':
      if (job.error_message?.toLowerCase().includes('offline')) {
        return 'Twitch only creates clips from live broadcasts. For past stream moments, use Jump into VOD — VOD-based clipping in Clip Studio is not available yet.'
      }
      if (job.failure_code === 'offline') {
        return 'The channel was offline when clip creation was attempted. Clip moments from live analytics require the broadcaster to be live; for past streams, use Jump into VOD.'
      }
      return job.error_message || 'Clip request failed (channel may be offline or unavailable).'
    case 'job_failed':
    case 'transcribe_failed':
    case 'render_failed':
      return job.error_message || 'Clip processing failed. Check clipper logs, then use Retry if the Twitch clip was created.'
    default:
      if (job.error_message?.includes('source video')) {
        return 'Source video was not ready in time — Twitch may still be processing the clip. Wait ~30–90s and click Retry; the worker resumes from twitch_clip_id or raw_path without re-creating the clip.'
      }
      return job.error_message || 'Clip processing failed before a source video was available. Try Retry after the Twitch clip finishes processing.'
  }
}

const CLIPPER_TERMINAL_STATES = new Set(['ready', 'failed', 'purged'])

const CLIPPER_STATE_LABELS: Record<string, string> = {
  queued: 'Queued',
  creating_clip: 'Creating clip',
  waiting_for_clip: 'Waiting for Twitch',
  downloading: 'Downloading',
  transcribing: 'Transcribing',
  rendering: 'Rendering',
  ready: 'Ready',
  failed: 'Failed',
  purged: 'Purged',
}

export function isClipperJobInProgress(job: Pick<ClipperJob, 'state'>): boolean {
  return !CLIPPER_TERMINAL_STATES.has(job.state)
}

export function describeClipperJobState(
  job: Pick<ClipperJob, 'state' | 'artifact_available'>,
): string {
  if (job.state === 'ready') {
    return job.artifact_available === 1 ? 'Exported' : 'Studio ready'
  }
  return CLIPPER_STATE_LABELS[job.state] || job.state
}

export const getClipperSourceVideoUrl = (jobId: string): string =>
  `${CLIPPER_BASE}/jobs/${encodeURIComponent(jobId)}/source.mp4`

export const getClipperFinalVideoUrl = (jobId: string): string =>
  `${CLIPPER_BASE}/jobs/${encodeURIComponent(jobId)}/final.mp4`
