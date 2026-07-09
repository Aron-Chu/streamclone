# Design Document

## Overview

This design specifies how to finish the **Auto Clipper + ReplayForge productization** to a production-ready **private beta**. It is organized by the same nine delivery phases as the requirements so the downstream task list can be emitted directly as a phased backlog (**RF-P0 … RF-P8**).

ReplayForge is the **primary owner**: it owns clip jobs, the SQLite `Job_Store`, FFmpeg render, Whisper transcription, the Clip Studio editor, export, durable artifact storage on Cloudflare R2, and playback. Streamclone integrates over **HTTP only**: Analytics moments, moment context, the Export Moment trigger, the `/studio` redirect, the mirrored `Job_State`, and callback authentication. `streampulse-ops` (private) owns production environment, secrets, deploy, and image promotion. Public repositories document **contracts only**.

Languages used across the boundary:
- **ReplayForge backend/worker:** Python (API + `Render_Worker`).
- **Streamclone services:** Go (`net/http`, chi).
- **Clip Studio + Streamclone frontends:** Vite + React + TypeScript (shadcn/Radix-style component set).
- **Ops/packaging:** Docker Compose + private `streampulse-ops` manifests.

Code examples below are structured pseudocode annotated with the target language; they define contracts, not final implementations.

### Hard gates (non-negotiable)

- No FFmpeg / render / Whisper / editor / export code in Streamclone Go services.
- No tokens (clip, access, refresh, auth) in bundles, URLs, filenames, logs, keys, or display strings.
- No unauthenticated endpoint that mutates `Clip_Job` state on either side.
- No client-side clip-candidate (Pulse) scoring — scoring is server-side.
- No "image cutover complete" claim without private-ops evidence.
- No "production-ready" claim for Auto Clipper without R2 durable storage **plus** signed-playback evidence.

### Relationship to the ReplayForge Trend Formula roadmap

The existing ReplayForge `docs/tasks.md` is the **Trend Formula Engine** roadmap — a distinct lens focused on scoring/formula-driven export profiles. This design does **not** conflate the two. They intersect only at three shared surfaces, which must stay compatible but are specified independently here:

| Shared surface | This design (productization) | Trend Formula roadmap |
|---|---|---|
| `Render_Worker` | Single serial worker, state machine, artifact upload | Formula-selected export profiles feed render params |
| Export profiles | Template/audio/export controls in Clip Studio | Formula engine may auto-pick a profile |
| Artifact storage | R2 manifest, retention, signed playback | Consumes stored artifacts for formula analytics |

Cross-reference only; do not merge phase numbering or ownership.

---

## Architecture

### Ownership map / boundary architecture (4 repos)

```
                    ┌─────────────────────────────────────────────┐
                    │ streampulse-ops (PRIVATE)                    │
                    │  • prod env + secrets                        │
                    │  • deploy manifests, image promotion (digest)│
                    │  • rollback plane                            │
                    └───────────────┬─────────────────────────────┘
                                    │ promotes by digest / injects env
                                    ▼
  ┌──────────────────────┐   HTTP (same-origin /v1/clipper/*)   ┌───────────────────────────┐
  │ streamclone (Go+TS)  │ ───────────────────────────────────▶ │ replayforge (Py+TS) PRIMARY│
  │ TRIGGER / MIRROR /    │                                      │ OWNER                      │
  │ REDIRECT only         │ ◀─── Status_Callback (authed) ────── │  • Job_Store (SQLite = SoT)│
  │  • Analytics moments   │                                     │  • FFmpeg / Whisper        │
  │  • moment_context      │                                     │  • Clip Studio editor      │
  │  • Export Moment       │                                     │  • R2 artifact store       │
  │  • /studio redirect    │                                     │  • signed playback         │
  │  • Job_Mirror          │                                     └───────────────────────────┘
  └──────────────────────┘
  ┌──────────────────────────────┐
  │ streamclone-pulse (INDEPENDENT)│  extension + portal — MUST work with ReplayForge absent
  └──────────────────────────────┘
```

