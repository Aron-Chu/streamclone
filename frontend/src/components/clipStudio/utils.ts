import type { ChannelEmote, ClipperJob } from '../../api'
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
