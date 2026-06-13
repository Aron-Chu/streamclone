# Contributing

Thanks for helping with Streamclone. **End users:** you do not need this file — see [docs/install-desktop.md](docs/install-desktop.md).

## Setup

```sh
git clone https://github.com/Aron-Chu/streamclone.git
cd streamclone
make install-hooks    # once — pre-commit (gitleaks, fmt, tsc)
make setup            # or: scripts/setup.ps1
```

Optional sibling repo for Analytics scraper: [streamclone-scraper](https://github.com/Aron-Chu/streamclone-scraper).

## Before you push

```sh
make security-scan
go test ./... && go vet ./...
cd frontend && npm ci && npm run build
make clipper-test
make smoke            # stack up
```

CI runs the same on `master` — see [`.github/workflows/ci.yml`](.github/workflows/ci.yml).

## Commits

[Conventional Commits](https://www.conventionalcommits.org/): `type(scope): summary`

Author: **Aron-Chu** `<aroncloudchu@gmail.com>` — no `Co-authored-by` agent trailers. Details: [`.cursor/rules/commits.mdc`](.cursor/rules/commits.mdc).

## Secrets

Never commit `.env`, tokens, `oauth-bundle.env`, `clipper-data/`, or build artifacts. Templates only: `.env.example`, `.env.dev`.

## More

| Topic | Doc |
|-------|-----|
| Security / hardening | [SECURITY.md](SECURITY.md) → [docs/security.md](docs/security.md) |
| Agent steering | `.kiro/steering/` (maintainers) |
| Repo cleanup index | [docs/repo-maintenance.md](docs/repo-maintenance.md) |
