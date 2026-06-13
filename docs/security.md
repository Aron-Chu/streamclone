# Security

Short reference for operators and contributors. **Report vulnerabilities:** [SECURITY.md](../SECURITY.md).

## Legal

Streamclone uses unofficial Twitch/7TV endpoints for **personal self-hosting**. Not affiliated with Twitch or 7TV. **You** are responsible for ToS and local law compliance.

## Localhost (default install)

- Watch, directory, and chat read work without login.
- OAuth tokens live in Redis when you sign in.
- `make setup` generates random secrets (`CURATOR_API_TOKEN`, `CLIPPER_WEBHOOK_TOKEN`, etc.).
- **Do not** expose raw compose ports (`5432`, `6379`, `8095`, …) to the internet.

### Empty secrets = no auth

| Variable | Risk if empty |
|----------|----------------|
| `CURATOR_API_TOKEN` | Emote admin API open |
| `CLIPPER_WEBHOOK_TOKEN` | Clipper webhooks unauthenticated |
| `SETUP_CONTROL_TOKEN` | Cannot start optional profiles from UI (mutations blocked) |

Run `make validate-env` before treating an install as production-ready.

## Public VM deploy

Use TLS (Caddy), firewall (443 only), rate limits, `TWITCH_DEV_TOKEN_IMPORT_ENABLED=false`, strong rotated secrets. Checklist: [deploy/FREE_DEPLOYMENT.md](../deploy/FREE_DEPLOYMENT.md).

## Developers

```sh
make install-hooks && make security-scan
```

CI: gitleaks, govulncheck, npm audit on `master`. Never commit `.env` or tokens.

## Uninstall

`Uninstall Streamclone` removes local `.env` and volumes on disk only — not GitHub or Twitch data.

## GitHub repo

Enable **secret scanning** and **push protection** under repo Settings → Code security.
