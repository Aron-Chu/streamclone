import { test } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  captureAnalyticsChart,
  captureAnalyticsStreamDetail,
  captureAnalyticsStreamsList,
  captureChannel,
  captureChannelLivePlayback,
  captureChannelTab,
  captureDirectory,
  captureDirectoryCategory,
  captureDirectorySearch,
  captureSyncGif,
  channelLogin,
  fetchTargetStreamId,
  getSyncStatus,
  imagesDir,
  openAnalyticsStream,
  pollSyncStatus,
  skipSync,
  startSync,
  syncIsActive,
} from './docs-media-helpers'

const here = path.dirname(fileURLToPath(import.meta.url))
const outDir = imagesDir(here)
const framesDir = path.join(outDir, '.sync-frames')

test('capture docs media', async ({ page }) => {
  test.setTimeout(skipSync ? 300_000 : 1_800_000)

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

  const target = await fetchTargetStreamId()
  await captureAnalyticsStreamsList(page, outDir)
  await openAnalyticsStream(page, target.slug)

  let active = syncIsActive(await getSyncStatus(target.streamId))
  if (!skipSync && !active) {
    await startSync(target.streamId, target.vodId)
    active = true
  }

  if (active) {
    await captureSyncGif(page, outDir, framesDir)
    const finalStatus = await pollSyncStatus(target.streamId, 1_200_000)
    if (finalStatus.phase === 'failed') {
      throw new Error(`Analytics sync failed: ${finalStatus.error ?? 'unknown error'}`)
    }
    await page.reload()
    await openAnalyticsStream(page, target.slug)
  }

  await captureAnalyticsChart(page, outDir)
  await captureAnalyticsStreamDetail(page, target.slug, outDir)
})
