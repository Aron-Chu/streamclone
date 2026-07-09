# Implementation Plan: Auto Clipper + ReplayForge Productization

## Overview

Phased implementation backlog derived from `requirements.md` and `design.md`. Task IDs follow `RF-P{phase}-{seq}`. Owner Repo ∈ {`replayforge`, `streamclone`, `streamclone-pulse`, `streampulse-ops`}. Each task carries its owner repo, acceptance check, verification method, and a private-beta **Blocking?** flag as sub-bullets.

The hard private-beta gate is **Phase 3 durable R2 artifact storage + signed-playback evidence** plus the **Phase 1 idempotent/authenticated mirror-callback contract**. Nothing may be called "production-ready" until those are proven.

Tasks are grouped by phase (Phase 0 → Phase 7). Phase order followed by sequence order is a valid topological sort of the dependency graph (see the Task Dependency Graph at the end), so executing top-to-bottom respects every dependency. Phase 8 (explicitly deferred, out of beta scope) and the external Operator Evidence Gate (`EOG-*`, owned by private `streampulse-ops`) are kept as reference sections below and are **not** executable tasks in this workspace.

## Assumptions

- ReplayForge is the primary owner (jobs, SQLite `Job_Store` = source of truth, FFmpeg, Whisper, editor, export, R2 artifacts, signed playback). Streamclone integrates over HTTP only (Analytics moments, `moment_context`, Export Moment trigger, `/studio` redirect, `Job_Mirror`, callback auth).
- `streampulse-ops` (private) owns production environment, secrets, deploy manifests, and image promotion by digest. Public repos document contracts only — never secrets.
- Languages are fixed by the design: ReplayForge backend/worker = Python; Streamclone services = Go; Clip Studio + Streamclone frontends = Vite + React + TypeScript with a shadcn/Radix-style component set.
- Private beta = single serial `Render_Worker`, serialized renders, small `Invite_List`, broadcaster-owned VODs only, token-scoped credentials.
- Cloudflare R2 is the durable `Artifact_Store`; local worker disk is a cache.
- State-ownership uses **Option B — minimal mirror**: authed idempotent `Status_Callback` + reconciliation fallback. Option A (link-out/poll) is the documented degrade path.
- No shared database between repos — HTTP contracts only.

## Non-Goals

- No FFmpeg, render, Whisper, clip editor, or export code added to Streamclone Go services (Requirement 1.1–1.3, 6.5).
- No tokens (clip, access, refresh, auth) in bundles, URLs, filenames, R2 object keys, logs, or display strings (Requirement 1.7, 1.8, 4.8, 4.10).
- No unauthenticated endpoint that mutates `Clip_Job` state on either side (Requirement 2.5–2.7).
- No client-side clip-candidate (Pulse) scoring — scoring is server-side (Requirement 6.8).
- No "image cutover complete" claim without private-ops evidence (Requirement 7.7).
- No "Auto Clipper production-ready" claim without R2 durable storage plus signed-playback evidence (Requirement 8.7).
- No moving ReplayForge ownership into Streamclone, and no moving Clip Studio into the StreamPulse portal/extension (Requirement 1.4, 1.5).
- Live Helix clip creation, IRC/chat-spike auto-triggering, and horizontal render scaling are deferred to Phase 8 (Requirement 9.1–9.3).

## Tasks

- [x] 1. Phase 0 — Audit and Boundary Cleanup
- [x] 1.1 RF-P0-001 (streamclone) — Go boundary-guard test (static scan)
  - Add a Go boundary-guard test asserting `cmd/*` and `internal/*` contain no FFmpeg exec, no Whisper, and no editor/export render code; fail CI if reintroduced.
  - Acceptance: test fails when a FFmpeg/Whisper/editor/export symbol is added under `cmd/`/`internal/`; passes on a clean tree.
  - Verification: `make test` (boundary guard package) + `make vet`. Blocking: Yes.
  - _Requirements: 1.1, 1.2, 1.3_
- [x] 1.2 RF-P0-002 (streamclone) — Document clipper responsibility allow-list and wire the guard
  - Document the Streamclone clipper responsibility allow-list (`moment_context`, Export Moment, `/studio`, `Job_Mirror`, callback auth) and wire the guard test to flag anything outside it touching clip render.
  - Depends on RF-P0-001. Acceptance: allow-list file exists; guard references it; out-of-list render-touching route trips review.
  - Verification: `make test` + doc lint. Blocking: Yes.
  - _Requirements: 1.6_
- [x] 1.3 RF-P0-003 (replayforge) — Shared Python token-redaction chokepoint
  - Implement the shared `redact()` chokepoint used by logger, URL builder, key builder, and display formatter over `SECRET_PATTERNS` (bearer/access/refresh/auth/clip token shapes).
  - Acceptance: every emitted string passes through `redact()`; known token shapes replaced before emit.
  - Verification: `cd ../replayforge && make test` (redaction unit + property P1). Blocking: Yes.
  - _Requirements: 1.8_
- [x] 1.4 RF-P0-004 (streamclone) — Matching Go `Redact()` chokepoint
  - Implement the Go `Redact()` chokepoint on the mirror/log/display path stripping the same token shapes.
  - Depends on RF-P0-001. Acceptance: mirror/log/display strings pass through `Redact()`; token shapes never emitted.
  - Verification: `make test` (redaction package). Blocking: Yes.
  - _Requirements: 1.7_
- [x] 1.5 RF-P0-005 (streamclone) — Correct boundary-affecting workspace docs
  - Confirm workspace docs and focused-workspace layout describe the two-repo split correctly; correct only boundary-affecting drift.
  - Depends on RF-P0-002. Acceptance: `docs/workspace.md` reflects ReplayForge sibling ownership; no render code claimed in Streamclone.
  - Verification: doc link/whitespace check. Blocking: No.
  - _Requirements: 1.4_
- [x] 1.6 RF-P0-006 (streamclone-pulse) — Record extension/portal independence from ReplayForge
  - Confirm ReplayForge is NOT a default StreamPulse portal/extension scope item; record independence via doc + `/v1/extension/health` check only (no code in the focused workspace).
  - Acceptance: portal/extension docs assert no ReplayForge dependency; independence noted.
  - Verification: doc check + `curl :8090/v1/extension/health`. Blocking: No.
  - _Requirements: 1.5_
- [x] 1.7 RF-P0-007 (streamclone) — Update boundary-relevant stale docs only
  - Update stale docs only where they affect ownership boundaries (clipper steering, agents doc); leave unrelated docs untouched.
  - Depends on RF-P0-002. Acceptance: boundary-relevant docs current; diff is narrow, no drive-by edits.
  - Verification: doc link/whitespace check. Blocking: No.
  - _Requirements: 1.4, 1.6_
