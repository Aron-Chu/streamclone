import { defineConfig } from '@playwright/test'

/**
 * Playwright config for visual regression tests in tests/playwright/.
 * Uses route mocking so no live backend is strictly required,
 * but the frontend dev server must be running at baseURL.
 */
export default defineConfig({
  testDir: '.',
  timeout: 60_000,
  retries: 0,
  use: {
    baseURL: 'http://localhost:8090',
    viewport: { width: 1920, height: 1080 },
    deviceScaleFactor: 1,
    screenshot: 'off',
    trace: 'retain-on-failure',
  },
  expect: {
    toHaveScreenshot: {
      maxDiffPixelRatio: 0.001,
    },
  },
  snapshotDir: './snapshots',
  snapshotPathTemplate: '{snapshotDir}/{testFilePath}/{arg}{ext}',
})
