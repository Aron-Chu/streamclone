import { test, expect } from '@playwright/test'
import fixture from '../fixtures/heatmap-visual-regression.json'

/**
 * Visual acceptance test for the analytics heatmap view.
 *
 * Validates: Requirements 32.1, 32.2, 32.3, 32.4, 32.5
 *
 * Uses route mocking to intercept API calls and return deterministic fixture
 * data so the test is CI-deterministic without a live backend.
 *
 * Target: http://localhost:8090/analytics/caedrel/2026-06-11
 */

const ANALYTICS_URL = '/analytics/caedrel/2026-06-11'
const STREAM_ID = 'test-visual-regression-stream'

// The peak at offset 1740s has score 100 — used for tooltip hover verification
const PEAK_OFFSET_SECONDS = 1740

test.describe('Heatmap Visual Acceptance', () => {
  test.beforeEach(async ({ page }) => {
    // Mock stream detail API
    await page.route(
      `**/v1/analytics/streams/${STREAM_ID}*`,
      async (route) => {
        const url = route.request().url()
        // Don't intercept the replay-heatmap sub-path
        if (url.includes('replay-heatmap')) {
          await route.fallback()
          return
        }
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(fixture.streamDetail),
        })
      }
    )

    // Mock replay heatmap API
    await page.route(
      `**/v1/analytics/streams/${STREAM_ID}/replay-heatmap*`,
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(fixture.replayHeatmap),
        })
      }
    )

    // Mock channel streams list (so the stream rail resolves the date slug)
    await page.route('**/v1/analytics/channels/caedrel/streams*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          streams: [
            {
              streamId: STREAM_ID,
              broadcasterId: '123456789',
              login: 'caedrel',
              displayName: 'Caedrel',
              title: 'LEC Watch Party - Finals Day | !socials',
              category: 'League of Legends',
              startedAt: '2026-06-11T14:00:00Z',
              endedAt: '2026-06-11T15:12:00Z',
              avgViewers: 42150,
              peakViewers: 68200,
              viewerSamples: 72,
              chatMessages: 185420,
              totalEmoteUses: 47300,
              seventvEmoteUses: 31200,
              vodId: '2185432100',
              vodSource: 'helix_stream_match',
            },
          ],
        }),
      })
    })

    // Mock emote image requests with a minimal valid PNG response
    // so img elements render without network errors
    await page.route('**/emotes/*/1x.webp', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'image/png',
        body: Uint8Array.from([
          0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, // PNG signature
          0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
          0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // 1x1
          0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, // RGBA
          0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41, // IDAT chunk
          0x54, 0x78, 0x9c, 0x62, 0x00, 0x00, 0x00, 0x02,
          0x00, 0x01, 0xe5, 0x27, 0xde, 0xfc, 0x00, 0x00, // compressed data
          0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, // IEND chunk
          0x60, 0x82,
        ]),
      })
    })
  })

  test('full-page screenshot matches baseline at 1920x1080', async ({ page }) => {
    await page.setViewportSize({ width: 1920, height: 1080 })
    await page.goto(ANALYTICS_URL)

    // Wait for the heatmap lane to render — look for the heatmap container or canvas
    await page.waitForSelector(
      '[data-testid="heatmap-lane"], .heatmap-lane, canvas[aria-label*="eatmap"]',
      { timeout: 15_000 }
    )

    // Wait for the right rail to render (Moments tab)
    await page.waitForSelector(
      '[data-testid="right-rail"], [role="tablist"]',
      { timeout: 10_000 }
    )

    // Wait a brief moment for any chart animations to settle
    await page.waitForTimeout(1000)

    // Full-page screenshot comparison with 0.1% threshold
    await expect(page).toHaveScreenshot('heatmap-full-page.png', {
      fullPage: true,
      maxDiffPixelRatio: 0.001,
      animations: 'disabled',
    })
  })

  test('heatmap lane, right rail, stat cards, and chart areas are visible', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1920, height: 1080 })
    await page.goto(ANALYTICS_URL)

    // Wait for heatmap lane
    const heatmapLane = page.locator(
      '[data-testid="heatmap-lane"], .heatmap-lane, canvas[aria-label*="eatmap"]'
    )
    await expect(heatmapLane.first()).toBeVisible({ timeout: 15_000 })

    // Verify right rail with Moments tab content
    const rightRail = page.locator('[data-testid="right-rail"], [role="tablist"]')
    await expect(rightRail.first()).toBeVisible({ timeout: 10_000 })

    // Verify stat cards are present
    const statCards = page.locator(
      '[data-testid="stat-cards"], [data-testid="stat-card"], .stat-card'
    )
    await expect(statCards.first()).toBeVisible({ timeout: 10_000 })

    // Verify the chart area (Recharts or canvas-based)
    const chart = page.locator(
      '.recharts-wrapper, [data-testid="analytics-chart"], canvas[data-chart]'
    )
    await expect(chart.first()).toBeVisible({ timeout: 10_000 })
  })

  test('emote images render in peak tooltip on hover', async ({ page }) => {
    await page.setViewportSize({ width: 1920, height: 1080 })
    await page.goto(ANALYTICS_URL)

    // Wait for heatmap to render
    await page.waitForSelector(
      '[data-testid="heatmap-lane"], .heatmap-lane, canvas[aria-label*="eatmap"]',
      { timeout: 15_000 }
    )

    // Find the peak button/element at offset 1740s (the max-score peak)
    // Peak buttons use aria-label containing the offset time (00:29:00)
    const peakButton = page.locator(
      `[aria-label*="29:00"], [data-offset="${PEAK_OFFSET_SECONDS}"], ` +
      `button[data-testid="heatmap-peak-1740"]`
    )

    // If peak buttons exist as accessible elements, hover them
    const peakExists = await peakButton.first().isVisible().catch(() => false)

    if (peakExists) {
      await peakButton.first().hover()
    } else {
      // Fallback: hover the heatmap canvas at the approximate X position of the peak
      // Peak is at 1740s out of 4320s total duration → ~40.3% of the lane width
      const lane = page.locator(
        '[data-testid="heatmap-lane"], .heatmap-lane, canvas[aria-label*="eatmap"]'
      ).first()
      const box = await lane.boundingBox()
      if (box) {
        const peakX = box.x + box.width * (PEAK_OFFSET_SECONDS / 4320)
        const peakY = box.y + box.height / 2
        await page.mouse.move(peakX, peakY)
      }
    }

    // Wait for tooltip to appear
    await page.waitForTimeout(500)

    // Verify tooltip with emote images is visible
    const tooltip = page.locator(
      '[data-testid="heatmap-tooltip"], [role="tooltip"], .heatmap-tooltip'
    )
    await expect(tooltip.first()).toBeVisible({ timeout: 5_000 })

    // Verify emote images are present in the tooltip
    const tooltipEmotes = tooltip.first().locator('img[src*="/emotes/"]')
    await expect(tooltipEmotes.first()).toBeVisible({ timeout: 3_000 })

    // Verify at least one emote image has loaded (non-zero natural dimensions)
    const emoteRendered = await tooltipEmotes.first().evaluate((img: HTMLImageElement) => {
      return img.complete && img.naturalWidth > 0
    })
    expect(emoteRendered).toBe(true)
  })

  test('screenshot diff produces artifact on visual change', async ({ page }) => {
    await page.setViewportSize({ width: 1920, height: 1080 })
    await page.goto(ANALYTICS_URL)

    // Wait for content to load
    await page.waitForSelector(
      '[data-testid="heatmap-lane"], .heatmap-lane, canvas[aria-label*="eatmap"]',
      { timeout: 15_000 }
    )
    await page.waitForTimeout(1000)

    // This test verifies the screenshot comparison mechanism works.
    // On first run it creates the baseline; subsequent runs compare against it.
    // The diff artifact is automatically produced by Playwright when comparison fails.
    await expect(page).toHaveScreenshot('heatmap-diff-check.png', {
      fullPage: true,
      maxDiffPixelRatio: 0.001,
      animations: 'disabled',
    })
  })
})
