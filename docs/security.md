# Security

Short operator checklist. Report vulnerabilities through [SECURITY.md](../SECURITY.md).

## Localhost Default

- Use `http://localhost:8090/`.
- Raw service ports bind to `127.0.0.1` in local compose.
- Do not expose Redis, Postgres, MinIO, MediaMTX, setup-control, or raw Go service ports.
- Do not paste `.env`, rendered compose config, setup-control diagnostics, cookies, or token-bearing logs into issues or PRs.

## Secrets

`make setup` generates local secrets. Production or public installs must rotate placeholders.

Important variables:

| Variable | Risk if weak or empty |
|----------|-----------------------|
| `AUTH_COOKIE_SECRET` | Signed auth cookies can be forged |
| `CURATOR_API_TOKEN` | Emote admin API open |
| `CLIPPER_WEBHOOK_TOKEN` | Clipper mutations unauthenticated |
| `SETUP_CONTROL_TOKEN` | Optional service mutations exposed |
| `S3_SECRET_KEY` | Object storage access |

Run:

```sh
make validate-env
make security-scan
```

## Frontend dependency audit

`make frontend-audit` remains a required prerequisite of `make check`. It runs
both full and production `npm audit --json --audit-level=high` reports. The full
command explicitly includes prod, dev, optional and peer dependencies; production
explicitly includes prod, optional and peer dependencies and omits dev. These
CLI settings override inherited npm include/omit defaults, so `NODE_ENV=production`
cannot remove dev from the full audit and inherited `include=dev` cannot expand
the production audit. The gate retains the raw JSON, stderr,
and command status in a printed temporary directory. An optional
`STREAMCLONE_AUDIT_REPORT_DIR` selects an evidence directory; existing reports
are never overwritten. Invalid reports and failed audit commands fail the gate.
Moderate findings remain reported under the existing high severity threshold.

The only full-audit disposition is
[GHSA-vfj7-8cjw-p6xm](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm), a
stack-exhaustion vulnerability in `braces` through deeply nested patterns. As of
2026-10-02 its advisory lists no patched version. The exact reviewed development
chain is `braces@3.0.3`, `chokidar@3.6.0`, `micromatch@4.0.8`,
`fast-glob@3.3.3`, and `tailwindcss@3.4.19`. Every installed node must be marked
development-only in this frontend's lock, with its complete reviewed regular
dependency key/range map from commit `86e7014c121303894185f47e6e0545bfad294be1`,
and every audit edge must lead to that exact advisory. New high/critical
advisories, additional occurrences, version changes, missing nodes, changed
metadata, added or missing regular dependency edges, or a production dependency
fail. Optional and peer dependencies are included in the audit scope but their
lock maps are not frozen by this disposition. The production audit accepts no
high/critical disposition.

Tailwind runs against repository-controlled source globs during CSS compilation.
The static browser application does not accept glob patterns for these tools.
The Vite production build inspects every emitted JavaScript chunk, including
lazy routes, and fails if any of these five packages enters browser output.
`frontend/tests/frontendAudit.test.ts` exercises report/graph drift and the
runtime boundary through the existing frontend test suite. The build tool itself
remains vulnerable to malicious build input; dependency updates or changes to
build inputs require a new review. Follow up with a compatible upstream fix or
a separately tested Tailwind migration when available.

This gate adapts the braces-only logic reviewed in streamclone-pulse PR #56 and
published at commit `1784c73ad285da1900ae0a8c56600b75a0f551a4`. It does not copy
that repository's React Router dispositions. Existing moderate Router findings
remain visible; they are not waived by the braces decision.

Operator secret files (webhooks, Azure connection strings) live outside the repo — see [`docs/operator-secrets.md`](operator-secrets.md). Initialize with `scripts/operator-secrets-init.ps1` or `scripts/operator-secrets-init.sh`.

## Public repo boundary

This public repository is **application source and contracts only**. Do not commit:

- Production host IPs or resolvable infrastructure hostnames used as deploy targets
- SSH key paths, fingerprints, or `root@…` shell accounts
- Private ops checkout paths, production env file paths, or live env values
- Operator runbooks, promotion manifests, soak evidence, or VPS deploy scripts

Hosted production execution lives in **private streampulse-ops**. Public contract stubs: [hosted-ops-stub.md](hosted-ops-stub.md) and [streampulse-product-boundary.md](streampulse-product-boundary.md). Extended hosted launch probes and health URLs are documented in private ops — not in this repo.

Pre-commit runs `scripts/pre-commit-public-ops-guard.sh` and `scripts/pre-commit-product-boundary-guard.sh` (strict on `master`).

## Tunnels

Forward only the Caddy proxy (`127.0.0.1:8090`).

When `PUBLIC_ORIGIN` or `FRONTEND_ORIGIN` is not loopback:

- Set `TWITCH_DEV_TOKEN_IMPORT_ENABLED=false`.
- Treat `SETUP_CONTROL_TOKEN` and `VITE_CLIPPER_TOKEN` as browser-visible.
- Leave setup-control and clipper mutation tokens unset unless every visitor is trusted.
- Prefer Cloudflare Tunnel or Tailscale over opening inbound ports.

Free ngrok warning pages can break fetch, HLS, and WebSocket traffic.

## Public VM

Use `deploy/docker-compose.prod.yml`, TLS through Caddy, a firewall with only `80/443` public, strong rotated secrets, and `TWITCH_DEV_TOKEN_IMPORT_ENABLED=false`.

Guide: [deploy/FREE_DEPLOYMENT.md](../deploy/FREE_DEPLOYMENT.md).

## GitHub

Enable secret scanning, push protection, Dependabot security updates, and branch protection requiring CI.