- [x] 1.8 RF-P0-008 (streampulse-ops) — Document source-build vs digest-promotion split (contract-only)
  - Confirm production image docs describe the source-build vs digest-promotion split (contract-only in public repos).
  - Acceptance: contract doc states `streamclone/*` source build → `streampulse/*` promotion by digest; no secrets in public repos.
  - Verification: doc review against promotion contract. Blocking: No.
  - _Requirements: 7.4, 7.5_
- [x] 1.9 RF-P0-009 (streamclone) — Doc hygiene: fix broken links and image refs
  - Fix the two broken StreamPulse relative doc links and replace shorthand image refs in the rc18 promotion example with fully-qualified names (e.g. `ghcr.io/aron-chu/streamclone/...:v0.3.0-rc18`).
  - Depends on RF-P0-007. Acceptance: broken StreamPulse links resolve; rc18 example uses fully-qualified image names.
  - Verification: doc link/whitespace check. Blocking: No.
  - _Requirements: 7.4_

- [x] 2. Phase 1 — ReplayForge Job Model Hardening + Mirror/Callback Contract
- [x] 2.1 RF-P1-001 (replayforge) — Canonical `Job_State` machine with adjacency table
  - Define the canonical state machine over the full `Job_State_Set` with an explicit adjacency table used by both worker and tests.
  - Depends on RF-P0-003. Acceptance: adjacency table encodes all legal transitions; illegal transitions rejected.
  - Verification: `cd ../replayforge && make test` (state-machine + property P3). Blocking: Yes.
  - _Requirements: 2.1_
- [x] 2.2 RF-P1-002 (replayforge) — Classify states active / terminal / retryable
  - Classify each `Job_State_Set` member as active / terminal / retryable with the retryable-vs-terminal outcome mapping.
  - Depends on RF-P1-001. Acceptance: each member has a documented kind; classification asserted in tests.
  - Verification: `cd ../replayforge && make test`. Blocking: Yes.
  - _Requirements: 2.1, 8.3, 8.4_
- [x] 2.3 RF-P1-003 (replayforge) — Duplicate suppression via `idempotency_key`
  - Implement duplicate suppression via `idempotency_key` (`chan:vod:start:end`) — create for a source with an active job returns the existing job id.
  - Depends on RF-P1-001. Acceptance: second create for same source returns existing `Clip_Job` id, no second active job.
  - Verification: `cd ../replayforge && make test` (property P8). Blocking: Yes.
  - _Requirements: 2.10_
- [x] 2.4 RF-P1-004 (replayforge) — Job status API for reconciliation pulls
  - Implement `GET /v1/jobs/{id}` returning SoT state for reconciliation pulls (authed; unauth rejected).
  - Depends on RF-P1-001. Acceptance: authed GET returns current `Job_Store` state; unauth rejected.
  - Verification: `cd ../replayforge && make test` + `curl :8095/v1/jobs/{id}`. Blocking: Yes.
  - _Requirements: 2.1, 2.8_
- [x] 2.5 RF-P1-005 (replayforge) — Queue depth / worker status endpoint
  - Expose queue depth / worker status for operator visibility.
  - Depends on RF-P1-002. Acceptance: endpoint reports queued count and worker busy/idle.
  - Verification: `cd ../replayforge && make test`. Blocking: No.
  - _Requirements: 8.1_
- [x] 2.6 RF-P1-006 (replayforge) — Idempotent authenticated `Status_Callback` emit
  - Emit `{job_id, state, seq, updated_at}` with monotonic `seq` and `Auth_Token` on every state change.
  - Depends on RF-P1-001, RF-P0-003. Acceptance: callback carries `Auth_Token` + increasing `seq`; emitted on each transition.
  - Verification: `cd ../replayforge && make test`. Blocking: Yes.
  - _Requirements: 2.3_
- [x] 2.7 RF-P1-007 (replayforge) — Bounded callback retry + backoff, then reconcile
  - Add bounded callback retry + exponential backoff (`CALLBACK_MAX_ATTEMPTS`, `CALLBACK_BACKOFF_MAX`), then fall back to reconcile enqueue.
  - Depends on RF-P1-006. Acceptance: attempts capped; backoff non-decreasing and ≤ cap; gives up to reconcile.
  - Verification: `cd ../replayforge && make test` (property P7). Blocking: Yes.
  - _Requirements: 2.9_
- [x] 2.8 RF-P1-008 (streamclone) — `Job_Mirror` read model via authed idempotent callback
  - Update `Job_Mirror` only via the authed idempotent `Status_Callback` handler; reject states outside `Job_State_Set` and stale/≤ `seq` callbacks with 200 no-op.
  - Depends on RF-P0-004. Acceptance: handler rejects non-set states and stale callbacks; auth required.
  - Verification: `make test` (mirror handler + properties P2, P5). Blocking: Yes.
  - _Requirements: 2.2, 2.4_
- [x] 2.9 RF-P1-009 (streamclone) — Reject unauthenticated mutation/callback with 401
  - Reject all unauthenticated `Clip_Job` mutation / callback requests with `401` and no state change.
  - Depends on RF-P1-008. Acceptance: missing/invalid `Auth_Token` → 401, `Job_Mirror` unchanged.
  - Verification: `make test` (property P4) + `make security-scan`. Blocking: Yes.
  - _Requirements: 2.5, 2.7_
- [x] 2.10 RF-P1-010 (streamclone) — Reconciliation pull sets mirror to store (SoT tie-break)
  - Implement reconciliation that sets `Job_Mirror := Job_Store` on disagreement.
  - Depends on RF-P1-004, RF-P1-008. Acceptance: after reconcile, mirror equals store for divergent jobs.
  - Verification: `make test` (property P6). Blocking: Yes.
  - _Requirements: 2.8_
- [x] 2.11 RF-P1-011 (streamclone) — Adopt Option B (minimal mirror); document Option A fallback
  - Record Option B (minimal mirror) as the state-ownership decision; document Option A poll/link-out as the degrade fallback.
  - Depends on RF-P1-008. Acceptance: decision recorded in integration doc; fallback path described.
  - Verification: doc review + `make test`. Blocking: Yes.
  - _Requirements: 2.2, 6.4_
- [x] 2.12 RF-P1-012 (replayforge) — Enforce authenticated `Clip_Job` mutation (symmetric 401)
  - Enforce authenticated mutation on the ReplayForge side (symmetric 401 on missing token).
  - Depends on RF-P1-006. Acceptance: unauthed mutation → 401, `Job_Store` unchanged.
  - Verification: `cd ../replayforge && make test` (property P4). Blocking: Yes.
  - _Requirements: 2.6_
