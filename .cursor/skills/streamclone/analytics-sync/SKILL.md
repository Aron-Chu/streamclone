---
description: Analytics sync work is now routed to streampulse-backend. Do not implement analytics API or rollup changes in this repo.
---

# Analytics Sync — Route to streampulse-backend

> **Boundary split.** Analytics sync, rollups, VOD backfill, and Pulse rollup export were moved out of this repository during the StreamPulse boundary split.

**Do not implement analytics changes in this repo.** Route to:

| Need | Repo |
|------|------|
| Analytics API, rollups, ingest, hub | private **streampulse-backend** (`internal/analytics`, BFF routes) |
| Portal UI | public **streamclone-pulse** (`streampulse-web/src/`) |
| Deploy / secrets | private **streampulse-ops** |

See [`docs/streampulse-product-boundary.md`](../../../../docs/streampulse-product-boundary.md) for the canonical boundary.

If you are debugging local watch charts (optional Influx/Grafana layer), see `.kiro/steering/analytics.md` in this repo.
