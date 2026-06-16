#!/usr/bin/env node
import { createRequire } from 'node:module'
import fs from 'node:fs/promises'
import path from 'node:path'

const requirePath = process.env.DOCS_PLAYWRIGHT_REQUIRE || path.join(process.cwd(), 'frontend', 'node_modules', 'playwright', 'package.json')
const require = createRequire(requirePath)
const { chromium } = require('playwright')

const root = process.cwd()
const width = Number(process.env.DOCS_VIEWPORT_WIDTH || 1920)
const height = Number(process.env.DOCS_VIEWPORT_HEIGHT || 1080)
const baseUrl = trimTrailingSlash(process.env.DOCS_BASE_URL || 'http://localhost:8090')
const pulseUrl = process.env.DOCS_PULSE_URL || 'http://localhost:3000/d/streamclone-emote-pulse/emote-pulse?from=now-24h&to=now&orgId=1&timezone=browser&refresh=30s'
const channelPath = process.env.DOCS_CHANNEL_PATH || '/c/xqc'
const analyticsPath = process.env.DOCS_ANALYTICS_PATH || '/analytics/xqc/2026-06-14'
const clipMs = Number(process.env.DOCS_CLIP_SECONDS || 7) * 1000
const imagesDir = path.join(root, 'docs', 'images')
const mediaDir = path.join(root, 'docs', 'media')
const videoDir = path.join(root, '.tmp', 'showcase-videos')
const requestedScenes = (process.env.DOCS_SCENES || '')
  .split(',')
  .map((scene) => scene.trim())
  .filter(Boolean)

const scenes = [
  {
    name: 'directory',
    url: `${baseUrl}/`,
    settleMs: 4000,
    beforeShot: closeStackStatusIfVisible,
    action: async (page) => {
      await gentleScroll(page)
      await page.mouse.move(250, 260, { steps: 18 })
      await page.mouse.move(1500, 760, { steps: 24 })
    },
  },
  {
    name: 'channel',
    url: `${baseUrl}${channelPath}`,
    settleMs: 7000,
    action: async (page) => {
      await page.mouse.move(1180, 320, { steps: 30 })
      await page.mouse.move(1640, 840, { steps: 30 })
      await page.keyboard.press('PageDown').catch(() => undefined)
      await sleep(900)
      await page.keyboard.press('PageUp').catch(() => undefined)
    },
  },
  {
    name: 'analytics',
    url: `${baseUrl}${analyticsPath}`,
    settleMs: 6000,
    action: async (page) => {
      await page.mouse.move(520, 520, { steps: 30 })
      await page.mouse.move(1120, 520, { steps: 30 })
      await gentleScroll(page)
    },
  },
  {
    name: 'pulse',
    url: pulseUrl,
    settleMs: 6000,
    beforeShot: loginGrafanaIfNeeded,
    action: async (page) => {
      await page.mouse.move(480, 360, { steps: 24 })
      await page.mouse.move(1320, 620, { steps: 24 })
      await page.keyboard.press('PageDown').catch(() => undefined)
      await sleep(1000)
      await page.keyboard.press('Home').catch(() => undefined)
    },
  },
]

const scenesToCapture = requestedScenes.length
  ? scenes.filter((scene) => requestedScenes.includes(scene.name))
  : scenes

if (!scenesToCapture.length) {
  throw new Error(`No matching scenes for DOCS_SCENES=${requestedScenes.join(',')}`)
}

await fs.mkdir(imagesDir, { recursive: true })
await fs.mkdir(mediaDir, { recursive: true })
await fs.mkdir(videoDir, { recursive: true })

const browser = await chromium.launch({
  headless: true,
  args: [
    `--window-size=${width},${height}`,
    '--disable-dev-shm-usage',
    '--autoplay-policy=no-user-gesture-required',
  ],
})

const results = []
try {
  for (const scene of scenesToCapture) {
    const result = await captureScene(browser, scene)
    results.push(result)
    console.log(`${result.ok ? 'ok' : 'skip'}: ${scene.name} - ${result.message}`)
  }
} finally {
  await browser.close()
}

await fs.writeFile(
  path.join(mediaDir, 'capture-summary.json'),
  JSON.stringify({ width, height, generatedAt: new Date().toISOString(), results }, null, 2) + '\n',
)

if (!results.some((result) => result.ok)) {
  process.exitCode = 1
}

