import { displayMomentReasonLabel } from './momentScore.ts'

export interface RecapMomentLike {
  offsetSeconds: number
  score: number
  reasons?: string[]
  chatCount?: number
  emoteCount?: number
  topEmotes?: Array<{ code: string; count: number; provider?: string }>
}

export interface RecapPeakLike {
  offsetSeconds: number
  score: number
  reasons?: string[]
  chatCount?: number
  emoteCount?: number
  topEmotes?: Array<{ name: string; count: number; provider?: string }>
}

const MOMENT_DEDUPE_TOLERANCE_SECONDS = 60

function momentHasReactionData(moment: RecapMomentLike): boolean {
  return (moment.chatCount ?? 0) > 0 || (moment.emoteCount ?? 0) > 0
}

function streamHasReactionCoverage(hasReactionCoverage?: boolean): boolean {
  return hasReactionCoverage ?? false
}

function compareMomentRank(
  a: RecapMomentLike,
  b: RecapMomentLike,
  hasReactionCoverage: boolean,
): number {
  if (hasReactionCoverage) {
    const rankA = momentHasReactionData(a) ? 0 : 1
    const rankB = momentHasReactionData(b) ? 0 : 1
    if (rankA !== rankB) return rankA - rankB
  }
  if (a.score !== b.score) return b.score - a.score
  return a.offsetSeconds - b.offsetSeconds
}

function sortMomentsByRank(
  moments: RecapMomentLike[],
  hasReactionCoverage: boolean,
): RecapMomentLike[] {
  return [...moments].sort((a, b) => compareMomentRank(a, b, hasReactionCoverage))
}

/** Merge recap moments, clip candidates, and optional peaks into one ranked list. */
export function mergeRecapMoments(
  recap:
    | {
        topMoments?: readonly RecapMomentLike[]
        clipCandidates?: readonly RecapMomentLike[]
      }
    | null
    | undefined,
  peaks: readonly RecapPeakLike[] | undefined,
  limit = 20,
  hasReactionCoverage = false,
): RecapMomentLike[] {
  const candidates: RecapMomentLike[] = []
  for (const moment of recap?.topMoments ?? []) {
    candidates.push(moment)
  }
  for (const moment of recap?.clipCandidates ?? []) {
    candidates.push(moment)
  }
  for (const peak of peaks ?? []) {
    candidates.push({
      offsetSeconds: peak.offsetSeconds,
      score: peak.score,
      reasons: peak.reasons,
      chatCount: peak.chatCount,
      emoteCount: peak.emoteCount,
      topEmotes: peak.topEmotes?.map(emote => ({
        code: emote.name,
        count: emote.count,
        provider: emote.provider,
      })),
    })
  }

  candidates.sort((a, b) => compareMomentRank(a, b, streamHasReactionCoverage(hasReactionCoverage)))
  const merged: RecapMomentLike[] = []
  for (const moment of candidates) {
    const duplicate = merged.find(
      existing => Math.abs(existing.offsetSeconds - moment.offsetSeconds) <= MOMENT_DEDUPE_TOLERANCE_SECONDS,
    )
    if (duplicate) {
      if (compareMomentRank(moment, duplicate, streamHasReactionCoverage(hasReactionCoverage)) < 0) {
        const index = merged.indexOf(duplicate)
        merged[index] = moment
      }
      continue
    }
    merged.push(moment)
    if (merged.length >= limit) break
  }
  return sortMomentsByRank(merged, streamHasReactionCoverage(hasReactionCoverage))
}

export function recapMomentReasonLabel(moment: RecapMomentLike): string {
  const reason = moment.reasons?.[0] ?? ''
  return displayMomentReasonLabel(reason)
}
