import { test, expect } from '@playwright/test'
import { waitForDirectoryReady, pickLiveChannelLink } from './screenshot-helpers'
import { ONBOARDING_DISMISSED_KEY } from '../src/onboardingStorage'

test('home directory loads', async ({ page }) => {
  await page.addInitScript((key) => {
    window.localStorage.setItem(key, '1')
  }, ONBOARDING_DISMISSED_KEY)
  await waitForDirectoryReady(page)
  await expect(page.getByRole('heading', { name: 'Recommended Live Channels' })).toBeVisible()
  await expect(page.locator('a[href^="/c/"]').first()).toBeVisible()
})

test('welcome overlay appears on first visit and dismisses', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('button', { name: 'Browse live streams' })).toBeVisible({ timeout: 30_000 })
  await expect(page.locator('a[href^="/c/"]').first()).toBeVisible()

  await page.getByRole('button', { name: 'Not now' }).click()
  await expect(page.getByRole('button', { name: 'Browse live streams' })).toBeHidden()
  await expect(page.evaluate((key) => window.localStorage.getItem(key), ONBOARDING_DISMISSED_KEY)).resolves.toBe('1')

  await page.reload()
  await expect(page.getByRole('button', { name: 'Browse live streams' })).toBeHidden()
  await expect(page.locator('a[href^="/c/"]').first()).toBeVisible()
})

test('/welcome redirect lands on usable directory when onboarding is already dismissed', async ({ page }) => {
  await page.addInitScript((key) => {
    window.localStorage.setItem(key, '1')
  }, ONBOARDING_DISMISSED_KEY)

  await page.goto('/welcome')
  await page.waitForURL('/')
  await expect(page.getByRole('button', { name: 'Browse live streams' })).toBeHidden()
  await expect(page.getByRole('heading', { name: 'Recommended Live Channels' })).toBeVisible({ timeout: 30_000 })
  await expect(page.locator('a[href^="/c/"]').first()).toBeVisible()
})

test('channel route loads player shell or structured offline state', async ({ page }) => {
  test.setTimeout(120_000)
  await page.addInitScript((key) => {
    window.localStorage.setItem(key, '1')
  }, ONBOARDING_DISMISSED_KEY)
  await waitForDirectoryReady(page)

  const streamLink = await pickLiveChannelLink(page)
  const href = await streamLink.getAttribute('href')
  const login = href?.replace(/^\/c\//, '').split('/')[0] ?? 'xqc'

  // Navigate directly — stack-status overlay can intercept card clicks after onboarding is dismissed.
  await page.goto(href ?? `/c/${login}`)
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

test('removed analytics routes redirect to directory', async ({ page }) => {
  await page.goto('/analytics/xqc')
  await expect(page).toHaveURL(/\/(\?.*)?$/)
})
