# Public ops cleanup inventory

**Status:** Executed 2026-07-07 — operator artifacts mirrored to private **streampulse-ops** at `b8d595f`, then removed from public tree.

Private mirror: `streampulse-ops/archive/public-import/2026-07-07/`

## Removed from public streamclone

| Category | Public paths removed | Private destination |
|----------|---------------------|---------------------|
| Ops scripts | `scripts/ops/*`, `scripts/load/hosted-*`, `scripts/load/pulse-load-*`, `scripts/cloudflared-tunnel-token-rotate.sh`, `scripts/batch-q-*` | `archive/public-import/2026-07-07/` + active private scripts |
| Ops runbooks | `docs/ops/**` (replaced by stub README) | private ops |
| Operator host doc | `docs/hosted-production-vps.md` (replaced by stub) | private ops |
| Deploy evidence | `docs/agent-notes/*hosted*`, `docs/pulse-extension/*evidence*.txt` with host topology | private ops |
| Internal ops smoke | `deploy/smoke/hosted-internal-ops-smoke.sh` | private ops |
| Migration internals | `migration-baseline.md`, `docs/ops-migration-plan.md`, `docs/ops-migration-prepared-report.md` | private ops |

## Stays in public streamclone

- App code, migrations, local compose, CI / GHCR release workflows
- `scripts/ops-stub.sh` and **public-API-only** `scripts/hosted-launch-probes.sh`
- Contract docs (redacted): `production-artifact-contract.md`, `production-promotion-contract.md`, `hosted-production-ops.md`
- Env templates: `deploy/env/profile-hosted-*.env.example`
- Public API smoke: `deploy/smoke/test-*-hosted.sh`

## History rewrite

After merge, run `scripts/ops/run-filter-repo-redaction.sh` from a **mirror clone** (path removal + identifier redaction). Force-push only after fresh-clone audit.

**Do not** push pre-redaction backup refs to public GitHub.