Boundary rules (Requirement 1):
- Streamclone Go retains **only**: Analytics moments, `moment_context`, Export Moment trigger, `/studio` redirect, `Job_Mirror`, callback auth.
- ReplayForge owns the full lifecycle, render, transcription, editor, export, artifacts, playback.
- streamclone-pulse has **no** ReplayForge dependency (verified by extension health path with ReplayForge down).
- No shared database — HTTP only.

## Components and Interfaces

| Component | Repo / language | Responsibility |
|---|---|---|
| `Export Moment` trigger | streamclone Go | Build `moment_context`, POST authed create to ReplayForge, store returned job id |
| `Job_Mirror` | streamclone Go + Postgres/local | Read model of `Job_State`; updated only via authed `Status_Callback`; reconciles to store |
| `Clipper Proxy` | streamclone Caddy + Go | Same-origin `/v1/clipper/*` → host ReplayForge `:8095` |
| `/studio` redirect | streamclone Go/TS | Resolve job id → ReplayForge Clip Studio URL |
| `Clip_Job API` | replayforge Python | Create/mutate jobs (authed), duplicate suppression, invite gate |
| `Job_Store` | replayforge SQLite | Single source of truth for job state |
| `Source Acquirer` | replayforge Python | VOD ownership validation, token-scoped download via Streamlink/FFmpeg argv arrays |
| `Render_Worker` | replayforge Python | Single serial worker: transcribe → render → upload |
| `Artifact_Store` client | replayforge Python | R2 upload, manifest, presigned URLs, retention/expiry |
| `Callback Emitter` | replayforge Python | Authed idempotent `Status_Callback` with bounded retry + backoff |
| `Clip Studio` SPA | replayforge Vite/React | Editor-first UX, previews, trim, captions, templates, progress, artifact library, error/retry states |

### Interfaces (HTTP contract)

| Direction | Method / path | Auth | Purpose |
|---|---|---|---|
| Streamclone → ReplayForge | `POST /v1/jobs` | `Auth_Token` | Create job with `moment_context` + `idempotency_key`; duplicate-suppressed |
| Streamclone → ReplayForge (proxied) | `/v1/clipper/*` → host `:8095` | same-origin | Editor/API passthrough |
| ReplayForge → Streamclone | `POST /v1/clipper/callback` | `Auth_Token` | Idempotent `Status_Callback` `{job_id, state, seq, updated_at}` |
| Streamclone → ReplayForge | `GET /v1/jobs/{id}` | `Auth_Token` | Reconciliation pull (SoT) |
| ReplayForge | `GET /v1/jobs/{id}/playback` / `/download` | `Auth_Token` | Returns time-limited `Signed_URL` |
| Streamclone | `GET /studio?job={id}` | session | Redirect to Clip Studio URL |
| Both | `GET /healthz` | none | Health probe |

## Data Models

### Clip_Job (ReplayForge SQLite `Job_Store` — source of truth)

```jsonc
{
  "id": "job_01H...",                 // opaque, token-free
  "broadcaster_key": "b_9f3c...",     // salted hash of broadcaster identity, not login/token
  "source": { "vod_id": "v123", "start_s": 3600, "end_s": 3690 },
  "idempotency_key": "chan:vod:start:end",   // duplicate suppression
  "state": "queued",                  // ∈ Job_State_Set
  "seq": 7,                           // monotonic; drives idempotent callbacks
  "failure_kind": null,               // recoverable|non_recoverable|null
  "retry_count": 0,
  "retention_expires_at": "2026-07-31T12:00:00Z",
  "created_at": "2026-07-01T12:00:00Z",
  "updated_at": "2026-07-01T12:05:00Z"
}
```

### Job_Mirror (Streamclone read model)

```jsonc
{
  "job_id": "job_01H...",
  "state": "rendering",               // ∈ Job_State_Set only
  "seq": 7,                           // last applied; older/equal callbacks ignored
  "last_callback_at": "2026-07-01T12:05:00Z",
  "stale": false                      // set true when ReplayForge unreachable
}
```

### Job_State_Set

`queued`, `validating_source`, `downloading_source`, `transcribing`, `ready_for_edit`, `rendering`, `rendered`, `uploading_artifact`, `complete`, `failed`, `retryable_failed`, `expired`, `source_unavailable`, `auth_required`, `vod_unavailable`.

### Artifact manifest

