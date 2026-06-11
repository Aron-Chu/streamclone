# Contributing

## Clone layout

```
parent/
  streamclone/          ← this repo (make bootstrap → http://localhost:8090)
  streamclone-scraper/  ← optional sibling for `make up-scraper`
```

## Local setup

1. `make setup` (or `scripts/setup.ps1` on Windows) — interactive wizard; CI parity: `scripts/setup.sh --profile core --non-interactive`
2. `make smoke` when services are healthy
3. `make validate-env` — check `.env` for your profile
4. `make install-hooks` — requires `pip install pre-commit`

Legacy one-liner: `make bootstrap` still works (core profile only).

## Tests before PR

```sh
make install-hooks   # once per clone
make security-scan   # gitleaks + validate-env
go test ./... && go vet ./...
cd frontend && npm ci && npm run build
make smoke          # stack must be up
make smoke-ui       # adds Playwright smoke-core
```

## CI

[`.github/workflows/ci.yml`](.github/workflows/ci.yml): **gitleaks** secret scan, backend (incl. govulncheck), frontend build (incl. npm audit), docker image builds, **smoke-core** (no scraper).

[`.github/workflows/smoke-scraper.yml`](.github/workflows/smoke-scraper.yml): nightly / manual scraper profile smoke.

See [`docs/security.md`](docs/security.md) for deployment hardening, dev-token import caveats, and the pre-PR security checklist.

## Secrets — never commit

`.env` and any `.env.*` except `.env.example` / `.env.dev`, Twitch/clipper tokens, `PROXY_*` credentials, `node_modules/`, `frontend/dist/`, `clipper/.venv/`, `clipper-data/`, `*.sqlite`, `.codegraph/`, `out.json`

Use `.env.dev` for local bootstrap; `.env.example` for the full reference.

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/) for every commit:

```
type(scope): short imperative summary
```

- **type** — `feat`, `fix`, `chore`, `docs`, `refactor`, `test`, `ci`, `build`, etc.
- **scope** — optional area, e.g. `analytics`, `clipper`, `frontend`, `repo`
- **summary** — lowercase, imperative mood, no trailing period; explain *why* when the diff alone is unclear

Examples from this repo:

```
fix(analytics): stop double IRC part on untrack
chore(repo): add bootstrap setup and README media
feat(clipper): add emote overlay templates
```

Multi-line bodies are fine for context; keep the first line within ~72 characters.

## Maintainer media

Regenerate README screenshots/GIF: `make docs-media` (healthy stack + ffmpeg for GIF).

Agent/developer docs live under `.kiro/steering/` — not linked from this README.
