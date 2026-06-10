import { test } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  captureAnalyticsMedia,
  captureChannel,
  captureChannelLivePlayback,
  captureChannelTab,
  captureDirectory,
  captureDirectoryCategory,
  captureDirectorySearch,
  channelLogin,
  imagesDir,
  skipSync,
} from './docs-media-helpers'

const here = path.dirname(fileURLToPath(import.meta.url))
const outDir = imagesDir(here)

test('capture docs media', async ({ page }) => {
  test.setTimeout(skipSync ? 300_000 : 900_000)

  await captureDirectory(page, outDir)
  await captureDirectoryCategory(page, outDir)
  await captureDirectorySearch(page, outDir)
  await captureChannel(page, outDir)
  await captureChannelLivePlayback(page, outDir).catch(() => undefined)

  const channelPath = `/c/${encodeURIComponent(channelLogin)}`
  await page.goto(channelPath)
  await page.waitForURL(/\/c\//, { timeout: 60_000 })
  await captureChannelTab(page, 'emotes', outDir, 'channel-emotes.png').catch(() => undefined)
  await page.goto(channelPath)
  await captureChannelTab(page, 'stats', outDir, 'channel-stats.png').catch(() => undefined)

  if (!skipSync) {
    await captureAnalyticsMedia(page, outDir)
  }
})
