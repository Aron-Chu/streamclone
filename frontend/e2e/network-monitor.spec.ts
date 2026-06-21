import { test, expect } from '@playwright/test'
import { ONBOARDING_DISMISSED_KEY } from '../src/onboardingStorage'

test.describe('network monitor', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript((key) => {
      window.localStorage.setItem(key, '1')
    }, ONBOARDING_DISMISSED_KEY)
  })

  test('dual bandwidth charts render on /network', async ({ page }) => {
    test.setTimeout(90_000)
    await page.goto('/network')
    await expect(page.getByRole('heading', { name: 'Network activity monitor' })).toBeVisible({ timeout: 60_000 })
    await expect(page.getByTestId('network-chart-live')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('network-chart-sync')).toBeVisible({ timeout: 30_000 })
  })
})