See the R2 artifact manifest schema in **Phase 3 (RF-P3)** below (raw source segment, transcript/caption JSON, edit recipe, rendered MP4, thumbnail/poster; token-free keys; `retention_expires_at`).

---

## Phase 0 (RF-P0) — Audit and Boundary Cleanup

**Goal:** Enforce ownership boundaries in code and docs.

Design:
- **Boundary guard test** (Go): static scan asserting `cmd/*` and `internal/*` contain no FFmpeg exec, no Whisper, no editor/export render code. Fails CI if reintroduced.
- **Responsibility inventory:** a documented allow-list of Streamclone clipper-related packages/routes (`moment_context`, Export Moment, `/studio`, `Job_Mirror`, callback auth). Anything outside the allow-list touching clip render fails review.
- **Token redaction layer** (shared pattern, both repos): a single serialization/logging chokepoint that scrubs known secret shapes (`clips:*` tokens, bearer/access/refresh tokens, auth tokens) from logs, URLs, filenames, keys, and display strings.

```python
# ReplayForge (Python) — redaction chokepoint used by logger, URL builder, key builder, display formatter
SECRET_PATTERNS = [BEARER_RE, REFRESH_RE, ACCESS_RE, AUTH_TOKEN_RE, CLIP_TOKEN_RE]

def redact(text: str) -> str:
    out = text
    for pat in SECRET_PATTERNS:
        out = pat.sub("‹redacted›", out)
    return out
# INVARIANT: every emitted string (log/url/filename/key/display) passes through redact()
```

```go
// Streamclone (Go) — same guarantee on the mirror/log/display path
func Redact(s string) string { /* strip token shapes before emit */ }
```

---

## Phase 1 (RF-P1) — Job Model Hardening + Mirror/Callback Contract

**Goal:** Honest, reliable job state consistent between ReplayForge (`Job_Store` = SoT) and Streamclone (`Job_Mirror`).

### Job state machine (full `Job_State_Set`)

States and classification:

| State | Kind | Notes |
|---|---|---|
| `queued` | active | entry / re-enqueue target |
| `validating_source` | active | ownership + scope check |
| `downloading_source` | active | Streamlink/FFmpeg argv download |
| `transcribing` | active | Whisper |
| `ready_for_edit` | active (pausable) | editor available |
| `rendering` | active | single serial worker |
| `rendered` | active | pre-upload |
| `uploading_artifact` | active | R2 upload |
| `complete` | **terminal** | signed playback/download available |
| `failed` | **terminal** | non-recoverable |
| `retryable_failed` | **retryable** | user retry → `queued` |
| `expired` | **terminal** | retention elapsed |
| `source_unavailable` | **terminal** | not owned / removed at source |
| `auth_required` | **retryable (external)** | creds absent/expired |
| `vod_unavailable` | **terminal** | VOD deleted/unavailable |

```
queued → validating_source → {source_unavailable | auth_required | vod_unavailable}
                           → downloading_source → transcribing → ready_for_edit
ready_for_edit → rendering → rendered → uploading_artifact → complete
rendering|uploading_artifact → retryable_failed  (recoverable)
any active → failed  (non-recoverable)
retryable_failed --retry--> queued
complete → expired  (retention)
```

Transition legality is defined by an explicit adjacency table; every mutation validates against it (illegal transitions rejected). This table is the single source used by both the worker and tests.

### Mirror / callback contract

- **SoT:** ReplayForge SQLite `Job_Store`. `Job_Mirror` is a read model, never authoritative.
- **Status_Callback:** on every state change ReplayForge POSTs `{job_id, state, seq, updated_at}` to Streamclone with an `Auth_Token`.
- **Idempotent:** callbacks carry a monotonically increasing `seq`. A callback whose state is already applied (or `seq` ≤ last applied) returns `200` without mutating the mirror.
- **Authenticated:** missing/invalid `Auth_Token` → `401`, no mutation. Symmetric on ReplayForge for job mutations.
- **Bounded retry + backoff:** delivery failures retry up to `CALLBACK_MAX_ATTEMPTS` with exponential backoff capped at `CALLBACK_BACKOFF_MAX`.
- **Reconciliation:** periodic/opportunistic; Streamclone pulls authoritative state and sets `Job_Mirror := Job_Store` on disagreement.
- **Duplicate suppression:** create for a source with an existing active job returns the existing job id.

