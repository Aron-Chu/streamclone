# Test Fixtures

## heatmap-visual-regression.json

Deterministic mock data for the Playwright visual acceptance test (task 20.2). Provides seeded API responses so the test does not depend on a live `localhost:8090` stream.

### Structure

```json
{
  "streamDetail": { /* AnalyticsStreamDetail response */ },
  "replayHeatmap": { /* HeatmapResponse with scored points */ }
}
```

### Contents

- **streamDetail**: A full `GET /v1/analytics/streams/{streamId}` response simulating the Caedrel LEC Watch Party stream (72 minutes, ~185k chat messages). Includes 72 minute-level rollups with realistic viewer/chat/emote spikes (peak ~68k viewers at minute 29).

- **replayHeatmap**: A `GET /v1/analytics/streams/{streamId}/replay-heatmap` response with 17 scored points (non-zero scores only, per the zero-score omission rule). Scores range from 28 to 100, with the global peak at offset 1740s (minute 29, matching the viewer/chat/emote peak in rollups).

### Key properties

| Property | Value |
|----------|-------|
| Stream ID | `test-visual-regression-stream` |
| VOD ID | `2185432100` |
| Channel | `caedrel` |
| Duration | 72 minutes (4320 seconds) |
| Rollup count | 72 (one per minute) |
| Heatmap points | 17 (scores 28–100) |
| Peak score | 100 at offset 1740s |
| Scoring version | `v1` |
| Overall confidence | 0.91 |
| Peaks with topEmotes | All 17 points have topEmotes (3+ have 3 emotes) |
| Emote imageUrl format | `/emotes/{id}/1x.webp` |

### Reason labels used

- `chat_spike` (8 points)
- `seventv_spike` (6 points)
- `viewer_spike` (3 points)

### Usage

Import in Playwright tests to mock API responses:

```typescript
import fixture from '../fixtures/heatmap-visual-regression.json';

// Route mock for stream detail
await page.route('**/v1/analytics/streams/test-visual-regression-stream*', route => {
  route.fulfill({ json: fixture.streamDetail });
});

// Route mock for heatmap
await page.route('**/v1/analytics/streams/test-visual-regression-stream/replay-heatmap*', route => {
  route.fulfill({ json: fixture.replayHeatmap });
});
```
