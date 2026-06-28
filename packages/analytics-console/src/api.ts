import { getConfiguredAnalyticsApi } from './configureApi.ts'
import type {
  AnalyticsStreamDetail,
  AnalyticsStreamsResponse,
  GameSegment,
  PulseBookmark,
  PulseStreamRecap,
  SyncStatus,
} from './apiTypes.ts'

export type {
  AnalyticsMinuteRollup,
  AnalyticsStream,
  AnalyticsStreamDetail,
  AnalyticsStreamsResponse,
  AnalyticsTopEmote,
  GameSegment,
  PulseBookmark,
  PulseStreamRecap,
  SourceStatus,
  SyncPhase,
  SyncStatus,
} from './apiTypes.ts'

export type AnalyticsViewMode = 'overview' | 'emotes' | 'spikes'

const api = () => getConfiguredAnalyticsApi()

export async function ensureChannelEmotes(login: string, twitchId: string, providers?: string[]) {
  return api().ensureChannelEmotes(login, twitchId, providers)
}

export async function getAnalyticsStream(
  streamId: string,
  opts?: { sparse?: boolean; channel?: string },
): Promise<AnalyticsStreamDetail | null> {
  return api().getAnalyticsStream(streamId, opts) as Promise<AnalyticsStreamDetail | null>
}

export async function getAnalyticsStreams(login: string, limit = 20): Promise<AnalyticsStreamsResponse> {
  return api().getAnalyticsStreams(login, limit) as Promise<AnalyticsStreamsResponse>
}

export async function getAnalyticsLive(login: string): Promise<AnalyticsStreamDetail> {
  return api().getAnalyticsLive(login) as Promise<AnalyticsStreamDetail>
}

export async function getPulseBookmarks(params: Record<string, unknown> = {}) {
  return api().getPulseBookmarks(params)
}

export async function getPulseStreamRecap(streamId: string): Promise<PulseStreamRecap | null> {
  return api().getPulseStreamRecap(streamId) as Promise<PulseStreamRecap | null>
}

export async function getTimeseriesStatus() {
  return api().getTimeseriesStatus()
}

export async function createPulseBookmark(input: Record<string, unknown>) {
  return api().createPulseBookmark(input)
}

export async function deletePulseBookmark(id: string) {
  return api().deletePulseBookmark(id)
}

export async function prefetchAnalyticsTracker(streamId: string, channel: string) {
  return api().prefetchAnalyticsTracker(streamId, channel)
}

export async function getChannel(login: string) {
  return api().getChannel(login)
}

export async function getChannelStreamHistory(login: string, period?: string) {
  return api().getChannelStreamHistory(login, period)
}

export async function watchAnalyticsChannel(login: string) {
  return api().watchAnalyticsChannel(login)
}

export async function getSyncStatus(streamId: string): Promise<SyncStatus | null> {
  return api().getSyncStatus(streamId) as Promise<SyncStatus | null>
}

export async function startHistoricalSync(
  streamId: string,
  login = '',
  options?: { viewersOnly?: boolean; vodId?: string; forceChat?: boolean },
) {
  return api().startHistoricalSync(streamId, login, options)
}

export async function getStreamGameSegments(streamId: string): Promise<GameSegment[]> {
  return api().getStreamGameSegments(streamId) as Promise<GameSegment[]>
}

export async function getReplayHeatmap(streamId: string, window = 60, channel?: string) {
  return api().getReplayHeatmap(streamId, window, channel)
}

export async function getReplayHeatmapDetail(streamId: string, window = 60, channel?: string) {
  return api().getReplayHeatmapDetail(streamId, window, channel)
}

export async function getSetupWelcome() {
  return api().getSetupWelcome()
}