```python
# ReplayForge — callback emit with bounded retry/backoff
def emit_callback(job, seq):
    payload = {"job_id": job.id, "state": job.state, "seq": seq, "updated_at": now()}
    for attempt in range(CALLBACK_MAX_ATTEMPTS):
        if post_signed(STREAMCLONE_CALLBACK_URL, payload, auth=AUTH_TOKEN).ok:
            return
        sleep(min(BASE * 2**attempt, CALLBACK_BACKOFF_MAX))
    enqueue_reconcile(job.id)   # give up → rely on reconciliation
```

```go
// Streamclone — idempotent authed callback handler
func HandleCallback(r *Request) Response {
    if !ValidAuth(r) { return Unauthorized() }          // 2.5
    cb := parse(r)
    if !InStateSet(cb.State) { return BadRequest() }     // 2.2
    cur := mirror.Get(cb.JobID)
    if cb.Seq <= cur.Seq || cb.State == cur.State {      // 2.4 idempotent
        return OK()
    }
    mirror.Apply(cb)
    return OK()
}
```

### State-ownership option comparison (RECOMMENDATION required)

| Option | Description | Pros | Cons | Beta fit |
|---|---|---|---|---|
| **A. ReplayForge-only + link-out / poll** | Streamclone stores only job id; UI links to Clip Studio; optional lightweight poll for a badge | Smallest surface; no callback auth infra; no mirror drift class | Streamclone UI can't show live state without polling; weaker "watch-desk" tracking | Simple |
| **B. Minimal mirror (RECOMMENDED)** | Streamclone keeps a small read-model of `Job_State` updated by authed idempotent `Status_Callback`, with reconciliation fallback | Live state in watch desk; idempotent + bounded retry keeps it honest; reconciliation heals drift; auth is a single shared token | One authed endpoint + reconcile job to build | **Best** — smallest safe option that meets Requirement 6.4 |
| **C. Full callback/event sync** | Rich event stream (per-field deltas, history, webhooks) | Full audit/history; real-time granular UI | Over-built for single-worker private beta; more attack surface, more drift modes | Overkill |

**Recommendation:** **Option B — minimal mirror.** It is the smallest option that still lets Streamclone show honest live `Job_State` inside the watch desk. Callbacks are authenticated and idempotent; a reconciliation pass makes ReplayForge SoT the tie-breaker, so mirror drift is self-healing without building a full event system. Option A is a valid fallback if callback auth cannot ship in time (degrade to poll). Option C is deferred.

---

## Phase 2 (RF-P2) — VOD-Backed Source Acquisition

**Goal:** Clips from broadcaster-owned VODs using token-scoped credentials.

Flow (drives 3.1–3.9):
1. Create → `validating_source`.
2. Confirm VOD ownership by requesting broadcaster.
   - not owned → `source_unavailable`
   - creds absent/expired → `auth_required`
   - VOD deleted/unavailable → `vod_unavailable`
3. Success → `downloading_source` (Streamlink/FFmpeg **argv arrays**, token-scoped `clips:edit` + VOD read).
4. Download complete → `transcribing`.

```python
# ReplayForge — argv arrays only (no shell string interpolation) — 3.9
streamlink_cmd = ["streamlink", vod_url, quality, "-o", segment_path]  # list, never joined
ffmpeg_cmd     = ["ffmpeg", "-ss", str(start), "-to", str(end), "-i", segment_path,
                  "-c", "copy", out_path]
run(streamlink_cmd)   # subprocess with list argv; user-controlled values stay single argv elements
```

Credentials: token-scoped to `clips:edit` + VOD read; never logged, never placed in filenames/URLs (redaction layer). Ownership check uses broadcaster identity, not client-supplied claims.

---

## Phase 3 (RF-P3) — Durable Artifact Storage on Cloudflare R2

**Goal:** Rendered clips survive worker/disk loss and are shareable via time-limited signed URLs.

### Artifact manifest schema

Per `Clip_Job`, stored in `Job_Store` and mirrored into an R2 `manifest.json`:

