import { test, expect, type Locator, type Page } from '@playwright/test'

/**
 * Pulse Wire page smoke — validates edition KPIs, feed load, and rank display.
 * Requires stack at http://localhost:8090 with PULSE_WIRE_ENABLED=true.
 */

const PULSE_URL = '/pulse-wire'

async function expectVerticalOrder(locators: Locator[]) {
  const boxes = await Promise.all(locators.map(locator => locator.boundingBox()))
  const missingIndex = boxes.findIndex(box => !box)
  expect(missingIndex, 'all ordered sections should be visible').toBe(-1)
  const positions = boxes.map(box => box?.y ?? 0)
  for (let i = 1; i < positions.length; i++) {
    expect(positions[i]).toBeGreaterThan(positions[i - 1])
  }
}

async function routeWireLayoutFixture(page: Page) {
  const now = '2026-06-17T20:00:00Z'
  const story = {
    story: {
      id: 905,
      title: 'Reddit thread spreads to YouTube while origin is pending',
      state: 'published',
      category: 'drama',
      updatedAt: now,
    },
    entity: { login: 'caseoh', displayName: 'CaseOh' },
    scores: { trend: null, volatility: null, confidence: 'corroborated', sentiment: null },
    windowScores: { sourceCount: 2, evidenceCount: 3, rankScore: 74, velocityScore: 62 },
    windowReceipts: [
      { sourceType: 'reddit_thread', pct: 88, label: 'LSF thread', url: 'https://reddit.com/r/LivestreamFail/example' },
      { sourceType: 'youtube_video', pct: 65, label: 'YouTube repost', url: 'https://youtube.com/watch?v=abc' },
    ],
    windowTimeline: [
      { at: now, sourceType: 'reddit_thread', label: 'LSF pickup' },
      { at: now, sourceType: 'youtube_video', label: 'YouTube repost' },
    ],
    evidenceGallery: [],
    matchExplanation: [],
    tracked: false,
  }

  await page.route('**/v1/pulse-wire/feed**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [story], window: '24h', since: now, sort: 'rank', rankModel: 'window-native-v1' }),
  }))
  await page.route('**/v1/pulse-wire/developing**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [] }),
  }))
  await page.route('**/v1/pulse-wire/source-health**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      sources: {
        instagram: { mode: 'link_only', healthy: true, hint: 'Instagram is link-only evidence.' },
        kick: { mode: 'deferred', healthy: false, hint: 'Kick discovery is planned after Evidence Gallery proves useful.' },
        reddit: { mode: 'degraded', healthy: false, last_error: 'BrowserContext.new_page failed' },
        streamerbans: { mode: 'active', healthy: true },
        tiktok: { mode: 'link_only', healthy: true, hint: 'TikTok links render as evidence previews; no direct discovery source is enabled.' },
        twitchclips: { mode: 'active', healthy: true },
        x: { mode: 'link_only', healthy: true, hint: 'X appears through extracted links/oEmbed unless optional x-ingest is enabled.' },
        youtube: { mode: 'error', healthy: false, last_error: 'no videos in ytInitialData' },
      },
      directorySampler: {},
      windowScoreCompute: {},
    }),
  }))
  await page.route('**/v1/pulse-wire/trending-streamers**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [], window: '24h', since: now }),
  }))
  await page.route('**/v1/pulse-wire/bans**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [], window: '24h', since: now }),
  }))
  await page.route('**/v1/pulse-wire/evidence/unlinked**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [], window: '24h', since: now }),
  }))
}

async function routeWireWarmupFixture(page: Page) {
  const now = '2026-06-17T20:00:00Z'

  await page.route('**/v1/pulse-wire/feed**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [], window: '24h', since: now, sort: 'rank', rankModel: 'window-native-v1' }),
  }))
  await page.route('**/v1/pulse-wire/developing**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [] }),
  }))
  await page.route('**/v1/pulse-wire/source-health**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      sources: {
        reddit: { mode: 'active', healthy: true, last_items: 0 },
        youtube: { mode: 'degraded', healthy: false, last_error: 'no videos in ytInitialData' },
        x: { mode: 'link_only', healthy: true, hint: 'X appears through extracted links/oEmbed unless optional x-ingest is enabled.' },
        tiktok: { mode: 'link_only', healthy: true, hint: 'TikTok links render as evidence previews; no direct discovery source is enabled.' },
      },
      directorySampler: { healthy: true, historyGathering: true },
      windowScoreCompute: { healthy: true },
    }),
  }))
  await page.route('**/v1/pulse-wire/trending-streamers**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [], window: '24h', since: now }),
  }))
  await page.route('**/v1/pulse-wire/bans**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [], window: '24h', since: now }),
  }))
  await page.route('**/v1/pulse-wire/evidence/unlinked**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [], window: '24h', since: now }),
  }))
}

