// Feature: moment-timeline — shared heatmap response types.
// Mirror of the Go structs in internal/analytics/heatmap (design Data Models).

export interface SignalComponent {
  rawScore: number
  weightedScore: number
  confidence: number
}

export interface HeatmapEmote {
  id: string
  name: string
  imageUrl: string
  count: number
  provider: string
}

// Compact point (default response, fits the 50 KB budget).
export interface ReplayHeatmapPoint {
  offsetSeconds: number
  durationSeconds: number
  score: number
  confidence: number
  reason: string
  topEmotes: HeatmapEmote[]
  vodId: string | null
  streamId: string
  minuteTs: string // ISO 8601
}

// Detail point (returned with ?detail=true, includes per-signal breakdown).
export interface ReplayHeatmapDetailPoint extends ReplayHeatmapPoint {
  components: Record<string, SignalComponent>
}

export interface HeatmapResponse {
  streamId: string
  windowSeconds: number
  confidence: number
  scoringVersion: string
  updatedAt: number
  points: ReplayHeatmapPoint[]
}

export interface HeatmapDetailResponse {
  streamId: string
  windowSeconds: number
  confidence: number
  scoringVersion: string
  updatedAt: number
  points: ReplayHeatmapDetailPoint[]
}
