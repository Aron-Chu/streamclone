# Pulse Wire — scope note

> **Scope split.** "Pulse Wire" has two meanings in this codebase that must not be conflated:
>
> - **Local Streamclone news wire** (`/pulse-wire` route, `cmd/storygraph`, `internal/storygraph/*`) — a watch-stack optional tier; lives in this repo if storygraph is present.
> - **StreamPulse ingest / analytics wire** — moved to private **streampulse-backend**; do not implement here.

## Storygraph / local news wire (this repo)

If `cmd/storygraph` is present, its scope is the local `/pulse-wire` page (Reddit LSF, YouTube, story scoring). Gate: `PULSE_WIRE_ENABLED=false` by default; Core Watch has no dependency on it.

See the historical `pulse-wire.md` content in `docs/archive/agent-plans/` if you need reference material for the old storygraph design.

Runtime probes (if storygraph is running):

```bash
curl http://localhost:8090/v1/pulse-wire/source-health
curl "http://localhost:8090/v1/pulse-wire/feed?window=24h&sort=rank"
```

Checks:

```bash
go test ./internal/storygraph/...
cd frontend && npm run build
```

## StreamPulse ingest / analytics wire → streampulse-backend

Any "Pulse Wire" ingest that feeds the StreamPulse hub, extension, or analytics portal lives in private **streampulse-backend**. Do not implement it here.

See [`docs/streampulse-product-boundary.md`](../../docs/streampulse-product-boundary.md) for ownership.