```jsonc
{
  "job_id": "job_01H...",              // opaque, token-free
  "broadcaster_key": "b_9f3c...",      // salted hash, NOT login/token
  "created_at": "2026-07-01T12:00:00Z",
  "retention_expires_at": "2026-07-31T12:00:00Z",
  "artifacts": {
    "raw_source_segment": { "key": "clips/<job>/source.mp4",     "bytes": 0, "sha256": "" },
    "transcript":         { "key": "clips/<job>/transcript.json","lang": "en" },
    "edit_recipe":        { "key": "clips/<job>/recipe.json" },   // project/edit recipe
    "rendered_mp4":       { "key": "clips/<job>/render.mp4",      "duration_s": 0 },
    "thumbnail":          { "key": "clips/<job>/poster.jpg" }
  }
}
```

Object-key rules (4.10): keys are `clips/{job_id}/{artifact}` — **no** broadcaster login, token, or PII. `job_id` and `broadcaster_key` are opaque/salted.

### State transitions (4.1–4.4)

```
rendering → rendered → uploading_artifact → complete   (upload ok)
uploading_artifact → retryable_failed                   (upload fail)
complete → expired                                      (retention elapsed)
```

### Signed URLs (4.5–4.8)

```python
# ReplayForge — presigned playback/download URL, token-free, always expiring
def signed_url(job_id, artifact, kind):           # kind: playback|download
    key = manifest(job_id).artifacts[artifact].key
    ttl = PLAYBACK_TTL if kind == "playback" else DOWNLOAD_TTL   # bounded, > 0
    url = r2.generate_presigned_url(key, expires_in=ttl,
                                    disposition="inline" if kind=="playback" else "attachment")
    assert "expires" in url or "X-Amz-Expires" in url            # 4.7 always expires
    return url   # contains no broadcaster token / Auth_Token — 4.8
```

- Every signed URL has an expiration ≤ configured max TTL.
- Signed URLs and keys exclude broadcaster tokens and `Auth_Token`s.

### Worker-disk-loss recovery

- `Job_Store` (SoT) + R2 are durable; local worker disk is a **cache**.
- On worker restart with lost local files: for `complete` jobs, playback/download re-derive signed URLs from R2 (no local file needed). For jobs mid-pipeline (`downloading_source`…`uploading_artifact`) that lost their working files, mark `retryable_failed` so a retry re-enqueues to `queued`.

### Retention / expiration (4.9)

- Each artifact carries `retention_expires_at`. A retention sweep sets `Job_State := expired` when `now > retention_expires_at`; expired artifacts are eligible for R2 deletion.

---

## Phase 4 (RF-P4) — Clip Studio UX Architecture

**Goal:** A dense, professional, editor-first media tool — usable on first screen, not a marketing page.

### Framework decision (task/decision, biased to NOT migrate)

**Decision: keep Vite + React + TypeScript + shadcn/Radix-style architecture. Do NOT migrate to Next.js for the private beta.**

| Factor | Vite React SPA (current, KEEP) | Next.js migration |
|---|---|---|
| App shape | Client-side editor SPA behind `:8096`, talks to `:8095` API | SSR/RSC — little benefit for an authed single-page editor |
| Ops cost | Static `dist/` behind existing proxy; simple image | Node server runtime, new deploy surface in private ops |
| SEO/SSR | Not needed (private, authed editor) | Main Next.js upside — irrelevant here |
| Migration risk | None | Rewrites routing/data-loading mid-productization |

Reconsider Next.js only with a **concrete product/ops reason** (public shareable render pages needing SSR/OG, or an ops mandate to consolidate on a Node/edge runtime). Record such a trigger in the risk register; otherwise keep Vite.

### Editor layout & UX surfaces (5.1–5.9)

