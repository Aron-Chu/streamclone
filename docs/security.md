# Security and legal

## Legal / Terms of Service notice

This project accesses third-party internal endpoints (Twitch internal GraphQL, Usher, anonymous IRC, 7TV v3 API) that are not part of any public developer programme. It is provided solely for **educational and personal self-hosting purposes**. It is **not affiliated with, endorsed by, or sponsored by Twitch Interactive, Inc. or 7TV**.

Operating this software may violate the Terms of Service of Twitch, 7TV, and other upstream platforms, as well as applicable laws in your jurisdiction. **The operator is solely responsible for ensuring compliance with all relevant terms, licenses, and laws.** The authors accept no liability for misuse.

## Viewer-facing access model

Viewer-facing read endpoints (directory, stream start, anonymous chat listening) and the emote asset CDN remain available without a first-party account. Sending real chat messages uses Twitch OAuth, stores Twitch tokens server-side in Redis, and sends through an authenticated IRC connection for the logged-in Twitch user. The app does not maintain its own username/password account system.

## Curator / Admin API

The Emote Service exposes a curator API for managing the emote database. All write/admin endpoints require `Authorization: Bearer <CURATOR_API_TOKEN>`. **Set a strong token before exposing the service outside localhost.** Deploying with the default `change-me` is a security defect.

If `CURATOR_API_TOKEN` is unset, curator routes accept an empty bearer token and become unauthenticated.

## Clipper / Clip Studio

The clipper service protects mutating webhook endpoints with `CLIPPER_WEBHOOK_TOKEN`. When that token is empty, webhook auth is skipped entirely (`clipper/liveclipper/security.py`). Run `make setup` so `env_generate_secrets` creates a token.

`VITE_CLIPPER_TOKEN` is injected into the browser config at runtime. Treat it as client-visible: use a strong random value and restrict network access.

Read paths (`GET /v1/jobs`, media downloads) are unauthenticated today. Do not expose clipper port `:8095` beyond trusted networks.

## Dev-only auth features

`TWITCH_DEV_TOKEN_IMPORT_ENABLED=true` in `.env.dev` enables loopback-only device-code and token-import endpoints. Disable (`false`) on any deployment reachable outside your machine.

Dev import checks loopback via `Host` / `X-Forwarded-Host`. If you run behind a reverse proxy, ensure it does not forward client-supplied `X-Forwarded-Host: localhost` to the chat service.

## Deployment hardening

For any public-facing deployment:

- TLS-terminating reverse proxy in front of the stack
- Rate limiting at the proxy for stream start, search, and WebSocket connect
- Restrict MinIO console (`9001`) to trusted networks
- Do not publish Postgres (`5432`), Redis (`6379`), MinIO (`9000`/`9001`), or scraper (`8000`) on the public internet
- Rotate `CURATOR_API_TOKEN`, `AUTH_COOKIE_SECRET`, `CLIPPER_WEBHOOK_TOKEN`, `SCRAPER_API_KEY`, Twitch OAuth credentials, and object-store credentials from defaults
- Set `TWITCH_DEV_TOKEN_IMPORT_ENABLED=false` outside local dev

See [`deploy/FREE_DEPLOYMENT.md`](../deploy/FREE_DEPLOYMENT.md) for a production checklist.

## Developer workflow

Local guards (run before opening a PR):

```sh
make install-hooks          # once per clone — gitleaks + env gate + fmt/vet/tsc
make security-scan          # gitleaks on full tree + validate-env for your profile
make validate-env           # fail on placeholder CURATOR / empty clipper token
go test ./... && go vet ./...
cd frontend && npm run build
```

CI mirrors this with a **gitleaks** job (full history), **govulncheck** (non-blocking), and **npm audit** (non-blocking). Enable [GitHub secret scanning and push protection](https://docs.github.com/en/code-security/secret-scanning) on the remote repo.

Never commit: `.env` (except templates), `clipper-data/`, `*.sqlite`, `.cursor/mcp.json`, compiled binaries, `__pycache__`, Playwright debug artifacts.

## Application security

Services validate client inputs (channel names, pagination, upload type/size), use parameterised SQL, pass subprocess arguments as argv slices (never shell strings), and render chat as plain text — not innerHTML.

Known local-dev gaps (acceptable on localhost, not for public deploy): permissive CORS on Go services, unauthenticated video/analytics control APIs, default compose credentials. See the [Streamclone Security Audit](https://github.com/Aron-Chu/streamclone) plan for the full findings table.

## GitHub settings

Enable secret scanning and push protection under **Settings → Code security**.
