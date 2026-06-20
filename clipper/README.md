# Clipper (legacy in-repo)

Clip Studio / clipper has moved to **ReplayForge** as a standalone sibling repo (`../../replayforge`).

- **ReplayForge API:** `http://localhost:8095` (`/healthz`, `/v1/jobs`, …)
- **ReplayForge UI:** `http://localhost:8096` (`/studio`, `/studio/:jobId`)

Streamclone still calls the clipper API for moment triggers and redirects `/studio` routes to ReplayForge UI when `VITE_REPLAYFORGE_UI_URL` is set.

This directory remains for transitional compatibility and `make clipper-test` fallback. Do not add new features here.

See [INTEGRATION.md](../../replayforge/docs/INTEGRATION.md) in the ReplayForge checkout for env vars and API contract.

For agents: [docs/agents-streamclone-and-replayforge.md](../docs/agents-streamclone-and-replayforge.md).
