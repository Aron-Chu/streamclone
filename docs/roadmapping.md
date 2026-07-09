# Streamclone Pulse — Product roadmap (phased)

| | |
|---|---|
| **Status** | Draft v1 |
| **Requirements** | sibling [`live-coverage-requirements.md`](../../streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md) |
| **Sources** | [`tools.md`](tools.md) |
| **Architecture** | [`finalplan.md`](finalplan.md) (Phases A–D verified) |

Keep **requirements** (what) separate from this doc (when). Do not mix Kick/cross-platform into R0–R1 MVP.

---

## Product line

```text
Live-first capture for protected channels
+ VOD backfill only when Twitch replay exists
+ cross-platform provider model later (Kick)
```

---

## Phase summary

| Phase | Focus | Outcome |
|-------|--------|---------|
| **R0** | Twitch reliability | Honest coverage UX; BearHost deploy; VOD state finalization |
| **R1** | Protected live-first | Go-live → IRC within 30–120s; `trackedFromStart` |
| **R2** | Twitch context | Clips, title/category, polls — not coverage-critical |
| **R3** | VOD backfill hardening | Retries, prefix-only jobs, admin chat JSON import |
| **R4** | Provider abstraction | `StreamProvider` interface before Kick |
| **R5** | Kick live beta | Live chat + rollups; **no VOD backfill promise** |
| **R6** | Cross-platform portal | Same coverage badges per platform |

---

## R0 — Make Twitch reliable (urgent)

```text
BearHost redeploy (helixEnabled, vod-hint, version != dev)
coverage.state always on pulse payload
waiting_for_vod timeout → vod_unavailable
Protect CTA → always-track backend
Extension: backend-stale warning when Helix missing
```

Fixes late-join confusion (e.g. partial coverage at T+15m with blocked backfill).

---

## R1 — Protected live-first capture

```text
Helix poll MVP for always-track roster (60–120s)
EventSub stream.online (preferred when webhook ready)
IRC join within 30–120s of go-live
trackedFromStart metric
Raise cap: MAX_CONCURRENT_TRACKED_CHANNELS 10 → 25–50 (after metrics)
```

**Not built yet:** dedicated go-live worker in analytics (EventSub today is emote/7TV only).

Env reference: private hosted env overlays in **streampulse-ops** (not this repo).

---

## R2 — Twitch context enrichment

Optional signals (parallel, non-blocking):

```text
category/title timeline
clips around spike windows
poll/prediction/hype-train events
schedule pre-warm
7TV emote set change markers
```

Does **not** fill missing chat — improves “why did chat spike?”

---

## R3 — VOD backfill hardening

```text
VOD id resolution retry schedule (live + post-end: 30s/2m/5m/15m)
vodStatus finalization policy
prefix-only backfill (cheaper than full stream)
moment-window-only backfill
admin import of streamer-supplied chat JSON → BuildMinuteRollupsFromComments
```

---

## R4–R6 — Cross-platform (later)

Sketch `StreamProvider` when starting R5 — not before R1 ships on Twitch.

Kick MVP: live status, go-live, live chat, rollups, coverage states, Protect.

Kick **not** in MVP: deleted VOD recovery, full historical chat, unofficial Pusher as core dep.

---

## Open questions

| # | Question | Default recommendation |
|---|----------|------------------------|
| OQ-1 | Helix poll vs EventSub for R1? | Poll first; EventSub when webhook route ready |
| OQ-2 | Beta live cap 25 vs 50? | 25 on hosted beta, raise after metrics |
| OQ-3 | VOD retry after stream end? | 30s / 2m / 5m / 15m; evaluate +1h |
| OQ-4 | Protected channels per principal? | Extension channel cap per user (hosted config) |
| OQ-5 | Extension VOD discovery? | Hint-only |
| OQ-6 | Shared CoverageCard in shared UI package? | Yes, reduces portal/extension drift |
| OQ-7 | Finalize `vod_unavailable` when? | After N failed Helix checks + post-end window |
| OQ-8 | Provider abstraction before Kick? | Sketch at R4, implement after R1 |

---

## Cross-links

```text
streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md  → canonical (pulse repo)
docs/streampulse-product-boundary.md                                  → public boundary stub
AGENTS.md                                                             → task router
docs/CODEX.md                                                         → Codex setup
```

---

## Revision history

| Date | Change |
|------|--------|
| 2026-06-23 | Structured roadmap (split from tools/notes) |
