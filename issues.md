# StreamPulse / Streamclone Trust Audit Issues

Date: 2026-07-06

Scope: read-only audit across Streamclone backend, Streamclone Pulse extension, StreamPulse public portal, public hub, channel console, and auto clipper / ReplayForge boundary.

This file is the **closed audit ledger** for the 2026-07-06 StreamPulse / Streamclone trust pass. All numbered issues below are **fixed, verified, documented, or intentionally closed**. Do not reopen unless a fresh validation failure directly traces to the relevant diff.

## Product Promise To Preserve

The strongest single promise across the extension, hub, channel console, and clips is:

> Honest live chat and emote intelligence, with private editor workflows built on top of that truth.

Do not let the product drift into three competing promises:

- Public discovery: "who is popping off right now" can overclaim when roster or viewer metadata is mixed with IRC-collected rollups.
- Editor productivity: clip queues can overclaim when a candidate is not yet renderable or privately playable.
- Demo polish: fixture and fallback layouts can overclaim when they hide missing backend data.

The follow-up agent should prefer backend-sourced truth, explicit empty states, and visible limitations over client-side repair or demo fallbacks.

## Verification Evidence From Audit

Commands run during audit:

```powershell
# Streamclone API contract heuristic, rerun via WSL because Windows python alias was unavailable.
wsl.exe --cd /mnt/c/Users/Aron/twitch-7tv-clone bash -lc 'if [ -f .cursor/skills/api-contract-drift-check/scripts/contract-keys.py ]; then python3 .cursor/skills/api-contract-drift-check/scripts/contract-keys.py; elif [ -f .github/skills/api-contract-drift-check/scripts/contract-keys.py ]; then python3 .github/skills/api-contract-drift-check/scripts/contract-keys.py; else echo contract-keys.py not found >&2; exit 1; fi'

# Focused portal e2e specs named by audit request.
Push-Location c:\Users\Aron\streamclone-pulse\streampulse-web
npm run test:e2e -- tests/e2e/analytics-hub-metrics-honesty.spec.ts tests/e2e/analytics-figma-parity.spec.ts --workers=1
Pop-Location
```

Important verification nuance:

- The Playwright output showed 17 tests run: 12 passed, 5 failed.
- The surrounding PowerShell command ended with `Pop-Location`, so the terminal exit code can be masked as 0. Treat the test log, not the final shell exit code, as truth.
- Failures observed during the original audit:
   - 3 failures in `analytics-figma-parity.spec.ts` looking for a missing or renamed "Pulse Moments Live" heading. These look like stale parity expectations unless that heading was intentionally required.
   - 2 failures in `analytics-hub-metrics-honesty.spec.ts` due to a React warning: `fetchPriority` prop passed to a DOM `img` element. This was not a data-truth failure, but the console guard correctly caught it.
- Later validation reported on 2026-07-06: `analytics-hub-metrics-honesty.spec.ts` passes 8/8 with no console warnings; keep P3-022 parity as the remaining known portal test drift.
- The API contract heuristic printed pulse-core exports and backend JSON-key samples. It is useful drift evidence, not a strict pass/fail gate.

## Auditor Spot-Check Updates

These notes reflect a second manual spot-check against the repo after this file was first drafted. Treat them as corrections and prioritization guidance for the auto agent.

