import { test } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  captureAnalyticsGifs,
  captureChannelGif,
  captureDirectoryGif,
  imagesDir,
  skipSync,
} from './docs-media-helpers'

const here = path.dirname(fileURLToPath(import.meta.url))
const outDir = imagesDir(here)

test('capture readme gifs', async ({ page }) => {
  test.setTimeout(skipSync ? 300_000 : 1_200_000)

  await captureDirectoryGif(page, outDir)
  await captureChannelGif(page, outDir)

  if (!skipSync) {
    await captureAnalyticsGifs(page, outDir)
  }
})