- [x] 2.13 RF-P1-013 (replayforge) — State-transition legality tests over adjacency table
  - Write legality tests over the adjacency table (property + example edge cases).
  - Depends on RF-P1-001, RF-P1-002. Acceptance: legal transitions accepted, illegal rejected; ≥100 iterations for property.
  - Verification: `cd ../replayforge && make test` (property P3). Blocking: Yes.
  - _Requirements: 2.1_

- [x] 3. Phase 2 — VOD-Backed Source Acquisition
- [x] 3.1 RF-P2-001 (replayforge) — Set `validating_source` and validate `moment_context`
  - On create, set `validating_source` and validate the `moment_context` payload shape (channel, vod_id, start/end, reason).
  - Depends on RF-P1-001. Acceptance: create transitions to `validating_source`; malformed context rejected.
  - Verification: `cd ../replayforge && make test` (property P3). Blocking: Yes.
  - _Requirements: 3.1_
- [x] 3.2 RF-P2-002 (replayforge) — VOD ownership check against broadcaster identity
  - Validate VOD ownership against requesting broadcaster identity (not client-supplied claims).
  - Depends on RF-P2-001. Acceptance: not-owned VOD → `source_unavailable`; owned VOD proceeds.
  - Verification: `cd ../replayforge && make test`. Blocking: Yes.
  - _Requirements: 3.2, 3.3_
- [x] 3.3 RF-P2-003 (replayforge) — Token-scoped `clips:edit` + VOD-read credentials
  - Use token-scoped credentials; never log or place in filenames/URLs (via redaction chokepoint).
  - Depends on RF-P2-002, RF-P0-003. Acceptance: credentials scoped to `clips:edit`+VOD read; no token leaks.
  - Verification: `cd ../replayforge && make test` + `make security-scan` (both repos). Blocking: Yes.
  - _Requirements: 3.4, 1.8_
- [x] 3.4 RF-P2-004 (replayforge) — Map validation outcomes to blocked states
  - Map validation outcomes to `source_unavailable` / `auth_required` / `vod_unavailable`.
  - Depends on RF-P2-002. Acceptance: each outcome yields the mandated `Job_State`.
  - Verification: `cd ../replayforge && make test` (property P3). Blocking: Yes.
  - _Requirements: 3.3, 3.5, 3.6_
- [x] 3.5 RF-P2-005 (replayforge) — Source segment download via argv arrays
  - Download via Streamlink/FFmpeg **argv arrays** (no shell string interpolation); success → `downloading_source` then `transcribing`.
  - Depends on RF-P2-003. Acceptance: args with spaces/metacharacters stay single argv elements; states advance on success.
  - Verification: `cd ../replayforge && make test` (property P9). Blocking: Yes.
  - _Requirements: 3.7, 3.8, 3.9_
- [x] 3.6 RF-P2-006 (replayforge) — Source media preview surface data
  - Provide source media preview (pre-edit) surface data for Clip Studio.
  - Depends on RF-P2-005. Acceptance: downloaded source segment previewable before edit.
  - Verification: `cd ../replayforge && make test`. Blocking: No.
  - _Requirements: 5.6_
- [x] 3.7 RF-P2-007 (replayforge) — Rate limits on source acquisition
  - Add rate limits to bound upstream calls.
  - Depends on RF-P2-005. Acceptance: acquisition requests bounded; excess throttled.
  - Verification: `cd ../replayforge && make test`. Blocking: No.
  - _Requirements: 3.7_
- [x] 3.8 RF-P2-008 (streamclone) — Local smoke: Analytics moment → ReplayForge create
  - Local smoke of the Analytics moment → ReplayForge create-job trigger path (VOD-backed candidate).
  - Depends on RF-P2-001, RF-P1-006. Acceptance: local Analytics→ReplayForge create succeeds through validation stage.
  - Verification: `make up` + manual Analytics→Export Moment smoke. Blocking: No.
  - _Requirements: 6.1_
- [x] 3.9 RF-P2-009 (replayforge) — Defer live Helix acquisition and IRC triggering
  - Document live Helix acquisition and IRC triggering as out-of-scope for Phase 2 (VOD-only path).
  - Depends on RF-P2-005. Acceptance: acquisition path is VOD-only; live/IRC noted deferred.
  - Verification: doc review. Blocking: No.
  - _Requirements: 9.1, 9.2_

- [x] 4. Phase 3 — Durable Artifact Storage on Cloudflare R2 (hard production-ready gate)
- [x] 4.1 RF-P3-001 (replayforge) — R2 artifact `manifest.json` schema
  - Define the manifest schema (raw source segment, transcript/caption JSON, edit recipe, rendered MP4, thumbnail/poster) with `retention_expires_at`.
  - Depends on RF-P1-001. Acceptance: manifest validates all five artifact entries + retention field.
  - Verification: `cd ../replayforge && make test`. Blocking: Yes.
  - _Requirements: 4.2_
- [x] 4.2 RF-P3-002 (replayforge) — Token-free object-key rules
  - Enforce keys `clips/{job_id}/{artifact}` with no login/token/PII; `job_id`/`broadcaster_key` opaque/salted.
  - Depends on RF-P3-001, RF-P0-003. Acceptance: keys contain no broadcaster login/token/PII.
  - Verification: `cd ../replayforge && make test` (property P1) + `make security-scan`. Blocking: Yes.
  - _Requirements: 4.10, 4.8_
- [x] 4.3 RF-P3-003 (replayforge) — `rendered → uploading_artifact → complete` with R2 upload
  - Implement the upload transitions; upload failure → `retryable_failed`.
  - Depends on RF-P3-001, RF-P1-001. Acceptance: success path reaches `complete`; upload failure yields `retryable_failed`.
  - Verification: `cd ../replayforge && make test` (property P3). Blocking: Yes.
  - _Requirements: 4.1, 4.2, 4.3, 4.4_
- [x] 4.4 RF-P3-004 (replayforge) — Presigned playback + download URLs with bounded expiry
  - Implement presigned URLs (`PLAYBACK_TTL`/`DOWNLOAD_TTL`), correct key, no tokens in URL.
  - Depends on RF-P3-002, RF-P3-003. Acceptance: every signed URL references correct key, expires > now and ≤ max TTL, token-free.
  - Verification: `cd ../replayforge && make test` (property P10). Blocking: Yes.
  - _Requirements: 4.5, 4.6, 4.7, 4.8_
- [x] 4.5 RF-P3-005 (replayforge) — Worker-disk-loss recovery
  - `complete` jobs re-derive signed URLs from R2; mid-pipeline jobs with lost files → `retryable_failed`.
  - Depends on RF-P3-003, RF-P3-004. Acceptance: after simulated disk loss, `complete` playback still resolves; mid-pipeline → `retryable_failed`.
  - Verification: `cd ../replayforge && make test` + durable-artifact smoke. Blocking: Yes.
  - _Requirements: 4.5, 4.6, 8.3_
