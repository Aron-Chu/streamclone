import { test } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { captureAnalyticsGifs, imagesDir } from './docs-media-helpers'

const here = path.dirname(fileURLToPath(import.meta.url))
const outDir = imagesDir(here)

test('capture analytics load gifs', async ({ page }) => {
  test.setTimeout(1_200_000)
  await captureAnalyticsGifs(page, outDir)
})
