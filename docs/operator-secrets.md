# Operator secrets (generic)

Hosted production secrets are **not** stored in this public repository. They live on the production host and in private **streampulse-ops** runbooks.

## Secret types (generic)

| Secret | Purpose |
|--------|---------|
| `AUTH_COOKIE_SECRET` | Session signing |
| `CURATOR_API_TOKEN` | Internal API auth |
| `SCRAPER_API_KEY` | Scraper ↔ analytics |
| Extension beta gate keys | Hosted extension beta gate |
| Twitch OAuth client id/secret | API access |
| Archive/R2 credentials | Corpus blob storage |
| Postgres password | Database |

## Local development

Run `make setup` to synthesize `.env` from `.env.example` + `deploy/env/profile-dev.env` with generated dev secrets.

## Production

Use private **streampulse-ops** and host paths such as `/etc/streamclone/secrets/` (mode 700). Never commit real values.

See [docs/security.md](security.md) and [docs/production-artifact-contract.md](production-artifact-contract.md).
