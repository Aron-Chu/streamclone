export interface ChartMinuteRollup {
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

export interface ChartGameSegment {
  id?: number | string
  gameName: string
  boxArtUrl?: string
  offsetSeconds: number
  durationSeconds: number
  /** Response-only honesty: e.g. snapshot, stored, category_fallback */
  source?: string
}

export interface ChartPlayhead {
  streamId: string
  offsetSeconds: number
  isPlaying: boolean
}
