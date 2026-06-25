/** Shared saved-moment reference — portal, extension bookmarks, future clip queue. */

export interface MomentRef {
  login: string
  streamId: string
  offsetSeconds: number
  vodId?: string
  label?: string
  source?: string
  createdAt?: string
  /** Backend-owned display field — never invent on create. */
  score?: number
}

export interface BuildMomentRefInput {
  login: string
  streamId: string
  offsetSeconds: number
  vodId?: string
  label?: string
  source?: string
  createdAt?: string
  score?: number
}

export interface BookmarkLike {
  login?: string
  streamId?: string
  vodId?: string
  offsetSeconds?: number
  label?: string
  source?: string
  createdAt?: string
  score?: number
}

export interface PortalStreamPathOptions {
  /** When false, stream analytics path is withheld (caller shows disabled affordance). */
  streamAnalyticsAvailable?: boolean
}

export interface CreateBookmarkPayload {
  login?: string
  streamId?: string
  vodId?: string
  offsetSeconds?: number
  label?: string
  notes?: string
  score?: number
  source?: 'web' | 'extension' | string
  /** Reject saves from estimated / fallback heat points. */
  estimated?: boolean
}

export type NormalizedBookmarkCreate = {
  login: string
  streamId: string
  offsetSeconds: number
  label: string
  notes: string
  vodId: string
  source: 'web' | 'extension'
  score?: number
}

export type NormalizeBookmarkResult =
  | { ok: true; value: NormalizedBookmarkCreate }
  | { ok: false; code: string; message: string }

const LOGIN_RE = /^[a-z0-9_]{1,25}$/

export function normalizeLogin(raw?: string): string {
  return (raw ?? '').trim().toLowerCase()
}

export function normalizeStreamId(raw?: string): string {
  return (raw ?? '').trim()
}

export function isValidLogin(login: string): boolean {
  return LOGIN_RE.test(login)
}

export function isValidOffsetSeconds(offsetSeconds: unknown): offsetSeconds is number {
  return typeof offsetSeconds === 'number' && Number.isFinite(offsetSeconds) && offsetSeconds >= 0
}

export function buildMomentRef(input: BuildMomentRefInput): MomentRef | null {
  const login = normalizeLogin(input.login)
  const streamId = normalizeStreamId(input.streamId)
  if (!isValidLogin(login) || !streamId || !isValidOffsetSeconds(input.offsetSeconds)) {
    return null
  }

  const ref: MomentRef = {
    login,
    streamId,
    offsetSeconds: Math.floor(input.offsetSeconds),
  }

  const vodId = (input.vodId ?? '').trim()
  if (vodId) ref.vodId = vodId

  const label = (input.label ?? '').trim()
  if (label) ref.label = label

  const source = (input.source ?? '').trim()
  if (source) ref.source = source

  const createdAt = (input.createdAt ?? '').trim()
  if (createdAt) ref.createdAt = createdAt

  if (typeof input.score === 'number' && Number.isFinite(input.score)) {
    ref.score = input.score
  }

  return ref
}

export function isValidMomentRef(ref: unknown): ref is MomentRef {
  if (!ref || typeof ref !== 'object') return false
  const candidate = ref as MomentRef
  return (
    buildMomentRef({
      login: candidate.login,
      streamId: candidate.streamId,
      offsetSeconds: candidate.offsetSeconds,
      vodId: candidate.vodId,
      label: candidate.label,
      source: candidate.source,
      createdAt: candidate.createdAt,
      score: candidate.score,
    }) !== null
  )
}

export function bookmarkToMomentRef(bookmark: BookmarkLike): MomentRef | null {
  if (bookmark.offsetSeconds == null) return null
  return buildMomentRef({
    login: bookmark.login ?? '',
    streamId: bookmark.streamId ?? '',
    offsetSeconds: bookmark.offsetSeconds,
    vodId: bookmark.vodId,
    label: bookmark.label,
    source: bookmark.source,
    createdAt: bookmark.createdAt,
    score: bookmark.score,
  })
}

export function momentRefToPortalChannelPath(ref: MomentRef): string | null {
  if (!isValidMomentRef(ref)) return null
  return `/dashboard/c/${encodeURIComponent(ref.login)}`
}

export function momentRefToPortalStreamPath(
  ref: MomentRef,
  options: PortalStreamPathOptions = {},
): string | null {
  if (options.streamAnalyticsAvailable === false) return null
  if (!isValidMomentRef(ref)) return null
  const streamId = normalizeStreamId(ref.streamId)
  if (!streamId) return null

  const base = `/dashboard/c/${encodeURIComponent(ref.login)}/s/${encodeURIComponent(streamId)}`
  if (ref.offsetSeconds > 0) {
    return `${base}?t=${Math.floor(ref.offsetSeconds)}`
  }
  return base
}

export function momentRefToTwitchVodUrl(ref: MomentRef): string | null {
  if (!isValidMomentRef(ref)) return null
  const vodId = (ref.vodId ?? '').trim()
  if (!vodId) return null
  const t = Math.max(0, Math.floor(ref.offsetSeconds))
  return `https://www.twitch.tv/videos/${encodeURIComponent(vodId)}?t=${t}s`
}

export function normalizeCreateBookmarkInput(input: CreateBookmarkPayload): NormalizeBookmarkResult {
  if (input.estimated === true) {
    return {
      ok: false,
      code: 'estimated_moment',
      message: 'Estimated fallback moments cannot be saved.',
    }
  }

  const login = normalizeLogin(input.login)
  const streamId = normalizeStreamId(input.streamId)

  if (!login || !isValidLogin(login)) {
    return { ok: false, code: 'invalid_login', message: 'Login is required and must be valid.' }
  }
  if (!streamId) {
    return { ok: false, code: 'invalid_stream_id', message: 'Stream id is required.' }
  }
  if (!isValidOffsetSeconds(input.offsetSeconds)) {
    return { ok: false, code: 'invalid_offset', message: 'Offset must be a non-negative number.' }
  }

  const source: 'web' | 'extension' = input.source === 'extension' ? 'extension' : 'web'
  const value: NormalizedBookmarkCreate = {
    login,
    streamId,
    offsetSeconds: Math.floor(input.offsetSeconds),
    label: (input.label ?? '').trim(),
    notes: (input.notes ?? '').trim(),
    vodId: (input.vodId ?? '').trim(),
    source,
  }

  if (
    source === 'extension' &&
    typeof input.score === 'number' &&
    Number.isFinite(input.score)
  ) {
    value.score = input.score
  }

  return { ok: true, value }
}
