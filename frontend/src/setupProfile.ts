export const STREAMPULSE_ANALYTICS_URL = 'https://streampulse.stream/analytics'

/** Desktop install no longer bundles the TwitchTracker scraper profile. */
export function profileNeedsScraper(_profile: string) {
  return false
}

export function coreMinuteChartsNeedScraper(_profile: string, _scraperState: string) {
  return false
}