- [x] 4.6 RF-P3-006 (replayforge) — Retention sweep sets `expired`
  - Set `Job_State := expired` when `now > retention_expires_at`; expired artifacts eligible for R2 deletion.
  - Depends on RF-P3-001, RF-P3-003. Acceptance: elapsed retention → `expired`; artifact flagged for cleanup.
  - Verification: `cd ../replayforge && make test` (property P3). Blocking: Yes.
  - _Requirements: 4.9_
- [x] 4.7 RF-P3-007 (replayforge) — Durable-artifact recovery smoke script
  - Trigger VOD job → render MP4 → upload → delete/restart worker output → confirm signed playback/download still works.
  - Depends on RF-P3-005. Acceptance: signed playback/download succeed after worker output wiped/restarted.
  - Verification: `cd ../replayforge && make up` + durable-artifact smoke script. Blocking: Yes.
  - _Requirements: 8.6, 8.7, 4.5_

- [x] 5. Phase 4 — Clip Studio UX / Product Polish
- [x] 5.1 RF-P4-001 (replayforge) — Record framework decision (keep Vite + React + TS)
  - Record KEEP Vite + React + TypeScript + shadcn/Radix; document the Next.js reconsideration trigger (route to Risk Register).
  - Acceptance: decision doc states keep-Vite; Next.js trigger recorded in Risk Register.
  - Verification: doc review. Blocking: No.
  - _Requirements: 5.2_
- [x] 5.2 RF-P4-002 (replayforge) — Editor-first layout
  - The editor surface is the first usable screen on load (no hero/landing).
  - Depends on RF-P4-001. Acceptance: on load, editor is immediately usable; no marketing hero.
  - Verification: `cd ../replayforge && npm run build` + snapshot/UI smoke. Blocking: Yes.
  - _Requirements: 5.1_
- [x] 5.3 RF-P4-003 (replayforge) — Job archive / queue view
  - List jobs with `Job_State`.
  - Depends on RF-P1-005. Acceptance: archive lists jobs and current states.
  - Verification: `cd ../replayforge && npm run build`. Blocking: No.
  - _Requirements: 5.1_
- [x] 5.4 RF-P4-004 (replayforge) — Source preview stage
  - Show source media before edit.
  - Depends on RF-P2-006. Acceptance: source segment previews before edit.
  - Verification: `cd ../replayforge && npm run build`. Blocking: No.
  - _Requirements: 5.6_
- [x] 5.5 RF-P4-005 (replayforge) — Render preview surface
  - Preview the rendered artifact via signed URL.
  - Depends on RF-P3-004. Acceptance: rendered clip previewable via signed URL.
  - Verification: `cd ../replayforge && npm run build`. Blocking: No.
  - _Requirements: 4.5_
- [x] 5.6 RF-P4-006 (replayforge) — Timeline trim with in/out handles
  - Depends on RF-P4-002. Acceptance: trim in/out handles adjust clip bounds.
  - Verification: `cd ../replayforge && npm run build`. Blocking: Yes.
  - _Requirements: 5.7_
- [x] 5.7 RF-P4-007 (replayforge) — Caption editor (per-line; hidden burn-in when empty)
  - Per-line text/timing; omit burn-in when transcript empty.
  - Depends on RF-P4-002. Acceptance: captions editable per line; empty transcript omits burn-in.
  - Verification: `cd ../replayforge && make test` (property P17) + `npm run build`. Blocking: Yes.
  - _Requirements: 5.7, 5.10_
- [x] 5.8 RF-P4-008 (replayforge) — Template controls
  - Depends on RF-P4-002. Acceptance: template selection applies to render params.
  - Verification: `cd ../replayforge && npm run build`. Blocking: No.
  - _Requirements: 5.7_
- [x] 5.9 RF-P4-009 (replayforge) — Audio controls
  - Depends on RF-P4-002. Acceptance: audio adjustments apply to render params.
  - Verification: `cd ../replayforge && npm run build`. Blocking: No.
  - _Requirements: 5.7_
- [x] 5.10 RF-P4-010 (replayforge) — Export profile controls
  - Depends on RF-P4-002. Acceptance: export profile selectable and passed to worker.
  - Verification: `cd ../replayforge && npm run build`. Blocking: No.
  - _Requirements: 5.7_
- [x] 5.11 RF-P4-011 (replayforge) — Render progress panel names current `Job_State`
  - Name the live in-progress state (`downloading_source`, `transcribing`, `rendering`, `uploading_artifact`).
  - Depends on RF-P1-001, RF-P4-002. Acceptance: progress panel names the live in-progress state.
  - Verification: `cd ../replayforge && make test` (property P16) + `npm run build`. Blocking: Yes.
  - _Requirements: 5.5_
- [x] 5.12 RF-P4-012 (replayforge) — Artifact library (previews, signed playback, download)
  - Depends on RF-P3-004. Acceptance: library lists artifacts with working signed playback/download.
  - Verification: `cd ../replayforge && npm run build`. Blocking: Yes.
  - _Requirements: 4.5, 4.6_
- [x] 5.13 RF-P4-013 (replayforge) — Error/retry UX and explanatory blocked states
  - Retry UX for `retryable_failed`; explanatory copy for `auth_required`, `source_unavailable`, `vod_unavailable`.
  - Depends on RF-P1-002, RF-P4-002. Acceptance: retry offered on `retryable_failed`; blocked states show explanatory copy.
  - Verification: `cd ../replayforge && make test` (property P16) + `npm run build`. Blocking: Yes.
  - _Requirements: 5.5, 8.8_
- [x] 5.14 RF-P4-014 (replayforge) — Operator diagnostics drawer
  - Dense, non-marketing drawer (job id, state history, last callback, retry count).
  - Depends on RF-P4-002. Acceptance: drawer shows diagnostics; no marketing chrome.
  - Verification: `cd ../replayforge && npm run build`. Blocking: No.
  - _Requirements: 5.1_
- [x] 5.15 RF-P4-015 (replayforge) — Mobile/narrow-viewport panes
  - Depends on RF-P4-002. Acceptance: narrow viewport uses panes; controls reachable.
  - Verification: `cd ../replayforge && npm run build`. Blocking: No.
  - _Requirements: 5.8_
- [x] 5.16 RF-P4-016 (replayforge) — A11y: keyboard nav + visible focus
  - Depends on RF-P4-002. Acceptance: interactive controls keyboard-reachable with visible focus.
  - Verification: `cd ../replayforge && npm run build` + axe/manual a11y review. Blocking: No.
  - _Requirements: 5.9_
