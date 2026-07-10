# Analytics — routed to streampulse-backend

> **Historical stub.** Streamclone analytics source (`cmd/analytics`, `internal/analytics`, `frontend/src/components/Analytics.tsx`) was removed from this repository as part of the StreamPulse boundary split. Do not re-add analytics API, rollup, or ingest code here.

## Ownership

| Concern | Owner |
|---------|-------|
| Watch UI, chat, emotes, playback | **streamclone** (this repo) |
| Analytics BFF, rollups, ingest, hub API | private **streampulse-backend** |
| Extension + portal UI | public **streamclone-pulse** |
| Deploy / secrets / evidence | private **streampulse-ops** |

See [`docs/streampulse-product-boundary.md`](../../docs/streampulse-product-boundary.md) for the canonical boundary.

## Where to work

- **Analytics API / hub / ingest changes** → private **streampulse-backend** (`internal/analytics`, BFF routes)
- **Portal UI changes** → public **streamclone-pulse** (`streampulse-web/src/`)
- **Scraper / TwitchTracker** → sibling **streamclone-scraper** (compose `--profile scraper`); scraper health still affects local watch charts

## Local watch charts (optional Influx/Grafana)

If you want Grafana dashboards for local development, the `pulse` compose profile is still available:

```bash
make pulse   # deploys charts/pulse/ (InfluxDB, Grafana, Prometheus)
```

This is an **optional local dev layer only** — not an analytics API or hub replacement.

## Checks (watch-only scope)

```bash
cd frontend && npm run build
make smoke
```

Do not run `go test ./internal/analytics/...` — the Go source was removed. If the directory still exists, it contains only test artifacts; do not extend it.
