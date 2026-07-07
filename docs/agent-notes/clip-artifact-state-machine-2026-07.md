# Clip artifact state machine (2026-07)

Defines honest clip status semantics before marketing hosted clips or extension "clip this peak".

## Problem

`ReplayForge ready` and `artifact_available` can be true while no durable hosted object exists. Portal must not label clips playable until private signed playback is verified.

## Desired states

```text
canonical candidate
  -> per-principal: new | saved | dismissed
  -> ReplayForge job: queued | rendering | failed | source_unavailable
  -> artifact mirror: missing | uploading | durable_ready | expired | deleted
  -> portal playback: signed_url_issued | signed_url_expired
```

## Rules

- **Worker ready ≠ portal playable.** UI label: "Worker ready (playback not verified)" until `durable_ready`.
- **`sourceStatus`** on candidates is first-class: `available`, `missing`, `restricted`, `unknown`.
- Portal JSON must never expose raw filesystem paths, unsigned URLs, or callback tokens.

## MVP UI (portal)

- [`clipCandidates.ts`](../../streamclone-pulse/streampulse-web/src/lib/clipCandidates.ts): `clipJobDisplayStatus()` maps job status to honest labels.
- [`Clips.tsx`](../../streamclone-pulse/streampulse-web/src/routes/dashboard/Clips.tsx): private queue only — gated route.

## Out of scope (this pass)

- R2 upload drill, Postgres artifact metadata migration, signed URL playback endpoint.

## Pre-launch drill (operator)

1. Upload fixture render to durable storage (R2).
2. Store sanitized metadata in Postgres.
3. Serve signed private read URL.
4. Delete worker-local copy; confirm portal playback still works.