- [x] 5.17 RF-P4-017 (replayforge) — Visual QA checklist (no AI-SaaS anti-patterns)
  - No gradient fills, no purple/blue AI-SaaS palette, no decorative blobs/orbs, no marketing hero, no nested card-inside-card.
  - Depends on RF-P4-002. Acceptance: visual QA checklist passes; anti-patterns absent.
  - Verification: `cd ../replayforge && npm run build` + visual QA checklist review. Blocking: Yes.
  - _Requirements: 5.3, 5.4_

- [x] 6. Phase 5 — Streamclone Integration
- [x] 6.1 RF-P5-001 (streamclone) — Verify the `moment_context` contract
  - Verify channel_login, vod_id, start/end, reason, server-side `candidate_score`, `requested_by`, `idempotency_key`.
  - Depends on RF-P2-001. Acceptance: payload matches contract; `candidate_score` server-computed.
  - Verification: `make test` (payload shape). Blocking: Yes.
  - _Requirements: 6.1, 6.8_
- [x] 6.2 RF-P5-002 (streamclone) — Authenticated Export Moment trigger
  - Streamclone → ReplayForge create with `Auth_Token`.
  - Depends on RF-P5-001, RF-P1-006. Acceptance: authed create accepted; unauth create rejected.
  - Verification: `make test` (property P14). Blocking: Yes.
  - _Requirements: 6.1_
- [x] 6.3 RF-P5-003 (streamclone) — Record returned `Clip_Job` id in `Job_Mirror`
  - Depends on RF-P5-002, RF-P1-008. Acceptance: accepted create stores job id in mirror.
  - Verification: `make test` (property P14). Blocking: Yes.
  - _Requirements: 6.2_
- [x] 6.4 RF-P5-004 (streamclone) — Display minimal mirrored status (Option B)
  - Use only `Job_State_Set` values.
  - Depends on RF-P1-008, RF-P1-011. Acceptance: watch desk shows live `Job_State` from mirror; no out-of-set values.
  - Verification: `make test` (property P2) + `make frontend-build`. Blocking: Yes.
  - _Requirements: 6.4_
- [x] 6.5 RF-P5-005 (streamclone) — Recent Clips listing with `/studio` links
  - Depends on RF-P5-003. Acceptance: recent jobs listed with working `/studio` links.
  - Verification: `make frontend-build`. Blocking: No.
  - _Requirements: 6.3_
- [x] 6.6 RF-P5-006 (streamclone) — Offline/degraded UX when ReplayForge is down
  - Honest "Clip Studio offline"; stale mirror flagged; no stack trace.
  - Depends on RF-P5-004. Acceptance: with ReplayForge down, core watch/analytics/mirror usable; degraded copy shown.
  - Verification: `make test` + `make frontend-build`. Blocking: Yes.
  - _Requirements: 1.5, 6.4_
- [x] 6.7 RF-P5-007 (streamclone) — `/studio` redirect resolves job id → Clip Studio URL
  - Depends on RF-P5-003. Acceptance: opening `/studio?job={id}` redirects to correct Clip Studio URL.
  - Verification: `make test` (property P15). Blocking: Yes.
  - _Requirements: 6.3_
- [x] 6.8 RF-P5-008 (streamclone) — Same-origin `/v1/clipper/*` proxy → host ReplayForge `:8095`
  - Depends on RF-P0-002. Acceptance: clipper calls route same-origin to host ReplayForge.
  - Verification: `make test` + `make compose-config-check`. Blocking: Yes.
  - _Requirements: 6.6_
- [x] 6.9 RF-P5-009 (streamclone) — Re-assert boundary guard after integration wiring
  - Confirm no render/FFmpeg/Whisper logic in Streamclone Go.
  - Depends on RF-P0-001, RF-P5-008. Acceptance: boundary guard still passes after integration wiring.
  - Verification: `make test` (boundary guard). Blocking: Yes.
  - _Requirements: 6.5_
- [x] 6.10 RF-P5-010 (streamclone) — Server-side clip-candidate scoring
  - No client-side Pulse scoring; `candidate_score` produced server-side, absent from client bundle.
  - Depends on RF-P5-001. Acceptance: `candidate_score` produced server-side; absent from client bundle.
  - Verification: `make test` (property P18 adjacent) + `make security-scan`. Blocking: Yes.
  - _Requirements: 6.8_
- [x] 6.11 RF-P5-011 (streamclone) — Exclude raw chat from public client responses
  - Depends on RF-P5-001. Acceptance: serialized public responses contain no raw chat.
  - Verification: `make test` (property P18). Blocking: Yes.
  - _Requirements: 6.7_
- [x] 6.12 RF-P5-012 (streamclone) — Client/API contract tests
  - Cover the create payload, callback idempotence, and `/studio` redirect.
  - Depends on RF-P5-002, RF-P5-007. Acceptance: contract tests cover payload shape, callback idempotence, redirect.
  - Verification: `make test` + `make frontend-test`. Blocking: No.
  - _Requirements: 6.1, 6.3_
- [x] 6.13 RF-P5-013 (streamclone) — Retire browser-visible clipper mutation token
  - Server-side proxy attaches mutation `Auth_Token` from server env; frontend never carries `VITE_CLIPPER_TOKEN` (remove from `frontend/src/config.ts`; update `docs/agents-streamclone-and-replayforge.md`).
  - Depends on RF-P0-004, RF-P5-008. Acceptance: no mutation token in frontend bundle/config; server proxy injects auth from env.
  - Verification: `make security-scan` + `make frontend-build` + grep confirms `VITE_CLIPPER_TOKEN` absent from client bundle. Blocking: Yes.
  - _Requirements: 1.7, 7.8_

- [x] 7. Phase 6 — Packaging and Private Ops Contract
- [x] 7.1 RF-P6-001 (replayforge) — ReplayForge API deploy image with `/healthz`
  - Depends on RF-P1-004. Acceptance: API image builds; `/healthz` returns healthy.
  - Verification: `cd ../replayforge && make build` + `curl :8095/healthz`. Blocking: Yes.
  - _Requirements: 7.1_
- [x] 7.2 RF-P6-002 (replayforge) — Clip Studio web image (or static `dist/`) with `/healthz`
  - Depends on RF-P4-002. Acceptance: web image/static artifact builds and serves; health OK.
  - Verification: `cd ../replayforge && make build`. Blocking: Yes.
  - _Requirements: 7.1_
- [x] 7.3 RF-P6-003 (replayforge) — Document deploy env contract (key names only)
  - For API + worker + web; no secret values.
  - Depends on RF-P6-001. Acceptance: contract lists all env keys by name; no secret values.
  - Verification: doc review. Blocking: Yes.
  - _Requirements: 7.3_
