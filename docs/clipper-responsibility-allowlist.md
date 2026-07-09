# Streamclone Clipper Responsibility Allow-List

Streamclone integrates with the clip pipeline over **HTTP only**. Clip render,
transcription, editor/export, durable artifacts, and signed playback belong to
**ReplayForge** (sibling repo, host `:8095`). This document is the human-readable
inventory of the **only** clipper-related responsibilities Streamclone Go is
permitted to own (Requirement 1.6 of the
`auto-clipper-replayforge-productization` spec).

The authoritative, machine-readable source of truth is
[`internal/boundaryguard/allowlist.go`](../internal/boundaryguard/allowlist.go)
(`ClipperResponsibilityAllowList`). The boundary guard test consults it, so any
render-touching package or route outside this list trips review. If the two ever
disagree, the Go file wins — update it first, then mirror the change here.

## Allowed responsibilities

| Responsibility | Requirement | Permitted surface | Route(s) |
|---|---|---|---|
| `moment_context` | 1.6 | Build the `moment_context` payload (channel, `vod_id`, start/end, reason) from Streamclone Analytics moments. No source download, no render. | `/v1/triggers/manual` |
| `export_moment_trigger` | 1.6 | Send an authenticated `Clip_Job` creation request (`moment_context` + `idempotency_key`) to ReplayForge and store the returned job id. Trigger only. | `/v1/jobs` |
| `studio_redirect` | 1.6 | Resolve a job id and redirect `/studio` to the ReplayForge Clip Studio URL. A redirect, not an editor. | `/studio` |
| `job_mirror` | 1.6 | Hold a read model of mirrored `Job_State` (from `Job_State_Set`), updated only via the authed idempotent `Status_Callback` and reconciled to the ReplayForge `Job_Store`. Display only. | `/v1/clipper/callback` |
| `callback_auth` | 1.6 | Authenticate `Status_Callback` and `Clip_Job` mutation requests; reject missing/invalid `Auth_Token` with `401`. Auth boundary only. | `/v1/clipper/callback` |

The same-origin `/v1/clipper/*` proxy is passthrough routing to host ReplayForge
`:8095`; it forwards requests and does not itself render, so it is not a render
route. The boundary guard additionally re-asserts the Phase 5 non-Go integration
surface — the frontend `/studio` redirect + Recent-Clips link builder
(`frontend/src/utils/studioLink.ts`, `frontend/src/components/StudioRedirect.tsx`)
and the Caddy `@clipper` proxy block (`deploy/Caddyfile`,
`deploy/Caddyfile.local-tunnel`) — confirming those only redirect, mirror, and
passthrough to `:8095` and carry no FFmpeg/Whisper/transcription/burn-in code.

## What is NOT on the allow-list (ReplayForge-owned)

- FFmpeg clip render / re-encode / filter / caption burn-in
- Whisper transcription
- Clip editor and export / export profiles
- Durable artifact storage (Cloudflare R2) and signed playback
- VOD/live source acquisition (Streamlink/FFmpeg download for clips)

Playback relay (`internal/video/*`) legitimately remuxes HLS with `-c copy` and
is a separate, Streamclone-owned playback responsibility — not clip render. It is
allow-listed only for that purpose by the boundary guard.

## How the guard uses this list

The guard in `internal/boundaryguard/` treats clip render as outside the
allow-list and trips review when a Streamclone Go package or route touches clip
render, detected by:

- **Render markers** — FFmpeg encode/filter tokens, Whisper, editor/export
  symbols (also covered by the Requirements 1.1–1.3 render-marker guard).
- **Package path** — a package whose path combines a clip marker with a
  render/edit/export verb (e.g. `internal/clipexport/`, `internal/clipeditor/`),
  even without a known FFmpeg/Whisper token.
- **Route literal** — a clip render route served from Streamclone Go
  (e.g. `/v1/clipper/render`).

If clipper render code needs to exist, it belongs in ReplayForge.
