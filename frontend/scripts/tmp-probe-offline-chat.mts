import { chromium } from '@playwright/test'

const LOGIN = process.argv[2] ?? 'xqc'

async function main() {
  const browser = await chromium.launch({ headless: true })
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
  await page.goto(`https://www.twitch.tv/${LOGIN}`, { waitUntil: 'domcontentloaded', timeout: 60_000 })
  await page.waitForTimeout(6000)

  const result = await page.evaluate(() => {
    const sel = (s: string) => document.querySelector(s)
    const rect = (el: Element | null) => {
      if (!el) return null
      const r = el.getBoundingClientRect()
      return { w: Math.round(r.width), h: Math.round(r.height) }
    }
    const chatCol = sel('[data-test-selector="chat-room-component-layout"]')
    const chatRect = rect(chatCol)
    return {
      offline: Boolean(sel('[data-a-target="channel-offline-still-image"]') || sel('[data-a-target="channel-offline-header"]')),
      liveVideo: (() => {
        const v = document.querySelector('video')
        return Boolean(v && v.duration === Infinity)
      })(),
      chatColumn: chatRect,
      chatUsable: chatRect ? chatRect.w >= 160 && chatRect.h >= 160 : false,
      chatHeader: Boolean(sel('[data-a-target="chat-room-header"]')),
      chatHeaderLine: Boolean(sel('[data-a-target="chat-room-header-line"]')),
      chatMessages: Boolean(sel('[data-test-selector="chat-scrollable-area"]')),
      collapseLabel: sel('[data-a-target="right-column__toggle-collapse-btn"]')?.getAttribute('aria-label') ?? null,
      offlineChatBanner: document.body.innerText.includes('offline') || document.body.innerText.includes('Offline'),
    }
  })

  console.log(JSON.stringify(result, null, 2))
  await browser.close()
}

main().catch(err => {
  console.error(err)
  process.exit(1)
})
