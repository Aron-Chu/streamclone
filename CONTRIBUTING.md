# Contributing

Thank you for your interest in Streamclone. This guide covers local setup, CI expectations, and what not to commit.

## Clone layout (required)

Streamclone and the analytics scraper are two repositories. Compose builds the scraper from a sibling path:

```
parent/
  streamclone/          ← this repo
  streamclone-scraper/  ← https://github.com/YOUR_GITHUB_USER/streamclone-scraper
```

From `deploy/docker-compose.yml`, the scraper build context is `../../streamclone-scraper` (one directory above the main repo root).

## Local development

1. Copy `.env.example` to `.env` and set `CURATOR_API_TOKEN` to a strong random value.
2. Start the stack: `make up` → open **`http://localhost:8090`**.
3. Run targeted tests before opening a PR:
   - `go test ./...` and `go vet ./...`
   - `cd frontend && npm ci && npm run build`

See [docs/development.md](docs/development.md) and [docs/getting-started.md](docs/getting-started.md) for more detail.

## Regenerating README screenshots

Requires a healthy stack and at least one live Twitch channel at capture time:

```sh
make up
make docs-screenshots
```

Outputs: `docs/images/directory.png`, `docs/images/channel.png`. See [docs/screenshots.md](docs/screenshots.md).

On Windows without `make`:

```powershell
cd frontend
npx playwright install chromium
npm run screenshots:readme
```

## CI (GitHub Actions)

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) runs on push/PR to `main`/`master`:

| Job | What it checks |
|---|---|
| `backend` | `go test ./...`, `go vet ./...` |
| `frontend` | `npm ci`, `npm run build` |
| `docker` | Compose config validation; Docker builds for metadata, chat, video, emote, **analytics**, and frontend |
| `smoke` | Full compose stack with scraper sibling checked out at `../streamclone-scraper` |

The smoke job clones **`${{ github.repository_owner }}/streamclone-scraper`**. If your fork uses a different org or the scraper repo is not published yet, smoke may fail until that repository exists — backend and frontend jobs still gate most changes.

Image publishing (manual): [`.github/workflows/publish-images.yml`](.github/workflows/publish-images.yml).

## Secrets and git hygiene

**Never commit:**

- `.env` or `.env.test` (live tokens, Duck DNS credentials, OAuth secrets)
- `node_modules/`, `frontend/dist/`, `clipper/.venv/`, `clipper-data/`
- `.codegraph/`, `cookies.txt`, `scratch/`, `memories/`, `frontend/test-results/`

Use `.env.example` as the template for new variables.

## Pull requests

- Keep changes focused; match existing naming and patterns in the touched package.
- Run the relevant tests locally (narrow packages first).
- Do not paste full API JSON or secrets into PR descriptions.
- Include updated screenshots only when UI changes warrant it.

## Legal

Operators are solely responsible for compliance with third-party Terms of Service. See [docs/security.md](docs/security.md).
