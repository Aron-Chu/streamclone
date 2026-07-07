import type { AnalyticsMinuteRollup } from '../apiTypes.ts'

export function parseMomentHash(hash: string): number | null {
  const params = new URLSearchParams(hash.replace(/^#/, ''))
  const raw = params.get('t')
  if (!raw) return null
  const seconds = Number(raw)
  return Number.isFinite(seconds) && seconds >= 0 ? seconds : null
}

/** Reads `#t=` (portal) or `?offset=` / `?t=` (extension / legacy) deep links. */
export function parseDeepLinkOffset(hash: string, search: string): number | null {
  const fromHash = parseMomentHash(hash)
  if (fromHash != null) return fromHash
  const query = search.startsWith('?') ? search.slice(1) : search
  if (!query) return null
  const params = new URLSearchParams(query)
  const raw = params.get('offset') ?? params.get('t')
  if (!raw) return null
  const seconds = Number(raw)
  return Number.isFinite(seconds) && seconds >= 0 ? seconds : null
}

export function rollupOffsetSeconds(rollup: AnalyticsMinuteRollup, startedAt?: string): number {
  if (!startedAt) return 0
  const startedMs = Date.parse(startedAt)
  const minuteMs = Date.parse(rollup.minuteTs)
  if (!Number.isFinite(startedMs) || !Number.isFinite(minuteMs)) return 0
  return Math.max(0, Math.round((minuteMs - startedMs) / 1000))
}

export function findNearestRollupByOffset(
  rollups: AnalyticsMinuteRollup[],
  startedAt: string | undefined,
  offsetSeconds: number,
): AnalyticsMinuteRollup | null {
  let best: AnalyticsMinuteRollup | null = null
  let bestDistance = Number.POSITIVE_INFINITY
  for (const rollup of rollups) {
    if (rollup.missing) continue
    const distance = Math.abs(rollupOffsetSeconds(rollup, startedAt) - offsetSeconds)
    if (distance < bestDistance) {
      best = rollup
      bestDistance = distance
    }
  }
  return best
}
