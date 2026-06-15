---
description: Debug or change Streamclone Analytics sync, rollups, VOD context, scraper-backed charts, or Pulse rollup export.
---

# Analytics Sync

Read `AGENTS.md`, `.kiro/steering/analytics.md`, and `.kiro/steering/tech.md`.

## First Checks

- Use `http://localhost:8090`, not raw service ports.
- Core Watch can have empty minute charts until Analytics/scraper is started.
- Scraper/Cloudflare details live in `docs/scraper-cloudflare-and-proxy.md`.

## Codegraph

- `get_blast_radius("mergeMinuteRollups")`
- `get_ast_chunk("gqlCommentText")`
- `get_ast_chunk("hasGoodChatCoverageFromRollups")`
- `get_ast_chunk("SyncProgressPanel")`

## Runtime

- `stack_health`
- `scraper_probe`
- `postgres_query` for rollup/status checks

## Tests

```sh
go test ./internal/analytics/...
cd frontend && npm run build
make scraper-check
```
