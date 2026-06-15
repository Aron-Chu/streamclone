# Technical Steering

Read `AGENTS.md` first. Use codegraph before broad source reads.

## Stack

- Go services: `cmd/*`, `internal/*`, `net/http`, chi, websocket, pgx, Redis, slog.
- Frontend: Vite, React, TypeScript, Tailwind, hls.js, Zustand.
- Infra: Docker Compose, Caddy, MediaMTX, PostgreSQL, Redis, MinIO.
- Optional: scraper profile, Clip Studio worker, Pulse dashboards.

Use `http://localhost:8090` as the browser/API boundary.

## Commands

```sh
make up
make down
make check
make security-scan
make compose-config-check
make codegraph
```

Narrow checks:

```sh
make test
make vet
make frontend-build
make frontend-test
make clipper-test
```

## Rules

- Keep configs environment-driven.
- Do not hardcode secrets, tokens, hostnames, or raw service ports.
- Use same-origin frontend config through Caddy unless intentionally bypassing.
- Use bounded queues, context cancellation, retry caps, and backoff for upstream loops.
- Keep release install behavior separate from source checkout behavior.

## Data

- PostgreSQL: durable emotes, sets, mappings, jobs, local follows.
- Redis: hot cache, pub/sub, chat fan-out, channel emote dictionaries.
- Object storage: rendered emotes at `{emote_id}/{scale}.webp`.

## When Done

- Docs-only: link/whitespace checks are enough unless commands changed.
- Code changes: run narrow tests first, then broader checks.
- Large symbol moves: run `make codegraph` and confirm `graph_status()`.