- **Editor-first:** on load, the editor surface is the first usable screen (no hero/landing).
- **Component set:** shadcn/Radix-quality icon controls, tabs, sheets, dropdowns, sliders, toggles.
- **Anti-patterns excluded:** no gradient fills, no purple/blue AI-SaaS palette, no decorative blobs/orbs, no marketing hero, **no nested card-inside-card**.
- **Layout regions:**
  - Source/preview stage (source media preview before edit).
  - Timeline **trim** with in/out handles.
  - **Caption editor** (per-line text/timing; hidden burn-in when transcript empty).
  - **Template / audio / export** controls (tabs or a right sheet).
  - **Render progress** panel naming the current `Job_State` (`downloading_source`, `transcribing`, `rendering`, `uploading_artifact`).
  - **Job archive / queue** and **artifact library** (previews, playback via signed URL, download).
  - **Error / retry states** for `retryable_failed`, and explanatory states for `auth_required`, `source_unavailable`, `vod_unavailable`.
  - **Operator diagnostics** drawer (job id, state history, last callback, retry count) — non-marketing, dense.
- **Mobile:** narrow viewports use **panes** instead of fixed desktop rails.
- **A11y:** keyboard navigation and visible focus indicators for interactive controls (full WCAG validation requires manual + axe testing).
- **Empty transcript:** render omits captions (5.10) — enforced in the render plan (Phase 3 worker), surfaced in UX.

---

## Phase 5 (RF-P5) — Streamclone Integration

**Goal:** Launch from an Analytics moment; track progress in the watch desk.

### `moment_context` contract

Streamclone → ReplayForge create payload:

```jsonc
{
  "moment": {
    "channel_login": "example",
    "vod_id": "v123456789",
    "start_s": 3600, "end_s": 3690,
    "reason": "chat_spike|viewer_peak|manual",
    "candidate_score": 0.82           // computed SERVER-SIDE (6.8), not in client
  },
  "requested_by": "b_9f3c...",        // broadcaster identity for ownership + invite gate
  "idempotency_key": "chan:vod:start:end"   // enables duplicate suppression
}
```

- **Trigger auth:** create request carries the `Auth_Token`; unauthenticated create is rejected on ReplayForge.
- **Job id handling:** on accept, Streamclone records the returned `Clip_Job` id in `Job_Mirror` (6.2).
- **Mirrored status:** UI shows `Job_State` from the mirror using only `Job_State_Set` values (6.4).
- **Recent Clips:** Streamclone lists recent jobs from the mirror with links to `/studio`.
- **`/studio` redirect:** resolves job id → ReplayForge Clip Studio URL (6.3).
- **Proxy:** clipper calls route same-origin `/v1/clipper/*` → host ReplayForge (6.6).
- **Raw chat exclusion:** public client responses exclude raw chat content (6.7); scoring is server-side (6.8).

### Offline / degraded UX (ReplayForge down)

- Streamclone core watch, Analytics, and mirror **remain usable**; Export Moment surfaces an honest "Clip Studio offline" state.
- `/studio` and `/v1/clipper/*` return a clear degraded response (not a stack trace); mirror shows last known state with a staleness note. This satisfies the streamclone-pulse independence requirement (1.5) transitively — no optional surface hard-depends on ReplayForge.

---

## Phase 6 (RF-P6) — Packaging and Private Ops Contract

**Goal:** Packaged deploy artifacts + documented image-promotion contract; public repos document contract only.

Design:
- **ReplayForge deploy artifacts:** API image + Clip Studio web image (or static `dist/` artifact) with **health probes** (`/healthz`), documented **deploy env contract**, resource limits (CPU/mem caps), disk quotas, **single-worker concurrency default**, log redaction on, and **smoke scripts**.
- **Image lineage (7.4–7.6):** Streamclone CI publishes `Source_Image` → `ghcr.io/aron-chu/streamclone/*`. Ops promotes by **digest** → `ghcr.io/aron-chu/streampulse/*`. Pre-cutover, ops may pin `streamclone/*` images.
- **Private ops manifest fields (documented as contract in public repos):** image ref (by digest), env keys (names only, no values), resource limits, disk quota, worker concurrency=1, callback URL + auth token key name, R2 bucket/endpoint key names, retention TTLs, rollback target digest.
- **Rollback plan:** ops pins previous known-good digest; documented in `streampulse-ops` (public repos link to the contract, not the secrets).
- **Env-driven auth (7.8):** `Auth_Token` configured via environment, never hardcoded.
- **Doc guards (7.7 / 8.7):** public repos must not claim image cutover complete without ops evidence, nor claim Auto Clipper production-ready without R2 + signed-playback evidence. Enforced by a docs-guard check.