- Use this file as the implementation backlog. The original highest-confidence immediate issues were P0-001, P1-002, P1-005, and P3-023; the current working-tree recheck below marks those lanes fixed or verified.
- P1-005 was more concrete than the original wording: the canonical `/analytics` route did not surface the `stats-fallback` warning that existed for the older dashboard home path. The current portal diff now appears to cover this; keep only verification/fallback smoke.
- P1-004 is real but partially mitigated on canonical `/analytics` by the hub coverage trust strip. Do not rewrite the whole hub first; fix labels/headline affordances that can still overclaim IRC collection.
- P1-008 fixed (2026-07-06). Late-start full-timeline regression tests + nested coverage propagation through `resolvePayloadCoverageStartOffset` / `pulseLiveAccess.coverageStartOffsetSeconds`.
- **P1-007 fixed (2026-07-06).** Clip inbox/renderability fields on backend + gated `/dashboard/clips` portal queue; post-batch review pass below.
- **P2-009 fixed (2026-07-06).** Backend Helix/VOD wins over local GQL-blocked pessimism in extension coverage card + diagnostics.
- **P2-016 fixed (2026-07-06).** Hosted public `/analytics/{login}` console hides operator sync/repair CTAs; e2e smoke asserts no Sync/Re-sync buttons.
- **P2-018 fixed (2026-07-06).** Extension Options/popup + portal hub/console label backend source; localhost cannot silently win on hosted prod; `dev:local` requires `VITE_ALLOW_LOCAL_BACKEND=1`.
- **P2-012 fixed (2026-07-06).** Portal bookmark adapter returns `{ supported: false, reason: 'private_beta' }` on public console; create/delete throw read-only message.
- **P3-026 fixed (2026-07-06).** Login/Setup quarantine copy clarifies public `/analytics` needs no beta key; `/login` and `/setup` redirect to `/analytics`.
- **P2-013 fixed (2026-07-06).** `/status` page reads `/v1/public/status` + extension health version; no placeholder-only copy.
- **P2-019 fixed (2026-07-06).** `status:hosted` ignores localhost env, validates hub/moments fields, prints portal pkg + API version.
- **P2-011 fixed (2026-07-06).** Go + TS golden-key contract tests for pulse, coverage, hub, moments, status, clips; CI `go test -run Contract`.
- **P2-021 fixed (2026-07-06).** Launch posture doc: [`docs/agent-notes/public-analytics-posture-2026-07.md`](../streamclone-pulse/docs/agent-notes/public-analytics-posture-2026-07.md).
- **P2-010 fixed (2026-07-06).** Public surfaces no longer route to `/setup` for install; extension Options remain canonical for backend/beta-key; requirements doc hosted-default banner; Login/Setup tombstone copy.
- **P2-020 fixed (2026-07-06).** `/dashboard/clips` links gated to dashboard only; `check:analytics-links` scans public surfaces; ReplayForge demoted to planned on landing roadmap.
- **P3-025 fixed (2026-07-06).** Deleted unreferenced `figmaMakeDemo.ts`; landing fixtures labeled illustrative; production analytics routes never inject xQc demo session.
- **P3-028 fixed (2026-07-06).** Hub emote economy donut uses backend `providerShares` only; honest unavailable copy when hourly rollups missing; corpus card no longer claims all providers observed.
- **P3-027 fixed (2026-07-06).** Drift classified in `analytics-surfaces.css`; `--fma-panel-alt` maps to `--sp-surface-2`; no drive-by figma-analytics.css rewrite.
- **P3-024 quarantined (2026-07-06).** `dashboard/Home.tsx` minimal private workspace landing; gated `/dashboard/clips` preserved; no public analytics links.
- **Audit closeout (2026-07-06).** All numbered issues closed — see [Final audit closeout](#final-audit-closeout-2026-07-06). Remaining work is known test hygiene and future roadmap only.

## Current Working Tree Recheck

Rechecked after the extension and analytics changes visible in the working tree on 2026-07-06. Do not delete the issues below yet; use these status notes to avoid duplicate work.

- **P1-005 verified (2026-07-06).** `analytics-hub-metrics-honesty.spec.ts` passes (8/8); `hubLiveWireFeed.test.tsx` covers paused cadence. `analyticsLandingPage.test.tsx` stats-fallback case exists but vitest render can hang/OOM locally — **e2e is authority** (see validation hygiene cleanup section).
- **P3-023 verified (2026-07-06).** No `fetchPriority` under `streampulse-web/`; honesty e2e passes with console guard clean.
- **Live Wire liveness work is now present.** The component gates live cadence on `feed.source === 'network'` and non-degraded hub state. Keep the trust addendum in `docs/website-portal/analytics-hub-liveness-tasks.md` as the source for final verification.
- **P1-003 fixed (2026-07-06).** Extension `PulseCoverage` includes `trackedFromStart`, `vodStatus`, `manualRetryAllowed`, `chatSource`, `chatSourceDetail`, and `copyKey`; `resolvePulseCoverage` / `coverageCardCopy` prefer backend `copyKey` + `message` when authoritative.
- **P0-001 fixed (2026-07-06).** Hosted `/v1/analytics/always-tracked` GET/POST require non-guest principal (`requireHostedNonGuestPrincipal`); routes wrapped in `pulseHostedAuthMiddleware` when hosted.
- **P1-002 fixed (2026-07-06).** `deletePulseWatchlist` only clears pool always-track when `IsLoginGloballyProtected` is false after delete; store regression test added (`pulse_watchlist_delete_test.go`, skips without `TEST_DATABASE_URL`).
- **P2-014 fixed (2026-07-06).** `livePoll.ts` uses jittered delays and capped exponential backoff after `GET_PULSE` failures; `computeLivePollDelayMs` + controller tests in `tests/livePoll.test.ts`.
- **P3-022 verified (2026-07-06).** `analytics-figma-parity.spec.ts` passes 9/9; canonical hub `h1` is **Command center** with `#section-pulse-moments` (not "Pulse Moments Live" page title).
- **Contract tests added (2026-07-06).** `api_contract_test.go`, `tests/coverageContract.test.ts`, `streampulse-web/tests/publicHub.contract.test.ts`; `contract-keys.py` root-walk fix in all mirrors.
- **P2-015 smoke (2026-07-06).** `analytics-hub-ux.spec.ts` requires bucket click → `/v1/public/hub/moments`; `deploy/smoke/test-hub-moments-hosted.sh` for hosted shape check.
- **P1-004 hardened (2026-07-06).** `HubCommandHeader` shows **Roster live** KPI when roster ≠ pool; matrix honesty e2e asserts roster label.
- **P2-017 documented (2026-07-06).** `docs/agent-notes/identity-model-2026-07.md`; `/login` and `/setup` quarantine comments; no public `/dashboard/clips` links on analytics surfaces.
- **P1-006 documented (2026-07-06).** `docs/agent-notes/clip-artifact-state-machine-2026-07.md`; `clipJobDisplayStatus()` honest labels in private clips queue.
- **P3-024 quarantined (2026-07-06).** `dashboard/Home.tsx` replaced with minimal private workspace landing; gated `/dashboard/clips` preserved.
- **Autonomous next-pass decision:** keep private dashboard/clip routes if they are gated and useful, but remove or hide public links/CTAs until identity and durable artifact semantics are settled. Do not delete clip/admin code just to simplify IA.
- **Autonomous next-pass decision:** canonical public analytics is `/analytics`. Treat `dashboard/Home.tsx`, `/dashboard`, `/analytics/streams`, setup/login/beta-key copy, and demo fixtures as quarantine/tombstone candidates unless a route is explicitly needed for private workspace flows.

## Pre-merge hygiene validation (2026-07-06)

Authority suite rerun for merge readiness. **Do not reopen audit-pass items 1–8** unless a failure below is traced to those diffs.

| Command | Exit | Result |
|---------|------|--------|
| `streampulse-web` `npm run typecheck` | 0 | pass |
| `npm run test:e2e -- analytics-hub-metrics-honesty.spec.ts --workers=1` | 0 | 8/8 pass |
| `npm run test:e2e -- analytics-visual-capture.spec.ts --workers=1` | 1 | 4/7 pass — **3 channel-console** cases timeout waiting for `region[name='Stream recap']` on hosted `/analytics/jynxzi/319253683932` (hub captures + backend-default check pass). **Unrelated to audit pass** (no recap/channel-console edits in pass). |
| `npm run test:e2e -- analytics-figma-parity.spec.ts --workers=1` | 0 | 9/9 pass |
| `streamclone-pulse` `npm run typecheck` | 2 | **pre-existing** — `PulseOverviewChart.tsx`, `recap*.ts`, `StreamRecapSection.tsx` (missing `../shared/coverage.ts`, emote fields). Vitest `npm test` 382/382 pass; `npm run build` pass. |
| `streamclone-pulse` `npm test` | 0 | 382/382 pass |
| `streamclone-pulse` `npm run build` | 0 | pass |
| `go test ./internal/analytics/... -run 'Pulse\|Coverage\|Watchlist\|AlwaysTrack\|Portal\|Hub\|Clip\|ReplayForge'` | 0 | pass |
| `npm test --prefix packages/pulse-core` | 0 | 43/43 pass |
| `bash deploy/smoke/test-013b-hosted.sh` | 0 | PASS (beta-key row skipped — `PULSE_BETA_KEY` unset) |
| `bash scripts/pulse-hosted-boundary-smoke.sh` | 1 | **hosted ops drift (not audit pass):** `/v1/portal/analytics/channels/ludwig/live` and `/streams` return 200 unauthenticated (script expects 401); `/v1/public/emotes/overview` 200 (script expects 404 post-atlas retirement). VOD canary PASS. |
| `python3 .cursor/skills/pulse/.../contract-keys.py` (WSL) | 0 | resolves repo; pulse-core + coverage keys print |

Reviewer pass (read-only): no secrets in diff artifacts; public `/analytics` surfaces have no `/dashboard/clips` links; gated dashboard nav only under `RequireAuth`; P0-001 `always-tracked` still behind `requireHostedNonGuestPrincipal` in hosted mode; contract tests green locally.

## Authority suite cleanup (2026-07-06 post-merge)

Post-audit validation debts cleared. **Do not reopen audit-pass items 1–8** unless a failure traces to those merges.

| Command | Exit | Result |
|---------|------|--------|
| `streamclone-pulse` `npm run typecheck` | 0 | pass — added `src/shared/coverage.ts`, emote optional fields, recap/chart type fixes |
| `streamclone-pulse` `npm test` | 0 | 382/382 pass |
| `streamclone-pulse` `npm run build` | 0 | pass |
| `npm run test:e2e -- analytics-visual-capture.spec.ts --workers=1` | 0 | **7/7 pass** — channel-console asserts default **ConsoleChannelView** landmarks (`Stream Recap` heading, timeline chart, `% chat coverage`) |
| `bash scripts/pulse-hosted-boundary-smoke.sh` (WSL) | 0 | **PASS** — raw `/v1/analytics/*` stays 401; sanitized `/v1/portal/analytics/*` and `/v1/public/emotes/overview` expect 200 (intentional public-safe BFF) |

## P1-007 clip inbox states (2026-07-06)

First-class candidate inbox/renderability fields added on backend + private portal queue.

| Command | Exit | Result |
|---------|------|--------|
| `go test ./internal/analytics/... -run 'ClipCandidate\|Clip\|ReplayForge\|ContractClip'` | 0 | pass — inbox/renderability enrichment + emote_spike_only + job-unverified tests |
| `streampulse-web` `npm test -- tests/clipCandidates.test.ts tests/ClipsPage.test.tsx` | 0 | 9/9 pass |
| `streampulse-web` `npm run test:e2e -- tests/e2e/dashboard-clips.spec.ts --workers=1` | 0 | 1/1 pass — honest ReplayForge labels (`Rendering queued`, `Worker ready (playback not verified)`) |

**Follow-up (not in this pass):** rights-sensitive music labeling, cross-stream caps beyond existing duplicate/hourly filters, durable artifact `playback_ready` gate (see `docs/agent-notes/clip-artifact-state-machine-2026-07.md`).

## P1-008 late-start chart regression (2026-07-06)

Regression coverage for extension full-timeline chart honesty when IRC tracking starts late (~45m). Extracted `resolveFullChartFromOffset` / `resolvePayloadCoverageStartOffset`; Overlay now passes `pulseLiveAccess.coverageStartOffsetSeconds` (includes nested `payload.coverage`).

| Command | Exit | Result |
|---------|------|--------|
| `streamclone-pulse` `npm test -- tests/chatActivityEmotes.test.ts tests/missedMoments.test.ts tests/resolvePulseLiveAccess.test.ts` | 0 | 48/48 pass — 45m late-start, no pre-coverage quiet bars, 120s tolerance, nested coverage propagation |
| `streamclone-pulse` `npm run typecheck` | 0 | pass |
| `streamclone-pulse` `npm test` | 0 | 390/390 pass |
| `streamclone-pulse` `npm run build` | 0 | pass |

## Post-batch review: P1-007 + P1-008 (2026-07-06)

Read-only trust review after both batches landed. **P1-007 and P1-008 remain closed** — batch scope checks pass; one unrelated portal typecheck debt noted below.

### Trust checks

| # | Check | Result | Evidence |
|---|-------|--------|----------|
| 1 | P1-007: no public clip CTAs / no implied public renderability | **Pass** | `/dashboard/clips` only under `RequireAuth` (`streampulse-web/src/routes/index.tsx`); no clip links on `/analytics` or public routes; `Clips.tsx` eyebrow **Private queue**; Render/Export buttons disabled (“Phase 2”); ReplayForge labels say **playback not verified** |
| 2 | P1-007: no raw chat / user messages | **Pass** | `TestClipCandidateJSONOmitsForbiddenFields` omits `rawChat`, `messages`, `chatter`, etc.; `statusCopy` uses deterministic recap labels only; UI shows aggregate `chatCount` / `topEmotes`, not message text |
| 3 | P1-007: backend ↔ portal status labels consistent | **Pass** | Go `enrichClipCandidateInbox` + portal `clipCandidates.ts` share enum values (`needs_source`, `worker_ready_unverified`, `emote_spike_only`); `clipCandidates.test.ts` + `clip_candidates_test.go` assert matching labels |
| 4 | P1-008: no quiet history before `coverageStartOffsetSeconds` | **Pass** | `resolveFullChartFromOffset` + `prepareChartRollups` start densification at coverage when >120s; `chatActivityEmotes.test.ts` 45m late-start asserts zero pre-coverage bars |
| 5 | P1-008: 120s tracked-from-start tolerance preserved | **Pass** | `FULL_CHART_STREAM_START_TOLERANCE_SEC === 120`; tests: 90s/120s → chart from 0; 121s+ / 45m → from coverage start; `resolvePulseLiveAccess` nested coverage propagation test |
| 6 | Authority suite green | **Mostly pass** | See validation table — scoped batch commands green; full `streamclone-pulse npm test` green on re-run (390/390); `streampulse-web npm run typecheck` fails on pre-existing debt |

### Post-batch validation

| Command | Exit | Result | Batch-related? |
|---------|------|--------|----------------|
| `go test ./internal/analytics/... -run 'ClipCandidate\|Clip\|ReplayForge'` | 0 | pass | P1-007 |
| `streamclone-pulse` `npm run typecheck` | 0 | pass | P1-008 / authority |
| `streamclone-pulse` `npm test` | 0 | **390/390 pass** (re-run; first attempt hit unrelated `livePoll.test.ts` timer flake 389/390) | unrelated flake on first run |
| `streamclone-pulse` `npm run build` | 0 | pass | authority |
| `streampulse-web` `npm run typecheck` | 2 | **fail** — `tests/analyticsConsoleUtils.test.ts`: missing export `resolveCanonicalSessionSlug` from analytics-console package | **unrelated debt** (pre-existing; not P1-007/P1-008) |
| `streampulse-web` `npm test -- tests/clipCandidates.test.ts` | 0 | 5/5 pass | P1-007 |

## P2-009 backend-first VOD/coverage UX (2026-07-06)

Extension coverage/backfill UI defers to backend Helix when `vodId` / `vodStatus` / authoritative `coverage` are present. Local GQL/ad-block notes demoted to footnotes via `backendResolvedVod` + `summarizeVodDebugBlockers({ backendVodResolved })`.

| Command | Exit | Result |
|---------|------|--------|
| `streamclone-pulse` `npm test -- tests/coverageDiagnostics.test.ts tests/coverageContract.test.ts tests/missedMoments.test.ts tests/pulseDebug.test.ts` | 0 | 19/19 pass — GQL blocked demoted when backend linked VOD; load CTA when backend has vodId |
| `streamclone-pulse` `npm run typecheck` | 0 | pass |
| `streamclone-pulse` `npm test` | 0 | 396/396 pass |
| `streamclone-pulse` `npm run build` | 0 | pass |

## P2-016 public console operator controls (2026-07-06)

Public `/analytics/{login}` hides operator sync/repair CTAs on hosted API. `ConsoleChannelView` keeps `enableSyncActions={usesLocalAnalyticsBackend()}`; `AnalyticsChart` no longer shows sync for TwitchTracker-only rows when `canSync` is false; hosted `startHistoricalSync` rejects; e2e asserts zero Sync/Re-sync buttons.

| Command | Exit | Result |
|---------|------|--------|
| `streampulse-web` `npm test -- tests/streamcloneAnalytics.test.ts` | 0 | 18/18 pass — hosted `startHistoricalSync` rejected |
| `streampulse-web` `npm run test:e2e -- tests/e2e/visual-analytics-smoke.spec.ts --workers=1` | 0 | **3/3 pass** — no Sync chat / Re-sync on default public console |
| Grep `streampulse-web/src/routes/analytics` for sync/backfill controls | — | **Only** `enableSyncActions={usesLocalAnalyticsBackend()}` in `ConsoleChannelView.tsx`; no operator sync strings in public route files |

## Validation hygiene cleanup (2026-07-06 post P2-009/P2-016)

Cleared pre-merge validation debts without reopening closed audit/P2 batches.

| Command | Exit | Result |
|---------|------|--------|
| `streampulse-web` `npm run typecheck` | 0 | pass — exported `resolveCanonicalSessionSlug` in `packages/analytics-console/src/utils/syncedLiveStream.ts` |
| `streamclone-pulse` `npm test -- tests/livePoll.test.ts` | 0 | 8/8 pass — fake-timer isolation (`beforeEach`/`afterEach`), fixed `Math.random`, avoid `vi.waitFor` advancing retry timer |
| `streamclone-pulse` `npm test` | 0 | **396/396 pass** |

**Documented (not rewritten):** `streampulse-web/tests/analyticsLandingPage.test.tsx` stats-fallback unit case can OOM/hang under full vitest (~50 min observed). File header + this note mark `tests/e2e/analytics-hub-metrics-honesty.spec.ts` as authority for stats-fallback honesty until render memory is isolated.

## P2-018 backend source / hosted-local divergence (2026-07-06)

Extension Options + popup and portal hub/console now label backend source explicitly. Hosted production cannot silently use localhost (extension `localBackendOptIn` gate; portal ignores localhost env unless `VITE_ALLOW_LOCAL_BACKEND=1` on `dev:local`).

| Command | Exit | Result |
|---------|------|--------|
| `streampulse-web` `npm run check:backend-url` | 0 | pass |
| `streampulse-web` `npm test -- tests/backendSource.test.ts` | 0 | 7/7 pass |
| `streamclone-pulse` `npm run typecheck` | 0 | pass |
| `streamclone-pulse` `npm test` | 0 | **399/399 pass** (includes `tests/extensionBackendSource.test.ts`) |

## P2-012 / P3-026 bookmark + account copy (2026-07-06)

Public `/analytics` bookmark adapter no longer silently returns empty lists; Login/Setup copy no longer implies beta key required for public hub.

| Command | Exit | Result |
|---------|------|--------|
| `rg` bookmark/beta/sign-in in `streampulse-web/src` | — | **No bookmark save UI** on public channel routes; `enableSyncActions={usesLocalAnalyticsBackend()}` only; Login/Setup quarantined with redirect |
| `streampulse-web` `npm test -- tests/portalBookmarks.test.ts tests/selectedMomentDisplay.test.ts` | 0 | 5/5 pass — `supported: false`, read-only create |
| `streampulse-web` `npm run typecheck` | 0 | pass |
| `streamclone-pulse` `npm run typecheck && npm test` | 0 | 399/399 pass |

## P2-010 / P2-020 / P3-025 / P3-028 / P3-027 launch cleanup (2026-07-06)

Public setup/beta-key affordances removed from landing nav and hero CTAs; clip queue stays dashboard-gated; demo/provider honesty on hub and landing.

| Command | Exit | Result |
|---------|------|--------|
| `rg` setup/beta/clips/ReplayForge in `streampulse-web/src` (public paths) | — | **No** `/dashboard/clips` or clip-queue CTAs outside `routes/dashboard/`; ReplayForge only on landing roadmap (planned) |
| `streampulse-web` `npm run check:analytics-links` | 0 | pass — href helpers + forbidden public clip-link scan |
| `streampulse-web` `npm run check:analytics-tailwind` | 0 | pass |
| `streampulse-web` `npm run typecheck` | 0 | pass |
| `streampulse-web` `npm test -- tests/landing.test.tsx` | 0 | 4/4 pass — no `/setup` or `/login` nav CTAs |
| `streampulse-web` `npm run test:e2e -- tests/e2e/analytics-visual-capture.spec.ts --workers=1` | 0 | 7/7 pass |
| `rg` `figmaMakeDemo` in `streampulse-web/src` | — | **0 hits** (file deleted) |
| `streamclone-pulse` `npm run typecheck && npm test` | 0 | 399/399 pass |

**Known (unchanged):** `streampulse-web/tests/analyticsLandingPage.test.tsx` can OOM/hang under full vitest — exclude for CI smoke; e2e honesty specs remain authority. Pre-existing unit drift: `analyticsConsoleVodLink.test.ts`, `hubTopEmotesTable.test.tsx`, `analyticsHubEmpty.test.tsx` (not introduced by this batch).

## P2-013 / P2-019 public status + hosted skew (2026-07-06)

| Command | Exit | Result |
|---------|------|--------|
| `curl -fsS https://api.streampulse.stream/v1/public/status` | 0 | `{"status":"operational","api":"up","degraded":false,...}` |
| `VITE_BACKEND_URL=https://api.streampulse.stream npm run status:hosted` | 0 | hub + moments + xqc sample + version `v0.3.0-rc18` |
| `streampulse-web` `npm run check:backend-url` | 0 | pass |
| `bash deploy/smoke/test-013b-hosted.sh` | 0 | PASS — public status shape keys |
| `bash scripts/pulse-hosted-boundary-smoke.sh` | 0 | PASS |

## P2-011 API contract drift gate (2026-07-06)

| Command | Exit | Result |
|---------|------|--------|
| `go test ./internal/analytics/... -run 'Contract\|Pulse\|Portal\|Hub\|Clip'` | 0 | pass — ExtensionCoverage, PublicHub, PublicHubMoments, PublicStatus, ExtensionPulse critical keys |
| `npm test --prefix packages/pulse-core` | 0 | 43/43 pass |
| `streamclone-pulse` `npm test -- tests/coverageContract.test.ts` | 0 | pass |
| `streampulse-web` `npm test -- tests/publicHub.contract.test.ts` | 0 | 4/4 pass |

## P2-021 public analytics posture (2026-07-06)

Decision doc: [`streamclone-pulse/docs/agent-notes/public-analytics-posture-2026-07.md`](../streamclone-pulse/docs/agent-notes/public-analytics-posture-2026-07.md) — public minute-level aggregates with backend sanitization; raw chat/user identity never public; bookmarks/clips private beta.

- P0: trust/security break; fix before public launch.
- P1: misleading product behavior or source-of-truth drift; fix before widening use.
- P2: important operational or test coverage gap; fix soon.
- P3: cleanup, IA, visual consistency, stale test, or documentation debt.

Trust classification:

- Trust-breaking: user can reasonably believe something false about tracking, coverage, privacy, renderability, or ownership.
- Misleading: copy or UI can imply more certainty than the backend has, but nearby context may partially correct it.
- Acceptable polish: cosmetic or internal consistency issue that does not materially change user belief.

## Issue P0-001: Legacy Global Protect Mutation Is Unauthenticated

Severity: P0

Trust classification: trust-breaking and operationally unsafe

Surface: backend hosted API, Protect / always-track

Primary files:

- `internal/analytics/api.go`
- `internal/analytics/pulse_watchlist.go`
- `internal/analytics/pulse_protected_cap.go`

Finding:

`Handler.Routes` registers these routes under `/v1/analytics`:

- `GET /v1/analytics/always-tracked`
- `POST /v1/analytics/always-tracked`

Those routes are not wrapped in hosted auth. In the same function, `/v1/analytics/channels/{login}/watch` is correctly gated in hosted mode, but `always-tracked` is not.

Spot-check detail:

- `setAlwaysTracked` only goes through `requirePulseWrite`, which checks read-only mode. It does not enforce hosted principal auth.
- `GET /v1/analytics/always-tracked` is lower risk than `POST`, but still exposes the global protected-channel list publicly. Treat it as ops-sensitive.

Why this matters:

- A public hosted caller could mutate global always-track state.
- A public caller could consume protected-channel capacity.
- A public caller could trigger tracking of arbitrary channels or clear protected channels.
- A public caller could enumerate globally protected channels through `GET /always-tracked` unless that read route is intentionally public and sanitized.
- The extension and portal promise "Protect this channel" as a per-principal user intent, but this legacy route is global state.

What I would want:

1. Either remove the legacy mutation route from hosted mode, or require operator-only auth for it.
2. Keep `/v1/pulse/watchlist` as the user-facing Protect API.
3. Restrict `GET /v1/analytics/always-tracked` in hosted mode too, unless there is a deliberate public product reason to expose the exact list.
4. If read access to global always-tracked is still needed, return only sanitized aggregate/status data in hosted mode, not the mutable global control surface.
5. Add a hosted auth boundary test proving unauthenticated `POST /v1/analytics/always-tracked` returns 401/403.

Suggested validation:

```bash
go test ./internal/analytics/... -run 'AlwaysTracked|Watchlist|Hosted|Pulse'
curl -i -X POST https://api.streampulse.stream/v1/analytics/always-tracked -d '{"login":"xqc","alwaysTrack":true}'
```

Expected result after fix:

- Hosted unauthenticated mutation is rejected.
- Principal-scoped watchlist Protect still works with valid hosted user state principal.

## Issue P1-002: Deleting One Principal's Watchlist Entry Can Clear Another Principal's Protect State

Severity: P1

Trust classification: trust-breaking for protected-channel expectations

Surface: backend Protect / always-track semantics

Primary file:

- `internal/analytics/pulse_watchlist.go`

Finding:

`deletePulseWatchlist` scopes deletion by `principal.ID`, which is good. But after deleting a row, if the deleted entry had `AlwaysTrack=true`, it unconditionally calls:

- `h.collector.SetPoolAlwaysTrack(login, false)`
- `h.collector.ReleaseForPrincipal(login, principal.ID)`

There is no visible check that another principal still has `always_track=true` for the same login.

Why this matters:

If two users protect `xqc`, and one removes it, the shared collector pool can be told that `xqc` is no longer always-track even though another principal still expects protection. That violates the "Protect this channel for future streams" promise.

What I would want:

1. Add a store query such as `AnyPulseWatchlistAlwaysTrack(ctx, login)` after deletion.
2. Only clear pool-level always-track when no remaining principal has `always_track=true` for that login.
3. Add a regression test with two principals protecting the same login; delete one; assert pool always-track remains true.

Suggested validation:

```bash
go test ./internal/analytics/... -run 'Watchlist|AlwaysTrack|Protected'
```

Expected result after fix:

- Principal A can remove their watchlist row without breaking Principal B's protection.
- Global cap accounting and collector priority stay consistent.

## Issue P1-003: Backend Coverage Truth Is Richer Than Extension And Portal Types Consume

Severity: P1

Trust classification: misleading; can become trust-breaking in late-join and VOD-blocked cases

Surfaces: extension overlay, portal console, backend BFF

Primary files:

- `internal/analytics/pulse_coverage.go`
- `internal/analytics/extension_api.go`
- `../streamclone-pulse/src/shared/messages.ts`
- `../streamclone-pulse/src/ui/resolvePulseLiveAccess.ts`
- `../streamclone-pulse/src/ui/missedMoments.ts`
- `../streamclone-pulse/streampulse-web/src/lib/figmaSessionAnalytics.ts`

Finding:

Backend `ExtensionCoverage` includes rich truth fields:

- `trackedFromStart`
- `vodStatus`
- `manualRetryAllowed`
- `chatSource`
- `chatSourceDetail`
- `copyKey`
- `message`
- `coverageStartOffsetSeconds`
- `coverageEndOffsetSeconds`
- `missingRanges`
- `canBackfill`
- `backfillReason`

The extension `PulseCoverage` type in `messages.ts` omits several of these fields, especially `trackedFromStart`, `vodStatus`, `manualRetryAllowed`, `chatSource`, `chatSourceDetail`, and `copyKey`.

The extension and portal therefore still contain local derivation logic for state/copy. Some derivation is practical, but it weakens the backend-as-source-of-truth contract.

Why this matters:

- A late-join user can see different nuance in extension vs portal.
- GQL/ad-block failure and backend Helix success can be represented inconsistently.
- The backend's 120s stream-start tolerance is implemented, but users may not see an explicit explanation.
- Client fallback code can become a silent repair layer that masks backend contract drift.

What I would want:

1. Update shared TS coverage types to include all backend coverage fields.
2. Make UI copy prefer backend `coverage.copyKey` and `coverage.message` where available.
3. Keep client derivation only as a compatibility shim for older backend payloads, and label it in code as legacy fallback.
4. Add contract tests comparing Go `ExtensionCoverage` JSON keys to extension and portal TS coverage types.
5. Add UI tests for these specific states:
   - `full_stream_tracked` with first rollup <= 120s.
   - `partial_tracking` with first rollup > 120s and no VOD.
   - `waiting_for_vod` while live.
   - `missing_ranges_detected` with `canBackfill=true`.
   - `backfill_running` and `backfill_failed`.
   - GQL blocked locally but backend has `vodStatus=available`.

Suggested validation:

```bash
go test ./internal/analytics/... -run 'Coverage|Pulse'
npm test --prefix packages/pulse-core
cd ../streamclone-pulse && npm test -- tests/coverage*.test.ts
cd ../streamclone-pulse && npm run typecheck
```

Expected result after fix:

- Extension, portal, and backend show the same coverage state and same user-facing meaning for late starts, VOD waits, backfill availability, and retry cases.

## Issue P1-004: Hub Can Overstate Live Tracking By Blending Roster-Live And IRC-Collected Channels

Severity: P1

Trust classification: misleading; trust-breaking if screenshots omit explanatory copy

Surfaces: `/analytics` hub, live matrix, KPI band, global activity chart

Primary files:

- `internal/analytics/hub_overview.go`
- `../streamclone-pulse/streampulse-web/src/lib/publicHub.ts`
- `../streamclone-pulse/streampulse-web/src/lib/hubMetricHelpers.ts`
- `../streamclone-pulse/streampulse-web/src/ui/components/analytics/LiveChannelsMatrix.tsx`
- `../streamclone-pulse/streampulse-web/tests/e2e/analytics-hub-metrics-honesty.spec.ts`

Finding:

The public hub uses corpus pipeline / Top-500 roster data as part of live hub presentation. In the portal normalizer, `coverage.liveChannels` is coerced to at least `corpusPipeline.roster.live`.

The e2e tests now assert wording that separates:

- live pool
- IRC collecting
- roster
- corpus lifetime streams

That is good. The remaining risk is that high-level labels and screenshots can still look like "tracked live channels" when some rows are metadata/roster live rather than IRC-collected minute rollups.

Nuance from spot-check:

- The canonical `/analytics` page now has a coverage trust strip that breaks down collecting, warming, metadata-only, uncovered live, and deficit rows. That is a real mitigation.
- The highest-risk remaining areas are KPI band/headline language, `HubCommandHeader`, matrix headers, and any social/visual capture where the expanded coverage strip is not visible.
- Do not treat this as a request to rewrite the whole hub. Prefer narrow label and affordance fixes first.

Why this matters:

A skeptical streamer or competitor can screenshot the hub and say it claims to track more live streams than it actually collects via IRC. If the pool is saturated, the UI must surface saturation as a capacity truth, not hide it under a healthy-looking live count.

What I would want:

1. Audit visible headline labels for `liveChannels`, `poolSize`, `collectorActive`, `collectorMax`, `roster.live`, and `liveCollectorDeficitRows`.
2. Reserve "tracked" for channels with rollup/IRC collection evidence.
3. Use "live in roster" or "live in pool" only when collection is not guaranteed.
4. Ensure the coverage trust strip is present and discoverable on canonical `/analytics`, not just old dashboard surfaces.
5. Add or preserve a persistent capacity/saturation state when roster live > collector active.
6. Keep `analytics-hub-metrics-honesty.spec.ts` as an authority test, not archaeology.

Suggested validation:

```bash
cd ../streamclone-pulse/streampulse-web
npm run test:e2e -- tests/e2e/analytics-hub-metrics-honesty.spec.ts --workers=1
curl -fsS "https://api.streampulse.stream/v1/public/hub?activityWindow=30m" | jq '{poolSize, rows:(.liveChannels|length), coverage, collector:{active:.corpusPipeline.collectorActive,max:.corpusPipeline.collectorMax,maxActiveIrc:.corpusPipeline.maxActiveIrcChannels,rosterLive:.corpusPipeline.roster.live}}'
```

Expected result after fix:

- A user can distinguish "live on Twitch / in roster" from "Pulse is actively collecting chat rollups" in every hub panel.

## Issue P1-005: Public Hub Stats Fallback Can Make A Broken Hub Look Operational

Severity: P1

Trust classification: misleading

Surfaces: `/analytics`, public hub fallback

Primary files:

- `../streamclone-pulse/streampulse-web/src/lib/publicHub.ts`
- `../streamclone-pulse/streampulse-web/src/hooks/usePublicHubData.ts`
- `../streamclone-pulse/streampulse-web/src/routes/analytics/AnalyticsLandingPage.tsx`
- `../streamclone-pulse/streampulse-web/src/ui/components/hub/HubDataHealthBanner.tsx`

Finding:

When `/v1/public/hub` fails, `fetchPublicHub` can fall back to `/v1/public/stats` and `/v1/public/status`, returning `loadSource: 'stats-fallback'`, `hubEndpointOk: false`, and a normalized hub object with aggregate stats.

That fallback is useful for avoiding a blank page, but it can also make the page look operational while live hub data is unavailable.

Concrete wiring gap from spot-check:

- `HubDataHealthBanner` already has copy for the fallback/unavailable case, but it is wired on the older dashboard home path, not the canonical `/analytics` page.
- `AnalyticsLandingPage` reads cache refresh / critical coverage state paths, but does not make `hub.loadSource === 'stats-fallback'` or `hub.hubEndpointOk === false` unmistakable on the canonical public hub.
- Therefore this is not only a generic "make fallback louder" request. It is a bug-shaped gap on the actual route users see.

Why this matters:

- Lighthouse/social screenshots could show healthy-looking corpus counters while live panels are degraded.
- Internal demos may normalize fake/partial hub behavior.
- Users may not understand that live discovery, moments, and matrix panels are unavailable.

What I would want:

1. Wire an explicit `stats-fallback` / `hubEndpointOk === false` banner into `AnalyticsLandingPage`.
2. Reuse `HubDataHealthBanner` if appropriate, but verify its styling/copy fits the canonical Figma analytics shell.
3. Ensure every live panel fed by fallback data renders an honest empty/degraded state.
4. Log or surface enough operator context to detect this in production.
5. Add an e2e test that forces `/v1/public/hub` failure while `/v1/public/stats` succeeds and asserts unmistakable degraded copy on `/analytics`.

Suggested validation:

```bash
cd ../streamclone-pulse/streampulse-web
npm run test:e2e -- tests/e2e/analytics-hub-ux.spec.ts --workers=1
```

Expected result after fix:

- Stats fallback is acceptable polish, not a silent product substitution.
- Canonical `/analytics` warns when the hub endpoint is unavailable even if aggregate stats still load.

## Issue P1-006: Auto Clipper `ready` Must Mean Durable Private Playback, Not Only ReplayForge Job State

Severity: P1

Trust classification: trust-breaking for editor workflow

Surfaces: `/v1/pulse/clips`, dashboard clips route, ReplayForge mirror

Primary files:

- `internal/analytics/clip_candidates_api.go`
- `internal/analytics/clip_candidates_store.go`
- `internal/analytics/clip_replayforge_api_test.go`
- `internal/analytics/replayforge_client.go`
- `migrations/000062_auto_clipper_candidates.up.sql`
- `migrations/000063_auto_clipper_replayforge_jobs.up.sql`
- `../streamclone-pulse/streampulse-web/src/lib/clipCandidates.ts`
- `../streamclone-pulse/streampulse-web/src/routes/dashboard/Clips.tsx`

Finding:

Important correction: `/v1/pulse/clips` is mounted via `h.registerPulseClipRoutes(r)` inside `PulseRoutes`. Hosted mode wraps `/v1/pulse` in hosted auth, and clip handlers additionally call private clip principal checks.

The remaining issue is state semantics. Tests show a job can become `ready` when ReplayForge status returns `state: ready` and `artifact_available: 1`. That is not the same as proving StreamPulse has a durable hosted artifact with private signed playback.

Why this matters:

- Editors may believe a clip is playable when the only copy is still on a VPS or ReplayForge temporary disk.
- ReplayForge 48h retention or worker cleanup could leave the portal with a stale `ready` mirror.
- A private editor queue becomes misleading if candidate -> job -> artifact -> playback is not a fully verified state machine.

What I would want:

Minimum artifact drill before treating hosted clipper as product:

1. Upload a fixture render artifact to durable storage, preferably R2.
2. Store sanitized artifact metadata in Postgres.
3. Serve a signed private read URL through the portal/backend.
4. Delete worker-local / ReplayForge-local file.
5. Confirm portal playback still works from durable storage.
6. Confirm portal JSON never exposes raw filesystem paths, internal object keys, callback tokens, or unsigned URLs.

Desired state machine:

```text
canonical candidate
  -> per-principal state: new | saved | dismissed
  -> ReplayForge job: queued | rendering | failed | source_unavailable
  -> artifact mirror: missing | uploading | durable_ready | expired | deleted
  -> portal playback: signed_url_issued | signed_url_expired
```

Rules:

- `ReplayForge ready` is not `portal playable`.
- `artifact_available` is not enough unless tied to durable object metadata.
- Candidate source status must be first-class: `source_available`, `source_missing`, `source_expired`, `needs_vod`, etc.

Suggested validation:

```bash
go test ./internal/analytics/... -run 'Clip|ReplayForge|Artifact'
curl -i https://api.streampulse.stream/v1/pulse/clips
```

Expected result after fix:

- Portal only shows "Ready" for clips with durable private playback available.

## Issue P1-007: Clip Candidate Generation Can Produce Unrenderable Or Low-Quality Rows Without First-Class Inbox States

**Status: fixed (2026-07-06)** — see [P1-007 clip inbox states](#p1-007-clip-inbox-states-2026-07-06) and [post-batch review](#post-batch-review-p1-007--p1-008-2026-07-06).

Severity: P1

Trust classification: misleading for editor productivity

Surfaces: auto clipper candidate queue

Primary files:

- `internal/analytics/clip_candidates_api.go`
- `internal/analytics/clip_candidates_store.go`
- `internal/analytics/clip_candidates*.go`
- `../streamclone-pulse/streampulse-web/src/routes/dashboard/Clips.tsx`

Finding:

Clip candidates are generated from recaps / rollups. That is a good deterministic base, but not every high-scoring minute is renderable or editorially useful.

Known risky cases:

- Missing source video / VOD unavailable.
- Emote spam produces high score without a real clip hook.
- Licensed music or rights-sensitive content in VOD segment.
- Duplicate spikes across nearby minutes.
- Operator thresholds tuned too loose or too strict.

What I would want:

1. Add explicit inbox states for source availability and renderability.
2. Add cross-stream/channel-hour caps and duplicate cooldowns.
3. Cap emote-dominance contribution for clip candidacy, or label `pick_reason=emote_spike_only` as lower confidence.
4. Keep candidate explanation deterministic and sanitized: no raw chat, no user messages.
5. Keep LLM/AI wrappers downstream of deterministic plan summaries only.

Suggested validation:

```bash
go test ./internal/analytics/... -run 'ClipCandidate|Recap'
```

Expected result after fix:

- Editors can distinguish "good moment candidate" from "renderable private clip" and "needs source".

## Issue P1-008: Extension Full-Timeline Chart Depends On Coverage Propagation; Add Regression Coverage For Late Starts

**Status: fixed (2026-07-06)** — see validation table in [P1-008 late-start chart regression](#p1-008-late-start-chart-regression-2026-07-06) and [post-batch review](#post-batch-review-p1-007--p1-008-2026-07-06) above.

Severity: P1

Trust classification: misleading if regressed

Surfaces: extension chart, late-join coverage honesty

Primary files:

- `../streamclone-pulse/src/ui/chatActivityEmotes.ts`
- `../streamclone-pulse/src/ui/LiveStatsBand.tsx`
- `../streamclone-pulse/src/ui/Overlay.tsx`
- `../streamclone-pulse/tests/chatActivityEmotes.test.ts`

Finding:

The current `prepareChartRollups` implementation is better than the initial concern: for full-window charts with full rollups, it uses `coverageStartOffsetSeconds` and starts densification from coverage start when coverage start is > 120 seconds.

However, this honesty depends on `coverageStartOffsetSeconds` being correctly propagated from backend payload -> overlay -> `LiveStatsBand` -> `prepareChartRollups`.

Why this matters:

The product's most sensitive promise is late-join honesty: do not imply data exists at 00:00 when rollups begin at minute N.

What I would want:

1. Add or strengthen tests for a stream with first rollup at 45 minutes.
2. Assert full chart does not synthesize 00:00 through 44:00 as quiet chat.
3. Assert copy says rollups begin at coverage start.
4. Assert 90-second first rollup is treated as tracked-from-start only because of the documented 120s tolerance.

Suggested validation:

```bash
cd ../streamclone-pulse
npm test -- tests/chatActivityEmotes.test.ts
npm run typecheck
npm run build
```

Expected result after fix:

- Late-start charts show coverage start -> now, not fabricated quiet history.

## Issue P2-009: GQL Blocked vs Backend Helix Resolution Is Not Prominent Enough In Primary UX

**Status: fixed (2026-07-06)** — see [P2-009 backend-first VOD/coverage UX](#p2-009-backend-first-vodcoverage-ux-2026-07-06).

Severity: P2

Trust classification: misleading

Surfaces: extension coverage diagnostics, missed moments flow

Primary files:

- `../streamclone-pulse/src/background/api.ts`
- `../streamclone-pulse/src/background/twitchPageInject.ts`
- `../streamclone-pulse/src/shared/pulseDebug.ts`
- `../streamclone-pulse/src/ui/coverageDiagnostics.ts`
- `../streamclone-pulse/src/ui/Overlay.tsx`

Finding:

The system can detect or explain GQL/ad-block failure in debug paths, and backend Helix can resolve VOD independently. But primary UX copy may still make the extension look more pessimistic than the backend when local GQL is blocked.

Why this matters:

If backend Helix resolves `vodId`, backend optimism should win. The user should not see a worse state just because page-level GQL discovery failed.

What I would want:

1. Treat backend `vodStatus` / `canBackfill` as authoritative.
2. Surface GQL-blocked as a diagnostic note, not the primary state when backend has resolved the VOD.
3. Add a test where local GQL discovery fails but backend payload says VOD/backfill is available.

Suggested validation:

```bash
cd ../streamclone-pulse
npm test -- tests/*coverage*.test.ts tests/*missed*.test.ts
```

Expected result after fix:

- Backend Helix success produces consistent extension and portal state.

## Issue P2-010: Hosted / Local Backend Override Can Drift And Is Not Fully Enforced

**Status: fixed (2026-07-06)** — see [P2-010 / P2-020 / P3-025 / P3-028 / P3-027 launch cleanup](#p2-010--p2-020--p3-025--p3-028--p3-027-launch-cleanup-2026-07-06). Enforcement details also in [P2-018](#p2-018-backend-source--hosted-local-divergence-2026-07-06).

Severity: P2

Trust classification: operationally misleading

Surfaces: portal defaults, extension options, setup docs

Primary files:

- `../streamclone-pulse/src/shared/storage.ts`
- `../streamclone-pulse/src/options/options.tsx`
- `../streamclone-pulse/streampulse-web/src/lib/auth.ts`
- `../streamclone-pulse/streampulse-web/src/routes/index.tsx`
- `../streamclone-pulse/streampulse-web/src/routes/public/Setup.tsx`
- `../streamclone-pulse/README.md`
- `../streamclone-pulse/docs/pulse-extension/requirements.md`

Finding:

The portal and extension mostly default to hosted API, which is correct. But setup/docs and extension options still permit or describe local backend flows in ways that can confuse users or produce contract drift.

Specific risks:

- `/setup` is routed to `/analytics`, while an unused setup component still contains copy-config and backend override flow.
- Extension options save arbitrary trimmed backend URLs without a visible HTTPS/non-localhost guard.
- Some docs still mention local defaults or beta-key setup in ways that conflict with public `/analytics` positioning.

What I would want:

1. Decide whether `/setup` exists. If not, delete or tombstone the component and docs.
2. Validate extension backend override values: hosted HTTPS or explicit local dev only.
3. Add UI warning for non-hosted backend override.
4. Add tests that production portal build does not accidentally point at localhost.

Suggested validation:

```bash
cd ../streamclone-pulse/streampulse-web
npm run check:backend-url
npm run typecheck
cd ..
npm run typecheck
```

Expected result after fix:

- Hosted production cannot silently point at local `:8090`.
- Local dev remains possible but explicit.

## Issue P2-011: API Contract Drift Check Is Heuristic, Not A CI-Grade Gate

**Status: fixed (2026-07-06)** — see [P2-011 API contract drift gate](#p2-011-api-contract-drift-gate-2026-07-06).

Severity: P2

Trust classification: operational risk

Surfaces: backend BFF, pulse-core, extension messages, portal clients

Primary files:

- `.github/skills/api-contract-drift-check/SKILL.md`
- `.cursor/skills/api-contract-drift-check/scripts/contract-keys.py`
- `internal/analytics/extension_api.go`
- `packages/pulse-core/`
- `../streamclone-pulse/src/shared/messages.ts`
- `../streamclone-pulse/streampulse-web/src/lib/`

Finding:

The contract script reports export names and common JSON keys. It does not fail CI on missing fields such as backend coverage fields that are not present in TS types.

Why this matters:

Contract drift is currently detectable by careful manual audit. It is not reliably blocked by CI.

What I would want:

1. Add schema snapshots or generated OpenAPI/JSON schema for critical payloads:
   - extension pulse payload
   - coverage payload
   - public hub payload
   - portal stream minutes/peaks/coverage-truth
   - pulse clips list/job payload
2. Add a CI check that fails when backend fields are missing from shared TS types unless explicitly marked backend-only.
3. Keep the heuristic script as a warning, but add strict tests for critical contracts.

Suggested validation:

```bash
go test ./internal/analytics/... -run 'Contract|Pulse|Portal|Hub|Clip'
npm test --prefix packages/pulse-core
cd ../streamclone-pulse && npm run typecheck && npm test
cd ../streamclone-pulse/streampulse-web && npm run typecheck && npm test
```

Expected result after fix:

- Breaking contract drift is caught before manual QA.

## Issue P2-012: Portal Saved Moments Adapter Is A No-Op In Public Console Context

**Status: fixed (2026-07-06)** — see [P2-012 / P3-026 bookmark + account copy](#p2-012--p3-026-bookmark--account-copy-2026-07-06).

Severity: P2

Trust classification: misleading if controls are visible

Surfaces: channel console, saved moments/bookmarks

Primary files:

- `../streamclone-pulse/streampulse-web/src/lib/streamcloneAnalytics.ts`
- `internal/analytics/bookmarks.go`
- `migrations/000038_pulse_bookmarks.up.sql`
- `migrations/000040_pulse_bookmarks_principal.up.sql`

Finding:

The portal analytics adapter can return empty bookmark lists and throw beta-key errors for create/delete depending on context. If public channel analytics exposes saved-moment affordances, this misleads users into thinking bookmarks are empty or broken rather than unavailable in public mode.

What I would want:

1. Verify whether save/delete controls render on public `/analytics/{login}`.
2. If public mode has no principal, hide bookmark controls or label them as sign-in/private features.
3. If bookmarks are supported for guest/device principal, route through the real `/v1/pulse/bookmarks` contract.

Suggested validation:

```bash
cd ../streamclone-pulse/streampulse-web
npm test -- tests/*bookmark*.test.ts tests/*selectedMoment*.test.ts
```

Expected result after fix:

- Users never see silent empty saved moments when the real reason is missing identity or unavailable principal state.

## Issue P2-013: Public Status Page Is Placeholder-Only Despite Backend Status Endpoint

**Status: fixed (2026-07-06)** — see [P2-013 / P2-019 public status + hosted skew](#p2-013--p2-019-public-status--hosted-skew-2026-07-06).

Severity: P2

Trust classification: misleading operationally

Surfaces: `/status`, hub health strip, public status endpoint

Primary files:

- `../streamclone-pulse/streampulse-web/src/routes/public/Status.tsx`
- `internal/analytics/public_api.go`
- `deploy/smoke/test-013b-hosted.sh`

Finding:

Backend public routes include `/v1/public/status`, and hosted smoke checks it. The public `/status` page is still placeholder-only.

Why this matters:

Users and operators can see three different health narratives:

- `/status` placeholder
- hub health strip
- hosted smoke / backend status endpoint

What I would want:

1. Either wire `/status` to `/v1/public/status` or remove/deprioritize the route until it is truthful.
2. Use the same source as hub health when possible.
3. Add a smoke/e2e check that `/status` does not contradict `/analytics` health copy.

Suggested validation:

```bash
curl -fsS https://api.streampulse.stream/v1/public/status | jq .
cd ../streamclone-pulse/streampulse-web && npm run test:e2e -- tests/e2e/*status*.spec.ts --workers=1
```

Expected result after fix:

- Public health surfaces agree or clearly describe different scopes.

## Issue P2-014: Extension Polling Has Cleanup But Lacks Backoff/Jitter Discipline

Severity: P2

Trust classification: operational risk, not direct trust break

Surfaces: extension service worker, content live poll

Primary files:

- `../streamclone-pulse/src/background/tracking.ts`
- `../streamclone-pulse/src/background/service-worker.ts`
- `../streamclone-pulse/src/content/livePoll.ts`
- `../streamclone-pulse/src/content/entry.ts`
- `../streamclone-pulse/src/shared/storage.ts`

Finding:

Timer cleanup exists: polling intervals are cleared on untrack/pause/resume/start. The content live poll also stops on route changes and syncs state.

The gap is retry/backoff/jitter. Polling is mostly fixed interval. On hosted API errors or many open tabs, this can become noisy.

What I would want:

1. Add backoff after repeated failed polls.
2. Add jitter to avoid synchronized extension clients.
3. Keep bounded window contract: no full timelines except explicit user request / full chart request.
4. Add tests for route changes, backgrounding, collapse/stop, and setting poll interval.

Suggested validation:

```bash
cd ../streamclone-pulse
npm test -- tests/*poll*.test.ts tests/*tracking*.test.ts
npm run typecheck
```

Expected result after fix:

- No double-polling or tight error loops on route changes and API failures.

## Issue P2-015: Hosted Moments Bucket Smoke Is Missing After Bucket-Merge Fix

Severity: P2

Trust classification: misleading if bucket clicks show stale or partial rows

Surfaces: `/analytics` global activity chart, `/v1/public/hub/moments`, Pulse Moments table/inspector

Primary files:

- `internal/analytics/hub_historical_moments.go`
- `../streamclone-pulse/streampulse-web/src/lib/publicHub.ts`
- `../streamclone-pulse/streampulse-web/src/lib/pulseMomentRow.ts`
- `../streamclone-pulse/streampulse-web/src/ui/components/analytics/PulseMomentsLivePanel.tsx`
- `../streamclone-pulse/streampulse-web/tests/e2e/analytics-hub-ux.spec.ts`
- `../streamclone-pulse/docs/website-portal/analytics-hub-liveness-tasks.md`

Finding:

The audit covered bucket filtering and noted no active `bucketFilterMiss` symbol. The spot-check adds a stronger requirement: after the recent bucket merge / semantic table work, hosted deploy verification must prove bucket clicks use `/v1/public/hub/moments` and return rows with populated viewers and emote breakdowns where backend data exists.

Why this matters:

This is one of the easiest places for the hub to look alive while quietly falling back to client-side filtered or partially enriched rows. A user clicking a global activity bucket expects the table and inspector to represent that backend bucket, not a locally repaired subset.

What I would want:

1. Add a hosted smoke checklist or script that clicks/selects a real bucket and captures the `/v1/public/hub/moments` response.
2. Assert the UI table and inspector use the same backend-enriched row fields.
3. Assert viewer cells, top emotes, provider-aware image URLs, and "breakdown unavailable" copy are consistent between table and inspector.
4. Assert no client-side bucket miss fallback repopulates rows silently when the backend returns empty/unavailable.

Suggested validation:

```bash
cd ../streamclone-pulse/streampulse-web
npm run test:e2e -- tests/e2e/analytics-hub-ux.spec.ts --workers=1
curl -fsS "https://api.streampulse.stream/v1/public/hub/moments?activityWindow=24h&bucketT=<bucket_ms>&limit=10" | jq '{status, reason, count:(.moments|length), first:.moments[0]}'
```

Expected result after fix:

- Hosted bucket click data matches the backend `/hub/moments` response and does not silently substitute local/demo rows.

## Issue P2-016: Public Channel Console May Expose Sync/Backfill Controls Public Users Cannot Use

**Status: fixed (2026-07-06)** — see [P2-016 public console operator controls](#p2-016-public-console-operator-controls-2026-07-06).

Severity: P2

Trust classification: misleading if controls render enabled or optimistic

Surfaces: `/analytics/{login}`, `/analytics/{login}/s/{streamId}`, public portal console, Streamclone local analytics page

Primary files:

- `../streamclone-pulse/streampulse-web/src/lib/streamcloneAnalytics.ts`
- `../streamclone-pulse/streampulse-web/src/routes/analytics/FigmaChannelView.tsx`
- `../streamclone-pulse/streampulse-web/src/ui/components/analytics/FigmaChannelDashboard.tsx`
- `packages/analytics-console/`
- `internal/analytics/portal_analytics_api.go`
- `internal/analytics/pulse_backfill_api.go`

Finding:

The audit covered bookmarks but underweighted sync/backfill affordances. Public portal users cannot run local scraper/operator sync flows, and hosted backfill must follow backend eligibility, auth, and rate limits. If the channel console exposes sync rail controls copied from Streamclone local dev, public users may believe they can repair data they cannot actually repair.

Why this matters:

- Public `/analytics/{login}` is not a streamer-owned admin console.
- "Load missed moments" is valid only when backend approves VOD backfill.
- Manual sync/scraper controls belong to local/operator Streamclone, not public anonymous portal users.

What I would want:

1. Grep/render-audit public channel console for sync/backfill/repair controls.
2. Hide operator-only sync controls on public portal.
3. Show backfill CTAs only from backend coverage state and only when the hosted route can actually accept the request.
4. Add tests that hosted public console does not show local scraper/sync affordances.

Suggested validation:

```bash
cd ../streamclone-pulse/streampulse-web
rg -n "sync|backfill|Load missed|scraper|repair|refresh" src/ui src/routes src/lib tests
npm run test:e2e -- tests/e2e/visual-analytics-smoke.spec.ts --workers=1
```

Expected result after fix:

- Public channel console shows backend job state honestly and does not expose operator-only repair controls.

## Issue P2-017: Identity Model Decision Blocks Watchlist, Bookmarks, Protect, And Clips

Severity: P2

Trust classification: misleading ownership and persistence model

Surfaces: Protect/watchlist, bookmarks, clip queue, login/setup/account, extension vs portal parity

Primary files:

- `internal/analytics/pulse_hosted.go`
- `internal/analytics/pulse_watchlist.go`
- `internal/analytics/bookmarks.go`
- `internal/analytics/clip_candidates_api.go`
- `../streamclone-pulse/streampulse-web/src/lib/apiClient.ts`
- `../streamclone-pulse/streampulse-web/src/routes/public/Login.tsx`
- `../streamclone-pulse/streampulse-web/src/routes/dashboard/`
- `../streamclone-pulse/docs/website-portal/design.md`

Finding:

The audit listed identity as a strategic question, but the spot-check rightly ties it to multiple concrete backlog items. Watchlist, Protect, bookmarks, and clips all require a principal model. The current codebase mixes beta-key, guest/local, future device/account language, and public `/analytics` flows.

Why this matters:

- Users need to know whether state persists per browser, beta key, device, account, or channel owner.
- Streamer-owned CTAs can be dishonest if any viewer can open `/analytics/{login}`.
- Clip queues are private workflows and should not appear as public hub affordances until identity is explicit.

What I would want:

1. Decide the MVP principal story: beta-key, device ID, guest principal, OAuth later, or no user state on public pages.
2. Document which routes require which principal kind.
3. Make UI copy match that model exactly.
4. Add route guards/tests for private features: watchlist, bookmarks, clips, account/dashboard.

Suggested validation:

```bash
cd ../streamclone-pulse
rg -n "principal|beta key|device|guest|OAuth|account|watchlist|bookmark|clips|Protect" docs src streampulse-web/src streampulse-web/tests
```

Expected result after fix:

- Private workflows have one coherent persistence/identity story, and public analytics does not imply ownership.

## Issue P2-018: Extension And Portal Backend Defaults Can Produce Product-Visible Data Divergence

Severity: P2

Trust classification: misleading cross-surface parity

Surfaces: extension Options, portal `/analytics`, hosted API, local `:8090`

Primary files:

- `../streamclone-pulse/src/shared/storage.ts`
- `../streamclone-pulse/src/options/options.tsx`
- `../streamclone-pulse/streampulse-web/src/lib/backendSource.ts`
- `../streamclone-pulse/streampulse-web/src/lib/auth.ts`
- `../streamclone-pulse/streampulse-web/tests/backendSource.test.ts`
- `../streamclone-pulse/README.md`

Finding:

The portal defaults to hosted API by policy. The extension also defaults hosted in current storage, but Options allow local override. The product-level issue is user confusion: "why does my extension show different data than the website?" when one surface points at local `:8090` and the other points at hosted production.

Why this matters:

Contract drift and data drift become visible as product inconsistency. A late-join stream can look partial in one surface and complete in another if the backends differ.

What I would want:

1. Show current backend source clearly in extension Options and debug surfaces.
2. On portal, keep local override explicit and visually marked.
3. Add docs/copy that local mode is for development and can diverge from hosted public analytics.
4. Add tests that production builds do not accidentally embed localhost.

Suggested validation:

```bash
cd ../streamclone-pulse/streampulse-web
npm run check:backend-url
npm test -- tests/backendSource.test.ts
cd ..
npm run typecheck
```

Expected result after fix:

- Users and operators can immediately tell which backend a surface is using.

## Issue P2-019: Hosted Deploy Freshness / API-Version Skew Is Not Surfaced Enough

**Status: fixed (2026-07-06)** — see [P2-013 / P2-019 public status + hosted skew](#p2-013--p2-019-public-status--hosted-skew-2026-07-06).

Severity: P2

Trust classification: operationally misleading

Surfaces: Cloudflare Pages portal, hosted API, status/health, hub route contracts

Primary files:

- `../streamclone-pulse/streampulse-web/scripts/check-backend-url.mjs`
- `../streamclone-pulse/streampulse-web/scripts/hosted-status.mjs`
- `deploy/smoke/test-013b-hosted.sh`
- `scripts/pulse-hosted-boundary-smoke.sh`
- `internal/analytics/extension_api.go`
- `internal/analytics/public_api.go`

Finding:

Cloudflare Pages portal code and hosted API code can be deployed at different times. If the portal expects a new `/v1/public/hub`, coverage, moments, or clip field but API is still on an older RC, the UI may fall back silently or tests may only catch it in manual QA.

Why this matters:

This directly affects the product-truth promise: a healthy-looking portal can be paired with stale or missing API contracts.

What I would want:

1. Expose backend version/build SHA and portal build SHA in health/debug surfaces.
2. Add a hosted contract smoke that probes the exact fields the current portal requires.
3. Make `status:hosted` avoid local env accidental overrides when the intent is production validation.
4. Fail CI/deploy smoke on missing required hub/moments/coverage fields.

Suggested validation:

```bash
cd ../streamclone-pulse/streampulse-web
VITE_BACKEND_URL=https://api.streampulse.stream npm run status:hosted
npm run build
cd ../../twitch-7tv-clone
bash deploy/smoke/test-013b-hosted.sh
bash scripts/pulse-hosted-boundary-smoke.sh
```

Expected result after fix:

- Hosted portal/API skew is visible and contract-breaking skew fails smoke checks.

## Issue P2-020: `/dashboard/clips` Must Not Be Publicly Linked Before Identity And Artifact Semantics Are Clear

**Status: fixed (2026-07-06)** — see [P2-010 / P2-020 / P3-025 / P3-028 / P3-027 launch cleanup](#p2-010--p2-020--p3-025--p3-028--p3-027-launch-cleanup-2026-07-06).

Severity: P2

Trust classification: misleading private workflow boundary

Surfaces: `/dashboard/clips`, public hub/nav, landing CTAs, private clip queue

Primary files:

- `../streamclone-pulse/streampulse-web/src/routes/dashboard/Clips.tsx`
- `../streamclone-pulse/streampulse-web/src/routes/index.tsx`
- `../streamclone-pulse/streampulse-web/src/ui/components/analytics/`
- `../streamclone-pulse/streampulse-web/src/ui/components/landing/`
- `internal/analytics/clip_candidates_api.go`

Finding:

The clip routes are backend-gated, but the public product must not link or market `/dashboard/clips` as a general public feature before identity and durable artifact semantics are settled.

Why this matters:

Clip queues are private editor workflows. If public hub users can discover a clip queue route that requires beta/private principal or cannot produce durable playback, the product appears half-launched.

What I would want:

1. Search public nav, hub, landing, and channel console for links to `/dashboard/clips` or clip queue CTAs.
2. Hide those links unless the current principal can actually use the feature.
3. Label the route as private/operator/beta if it remains reachable.
4. Do not add extension "clip this peak" until durable artifact playback exists.

Decision guidance for the next agent:

- Do **not** delete `/dashboard/clips` solely because it is private. Keep it as a gated workspace route if it is useful for operator/beta review.
- Do remove public navigation, landing CTAs, hub CTAs, and channel-console CTAs that advertise clips as launched public product before identity/artifact rules are ready.

Suggested validation:

```bash
cd ../streamclone-pulse/streampulse-web
rg -n "dashboard/clips|clip queue|clips|ReplayForge|send this peak|clip this" src tests docs
npm run check:analytics-links
```

Expected result after fix:

- Public users do not encounter a private clip workflow as if it were launched product.

## Issue P2-021: Public Minute-Level Analytics Needs Explicit Competitive / ToS Posture

**Status: fixed (2026-07-06)** — decision doc [`public-analytics-posture-2026-07.md`](../streamclone-pulse/docs/agent-notes/public-analytics-posture-2026-07.md).

Severity: P2 product decision, not a direct code bug

Trust classification: launch risk

Surfaces: public `/analytics/{login}`, hub discovery, screenshots/social previews, docs/landing copy

Primary files:

- `../streamclone-pulse/docs/pulse-extension/website-portal-requirements.md`
- `../streamclone-pulse/docs/website-portal/design.md`
- `../streamclone-pulse/streampulse-web/src/routes/public/Landing.tsx`
- `../streamclone-pulse/streampulse-web/src/routes/analytics/AnalyticsLandingPage.tsx`

Finding:

Public minute-level chat/emote analytics for arbitrary Twitch channels is a product/legal/competitive posture question. The code can make the UI honest, but it cannot decide whether the public surface should be coarse, rate-limited, gated for owners, or fully public.

Why this matters:

Streamers, Twitch policy reviewers, or competitors may frame public minute-level analytics as competitive intelligence or scraping at scale. This is bigger than copy polish.

What I would want:

1. Add a launch decision doc section for public analytics granularity.
2. Decide whether non-owner views need coarser aggregation, rate limits, robots/noindex, or delayed data.
3. Ensure landing copy does not promise ownership, private stats, or sanctioned Twitch integration unless true.
4. Keep raw chat/user data out of public responses.

Suggested validation:

```bash
cd ../streamclone-pulse
rg -n "public analytics|minute-level|owner|streamer|private|Twitch|competitive|raw chat|viewer" docs streampulse-web/src
```

Expected result after fix:

- Launch positioning is explicit, and public analytics granularity is intentional.

## Issue P3-022: `analytics-figma-parity.spec.ts` Has Stale Expectations Or The Product Lost A Required Heading

**Status: verified fixed (2026-07-06)** — `analytics-figma-parity.spec.ts` passes 9/9; hub `h1` is **Command center** with `#section-pulse-moments`.

Severity: P3 unless the heading is product-required

Trust classification: test archaeology unless confirmed as required copy

Primary files:

- `../streamclone-pulse/streampulse-web/tests/e2e/analytics-figma-parity.spec.ts`
- `../streamclone-pulse/streampulse-web/src/routes/analytics/AnalyticsLandingPage.tsx`
- `../streamclone-pulse/streampulse-web/src/ui/components/analytics/PulseMomentsLivePanel.tsx`

Finding:

The focused Playwright run failed three viewport variants because the test expects a heading matching `/Pulse Moments Live/i`. The current `/analytics` page did render the main hub and channel/session routes, but this specific heading was not found.

What I would want:

1. Decide whether "Pulse Moments Live" is still required user-facing copy.
2. If required, restore the heading/accessibility label.
3. If not required, update the test to assert the current canonical moments section.
4. Keep overflow checks; those are still useful.

Suggested validation:

```bash
cd ../streamclone-pulse/streampulse-web
npm run test:e2e -- tests/e2e/analytics-figma-parity.spec.ts --workers=1
```

Expected result after fix:

- The parity test verifies current product truth rather than stale Figma copy.

## Issue P3-023: React `fetchPriority` Warning Breaks Console-Error Guard

Severity: P3

Trust classification: acceptable polish, but blocks clean e2e signal

Primary files:

- `../streamclone-pulse/streampulse-web/src/ui/components/analytics/ActivityBucketInspector.tsx`
- `../streamclone-pulse/streampulse-web/tests/e2e/analytics-hub-metrics-honesty.spec.ts`

Finding:

Two Playwright tests failed because React warned:

`React does not recognize the fetchPriority prop on a DOM element... spell it as lowercase fetchpriority instead...`

This warning came from an `img` rendered under `InspectorStreamersFooter` in `ActivityBucketInspector.tsx`.

Why this matters:

The hub honesty test is otherwise valuable. Console warnings make it noisy and can hide real regressions.

What I would want:

1. Use the correct React-supported prop for this React version, or remove the prop.
2. Rerun the hub honesty spec.

Suggested validation:

```bash
cd ../streamclone-pulse/streampulse-web
npm run test:e2e -- tests/e2e/analytics-hub-metrics-honesty.spec.ts --workers=1
```

Expected result after fix:

- Hub honesty tests fail only on product/data regressions, not React warning noise.

## Issue P3-024: Dual Hub / Dashboard Implementations Create IA And Visual Drift Risk

**Status: quarantined (2026-07-06)** — minimal private `dashboard/Home.tsx`; canonical public hub is `/analytics`; no public clip links. See [Final audit closeout](#final-audit-closeout-2026-07-06).

Severity: P3, but treat as P2 if `dashboard/Home.tsx` is reachable, linked, or still product-facing

Trust classification: misleading IA

Surfaces: `/analytics`, `/dashboard`, `/analytics/streams`, old hub components

Primary files:

- `../streamclone-pulse/streampulse-web/src/routes/analytics/AnalyticsLandingPage.tsx`
- `../streamclone-pulse/streampulse-web/src/routes/dashboard/Home.tsx`
- `../streamclone-pulse/streampulse-web/src/routes/analytics/StreamsHubPlaceholder.tsx`
- `../streamclone-pulse/streampulse-web/src/routes/index.tsx`
- `../streamclone-pulse/streampulse-web/src/main.tsx`
- `../streamclone-pulse/streampulse-web/src/ui/components/hub/hub.css`
- `../streamclone-pulse/streampulse-web/src/ui/components/analytics/figma-analytics.css`

Finding:

The current product memory says `/analytics` is the search-first public hub / door to `/analytics/{login}`. But `dashboard/Home.tsx` still exists with overlapping hub-like panels and demo/static channel shortcuts. `/analytics/streams` redirects or placeholders still exist.

Why this matters:

Two hub implementations can create contradictory rankings, labels, fallbacks, design tokens, and test expectations.

What I would want:

1. Decide canonical route ownership:
   - Public analytics hub: `/analytics`.
   - Channel console: `/analytics/{login}`.
   - Session console: `/analytics/{login}/s/{streamId}`.
   - Private dashboard/clip queue: only if identity story is real.
2. Tombstone or quarantine old dashboard hub code if it is design lab only.
3. Make `/analytics/streams` either a clear redirect or a real route; do not imply a separate directory product.
4. Remove or isolate `.hubx` embeds when they visually or behaviorally drift from the canonical analytics design system.

Decision guidance for the next agent:

- Prefer one public hub: `/analytics`.
- Keep private dashboard routes only when they are principal-gated and named/copy-scoped as private workspace, not public StreamPulse discovery.
- If `/dashboard/Home.tsx` is reachable only as a design lab, remove public links and label or quarantine it rather than merging its UX back into `/analytics`.

Suggested validation:

```bash
cd ../streamclone-pulse/streampulse-web
npm run check:analytics-routes-spa
npm run check:analytics-links
npm run typecheck
```

Expected result after fix:

- There is one public analytics hub, not two competing products.

## Issue P3-025: Demo / Fixture Surfaces Need A Hard Boundary From Production Analytics

**Status: fixed (2026-07-06)** — see [P2-010 / P2-020 / P3-025 / P3-028 / P3-027 launch cleanup](#p2-010--p2-020--p3-025--p3-028--p3-027-launch-cleanup-2026-07-06).

Severity: P3, can become P1 if demo data appears in production analytics routes

Trust classification: misleading if unlabeled

Surfaces: landing demos, Figma session demo, hub featured fallback

Primary files:

- `../streamclone-pulse/streampulse-web/src/lib/figmaMakeDemo.ts`
- `../streamclone-pulse/streampulse-web/src/ui/components/analytics/FigmaSessionHeaderStrip.tsx`
- `../streamclone-pulse/streampulse-web/src/ui/components/landing/LiveSignalScrollGraph.tsx`
- `../streamclone-pulse/streampulse-web/src/ui/components/landing/TrackedChannels.tsx`
- `../streamclone-pulse/streampulse-web/src/routes/public/Landing.tsx`

Finding:

`figmaMakeDemo.ts` defines a deterministic xQc demo session. It appears unreferenced in the current source search, but several landing/demo surfaces still use xQc-style fixture visuals.

Marketing demos are acceptable when clearly illustrative. They are not acceptable inside analytics routes unless unmistakably labeled.

What I would want:

1. Delete `figmaMakeDemo.ts` if truly unreferenced.
2. Ensure any fixture/demo model includes `demo: true` or equivalent.
3. Ensure production analytics routes never fall back to fake xQc data.
4. Landing visuals should use copy that makes them clearly illustrative, not live stats.

Suggested validation:

```bash
cd ../streamclone-pulse/streampulse-web
rg -n "figmaMakeDemo|buildDemoSessionViewModel|demo-stream-xqc|preview layout|xQc" src tests
npm run test:e2e -- tests/e2e/analytics-visual-capture.spec.ts --workers=1
```

Expected result after fix:

- Demo data is either gone from analytics routes or unmistakably labeled.

## Issue P3-026: Account / Beta-Key / Dashboard Mental Model Is Half-Removed

**Status: fixed (2026-07-06)** — see [P2-012 / P3-026 bookmark + account copy](#p2-012--p3-026-bookmark--account-copy-2026-07-06).

Severity: P3, but treat as P2 for launch if public CTAs/docs still say "Sign in" or beta-key setup is presented as required for public `/analytics`

Trust classification: misleading IA

Surfaces: landing, login, setup, dashboard, docs, tests

Primary files:

- `../streamclone-pulse/streampulse-web/src/routes/public/Login.tsx`
- `../streamclone-pulse/streampulse-web/src/routes/public/Setup.tsx`
- `../streamclone-pulse/streampulse-web/src/routes/dashboard/`
- `../streamclone-pulse/streampulse-web/src/lib/apiClient.ts`
- `../streamclone-pulse/docs/pulse-extension/website-portal-requirements.md`
- `../streamclone-pulse/docs/pulse-extension/requirements.md`

Finding:

The product is now largely public `/analytics`, but beta-key/account/dashboard copy and routes still exist. Some private state still legitimately needs a principal model: watchlist, bookmarks, clips. The issue is not that identity exists; the issue is that the user-facing story is unclear.

What I would want:

1. Separate public analytics from private workspace features in route names and copy.
2. Do not label `/analytics/{login}` as "your analytics" unless the user owns/verifies that channel.
3. Keep beta-key language only where beta-key auth actually exists.
4. Delete stale account shell copy or reintroduce a real account/device identity story.

Suggested validation:

```bash
cd ../streamclone-pulse
rg -n "beta key|beta-key|account|Sign in|your analytics|dashboard|device token" README.md docs src streampulse-web/src streampulse-web/tests
```

Expected result after fix:

- Public pages read as public analytics; private pages read as private tools.

## Issue P3-027: Design-System Drift Remains Between Figma Analytics, HubX, And Token Rules

**Status: fixed (2026-07-06)** — classified + minimal active fix; see [P2-010 / P2-020 / P3-025 / P3-028 / P3-027 launch cleanup](#p2-010--p2-020--p3-025--p3-028--p3-027-launch-cleanup-2026-07-06).

Severity: P3

Trust classification: acceptable polish, but increases regression risk

Surfaces: portal analytics hub and console

Primary files:

- `../streamclone-pulse/streampulse-web/src/ui/components/analytics/figma-analytics.css`
- `../streamclone-pulse/streampulse-web/src/ui/components/hub/hub.css`
- `../streamclone-pulse/streampulse-web/src/ui/themes/analytics-surfaces.css`
- `../streamclone-pulse/streampulse-web/src/ui/analytics-tailwind.css`

Finding:

The project has explicit token guidance in `analytics-surfaces.css`, but searches still show many legacy `rgba(255, 255, 255, ...)` surfaces and multiple design layers. Some are legitimate tokens; others are old component styles or mockups.

What I would want:

1. Classify token violations into:
   - intentional token definitions
   - mockups / dead files
   - active component surfaces needing migration
2. Avoid doing this as a drive-by refactor during truth fixes.
3. Keep semantic tables where data is tabular; avoid CSS-grid pseudo tables for analytics rows unless accessibility is preserved.

Suggested validation:

```bash
cd ../streamclone-pulse/streampulse-web
npm run check:analytics-tailwind
rg -n "rgba\(255,\s*255,\s*255|display:\s*grid|role=\"table\"|<table" src/ui/components src/ui/themes
```

Expected result after fix:

- Active analytics surfaces use canonical tokens and accessible table semantics.

## Issue P3-028: Public Provider Breakdown Must Not Outrun Backend Rollup Persistence

**Status: fixed (2026-07-06)** — see [P2-010 / P2-020 / P3-025 / P3-028 / P3-027 launch cleanup](#p2-010--p2-020--p3-025--p3-028--p3-027-launch-cleanup-2026-07-06).

Severity: P3, can become P1 if UI claims unavailable provider truth

Trust classification: misleading

Surfaces: hub emote economy, provider lines, console tables

Primary files:

- `migrations/000048_emote_history_phase1a.up.sql`
- `migrations/000051_public_emote_provider_hourly_rollups.up.sql`
- `internal/analytics/*emote*`
- `../streamclone-pulse/streampulse-web/src/lib/publicHub.ts`
- `../streamclone-pulse/streampulse-web/src/ui/components/analytics/`

Finding:

Provider-aware emote rendering has improved, and public provider hourly rollups exist. The risk is UI showing BTTV/FFZ/Twitch provider breakdown where live rollup persistence only supports partial provider truth.

What I would want:

1. Audit every provider line/chart for its exact backend source.
2. If provider data is unavailable, show "provider breakdown unavailable" rather than `--` or inferred shares.
3. Keep per-provider enrichment server-side or through explicit sanitized aggregate endpoints.

Suggested validation:

```bash
go test ./internal/analytics/... -run 'Emote|Provider|Hub'
cd ../streamclone-pulse/streampulse-web && npm test -- tests/*emote*.test.ts tests/*hub*.test.ts
```

Expected result after fix:

- Provider charts never imply BTTV/FFZ/Twitch splits that backend did not persist.

## Non-Findings / Corrections From Audit

These are important so a follow-up agent does not chase stale reports.

1. `/v1/pulse/clips` is mounted.
   - Initial subagent report said the route was absent. Direct read showed `PulseRoutes` calls `h.registerPulseClipRoutes(r)`.
   - The actual clipper issue is artifact/readiness semantics, not route absence.

2. Extension full timeline does not blindly densify from 00:00 in the current `prepareChartRollups` path when `coverageStartOffsetSeconds` is provided.
   - The remaining issue is regression coverage and full propagation of backend coverage truth.

3. Content script no-fetch rule mostly holds for production API calls.
   - API fetches are in background scripts.
   - Content-side dev reload uses local fetch for development reload behavior; classify separately from product API fetches.

4. Hub metrics honesty tests are valuable.
   - Original audit failures included stale heading expectation and React warning noise, not direct evidence that KPI honesty copy was broken. Later validation reports the honesty spec is clean; keep the parity heading drift separate.

## Final audit closeout (2026-07-06)

**Decision: no audit blockers remain for merge/PR.** All numbered issues in this file are closed with validation evidence. Do not reopen unless a fresh failure traces to the closed diff.

### Final validation summary

| Command | Exit | Result |
|---------|------|--------|
| `streampulse-web` `npm run check:analytics-links` | 0 | pass — href helpers + forbidden public clip-link scan |
| `streampulse-web` `npm run check:analytics-tailwind` | 0 | pass |
| `streampulse-web` `npm run typecheck` | 0 | pass |
| `streampulse-web` `npm run test:e2e -- analytics-hub-metrics-honesty.spec.ts --workers=1` | 0 | 8/8 pass |
| `streampulse-web` `npm run test:e2e -- analytics-visual-capture.spec.ts --workers=1` | 0 | 7/7 pass |
| `streampulse-web` `npm run test:e2e -- analytics-figma-parity.spec.ts --workers=1` | 0 | 9/9 pass |
| `streampulse-web` `npm run test:e2e -- visual-analytics-smoke.spec.ts --workers=1` | 0 | 3/3 pass — no public Sync/Re-sync |
| `streampulse-web` `npm test -- tests/landing.test.tsx tests/portalBookmarks.test.ts tests/publicHub.contract.test.ts` | 0 | pass |
| `streamclone-pulse` `npm run typecheck && npm test && npm run build` | 0 | **399/399** pass; build pass |
| `go test ./internal/analytics/... -run 'Pulse\|Coverage\|Watchlist\|AlwaysTrack\|Portal\|Hub\|Clip\|ReplayForge\|Contract'` | 0 | pass |
| `npm test --prefix packages/pulse-core` | 0 | 43/43 pass |
| `bash deploy/smoke/test-013b-hosted.sh` | 0 | PASS |
| `bash scripts/pulse-hosted-boundary-smoke.sh` | 0 | PASS |
| `git diff --check` (streamclone + streamclone-pulse tracked diffs) | 0 | pass after whitespace hygiene |
| Secret grep on changed tracked files | — | **no obvious tokens** in diff scope |

### Known unchanged debt (not audit blockers)

Do **not** reopen closed issues for these unless a regression is introduced by new diffs:

| Item | Notes |
|------|--------|
| `streampulse-web/tests/analyticsLandingPage.test.tsx` | Full vitest run can OOM/hang (~50 min observed). **E2E authority:** `analytics-hub-metrics-honesty.spec.ts`. |
| `streampulse-web/tests/analyticsConsoleVodLink.test.ts` | Pre-existing unit drift (`resolveAnalyticsVodId` recap fallback). |
| `streampulse-web/tests/hubTopEmotesTable.test.tsx` | Pre-existing unit drift (provider pill label casing). |
| `streampulse-web/tests/analyticsHubEmpty.test.tsx` | Pre-existing unit drift (dashboard home heading selectors). |

**Recommended CI posture:** run authority e2e specs + scoped unit tests; exclude `analyticsLandingPage.test.tsx` from default vitest until render memory is isolated.

### Remaining work (future roadmap — not audit backlog)

- Product decisions in [Strategic Product Calls](#strategic-product-calls-needed) (identity model, public granularity, clip productization).
- Chrome Web Store listing URL (placeholder in extension install links).
- Optional: fix the four unit-test drifts above; isolate `analyticsLandingPage` render fixture.
- Hosted ops: deploy pinned `IMAGE_TAG` per `docs/production-artifact-contract.md` (operator lane).

## Suggested Implementation Order

**All audit issues closed (2026-07-06).** Remaining work is **known test hygiene** and **future roadmap** — not this ledger.

1. **Do not reopen** numbered issues unless fresh validation fails and traces to the closed diff.
2. **Pre-merge:** use [Smallest Test Suite I Would Keep As Authority](#smallest-test-suite-i-would-keep-as-authority) below; exclude `analyticsLandingPage.test.tsx` from full vitest.
3. **Commit/PR:** group by backend contracts, extension honesty, portal public analytics, docs/skills/closeout (see agent handoff or PR prep notes).
4. **Post-merge:** address strategic product calls and unit-test hygiene on a separate track.

## Smallest Test Suite I Would Keep As Authority

Portal:

```bash
cd ../streamclone-pulse/streampulse-web
npm run typecheck
npm run test:e2e -- tests/e2e/analytics-hub-metrics-honesty.spec.ts --workers=1
npm run test:e2e -- tests/e2e/analytics-visual-capture.spec.ts --workers=1
npm run test:e2e -- tests/e2e/analytics-figma-parity.spec.ts --workers=1
```

Extension:

```bash
cd ../streamclone-pulse
npm run typecheck
npm test
npm run build
```

Backend:

```bash
cd c:/Users/Aron/twitch-7tv-clone
go test ./internal/analytics/... -run 'Pulse|Coverage|Watchlist|AlwaysTrack|Portal|Hub|Clip|ReplayForge'
npm test --prefix packages/pulse-core
```

Hosted smoke:

```bash
cd c:/Users/Aron/twitch-7tv-clone
bash deploy/smoke/test-013b-hosted.sh
bash scripts/pulse-hosted-boundary-smoke.sh
```

Run hosted checks carefully: scripts that read local env files can accidentally probe localhost if `VITE_BACKEND_URL` or local env overrides are present.

## Strategic Product Calls Needed

These are not code bugs, but the implementation agent should not invent answers silently.

1. Is `/analytics` the only public hub? If yes, delete/quarantine dashboard hub duplicates.
2. Is `/dashboard` a private product, a design lab, or dead code?
3. Does `Protect` require real identity/account later, or is beta-key/device principal enough for MVP?
4. Are public minute-level analytics acceptable for arbitrary channels, or should some non-owner views be coarser?
5. Is hosted auto-clipper a user product or an operator tool until durable artifacts exist?
6. Should extension "clip this peak" be built? My answer: yes only after private durable artifact playback exists.
