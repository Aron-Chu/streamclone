import { chromium } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const outDir = path.resolve(__dirname, '../../docs/media')
const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:8090'

await mkdir(outDir, { recursive: true })

const browser = await chromium.launch({ headless: true })
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })

await page.goto(`${baseURL}/pulse-wire?window=24h`, { waitUntil: 'networkidle', timeout: 90_000 })
await page.getByRole('heading', { name: /More community threads|Hot on Reddit/i }).first().waitFor({ timeout: 60_000 })

const community = await page.evaluate(async () => {
  const res = await fetch('/v1/pulse-wire/community?window=24h&sort=hot&limit=5')
  return res.json()
})

const cardMeta = await page.evaluate(() => {
  const previews = [...document.querySelectorAll('[data-testid="community-preview"]')].map(img => ({
    src: img.getAttribute('src') || '',
    ok: img.naturalWidth > 0,
  }))
  const embeds = [...document.querySelectorAll('[data-testid="community-embed"]')].length
  const upvoteLines = [...document.querySelectorAll('article')].map(a => a.textContent?.match(/([\d,—]+)\s+upvotes/)?.[1] || null).filter(Boolean)
  return { previews, embedCount: embeds, upvoteLines: upvoteLines.slice(0, 8) }
})

await page.screenshot({ path: path.join(outDir, 'pulse-wire-reddit-viewport.png'), fullPage: false })
await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight))
await page.waitForTimeout(800)
await page.screenshot({ path: path.join(outDir, 'pulse-wire-reddit-full.png'), fullPage: true })

console.log(JSON.stringify({
  url: `${baseURL}/pulse-wire?window=24h`,
  apiItems: (community.items || []).map(item => ({
    title: item.title?.slice(0, 60),
    score: item.score,
    comments: item.comments,
    previewKind: item.previewKind,
    hasThumb: Boolean(item.displayThumbnailUrl),
    hasEmbed: Boolean(item.embedHtml || item.embedUrl),
  })),
  dom: cardMeta,
}, null, 2))

await browser.close()
