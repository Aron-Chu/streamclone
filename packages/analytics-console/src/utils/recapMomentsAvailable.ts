import { mergeRecapMoments } from '@streamclone/pulse-core'
import type { PulseStreamRecap } from '../apiTypes.ts'
import { MOMENTS_MAX_VISIBLE } from './momentListDisplay.tsx'

export function recapHasReactionCoverage(recap: PulseStreamRecap): boolean {
  const moments = [...(recap.topMoments ?? []), ...(recap.clipCandidates ?? [])]
  return (
    moments.some(
      (moment) => (moment.chatCount ?? 0) > 0 || (moment.emoteCount ?? 0) > 0,
    ) || (recap.totalMessages ?? 0) > 0
  )
}

/** True when recap API yields at least one ranked moment row for the session. */
export function hasRecapMomentsAvailable(recap: PulseStreamRecap): boolean {
  const hasReactionCoverage = recapHasReactionCoverage(recap)
  return mergeRecapMoments(recap, undefined, MOMENTS_MAX_VISIBLE, hasReactionCoverage).length > 0
}