- [x] 7.4 RF-P6-004 (replayforge) — Resource limits (CPU/mem caps)
  - Depends on RF-P6-001. Acceptance: compose/deploy declares CPU + mem caps.
  - Verification: `cd ../replayforge && make compose-config-check` (or equivalent). Blocking: No.
  - _Requirements: 7.1_
- [x] 7.5 RF-P6-005 (replayforge) — Default single-worker render concurrency (=1)
  - Depends on RF-P6-001, RF-P1-001. Acceptance: packaged config sets worker concurrency 1.
  - Verification: `cd ../replayforge && make test` (property P12). Blocking: Yes.
  - _Requirements: 8.1_
- [x] 7.6 RF-P6-006 (replayforge) — Disk quotas for the local worker cache
  - Depends on RF-P6-001. Acceptance: disk quota enforced on worker cache dir.
  - Verification: deploy config review + smoke. Blocking: No.
  - _Requirements: 7.1_
- [x] 7.7 RF-P6-007 (replayforge) — Log redaction enabled in all deploy artifacts
  - Redaction chokepoint active by default.
  - Depends on RF-P0-003, RF-P6-001. Acceptance: deployed logs pass through `redact()`; no token shapes.
  - Verification: `cd ../replayforge && make test` + `make security-scan`. Blocking: Yes.
  - _Requirements: 1.7, 1.8_
- [x] 7.8 RF-P6-008 (replayforge) — Smoke scripts for API/worker/web
  - `/healthz`, minimal job path.
  - Depends on RF-P6-001, RF-P6-002. Acceptance: smoke scripts run green against packaged images.
  - Verification: `cd ../replayforge && make up` + smoke scripts. Blocking: Yes.
  - _Requirements: 7.1_
- [x] 7.9 RF-P6-011 (replayforge) — Env-driven `Auth_Token` (never hardcoded)
  - For job mutation + callback auth.
  - Depends on RF-P1-006, RF-P6-003. Acceptance: `Auth_Token` sourced from env; no hardcoded secret.
  - Verification: `cd ../replayforge && make test` + `make security-scan`. Blocking: Yes.
  - _Requirements: 7.8_
- [x] 7.10 RF-P6-012 (streamclone) — Doc guards for cutover / production-ready claims
  - Block "image cutover complete" without ops evidence and "Auto Clipper production-ready" without R2 + signed-playback evidence.
  - Depends on RF-P0-007. Acceptance: docs-guard check fails on unsupported claims.
  - Verification: `make test` (docs-guard) + doc review. Blocking: Yes.
  - _Requirements: 7.7, 8.7_

- [ ] 8. Phase 7 — Private Beta Validation
- [x] 8.1 RF-P7-001 (replayforge) — ReplayForge `/healthz` smoke against packaged image
  - Depends on RF-P6-001. Acceptance: `/healthz` healthy on packaged deploy.
  - Verification: `cd ../replayforge && make up` + `curl :8095/healthz`. Blocking: Yes.
  - _Requirements: 7.1_
- [x] 8.2 RF-P7-002 (streamclone) — Validate Analytics → Export Moment → job creation e2e
  - For an invite account.
  - Depends on RF-P5-002, RF-P7-001. Acceptance: moment trigger creates job for invite account.
  - Verification: `make up` + manual Analytics→Export Moment smoke. Blocking: Yes.
  - _Requirements: 6.1, 8.6_
- [x] 8.3 RF-P7-003 (replayforge) — Validate source segment preview
  - Depends on RF-P2-006, RF-P7-002. Acceptance: source preview renders for the job.
  - Verification: durable-artifact smoke + manual review. Blocking: No.
  - _Requirements: 5.6_
- [x] 8.4 RF-P7-004 (replayforge) — Validate edit project applies to render params
  - Trim/caption/template/audio.
  - Depends on RF-P4-006, RF-P4-007. Acceptance: edits reflected in render output.
  - Verification: `cd ../replayforge && make test` + manual. Blocking: No.
  - _Requirements: 5.7_
- [x] 8.5 RF-P7-005 (replayforge) — Validate render via single serial worker
  - Depends on RF-P3-003, RF-P6-005. Acceptance: final MP4 rendered; only one job `rendering` at a time.
  - Verification: `cd ../replayforge && make test` (property P12) + smoke. Blocking: Yes.
  - _Requirements: 8.1_
- [x] 8.6 RF-P7-006 (replayforge) — Validate R2 upload and `complete` transition
  - Depends on RF-P3-003, RF-P7-005. Acceptance: artifact in R2; job `complete`.
  - Verification: durable-artifact smoke. Blocking: Yes.
  - _Requirements: 4.3, 8.6_
- [x] 8.7 RF-P7-007 (replayforge) — Validate signed playback/download after worker output wipe
  - Depends on RF-P3-005, RF-P7-006. Acceptance: signed playback/download succeed after worker output wiped.
  - Verification: durable-artifact smoke script. Blocking: Yes.
  - _Requirements: 4.5, 8.6_
- [x] 8.8 RF-P7-008 (replayforge) — Validate retry of `retryable_failed` re-enqueues to `queued`
  - Depends on RF-P3-003, RF-P1-002. Acceptance: retry sets `queued` and re-enters pipeline.
  - Verification: `cd ../replayforge && make test` (property P11). Blocking: Yes.
  - _Requirements: 8.5_
- [x] 8.9 RF-P7-009 (replayforge) — Validate duplicate moment trigger returns existing job
  - Depends on RF-P1-003, RF-P7-002. Acceptance: duplicate trigger returns existing job id.
  - Verification: `cd ../replayforge && make test` (property P8) + smoke. Blocking: Yes.
  - _Requirements: 2.10_
- [x] 8.10 RF-P7-010 (replayforge) — Validate bad Twitch token → `auth_required`
  - Depends on RF-P2-004, RF-P4-013. Acceptance: bad/expired token yields `auth_required` with explanatory copy.
  - Verification: `cd ../replayforge && make test` (property P16) + smoke. Blocking: Yes.
  - _Requirements: 3.5, 8.8_
- [x] 8.11 RF-P7-011 (replayforge) — Validate VOD-unavailable → `vod_unavailable`
  - Depends on RF-P2-004, RF-P4-013. Acceptance: deleted/unavailable VOD yields `vod_unavailable`.
  - Verification: `cd ../replayforge && make test` (property P16) + smoke. Blocking: Yes.
  - _Requirements: 3.6, 8.8_
