import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import type { Page } from '@playwright/test'
import {
  pickLiveChannelLink,
  settleBeforeScreenshot,
  watchChannelNavigation,
  waitForDirectoryReady,
} from './screenshot-helpers'

export const channelLogin = process.env.DOCS_CHANNEL_LOGIN ?? 'xqc'

export function imagesDir(fromDir: string) {
  return path.resolve(fromDir, '../../docs/images')
}

/** README screenshots: fixed 16:9 desktop — avoids squished GitHub renders. */
export async function prepareScreenshotViewport(page: Page) {
  await page.setViewportSize({ width: 1920, height: 1080 })
}

async function shot(page: Page, outDir: string, fileName: string) {
  await prepareScreenshotViewport(page)
  await settleBeforeScreenshot(page)
  await page.screenshot({
    path: path.join(outDir, fileName),
    fullPage: false,
    animations: 'disabled',
    type: 'png',
  })
}

export function resolveFfmpegCommand(): string {
  if (process.env.DOCS_FFMPEG) return process.env.DOCS_FFMPEG
  return 'ffmpeg'
}

function assembleGif(outDir: string, framesDir: string, frameCount: number, gifName: string) {
  const gifPath = path.join(outDir, gifName)
  const inputPattern = path.join(framesDir, 'frame-%02d.png')
  const vf = 'scale=960:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128[p];[s1][p]paletteuse=dither=bayer'
  const ffmpeg = resolveFfmpegCommand()
  execFileSync(ffmpeg, [
    '-y', '-framerate', '2', '-i', inputPattern,
    '-frames:v', String(Math.max(1, frameCount)),
    '-vf', vf, '-loop', '0', gifPath,
  ], { stdio: 'ignore' })
}

async function captureSimpleGif(
  page: Page,
  outDir: string,
  opts: {
    gifName: string
    framesSubdir: string
    frameCount: number
    intervalMs: number
    setup: () => Promise<void>
  },
) {
  await opts.setup()
  const framesDir = path.join(outDir, opts.framesSubdir)
  fs.mkdirSync(framesDir, { recursive: true })
  for (const file of fs.readdirSync(framesDir)) {
    if (file.endsWith('.png')) fs.unlinkSync(path.join(framesDir, file))
  }
  for (let i = 0; i < opts.frameCount; i++) {
    await page.screenshot({
      path: path.join(framesDir, `frame-${String(i).padStart(2, '0')}.png`),
      fullPage: false,
      animations: 'disabled',
      type: 'png',
    })
    if (i + 1 < opts.frameCount) {
      await page.waitForTimeout(opts.intervalMs)
    }
  }
  assembleGif(outDir, framesDir, opts.frameCount, opts.gifName)
}

export async function captureDirectoryGif(page: Page, outDir: string) {
  await captureSimpleGif(page, outDir, {
    gifName: 'directory.gif',
    framesSubdir: 'directory-frames',
    frameCount: 14,
    intervalMs: 400,
    setup: async () => {
      await waitForDirectoryReady(page)
    },
  })
}

export async function captureChannelGif(page: Page, outDir: string) {
  await captureSimpleGif(page, outDir, {
    gifName: 'channel.gif',
    framesSubdir: 'channel-frames',
    frameCount: 16,
    intervalMs: 500,
    setup: async () => {
      const streamLink = await pickLiveChannelLink(page)
      const href = await streamLink.getAttribute('href')
      const login = href?.replace(/^\/c\//, '').split('/')[0] ?? channelLogin
      const finishReady = watchChannelNavigation(page, login)
      await streamLink.click()
      await page.waitForURL(/\/c\//, { timeout: 60_000 })
      await finishReady()
    },
  })
}

export async function captureDirectory(page: Page, outDir: string) {
  await waitForDirectoryReady(page)
  await shot(page, outDir, 'directory.png')
}

export async function captureChannel(page: Page, outDir: string) {
  const login = channelLogin
  const finishReady = watchChannelNavigation(page, login)
  await page.goto(`/c/${encodeURIComponent(login)}`)
  await page.waitForURL(/\/c\//, { timeout: 60_000 })
  try {
    await finishReady()
  } catch {
    const streamLink = await pickLiveChannelLink(page)
    const href = await streamLink.getAttribute('href')
    const fallbackLogin = href?.replace(/^\/c\//, '').split('/')[0] ?? login
    const finishFallback = watchChannelNavigation(page, fallbackLogin)
    await streamLink.click()
    await page.waitForURL(/\/c\//, { timeout: 30_000 })
    await finishFallback()
  }
  await shot(page, outDir, `channel-${login}.png`)
}

export async function captureDirectoryCategory(page: Page, outDir: string) {
  await waitForDirectoryReady(page)
  const category = page.locator('button').filter({ has: page.locator('img[alt]') }).first()
  await category.waitFor({ state: 'visible', timeout: 30_000 })
  await category.click()
  await page.waitForResponse(res => res.url().includes('/v1/categories/') && res.ok(), { timeout: 30_000 }).catch(() => undefined)
  await shot(page, outDir, 'directory-category.png')
}

export async function captureDirectorySearch(page: Page, outDir: string, term = 'jynxzi') {
  await waitForDirectoryReady(page)
  await page.getByPlaceholder('Search channels or categories').fill(term)
  await page.waitForResponse(res => res.url().includes('/v1/search') && res.ok(), { timeout: 30_000 }).catch(() => undefined)
  await page.getByRole('heading', { level: 1, name: new RegExp(`Search:\\s*${term}`, 'i') }).waitFor({ timeout: 30_000 }).catch(() => undefined)
  await shot(page, outDir, 'directory-search.png')
}

export async function captureChannelTab(page: Page, tab: 'emotes' | 'stats', outDir: string, fileName: string) {
  await page.getByRole('button', { name: tab === 'emotes' ? 'Emotes' : 'Stats', exact: true }).click()
  await page.waitForTimeout(500)
  await shot(page, outDir, fileName)
}

export async function captureChannelLivePlayback(page: Page, outDir: string) {
  const streamLink = await pickLiveChannelLink(page)
  const href = await streamLink.getAttribute('href')
  const login = href?.replace(/^\/c\//, '').split('/')[0] ?? channelLogin
  const finishReady = watchChannelNavigation(page, login)
  await streamLink.click()
  await page.waitForURL(/\/c\//, { timeout: 30_000 })
  await finishReady().catch(() => undefined)
  await shot(page, outDir, 'channel-live.png')
}
