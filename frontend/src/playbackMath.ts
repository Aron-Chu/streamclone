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

export interface LiveDelayMetricsInput {
  latencyToLiveSec: number | null
  targetLatencySec: number | null
  behindLiveSec: number | null
}

export interface LiveDelayRelayInput {
  liveEdge?: number
  hlsProbe?: { targetDuration?: string }
}

export interface EndToEndLiveDelayResult {
  displayDelaySec: number | null
  relayDelaySec: number | null
  bufferDelaySec: number | null
  syncDriftSec: number | null
  tooltip: string
}

export type LiveDelayBreakdown = EndToEndLiveDelayResult

type RelaySource = LiveDelayRelayInput | {
  liveEdge?: number
  hlsProbe?: { targetDuration?: string }
} | null | undefined

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

export function parseHlsTargetDuration(targetDuration: string | undefined): number {
  const parsed = Number.parseFloat(String(targetDuration ?? '').trim())
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 2
}

export function computeEndToEndLiveDelaySec(
  metrics: LiveDelayMetricsInput,
  relay?: LiveDelayRelayInput | null,
): EndToEndLiveDelayResult {
  const latencyToLiveSec = finiteOrNull(metrics.latencyToLiveSec)
  const targetLatencySec = finiteOrNull(metrics.targetLatencySec)
  const behindLiveSec = finiteOrNull(metrics.behindLiveSec)
  const liveEdge = relay?.liveEdge
  const segmentDuration = parseHlsTargetDuration(relay?.hlsProbe?.targetDuration)
  const relayDelaySec = Number.isFinite(liveEdge) && liveEdge !== undefined && liveEdge > 0
    ? roundSec(liveEdge * segmentDuration)
    : null
  const bufferDelaySec = targetLatencySec
  const syncDriftSec = behindLiveSec

  let displayDelaySec: number | null
  if (latencyToLiveSec !== null && latencyToLiveSec >= 0.5) {
    displayDelaySec = latencyToLiveSec
  } else if (relayDelaySec !== null) {
    displayDelaySec = roundSec(relayDelaySec + (targetLatencySec ?? 0))
  } else {
    displayDelaySec = latencyToLiveSec ?? behindLiveSec
  }

  const tooltipParts = [
    relayDelaySec !== null ? `Relay ~${formatDelayPart(relayDelaySec)}` : null,
    bufferDelaySec !== null ? `buffer ~${formatDelayPart(bufferDelaySec)}` : null,
    syncDriftSec !== null ? `sync drift ~${formatDelayPart(syncDriftSec)}` : null,
  ].filter(Boolean)

  return {
    displayDelaySec,
    relayDelaySec,
    bufferDelaySec,
    syncDriftSec,
    tooltip: tooltipParts.length ? tooltipParts.join(' + ') : 'End-to-end live delay unavailable',
  }
}

function normalizeRelaySource(relay?: RelaySource): LiveDelayRelayInput | null {
  if (!relay) return null
  return {
    liveEdge: relay.liveEdge,
    hlsProbe: relay.hlsProbe,
  }
}

export function getLiveDelayBreakdown(
  metrics: LiveDelayMetricsInput,
  relay?: RelaySource,
): LiveDelayBreakdown {
  return computeEndToEndLiveDelaySec(metrics, normalizeRelaySource(relay))
}

export function formatLiveDelayTooltip(breakdown: LiveDelayBreakdown): string {
  return breakdown.tooltip
}

function finiteOrNull(value: number | null | undefined) {
  return Number.isFinite(value) ? Number(value) : null
}

function roundSec(value: number) {
  return Number(value.toFixed(2))
}

function formatDelayPart(value: number) {
  return `${value >= 10 ? Math.round(value) : value.toFixed(1)}s`
}
