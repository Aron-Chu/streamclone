# Security

Short reference for operators and contributors. **Report vulnerabilities:** [SECURITY.md](../SECURITY.md).

## Legal

Streamclone uses unofficial Twitch/7TV endpoints for **personal self-hosting**. Not affiliated with Twitch or 7TV. **You** are responsible for ToS and local law compliance.

## Localhost (default install)

- Watch, directory, and chat read work without login.
- OAuth tokens live in Redis when you sign in.
- `make setup` generates random secrets (`CURATOR_API_TOKEN`, `CLIPPER_WEBHOOK_TOKEN`, etc.).
- Raw compose ports bind to `127.0.0.1` by default. Treat `http://localhost:8090` as the browser entrypoint.
- **Do not** expose raw compose ports (`5432`, `6379`, `8095`, …) to the internet.
- Do not paste raw `docker compose config`, `.env`, setup-control diagnostics, or container env dumps into issues/PRs. They can contain OAuth tokens, setup-control tokens, clipper tokens, and generated install secrets.

### Empty secrets = no auth

| Variable | Risk if empty |
|----------|----------------|
| `CURATOR_API_TOKEN` | Emote admin API open |
| `CLIPPER_WEBHOOK_TOKEN` | Clipper webhooks unauthenticated |
| `SETUP_CONTROL_TOKEN` | Cannot start optional profiles from UI (mutations blocked) |

Run `make validate-env` before treating an install as production-ready.

## Tunnels

Local tunnels should forward the loopback proxy (`127.0.0.1:8090`) only. When `PUBLIC_ORIGIN` or `FRONTEND_ORIGIN` is not loopback:

- Set `TWITCH_DEV_TOKEN_IMPORT_ENABLED=false`.
- Treat `SETUP_CONTROL_TOKEN` and `VITE_CLIPPER_TOKEN` as browser-visible. Any visitor who can load `/config.js` can read them.
- Leave `SETUP_CONTROL_TOKEN` unset unless every tunnel visitor is trusted to start optional profiles or sync clipper auth.
- Leave `VITE_CLIPPER_TOKEN` unset unless every tunnel visitor is trusted to call clipper mutation APIs.

## Public VM deploy

Use TLS (Caddy), firewall (443 only), rate limits, `TWITCH_DEV_TOKEN_IMPORT_ENABLED=false`, strong rotated secrets. The production compose overlay removes raw service ports; Caddy on `80/443` should be the only public entrypoint. Checklist: [deploy/FREE_DEPLOYMENT.md](../deploy/FREE_DEPLOYMENT.md).

## Developers

```sh
make install-hooks && make security-scan
```

CI: gitleaks, govulncheck, npm audit, tests, and compose validation. Never commit `.env` or tokens.

Before sharing support bundles, redact `.env`, rendered compose config, setup-control output, clipper diagnostics, cookies, and screenshots that show tokens or local account data.

## Uninstall

`Uninstall Streamclone` removes local `.env` and volumes on disk only — not GitHub or Twitch data.

## GitHub repo

Enable **secret scanning** and **push protection** under repo Settings → Code security.
