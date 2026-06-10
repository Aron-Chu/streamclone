import { execFileSync, execSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import type { Page } from '@playwright/test'
import {
  pickLiveChannelLink,
  settleBeforeScreenshot,
  watchChannelNavigation,
  waitForDirectoryReady,
} from './screenshot-helpers'

const BASE = process.env.DOCS_BASE_URL ?? 'http://localhost:8090'

export const channelLogin = process.env.DOCS_CHANNEL_LOGIN ?? 'xqc'
export const analyticsStreamHint = process.env.DOCS_ANALYTICS_STREAM?.trim() ?? ''
export const skipSync = process.env.DOCS_SKIP_SYNC === '1'

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

type StreamHistoryItem = {
  id: string
  startedAt?: string
  durationMinutes?: number
  title?: string
  videoId?: string
}

type AnalyticsStreamItem = {
  id: string
  startedAt?: string
  title?: string
}

export async function fetchTargetStreamId(): Promise<{ streamId: string; vodId?: string; slug: string }> {
  if (analyticsStreamHint) {
    return { streamId: analyticsStreamHint, slug: analyticsStreamHint }
  }

  const historyRes = await fetch(`${BASE}/v1/channels/${encodeURIComponent(channelLogin)}/streams/history?period=all`)
  const history = await historyRes.json() as { items?: StreamHistoryItem[] }
  const items = history.items ?? []
  const pick = items
    .filter(item => item.id && /^\d+$/.test(item.id))
    .sort((a, b) => {
      const dur = (b.durationMinutes ?? 0) - (a.durationMinutes ?? 0)
      if (dur !== 0) return dur
      return (b.startedAt ?? '').localeCompare(a.startedAt ?? '')
    })[0]
  if (pick?.id) {
    return { streamId: pick.id, vodId: pick.videoId, slug: pick.id }
  }

  const analyticsRes = await fetch(`${BASE}/v1/analytics/channels/${encodeURIComponent(channelLogin)}/streams?limit=20`)
  const analytics = await analyticsRes.json() as { items?: AnalyticsStreamItem[] }
  const fallback = analytics.items?.[0]
  if (fallback?.id) {
    return { streamId: fallback.id, slug: fallback.id }
  }

  throw new Error(`No historical streams found for ${channelLogin}`)
}

export async function waitForAnalyticsStreams(page: Page) {
  await page.goto(`/analytics/${encodeURIComponent(channelLogin)}`)
  await page.getByText('Streams', { exact: true }).first().waitFor({ state: 'visible', timeout: 60_000 })
  await page.waitForResponse(
    res => res.url().includes('/streams/history') && res.ok(),
    { timeout: 60_000 },
  ).catch(() => undefined)
  await page.waitForFunction(() => {
    const links = document.querySelectorAll('a[href^="/analytics/"]')
    return links.length >= 2
  }, { timeout: 60_000 }).catch(() => undefined)
  await settleBeforeScreenshot(page)
}

export async function openAnalyticsStream(page: Page, slug: string) {
  await page.goto(`/analytics/${encodeURIComponent(channelLogin)}/${encodeURIComponent(slug)}`)
  await page.waitForResponse(
    res => res.url().includes('/v1/analytics/streams/') && res.ok(),
    { timeout: 60_000 },
  ).catch(() => undefined)
  await settleBeforeScreenshot(page)
}

export async function captureAnalyticsStreamsList(page: Page, outDir: string) {
  await waitForAnalyticsStreams(page)
  await shot(page, outDir, 'analytics-xqc-streams.png')
}

type SyncStatusBody = {
  phase?: string
  rollupsWritten?: number
  error?: string
  message?: string
}

export async function getSyncStatus(streamId: string): Promise<SyncStatusBody | null> {
  const res = await fetch(`${BASE}/v1/analytics/streams/${encodeURIComponent(streamId)}/sync/status`)
  if (!res.ok) return null
  const body = await res.json() as SyncStatusBody
  if (!body.phase || body.phase === 'idle') return null
  return body
}

export async function pollSyncStatus(streamId: string, timeoutMs = 900_000) {
  const started = Date.now()
  let lastPhase = ''
  while (Date.now() - started < timeoutMs) {
    const body = await getSyncStatus(streamId)
    if (body) {
      if (body.phase !== lastPhase) {
        lastPhase = body.phase ?? ''
        console.log(`sync phase: ${body.phase} rollups=${body.rollupsWritten ?? 0}`)
      }
      if (body.phase === 'completed' || body.phase === 'failed') {
        return body
      }
    }
    await new Promise(r => setTimeout(r, 2000))
  }
  throw new Error(`Sync timed out after ${timeoutMs}ms`)
}

export async function startSync(streamId: string, vodId?: string) {
  const params = new URLSearchParams({ channel: channelLogin })
  if (vodId) params.set('vod_id', vodId)
  const res = await fetch(`${BASE}/v1/analytics/streams/${encodeURIComponent(streamId)}/sync?${params}`, {
    method: 'POST',
  })
  if (!res.ok) {
    throw new Error(`Sync start failed: ${res.status} ${await res.text()}`)
  }
  return res.json()
}

export async function waitForChartData(page: Page, timeout = 120_000) {
  await page.waitForFunction(() => {
    const svg = document.querySelector('svg')
    if (!svg) return false
    const paths = svg.querySelectorAll('path')
    return paths.length >= 3
  }, { timeout }).catch(() => undefined)
}

export function resolveFfmpegCommand(): string {
  if (process.env.DOCS_FFMPEG) return process.env.DOCS_FFMPEG
  return 'ffmpeg'
}

export async function captureSyncGif(page: Page, outDir: string, framesDir: string) {
  fs.mkdirSync(framesDir, { recursive: true })
  for (const file of fs.readdirSync(framesDir)) {
    if (file.endsWith('.png')) fs.unlinkSync(path.join(framesDir, file))
  }

  const frameCount = 10
  for (let i = 0; i < frameCount; i += 1) {
    await page.waitForTimeout(1000)
    await prepareScreenshotViewport(page)
    await page.screenshot({
      path: path.join(framesDir, `frame-${String(i).padStart(2, '0')}.png`),
      fullPage: false,
      animations: 'disabled',
      type: 'png',
    })
  }

  const gifPath = path.join(outDir, 'analytics-xqc-sync.gif')
  const inputPattern = path.join(framesDir, 'frame-%02d.png')
  const vf = 'scale=1920:-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse'
  try {
    if (process.env.DOCS_FFMPEG === 'docker') {
      const mount = outDir.replace(/\\/g, '/').replace(/^([A-Za-z]):/, (_, d) => `/${d.toLowerCase()}`)
      execSync(
        `docker run --rm -v "${mount}:/data" jrottenberg/ffmpeg:4.4-alpine -y -framerate 1 -i /data/.sync-frames/frame-%02d.png -vf "${vf}" /data/analytics-xqc-sync.gif`,
        { stdio: 'pipe', shell: true },
      )
    } else {
      execFileSync(resolveFfmpegCommand(), [
        '-y',
        '-framerate', '1',
        '-i', inputPattern,
        '-vf', vf,
        gifPath,
      ], { stdio: 'pipe' })
    }
  } catch (err) {
    const hint = 'Install ffmpeg on PATH, set DOCS_FFMPEG, or run: docker exec streamclone-clipper-1 ffmpeg ...'
    throw new Error(`${err instanceof Error ? err.message : 'ffmpeg failed'} — ${hint}`)
  }
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

export async function captureAnalyticsStreamDetail(page: Page, slug: string, outDir: string) {
  await openAnalyticsStream(page, slug)
  await shot(page, outDir, 'analytics-stream-detail.png')
}

export async function captureAnalyticsChart(page: Page, outDir: string) {
  await waitForChartData(page)
  await shot(page, outDir, 'analytics-xqc-chart.png')
}

export function syncIsActive(status: SyncStatusBody | null) {
  if (!status?.phase) return false
  return status.phase !== 'completed' && status.phase !== 'failed' && status.phase !== 'idle'
}
