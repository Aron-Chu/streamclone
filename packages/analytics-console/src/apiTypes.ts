export interface SourceStatus {
  source: string
  state: string
  label?: string
}

export interface AnalyticsStream {
  streamId: string
  login: string
  displayName?: string
  title?: string
  category?: string
  startedAt: string
  endedAt?: string | null
  currentViewers?: number
  peakViewers?: number
  vodId?: string
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
  state: 'live' | 'historical' | 'not_collected' | 'syncing' | string
  stream?: AnalyticsStream
  rollups: AnalyticsMinuteRollup[]
  topEmotes: AnalyticsTopEmote[]
  sources: SourceStatus[]
  updatedAt: number
  vodId?: string
  syncPhase?: string
  chatCoveragePct?: number
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

export interface SyncStatus {
  streamId: string
  phase: SyncPhase
  message?: string
  startedAt?: string
  updatedAt: string
  stale?: boolean
  error?: string
  viewerStatus?: string
}

export interface PulseBookmark {
  id: string
  login?: string
  streamId?: string
  label?: string
}

export interface PulseStreamRecap {
  streamId: string
  clipCandidates?: unknown[]
}