test.describe('Pulse Wire', () => {
  test.beforeEach(async ({ page }) => {
    await page.route('**/v1/pulse-wire/watch-entries', route => {
      if (route.request().method() !== 'GET') {
        return route.fallback()
      }
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ items: [] }),
      })
    })
    await page.goto(PULSE_URL)
    await page.waitForLoadState('networkidle')
  })

  test('loads trending tab by default without legacy sub-tabs', async ({ page }) => {
    const disabled = page.getByText(/Pulse Wire is disabled/i)
    if (await disabled.isVisible().catch(() => false)) {
      test.skip(true, 'Pulse Wire disabled on this install')
    }

    await expect(page.getByText('Pulse Wire', { exact: true }).first()).toBeVisible()
    await expect(page.getByRole('tab', { name: 'Trending' })).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByRole('button', { name: 'Clips', exact: true })).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Drama', exact: true })).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Funny', exact: true })).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Bans', exact: true })).toHaveCount(0)
    await expect(page.getByRole('heading', { name: /More community threads|Top clips|Hot on Reddit|Top clip/i }).first()).toBeVisible()
  })

  test('wire tab loads clustered story feed', async ({ page }) => {
    const disabled = page.getByText(/Pulse Wire is disabled/i)
    if (await disabled.isVisible().catch(() => false)) {
      test.skip(true, 'Pulse Wire disabled on this install')
    }

    await page.getByRole('tab', { name: /Cross-platform/i }).click()
    await expect(page.getByRole('tab', { name: /Cross-platform/i })).toHaveAttribute('aria-selected', 'true')

    await expect(page.getByText('Lead story')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByRole('heading', { name: /Developing now|Confirmed across sources|Needs origin/i })).toBeVisible({ timeout: 45_000 })
    await expect(page.getByRole('heading', { name: 'Source health' })).toBeVisible()
  })

  test('trending tab renders community cards when API has items', async ({ page, request }) => {
    const communityRes = await request.get('/v1/pulse-wire/community?window=24h&sort=hot&limit=5')
    if (communityRes.status() === 503) {
      test.skip(true, 'Pulse Wire API disabled')
    }
    expect(communityRes.ok()).toBeTruthy()
    const community = await communityRes.json() as { items?: Array<{ title?: string; score?: number }> }
    if (!community.items?.length) {
      test.skip(true, 'No community items ingested yet')
    }

    await page.goto(PULSE_URL)
    await page.waitForLoadState('networkidle')
    const first = community.items[0]
    await expect(page.getByText(first.title ?? '', { exact: false }).first()).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText(/upvotes/i).first()).toBeVisible()
  })

  test('community API avoids comment-count placeholder titles', async ({ request }) => {
    const res = await request.get('/v1/pulse-wire/community?window=24h&sort=hot&limit=10')
    if (res.status() === 503) {
      test.skip(true, 'Pulse Wire API disabled')
    }
    expect(res.ok()).toBeTruthy()
    const body = await res.json() as { items?: Array<{ title?: string }> }
    for (const item of body.items ?? []) {
      expect(item.title ?? '').not.toMatch(/^\d+ comments?$/i)
    }
  })

  test('wire feed avoids comment-count placeholder lead titles', async ({ request }) => {
    const res = await request.get('/v1/pulse-wire/feed?window=24h&sort=rank&limit=10')
    if (res.status() === 503) {
      test.skip(true, 'Pulse Wire API disabled')
    }
    expect(res.ok()).toBeTruthy()
    const body = await res.json() as { items?: Array<{ story?: { title?: string } }> }
    if (!(body.items?.length)) {
      test.skip(true, 'No wire stories ingested yet')
    }
    for (const item of body.items ?? []) {
      expect(item.story?.title ?? '').not.toMatch(/^\d+ comments?$/i)
    }
  })

  test('community API returns hot-sorted items', async ({ request }) => {
    const res = await request.get('/v1/pulse-wire/community?window=24h&sort=hot&limit=5')
    if (res.status() === 503) {
      test.skip(true, 'Pulse Wire API disabled')
    }
    expect(res.ok()).toBeTruthy()
    const body = await res.json()
    expect(body).toHaveProperty('items')
    expect(Array.isArray(body.items)).toBeTruthy()
    expect(body.sort).toBe('hot')
  })

  test('community API avoids fake twitch CDN slugs from reddit ids', async ({ request }) => {
    const res = await request.get('/v1/pulse-wire/community?window=24h&sort=hot&limit=10')
    if (res.status() === 503) {
      test.skip(true, 'Pulse Wire API disabled')
    }
    expect(res.ok()).toBeTruthy()
    const body = await res.json() as { items?: Array<{ externalId?: string; thumbnailUrl?: string; displayThumbnailUrl?: string; previewKind?: string }> }
    for (const item of body.items ?? []) {
      const externalId = item.externalId ?? ''
      const thumb = item.thumbnailUrl ?? ''
      const display = item.displayThumbnailUrl ?? ''
      if (externalId && thumb.includes(`${externalId}-preview`)) {
        throw new Error(`fake twitch thumb for reddit id ${externalId}: ${thumb}`)
      }
      if (item.previewKind === 'none') {
        expect(thumb).toBe('')
        expect(display).toBe('')
      }
      if (display && (display.includes('clips-media-assets') || display.includes('redd.it'))) {
        expect(display).toContain('/v1/pulse-wire/thumb?')
      }
    }
  })

  test('reddit cards never use fake twitch CDN slugs in DOM', async ({ page, request }) => {
    const communityRes = await request.get('/v1/pulse-wire/community?window=24h&sort=hot&limit=5')
    if (communityRes.status() === 503) {
      test.skip(true, 'Pulse Wire API disabled')
    }
    const community = await communityRes.json() as { items?: unknown[] }
    if (!community.items?.length) {
      test.skip(true, 'No community items ingested yet')
    }

    await page.goto(`${PULSE_URL}?window=24h`)
    await page.waitForLoadState('networkidle')
    const imgs = page.locator('[data-testid="community-preview"]')
    const count = await imgs.count()
    for (let i = 0; i < count; i++) {
      const src = await imgs.nth(i).getAttribute('src') ?? ''
      expect(src).not.toMatch(/clips-media-assets.*\/1u[a-z0-9]+-preview/)
      if (src.includes('clips-media-assets') || src.includes('redd.it')) {
        expect(src).toContain('/v1/pulse-wire/thumb?')
      }
    }
  })

  test('clips API exposes displayThumbnailUrl for top clips', async ({ request }) => {
    const res = await request.get('/v1/pulse-wire/clips/top?window=24h&limit=3')
    if (res.status() === 503) {
      test.skip(true, 'Pulse Wire API disabled')
    }
    expect(res.ok()).toBeTruthy()
    const body = await res.json() as { items?: Array<{ displayThumbnailUrl?: string; thumbnailUrl?: string }> }
    for (const clip of body.items ?? []) {
      if (clip.displayThumbnailUrl) {
        expect(clip.displayThumbnailUrl.length).toBeGreaterThan(0)
        if (clip.displayThumbnailUrl.includes('clips-media-assets') || clip.displayThumbnailUrl.includes('jtvnw.net')) {
          expect(clip.displayThumbnailUrl).toContain('/v1/pulse-wire/thumb?')
        }
      }
    }
  })

  test('loaded clip preview images have naturalWidth > 0', async ({ page, request }) => {
    const clipsRes = await request.get('/v1/pulse-wire/clips/top?window=24h&limit=3')
    if (clipsRes.status() === 503) {
      test.skip(true, 'Pulse Wire API disabled')
    }
    const clips = await clipsRes.json() as { items?: unknown[] }
    if (!clips.items?.length) {
      test.skip(true, 'No clips ingested yet')
    }

    await page.goto(`${PULSE_URL}?window=24h`)
    await page.waitForLoadState('networkidle')
    const clipImg = page.locator('[data-testid="clip-thumb"]').first()
    await expect(clipImg).toBeVisible({ timeout: 30_000 })
    const width = await clipImg.evaluate((el: HTMLImageElement) => el.naturalWidth)
    expect(width).toBeGreaterThan(0)
  })

  test('wire shows unlinked fallback when feed empty but community has data', async ({ page, request }) => {
    const feedRes = await request.get('/v1/pulse-wire/feed?window=24h')
    const communityRes = await request.get('/v1/pulse-wire/community?window=24h&limit=1')
    if (feedRes.status() === 503 || communityRes.status() === 503) {
      test.skip(true, 'Pulse Wire API disabled')
    }
    const feed = await feedRes.json() as { items?: unknown[] }
    const community = await communityRes.json() as { items?: unknown[] }
    if ((feed.items?.length ?? 0) > 0 || !(community.items?.length)) {
      test.skip(true, 'Wire feed already populated or community empty')
    }

    await page.goto(`${PULSE_URL}?tab=wire&window=24h`)
    await page.waitForLoadState('networkidle')
    await expect(page.getByRole('heading', { name: 'Unlinked evidence' })).toBeVisible({ timeout: 30_000 })
  })

  test('bans endpoint returns structured items or empty array', async ({ request }) => {
    const res = await request.get('/v1/pulse-wire/bans?window=24h&limit=5')
    if (res.status() === 503) {
      test.skip(true, 'Pulse Wire API disabled')
    }
    expect(res.ok()).toBeTruthy()
    const body = await res.json()
    expect(Array.isArray(body.items)).toBeTruthy()
  })

  test('source-health exposes window score compute status', async ({ request }) => {
    const res = await request.get('/v1/pulse-wire/source-health')
    if (res.status() === 503) {
      test.skip(true, 'Pulse Wire API disabled')
    }
    expect(res.ok()).toBeTruthy()
    const body = await res.json()
    expect(body).toHaveProperty('windowScoreCompute')
    expect(body).toHaveProperty('directorySampler')
  })
})

