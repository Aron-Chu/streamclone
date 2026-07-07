export interface SourceStatus {
  source: string
  state: string
  label?: string
}

export interface AnalyticsStream {
  streamId: string
  broadcasterId?: string
  canonicalStreamId?: string
  login: string
  displayName?: string
  title?: string
  category?: string
  gamesSummary?: string
  startedAt: string
  endedAt?: string | null
  lastSeenAt?: string
  currentViewers?: number
  peakViewers?: number
  avgViewers?: number
  viewerSamples?: number
  chatMessages?: number
  vodId?: string
}

export interface ChannelEmote {
  name: string
  emote_id: string
  url: string
  zw: boolean
  provider?: string
}

export interface ChatCoverageSummary {
  coveragePct?: number
  chatSpanMinutes?: number
  streamSpanMinutes?: number
  partial?: boolean
}

export interface AnalyticsMinuteRollup {
  minuteTs: string
  viewerAvg?: number
  viewerMax?: number
  viewerLatest?: number
  viewerSamples?: number
  chatCount?: number
  totalEmoteCount?: number
  seventvEmoteCount?: number
  emotes?: Record<string, number>
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
  state: 'live' | 'historical' | 'not_collected' | 'syncing' | string
  stream?: AnalyticsStream
  rollups: AnalyticsMinuteRollup[]
  /** Full timeline rollups for moments panel; chart uses downsampled `rollups`. */
  momentRollups?: AnalyticsMinuteRollup[]
  topEmotes: AnalyticsTopEmote[]
  sources: SourceStatus[]
  updatedAt: number
  vodId?: string
  vodAlignSeconds?: number
  syncPhase?: string
  chatCoveragePct?: number
  chatCoverage?: ChatCoverageSummary
  viewerSource?: string
  timelineMinutes?: number
  analyticsQuality?: string
  coverageStartOffsetSeconds?: number
  /** Hosted portal: detail loaded but /minutes fetch failed or returned empty. */
  minutesUnavailable?: boolean
}

export interface AnalyticsStreamsResponse {
  channel: string
  items: AnalyticsStream[]
  sources: SourceStatus[]
  updatedAt: number
}

export interface GameSegment {
  id: number
  streamId: string
  gameName: string
  boxArtUrl: string
  offsetSeconds: number
  durationSeconds: number
  createdAt: string
  source?: string
}

export type SyncPhase =
  | 'starting'
  | 'scraping_tracker'
  | 'parsing_tracker'
  | 'resolving_vod'
  | 'fetching_comments'
  | 'writing_rollups'
  | 'exporting_archive'
  | 'export_pending'
  | 'completed'
  | 'failed'
  | string

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

export interface SyncStatus {
  streamId: string
  phase: SyncPhase
  message?: string
  startedAt?: string
  updatedAt: string
  stale?: boolean
  error?: string
  viewerStatus?: string
  rollupsWritten?: number
  viewersOnly?: boolean
}

export interface PulseBookmark {
  id: string
  login?: string
  streamId?: string
  label?: string
}

export interface PulseRecapEmote {
  code: string
  count: number
  provider?: string
  id?: string
  imageUrl?: string
}

export interface PulseRecapMoment {
  offsetSeconds: number
  score: number
  reasons?: string[]
  topEmotes?: PulseRecapEmote[]
}

export interface PulseStreamRecap {
  streamId: string
  login?: string
  vodId?: string
  durationSeconds?: number
  totalMessages?: number
  peakChatPerMin?: number
  topMoments?: PulseRecapMoment[]
  topEmotes?: PulseRecapEmote[]
  biggestChatSpike?: {
    offsetSeconds: number
    chatPerMin: number
  }
  funniestEmoteBurst?: {
    offsetSeconds: number
    code?: string
    count: number
  }
  clipCandidates?: PulseRecapMoment[]
}
