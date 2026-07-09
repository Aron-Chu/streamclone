import { chromium } from 'playwright'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const LOGIN = process.argv[2] ?? 'xqc'
const EXT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../../streamclone-pulse/dist')

async function readState(page) {
  return page.evaluate(() => {
    function host(id) {
      const el = document.getElementById(id)
      if (!el) return { present: false }
      const r = el.getBoundingClientRect()
      const mount = el.shadowRoot?.querySelector('.pulse-root')
      const text = mount?.textContent?.replace(/\s+/g, ' ').trim().slice(0, 300) ?? ''
      return {
        present: true,
        display: getComputedStyle(el).display,
        w: Math.round(r.width),
        h: Math.round(r.height),
        text,
      }
    }
    const column = document.querySelector('[data-test-selector="chat-room-component-layout"], .channel-root__right-column')
    const cr = column?.getBoundingClientRect()
    const collapseBtn = document.querySelector('[data-a-target="right-column__toggle-collapse-btn"]')
    const offlineStill = document.querySelector('[data-a-target="channel-offline-still-image"]')
    const offlineHeader = document.querySelector('[data-a-target="channel-offline-header"]')
    const chatInput = document.querySelector('[data-a-target="chat-input"], textarea[data-a-target="chat-input"]')
    const roleLog = column?.querySelector('[role="log"]')

    return {
      url: location.href,
      offlineStill: !!offlineStill,
      offlineHeader: !!offlineHeader,
      collapseLabel: collapseBtn?.getAttribute('aria-label') ?? null,
      column: cr ? { w: Math.round(cr.width), h: Math.round(cr.height) } : null,
      chatInputVisible: chatInput ? getComputedStyle(chatInput).display !== 'none' : false,
      roleLogH: roleLog ? Math.round(roleLog.getBoundingClientRect().height) : 0,
      tabs: host('streamclone-pulse-tabs'),
      panel: host('streamclone-pulse-root'),
    }
  })
}

async function clickIntoChat(page) {
  // Offline channel pages often show the same URL; chat may be collapsed or not focused.
  const expand = page.locator('[data-a-target="right-column__toggle-collapse-btn"][aria-label*="Expand" i]').first()
  if (await expand.isVisible({ timeout: 2000 }).catch(() => false)) {
    await expand.click()
    await page.waitForTimeout(1500)
    return 'expanded-collapsed-chat'
  }

  // Click chat column / stream chat header area to focus chat on offline pages
  const chatHeader = page.locator('[data-a-target="chat-room-header"], [data-test-selector="chat-room-header"]').first()
  if (await chatHeader.isVisible({ timeout: 2000 }).catch(() => false)) {
    await chatHeader.click()
    await page.waitForTimeout(1500)
    return 'clicked-chat-header'
  }

  const chatColumn = page.locator('[data-test-selector="chat-room-component-layout"], .channel-root__right-column').first()
  if (await chatColumn.isVisible({ timeout: 2000 }).catch(() => false)) {
    await chatColumn.click({ position: { x: 80, y: 120 } })
    await page.waitForTimeout(1500)
    return 'clicked-chat-column'
  }

  // Offline pages sometimes show a "Chat" tab or community panel entry
  const chatTab = page.getByRole('button', { name: /^chat$/i }).first()
  if (await chatTab.isVisible({ timeout: 2000 }).catch(() => false)) {
    await chatTab.click()
    await page.waitForTimeout(1500)
    return 'clicked-chat-tab'
  }

  return 'no-chat-click-target'
}

async function main() {
  const context = await chromium.launchPersistentContext('', {
    headless: false,
    args: [`--disable-extensions-except=${EXT}`, `--load-extension=${EXT}`],
    viewport: { width: 1440, height: 900 },
  })
  const page = context.pages()[0] ?? await context.newPage()

  await page.goto(`https://www.twitch.tv/${LOGIN}`, { waitUntil: 'domcontentloaded', timeout: 90_000 })
  await page.waitForTimeout(8000)

  const before = await readState(page)
  const clickAction = await clickIntoChat(page)
  await page.waitForTimeout(5000)
  const after = await readState(page)

  console.log(JSON.stringify({ login: LOGIN, clickAction, before, after }, null, 2))
  await context.close()
}

main().catch(err => {
  console.error(err)
  process.exit(1)
})
