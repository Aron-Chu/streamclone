export const ONBOARDING_DISMISSED_KEY = 'streamclone-onboarding-v1'
export const ONBOARDING_DISMISSED_INSTALL_KEY = 'streamclone-onboarding-install-id'
export const SCRAPER_BANNER_DISMISSED_KEY = 'streamclone-scraper-banner-dismissed'
export const CORE_ANALYTICS_BANNER_DISMISSED_KEY = 'streamclone-core-analytics-banner-dismissed'

export function getOnboardingDismissedInstallId(): string {
  if (typeof window === 'undefined') return ''
  return window.localStorage.getItem(ONBOARDING_DISMISSED_INSTALL_KEY) ?? ''
}

export function isOnboardingDismissed(installId = '') {
  if (typeof window === 'undefined') return false
  const scoped = installId.trim()
  if (scoped) {
    return getOnboardingDismissedInstallId() === scoped
  }
  return window.localStorage.getItem(ONBOARDING_DISMISSED_KEY) === '1'
}

export function markOnboardingDismissed(installId = '') {
  if (typeof window === 'undefined') return
  const scoped = installId.trim()
  if (scoped) {
    window.localStorage.setItem(ONBOARDING_DISMISSED_INSTALL_KEY, scoped)
  }
  window.localStorage.setItem(ONBOARDING_DISMISSED_KEY, '1')
}
