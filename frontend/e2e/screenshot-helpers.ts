import type { Locator, Page } from '@playwright/test'

const DEFAULT_TIMEOUT = 90_000

export async function waitForDirectoryReady(page: Page, timeout = DEFAULT_TIMEOUT) {
  await page.goto('/')

  await page.getByRole('heading', { name: 'Recommended Live Channels' }).waitFor({ state: 'visible', timeout })

  await page.waitForFunction(() => {
    const skeletonCards = document.querySelectorAll('.aspect-video.animate-pulse')
    const streamLinks = document.querySelectorAll('a[href^="/c/"]')
    return skeletonCards.length === 0 && streamLinks.length >= 1
  }, { timeout })

  const cardCount = await page.locator('a[href^="/c/"]').count()
  await waitForImagesLoaded(page, 'a[href^="/c/"] img', Math.min(4, Math.max(1, cardCount)), timeout)
}

export async function pickLiveChannelLink(page: Page, timeout = 30_000): Promise<Locator> {
  const liveLink = page.locator('a[href^="/c/"]').filter({
    has: page.getByText('Live', { exact: true }),
  })
  if (await liveLink.count()) {
    await liveLink.first().waitFor({ state: 'visible', timeout })
    return liveLink.first()
  }
  const anyLink = page.locator('a[href^="/c/"]').first()
  await anyLink.waitFor({ state: 'visible', timeout })
  return anyLink
}

/** Register before clicking a channel card so responses are not missed. */
export function watchChannelNavigation(page: Page, login: string, timeout = 120_000) {
  const detailsResponse = page.waitForResponse(
    res => res.url().includes(`/v1/channels/${login}/details`) && isOkJson(res),
    { timeout },
  )
  const streamStartResponse = page.waitForResponse(
    res => res.url().includes('/v1/stream/start') && res.request().method() === 'POST' && res.ok(),
    { timeout },
  ).catch(() => null)

  return async () => {
    await Promise.all([detailsResponse, streamStartResponse])
    await waitForChannelPlayback(page, timeout)
  }
}

export async function waitForChannelPlayback(page: Page, timeout = 120_000) {
  const offline = page.getByText('Offline', { exact: true })
  if (await offline.isVisible().catch(() => false)) {
    throw new Error('Channel is offline — pick a live card for channel.png')
  }

  await page.locator('header span').filter({ hasText: /^playing$/i }).first().waitFor({
    state: 'visible',
    timeout,
  })

  await page.waitForFunction(() => {
    const video = document.querySelector('video')
    if (!video) return false
    return video.readyState >= 3 && !video.paused && video.currentTime > 0 && video.videoWidth > 0
  }, { timeout })

  await page.waitForFunction(() => {
    const pills = Array.from(document.querySelectorAll('span'))
      .map(node => node.textContent?.trim().toLowerCase())
    const chatReady = pills.includes('open') || document.querySelectorAll('.chat-row').length > 0
    const listening = Array.from(document.querySelectorAll('div'))
      .some(node => node.textContent?.includes('Subscribed and listening live'))
    return chatReady || listening
  }, { timeout: 45_000 })
}

async function waitForImagesLoaded(page: Page, selector: string, minCount: number, timeout: number) {
  await page.waitForFunction(({ sel, min }) => {
    const imgs = Array.from(document.querySelectorAll<HTMLImageElement>(sel))
    if (imgs.length < min) return false
    return imgs.slice(0, min).every(img => img.complete && img.naturalWidth > 0)
  }, { sel: selector, min: minCount }, { timeout })
}

export async function settleBeforeScreenshot(page: Page) {
  await page.waitForLoadState('networkidle', { timeout: 15_000 }).catch(() => undefined)
  await page.evaluate(() => new Promise<void>(resolve => {
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
  }))
}
