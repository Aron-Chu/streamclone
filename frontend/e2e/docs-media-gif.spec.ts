import { test } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  captureSyncGif,
  channelLogin,
  fetchTargetStreamId,
  imagesDir,
  openAnalyticsStream,
  prepareScreenshotViewport,
} from './docs-media-helpers'

const here = path.dirname(fileURLToPath(import.meta.url))
const outDir = imagesDir(here)
const framesDir = path.join(outDir, '.sync-frames')

test('capture analytics sync gif', async ({ page }) => {
  test.setTimeout(120_000)
  await prepareScreenshotViewport(page)
  const target = await fetchTargetStreamId()
  await openAnalyticsStream(page, target.slug)
  await captureSyncGif(page, outDir, framesDir)
})
