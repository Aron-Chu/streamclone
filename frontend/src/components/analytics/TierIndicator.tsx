import { STREAMPULSE_ANALYTICS_URL } from '../../setupProfile'
import { CoreMinuteChartsNotice } from '../OptionalServicesPanel'

/**
 * Analytics tier badge — desktop install is core-only; minute charts live on StreamPulse.
 */
export type AnalyticsTier = 'core' | 'checking'

export function useAnalyticsTier() {
  return {
    tier: 'core' as AnalyticsTier,
    isAnalyticsActive: false,
    isCore: true,
    scraperOffline: true,
    controlReady: false,
    isStarting: () => false,
    startService: async () => {},
  }
}

export default function TierIndicator() {
  return (
    <a
      href={STREAMPULSE_ANALYTICS_URL}
      target="_blank"
      rel="noreferrer"
      aria-label="Analytics tier: Core — minute charts on StreamPulse"
      title="Core tier locally — per-minute viewer, chat, and emote charts live on hosted StreamPulse."
      className="rounded px-2 py-1 bg-amber-500/15 text-amber-100 transition hover:bg-amber-500/25"
    >
      Core · StreamPulse
    </a>
  )
}

export function CoreTierChartGuidance({ compact = false }: { compact?: boolean }) {
  return <CoreMinuteChartsNotice compact={compact} />
}
