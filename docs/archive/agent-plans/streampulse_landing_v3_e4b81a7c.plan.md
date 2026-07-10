---
name: StreamPulse landing v3
overview: Redesign streampulse-web landing (v3 mockup + Warp patterns). Phase 0 requirements sync required before code. Includes MomentPlaybackPreview, HighestActivitySection, portal-framed tracking, expanded fixtures, no TrustedByStrip. CSS-only stack; no backend changes.
todos:
  - id: phase-0-requirements
    content: "Phase 0: Update streampulse-landing-requirements.md for v3 section order and new acceptance criteria"
    status: pending
  - id: phase-a-foundation
    content: "Phase A: landingDemo.ts (expanded schema) + landing.css tokens + ProductShellFrame"
    status: pending
  - id: phase-b-nav-hero
    content: "Phase B: TopNav + HeroProductShell + capability chips (no TrustedByStrip)"
    status: pending
  - id: phase-c-scale-portal
    content: "Phase C: PlatformScaleSection + PortalShellPreview + LiveAnalyticsPortalSection"
    status: pending
  - id: phase-d-playback
    content: "Phase D: MomentPlaybackPreview (watch moment / replay context)"
    status: pending
  - id: phase-e-tracking-activity
    content: "Phase E: LiveTrackingSection (portal columns) + HighestActivitySection + narrative bridge"
    status: pending
  - id: phase-f-api-faq-footer
    content: "Phase F: ApiIntegrationsSection + FAQ + FinalCta + Footer + Landing.tsx compose"
    status: pending
  - id: phase-g-tests-visual
    content: "Phase G: Tests + typecheck/build + capture:landing"
    status: pending
  - id: phase-h-audit-report
    content: "Phase H: Final audit report (placeholders, limitations, screenshots)"
    status: pending
isProject: false
---

# StreamPulse landing v3 redesign

**Canonical plan doc:** [`streamclone-pulse/docs/design/streampulse-landing-v3.md`](streamclone-pulse/docs/design/streampulse-landing-v3.md)

**Status:** Plan patched pre-implementation. **Do not implement production code until Phase 0 completes.**

## v3 section order

1. TopNav
2. HeroProductShell (+ capability chips, not trust logos)
3. PlatformScaleSection
4. LiveAnalyticsPortalSection
5. **MomentPlaybackPreview**
6. LiveTrackingSection (portal snapshot columns)
7. **HighestActivitySection** (moments/windows, anonymized — not HighestChatter)
8. ApiIntegrationsSection
9. FAQ
10. FinalCtaSection
11. Footer

## Key plan patches (2026-06-24)

- **Phase 0** — sync `streampulse-landing-requirements.md` before code
- **Playback** — first-class `MomentPlaybackPreview`
- **Activity** — `HighestActivitySection`; no user surveillance framing
- **Tracking** — portal product columns (Channel, Status, Coverage, Latest spike, Top emote, Last updated, Action)
- **TrustedByStrip** — **deferred**; use hero capability chips
- **Fixtures** — expanded `landingDemo.ts` schema (see canonical doc)
- **API honesty** — Example API shape badge when aspirational; docs link non-404
- **Synthetic safety** — fictional channels, no Twitch branding misuse
- **Visual density** — one large shell per viewport; no adjacent dense tables without bridge

## Implementation sequence

Phase 0 → A (fixture/CSS) → B (nav/hero) → C (scale/portal) → D (playback) → E (tracking/activity) → F (API/FAQ/footer) → G (tests/visual) → H (audit report)

See full audit, component list, tests, and risks in the canonical markdown file.
