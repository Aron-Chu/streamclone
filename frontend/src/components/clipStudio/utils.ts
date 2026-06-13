import type { ChannelEmote, ClipperJob, ClipperMomentContext } from '../../api'
import { normalizeBrowserOriginUrl } from '../../config'

export function formatTime(seconds: number): string {
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m}:${s.toFixed(1).padStart(s < 10 ? 4 : 3, '0')}`
}

export function parseTimeInput(value: string): number | null {
  if (value.includes(':')) {
    const [min, sec] = value.split(':')
    const parsed = parseFloat(min) * 60 + parseFloat(sec)
    return isNaN(parsed) ? null : parsed
  }
  const parsed = parseFloat(value)
  return isNaN(parsed) ? null : parsed
}

export function buildEmoteMap(emotes: ChannelEmote[]): Record<string, string> {
  const map: Record<string, string> = {}
  for (const emote of emotes) {
    const key = emote.name.trim()
    if (!key) continue
    map[key] = normalizeBrowserOriginUrl(emote.url, ['/emotes/'])
  }
  return map
}

export function spikePositionInSource(job: ClipperJob, sourceDuration: number): number | null {
  const ctx = job.moment_context
  if (!ctx || ctx.vod_offset_seconds == null) return null
  if (ctx.vod_segment_start != null) {
    return Math.max(0, Math.min(sourceDuration, ctx.vod_offset_seconds - ctx.vod_segment_start))
  }
  const clipStart = Math.max(0, (ctx.vod_offset_seconds ?? 0) - (job.source_duration / 2))
  return Math.max(0, Math.min(sourceDuration, (ctx.vod_offset_seconds ?? 0) - clipStart))
}

export function buildUploadPackage(job: ClipperJob, trimDuration: number): string {
  const ctx = job.moment_context
  const title = job.title || `${job.channel} highlight`
  const hashtags = ['#twitch', `#${job.channel}`, '#gaming', '#clips', '#shorts'].join(' ')
  const lines = [
    `Title: ${title}`,
    `Hashtags: ${hashtags}`,
    '',
    job.twitch_clip_url ? `Twitch clip: ${job.twitch_clip_url}` : 'Twitch clip: (pending)',
    '',
    'Moment stats:',
    ctx?.viewer_count != null ? `  Viewers: ${ctx.viewer_count}` : null,
    ctx?.chat_per_min != null ? `  Chat: ${ctx.chat_per_min}/min` : null,
    ctx?.emote_per_min != null ? `  Emotes: ${ctx.emote_per_min}/min` : null,
    ctx?.chat_multiplier != null ? `  Chat multiplier: ${ctx.chat_multiplier.toFixed(1)}x` : null,
    ctx?.moment_score != null ? `  Moment score: ${ctx.moment_score.toFixed(2)}` : null,
    ctx?.pick_reason ? `  Pick reason: ${ctx.pick_reason}` : null,
    job.reason ? `  Trigger: ${job.reason}` : null,
    '',
    `Export window: ${trimDuration.toFixed(1)}s`,
    'Download: use Final MP4 in Clip Studio or GET /v1/jobs/{id}/final.mp4',
  ].filter((line): line is string => Boolean(line))
  return lines.join('\n')
}

export function pickReasonLabel(reason?: string): string {
  switch (reason) {
    case 'viewer_spike': return 'Viewer spike'
    case 'chat_spike': return 'Chat spike'
    case 'emote_spike': return 'Emote spike'
    case 'manual': return 'Manual pick'
    default: return reason || 'Highlight moment'
  }
}

export function hookStrengthScore(ctx?: ClipperMomentContext | null): number | null {
  if (!ctx) return null
  let score = 50
  if (ctx.moment_score != null) {
    score = Math.round(Math.min(100, Math.max(0, ctx.moment_score * 25)))
  }
  if (ctx.chat_multiplier != null) {
    score = Math.round(Math.min(100, score + (ctx.chat_multiplier - 1) * 12))
  }
  if (ctx.emote_per_min != null && ctx.emote_per_min > 0) {
    score = Math.round(Math.min(100, score + Math.min(15, ctx.emote_per_min / 4)))
  }
  return Math.max(0, Math.min(100, score))
}

export function predictedReachLabel(score: number | null): 'High' | 'Medium' | 'Low' | null {
  if (score == null) return null
  if (score >= 75) return 'High'
  if (score >= 45) return 'Medium'
  return 'Low'
}

export function formatHighlightRange(start: number, end: number): string {
  return `${formatTime(start)} – ${formatTime(end)}`
}

export function buildActivityBars(
  duration: number,
  captions: Array<{ start: number; end: number }>,
  spikePos: number | null,
  barCount = 48,
): number[] {
  if (duration <= 0) return Array(barCount).fill(0)
  const bucketSize = duration / barCount
  const bars = Array(barCount).fill(0)
  for (const cap of captions) {
    const startIdx = Math.max(0, Math.floor(cap.start / bucketSize))
    const endIdx = Math.min(barCount - 1, Math.floor(cap.end / bucketSize))
    for (let i = startIdx; i <= endIdx; i++) bars[i] += 1
  }
  if (spikePos != null) {
    const spikeIdx = Math.min(barCount - 1, Math.max(0, Math.floor(spikePos / bucketSize)))
    bars[spikeIdx] += 3
    if (spikeIdx > 0) bars[spikeIdx - 1] += 1
    if (spikeIdx < barCount - 1) bars[spikeIdx + 1] += 1
  }
  const max = Math.max(...bars, 1)
  return bars.map(v => v / max)
}
