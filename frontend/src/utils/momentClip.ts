import type { ReplayHeatmapPoint } from '../types/heatmap'
import type { ClipperMomentContext } from '../api'
import { REPLAYFORGE_UI } from '../config'

export interface ClipRequest {
  channel: string
  streamId: string
  vodId?: string
  offsetSeconds: number
  score: number
  reason: string
  topEmotes?: Array<{ id: string; name: string; count: number }>
  momentContext: ClipperMomentContext
}

export type ClipperAuthError =
  | 'missing_scope'
  | 'invalid_token'
  | 'twitch_not_configured'
  | 'vod_auth_failed'

const CLIPPER_AUTH_ERRORS: ReadonlySet<string> = new Set([
  'missing_scope',
  'invalid_token',
  'twitch_not_configured',
  'vod_auth_failed',
])

export function isClipperAuthError(errorCode: string): errorCode is ClipperAuthError {
  return CLIPPER_AUTH_ERRORS.has(errorCode)
}

export function buildClipRequest(
  point: ReplayHeatmapPoint,
  streamId: string,
  vodId: string | undefined,
  channel: string,
): ClipRequest {
  const topEmotes = (point.topEmotes ?? []).slice(0, 5).map(e => ({
    id: e.id,
    name: e.name,
    count: e.count,
  }))

  const momentContext: ClipperMomentContext = {
    stream_id: streamId,
    minute_ts: point.minuteTs,
    vod_id: vodId,
    vod_offset_seconds: point.offsetSeconds,
    source_kind: vodId ? 'vod' : 'live',
    moment_score: point.score,
    top_emotes: (point.topEmotes ?? []).slice(0, 5).map(e => ({
      name: e.name,
      count: e.count,
      image_url: e.imageUrl,
    })),
    pick_reason: point.reason as ClipperMomentContext['pick_reason'],
  }

  return {
    channel,
    streamId,
    vodId: vodId || undefined,
    offsetSeconds: point.offsetSeconds,
    score: point.score,
    reason: point.reason,
    topEmotes: topEmotes.length > 0 ? topEmotes : undefined,
    momentContext,
  }
}

export function selectBatchClipCandidates(
  points: ReplayHeatmapPoint[],
  existingJobOffsets: Set<number>,
  chatCount: number,
  streamDurationMin: number,
  minChatMinutes: number = 10,
): ReplayHeatmapPoint[] {
  if (chatCount <= 0 || streamDurationMin < minChatMinutes) {
    return []
  }

  const filtered = points.filter(p => {
    const offsetMinute = Math.floor(p.offsetSeconds / 60)
    return !existingJobOffsets.has(offsetMinute)
  })

  const sorted = [...filtered].sort((a, b) => b.score - a.score)
  return sorted.slice(0, 5)
}

export function clipStudioUrl(jobId: string): string {
  const ui = REPLAYFORGE_UI.replace(/\/$/, '')
  if (ui) {
    return `${ui}/studio/${encodeURIComponent(jobId)}`
  }
  return `/studio/${encodeURIComponent(jobId)}`
}

export interface ClipperAuthErrorInfo {
  code: ClipperAuthError
  title: string
  description: string
  remediation: string
}

const AUTH_ERROR_INFO: Record<ClipperAuthError, Omit<ClipperAuthErrorInfo, 'code'>> = {
  missing_scope: {
    title: 'Missing Twitch scope',
    description: 'The Twitch token is missing the clips:edit scope required for clip creation.',
    remediation: 'Run make twitch-local-auth with clips:edit scope, update .env, and restart the clipper container.',
  },
  invalid_token: {
    title: 'Token expired or revoked',
    description: 'The clipper Twitch token is expired or has been revoked.',
    remediation: 'Run make twitch-local-auth, approve the Twitch login, then recreate the clipper service.',
  },
  twitch_not_configured: {
    title: 'Twitch not configured',
    description: 'Clipper Twitch credentials are not configured.',
    remediation: 'Set CLIPPER_TWITCH_CLIENT_ID and CLIPPER_TWITCH_USER_ACCESS_TOKEN in .env, then restart clipper.',
  },
  vod_auth_failed: {
    title: 'VOD authentication failed',
    description: 'VOD export could not authenticate with Twitch.',
    remediation: 'Run make twitch-local-auth, update .env, and recreate the clipper service.',
  },
}

export function getClipperAuthErrorInfo(code: string): ClipperAuthErrorInfo | null {
  if (!isClipperAuthError(code)) return null
  return { code, ...AUTH_ERROR_INFO[code] }
}
