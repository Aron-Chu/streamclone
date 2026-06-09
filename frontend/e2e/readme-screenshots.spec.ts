import { test } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  pickLiveChannelLink,
  settleBeforeScreenshot,
  watchChannelNavigation,
  waitForDirectoryReady,
} from './screenshot-helpers'

const here = path.dirname(fileURLToPath(import.meta.url))
const imagesDir = path.resolve(here, '../../docs/images')

test('capture README screenshots', async ({ page }) => {
  test.setTimeout(180_000)

  await waitForDirectoryReady(page)
  await settleBeforeScreenshot(page)

  await page.screenshot({
    path: path.join(imagesDir, 'directory.png'),
    fullPage: false,
    animations: 'disabled',
  })

  const streamLink = await pickLiveChannelLink(page)
  const href = await streamLink.getAttribute('href')
  const login = href?.replace(/^\/c\//, '').split('/')[0] ?? 'sodapoppin'

  const finishChannelReady = watchChannelNavigation(page, login)
  await streamLink.click()
  await page.waitForURL(/\/c\//, { timeout: 30_000 })
  await finishChannelReady()
  await settleBeforeScreenshot(page)

  await page.screenshot({
    path: path.join(imagesDir, 'channel.png'),
    fullPage: false,
    animations: 'disabled',
  })
})
