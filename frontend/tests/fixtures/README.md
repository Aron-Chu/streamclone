# Frontend Fixtures

`heatmap-visual-regression.json` is deterministic mock Analytics data for Playwright visual tests.

It includes:

- stream detail response
- replay heatmap response
- 72 minute rollups
- 17 scored heatmap points
- top emote metadata

Use it to mock:

- `GET /v1/analytics/streams/{streamId}`
- `GET /v1/analytics/streams/{streamId}/replay-heatmap`

Keep fixture IDs stable unless the matching tests are updated.
