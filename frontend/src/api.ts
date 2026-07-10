import { CHAT_HTTP, EMOTE, METADATA, VIDEO, SETUP_CONTROL_BASE, SETUP_CONTROL_TOKEN } from './config'
import type { ClipPeriod, PlaybackLatencyMode, StatsPeriod } from './settings'
import { buildVodStartRequestBody } from './utils/vodLink.ts'

export class ApiError extends Error {
  status: number
  code?: string
  retryable?: boolean
  reason?: string

  constructor(message: string, status: number, code?: string, retryable?: boolean, reason?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.retryable = retryable
    this.reason = reason
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
  viewers?: number
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
  thumbnailUrl?: string
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
  pulse: SetupServiceState
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
  service: 'scraper' | 'clipper' | 'pulse'
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

export interface HostNetworkContainerStats {
  name: string
  cpuPerc: string
  memUsage: string
  rxBytes: number
  txBytes: number
  rxHuman: string
  txHuman: string
}

export interface HostNetworkDiagnostics {
  ok?: boolean
  containers: HostNetworkContainerStats[]
  updatedAt: number
}

export interface OpsNetworkPromSeries {
  labels?: Record<string, string>
  value: number
}

export interface OpsNetworkPromMetric {
  query: string
  value?: number | null
  series?: OpsNetworkPromSeries[]
}

export interface OpsNetworkPrometheus {
  httpRequestsPerSec?: OpsNetworkPromMetric
  chatConnections?: OpsNetworkPromMetric
  streamListeners?: OpsNetworkPromMetric
  chatMessagesOutPerSec?: OpsNetworkPromMetric
  upstreamP95Sec?: OpsNetworkPromMetric
  streamListenersByChannel?: OpsNetworkPromMetric
}

export interface SyncNetworkUsage {
  trackerScrapeBytes?: number
  gqlFetchBytes?: number
  emotePreloadBytes?: number
  helixBytes?: number
  totalBytes?: number
  lastRateBps?: number
}

export interface TrackingSnapshot {
  tracked: string[]
  alwaysTracked?: string[]
  active?: number
  max?: number
  updatedAt?: number
}

export interface OpsActiveStream {
  channel: string
  listeners: number
  quality?: string
  liveEdge?: number
  workerBackend?: string
  hlsProbeDurationMs?: number
  targetDuration?: string
  bandwidth?: number
  latencyMode?: string
}

export interface OpsNetworkSnapshot {
  services: MetadataDiagnosticsServices
  pulseReady: boolean
  prometheus?: OpsNetworkPrometheus
  activeStreams: OpsActiveStream[]
  trackingSnapshot?: TrackingSnapshot
  updatedAt: number
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
  recentStartLogs: { scraper: string; clipper: string; pulse?: string }
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
  pulse: SetupServiceState
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

export interface VodStartResponse extends StartResponse {
  vod_id: string
  offset_seconds: number
  seek_seconds: number
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
  /** Relay transport currently serving playback: 'll-hls' | 'hls-mpegts' ('webrtc' reserved). */
  activeTransport?: string
  /** Server-side end-to-end live delay estimate (seconds) from live edge + segment/part window. */
  measuredDelaySec?: number
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
        message = 'Request rejected by upstream service.'
      }
      throw new ApiError(
        message,
        res.status,
        typeof body.code === 'string' ? body.code : undefined,
        typeof body.retryable === 'boolean' ? body.retryable : undefined,
        typeof body.reason === 'string' ? body.reason : undefined,
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

export interface StreamsPage {
  items: Stream[]
  cursor: string
}

export interface CategoriesPage {
  items: Category[]
  cursor: string
}

export const getStreamsPage = ({
  limit = 30,
  cursor,
}: {
  limit?: number
  cursor?: string
} = {}): Promise<StreamsPage> => {
  const params = new URLSearchParams({ limit: String(limit) })
  if (cursor) params.set('cursor', cursor)
  return fetch(`${METADATA}/v1/streams?${params}`)
    .then(r => json<Page<Stream>>(r))
    .then(p => ({ items: p.items ?? [], cursor: p.cursor ?? '' }))
}

export const getCategoriesPage = ({
  limit = 30,
  cursor,
}: {
  limit?: number
  cursor?: string
} = {}): Promise<CategoriesPage> => {
  const params = new URLSearchParams({ limit: String(limit) })
  if (cursor) params.set('cursor', cursor)
  return fetch(`${METADATA}/v1/categories?${params}`)
    .then(r => json<Page<Category>>(r))
    .then(p => ({ items: p.items ?? [], cursor: p.cursor ?? '' }))
}

export const getCategoryStreamsPage = (
  categoryId: string,
  { limit = 30, cursor }: { limit?: number; cursor?: string } = {},
): Promise<StreamsPage> => {
  const params = new URLSearchParams({ limit: String(limit) })
  if (cursor) params.set('cursor', cursor)
  return fetch(`${METADATA}/v1/categories/${encodeURIComponent(categoryId)}/streams?${params}`)
    .then(r => json<Page<Stream>>(r))
    .then(p => ({ items: p.items ?? [], cursor: p.cursor ?? '' }))
}

export const getStreams = (): Promise<Stream[]> => getStreamsPage({ limit: 20 }).then(p => p.items)

export const getRandomStream = (pool = 20000): Promise<RandomStreamResponse> =>
  fetch(`${METADATA}/v1/streams/random?pool=${encodeURIComponent(pool)}`).then(r => json<RandomStreamResponse>(r))

export const getCategories = (): Promise<Category[]> =>
  getCategoriesPage({ limit: 12 }).then(p => p.items)

export const getCategoryStreams = (categoryId: string): Promise<Stream[]> =>
  getCategoryStreamsPage(categoryId, { limit: 24 }).then(p => p.items)

export const search = (q: string): Promise<SearchResult> =>
  fetch(`${METADATA}/v1/search?q=${encodeURIComponent(q)}&limit=24`).then(r => json<SearchResult>(r))

export const getChannel = (login: string): Promise<ChannelInfo> =>
  fetch(`${METADATA}/v1/channels/${encodeURIComponent(login)}`).then(r => json<ChannelInfo>(r))

export const getChannelBadges = (login: string): Promise<ChatBadgeCatalog> =>
  fetch(`${METADATA}/v1/channels/${encodeURIComponent(login)}/badges`).then(r => json<ChatBadgeCatalog>(r))

export const getChannelDetails = (login: string): Promise<ChannelDetails> =>
  fetch(`${METADATA}/v1/channels/${encodeURIComponent(login)}/details`).then(r => json<ChannelDetails>(r))

export const getChannelInsights = (
  login: string,
  period: StatsPeriod = '7d',
  clipPeriod: ClipPeriod = period,
  lsfPeriod: ClipPeriod = period,
  lsfSort: 'top' | 'hot' | 'new' = 'top',
  options?: { lsfRefresh?: boolean },
): Promise<ChannelInsights> => {
  const params = new URLSearchParams({
    period,
    clipPeriod,
    lsfPeriod,
    lsfSort,
  })
  if (options?.lsfRefresh) params.set('lsfRefresh', '1')
  return fetch(`${METADATA}/v1/channels/${encodeURIComponent(login)}/insights?${params}`).then(r => json<ChannelInsights>(r))
}

export interface StreamHistoryResponse {
  channel: string
  period: StatsPeriod
  items: StreamStat[]
  sources: SourceStatus[]
  updatedAt: number
}

export const getChannelStreamHistory = (login: string, period: StatsPeriod = '30d'): Promise<StreamHistoryResponse> =>
  fetch(`${METADATA}/v1/channels/${encodeURIComponent(login)}/streams/history?period=${encodeURIComponent(period)}`).then(r => json<StreamHistoryResponse>(r))

export const getTwitchDayClips = (login: string, startedAt: string, endedAt: string, cursor = ''): Promise<ClipsResponse> => {
  const params = new URLSearchParams({ startedAt, endedAt, cursor })
  return fetch(`${METADATA}/v1/channels/${encodeURIComponent(login)}/clips?${params}`).then(r => json<ClipsResponse>(r))
}

export const startStream = (
  channel: string,
  quality?: string,
  latencyMode?: PlaybackLatencyMode,
  options?: { prewarm?: boolean },
): Promise<StartResponse> =>
  fetch(`${VIDEO}/v1/stream/start`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ channel, quality, latency_mode: latencyMode, prewarm: options?.prewarm === true }),
  }).then(r => json<StartResponse>(r))

export const startVodPlayback = (
  vodId: string,
  offsetSeconds = 0,
  quality?: string,
  latencyMode?: PlaybackLatencyMode,
): Promise<VodStartResponse> =>
  fetch(`${VIDEO}/v1/stream/vod/start`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(buildVodStartRequestBody(vodId, offsetSeconds, quality, latencyMode)),
  }).then(r => json<VodStartResponse>(r))