Public repos document the **contract only**; secrets live in `streampulse-ops`.

---

## Phase 7 (RF-P7) — Private Beta Validation

**Goal:** Validate the end-to-end journey including failure recovery.

Design:
- **Single serial `Render_Worker`:** at most one render in progress at any time (8.1).
- **Invite gate (8.2):** create allowed iff broadcaster ∈ `Invite_List`; otherwise rejected.
- **Failure classification (8.3–8.4):** recoverable → `retryable_failed`; non-recoverable → `failed`.
- **Retry (8.5):** retry of `retryable_failed` re-enqueues → `queued`.
- **End-to-end validation (8.6):** run moment discovery → trigger → download → transcribe → edit → render → R2 upload → signed playback for an invite account before declaring ready.
- **Production-ready gate (8.7):** absent R2 + signed-playback evidence → no production-ready claim.
- **Explanatory states (8.8):** `auth_required`, `source_unavailable`, `vod_unavailable` render explanatory copy naming the condition.

---

## Phase 8 (RF-P8) — Later and Deferred

Explicitly out of private-beta scope:
- Live Helix clip creation (9.1).
- IRC / chat-spike automatic triggering (9.2).
- Horizontal render scaling / concurrent workers (9.3).
- **Risk register (9.4):** record minimal Terms-of-Service, subscriber-only, and deleted-VOD handling as risk items. (Next.js migration trigger from Phase 4 is also recorded here.)

---

## Error Handling

| Condition | Handling | Resulting `Job_State` |
|---|---|---|
| VOD not owned | reject acquisition | `source_unavailable` |
| Creds absent/expired | prompt re-auth | `auth_required` |
| VOD deleted at source | stop | `vod_unavailable` |
| Recoverable render/upload error | allow retry | `retryable_failed` |
| Non-recoverable error | stop | `failed` |
| Retention elapsed | sweep | `expired` |
| Callback delivery failure | bounded retry + backoff, then reconcile | (unchanged; heals via reconcile) |
| Unauthenticated mutation/callback | reject `401`, no state change | (unchanged) |
| ReplayForge down (Streamclone side) | degraded UX, honest message, stale mirror flagged | (last known) |

All error surfaces route through the redaction layer; no tokens in messages, logs, or URLs.

---

## Testing Strategy

**Dual approach:** property-based tests for universal logic (state machine, redaction, idempotence, signing), plus example/integration tests for boundaries, UX, and ops.

- **Property tests:** ≥ 100 iterations each; tagged `Feature: auto-clipper-replayforge-productization, Property {n}: {text}`.
- **Example tests:** editor-first render, proxy route mapping, moment-context payload shape.
- **Integration/smoke:** boundary guards (no FFmpeg/Whisper in Go), image-name assertions, doc guards, end-to-end invite-account journey (single run), extension independence with ReplayForge down.
- **Not PBT:** ops/packaging, deploy/promotion, R2 service behavior, and UI aesthetics — use smoke/integration/lint/snapshot and (for a11y) axe + manual review.

---

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system — a formal statement about what the system should do. Properties bridge human-readable specifications and machine-verifiable correctness guarantees.*

### Property 1: Secret/token redaction is universal

*For any* token or secret value (clip, access, refresh, or auth token) embedded in any emitted string — log line, URL, filename, R2 object key, or display string — the emitted output SHALL NOT contain that token value.

**Validates: Requirements 1.7, 1.8, 4.8, 4.10**

### Property 2: Mirrored/displayed state is always in the Job_State_Set

*For any* `Status_Callback` applied to the `Job_Mirror` and any state rendered by Streamclone, the value SHALL be a member of the defined `Job_State_Set`; values outside the set are rejected and never applied or displayed.

**Validates: Requirements 2.2, 6.4**

### Property 3: Job state transitions are legal and outcome-driven

*For any* `Clip_Job` and any applied transition, the transition SHALL be permitted by the state-machine adjacency table, and the resulting `Job_State` SHALL equal the state mandated by the driving outcome (validation result, download/render/upload success or failure, retention, or failure classification).

**Validates: Requirements 3.1, 3.2, 3.3, 3.5, 3.6, 3.7, 3.8, 4.1, 4.2, 4.3, 4.4, 4.9, 8.3, 8.4**

