# Production promotion contract (StreamPulse hosted)

Public contract for **how StreamPulse production consumes backend images**. Application **source** and CI builds remain in [`production-artifact-contract.md`](production-artifact-contract.md). Private deploy execution lives in **streampulse-ops** (never in public workspaces).

**Status:** `pre-cutover` — production manifests may still reference `ghcr.io/aron-chu/streamclone/*` until private ops evidence and digest promotion complete. Do not claim cutover success from public docs alone.

**Active migration spec:** sibling [streamclone-image-exit-audit-2026-07.md](../../streamclone-pulse/docs/pulse-extension/evidence/streamclone-image-exit-audit-2026-07.md)

**Launch hardening (interim):** [production-artifact-decision-2026-07.md](../../streamclone-pulse/docs/pulse-extension/evidence/production-artifact-decision-2026-07.md)

No host IPs or operator secrets belong in this document.

---

## Roles

| Layer | Owner | Responsibility |
|-------|-------|----------------|
| Backend source | `Aron-Chu/streamclone` | Go APIs, BFF, workers, migrations, `pulse-core`, CI |
| Source images | `ghcr.io/aron-chu/streamclone/*` | Built on tag push; see source-build contract |
| Production promotion | private **streampulse-ops** | Pin tags, promote digests, compose, secrets, smoke, rollback |
| Production images (target) | `ghcr.io/aron-chu/streampulse/*` | Digest-promoted from streamclone source images |
| Client/product | `streamclone-pulse` | Extension, portal, product docs — not backend image builds |

---

## Promotion invariant

For every hosted production deploy:

```text
api image digest == workers image digest (same analytics source)
migrate image tag/digest == compatible source revision with api/workers
SOURCE_SHA documents the streamclone git commit that built the source images
```

Promote by **digest**, not by rebuilding from different source. Rollback across a digest-identical promotion is a compose/image reference change only — no schema down-migration unless a separate migration ran.

Scraper may use a separate **`SCRAPER_IMAGE_TAG`** when built from `streamclone-scraper`. Document exceptions in the private deployment manifest.

---

## Target image map (post-cutover)

| Runtime role | Production image (target) | Source digest of |
|--------------|---------------------------|------------------|
| API / BFF | `ghcr.io/aron-chu/streampulse/api:${IMAGE_TAG}` | `streamclone/analytics:${IMAGE_TAG}` |
| Workers | `ghcr.io/aron-chu/streampulse/workers:${IMAGE_TAG}` | same as API |
| Migrations | `ghcr.io/aron-chu/streampulse/migrate:${IMAGE_TAG}` | `streamclone/migrate:${IMAGE_TAG}` |
| Metadata | `ghcr.io/aron-chu/streampulse/metadata:${IMAGE_TAG}` | if still required after runtime shrink |
| Emotes | `ghcr.io/aron-chu/streampulse/emote:${IMAGE_TAG}` | if still required after runtime shrink |

Pre-cutover: private compose may still list `streamclone/*` names. Reconcile running digests on VPS before renaming.

---

## Required private manifest fields

Copy template: [`docs/ops/promotion-manifest.template.md`](ops/promotion-manifest.template.md)

Every promotion in `streampulse-ops/docs/deployments/` must record:

| Field | Purpose |
|-------|---------|
| `IMAGE_TAG` | Immutable release identity |
| `SOURCE_SHA` | Streamclone git SHA for the tagged build |
| Per-service **source** image + digest | `streamclone/*` build artifacts |
| Per-service **production** image + digest | `streampulse/*` after promotion (or `streamclone/*` pre-cutover) |
| `MIGRATE_IMAGE` / digest | Must match API source revision |
| `ROLLBACK_TAG` + rollback digests | Known-good previous promotion |
| `SMOKE_RESULTS` | Health, hub, boundary, internal ops probes |
| Caps / kill switches | Launch posture evidence |

Phase 0 VPS reconcile (from exit audit) must exist **before** changing production image names.

---

## Agent guardrails

1. Read the exit audit before proposing image name or compose changes.
2. **Streamclone** = backend source; **streampulse** = production promotion namespace (target).
3. Do not recommend backend repo split as step 1.
4. Do not remove runtime services without dependency checks in the exit audit.
5. Do not edit private **streampulse-ops** from public repos without operator scope.
6. Distinguish local dev (`http://localhost:8090`) from hosted promotion.

---

## Operator entrypoints

Public runbooks and probes: [`hosted-production-ops.md`](hosted-production-ops.md), [`hosted-production-vps.md`](hosted-production-vps.md)

Private (streampulse-ops only):

- Production deploy scripts under `scripts/deploy/`
- Compose under `compose/production/`
- Evidence under `docs/deployments/`

---

## Cutover and rollback

**Cutover:** follow exit audit checklist — promote digests, switch private compose to `streampulse/*`, post-promotion smoke, then update public docs.

**Rollback:** restore previous manifest image names/digests; `docker compose pull && docker compose up -d`; re-run smoke. No down-migration unless schema changed independently.

Launch ledger: sibling [improvements.md](../../streamclone-pulse/docs/pulse-extension/evidence/improvements.md).
