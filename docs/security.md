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
