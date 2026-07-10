---
description: Clip Studio / ReplayForge work is out of scope for Streamclone. Route to sibling replayforge.
---

# Clipper Local — Route to replayforge

> **Boundary lock.** Streamclone no longer ships Clip Studio, ReplayForge deeplinks, or an in-repo clipper worker.

**Do not implement clipper/Clip Studio changes in this repo.** Route to:

| Need | Repo |
|------|------|
| Clip Studio UI / worker / exports | sibling **replayforge** |
| Analytics moment → clip triggers | private **streampulse-backend** / **streamclone-pulse** |

See [`docs/streampulse-product-boundary.md`](../../../../docs/streampulse-product-boundary.md).
