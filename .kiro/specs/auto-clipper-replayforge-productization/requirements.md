# Requirements Document

## Introduction

This feature finishes the Auto Clipper + ReplayForge productization to a production-ready **private beta**. ReplayForge is the primary owner repository: it owns clip jobs, SQLite state, FFmpeg rendering, Whisper transcription, the Clip Studio editor, export, durable artifact storage, and playback. Streamclone integrates via HTTP only: it owns Analytics moments, moment context, the Export Moment trigger, the `/studio` redirect, mirrored job state, and callback authentication. The private ops repository (`streampulse-ops`) owns production environment, secrets, deploy, and image promotion; public repositories document contracts only.

The hard gate for calling Auto Clipper "production-ready" is durable artifact storage on Cloudflare R2 with signed playback and download URLs, plus a hardened, idempotent, authenticated job mirror/callback contract. This document specifies the end-to-end private-beta journey (discover moment → trigger job → track state → edit → render → store durably → share/recover) and maps every requirement to one of the delivery phases so the downstream task list can be organized by phase.

This is a private beta scoped to a single render worker with serialized renders and a small invite list. Live Helix clip creation and IRC/chat-spike auto-triggering are explicitly deferred to Phase 8. VOD source acquisition is limited to broadcaster-owned VODs accessed with token-scoped credentials.

## Glossary

- **ReplayForge**: The standalone Clip Studio application (Python API + worker at host `:8095`, SPA at `:8096`). Source of truth for clip jobs, SQLite state, FFmpeg render, Whisper transcription, editor, export, durable artifacts, and playback.
- **Streamclone**: The local watch-desk application (Go services + React frontend). Owns Analytics moments, moment context, Export Moment trigger, `/studio` redirect, and the mirrored job state with callback authentication.
- **Clip_Studio**: The ReplayForge editor UI where a user previews source media and edits trim, captions, template, and audio before render.
- **Render_Worker**: The single ReplayForge worker process that performs serialized clip renders during private beta.
- **Job_Store**: The ReplayForge SQLite datastore that holds clip job records as source of truth.
- **Job_Mirror**: The Streamclone-side representation of clip job state, synchronized from ReplayForge over HTTP.
- **Status_Callback**: The HTTP request ReplayForge sends to Streamclone to update mirrored job state.
- **Artifact_Store**: Cloudflare R2 object storage holding rendered clips and derived artifacts.
- **Signed_URL**: A time-limited presigned R2 URL used for artifact playback or download.
- **VOD**: A broadcaster-owned Twitch Video On Demand used as the clip source.
- **Clip_Job**: A unit of work that produces a rendered short-form clip from a source moment.
- **Job_State**: The current lifecycle state of a Clip_Job, one of the defined state set.
- **Ops_Repo**: The private `streampulse-ops` repository owning production environment, secrets, deploy, and image promotion.
- **Source_Image**: A container image built by Streamclone CI, published to `ghcr.io/aron-chu/streamclone/*`.
- **Promoted_Image**: A container image promoted by digest into `ghcr.io/aron-chu/streampulse/*` by Ops_Repo.
- **Auth_Token**: A server-side token used to authenticate job mutations and status callbacks between Streamclone and ReplayForge.
- **Invite_List**: The enumerated set of broadcaster accounts authorized to use the private beta.
- **Job_State_Set**: The complete set of allowed Job_State values: `queued`, `validating_source`, `downloading_source`, `transcribing`, `ready_for_edit`, `rendering`, `rendered`, `uploading_artifact`, `complete`, `failed`, `retryable_failed`, `expired`, `source_unavailable`, `auth_required`, `vod_unavailable`.

## Requirements

### Requirement 1 — Phase 0: Audit and Boundary Cleanup

**User Story:** As a maintainer, I want the ownership boundaries between ReplayForge and Streamclone enforced in code and docs, so that no render or transcription logic leaks into Streamclone Go services.

#### Acceptance Criteria

1. THE Streamclone SHALL exclude all FFmpeg rendering code from Streamclone Go services.
2. THE Streamclone SHALL exclude all Whisper transcription code from Streamclone Go services.
3. THE Streamclone SHALL exclude all clip editor and export code from Streamclone Go services.
4. THE ReplayForge SHALL contain the clip job lifecycle, SQLite Job_Store, FFmpeg render, Whisper transcription, editor, export, durable artifact, and playback responsibilities.
5. WHERE the StreamPulse extension or portal is present, THE StreamPulse extension SHALL operate without requiring ReplayForge.
6. THE Streamclone SHALL retain only Analytics moments, moment context, the Export Moment trigger, the `/studio` redirect, mirrored Job_State, and callback authentication as clipper-related responsibilities.
7. WHEN a Clip_Job token, access token, or refresh token is present, THE Streamclone SHALL exclude that token from bundles, URLs, filenames, logs, and display strings.
8. WHEN a Clip_Job token, access token, or refresh token is present, THE ReplayForge SHALL exclude that token from bundles, URLs, filenames, logs, and display strings.

### Requirement 2 — Phase 1: Job Model Hardening

