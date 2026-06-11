import { useState } from 'react'
import { profileNeedsScraper } from '../setupProfile'
import { CORE_ANALYTICS_BANNER_DISMISSED_KEY, SCRAPER_BANNER_DISMISSED_KEY } from '../onboardingStorage'
import { useOptionalServices } from '../hooks/useOptionalServices'
import OptionalServicesPanel from './OptionalServicesPanel'

export default function ServiceStatusBanner() {
  const { setup, profile, scraperOffline } = useOptionalServices()
  const [dismissedScraper, setDismissedScraper] = useState(
    () => typeof window !== 'undefined' && window.localStorage.getItem(SCRAPER_BANNER_DISMISSED_KEY) === '1',
  )
  const [dismissedCoreInfo, setDismissedCoreInfo] = useState(
    () => typeof window !== 'undefined' && window.localStorage.getItem(CORE_ANALYTICS_BANNER_DISMISSED_KEY) === '1',
  )

  const isCoreProfile = profile === 'core'
  const showScraperActionBanner = profileNeedsScraper(profile) && scraperOffline && !dismissedScraper
  const showCoreInfoBanner = isCoreProfile && scraperOffline && !dismissedCoreInfo

  if (setup.isLoading || (!showScraperActionBanner && !showCoreInfoBanner)) {
    return null
  }

  const dismissScraper = () => {
    window.localStorage.setItem(SCRAPER_BANNER_DISMISSED_KEY, '1')
    setDismissedScraper(true)
  }

  const dismissCoreInfo = () => {
    window.localStorage.setItem(CORE_ANALYTICS_BANNER_DISMISSED_KEY, '1')
    setDismissedCoreInfo(true)
  }

  return (
    <div className="mb-4">
      <OptionalServicesPanel variant="banner" focus="scraper" />
      <div className="mt-2 flex justify-end">
        <button
          type="button"
          onClick={showCoreInfoBanner ? dismissCoreInfo : dismissScraper}
          className="rounded px-2 py-1 text-[11px] font-bold text-zinc-400 transition hover:bg-white/10 hover:text-zinc-200"
        >
          Dismiss banner
        </button>
      </div>
    </div>
  )
}