export function vodSessionKey(vodId: string): string {
  return `vod:${vodId.trim()}`
}

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
  state: 'ready' | 'processing' | 'partial' | 'failed'
  count: number
  pending: number
  failed?: number
  total?: number
  percent?: number
  error?: string
}

export interface EmoteEnsureResponse {
  state: 'ready' | 'processing' | 'partial'
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
  fetch(`${SETUP_CONTROL_BASE}/health`).then(r => json<SetupControlHealth>(r))

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

export const getSetupStartStatus = (service: 'scraper' | 'clipper' | 'pulse'): Promise<SetupControlStartStatus> =>
  fetch(`${SETUP_CONTROL_BASE}/start/${service}/status`).then(r => json<SetupControlStartStatus>(r))

export const startSetupService = (service: 'scraper' | 'clipper' | 'pulse'): Promise<SetupControlStartResponse> => {
  const headers: Record<string, string> = {}
  if (SETUP_CONTROL_TOKEN) {
    headers['X-Streamclone-Setup-Token'] = SETUP_CONTROL_TOKEN
  }
  return fetch(`${SETUP_CONTROL_BASE}/start/${service}`, { method: 'POST', headers }).then(async r => {
    const body = await r.json().catch(() => ({})) as SetupControlStartResponse
    if (!r.ok) {
      throw setupControlStartError(r.status, body)
    }
    return body
  })
}

export const stopSetupService = (service: 'scraper' | 'clipper' | 'pulse'): Promise<{ ok: boolean; message?: string; error?: string }> => {
  const headers: Record<string, string> = {}
  if (SETUP_CONTROL_TOKEN) {
    headers['X-Streamclone-Setup-Token'] = SETUP_CONTROL_TOKEN
  }
  return fetch(`${SETUP_CONTROL_BASE}/stop/${service}`, { method: 'POST', headers }).then(async r => {
    const body = await r.json().catch(() => ({})) as { ok: boolean; message?: string; error?: string }
    if (!r.ok) {
      throw new ApiError(body.error || r.statusText, r.status)
    }
    return body
  })
}

export const getHostDiagnostics = (): Promise<HostDiagnostics> =>
  fetch(`${SETUP_CONTROL_BASE}/diagnostics`).then(r => json<HostDiagnostics>(r))

export const getHostNetworkDiagnostics = (): Promise<HostNetworkDiagnostics> =>
  fetch(`${SETUP_CONTROL_BASE}/diagnostics/network`).then(r => json<HostNetworkDiagnostics>(r))

export const getMetadataDiagnostics = (): Promise<MetadataDiagnostics> =>
  fetch(`${METADATA}/v1/setup/diagnostics`).then(r => json<MetadataDiagnostics>(r))

export const getOpsNetworkSnapshot = (): Promise<OpsNetworkSnapshot> =>
  fetch(`${METADATA}/v1/ops/network`).then(r => json<OpsNetworkSnapshot>(r))

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
export const getFollowedChannels = (includeTwitch = true): Promise<FollowedChannel[]> =>
  Promise.all([
    includeTwitch ? getTwitchFollowedChannels() : Promise.resolve([] as FollowedChannel[]),
    getLocalFollowedChannels(),
  ]).then(([twitch, local]) => mergeFollowedChannels(twitch, local))

export const followChannel = (login: string): Promise<{ ok: boolean; login: string; following: boolean }> =>
  fetch(`${METADATA}/v1/channels/${encodeURIComponent(login)}/follow`, { method: 'POST' })
    .then(r => json<{ ok: boolean; login: string; following: boolean }>(r))

export const unfollowChannel = (login: string): Promise<{ ok: boolean; login: string; following: boolean }> =>
  fetch(`${METADATA}/v1/channels/${encodeURIComponent(login)}/follow`, { method: 'DELETE' })
    .then(r => json<{ ok: boolean; login: string; following: boolean }>(r))