### Property 4: Unauthenticated job mutation or callback is rejected without side effects

*For any* `Clip_Job` mutation request (ReplayForge) or `Status_Callback` (Streamclone) that lacks a valid `Auth_Token`, the system SHALL return an unauthorized response and SHALL NOT change the `Job_Store` or `Job_Mirror`.

**Validates: Requirements 2.5, 2.6, 2.7**

### Property 5: Status_Callback application is idempotent

*For any* `Status_Callback` whose state has already been applied (or whose sequence is not newer), applying it any number of additional times SHALL return success and leave the `Job_Mirror` unchanged.

**Validates: Requirements 2.4**

### Property 6: Reconciliation converges the mirror to the store

*For any* pair of `Job_Mirror` and `Job_Store` states that disagree, after reconciliation the `Job_Mirror` value SHALL equal the `Job_Store` value.

**Validates: Requirements 2.1, 2.8**

### Property 7: Callback delivery retries are bounded with backoff

*For any* sequence of transient callback-delivery failures, the number of delivery attempts SHALL NOT exceed the configured maximum, and successive backoff intervals SHALL be non-decreasing and bounded by the configured cap.

**Validates: Requirements 2.9**

### Property 8: Duplicate create returns the existing active job

*For any* source that already has an active `Clip_Job`, a subsequent create request for that source SHALL return the existing `Clip_Job` identifier rather than creating a second active job.

**Validates: Requirements 2.10**

### Property 9: External process commands preserve arguments as argv elements

*For any* argument value (including values containing spaces or shell metacharacters), the constructed Streamlink or FFmpeg invocation SHALL place that value as a single element of an argument array and SHALL NOT interpolate it into a shell string.

**Validates: Requirements 3.9**

### Property 10: Every signed URL is time-limited and key-correct

*For any* playback or download request for a completed `Clip_Job`, the returned `Signed_URL` SHALL reference the correct artifact object key and SHALL carry an expiration greater than the current time and no greater than the configured maximum TTL.

**Validates: Requirements 4.5, 4.6, 4.7**

### Property 11: Retry of a retryable_failed job re-enqueues to queued

*For any* `Clip_Job` in `retryable_failed`, a retry SHALL set the `Job_State` to `queued` and re-enqueue the job into the render pipeline.

**Validates: Requirements 8.5**

### Property 12: Rendering concurrency never exceeds one

*For any* set of concurrently enqueued `Clip_Jobs`, the number of jobs in the `rendering` state at any instant SHALL be at most one.

**Validates: Requirements 8.1**

### Property 13: Invite-list gating

*For any* broadcaster account, `Clip_Job` creation SHALL be accepted if and only if the account is a member of the `Invite_List`.

**Validates: Requirements 8.2**

### Property 14: Moment trigger creates a job carrying its context and records the id

*For any* Analytics moment, triggering Export Moment SHALL send a create request whose payload carries that moment's context, and upon acceptance SHALL record the returned `Clip_Job` identifier in the `Job_Mirror`.

**Validates: Requirements 6.1, 6.2**

### Property 15: `/studio` redirect resolves to the job's Clip Studio URL

*For any* `Clip_Job` identifier, opening `/studio` in Streamclone SHALL resolve to the ReplayForge Clip Studio URL associated with that identifier.

**Validates: Requirements 6.3**

### Property 16: In-progress and blocked states are named in the UI

*For any* `Clip_Job` in an in-progress state (`downloading_source`, `transcribing`, `rendering`, `uploading_artifact`) or a blocked state (`auth_required`, `source_unavailable`, `vod_unavailable`), the Clip Studio SHALL display a state message that names the current condition.

**Validates: Requirements 5.5, 8.8**

### Property 17: Empty transcript renders without captions

*For any* `Clip_Job` whose transcript is empty or whitespace-only, the render plan SHALL omit the caption/subtitle step and produce a clip without captions.

**Validates: Requirements 5.10**

### Property 18: Public client responses exclude raw chat content

*For any* Streamclone public client response derived from moment or clip data, the serialized payload SHALL exclude raw chat message content.

**Validates: Requirements 6.7**
