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
const gifMaxFrames = Number.parseInt(process.env.DOCS_GIF_MAX_FRAMES ?? '24', 10)
const gifMaxDurationMs = Number.parseInt(process.env.DOCS_GIF_MAX_DURATION_MS ?? '120000', 10)

const DOCS_FALLBACK_STREAM_ID = process.env.DOCS_FALLBACK_STREAM_ID ?? '319508098137'
const DOCS_FALLBACK_VOD_ID = process.env.DOCS_FALLBACK_VOD_ID ?? '2784828860'

function docsFallbackTarget() {
  console.warn(`Using docs fallback stream ${DOCS_FALLBACK_STREAM_ID} (set DOCS_GIF_SYNC_STREAM to override)`)
  return { streamId: DOCS_FALLBACK_STREAM_ID, vodId: DOCS_FALLBACK_VOD_ID, slug: DOCS_FALLBACK_STREAM_ID }
}

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

  return docsFallbackTarget()
}

export async function waitForAnalyticsStreams(page: Page) {
  await page.goto(`/analytics/${encodeURIComponent(channelLogin)}`)
  await page.getByText('Streams', { exact: true }).first().waitFor({ state: 'visible', timeout: 60_000 })
  await page.waitForResponse(
    res => res.url().includes('/v1/analytics/channels/') && res.url().includes('/streams') && res.ok(),
    { timeout: 60_000 },
  ).catch(() => undefined)
  await page.waitForResponse(
    res => res.url().includes('/streams/history') && res.ok(),
    { timeout: 60_000 },
  ).catch(() => undefined)
  await page.waitForFunction(() => {
    const links = document.querySelectorAll('a[href*="/analytics/"]')
    return links.length >= 1
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
  const synced = page.locator('a[href*="/analytics/"]').filter({ hasText: /Synced/i }).first()
  const anyStream = page.locator('a[href*="/analytics/"]').filter({ hasNotText: /Open Full Analytics/i }).first()
  const pick = (await synced.count()) > 0 ? synced : anyStream
  if (await pick.count() > 0) {
    await pick.click()
    await page.waitForResponse(
      res => res.url().includes('/v1/analytics/streams/') && res.ok(),
      { timeout: 60_000 },
    ).catch(() => undefined)
    await waitForChartData(page, 60_000).catch(() => undefined)
  }
  await shot(page, outDir, 'analytics-xqc-streams.png')
}

type SyncStatusBody = {
  phase?: string
  rollupsWritten?: number
  error?: string
  message?: string
  viewerStatus?: string
}

type PhaseGifOptions = {
  gifName: string
  framesSubdir: string
  streamId: string
  /** Only capture frames while sync is in one of these phases (empty = any active phase). */
  includePhases?: string[]
  /** Stop once a frame is captured during one of these phases (after minFrames). */
  stopAfterPhases?: string[]
  minFrames?: number
  maxFrames?: number
  maxDurationMs?: number
  /** Stop early if the chart SVG grows past this many paths (avoids finished charts in load GIFs). */
  maxChartPaths?: number
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

export async function startSync(streamId: string, vodId?: string, opts?: { viewersOnly?: boolean; forceChat?: boolean }) {
  const params = new URLSearchParams({ channel: channelLogin })
  if (vodId) params.set('vod_id', vodId)
  if (opts?.viewersOnly) params.set('viewers_only', 'true')
  if (opts?.forceChat) params.set('force_chat', 'true')
  const res = await fetch(`${BASE}/v1/analytics/streams/${encodeURIComponent(streamId)}/sync?${params}`, {
    method: 'POST',
  })
  if (!res.ok) {
    throw new Error(`Sync start failed: ${res.status} ${await res.text()}`)
  }
  return res.json()
}

export async function waitForSyncIdle(streamId: string, timeoutMs = 120_000) {
  const started = Date.now()
  while (Date.now() - started < timeoutMs) {
    const status = await getSyncStatus(streamId)
    if (!status) return
    if (status.phase === 'completed' || status.phase === 'failed' || status.phase === 'idle') return
    await new Promise(r => setTimeout(r, 1500))
  }
}

async function chartPathCount(page: Page): Promise<number> {
  return page.evaluate(() => document.querySelector('svg')?.querySelectorAll('path').length ?? 0)
}

async function hideClipperNoise(page: Page) {
  await page.addStyleTag({
    content: `
      [class*="clipper"], [data-clipper-panel] { display: none !important; }
    `,
  }).catch(() => undefined)
  await page.evaluate(() => {
    for (const node of document.querySelectorAll('button, h2, h3, div')) {
      const text = node.textContent?.trim() ?? ''
      if (text === 'Clipper Edits' || text.startsWith('Analytics Spike')) {
        let el: HTMLElement | null = node instanceof HTMLElement ? node : null
        for (let depth = 0; el && depth < 6; depth += 1) {
          if (el.tagName === 'ASIDE' || el.className.includes('col-span')) {
            el.style.display = 'none'
            break
          }
          el = el.parentElement
        }
      }
    }
  }).catch(() => undefined)
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

async function captureSyncFrame(page: Page, framesDir: string, index: number) {
  await prepareScreenshotViewport(page)
  await settleBeforeScreenshot(page)
  await page.screenshot({
    path: path.join(framesDir, `frame-${String(index).padStart(2, '0')}.png`),
    fullPage: false,
    animations: 'disabled',
    type: 'png',
  })
}

function assembleGif(outDir: string, framesDir: string, frameCount: number, gifName: string) {
  const gifPath = path.join(outDir, gifName)
  const inputPattern = path.join(framesDir, 'frame-%02d.png')
  const vf = 'scale=960:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128[p];[s1][p]paletteuse=dither=bayer'
  const framerate = frameCount >= 16 ? '2' : '1.5'
  try {
    if (process.env.DOCS_FFMPEG === 'docker') {
      const mount = outDir.replace(/\\/g, '/').replace(/^([A-Za-z]):/, (_, d) => `/${d.toLowerCase()}`)
      execSync(
        `docker run --rm -v "${mount}:/data" jrottenberg/ffmpeg:4.4-alpine -y -framerate ${framerate} -i /data/${framesSubdirName(framesDir)}/frame-%02d.png -vf "${vf}" /data/${gifName}`,
        { stdio: 'pipe', shell: true },
      )
    } else {
      execFileSync(resolveFfmpegCommand(), [
        '-y',
        '-framerate', framerate,
        '-i', inputPattern,
        '-vf', vf,
        gifPath,
      ], { stdio: 'pipe' })
    }
  } catch (err) {
    const hint = 'Install ffmpeg on PATH, set DOCS_FFMPEG, or run with DOCS_FFMPEG=docker'
    throw new Error(`${err instanceof Error ? err.message : 'ffmpeg failed'} — ${hint}`)
  }
}

function framesSubdirName(framesDir: string) {
  return path.basename(framesDir)
}

async function hideChartForLoadCapture(page: Page) {
  await page.addStyleTag({
    content: `
      .recharts-wrapper, .recharts-surface, main svg { visibility: hidden !important; }
    `,
  }).catch(() => undefined)
}

async function waitForFullChartData(page: Page, timeout = 180_000) {
  await waitForChartData(page, timeout)
  // Chat/emote lines need VOD comment sync; viewer chart alone is enough for README PNG.
  await page.waitForFunction(() => {
    const body = document.body.innerText
    return !/Syncing chart data/i.test(body)
  }, { timeout: 60_000 }).catch(() => undefined)
}

/** Capture frames during specific sync phases — for README load GIFs (not finished charts). */
async function capturePhaseGif(page: Page, outDir: string, opts: PhaseGifOptions) {
  await hideChartForLoadCapture(page)
  const framesDir = path.join(outDir, opts.framesSubdir)
  fs.mkdirSync(framesDir, { recursive: true })
  for (const file of fs.readdirSync(framesDir)) {
    if (file.endsWith('.png')) fs.unlinkSync(path.join(framesDir, file))
  }

  const minFrames = opts.minFrames ?? 10
  const maxFrames = opts.maxFrames ?? gifMaxFrames
  const maxDurationMs = opts.maxDurationMs ?? gifMaxDurationMs
  const include = opts.includePhases?.length ? new Set(opts.includePhases) : null
  const stopAfter = new Set(opts.stopAfterPhases ?? [])

  const started = Date.now()
  let frameIndex = 0
  let lastPhase = ''
  let lastCaptureAt = 0

  while (frameIndex < maxFrames && Date.now() - started < maxDurationMs) {
    const status = await getSyncStatus(opts.streamId)
    const phase = status?.phase ?? ''
    const paths = await chartPathCount(page)

    if (opts.maxChartPaths != null && paths >= opts.maxChartPaths && frameIndex >= minFrames) {
      console.log(`phase-gif ${opts.gifName}: stopping — chart has ${paths} paths`)
      break
    }

    if (status && (phase === 'completed' || phase === 'failed' || phase === 'idle')) {
      if (frameIndex >= minFrames) break
    }

    const phaseAllowed = !include || include.has(phase) || phase === '' || phase === 'starting'
    const phaseChanged = phase !== lastPhase
    const elapsedSinceCapture = Date.now() - lastCaptureAt

    if (phaseAllowed && (frameIndex === 0 || phaseChanged || elapsedSinceCapture >= 2000)) {
      const panelVisible = await page.getByText(/Sync progress|Syncing chart data|Scraping TwitchTracker/i).first().isVisible().catch(() => false)
      if (!panelVisible && frameIndex > 0) {
        await page.waitForTimeout(800)
        continue
      }
      await captureSyncFrame(page, framesDir, frameIndex)
      frameIndex += 1
      lastCaptureAt = Date.now()
      console.log(`phase-gif ${opts.gifName}: frame ${frameIndex} phase=${phase || 'pending'} paths=${paths}`)
      if (stopAfter.has(phase) && frameIndex >= minFrames) break
    }

    lastPhase = phase
    await page.waitForTimeout(800)
  }

  if (frameIndex === 0) {
    await captureSyncFrame(page, framesDir, 0)
    frameIndex = 1
  }

  assembleGif(outDir, framesDir, frameIndex, opts.gifName)
}

async function fetchDocsSyncTarget(): Promise<{ streamId: string; vodId?: string; slug: string }> {
  if (process.env.DOCS_GIF_SYNC_STREAM?.trim()) {
    const streamId = process.env.DOCS_GIF_SYNC_STREAM.trim()
    return { streamId, vodId: process.env.DOCS_GIF_SYNC_VOD?.trim(), slug: streamId }
  }

  const historyRes = await fetch(`${BASE}/v1/channels/${encodeURIComponent(channelLogin)}/streams/history?period=all`)
  const history = await historyRes.json() as { items?: StreamHistoryItem[] }
  const pick = (history.items ?? [])
    .filter(item => item.id && /^\d+$/.test(item.id))
    .filter(item => (item.durationMinutes ?? 0) >= 90 && (item.durationMinutes ?? 0) <= 240)
    .sort((a, b) => (b.durationMinutes ?? 0) - (a.durationMinutes ?? 0))[0]
    ?? (history.items ?? []).find(item => item.id && /^\d+$/.test(item.id))

  if (pick?.id) {
    return { streamId: pick.id, vodId: pick.videoId, slug: pick.id }
  }
  return docsFallbackTarget()
}

async function waitForSyncPanel(page: Page, timeoutMs = 90_000) {
  await page.getByText(/Sync progress|Syncing chart data|Scraping TwitchTracker/i).first().waitFor({
    state: 'visible',
    timeout: timeoutMs,
  })
}

async function clickSyncIfIdle(page: Page) {
  const btn = page.getByRole('button', { name: /sync historical data|sync chat\/emotes|sync chat/i })
  if (await btn.count() > 0) {
    await btn.first().click()
  }
}

/** Capture two README load GIFs: TwitchTracker scrape first, then initial sync (no finished charts). */
export async function captureAnalyticsMedia(page: Page, outDir: string) {
  await prepareScreenshotViewport(page)
  await hideClipperNoise(page)
  const target = await fetchDocsSyncTarget()
  console.log(`docs sync target: ${target.streamId} vod=${target.vodId ?? 'auto'}`)

  await waitForSyncIdle(target.streamId, 30_000)

  // 1) TwitchTracker scrape — viewers-only sync before any chart exists
  await startSync(target.streamId, target.vodId, { viewersOnly: true })
  await openAnalyticsStream(page, target.slug)
  await waitForSyncPanel(page)
  await capturePhaseGif(page, outDir, {
    gifName: 'analytics-tt-scrape.gif',
    framesSubdir: '.tt-scrape-frames',
    streamId: target.streamId,
    includePhases: ['starting', 'scraping_tracker', 'parsing_tracker'],
    stopAfterPhases: ['parsing_tracker'],
    minFrames: 10,
    maxDurationMs: 180_000,
    maxChartPaths: 1,
  })

  await waitForSyncIdle(target.streamId, 180_000)
  await page.reload()
  await hideClipperNoise(page)

  // 2) Initial sync load — force chat re-index, stop before chart fills in
  await openAnalyticsStream(page, target.slug)
  await clickSyncIfIdle(page)
  await startSync(target.streamId, target.vodId, { forceChat: true })
  await waitForSyncPanel(page).catch(async () => {
    await clickSyncIfIdle(page)
    await waitForSyncPanel(page)
  })

  await capturePhaseGif(page, outDir, {
    gifName: 'analytics-sync-load.gif',
    framesSubdir: '.sync-load-frames',
    streamId: target.streamId,
    includePhases: ['starting', 'scraping_tracker', 'parsing_tracker', 'resolving_vod', 'fetching_comments', 'writing_rollups'],
    stopAfterPhases: ['resolving_vod', 'fetching_comments'],
    minFrames: 12,
    maxDurationMs: 120_000,
    maxChartPaths: 2,
  })

  // 3) Finish sync, then capture static PNGs (instant chart + streams list — not in GIFs)
  await waitForSyncIdle(target.streamId, 600_000)
  const active = await getSyncStatus(target.streamId)
  if (active) {
    await pollSyncStatus(target.streamId, 900_000)
  }
  await startSync(target.streamId, target.vodId, { forceChat: true })
  await waitForSyncIdle(target.streamId, 900_000)
  await page.reload()
  await hideClipperNoise(page)
  await openAnalyticsStream(page, target.slug)
  await waitForFullChartData(page, 180_000)
  await page.getByText(/Syncing chart data/i).waitFor({ state: 'hidden', timeout: 120_000 }).catch(() => undefined)
  await captureAnalyticsChart(page, outDir)
  await captureAnalyticsStreamsList(page, outDir)
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
