export interface AnalyticsStreamOptions {
  sparse?: boolean
  channel?: string
}

export interface PulseBookmarkQuery {
  login?: string
  streamId?: string
  vodId?: string
  limit?: number
  cursor?: string
}

export interface StartHistoricalSyncOptions {
  viewersOnly?: boolean
  forceChat?: boolean
  vodId?: string
}

export interface SetupWelcome {
  profile: string
  services: Record<string, string>
  incomplete: boolean
  showWelcome: boolean
  setupGuideUrl?: string
}

export interface TwitchDayClip {
  id: string
  url: string
  title: string
  thumbnailUrl?: string
  durationSeconds?: number
  viewCount?: number
  creatorName?: string
}

export interface TwitchDayClipsResponse {
  items: TwitchDayClip[]
}

export interface StreamSummaryMetrics {
  sync_health_state?: string
  data_coverage_pct?: number
  minutesWithData?: number
  viewerSampleCount?: number
}

export interface StreamSummaryResponse {
  channel?: string
  metrics?: StreamSummaryMetrics
  analyticsQuality?: string
  updatedAt?: number
}

export interface AnalyticsApi {
  ensureChannelEmotes(login: string, twitchId: string, providers?: string[]): Promise<unknown>
  getAnalyticsStream(streamId: string, opts?: AnalyticsStreamOptions): Promise<unknown | null>
  getStreamSummary?(streamId: string, channel?: string): Promise<StreamSummaryResponse | null>
  getAnalyticsStreams(login: string, limit?: number): Promise<unknown>
  getPulseBookmarks(params?: PulseBookmarkQuery): Promise<unknown>
  getPulseStreamRecap(streamId: string): Promise<unknown | null>
  getTimeseriesStatus(): Promise<unknown>
  createPulseBookmark(input: Record<string, unknown>): Promise<unknown>
  deletePulseBookmark(id: string): Promise<void>
  prefetchAnalyticsTracker(streamId: string, channel: string): Promise<{ status: string }>
  getChannel(login: string): Promise<unknown>
  getChannelStreamHistory(login: string, period?: string): Promise<unknown>
  watchAnalyticsChannel(login: string): Promise<unknown>
  getAnalyticsLive(login: string): Promise<unknown>
  getSyncStatus(streamId: string): Promise<unknown | null>
  startHistoricalSync(streamId: string, login?: string, options?: StartHistoricalSyncOptions): Promise<unknown>
  getStreamGameSegments(streamId: string): Promise<unknown>
  getReplayHeatmap(streamId: string, window?: number, channel?: string): Promise<unknown | null>
  getReplayHeatmapDetail(streamId: string, window?: number, channel?: string): Promise<unknown | null>
  getVodStoryboardThumb(vodId: string, offsetSec: number): Promise<unknown | null>
  getTwitchDayClips(login: string, startedAt: string, endedAt: string): Promise<TwitchDayClipsResponse>
  getSetupWelcome(): Promise<SetupWelcome>
}

let configuredAnalyticsApi: AnalyticsApi | null = null
let emoteAssetBaseResolver: (() => string) | null = null

export function configureAnalyticsApi(api: AnalyticsApi): void {
  configuredAnalyticsApi = api
}

export function getConfiguredAnalyticsApi(): AnalyticsApi {
  if (!configuredAnalyticsApi) throw new Error('Analytics API has not been configured')
  return configuredAnalyticsApi
}

export function configureEmoteAssetBase(resolveBase: () => string): void {
  emoteAssetBaseResolver = resolveBase
}

export function resolveEmoteAssetUrl(path: string | null | undefined): string {
  if (!path) return ''
  if (/^https?:\/\//i.test(path) || path.startsWith('data:')) return path
  const base = emoteAssetBaseResolver?.().replace(/\/+$/, '') ?? ''
  const suffix = path.startsWith('/') ? path : `/${path}`
  return `${base}${suffix}`
}
