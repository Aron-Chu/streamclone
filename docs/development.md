# Development

## Local Go backend

```sh
make build
make test
make vet
```

These run `go build ./...`, `go test ./...`, and `go vet ./...` against the host Go toolchain.

For package-scoped changes, prefer narrow tests first (e.g. `go test ./internal/chat/auth/...`) before the full suite.

## Frontend

The checked-in `frontend/node_modules` can contain WSL-style shims that Windows PowerShell cannot execute. If `npm run build` fails with `tsc is not recognized`, either run frontend commands through WSL:

```sh
cd /mnt/c/Users/Aron/twitch-7tv-clone/frontend
npm run dev -- --host 127.0.0.1 --port 5174
npm run build
```

Or reinstall dependencies from Windows in `frontend/`:

```sh
npm ci
npm run build
```

For auth, chat, and playback changes, validate against the proxied bundle at **`http://localhost:8090`**, not just the standalone Vite dev server on `5174`.

## Local workflow notes

- Treat **`http://localhost:8090`** as the stable local entrypoint. Keep frontend runtime URLs on same-origin/`auto` values when using the local proxy; mixing direct `5174`, `8081`, `8082`, or `8083` browser origins causes auth and capability drift.
- The **Use local token** button is controlled by `GET /v1/me` returning `canImportLocalToken=true`, not by a frontend feature flag alone. It first claims any prepared local token from `make twitch-local-auth`, then falls back to the backend-managed Twitch device-code flow. If the button disappears, verify `TWITCH_DEV_TOKEN_IMPORT_ENABLED=true`, open the app through `http://localhost:8090`, and check `http://localhost:8090/v1/auth/debug`.
- The player separates **Request** quality from **Loaded** quality. Once the backend discovers Usher renditions, the request menu narrows to actual requestable variants only.
- The lower channel workspace supports an expandable panel plus **Comfort** and **Dense** density modes. Preserve the Diagnostics, LSF, and Emotes surfaces when changing this area.
- If chat shows intermittent websocket `502` errors on `/v1/ws`, use the standard `caddy:2` image for the reverse proxy. In this repo, `caddy:2-alpine` produced Docker DNS lookup timeouts such as `lookup chat: i/o timeout` during websocket proxying.

## Windows / Docker

See [`.kiro/steering/windows-dev.md`](../.kiro/steering/windows-dev.md) for `wslrelay` port conflicts, codegraph on WSL, and other Windows-specific pitfalls.

## Auth

Localhost-only Twitch chat login: [oauth.md](../oauth.md) and [`.kiro/steering/local-auth.md`](../.kiro/steering/local-auth.md).

Useful debug:

```sh
make twitch-debug
```

## AI agents and code navigation

Contributors using Cursor or other agents should read [AGENTS.md](../AGENTS.md) for the task router, steering docs, and codegraph MCP (`make codegraph-install && make codegraph`).

## README screenshots

To regenerate documentation screenshots:

```sh
make up
make docs-screenshots
```

See [screenshots.md](screenshots.md).