- [x] 8.12 RF-P7-012 (streamclone) — Validate ReplayForge-offline behavior from Streamclone
  - Degraded UX, stale mirror flagged.
  - Depends on RF-P5-006, RF-P7-001. Acceptance: with ReplayForge down, Streamclone stays usable with honest states.
  - Verification: `make up` + `curl :8090/v1/extension/health` (ReplayForge stopped). Blocking: Yes.
  - _Requirements: 1.5_
- [x] 8.13 RF-P7-013 (replayforge) — Validate single-serial-worker + invite gate together
  - Create accepted iff broadcaster ∈ `Invite_List`.
  - Depends on RF-P6-005, RF-P7-005. Acceptance: non-invite account rejected; concurrency ≤ 1 under load.
  - Verification: `cd ../replayforge && make test` (properties P12, P13). Blocking: Yes.
  - _Requirements: 8.1, 8.2_
- [-] 8.14 RF-P7-014 (replayforge) — Run the full end-to-end validation gate
  - Discovery → trigger → download → transcribe → edit → render → R2 upload → signed playback for an invite account before declaring "ready"; capture evidence.
  - Depends on RF-P7-002, RF-P7-006, RF-P7-007. Acceptance: complete journey passes for one invite account; evidence captured.
  - Verification: durable-artifact smoke + manual end-to-end run. Blocking: Yes.
  - _Requirements: 8.6, 8.7_

## Phase 8 — Later / Explicitly Deferred (reference only, not executable in this workspace)

Explicitly out of private-beta scope; documentation/decision items, not implementation tasks. All depend on RF-P7-014.

| Task ID | Owner Repo | Task | Acceptance | Blocking? |
|---|---|---|---|---|
| RF-P8-001 | replayforge | (Deferred) Live Helix clip creation hardening | Documented as post-beta; not in beta scope | No |
| RF-P8-002 | replayforge | (Deferred) IRC / chat-spike automatic triggering | Documented as post-beta; not in beta scope | No |
| RF-P8-003 | replayforge | (Deferred) Trend/formula engine — cross-reference ReplayForge `docs/tasks.md` (do not merge phase numbering/ownership) | Cross-ref recorded; roadmaps kept separate | No |
| RF-P8-004 | replayforge | (Deferred) Public sharing expansion beyond signed URLs | Documented as post-beta | No |
| RF-P8-006 | streamclone | (Deferred) Full backend source split / horizontal render scaling | Documented as post-beta; single worker remains beta default | No |
| RF-P8-007 | streamclone-pulse | (Deferred) Making ReplayForge a default StreamPulse portal feature | Documented as post-beta; portal stays independent | No |

## Operator Evidence Gate (External Blockers — streampulse-ops)

Owned by the private **streampulse-ops** repo and intentionally **outside** the executable workspace. They are external evidence gates the PUBLIC repos depend on before any readiness/cutover claim; the public plan references them (via `RF-P6-012` doc guards and contract docs) but does not implement them.

| Gate ID | Owner Repo | Evidence Requirement | Satisfied When | Blocking Claim |
|---|---|---|---|---|
| EOG-001 | streampulse-ops (external) | Private ops manifest fields documented contract-only (image ref by digest, env key names, resource limits, disk quota, worker concurrency=1, callback URL + auth token key name, R2 bucket/endpoint key names, retention TTLs, rollback target digest) — no secret values in public repos | Private manifest enumerates all fields; public repos carry contract/key-names only | "image cutover complete" |
| EOG-002 | streampulse-ops (external) | Rollback plan pins a previous known-good **digest**; procedure lives in `streampulse-ops`; public repos link-only | Rollback-by-digest procedure documented privately | "image cutover complete" |
| EOG-003 | streampulse-ops (external) | Ops owns actual deploy/secrets and image **promotion by digest** (source-build `streamclone/*` → promoted `streampulse/*`) | Ops repo holds deploy/secrets + digest promotion; public repos contract-only | "image cutover complete" |
| EOG-004 | streampulse-ops (external) | DMCA / legal / policy review — ToS, subscriber-only, and deleted-VOD handling recorded and reviewed | Legal/policy review recorded and signed off | "Auto Clipper production-ready" |

## Cross-Repo Documentation Updates

| Doc | Owner Repo | Update Needed | When |
|---|---|---|---|
| `replayforge/README.md` | replayforge | Reflect productization scope: primary owner, R2 durable artifacts, private-beta gate, single serial worker | Phase 6 |
| `replayforge/docs/INTEGRATION.md` | replayforge | Finalize API/env contract: `/v1/jobs`, callback, `/healthz`, `Auth_Token` env key, R2 key names, Option B mirror | Phase 1 + Phase 6 |
| `streamclone/docs/agents-streamclone-and-replayforge.md` | streamclone | Update integration ownership map, trigger/mirror/callback contract, proxy, offline degrade; remove `VITE_CLIPPER_TOKEN` guidance and document server-side proxy `Auth_Token` injection (RF-P5-013) | Phase 1 + Phase 5 |
| `streamclone/.kiro/steering/clipper.md` | streamclone | Reflect Option B mirror decision, boundary guard, production-ready gate wording | Phase 0 + Phase 1 |
| `streamclone/docs/workspace.md` | streamclone | Confirm two-repo focused-workspace layout and ownership split | Phase 0 |
| `streamclone/docs/production-promotion-contract.md` | streamclone | Add ReplayForge image lineage (source-build → digest promotion); doc guards | Phase 6 |
| `streamclone-pulse/docs/CONTEXT.md` | streamclone-pulse | Note extension/portal independence from ReplayForge | Phase 0 |
| `streamclone-pulse/docs/pulse-extension/evidence/production-artifact-decision-2026-07.md` | streamclone-pulse | Record durable-artifact (R2) decision + production-ready gate evidence linkage | Phase 3 + Phase 7 |
| `streamclone-pulse/docs/pulse-extension/evidence/streamclone-image-exit-audit-2026-07.md` | streamclone-pulse | Update image cutover status; no "cutover complete" without ops evidence | Phase 6 |

## Verification Matrix