async function captureScene(browser, scene) {
  const context = await browser.newContext({
    viewport: { width, height },
    screen: { width, height },
    deviceScaleFactor: 1,
    colorScheme: 'dark',
    ignoreHTTPSErrors: true,
    recordVideo: { dir: videoDir, size: { width, height } },
  })

  const page = await context.newPage()
  page.setDefaultTimeout(15000)
  page.setDefaultNavigationTimeout(45000)

  try {
    await page.goto(scene.url, { waitUntil: 'domcontentloaded', timeout: 45000 })
    await page.waitForLoadState('networkidle', { timeout: 15000 }).catch(() => undefined)
    await sleep(scene.settleMs || 3000)

    if (scene.beforeShot) {
      await scene.beforeShot(page, scene)
      await page.waitForLoadState('networkidle', { timeout: 15000 }).catch(() => undefined)
      await sleep(2000)
    }

    await dismissCommonOverlays(page)
    await page.screenshot({ path: path.join(imagesDir, `${scene.name}.png`), fullPage: false })

    const actionStarted = Date.now()
    if (scene.action) {
      await scene.action(page)
    }
    await sleep(Math.max(0, clipMs - (Date.now() - actionStarted)))

    const video = page.video()
    await context.close()
    if (video) {
      const rawPath = await video.path()
      await fs.copyFile(rawPath, path.join(mediaDir, `${scene.name}.webm`))
    }

    return { ok: true, scene: scene.name, message: `${width}x${height} PNG and WebM saved` }
  } catch (error) {
    await context.close().catch(() => undefined)
    return { ok: false, scene: scene.name, message: error?.message || String(error) }
  }
}

async function loginGrafanaIfNeeded(page, scene) {
  const user = process.env.DOCS_GRAFANA_USER || 'admin'
  const passwords = (process.env.DOCS_GRAFANA_PASSWORDS || process.env.DOCS_GRAFANA_PASSWORD || process.env.GRAFANA_ADMIN_PASSWORD || 'streampulse,devpulse,admin')
    .split(',')
    .map((password) => password.trim())
    .filter(Boolean)
  const userInput = page.locator('input[name="user"], input[aria-label="Username"], input[placeholder="email or username"]').first()
  if (await userInput.count().catch(() => 0)) {
    for (const password of passwords) {
      await userInput.fill(user)
      await page.locator('input[name="password"], input[type="password"]').first().fill(password)
      await page.locator('button[type="submit"], button:has-text("Log in")').first().click()
      await page.waitForLoadState('networkidle', { timeout: 15000 }).catch(() => undefined)
      await sleep(1500)
      if (!(await page.locator('input[name="user"], input[placeholder="email or username"]').count().catch(() => 0))) {
        const skipButton = page.locator('button:has-text("Skip"), a:has-text("Skip")').first()
        if (await skipButton.count().catch(() => 0)) {
          await skipButton.click().catch(() => undefined)
        }
        await page.goto(scene.url, { waitUntil: 'domcontentloaded', timeout: 45000 })
        return
      }
    }
  }
}

async function closeStackStatusIfVisible(page) {
  const browseButton = page.locator('button:has-text("Browse live streams"), a:has-text("Browse live streams")').first()
  if (await browseButton.count().catch(() => 0)) {
    await browseButton.click().catch(() => undefined)
    await sleep(1200)
    return
  }

  const closeButton = page.locator('button:has-text("Close"), a:has-text("Close"), button:has-text("Not now")').first()
  if (await closeButton.count().catch(() => 0)) {
    await closeButton.click().catch(() => undefined)
    await sleep(1200)
  }
}

async function dismissCommonOverlays(page) {
  for (const selector of [
    'button:has-text("Browse live streams")',
    'button:has-text("Not now")',
    'button:has-text("Close")',
    'button:has-text("Accept")',
    'button:has-text("Dismiss")',
    'button:has-text("Got it")',
    'button:has-text("Skip")',
  ]) {
    const button = page.locator(selector).first()
    if (await button.count().catch(() => 0)) {
      await button.click({ timeout: 1000 }).catch(() => undefined)
    }
  }
}

async function gentleScroll(page) {
  await page.mouse.wheel(0, 520).catch(() => undefined)
  await sleep(700)
  await page.mouse.wheel(0, -260).catch(() => undefined)
  await sleep(700)
}

function trimTrailingSlash(value) {
  return value.replace(/\/$/, '')
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}