test.describe('Pulse Wire reader fixtures', () => {
  test('trending shows compact source health when source coverage is degraded', async ({ page }) => {
    await page.route('**/v1/pulse-wire/source-health**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        sources: {
          reddit: { mode: 'degraded', healthy: false, last_error: 'scraper EOF' },
          youtube: { mode: 'degraded', healthy: false, last_error: 'shared scraper unhealthy' },
          twitchclips: { mode: 'active', healthy: true, last_items: 12 },
          streamerbans: { mode: 'off', healthy: false },
          instagram: { mode: 'link_only', healthy: true },
          x: { mode: 'link_only', healthy: true },
          tiktok: { mode: 'link_only', healthy: true },
          kick: { mode: 'deferred', healthy: false },
        },
        directorySampler: { healthy: true },
        windowScoreCompute: { healthy: true },
      }),
    }))
    await page.route('**/v1/pulse-wire/community**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', sort: 'hot' }),
    }))
    await page.route('**/v1/pulse-wire/clips/top**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h' }),
    }))
    await page.route('**/v1/pulse-wire/feed**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: '2026-06-17T20:00:00Z', sort: 'rank', rankModel: 'window-native-v1' }),
    }))
    await page.route('**/v1/pulse-wire/developing**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))
    await page.route('**/v1/pulse-wire/trending-streamers**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h' }),
    }))
    await page.route('**/v1/pulse-wire/bans**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h' }),
    }))
    await page.route('**/v1/pulse-wire/evidence/unlinked**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))
    await page.route('**/v1/pulse-wire/watch-entries**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))

    const healthResponse = page.waitForResponse(response => response.url().includes('/v1/pulse-wire/source-health'))
    await page.goto('/pulse-wire')
    await healthResponse
    const row = page.locator('[data-testid="trending-source-health"]')
    await expect(row).toBeVisible()
    await expect(row).toContainText('Reddit')
    await expect(row).toContainText('degraded')
    await expect(row).toContainText('YouTube')
    await expect(row).toContainText('Clips')
    await expect(row).toContainText('active')
    await expect(row).toContainText('StreamerBans')
    await expect(row).toContainText('off')
  })

  test('trending community cards link to Reddit without Open thread button', async ({ page }) => {
    const now = '2026-06-17T20:00:00Z'
    const threadUrl = 'https://reddit.com/r/LivestreamFail/comments/fixture123/clickable-thread/'

    await page.route('**/v1/pulse-wire/source-health**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        sources: {
          reddit: { mode: 'active', healthy: true },
          twitchclips: { mode: 'active', healthy: true },
          streamerbans: { mode: 'active', healthy: true },
        },
      }),
    }))
    await page.route('**/v1/pulse-wire/community/flairs**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h' }),
    }))
    await page.route('**/v1/pulse-wire/community**', route => {
      if (route.request().url().includes('/community/flairs')) {
        return route.fallback()
      }
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          items: [{
            id: 601,
            title: 'Clickable community card fixture',
            url: threadUrl,
            permalink: threadUrl,
            source: 'reddit',
            subreddit: 'LivestreamFail',
            score: 2400,
            comments: 120,
            displayThumbnailUrl: 'https://i.redd.it/fixture.jpg',
            previewKind: 'reddit',
          }],
          window: '24h',
          sort: 'hot',
        }),
      })
    })
    await page.route('**/v1/pulse-wire/clips/top**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/bans**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/feed**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now, sort: 'rank' }),
    }))
    await page.route('**/v1/pulse-wire/developing**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))
    await page.route('**/v1/pulse-wire/trending-streamers**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/evidence/unlinked**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/watch-entries**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))

    await page.goto('/pulse-wire')
    await expect(page.getByText('Open thread')).toHaveCount(0)
    const lead = page.getByTestId('trending-lead')
    await expect(lead).toBeVisible()
    await expect(lead).toHaveAttribute('href', threadUrl)
    const cards = page.getByTestId('community-post-card')
    await expect(cards.first()).toHaveAttribute('href', threadUrl)
  })

  test('legacy view=bans URL lands on hot trending without view param', async ({ page }) => {
    await page.route('**/v1/pulse-wire/source-health**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        sources: {
          reddit: { mode: 'active', healthy: true },
          twitchclips: { mode: 'active', healthy: true },
          streamerbans: { mode: 'off', healthy: false },
        },
      }),
    }))
    await page.route('**/v1/pulse-wire/community/flairs**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '7d' }),
    }))
    await page.route('**/v1/pulse-wire/community**', route => {
      if (route.request().url().includes('/community/flairs')) {
        return route.fallback()
      }
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ items: [], window: '24h', sort: 'hot' }),
      })
    })
    await page.route('**/v1/pulse-wire/clips/top**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h' }),
    }))
    await page.route('**/v1/pulse-wire/bans**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h' }),
    }))
    await page.route('**/v1/pulse-wire/feed**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', sort: 'rank' }),
    }))
    await page.route('**/v1/pulse-wire/developing**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))
    await page.route('**/v1/pulse-wire/trending-streamers**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h' }),
    }))
    await page.route('**/v1/pulse-wire/evidence/unlinked**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h' }),
    }))
    await page.route('**/v1/pulse-wire/watch-entries**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))

    await page.goto('/pulse-wire?view=bans')
    await expect(page).not.toHaveURL(/view=/)
    await expect(page.getByRole('heading', { name: 'Hot on Reddit' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Recent bans' })).toHaveCount(0)
  })

  test('flair chip filters community feed and sets 7d window', async ({ page }) => {
    const now = '2026-06-17T20:00:00Z'
    let communityQuery = ''

    await page.route('**/v1/pulse-wire/source-health**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        sources: {
          reddit: { mode: 'active', healthy: true },
          twitchclips: { mode: 'active', healthy: true },
          streamerbans: { mode: 'active', healthy: true },
        },
      }),
    }))
    await page.route('**/v1/pulse-wire/community/flairs**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        items: [
          { flair: 'News', count: 42 },
          { flair: 'Meta', count: 18 },
        ],
        window: '7d',
      }),
    }))
    await page.route('**/v1/pulse-wire/community**', route => {
      if (route.request().url().includes('/community/flairs')) {
        return route.fallback()
      }
      communityQuery = route.request().url()
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          items: communityQuery.includes('flair=News')
            ? [{
                id: 501,
                title: 'News flair fixture thread',
                url: 'https://reddit.com/r/LivestreamFail/news-fixture',
                source: 'reddit',
                subreddit: 'LivestreamFail',
                score: 1200,
                comments: 88,
                flair: 'News',
              }]
            : [],
          window: communityQuery.includes('window=7d') ? '7d' : '24h',
          sort: 'hot',
          flair: communityQuery.includes('flair=News') ? 'News' : undefined,
        }),
      })
    })
    await page.route('**/v1/pulse-wire/clips/top**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '7d', since: now }),
    }))
    await page.route('**/v1/pulse-wire/bans**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '7d', since: now }),
    }))
    await page.route('**/v1/pulse-wire/feed**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now, sort: 'rank' }),
    }))
    await page.route('**/v1/pulse-wire/developing**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))
    await page.route('**/v1/pulse-wire/trending-streamers**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/evidence/unlinked**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/watch-entries**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))

    await page.goto('/pulse-wire')
    const chips = page.locator('[data-testid="trending-flair-chips"]')
    await expect(chips.getByRole('button', { name: 'All', exact: true })).toBeVisible()
    await expect(chips.getByRole('button', { name: /News/ })).toBeVisible()

    await chips.getByRole('button', { name: /News/ }).click()
    await expect(page).toHaveURL(/flair=News/)
    await expect(page).toHaveURL(/window=7d/)
    await expect(page.getByRole('heading', { name: 'News on Reddit' })).toBeVisible()
    await expect(page.getByText('News flair fixture thread')).toBeVisible()
    await expect(page.getByTestId('community-flair-badge')).toContainText('News')
    expect(communityQuery).toContain('flair=News')
    expect(communityQuery).toContain('window=7d')
  })

  test('wire reader shows honest empty warmup state without fake stories', async ({ page }) => {
    await routeWireWarmupFixture(page)

    await page.setViewportSize({ width: 1280, height: 850 })
    await page.goto('/pulse-wire?tab=wire&window=24h')

    await expect(page.getByRole('heading', { name: 'Pulse Wire' })).toBeVisible()
    await expect(page.getByText(/Warming Pulse Wire/i)).toBeVisible()
    await expect(page.getByText(/first stories usually appear within a minute/i)).toBeVisible()
    await expect(page.getByText('No stories on the wire in 24h yet.')).toHaveCount(0)
    await expect(page.getByText('Lead story')).toHaveCount(0)
    await expect(page.getByText('Open story')).toHaveCount(0)
    await expect(page.getByRole('heading', { name: 'Source health' })).toBeVisible()
    await expect(page.getByText('no videos in ytInitialData').last()).toBeVisible()

    const hasHorizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1)
    expect(hasHorizontalOverflow).toBeFalsy()
  })

  test('wire reader rail stays full on desktop and collapses on narrow layouts', async ({ page }) => {
    await routeWireLayoutFixture(page)

    await page.setViewportSize({ width: 1280, height: 850 })
    await page.goto('/pulse-wire?tab=wire&window=24h')
    await expect(page.getByRole('heading', { name: 'Reddit thread spreads to YouTube while origin is pending' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Source health' })).toBeVisible()
    await expect(page.getByText('Instagram is link-only evidence.').last()).toBeVisible()
    await expect(page.getByText('Kick discovery is planned after Evidence Gallery proves useful.').last()).toBeVisible()
    await expect(page.getByText('BrowserContext.new_page failed').last()).toBeVisible()
    await expect(page.getByText('no videos in ytInitialData').last()).toBeVisible()
    await expect(page.getByText('link only').last()).toBeVisible()
    await expect(page.getByText('deferred').last()).toBeVisible()
    await expect(page.getByText('degraded').last()).toBeVisible()
    await page.getByRole('button', { name: 'View all', exact: true }).click()
    await expect(page.getByRole('button', { name: 'Operator tools Hide' })).toBeVisible()
    await expect(page.locator('#pulse-wire-operator-tools-desktop').getByText('Wire modes')).toBeVisible()
    await expect(page.getByText('Reader rail', { exact: true })).toBeHidden()

    await page.setViewportSize({ width: 820, height: 900 })
    await page.reload()
    await expect(page.getByRole('heading', { name: 'Reddit thread spreads to YouTube while origin is pending' })).toBeVisible()
    await expect(page.getByText('Reader rail', { exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Source health' })).toBeHidden()
    await page.getByText('Reader rail', { exact: true }).click()
    await expect(page.getByRole('heading', { name: 'Source health' })).toBeVisible()

    await page.setViewportSize({ width: 390, height: 844 })
    await page.reload()
    await expect(page.getByText('Reader rail', { exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Reddit thread spreads to YouTube while origin is pending' })).toBeVisible()
    const hasHorizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1)
    expect(hasHorizontalOverflow).toBeFalsy()
  })

  test('wire saved filter shows followed stories from story_follows state', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 850 })
    const now = '2026-06-17T20:00:00Z'
    const followedStory = {
      story: {
        id: 918,
        title: 'Saved story stays in the followed rail',
        state: 'published',
        category: 'drama',
        updatedAt: now,
      },
      entity: { login: 'caseoh', displayName: 'CaseOh' },
      scores: { trend: null, volatility: null, confidence: 'corroborated', sentiment: null },
      windowScores: { sourceCount: 2, evidenceCount: 2, rankScore: 82, velocityScore: 52 },
      windowReceipts: [
        { sourceType: 'reddit_thread', pct: 82, label: 'LSF thread', url: 'https://reddit.com/r/LivestreamFail/saved' },
        { sourceType: 'youtube_video', pct: 61, label: 'YouTube context', url: 'https://youtube.com/watch?v=saved123456' },
      ],
      windowTimeline: [
        { at: now, sourceType: 'reddit_thread', label: 'LSF pickup' },
      ],
      evidenceGallery: [],
      matchExplanation: [],
      tracked: true,
    }
    const unsavedStory = {
      story: {
        id: 919,
        title: 'Unsaved story should leave the saved filter',
        state: 'published',
        category: 'drama',
        updatedAt: now,
      },
      entity: { login: 'jynxzi', displayName: 'Jynxzi' },
      scores: { trend: null, volatility: null, confidence: 'corroborated', sentiment: null },
      windowScores: { sourceCount: 2, evidenceCount: 2, rankScore: 96, velocityScore: 77 },
      windowReceipts: [
        { sourceType: 'reddit_thread', pct: 88, label: 'LSF thread', url: 'https://reddit.com/r/LivestreamFail/unsaved' },
        { sourceType: 'youtube_video', pct: 72, label: 'YouTube context', url: 'https://youtube.com/watch?v=unsaved1234' },
      ],
      windowTimeline: [
        { at: now, sourceType: 'reddit_thread', label: 'LSF pickup' },
      ],
      evidenceGallery: [],
      matchExplanation: [],
      tracked: false,
    }

    await page.route('**/v1/pulse-wire/feed**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [unsavedStory, followedStory], window: '24h', since: now, sort: 'rank' }),
    }))
    await page.route('**/v1/pulse-wire/developing**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))
    await page.route('**/v1/pulse-wire/source-health**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ sources: { reddit: { mode: 'active', healthy: true }, youtube: { mode: 'active', healthy: true } } }),
    }))
    await page.route('**/v1/pulse-wire/trending-streamers**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/bans**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/evidence/unlinked**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))

    await page.goto('/pulse-wire?tab=wire&window=24h')

    const desktopRail = page.locator('aside').last()
    await expect(page.getByRole('heading', { name: 'Unsaved story should leave the saved filter' })).toBeVisible()
    await expect(desktopRail.getByRole('heading', { name: 'Followed stories' })).toBeVisible()
    await expect(desktopRail.getByText('Saved story stays in the followed rail')).toBeVisible()
    await expect(desktopRail.getByRole('heading', { name: 'Your watchlist' })).toBeVisible()
    await expect(desktopRail.getByText('CaseOh').last()).toBeVisible()
    await expect(desktopRail.getByText('1 followed')).toBeVisible()

    await page.getByRole('button', { name: 'Saved', exact: true }).click()
    await expect(page).toHaveURL(/filter=saved/)
    await expect(page.getByRole('heading', { name: 'Saved story stays in the followed rail' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Unsaved story should leave the saved filter' })).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Saved', exact: true })).toHaveAttribute('aria-pressed', 'true')
    await expect(desktopRail.getByText('Showing saved stories')).toBeVisible()
  })

  test('wire watchlist persists category and keyword quick filters', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 850 })
    const now = '2026-06-17T20:00:00Z'
    const story = {
      story: {
        id: 923,
        title: 'Contract leak topic appears in drama watch',
        state: 'published',
        category: 'drama',
        storyClass: 'drama_claim',
        updatedAt: now,
      },
      entity: { login: 'xqc', displayName: 'xQc' },
      scores: { trend: null, volatility: null, confidence: 'corroborated', sentiment: null },
      windowScores: { sourceCount: 2, evidenceCount: 2, rankScore: 84, velocityScore: 52 },
      windowReceipts: [
        { sourceType: 'reddit_thread', pct: 82, label: 'LSF thread', url: 'https://reddit.com/r/LivestreamFail/watch-entry' },
      ],
      windowTimeline: [
        { at: now, sourceType: 'reddit_thread', label: 'LSF pickup' },
      ],
      evidenceGallery: [],
      matchExplanation: [],
      tracked: false,
    }
    const watchEntries = [
      { id: 3001, kind: 'category', value: 'drama', label: 'Drama', createdAt: now },
      { id: 3002, kind: 'keyword', value: 'contract leak', label: 'contract leak', createdAt: now },
    ]
    let nextID = 3003

    await page.route('**/v1/pulse-wire/watch-entries**', async route => {
      const request = route.request()
      const url = new URL(request.url())
      if (request.method() === 'DELETE') {
        const id = Number(url.pathname.split('/').pop())
        const index = watchEntries.findIndex(item => item.id === id)
        if (index >= 0) watchEntries.splice(index, 1)
        await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ status: 'unwatched' }) })
        return
      }
      if (request.method() === 'POST') {
        const body = JSON.parse(request.postData() || '{}')
        const value = String(body.value || '').trim().toLowerCase()
        const item = {
          id: nextID++,
          kind: body.kind,
          value,
          label: body.label || value,
          createdAt: now,
        }
        watchEntries.unshift(item)
        await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ status: 'watched', item }) })
        return
      }
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: watchEntries }) })
    })
    await page.route('**/v1/pulse-wire/feed**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [story], window: '24h', since: now, sort: 'rank' }),
    }))
    await page.route('**/v1/pulse-wire/developing**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))
    await page.route('**/v1/pulse-wire/source-health**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ sources: { reddit: { mode: 'active', healthy: true } } }),
    }))
    await page.route('**/v1/pulse-wire/trending-streamers**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/bans**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/evidence/unlinked**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))

    await page.goto('/pulse-wire?tab=wire&window=24h')

    const rail = page.locator('aside').last()
    await expect(rail.getByRole('heading', { name: 'Your watchlist' })).toBeVisible()
    await expect(rail.getByText('Categories')).toBeVisible()
    await expect(rail.getByRole('button', { name: 'Drama', exact: true })).toBeVisible()
    await expect(rail.getByText('Keywords')).toBeVisible()
    await expect(rail.getByRole('button', { name: 'contract leak', exact: true })).toBeVisible()

    await rail.getByRole('button', { name: 'Drama', exact: true }).click()
    await expect(page).toHaveURL(/category=drama/)

    await rail.getByRole('button', { name: 'contract leak', exact: true }).click()
    await expect(page).toHaveURL(/q=contract\+leak/)

    await page.getByRole('searchbox').fill('lawsuit')
    await expect(rail.getByRole('button', { name: 'Watch "lawsuit"' })).toBeVisible()
    await rail.getByRole('button', { name: 'Watch "lawsuit"' }).click()
    await expect(rail.getByRole('button', { name: 'lawsuit', exact: true })).toBeVisible()

    await rail.getByRole('button', { name: 'Remove contract leak watch' }).click()
    await expect(rail.getByRole('button', { name: 'contract leak', exact: true })).toHaveCount(0)
  })

  test('wire search filters streamers first, story titles second, and topics third', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 850 })
    const now = '2026-06-17T20:00:00Z'
    const streamerMatch = {
      story: {
        id: 920,
        title: 'Viewer record spreads across LSF',
        state: 'published',
        category: 'records',
        updatedAt: now,
      },
      entity: { login: 'caseoh', displayName: 'CaseOh' },
      scores: { trend: null, volatility: null, confidence: 'corroborated', sentiment: null },
      windowScores: { sourceCount: 2, evidenceCount: 2, rankScore: 79, velocityScore: 47 },
      windowReceipts: [
        { sourceType: 'reddit_thread', pct: 82, label: 'LSF thread', url: 'https://reddit.com/r/LivestreamFail/search-streamer' },
      ],
      windowTimeline: [
        { at: now, sourceType: 'reddit_thread', label: 'LSF pickup' },
      ],
      evidenceGallery: [],
      matchExplanation: [],
      tracked: false,
    }
    const titleMatch = {
      story: {
        id: 921,
        title: 'Lawsuit story title match survives search',
        state: 'published',
        category: 'drama',
        updatedAt: now,
      },
      entity: { login: 'jynxzi', displayName: 'Jynxzi' },
      scores: { trend: null, volatility: null, confidence: 'corroborated', sentiment: null },
      windowScores: { sourceCount: 2, evidenceCount: 2, rankScore: 96, velocityScore: 71 },
      windowReceipts: [
        { sourceType: 'reddit_thread', pct: 88, label: 'LSF thread', url: 'https://reddit.com/r/LivestreamFail/search-title' },
      ],
      windowTimeline: [
        { at: now, sourceType: 'reddit_thread', label: 'LSF pickup' },
      ],
      evidenceGallery: [],
      matchExplanation: [],
      tracked: false,
    }
    const topicMatch = {
      story: {
        id: 922,
        title: 'Authority receipt reaches the wire',
        state: 'published',
        category: 'bans',
        storyClass: 'ban_event',
        updatedAt: now,
      },
      entity: { id: 55, login: 'hasanabi', displayName: 'HasanAbi' },
      scores: { trend: null, volatility: null, confidence: 'corroborated', sentiment: null },
      windowScores: { sourceCount: 2, evidenceCount: 2, rankScore: 88, velocityScore: 59 },
      windowReceipts: [
        { sourceType: 'streamerbans', pct: 91, label: 'StreamerBans authority', url: 'https://streamerbans.example/search-topic' },
      ],
      windowTimeline: [
        { at: now, sourceType: 'streamerbans', label: 'StreamerBans pickup' },
      ],
      evidenceGallery: [],
      matchExplanation: [],
      tracked: false,
    }

    await page.route('**/v1/pulse-wire/feed**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [titleMatch, topicMatch, streamerMatch], window: '24h', since: now, sort: 'rank' }),
    }))
    await page.route('**/v1/pulse-wire/developing**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))
    await page.route('**/v1/pulse-wire/source-health**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ sources: { reddit: { mode: 'active', healthy: true } } }),
    }))
    await page.route('**/v1/pulse-wire/trending-streamers**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/bans**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/evidence/unlinked**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))

    await page.goto('/pulse-wire?tab=wire&window=24h')

    const search = page.getByRole('searchbox')
    await expect(page.getByRole('heading', { name: 'Lawsuit story title match survives search' })).toBeVisible()

    await search.fill('CaseOh')
    await expect(page).toHaveURL(/q=CaseOh/)
    await expect(page.getByRole('heading', { name: 'Viewer record spreads across LSF' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Lawsuit story title match survives search' })).toHaveCount(0)
    await expect(page.getByRole('heading', { name: 'Authority receipt reaches the wire' })).toHaveCount(0)

    await search.fill('lawsuit')
    await expect(page).toHaveURL(/q=lawsuit/)
    await expect(page.getByRole('heading', { name: 'Lawsuit story title match survives search' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Viewer record spreads across LSF' })).toHaveCount(0)
    await expect(page.getByRole('heading', { name: 'Authority receipt reaches the wire' })).toHaveCount(0)

    await search.fill('ban event')
    await expect(page).toHaveURL(/q=ban\+event/)
    await expect(page.getByRole('heading', { name: 'Authority receipt reaches the wire' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Viewer record spreads across LSF' })).toHaveCount(0)
    await expect(page.getByRole('heading', { name: 'Lawsuit story title match survives search' })).toHaveCount(0)

    await page.getByRole('button', { name: 'Clear search' }).click()
    await expect(page).not.toHaveURL(/q=/)
    await expect(search).toHaveValue('')
  })

  test('wire reader shows why trending, spread states, and no default Breaking label', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 850 })
    const now = '2026-06-17T20:00:00Z'
    const needsOrigin = {
      story: {
        id: 901,
        title: 'LSF thread spreads to YouTube before Twitch origin is found',
        state: 'published',
        category: 'drama',
        originSearchStatus: 'searched_missing',
        originCheckedAt: now,
        updatedAt: now,
      },
      entity: { login: 'caseoh', displayName: 'CaseOh' },
      scores: { trend: null, volatility: null, confidence: 'corroborated', sentiment: null },
      windowScores: { sourceCount: 2, evidenceCount: 3, recentSourceDelta: 1, rankScore: 74, velocityScore: 62 },
      windowReceipts: [
        { sourceType: 'reddit_thread', pct: 88, label: 'LSF thread', url: 'https://reddit.com/r/LivestreamFail/example' },
        { sourceType: 'youtube_video', pct: 65, label: 'YouTube repost', url: 'https://youtube.com/watch?v=abc' },
      ],
      windowTimeline: [
        { at: now, sourceType: 'reddit_thread', label: 'LSF pickup' },
        { at: now, sourceType: 'youtube_video', label: 'YouTube repost' },
      ],
      evidenceGallery: [],
      matchExplanation: [],
      tracked: false,
    }
    const singleSource = {
      story: {
        id: 902,
        title: 'Single Reddit thread is still developing',
        state: 'published',
        category: 'news',
        updatedAt: now,
      },
      entity: { login: 'jynxzi', displayName: 'Jynxzi' },
      scores: { trend: null, volatility: null, confidence: 'single_source', sentiment: null },
      windowScores: { sourceCount: 1, evidenceCount: 1, rankScore: 32, velocityScore: 22 },
      windowReceipts: [
        { sourceType: 'reddit_thread', pct: 76, label: 'LSF thread', url: 'https://reddit.com/r/LivestreamFail/single' },
      ],
      windowTimeline: [],
      evidenceGallery: [],
      matchExplanation: [],
      tracked: false,
    }

    await page.route('**/v1/pulse-wire/feed**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        items: [needsOrigin, singleSource],
        window: '24h',
        since: now,
        sort: 'rank',
        rankModel: 'window-native-v1',
      }),
    }))
    await page.route('**/v1/pulse-wire/stories/901**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify(needsOrigin),
    }))
    await page.route('**/v1/pulse-wire/developing**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))
    await page.route('**/v1/pulse-wire/source-health**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        sources: {
          reddit: { mode: 'active', healthy: true, details: { comments: { healthy: true, last_items: 8 } } },
          youtube: { mode: 'error', healthy: false, last_error: 'scraper unhealthy' },
          x: { mode: 'link_only', hint: 'URLs only' },
          tiktok: { mode: 'link_only', hint: 'URLs only' },
        },
        directorySampler: {},
        windowScoreCompute: {},
      }),
    }))
    await page.route('**/v1/pulse-wire/trending-streamers**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/bans**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        items: [
          {
            id: 77,
            streamerLogin: 'caseoh',
            streamerDisplayName: 'CaseOh',
            eventType: 'ban_confirmed',
            platform: 'twitch',
            source: 'streamerbans',
            headline: 'CaseOh Twitch ban confirmed',
            sourceUrl: 'https://streamerbans.example/caseoh',
            occurredAt: now,
            confidence: 0.7,
          },
        ],
        window: '24h',
        since: now,
      }),
    }))
    await page.route('**/v1/pulse-wire/evidence/unlinked**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))

    await page.goto('/pulse-wire?tab=wire&window=24h')

    await expect(page.getByRole('heading', { name: 'Pulse Wire' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Bans & moderation' })).toBeVisible()
    await expect(page.getByText('Needs origin').first()).toBeVisible()
    await expect(page.getByText('No Twitch origin found').first()).toBeVisible()
    await expect(page.getByText('Analytics searched the available Twitch moments for this story and did not find a matching origin yet.')).toBeVisible()
    await expect(page.getByText('Pulse origin graph and top emotes are waiting on real Analytics origin matching.')).toBeVisible()
    await expect(page.getByRole('button', { name: 'View origin moment' })).toHaveCount(0)
    await expect(page.getByRole('link', { name: 'View origin moment' })).toHaveCount(0)
    await expect(page.locator('[aria-label="Evidence pulse"]')).toHaveCount(0)
    await expect.poll(async () => page.evaluate(() => {
      const isVisible = (el: Element) => {
        const rect = el.getBoundingClientRect()
        const style = window.getComputedStyle(el)
        return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none'
      }
      const cards = Array.from(document.querySelectorAll('[aria-label="Evidence spread cards"]'))
        .filter(isVisible)
      return {
        hasYouTube: cards.some(card => card.textContent?.includes('YouTube')),
        hasNoX: cards.some(card => card.textContent?.includes('No X link')),
      }
    })).toEqual({ hasYouTube: true, hasNoX: true })
    await expect(page.getByRole('heading', { name: 'Developing now' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Confirmed across sources' })).toHaveCount(0)
    await expect(page.getByText('Single Reddit thread is still developing')).toBeVisible()
    await expect(page.getByText('Breaking')).toHaveCount(0)
    await expect(page.getByText('scraper unhealthy').last()).toBeVisible()
    await expect(page.getByText('comments').last()).toBeVisible()
    await expect(page.getByText('8 items').last()).toBeVisible()
    await expect(page.getByText('+1 source in last hour').last()).toBeVisible()
    await expect(page.getByText('Reddit thread active').last()).toBeVisible()
    await expect(page.getByText('CaseOh Twitch ban confirmed').last()).toBeVisible()
    await expect(page.getByText('70%').last()).toBeVisible()
    await page.getByRole('button', { name: 'Operator tools Show' }).click()
    await expect(page.getByText('Stories waiting for origin match').last()).toBeVisible()
    await expect(page.getByRole('link', { name: 'Review missing evidence' })).toHaveCount(0)
    await page.getByRole('button', { name: 'Show analyst gaps' }).click()
    const missingEvidenceLink = page.getByRole('link', { name: 'Review missing evidence' }).first()
    await expect(missingEvidenceLink).toHaveAttribute('href', '/pulse-wire/901?tab=wire&window=24h#missing-evidence')
    await missingEvidenceLink.click()
    await expect(page).toHaveURL(/\/pulse-wire\/901\?tab=wire&window=24h#missing-evidence/)
    await expect(page.locator('#missing-evidence')).toBeVisible()
    await expect.poll(async () => page.locator('#missing-evidence').evaluate(el => {
      const rect = el.getBoundingClientRect()
      return rect.bottom > 0 && rect.top < window.innerHeight
    })).toBe(true)
    await expect(page.locator('#missing-evidence').getByText('No official response found')).toBeVisible()
  })

  test('wire reader links origin moment only when origin includes a VOD id', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 850 })
    const now = '2026-06-17T20:00:00Z'
    const story = {
      story: {
        id: 906,
        title: 'Pulse matched story can open the origin VOD moment',
        state: 'published',
        category: 'drama',
        updatedAt: now,
      },
      entity: { login: 'caseoh', displayName: 'CaseOh' },
      origin: {
        id: 77,
        streamId: '316955094498',
        vodId: '2371095470',
        vodOffsetS: 420,
        quotes: ['chat spike matched the story origin'],
        chatSpikeSummary: 'Chat jumped 3.2x near the matched quote window.',
        originConfidence: 0.87,
        originSpikePoints: [
          { offsetS: 240, relativeS: -180, chatCount: 8, totalEmoteCount: 4, sevenTvEmoteCount: 2, viewerMax: 1200 },
          { offsetS: 300, relativeS: -120, chatCount: 12, totalEmoteCount: 5, sevenTvEmoteCount: 3, viewerMax: 1250 },
          { offsetS: 360, relativeS: -60, chatCount: 26, totalEmoteCount: 18, sevenTvEmoteCount: 12, viewerMax: 1500 },
          { offsetS: 420, relativeS: 0, chatCount: 81, totalEmoteCount: 66, sevenTvEmoteCount: 44, viewerMax: 2100 },
          { offsetS: 480, relativeS: 60, chatCount: 34, totalEmoteCount: 20, sevenTvEmoteCount: 11, viewerMax: 1700 },
          { offsetS: 540, relativeS: 120, chatCount: 18, totalEmoteCount: 9, sevenTvEmoteCount: 4, viewerMax: 1380 },
        ],
        topEmotes: [
          { name: 'OMEGALUL', provider: '7tv', count: 42 },
          { name: 'Aware', provider: 'bttv', count: 28 },
        ],
      },
      scores: { trend: null, volatility: null, confidence: 'corroborated', sentiment: null },
      windowScores: { sourceCount: 2, evidenceCount: 2, rankScore: 84, velocityScore: 52 },
      windowReceipts: [
        { sourceType: 'pulse_origin', pct: 100, label: 'Pulse origin' },
        { sourceType: 'reddit_thread', pct: 77, label: 'LSF thread', url: 'https://reddit.com/r/LivestreamFail/pulse-origin' },
      ],
      windowTimeline: [
        { at: now, sourceType: 'pulse_origin', label: 'Pulse origin moment' },
        { at: now, sourceType: 'reddit_thread', label: 'LSF pickup' },
      ],
      evidenceGallery: [],
      matchExplanation: [],
      operatorActions: [
        {
          id: 90601,
          clusterId: 906,
          action: 'confirm_origin_moment',
          operator: 'operator',
          note: 'Fixture verifies origin confirmation audit display.',
          beforeData: { momentFpId: 77, streamId: '316955094498', vodId: '2371095470', vodOffsetS: 420 },
          afterData: { momentFpId: 77, streamId: '316955094498', vodId: '2371095470', vodOffsetS: 420 },
          createdAt: now,
        },
      ],
      tracked: false,
    }

    await page.route('**/v1/pulse-wire/feed**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [story], window: '24h', since: now, sort: 'rank' }),
    }))
    await page.route('**/v1/pulse-wire/stories/906**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify(story),
    }))
    await page.route('**/v1/pulse-wire/developing**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))
    await page.route('**/v1/pulse-wire/source-health**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ sources: { reddit: { mode: 'active', healthy: true } } }),
    }))
    await page.route('**/v1/pulse-wire/trending-streamers**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/bans**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/evidence/unlinked**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))

    await page.goto('/pulse-wire?tab=wire&window=24h')

    await expect(page.getByText('Pulse matched').first()).toBeVisible()
    const leadOriginChart = page.getByLabel('Chat activity spike chart').first()
    await expect(leadOriginChart).toBeVisible()
    await expect(page.getByText('6 real rollups').first()).toBeVisible()
    await expect(leadOriginChart.getByText('0s', { exact: true })).toBeVisible()
    await expect(page.getByRole('link', { name: 'View origin moment' }).first()).toHaveAttribute(
      'href',
      '/c/caseoh?vod=2371095470&offset=420&from=analytics&sid=316955094498',
    )

    await page.getByRole('button', { name: 'Open story' }).click()
    await expect(page.locator('#origin')).toBeVisible()
    await expect(page.getByText('Pulse origin matched')).toBeVisible()
    await expect(page.getByText('Chat jumped 3.2x near the matched quote window.').last()).toBeVisible()
    await expect(page.getByText('Origin confidence').last()).toBeVisible()
    await expect(page.getByText('87%').last()).toBeVisible()
    await expect(page.getByLabel('Chat activity spike chart').last()).toBeVisible()
    await expect(page.getByText('Top emotes near origin').last()).toBeVisible()
    await expect(page.getByText('OMEGALUL').last()).toBeVisible()
    await expect(page.getByRole('link', { name: 'View origin moment' }).last()).toHaveAttribute(
      'href',
      '/c/caseoh?vod=2371095470&offset=420&from=analytics&sid=316955094498',
    )
    await expect(page.getByText('Confirmed origin moment')).toBeVisible()
    await expect(page.getByText('origin 316955094498 / 2371095470 @ 420s confirmed')).toBeVisible()
    await expect(page.getByText('Fixture verifies origin confirmation audit display.')).toBeVisible()
  })

  test('story detail evidence gallery renders provider-specific states', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 850 })
    const now = '2026-06-17T20:00:00Z'
    const story = {
      story: {
        id: 907,
        title: 'Cross-platform evidence gallery fixture',
        state: 'published',
        category: 'drama',
        updatedAt: now,
      },
      entity: { login: 'caseoh', displayName: 'CaseOh' },
      scores: { trend: null, volatility: null, confidence: 'corroborated', sentiment: null },
      windowScores: { sourceCount: 6, evidenceCount: 6, rankScore: 81, velocityScore: 57 },
      windowReceipts: [
        { sourceType: 'twitch_clip', pct: 88, label: 'Twitch clip', url: 'https://clips.twitch.tv/provider-fixture' },
        { sourceType: 'reddit_thread', pct: 82, label: 'LSF thread', url: 'https://reddit.com/r/LivestreamFail/comments/provider/story/' },
        { sourceType: 'youtube_video', pct: 72, label: 'YouTube Short', url: 'https://youtube.com/shorts/abc123def45' },
        { sourceType: 'x_post', pct: 61, label: 'X post', url: 'https://x.com/creator/status/123' },
        { sourceType: 'tiktok_video', pct: 59, label: 'TikTok video', url: 'https://www.tiktok.com/@creator/video/999' },
        { sourceType: 'manual_curation', pct: 42, label: 'Official statement', url: 'https://creator.example/statement' },
      ],
      windowTimeline: [
        { at: now, sourceType: 'twitch_clip', label: 'Twitch clip posted' },
        { at: now, sourceType: 'reddit_thread', label: 'LSF discussion' },
      ],
      evidenceGallery: [
        {
          id: 9071,
          canonicalUrl: 'https://clips.twitch.tv/provider-fixture',
          platform: 'twitch',
          providerName: 'Twitch',
          title: 'Original Twitch clip',
          thumbnailUrl: 'https://static-cdn.jtvnw.net/previews-ttv/live_user_caseoh-320x180.jpg',
          previewStatus: 'ready',
          matchKind: 'url',
        },
        {
          id: 9072,
          canonicalUrl: 'https://www.reddit.com/r/LivestreamFail/comments/provider/story/',
          platform: 'reddit',
          providerName: 'Reddit / LSF',
          title: 'LSF discussion thread',
          previewStatus: 'fallback',
          matchKind: 'comment_link',
        },
        {
          id: 9073,
          canonicalUrl: 'https://www.youtube.com/shorts/abc123def45',
          platform: 'youtube',
          providerName: 'YouTube',
          title: 'Short repost',
          previewStatus: 'ready',
          matchKind: 'url',
        },
        {
          id: 9074,
          canonicalUrl: 'https://x.com/creator/status/123',
          platform: 'x',
          providerName: 'X',
          title: 'Creator post',
          previewStatus: 'fallback',
          matchKind: 'url',
        },
        {
          id: 9075,
          canonicalUrl: 'https://www.tiktok.com/@creator/video/999',
          platform: 'tiktok',
          providerName: 'TikTok',
          title: 'TikTok repost',
          previewStatus: 'fallback',
          matchKind: 'url',
        },
        {
          id: 9076,
          canonicalUrl: 'https://creator.example/statement',
          platform: 'web',
          providerName: 'Creator site',
          title: 'Official statement',
          previewStatus: 'fallback',
          matchKind: 'manual_curation',
        },
      ],
      matchExplanation: [],
      tracked: false,
    }

    await page.route('**/v1/pulse-wire/feed**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now, sort: 'rank' }),
    }))
    await page.route('**/v1/pulse-wire/stories/907**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify(story),
    }))
    await page.route('**/v1/pulse-wire/developing**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))
    await page.route('**/v1/pulse-wire/source-health**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        sources: {
          reddit: { mode: 'active', healthy: true },
          youtube: { mode: 'active', healthy: true },
          x: { mode: 'link_only', healthy: true },
          tiktok: { mode: 'link_only', healthy: true },
          twitchclips: { mode: 'active', healthy: true },
        },
      }),
    }))
    await page.route('**/v1/pulse-wire/trending-streamers**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/bans**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/evidence/unlinked**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))

    await page.goto('/pulse-wire/907?window=24h')

    await expect(page.getByRole('heading', { name: 'Cross-platform evidence gallery fixture' })).toBeVisible()
    const gallery = page.locator('#evidence-gallery')
    await expect(gallery).toBeVisible()
    await expect(gallery.getByText('Twitch clip', { exact: true })).toBeVisible()
    await expect(gallery.getByText('Reddit thread', { exact: true })).toBeVisible()
    await expect(gallery.getByText('YouTube Short', { exact: true })).toBeVisible()
    await expect(gallery.getByText('X linked post', { exact: true })).toBeVisible()
    await expect(gallery.getByText('TikTok linked video', { exact: true })).toBeVisible()
    await expect(gallery.getByText('Generic link', { exact: true })).toBeVisible()
  })

  test('wire reader shows empty moderation state without fake ban rows', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 850 })
    const now = '2026-06-17T20:00:00Z'
    const story = {
      story: {
        id: 903,
        title: 'Ban rumor needs authority source',
        state: 'published',
        category: 'bans',
        updatedAt: now,
      },
      entity: { login: 'hasanabi', displayName: 'HasanAbi' },
      scores: { trend: null, volatility: null, confidence: 'single_source', sentiment: null },
      windowScores: { sourceCount: 1, evidenceCount: 1, rankScore: 28, velocityScore: 18 },
      windowReceipts: [
        { sourceType: 'reddit_thread', pct: 63, label: 'LSF thread', url: 'https://reddit.com/r/LivestreamFail/ban-rumor' },
      ],
      windowTimeline: [],
      evidenceGallery: [],
      matchExplanation: [],
      tracked: false,
    }

    await page.route('**/v1/pulse-wire/feed**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [story], window: '24h', since: now, sort: 'rank' }),
    }))
    await page.route('**/v1/pulse-wire/developing**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))
    await page.route('**/v1/pulse-wire/source-health**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ sources: { reddit: { mode: 'active', healthy: true }, streamerbans: { mode: 'active', healthy: true } } }),
    }))
    await page.route('**/v1/pulse-wire/trending-streamers**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/bans**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/evidence/unlinked**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))

    await page.goto('/pulse-wire?tab=wire&window=24h')

    await expect(page.getByRole('heading', { name: 'Bans & moderation' })).toBeVisible()
    await expect(page.getByText('No ban or moderation events found in this window.').last()).toBeVisible()
    await expect(page.getByText('CaseOh Twitch ban confirmed')).toHaveCount(0)
  })

  test('bans story detail shows moderation context from attached receipts', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 850 })
    const now = '2026-06-17T20:00:00Z'
    const story = {
      story: {
        id: 904,
        title: 'HasanAbi Twitch ban confirmed by StreamerBans',
        summary: 'Authority receipt and discussion source are attached.',
        state: 'published',
        category: 'bans',
        updatedAt: now,
      },
      entity: { login: 'hasanabi', displayName: 'HasanAbi' },
      scores: { trend: null, volatility: null, confidence: 'corroborated', sentiment: null },
      windowScores: { sourceCount: 2, evidenceCount: 2, rankScore: 64, velocityScore: 38 },
      windowReceipts: [
        {
          sourceType: 'streamerbans',
          pct: 92,
          label: 'StreamerBans confirmation',
          url: 'https://streamerbans.example/hasanabi',
          previewStatus: 'ready',
          occurredAt: now,
        },
        { sourceType: 'reddit_thread', pct: 70, label: 'LSF discussion', url: 'https://reddit.com/r/LivestreamFail/hasan-ban' },
      ],
      windowTimeline: [
        { at: now, sourceType: 'streamerbans', label: 'StreamerBans event', sourceUrl: 'https://streamerbans.example/hasanabi' },
      ],
      evidenceGallery: [
        {
          id: 55,
          canonicalUrl: 'https://streamerbans.example/hasanabi',
          platform: 'streamerbans',
          providerName: 'StreamerBans',
          title: 'HasanAbi Twitch ban confirmed',
          previewStatus: 'ready',
          embedHtml: '<strong>UNSAFE EMBED SHOULD NOT RENDER</strong>',
          matchKind: 'authority_feed',
          createdAtSrc: now,
        },
      ],
      matchExplanation: [
        {
          sourceType: 'streamerbans',
          matchedBy: 'authority_feed',
          confidence: 0.92,
          sourceUrl: 'https://streamerbans.example/hasanabi',
          author: 'StreamerBansBot',
          factors: ['streamer login match', 'duplicate_author:streamerbansbot'],
        },
      ],
      operatorActions: [
        {
          id: 90401,
          clusterId: 904,
          action: 'mark_community_meta',
          operator: 'operator',
          note: 'Fixture verifies auditable operator action display.',
          beforeData: { category: 'drama', storyClass: '', state: 'published' },
          afterData: { category: 'community_meta', storyClass: 'community_meta', state: 'published' },
          createdAt: now,
        },
        {
          id: 90402,
          clusterId: 904,
          action: 'manual_suppress',
          operator: 'operator',
          note: 'Fixture verifies manual suppress audit display.',
          beforeData: { category: 'community_meta', storyClass: 'community_meta', state: 'published' },
          afterData: { category: 'community_meta', storyClass: 'community_meta', state: 'suppressed' },
          createdAt: now,
        },
        {
          id: 90403,
          clusterId: 904,
          action: 'confirm_streamer_entity',
          operator: 'operator',
          note: 'Fixture verifies entity confirmation audit display.',
          beforeData: { entityId: 55, entityLogin: 'hasanabi', entityDisplayName: 'HasanAbi' },
          afterData: { entityId: 55, entityLogin: 'hasanabi', entityDisplayName: 'HasanAbi' },
          createdAt: now,
        },
        {
          id: 90404,
          clusterId: 904,
          action: 'merge_duplicate_story',
          operator: 'operator',
          note: 'Fixture verifies duplicate merge audit display.',
          beforeData: { source: { clusterId: 904 }, target: { clusterId: 901 }, evidenceIds: [9101, 9102], previewIds: [8101] },
          afterData: { sourceClusterId: 904, targetClusterId: 901, sourceState: 'suppressed', evidenceIds: [9101, 9102], previewIds: [8101], movedEvidence: 2, movedPreviews: 1 },
          createdAt: now,
        },
        {
          id: 90405,
          clusterId: 904,
          action: 'split_unrelated_evidence',
          operator: 'operator',
          note: 'Fixture verifies evidence split audit display.',
          beforeData: { source: { clusterId: 904 }, evidenceIds: [9201], previewIds: [8201] },
          afterData: { sourceClusterId: 904, newClusterId: 990, evidenceIds: [9201], previewIds: [8201], title: 'Separated evidence story' },
          createdAt: now,
        },
      ],
      tracked: false,
    }

    await page.route('**/v1/pulse-wire/feed**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now, sort: 'rank' }),
    }))
    await page.route('**/v1/pulse-wire/stories/904**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify(story),
    }))
    await page.route('**/v1/pulse-wire/developing**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] }),
    }))
    await page.route('**/v1/pulse-wire/source-health**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ sources: { streamerbans: { mode: 'active', healthy: true }, reddit: { mode: 'active', healthy: true } } }),
    }))
    await page.route('**/v1/pulse-wire/trending-streamers**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/bans**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))
    await page.route('**/v1/pulse-wire/evidence/unlinked**', route => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [], window: '24h', since: now }),
    }))

    await page.goto('/pulse-wire/904?window=24h&analyst=1')

    await expect(page.getByRole('heading', { name: 'HasanAbi Twitch ban confirmed by StreamerBans' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Back to Cross-platform' })).toBeVisible()
    await expect(page.getByRole('navigation', { name: 'Story detail sections' })).toBeVisible()
    await expect(page.locator('#summary')).toBeVisible()
    await expect(page.locator('#origin')).toBeVisible()
    await expect(page.locator('#evidence-gallery')).toBeVisible()
    await expect(page.locator('#spread-timeline')).toBeVisible()
    await expect(page.locator('#source-comparison')).toBeVisible()
    await expect(page.locator('#match-explanation')).toBeVisible()
    await expect(page.locator('#operator-actions')).toBeVisible()
    await expect(page.getByText('Origin pending')).toBeVisible()
    await expect(page.getByLabel('Chat activity spike chart')).toHaveCount(0)
    await expect(page.getByText('Moderation context')).toBeVisible()
    await expect(page.getByRole('heading', { name: 'StreamerBans confirmation' })).toBeVisible()
    await expect(page.getByText('92%').first()).toBeVisible()
    await expect(page.getByRole('link', { name: 'Open moderation source' })).toHaveAttribute('href', 'https://streamerbans.example/hasanabi')
    await expect(page.getByText('Missing evidence', { exact: true })).toBeVisible()
    await expect(page.getByText('No Pulse origin matched')).toBeVisible()
    await expect(page.getByText('No original Twitch clip found')).toBeVisible()
    await expect(page.getByText('No YouTube repost found')).toBeVisible()
    await expect(page.getByText('No X link found')).toBeVisible()
    await expect(page.getByText('No TikTok link found')).toBeVisible()
    await expect(page.getByText('Evidence Gallery')).toBeVisible()
    await expect(page.getByText('Link preview unavailable')).toBeVisible()
    await expect(page.getByText('UNSAFE EMBED SHOULD NOT RENDER')).toHaveCount(0)
    await expect(page.getByRole('link', { name: 'Open source' })).toHaveAttribute('href', 'https://streamerbans.example/hasanabi')
    await expect(page.locator('#spread-timeline-heading')).toBeVisible()
    await expect(page.locator('#source-comparison-heading')).toBeVisible()
    await expect(page.getByText('duplicate_author:streamerbansbot')).toBeVisible()
    await expect(page.locator('#operator-actions-heading')).toBeVisible()
    await expect(page.getByText('Marked community meta')).toBeVisible()
    await expect(page.getByText('drama -> community_meta')).toBeVisible()
    await expect(page.getByText('class unset -> community_meta')).toBeVisible()
    await expect(page.getByText('Fixture verifies auditable operator action display.')).toBeVisible()
    await expect(page.getByText('Manually suppressed')).toBeVisible()
    await expect(page.getByText('published -> suppressed')).toBeVisible()
    await expect(page.getByText('Fixture verifies manual suppress audit display.')).toBeVisible()
    await expect(page.getByText('Confirmed streamer entity')).toBeVisible()
    await expect(page.getByText('entity HasanAbi confirmed')).toBeVisible()
    await expect(page.getByText('Fixture verifies entity confirmation audit display.')).toBeVisible()
    await expect(page.getByText('Merged duplicate story')).toBeVisible()
    await expect(page.getByText('2 evidence -> story 901')).toBeVisible()
    await expect(page.getByText('Fixture verifies duplicate merge audit display.')).toBeVisible()
    await expect(page.getByText('Split unrelated evidence')).toBeVisible()
    await expect(page.getByText('1 evidence -> new story 990')).toBeVisible()
    await expect(page.getByText('Fixture verifies evidence split audit display.')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Not news' })).toHaveCount(0)
    await expectVerticalOrder([
      page.locator('#summary'),
      page.locator('#origin'),
      page.getByText('Moderation context', { exact: true }).first(),
      page.getByText('Missing evidence', { exact: true }).first(),
      page.locator('#evidence-gallery'),
      page.locator('#spread-timeline'),
      page.locator('#source-comparison'),
      page.locator('#match-explanation'),
      page.locator('#operator-actions'),
    ])
  })
})