| Repo | Command / Smoke | Required For |
|---|---|---|
| replayforge | `make test` | RF-P0-003, RF-P1-*, RF-P2-*, RF-P3-*, RF-P4-007/011/013, RF-P6-005/007/011, RF-P7-* property/state tests |
| replayforge | `make build` | RF-P6-001, RF-P6-002 packaged images |
| replayforge | `make up` | RF-P3-007, RF-P6-008, RF-P7-001 local packaged smoke |
| replayforge | `curl http://localhost:8095/healthz` | RF-P6-001, RF-P7-001 health probe |
| streamclone | `make up` | RF-P2-008, RF-P7-002, RF-P7-012 local integration smoke |
| streamclone | `curl http://localhost:8090/v1/extension/health` | RF-P0-006, RF-P7-012 extension independence with ReplayForge down |
| streamclone | manual Analytics → Export Moment → ReplayForge Studio | RF-P2-008, RF-P7-002, RF-P7-014 end-to-end journey |
| streamclone | `make test` / `make frontend-build` / `make security-scan` / `make compose-config-check` | RF-P0-001/004, RF-P1-008/009/010, RF-P5-*, RF-P6-012 |
| streamclone-pulse | `npm test` / `npm run typecheck` / `npm run build` (if touched) | RF-P0-006 extension/portal independence |
| durable artifact smoke | trigger VOD-backed job → render final MP4 → upload durable artifact → delete/restart local worker output → confirm signed playback/download still works | RF-P3-005, RF-P3-007, RF-P7-006, RF-P7-007, RF-P7-014 (production-ready gate) |

## Risk Register

| Risk | Severity | Mitigation |
|---|---|---|
| Durable-storage gate not met yet "production-ready" claimed | Critical | Phase 3 is the hard gate; RF-P6-012 docs-guard blocks the claim; RF-P7-014 captures signed-playback evidence before "ready" |
| Broadcaster-token / VOD ToS / subscriber-only / deleted-VOD compliance | High | VOD-owned-only acquisition (RF-P2-002), token-scoped creds (RF-P2-003), explanatory blocked states; ToS/sub-only/deleted-VOD reviewed under EOG-004 |
| Premature "image cutover complete" claim | High | RF-P6-012 docs-guard requires private-ops evidence; source-build vs digest-promotion split documented (RF-P0-008); EOG-001/002/003 |
| Browser-visible clipper mutation token in bundle/display | High | Retire `VITE_CLIPPER_TOKEN` (RF-P5-013); server proxy injects `Auth_Token` from env (RF-P5-008); redaction chokepoints (RF-P0-003/004); `make security-scan` + bundle grep |
| Next.js migration temptation mid-productization | Medium | RF-P4-001 keeps Vite for beta; Next.js only with a concrete trigger recorded in this register |
| Single-worker throughput bottleneck under load | Medium | Worker concurrency=1 default (RF-P6-005, property P12); horizontal scaling deferred (RF-P8-006); queue depth surfaced (RF-P1-005) |
| Callback auth drift / mirror divergence | Medium | Authed idempotent callbacks (RF-P1-006/008/009), bounded retry+backoff (RF-P1-007), reconciliation to SoT (RF-P1-010), env-driven `Auth_Token` (RF-P6-011) |

## Recommended First Implementation Batch

- **Batch 1 (P0 boundary + redaction)** — RF-P0-001, RF-P0-003, RF-P0-004. Locks ownership and token-safety invariants first.
- **Batch 2 (P1 full mirror/callback contract)** — RF-P1-001, RF-P1-004, RF-P1-006, RF-P1-007, RF-P1-008, RF-P1-009, RF-P1-010, RF-P1-012. Proves the authenticated idempotent mirror/callback contract (half the hard gate).
- **Batch 3 (P3 durable-storage gate)** — RF-P3-001, RF-P3-002, a thin RF-P3-003, then RF-P3-004 (presigned URLs must come after token-free keys + uploaded artifact).

## Notes

- Tasks with **Blocking? = Yes** must ship before the private beta is declared ready; **No** tasks improve the product but do not gate the beta.
- Property numbers `P1`–`P18` map to the design's Correctness Properties.
- The durable-artifact smoke (trigger → render → upload → delete/restart worker → signed playback) is the single most important production-ready evidence artifact.
- Property-based tests apply where the design defines Correctness Properties; packaging/deploy/UX aesthetics use smoke/integration/lint/snapshot + a11y (axe + manual).
- The **Operator Evidence Gate** items (`EOG-001`–`EOG-004`) and **Phase 8** are external/deferred and excluded from the executable task list.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["RF-P0-001", "RF-P0-003", "RF-P0-006", "RF-P0-008", "RF-P4-001"] },
    { "id": 1, "tasks": ["RF-P0-002", "RF-P0-004", "RF-P1-001"] },
    { "id": 2, "tasks": ["RF-P0-005", "RF-P0-007", "RF-P1-002", "RF-P1-003", "RF-P1-004", "RF-P1-006", "RF-P3-001"] },
    { "id": 3, "tasks": ["RF-P0-009", "RF-P1-005", "RF-P1-007", "RF-P1-008", "RF-P1-012", "RF-P1-013", "RF-P2-001", "RF-P3-002"] },
    { "id": 4, "tasks": ["RF-P1-009", "RF-P1-010", "RF-P1-011", "RF-P2-002", "RF-P3-003"] },
    { "id": 5, "tasks": ["RF-P2-003", "RF-P2-004", "RF-P3-006", "RF-P4-002"] },
    { "id": 6, "tasks": ["RF-P2-005", "RF-P3-004", "RF-P4-003", "RF-P4-006", "RF-P4-007", "RF-P4-008", "RF-P4-009", "RF-P4-010", "RF-P4-011", "RF-P4-014", "RF-P4-015", "RF-P4-016", "RF-P4-017"] },
    { "id": 7, "tasks": ["RF-P2-006", "RF-P2-007", "RF-P2-009", "RF-P3-005", "RF-P4-005", "RF-P4-012", "RF-P4-013", "RF-P5-001"] },
    { "id": 8, "tasks": ["RF-P2-008", "RF-P3-007", "RF-P5-002", "RF-P5-008", "RF-P5-010", "RF-P5-011"] },
    { "id": 9, "tasks": ["RF-P5-003", "RF-P5-009", "RF-P5-013", "RF-P6-001", "RF-P6-002"] },
    { "id": 10, "tasks": ["RF-P5-004", "RF-P5-005", "RF-P5-007", "RF-P6-003", "RF-P6-004", "RF-P6-006"] },
    { "id": 11, "tasks": ["RF-P5-006", "RF-P5-012", "RF-P6-005", "RF-P6-007", "RF-P6-008", "RF-P6-011", "RF-P6-012"] },
    { "id": 12, "tasks": ["RF-P7-001", "RF-P7-005"] },
    { "id": 13, "tasks": ["RF-P7-002", "RF-P7-006", "RF-P7-008", "RF-P7-010", "RF-P7-011", "RF-P7-013"] },
    { "id": 14, "tasks": ["RF-P7-003", "RF-P7-004", "RF-P7-007", "RF-P7-009", "RF-P7-012"] },
    { "id": 15, "tasks": ["RF-P7-014"] },
    { "id": 16, "tasks": ["RF-P8-001", "RF-P8-002", "RF-P8-003", "RF-P8-004", "RF-P8-006", "RF-P8-007"] }
  ]
}
```
