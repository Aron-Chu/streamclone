# ADR stub: Admin corpus visibility surface

**Status:** Proposed — decision pending
**Date:** 2026-07-01
**Context:** [`docs/requirements/corpus-scaling-observability.md`](../requirements/corpus-scaling-observability.md) PR 4 mentions internal corpus detail APIs and Grafana; admin UI ADR required before implementation.

---

## Question

Where should operators view corpus segment grids, gap lists, requeue actions, and worker lease health?

| Option | Description |
|--------|-------------|
| **A. Grafana only** | Extend `deploy/grafana/dashboards/streamclone-ops.json` |
| **B. StreamPulse web internal route** | e.g. `/analytics/internal/corpus` (auth-gated) |
| **C. Streamclone backend frontend** | Existing compose `frontend` admin panels |
| **D. Separate internal dashboard** | New lightweight ops app |

---

## Pros / cons (sketch)

### A. Grafana only

- **Pros:** Metrics already exist; no new public attack surface; fits ops workflows.
- **Cons:** Poor for per-segment requeue UX; SQL panels need careful readonly boundaries.

### B. StreamPulse web internal route

- **Pros:** Same auth stack as portal; aligns with analytics hub brand.
- **Cons:** Must not leak into `/v1/public/*`; Cloudflare Pages deploy separate from workers.

### C. Streamclone backend frontend

- **Pros:** Co-located with BFF; fast iteration for internal APIs.
- **Cons:** Not where most StreamPulse operators look today; duplicate UX vs portal.

### D. Separate dashboard

- **Pros:** Isolated blast radius.
- **Cons:** Highest maintenance; auth SSO wiring.

---

## Security / privacy

- **Never** expose on public hub: segment rows, `lease_owner`, logins, stream IDs, job errors, raw chat.
- Internal APIs: operational metadata only (VOD id, offsets, status, sanitized error class).
- Requeue/write actions: same auth as existing internal analytics routes — **not** hosted MCP.

---

## Recommendation (placeholder)

**TBD after PR 0B–2 soak.** Interim: Grafana + hosted MCP read-only queries (`corpus-0b2-hosted-verify.md`) until PR 3 internal APIs exist.

**Do not implement UI in PR 0B-2 or 0B-3.**
