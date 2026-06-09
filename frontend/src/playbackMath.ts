export interface LiveEdgeInput {
  currentTimeSec: number | null
  liveSyncPositionSec: number | null
  seekableEndSec: number | null
  targetLatencySec: number | null
}

export interface LiveEdgeResult {
  behindLiveSec: number | null
  canJumpLive: boolean
  jumpTargetSec: number | null
}

export function calculateLiveEdge(input: LiveEdgeInput): LiveEdgeResult {
  const currentTimeSec = finiteOrNull(input.currentTimeSec)
  const liveSyncPositionSec = finiteOrNull(input.liveSyncPositionSec)
  const seekableEndSec = finiteOrNull(input.seekableEndSec)
  const targetLatencySec = finiteOrNull(input.targetLatencySec)
  const referenceEndSec = liveSyncPositionSec ?? seekableEndSec
  const behindLiveSec = referenceEndSec !== null && currentTimeSec !== null
    ? roundSec(Math.max(0, referenceEndSec - currentTimeSec))
    : null
  const canJumpLive = behindLiveSec !== null && behindLiveSec > Math.max((targetLatencySec ?? 0) + 4, 10)
  let jumpTargetSec: number | null = null
  if (liveSyncPositionSec !== null) {
    jumpTargetSec = liveSyncPositionSec
  } else if (seekableEndSec !== null) {
    jumpTargetSec = roundSec(Math.max(0, seekableEndSec - Math.max(targetLatencySec ?? 1, 1)))
  }
  return { behindLiveSec, canJumpLive, jumpTargetSec }
}

function finiteOrNull(value: number | null | undefined) {
  return Number.isFinite(value) ? Number(value) : null
}

function roundSec(value: number) {
  return Number(value.toFixed(2))
}
