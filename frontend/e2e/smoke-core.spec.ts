import { test, expect } from '@playwright/test'
import { waitForDirectoryReady, pickLiveChannelLink } from './screenshot-helpers'

test('home directory loads', async ({ page }) => {
  await waitForDirectoryReady(page)
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
  await expect(page.locator('a[href^="/c/"]').first()).toBeVisible()
})

test('channel route loads player shell or structured offline state', async ({ page }) => {
  test.setTimeout(120_000)
  await waitForDirectoryReady(page)

  const streamLink = await pickLiveChannelLink(page)
  const href = await streamLink.getAttribute('href')
  const login = href?.replace(/^\/c\//, '').split('/')[0] ?? 'xqc'

  await streamLink.click()
  await page.waitForURL(new RegExp(`/c/${login}`), { timeout: 30_000 })

  const offline = page.getByText('Offline', { exact: true })
  const playing = page.locator('header span').filter({ hasText: /^playing$/i })
  const errorBanner = page.getByText(/failed|error|unavailable/i)

  await expect(async () => {
    const isOffline = await offline.isVisible().catch(() => false)
    const isPlaying = await playing.first().isVisible().catch(() => false)
    const hasError = await errorBanner.first().isVisible().catch(() => false)
    expect(isOffline || isPlaying || hasError).toBeTruthy()
  }).toPass({ timeout: 60_000 })
})

test('analytics page shell loads', async ({ page }) => {
  test.setTimeout(90_000)
  await page.goto('/analytics/xqc')
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible({ timeout: 60_000 })
})
