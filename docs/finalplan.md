# Streamclone decoupling notes — HISTORICAL

| | |
|---|---|
| **Status** | HISTORICAL (rewritten stub 2026-07) |
| **Original goal** | Live-first archive phases A–D + Bronze inside the Streamclone monorepo |
| **Current reality** | Analytics ingest, archive workers, and hosted Pulse stack live in **streampulse-backend** / **streampulse-ops**. Public Streamclone is watch-core only. |

## Do not use this file for

- Claiming Streamclone owns analytics APIs, rollups, hub, IRC collectors, or VOD chat gold
- Deploying BearHost / hosted Pulse profiles from this repo
- Guiding extension or portal work (use `streamclone-pulse`)
- Guiding BFF or migration work (use `streampulse-backend`)

## Current principles (watch stack)

1. Core Watch must work without StreamPulse extension surfaces, ReplayForge, or Grafana.
2. Do not add Pulse BFF routes, portal pages, or collector admission here.
3. Legacy `/v1/clipper` and `/studio` adapters shrink only — no new features.
4. Product ownership: [`docs/streampulse-product-boundary.md`](streampulse-product-boundary.md).

## Where the old plan content went

- Agent plans: [`docs/archive/agent-plans/`](archive/agent-plans/)
- Multi-repo routing: [`../streampulse-sdlc/AGENTS.md`](../../streampulse-sdlc/AGENTS.md) (private control plane)
- Archive / gold / silver worker code: **streampulse-backend** (not this public tree)

If you need archaeology of Phases A–D wording, recover from git history before this stub rewrite — do not reintroduce it as active guidance.
