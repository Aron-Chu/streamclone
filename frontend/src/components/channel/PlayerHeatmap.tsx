import { useEffect, useState } from 'react'

import type { AnalyticsMinuteRollup } from '../../api'
import ActivityWaveform from '../analytics/ActivityWaveform'

export interface PlayerHeatmapProps {
  rollups: AnalyticsMinuteRollup[] | null
  totalDurationSec: number
  isLoading?: boolean
  isError?: boolean
  onSeek?: (offsetSeconds: number) => void
  className?: string
}

const LOAD_TIMEOUT_MS = 5000

/**
 * Compact activity waveform strip above the VOD progress bar.
 * Rollups are fetched upstream when the deep link carries analytics context
 * (`from=analytics` + `sid`).
 */
export function PlayerHeatmap({
  rollups,
  totalDurationSec,
  isLoading,
  isError,
  onSeek,
  className,
}: PlayerHeatmapProps) {
  const [timedOut, setTimedOut] = useState(false)

  useEffect(() => {
    if (!isLoading) return
    const timer = setTimeout(() => setTimedOut(true), LOAD_TIMEOUT_MS)
    return () => clearTimeout(timer)
  }, [isLoading])

  useEffect(() => {
    if (rollups !== null) setTimedOut(false)
  }, [rollups])

  if (rollups === null || isError || timedOut) {
    return null
  }

  if (!rollups.length || !Number.isFinite(totalDurationSec) || totalDurationSec <= 0) {
    return null
  }

  return (
    <ActivityWaveform
      rollups={rollups}
      totalDurationSec={totalDurationSec}
      variant="player"
      showLayerToggles={false}
      onSeek={onSeek}
      className={className}
    />
  )
}

export default PlayerHeatmap