**User Story:** As a broadcaster, I want honest and reliable job state that stays consistent between ReplayForge and Streamclone, so that I can trust what the interface tells me.

#### Acceptance Criteria

1. THE ReplayForge SHALL treat the SQLite Job_Store as the single source of truth for Clip_Job state.
2. THE Job_Mirror SHALL represent Job_State values only from the defined Job_State_Set.
3. WHEN ReplayForge changes a Clip_Job state, THE ReplayForge SHALL send a Status_Callback to Streamclone with the new Job_State.
4. WHEN Streamclone receives a Status_Callback carrying a Job_State that has already been applied, THE Streamclone SHALL return a success response without changing the Job_Mirror.
5. IF a Status_Callback arrives without a valid Auth_Token, THEN THE Streamclone SHALL reject the request with an unauthorized response.
6. IF a Clip_Job mutation request arrives without a valid Auth_Token, THEN THE ReplayForge SHALL reject the request with an unauthorized response.
7. THE Streamclone SHALL exclude every unauthenticated endpoint that mutates Clip_Job state.
8. WHEN the Job_Mirror and the Job_Store disagree on a Clip_Job state, THE Streamclone SHALL reconcile the Job_Mirror to the Job_Store value.
9. WHEN a Status_Callback delivery fails, THE ReplayForge SHALL retry delivery with a bounded retry count and backoff.
10. WHEN a Clip_Job is requested for a source that already has an active Clip_Job, THE ReplayForge SHALL return the existing Clip_Job rather than creating a duplicate.

### Requirement 3 — Phase 2: VOD-Backed Source Acquisition

**User Story:** As a broadcaster, I want to create clips from my own VODs using my authorized credentials, so that source acquisition is scoped and lawful.

#### Acceptance Criteria

1. WHEN a Clip_Job is created, THE ReplayForge SHALL set the Job_State to `validating_source`.
2. WHILE the Job_State is `validating_source`, THE ReplayForge SHALL confirm that the VOD is owned by the requesting broadcaster.
3. IF the VOD is not owned by the requesting broadcaster, THEN THE ReplayForge SHALL set the Job_State to `source_unavailable`.
4. WHERE VOD access requires broadcaster credentials, THE ReplayForge SHALL use token-scoped credentials limited to the `clips:edit` and VOD read scope.
5. IF the required broadcaster credentials are absent or expired, THEN THE ReplayForge SHALL set the Job_State to `auth_required`.
6. IF the requested VOD is deleted or unavailable at the source, THEN THE ReplayForge SHALL set the Job_State to `vod_unavailable`.
7. WHEN source validation succeeds, THE ReplayForge SHALL set the Job_State to `downloading_source`.
8. WHEN the source segment download completes, THE ReplayForge SHALL set the Job_State to `transcribing`.
9. THE ReplayForge SHALL invoke Streamlink and FFmpeg using argument arrays rather than shell string interpolation.

### Requirement 4 — Phase 3: Durable Artifact Storage

**User Story:** As a broadcaster, I want my rendered clips stored durably and shareable through secure links, so that clips survive worker restarts and can be reopened later.

#### Acceptance Criteria

1. WHEN a render completes, THE ReplayForge SHALL set the Job_State to `rendered`.
2. WHEN the Job_State becomes `rendered`, THE ReplayForge SHALL set the Job_State to `uploading_artifact` and upload the rendered clip to the Artifact_Store.
3. WHEN the artifact upload to the Artifact_Store succeeds, THE ReplayForge SHALL set the Job_State to `complete`.
4. IF the artifact upload to the Artifact_Store fails, THEN THE ReplayForge SHALL set the Job_State to `retryable_failed`.
5. WHEN a user requests playback of a completed Clip_Job, THE ReplayForge SHALL return a Signed_URL for playback from the Artifact_Store.
6. WHEN a user requests download of a completed Clip_Job, THE ReplayForge SHALL return a Signed_URL for download from the Artifact_Store.
7. THE ReplayForge SHALL set an expiration on every Signed_URL.
8. WHEN a Signed_URL is generated, THE ReplayForge SHALL exclude broadcaster tokens and Auth_Tokens from the URL.
9. WHEN the retention period for a stored artifact elapses, THE ReplayForge SHALL set the Job_State to `expired`.
10. THE ReplayForge SHALL store rendered artifacts using object keys that exclude broadcaster tokens and personally identifying tokens.

### Requirement 5 — Phase 4: Clip Studio UX and Product Polish

**User Story:** As a broadcaster, I want a sleek, dense, media-production editor that is usable on first screen, so that editing clips feels like a professional tool rather than a marketing page.

#### Acceptance Criteria

