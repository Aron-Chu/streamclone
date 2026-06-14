import { formatDuration } from './durationFormat.ts'

const REASON_LABELS: Record<string, string> = {
  chat_spike: 'chat spike',
  seventv_spike: '7TV spike',
  twitch_emote_spike: 'Twitch emote spike',
  ffz_spike: 'FFZ spike',
  viewer_spike: 'viewer spike',
  game_change: 'game change',
  manual: 'manual',
}

export function buildHeatmapPeakAriaLabel(offsetSeconds: number, score: number, reason: string): string {
  const offset = formatDuration(offsetSeconds)
  const label = REASON_LABELS[reason] ?? reason
  return `Moment at ${offset}, score ${score}, ${label}`
}
