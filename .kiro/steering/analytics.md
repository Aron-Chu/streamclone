# Analytics Steering

Analytics turns Twitch stream history, viewer data, chat rollups, emotes, VOD context, and clip jobs into local insight views.

## Boundaries

- Go service: `cmd/analytics`, `internal/analytics`.
- Metadata helpers: `internal/metadata/api`, `internal/metadata/helix`.
- Frontend: `frontend/src/components/Analytics.tsx` and related utilities.
- Optional scraper: TwitchTracker minute charts. Core Watch must still work without it.

## Current Rules

- Prefer synced local rollups over live-only guesses.
- Preserve honest empty states: missing scraper, no VOD, no chat coverage, auth issue, or upstream block should say what happened.
- Keep TwitchTracker scraping direct through Camoufox unless scraper routing proves otherwise.
- Reddit LSF is best-effort and must degrade cleanly.
- Shared IRC ingest is a future simplification; avoid adding more independent IRC clients.

## Pulse

Pulse is optional Grafana/Influx over local Analytics rollups. In-app Analytics remains canonical.

## Codegraph Hints

- `get_blast_radius("mergeMinuteRollups")`
- `get_ast_chunk("gqlCommentText")`
- `get_ast_chunk("hasGoodChatCoverageFromRollups")`
- `get_ast_chunk("SyncProgressPanel")`

## Checks

```sh
go test ./internal/analytics/...
cd frontend && npm run build
make scraper-check
```

For scraper or public/tunnel changes, also run `make security-scan`.
