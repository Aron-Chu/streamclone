# README screenshots

Documentation screenshots live in [`docs/images/`](images/) and appear in the root [README.md](../README.md).

## Regenerate

1. Start the stack:

   ```sh
   make up
   ```

2. Capture screenshots (requires Node.js and Playwright Chromium):

   ```sh
   make docs-screenshots
   ```

This runs a Playwright test that:

1. Opens the live directory at `http://localhost:8090/`, waits for `/v1/streams` + `/v1/categories`, skeleton cards to disappear, and stream thumbnails to finish loading, then saves `docs/images/directory.png`.
2. Clicks the first **Live** stream card (or the first card if none are live), waits for channel details + HLS relay start, **playback = playing**, decoded video frames, and chat connection, then saves `docs/images/channel.png`.

Helpers live in `frontend/e2e/screenshot-helpers.ts`. The test timeout is **3 minutes** (channel relay + first frame can take ~60–90s).

Viewport size is 1280×800.

## Files

| File | View |
|---|---|
| `docs/images/directory.png` | Live channel directory |
| `docs/images/channel.png` | Channel workspace (player + chat) |

## Troubleshooting

- **`ECONNREFUSED` on 8090** — run `make up` and wait for containers to become healthy.
- **Empty directory grid** — metadata returned no streams; confirm Twitch/Helix connectivity and try again.
- **Channel offline** — the script requires a **Live** card for `channel.png`; start `make up`, wait for healthy containers, then re-run.
- **Timeout on channel.png** — relay or first frame exceeded 3 minutes; check `docker compose logs video mediamtx` and open the same channel manually at `:8090`.
- **Playwright browser missing** — `make docs-screenshots` runs `npx playwright install chromium` automatically.
- **Windows `node_modules` shims** — run from WSL or reinstall frontend deps with `npm ci` in `frontend/`.

Source: [`frontend/e2e/readme-screenshots.spec.ts`](../frontend/e2e/readme-screenshots.spec.ts).