1. WHEN Clip_Studio loads, THE Clip_Studio SHALL present the editor surface as the first usable screen.
2. THE Clip_Studio SHALL render controls using a shadcn and Radix quality component set including icon controls, tabs, sheets, dropdowns, sliders, and toggles.
3. THE Clip_Studio SHALL exclude gradient fills, purple-and-blue AI-SaaS styling, decorative blobs, decorative orbs, and marketing hero sections.
4. THE Clip_Studio SHALL exclude nested card-inside-card layouts.
5. WHILE a Clip_Job is `downloading_source`, `transcribing`, `rendering`, or `uploading_artifact`, THE Clip_Studio SHALL display a progress state that names the current Job_State.
6. THE Clip_Studio SHALL provide source media preview before edit.
7. THE Clip_Studio SHALL provide trim, caption, template, and audio editing controls.
8. WHERE the viewport is narrow or mobile, THE Clip_Studio SHALL present editing surfaces using panes adapted to the viewport.
9. THE Clip_Studio SHALL support keyboard navigation and visible focus indicators for interactive controls.
10. WHEN a transcript is empty, THE ReplayForge SHALL render the clip without captions.

### Requirement 6 — Phase 5: Streamclone Integration

**User Story:** As a broadcaster, I want to launch a clip job from a Streamclone Analytics moment and follow its progress in Streamclone, so that discovery and tracking stay in the watch desk.

#### Acceptance Criteria

1. WHEN a user triggers Export Moment on a Streamclone Analytics moment, THE Streamclone SHALL send a Clip_Job creation request to ReplayForge with the moment context.
2. WHEN ReplayForge accepts a Clip_Job creation request, THE Streamclone SHALL record the returned Clip_Job identifier in the Job_Mirror.
3. WHEN a user opens `/studio` in Streamclone, THE Streamclone SHALL redirect to the ReplayForge Clip_Studio for the associated Clip_Job.
4. THE Streamclone SHALL display Job_State from the Job_Mirror using only values from the Job_State_Set.
5. THE Streamclone SHALL exclude render, FFmpeg, and Whisper code from Streamclone Go services.
6. WHEN Streamclone proxies clipper requests, THE Streamclone SHALL route them to the host ReplayForge API through the same-origin `/v1/clipper/*` path.
7. THE Streamclone SHALL exclude raw chat content from public client responses.
8. THE Streamclone SHALL compute clip candidate scoring on the server rather than in the client.

### Requirement 7 — Phase 6: Packaging and Private Ops Contract

**User Story:** As an operator, I want packaged deploy artifacts and a documented image-promotion contract, so that the private beta can be deployed through streampulse-ops without leaking secrets into public repos.

#### Acceptance Criteria

1. THE ReplayForge SHALL provide packaged deploy artifacts for the private beta backend and Clip_Studio.
2. THE Ops_Repo SHALL own the production environment, secrets, deploy configuration, and image promotion.
3. THE public repositories SHALL document deploy and secret contracts without containing production secrets.
4. WHEN Streamclone CI publishes a container image, THE Streamclone SHALL publish it as a Source_Image to `ghcr.io/aron-chu/streamclone/*`.
5. WHEN the Ops_Repo promotes an image, THE Ops_Repo SHALL reference the Promoted_Image in `ghcr.io/aron-chu/streampulse/*` by digest.
6. WHERE production runs before cutover, THE Ops_Repo SHALL be permitted to pin `ghcr.io/aron-chu/streamclone/*` images.
7. IF private ops cutover evidence is absent, THEN THE public repositories SHALL exclude any claim that image cutover is complete.
8. THE ReplayForge SHALL define the server-side Auth_Token configuration through environment-driven values rather than hardcoded secrets.

### Requirement 8 — Phase 7: Private Beta Validation

**User Story:** As an operator, I want the end-to-end private-beta journey validated including failure recovery, so that a limited invite list can use Auto Clipper reliably.

#### Acceptance Criteria

1. THE Render_Worker SHALL process renders serially as a single worker during private beta.
2. WHERE a broadcaster account is absent from the Invite_List, THE ReplayForge SHALL reject Clip_Job creation for that account.
3. WHEN a render fails for a recoverable reason, THE ReplayForge SHALL set the Job_State to `retryable_failed`.
4. WHEN a render fails for a non-recoverable reason, THE ReplayForge SHALL set the Job_State to `failed`.
5. WHEN a user retries a Clip_Job in `retryable_failed`, THE ReplayForge SHALL re-enqueue the Clip_Job and set the Job_State to `queued`.
6. THE ReplayForge SHALL validate the end-to-end journey from moment discovery through signed playback for an Invite_List account before the private beta is declared ready.
7. IF durable artifact storage and signed playback evidence are absent, THEN THE public repositories SHALL exclude any production-ready claim for Auto Clipper.
8. WHEN a Clip_Job enters `auth_required`, `source_unavailable`, or `vod_unavailable`, THE Clip_Studio SHALL display an explanatory state describing the condition.

### Requirement 9 — Phase 8: Later and Deferred

**User Story:** As a maintainer, I want live clip creation and chat-triggered clipping explicitly deferred, so that the private beta scope stays bounded.

#### Acceptance Criteria

1. THE private beta SHALL exclude live Helix clip creation from its scope.
2. THE private beta SHALL exclude IRC and chat-spike automatic triggering from its scope.
3. THE private beta SHALL exclude horizontal render scaling and concurrent render workers from its scope.
4. WHERE deferred capabilities are documented, THE documentation SHALL record the minimal Terms-of-Service, subscriber-only, and deleted-VOD handling as items in the risk register.
