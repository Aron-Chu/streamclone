export const ONBOARDING_DISMISSED_KEY = 'streamclone-onboarding-v1'
export const SCRAPER_BANNER_DISMISSED_KEY = 'streamclone-scraper-banner-dismissed'
export const CORE_ANALYTICS_BANNER_DISMISSED_KEY = 'streamclone-core-analytics-banner-dismissed'

export function isOnboardingDismissed() {
  if (typeof window === 'undefined') return false
  return (
    window.localStorage.getItem(ONBOARDING_DISMISSED_KEY) === '1' ||
    window.sessionStorage.getItem(ONBOARDING_DISMISSED_KEY) === '1'
  )
}

export function markOnboardingDismissed() {
  window.localStorage.setItem(ONBOARDING_DISMISSED_KEY, '1')
  window.sessionStorage.setItem(ONBOARDING_DISMISSED_KEY, '1')
}
