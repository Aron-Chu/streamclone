import { useSystemHealth } from '../../hooks/useSystemHealth'
import { CoreMinuteChartsNotice } from '../OptionalServicesPanel'

/**
 * Analytics tier indicator (Requirement 38).
 *
 * Derives the current analytics profile state (Core vs Analytics active) from
 * the existing `useSystemHealth` / `useOptionalServices` hooks rather than a
 * separate health probe (Requirement 38.5), so we never duplicate compose-level
 * health checks.
 *
 * - "Analytics active" — scraper service is reachable and the analytics profile
 *   is active; minute-level viewer charts are available.
 * - "Core" — scraper not reachable or compose profile excludes the scraper;
 *   minute charts require starting the Analytics tier via setup-control.
 * - "checking" — health data still loading; render a neutral placeholder so the
 *   badge does not flash "Core" before status resolves.
 */
export type AnalyticsTier = 'core' | 'analytics' | 'checking'

export function useAnalyticsTier() {
  const health = useSystemHealth()
  const scraper = health.services?.scraper
  const loading =
    health.setup.isLoading &&
    health.diagnostics.isLoading &&
    !health.setup.data &&
    !health.diagnostics.data

  const tier: AnalyticsTier = scraper === 'ready' ? 'analytics' : loading ? 'checking' : 'core'

  return {
    tier,
    isAnalyticsActive: tier === 'analytics',
    isCore: tier === 'core',
    scraperOffline: health.scraperOffline,
    controlReady: health.controlReady,
    isStarting: health.isStarting('scraper'),
    startService: health.startService,
  }
}

/**
 * Header badge showing whether the analytics tier is Core or Analytics active
 * (Requirements 38.1, 38.4). Positioned in the analytics header alongside the
 * stream title and source quality badges; it MUST NOT be duplicated in stat
 * cards or the chart area.
 */
export default function TierIndicator() {
  const { tier } = useAnalyticsTier()

  if (tier === 'checking') {
    return (
      <span
        aria-label="Analytics tier: checking"
        className="rounded px-2 py-1 bg-white/10 text-zinc-400"
      >
        Tier…
      </span>
    )
  }

  const isAnalytics = tier === 'analytics'
  return (
    <span
      aria-label={`Analytics tier: ${isAnalytics ? 'Analytics active' : 'Core'}`}
      title={
        isAnalytics
          ? 'Analytics tier active — minute-level viewer, chat, and emote charts are available.'
          : 'Core tier — minute-level viewer charts require the Analytics (scraper) profile.'
      }
      className={`rounded px-2 py-1 ${
        isAnalytics ? 'bg-emerald-500/15 text-emerald-100' : 'bg-amber-500/15 text-amber-100'
      }`}
    >
      {isAnalytics ? 'Analytics active' : 'Core'}
    </span>
  )
}

/**
 * Empty-state guidance for the Core tier (Requirement 38.2). When the tier is
 * Core, the chart empty state should explain that minute-level viewer charts
 * require the Analytics tier (scraper profile) and provide an action to start
 * it via setup-control. Renders nothing when the Analytics tier is active so
 * the chart can follow standard sync/collecting messaging (Requirement 38.3).
 */
export function CoreTierChartGuidance({ compact = false }: { compact?: boolean }) {
  const { isCore } = useAnalyticsTier()
  if (!isCore) return null
  return <CoreMinuteChartsNotice compact={compact} />
}
