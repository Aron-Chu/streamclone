import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getSetupControlHealth,
  getSetupWelcome,
  startSetupService,
  type SetupWelcome,
} from '../api'
import { profileNeedsScraper, SCRAPER_SETUP_DOC_URL } from '../setupProfile'

const SCRAPER_BANNER_DISMISSED_KEY = 'streamclone-scraper-banner-dismissed'
const CORE_ANALYTICS_BANNER_DISMISSED_KEY = 'streamclone-core-analytics-banner-dismissed'

export default function ServiceStatusBanner() {
  const queryClient = useQueryClient()
  const [dismissedScraper, setDismissedScraper] = useState(
    () => typeof window !== 'undefined' && window.localStorage.getItem(SCRAPER_BANNER_DISMISSED_KEY) === '1',
  )
  const [dismissedCoreInfo, setDismissedCoreInfo] = useState(
    () => typeof window !== 'undefined' && window.localStorage.getItem(CORE_ANALYTICS_BANNER_DISMISSED_KEY) === '1',
  )
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const setup = useQuery({
    queryKey: ['setup-welcome'],
    queryFn: getSetupWelcome,
    staleTime: 10_000,
    refetchInterval: 15_000,
  })
  const control = useQuery({
    queryKey: ['setup-control-health'],
    queryFn: getSetupControlHealth,
    staleTime: 10_000,
    retry: false,
  })

  const profile = setup.data?.profile ?? 'core'
  const scraperOffline = setup.data?.services.scraper === 'offline'
  const controlReady = Boolean(control.data?.ok)
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

  const handleStart = async () => {
    setError(null)
    if (!controlReady) {
      setError('Run Start Streamclone from your Desktop shortcut, then try again.')
      return
    }
    setStarting(true)
    try {
      await startSetupService('scraper')
      for (let attempt = 0; attempt < 15; attempt++) {
        await new Promise(resolve => window.setTimeout(resolve, 2000))
        await queryClient.invalidateQueries({ queryKey: ['setup-welcome'] })
        const latest = queryClient.getQueryData<SetupWelcome>(['setup-welcome'])
        if (latest?.services.scraper === 'ready') {
          dismissScraper()
          return
        }
      }
      setError('Scraper is still starting. Check Docker Desktop and retry in a minute.')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to start scraper.')
    } finally {
      setStarting(false)
    }
  }

  if (showCoreInfoBanner) {
    return (
      <div className="mb-4 rounded-lg border border-cyan-300/20 bg-cyan-400/10 px-3 py-2.5 sm:px-4">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-xs font-semibold leading-5 text-cyan-50/90 sm:text-sm">
            Core Watch is active — minute-level viewer charts need the optional Analytics (scraper) tier.
          </p>
          <div className="flex shrink-0 items-center gap-2">
            <a
              href={SCRAPER_SETUP_DOC_URL}
              target="_blank"
              rel="noreferrer"
              className="rounded border border-cyan-200/30 bg-cyan-300/15 px-2.5 py-1 text-[11px] font-black uppercase tracking-wide text-cyan-50 transition hover:bg-cyan-300/25"
            >
              Analytics setup
            </a>
            <button
              type="button"
              onClick={dismissCoreInfo}
              className="rounded px-2 py-1 text-[11px] font-bold text-cyan-100/70 transition hover:bg-cyan-300/10 hover:text-cyan-50"
            >
              Dismiss
            </button>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="mb-4 rounded-lg border border-amber-300/20 bg-amber-400/10 px-3 py-2.5 sm:px-4">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-xs font-semibold leading-5 text-amber-50/90 sm:text-sm">
          Viewer chart sync is paused — the scraper is not running.
        </p>
        <div className="flex shrink-0 items-center gap-2">
          {controlReady ? (
            <button
              type="button"
              onClick={() => void handleStart()}
              disabled={starting}
              className="rounded border border-amber-200/30 bg-amber-300/15 px-2.5 py-1 text-[11px] font-black uppercase tracking-wide text-amber-50 transition hover:bg-amber-300/25 disabled:opacity-50"
            >
              {starting ? 'Starting…' : 'Start scraper'}
            </button>
          ) : null}
          <button
            type="button"
            onClick={dismissScraper}
            className="rounded px-2 py-1 text-[11px] font-bold text-amber-100/70 transition hover:bg-amber-300/10 hover:text-amber-50"
          >
            Dismiss
          </button>
        </div>
      </div>
      {error ? (
        <p className="mt-2 text-[11px] font-semibold text-amber-100/80">{error}</p>
      ) : null}
    </div>
  )
}
