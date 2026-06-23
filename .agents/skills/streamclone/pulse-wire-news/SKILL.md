---
description: Pulse Wire news — feed, bans, LSF/Reddit, storygraph ingest. Distinguish Pulse Wire news from Pulse analytics time-series.
---

# Pulse Wire News

Read `AGENTS.md`, `.kiro/steering/pulse-wire.md`, and `docs/options.md`.

## Pulse Wire news vs Pulse analytics

| Area | Pulse Wire (news) | Pulse (analytics / Grafana) |
|------|-------------------|-------------------------------|
| Purpose | Social/news feed, bans, LSF/Reddit matching, storygraph | Live stats time-series, Influx, Grafana dashboards |
| Profile | `pulse-wire` (+ optional `pulse-wire-semantic`) | `pulse` |
| UI | `frontend/src/components/pulsewire/` | Analytics charts, Grafana `:3000` |
| Services | storygraph, media-matcher, x-ingest | influxdb, prometheus, grafana |
| Data | Postgres storygraph tables, social ingest | Influx minute buckets, Prometheus metrics |

Do not confuse **Pulse Wire ingest** with **Analytics minute rollups** — different profiles, tables, and steering docs.

## Workflow

1. Confirm `pulse-wire` profile services up (`storygraph`, ingest workers).
2. Feed / bans / LSF/Reddit: `internal/social/`, `internal/storygraph/`
3. UI: `get_ast_chunk("PulseWirePage")`, `get_blast_radius("ingestAll")`

## MCP / runtime

- `compose_logs("storygraph")` — ingest API/worker logs
- `postgres_query` — storygraph tables (SELECT only)
- `stack_health` — optional service presence

## Tests

```sh
make test-storygraph
make smoke-ui   # pulse-wire Playwright when stack + profile up
cd frontend && npm run test:smoke
```

## Related (not duplicated)

- Analytics sync / scraper charts → `analytics-sync` skill
- Stack health / ports → `stack-debug` skill
