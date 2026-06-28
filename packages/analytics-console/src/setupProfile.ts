export const SCRAPER_SETUP_DOC_URL =
  'https://github.com/Aron-Chu/streamclone/blob/master/docs/scraper-cloudflare-and-proxy.md'

export function profileNeedsScraper(profile: string) {
  return profile === 'scraper' || profile === 'full'
}

export function coreMinuteChartsNeedScraper(profile: string, scraperState: string) {
  return profile === 'core' && scraperState === 'offline'
}
