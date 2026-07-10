import { test } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  captureChannelGif,
  captureDirectoryGif,
  imagesDir,
} from './docs-media-helpers'

const here = path.dirname(fileURLToPath(import.meta.url))
const outDir = imagesDir(here)

test('capture readme gifs', async ({ page }) => {
  test.setTimeout(300_000)

  await captureDirectoryGif(page, outDir)
  await captureChannelGif(page, outDir)
})
