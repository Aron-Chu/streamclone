/** Desktop install is core watch only (no bundled scraper tier). */
export function profileNeedsScraper(_profile: string) {
  return false
}

export function coreMinuteChartsNeedScraper(_profile: string, _scraperState: string) {
  return false
}
