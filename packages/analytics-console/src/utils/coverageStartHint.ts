import { formatHeatOffset } from '@streamclone/pulse-core'

export const PULSE_STREAM_START_TOLERANCE_SEC = 120
export const COVERAGE_START_SOFT_MAX_SEC = 600
export const COVERAGE_TIER_ACTIVE_LIVE = 'active_live_coverage'

export type CoverageStartHintTone = 'none' | 'soft' | 'warn'

export interface CoverageStartHintInput {
  coverageStartOffsetSeconds: number
  trackedFromStart?: boolean
  canBackfill?: boolean
  coverageTier?: string | null
  tracking?: boolean
  isLive?: boolean
}

export interface CoverageStartHint {
  show: boolean
  tone: CoverageStartHintTone
  text: string
}

export function resolveCoverageStartHint(input: CoverageStartHintInput): CoverageStartHint {
  const offset = Math.max(0, input.coverageStartOffsetSeconds ?? 0)
  const trackedFromStart =
    input.trackedFromStart === true || offset <= PULSE_STREAM_START_TOLERANCE_SEC

  if (trackedFromStart) {
    return { show: false, tone: 'none', text: '' }
  }
  if (input.canBackfill) {
    return { show: false, tone: 'none', text: '' }
  }

  const inCapLive =
    Boolean(input.tracking)
    && input.isLive !== false
    && input.coverageTier === COVERAGE_TIER_ACTIVE_LIVE

  if (inCapLive && offset <= COVERAGE_START_SOFT_MAX_SEC) {
    return {
      show: true,
      tone: 'soft',
      text: `Live chat from ${formatHeatOffset(offset)} — earlier minutes need VOD replay`,
    }
  }

  return {
    show: true,
    tone: 'warn',
    text: `Rollups since ${formatHeatOffset(offset)} — tracking started after stream start`,
  }
}
