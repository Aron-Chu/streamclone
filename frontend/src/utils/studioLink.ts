// Studio-link builder for the Recent Clips listing (spec
// auto-clipper-replayforge-productization, RF-P5-005 / Requirement 6.3).
//
// The Recent Clips listing links each mirrored Clip_Job out to the ReplayForge
// Clip Studio via Streamclone's `/studio` redirect. These builders are the
// single, pure source of truth for that link so the listing, the retry
// redirect, and the `/studio` redirect all agree on the shape.
//
// Invariants enforced here (and asserted by tests):
//   - The link is derived ONLY from the opaque ReplayForge job id (and the
//     configured Clip Studio base for the external URL). No auth/access/refresh
//     or clip token is ever placed in the path — Streamclone owns the redirect,
//     ReplayForge owns credentials.
//   - A blank/whitespace job id resolves to the archive index (`/studio`) rather
//     than a dangling `/studio/` segment.

const STUDIO_ROOT = '/studio'

/**
 * studioPath returns the in-app SPA route for opening a Clip_Job in Clip Studio.
 * The job id is the only dynamic input and is URL-encoded so opaque ids remain
 * safe path segments. A blank id resolves to the archive root.
 */
export function studioPath(jobId?: string | null): string {
  const trimmed = (jobId ?? '').trim()
  if (!trimmed) {
    return STUDIO_ROOT
  }
  return `${STUDIO_ROOT}/${encodeURIComponent(trimmed)}`
}

/**
 * replayforgeStudioUrl resolves a Clip_Job id to the absolute ReplayForge Clip
 * Studio URL that the `/studio` redirect sends the browser to. It normalizes a
 * trailing slash on the configured base and reuses studioPath so the job-id
 * segment is built identically to the in-app link.
 */
export function replayforgeStudioUrl(baseUrl: string, jobId?: string | null): string {
  const base = (baseUrl ?? '').replace(/\/+$/, '')
  return `${base}${studioPath(jobId)}`
}
