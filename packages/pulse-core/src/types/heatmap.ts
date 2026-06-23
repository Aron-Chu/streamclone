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

export interface ReplayHeatmapPoint {
  offsetSeconds: number
  durationSeconds: number
  score: number
  confidence: number
  reason: string
  topEmotes: HeatmapEmote[]
  vodId: string | null
  streamId: string
  minuteTs: string
}

export interface ReplayHeatmapDetailPoint extends ReplayHeatmapPoint {
  components: Record<string, SignalComponent>
}
