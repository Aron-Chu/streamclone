import { test, expect } from '@playwright/test'
import { waitForDirectoryReady, pickLiveChannelLink } from './screenshot-helpers'
import { ONBOARDING_DISMISSED_KEY } from '../src/onboardingStorage'

test.describe('chat UX', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript((key) => {
      window.localStorage.setItem(key, '1')
    }, ONBOARDING_DISMISSED_KEY)
  })

  test('scroll-up latches pause until jump to bottom', async ({ page }) => {
    test.setTimeout(120_000)
    await waitForDirectoryReady(page)
    const streamLink = await pickLiveChannelLink(page)
    const href = await streamLink.getAttribute('href')
    await page.goto(href ?? '/c/xqc')
    await page.waitForURL(/\/c\//, { timeout: 30_000 })

    const chatScroll = page.getByTestId('chat-scroll-container')
    await expect(chatScroll).toBeVisible({ timeout: 60_000 })

    await expect(async () => {
      const rowCount = await page.locator('.chat-row').count()
      expect(rowCount).toBeGreaterThan(0)
    }).toPass({ timeout: 60_000 })

    await chatScroll.evaluate(el => {
      el.scrollTop = Math.max(0, el.scrollHeight - el.clientHeight - 120)
    })
    await chatScroll.dispatchEvent('wheel', { deltaY: -120 })

    await expect(page.getByTestId('chat-jump-bottom')).toBeVisible({ timeout: 10_000 })

    await page.getByTestId('chat-jump-bottom').click()
    await expect(page.getByTestId('chat-jump-bottom')).toBeHidden({ timeout: 10_000 })
  })

  test('username opens twitch-like user card', async ({ page }) => {
    test.setTimeout(120_000)
    await waitForDirectoryReady(page)
    const streamLink = await pickLiveChannelLink(page)
    const href = await streamLink.getAttribute('href')
    await page.goto(href ?? '/c/xqc')
    await page.waitForURL(/\/c\//, { timeout: 30_000 })

    const chatScroll = page.getByTestId('chat-scroll-container')
    await expect(chatScroll).toBeVisible({ timeout: 60_000 })

    const usernameButton = page.locator('.chat-row button').first()
    await expect(usernameButton).toBeVisible({ timeout: 60_000 })
    await usernameButton.click()

    const card = page.getByTestId('chat-user-card')
    await expect(card).toBeVisible()
    await expect(card.getByRole('menuitem', { name: 'Open on Twitch' })).toBeVisible()
    await expect(card.getByRole('menuitem', { name: 'Copy login' }).or(card.getByRole('menuitem', { name: 'Copy name' }))).toBeVisible()
  })
})
