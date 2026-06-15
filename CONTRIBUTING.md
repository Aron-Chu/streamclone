# Contributing

End users usually want [docs/install-desktop.md](docs/install-desktop.md), not this file.

## Setup

```sh
git clone https://github.com/Aron-Chu/streamclone.git
cd streamclone
make install-hooks
make setup
```

Optional Analytics scraper repo: [streamclone-scraper](https://github.com/Aron-Chu/streamclone-scraper).

## Before You Push

Use the bundled check target:

```sh
make check
```

Useful narrower checks:

```sh
make security-scan
make test
make vet
make frontend-build
make frontend-test
make frontend-audit
make clipper-test
make compose-config-check
```

`make test`, `make vet`, and `make build` fall back to Docker when local Go is missing.

## Commits

Use Conventional Commits: `type(scope): summary`.

Author commits as **Aron-Chu** `<aroncloudchu@gmail.com>`. Do not add agent co-author trailers.

## Secrets

Never commit `.env`, tokens, OAuth bundles, `clipper-data/`, rendered compose config, or token-bearing logs. Templates only: `.env.example`, `.env.dev`.

## More

- Security: [SECURITY.md](SECURITY.md) and [docs/security.md](docs/security.md)
- Maintainer index: [docs/repo-maintenance.md](docs/repo-maintenance.md)
- Agent steering: `AGENTS.md` and `.kiro/steering/`
