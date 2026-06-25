# Streamclone Pulse — Live coverage requirements (redirect)

Canonical spec lives in the **streamclone-pulse** repo (extension + portal docs):

**[streamclone-pulse `docs/pulse-extension/live-coverage-requirements.md`](../../streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md)**

Local path (multi-root / sibling checkout):

```text
../streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md
```

GitHub:

```text
https://github.com/Aron-Chu/streamclone-pulse/blob/master/docs/pulse-extension/live-coverage-requirements.md
```

Agents (Cursor, Codex): read the canonical file before changing coverage, backfill, Protect channel, or BearHost pulse BFF behavior.

Related in this repo:

- `internal/analytics/pulse_coverage.go`, `pulse_backfill.go`, `extension_api.go`
- [`docs/roadmapping.md`](../roadmapping.md) — phased delivery R0–R6
- [`docs/tools.md`](../tools.md) — ingest source tiers
- [`docs/CODEX.md`](../CODEX.md) — Codex review prompt for live coverage
