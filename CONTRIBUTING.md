# Contributing

## Clone layout

```
parent/
  streamclone/          ← this repo (make bootstrap → http://localhost:8090)
  streamclone-scraper/  ← optional sibling for `make up-scraper`
```

## Local setup

1. `make bootstrap` (or `scripts/bootstrap.ps1` on Windows)
2. `make smoke` when services are healthy
3. `make install-hooks` — requires `pip install pre-commit`

## Tests before PR

```sh
go test ./... && go vet ./...
cd frontend && npm ci && npm run build
make smoke          # stack must be up
make smoke-ui       # adds Playwright smoke-core
```

## CI

[`.github/workflows/ci.yml`](.github/workflows/ci.yml): backend, frontend build, docker image builds, **smoke-core** (no scraper).

[`.github/workflows/smoke-scraper.yml`](.github/workflows/smoke-scraper.yml): nightly / manual scraper profile smoke.

## Secrets — never commit

`.env` and any `.env.*` except `.env.example` / `.env.dev`, Twitch/clipper tokens, `PROXY_*` credentials, `node_modules/`, `frontend/dist/`, `clipper/.venv/`, `clipper-data/`, `*.sqlite`, `.codegraph/`, `out.json`

Use `.env.dev` for local bootstrap; `.env.example` for the full reference.

## Maintainer media

Regenerate README screenshots/GIF: `make docs-media` (healthy stack + ffmpeg for GIF).

Agent/developer docs live under `.kiro/steering/` — not linked from this README.
