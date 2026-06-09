# Getting started

## Prerequisites

- Docker Desktop with `docker compose` on PATH
- Host Go is only needed for direct backend tests/builds outside containers

## Two-repo clone layout

Streamclone and the analytics scraper are **separate repositories**. Docker Compose builds the scraper from a sibling directory (`../../streamclone-scraper` relative to `deploy/`):

```
parent/
  streamclone/          ← this repo
  streamclone-scraper/  ← sibling (TwitchTracker / Playwright scraper)
```

Clone both into the same parent folder before running `make up`. See the [streamclone-scraper README](../streamclone-scraper/README.md) for standalone scraper use.

## Quick start

```sh
cp .env.example .env
```

Edit `.env` and set at minimum `CURATOR_API_TOKEN` to a strong random value before exposing the stack.

The checked-in example favors the single-origin local proxy on `http://localhost:8090`. Keep those defaults for the fastest same-machine development loop. Only change `PUBLIC_ORIGIN` and related values when you need a public tunnel or HTTPS callback.

```sh
make up
```

This runs `docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml up -d --build`. On first run the `migrate` job initialises the PostgreSQL schema automatically and the local Caddy proxy is exposed on `http://localhost:8090`.

Open the frontend at **`http://localhost:8090`**.

On Windows without `make`, run the same compose files manually from the repo root (see [Makefile](../Makefile) `up` target).

## Common commands

| Command | Purpose |
|---|---|
| `make migrate` | Run migrations separately (e.g. after a schema update) |
| `make down` | Stop and remove containers (main stack, observability overlay, prod overlay, and orphans) |
| `make down-clean` | Wipe containers **and** named volumes (`pg-data`, `minio-data`, `clipper-data`) |
| `make ps` | List Streamclone containers |
| `make ports` | Windows: show which process holds 1935/8090/etc. |

## Port conflicts

If `make app` fails with `address already in use` on port `1935` or similar, the old stack is usually still running:

```sh
make down
make ps          # should show no streamclone containers
make ports       # Windows: who holds 1935/8090/etc.
make app
```

On Windows, stale `wslrelay` bindings can also serve old data on localhost — see [`.kiro/steering/windows-dev.md`](../.kiro/steering/windows-dev.md).

## Smoke checklist

After `make up`, verify the runnable demo:

```sh
curl http://localhost:8090/
curl http://localhost:8090/v1/auth/debug
curl http://localhost:8081/healthz
curl http://localhost:8082/healthz
curl http://localhost:8083/healthz
curl http://localhost:8084/healthz
```

Then open `http://localhost:8090`, confirm the directory renders, search and category filters respond, a channel route shows either playback or a structured stream error, and chat shows its connection state.

For optional chat sending, use `http://localhost:8090` and click **Use local token** — see [oauth.md](../oauth.md) and [development.md](development.md).

## Next steps

- [Configuration](configuration.md) — environment variables
- [Development](development.md) — local Go/frontend workflow
- [Deployment](deployment.md) — observability, production, clipper
- [DISTRIBUTION.md](../DISTRIBUTION.md) — sharing and cloud storage tiers
- [CONTRIBUTING.md](../CONTRIBUTING.md) — clone layout, CI, secrets hygiene
