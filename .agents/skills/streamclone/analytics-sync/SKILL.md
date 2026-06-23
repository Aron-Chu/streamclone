---
description: Debug or change Streamclone Analytics sync, rollups, VOD context, scraper-backed charts, or Pulse rollup export.
---

# Analytics Sync

Read `AGENTS.md`, `.kiro/steering/analytics.md`, and `.kiro/steering/tech.md`.

## Bottleneck: GQL rate limits (not CPU)

Analytics sync is usually limited by **Twitch GQL pages and rate limits** — VOD chat GQL, pagination, and backoff — not CPU on the analytics service. Check sync status and GQL error rates before scaling containers.

## Pulse Grafana vs Analytics charts

| Area | Pulse (Grafana / Influx) | Analytics (charts in app) |
|------|--------------------------|-----------------------------|
| Profile | `pulse` | core + optional `scraper` |
| Data | Influx time-series, Prometheus | Postgres rollups, scraper minute data |
| UI | Grafana `:3000` | `SyncProgressPanel`, minute charts, heatmaps |
| Steering | `.kiro/steering/analytics.md` (Pulse section) | Same + scraper docs |

Do not confuse Grafana live stats with scraper-backed minute charts in the Watch UI.

## First checks

- Use `http://localhost:8090`, not raw service ports.
- Core Watch can have empty minute charts until Analytics/scraper is started.
- Scraper/Cloudflare details: `docs/scraper-cloudflare-and-proxy.md`.
- Rollups, heatmaps, VOD context: Postgres analytics tables via `postgres_query` (SELECT only).

## Codegraph

- `get_blast_radius("mergeMinuteRollups")`
- `get_ast_chunk("gqlCommentText")`
- `get_ast_chunk("hasGoodChatCoverageFromRollups")`
- `get_ast_chunk("SyncProgressPanel")`

## MCP / runtime

- `stack_health`
- `scraper_probe`
- `postgres_query` for rollup/status checks

## Tests

```sh
make test-analytics
go test ./internal/analytics/...
make scraper-check
```
