export const CLIENT_NETWORK_MAX_SAMPLES = 60
export const CLIENT_NETWORK_PROBE_INTERVAL_MS = 2_000

export interface ClientNetworkSample {
  at: number
  downlinkMbps: number | null
  rttMs: number | null
  effectiveType: string | null
  saveData: boolean
  /** Measured download throughput from a same-origin probe (Mbps). */
  probeMbps: number | null
  probeFailed: boolean
}

export type ClientNetworkWarningKind =
  | 'high-utilization'
  | 'unstable-rtt'
  | 'probe-failures'
  | 'save-data'
  | 'slow-link'

export interface ClientNetworkWarning {
  kind: ClientNetworkWarningKind
  severity: 'info' | 'warn' | 'critical'
  message: string
}

export interface ClientNetworkAnalysis {
  latestProbeMbps: number | null
  latestDownlinkMbps: number | null
  utilizationPct: number | null
  avgRttMs: number | null
  rttJitterMs: number | null
  probeFailureRate: number
  warnings: ClientNetworkWarning[]
}

export async function measureProbeThroughput(probeUrl = '/logo.svg'): Promise<{ mbps: number | null; failed: boolean }> {
  const url = `${probeUrl}${probeUrl.includes('?') ? '&' : '?'}_netprobe=${Date.now()}`
  const started = performance.now()
  try {
    const response = await fetch(url, { cache: 'no-store' })
    if (!response.ok) return { mbps: null, failed: true }
    const blob = await response.blob()
    const elapsedSec = Math.max((performance.now() - started) / 1000, 0.001)
    const mbps = (blob.size * 8) / elapsedSec / 1_000_000
    return { mbps, failed: false }
  } catch {
    return { mbps: null, failed: true }
  }
}

function stdDev(values: number[]): number {
  if (values.length < 2) return 0
  const mean = values.reduce((sum, v) => sum + v, 0) / values.length
  const variance = values.reduce((sum, v) => sum + (v - mean) ** 2, 0) / values.length
  return Math.sqrt(variance)
}

export function analyzeClientNetwork(samples: ClientNetworkSample[]): ClientNetworkAnalysis {
  const recent = samples.slice(-CLIENT_NETWORK_MAX_SAMPLES)
  const latest = recent.length ? recent[recent.length - 1] : undefined
  const rttValues = recent.map(s => s.rttMs).filter((v): v is number => v != null && v > 0)
  const probeValues = recent.map(s => s.probeMbps).filter((v): v is number => v != null && v >= 0)
  const probeFailures = recent.filter(s => s.probeFailed).length
  const probeFailureRate = recent.length ? probeFailures / recent.length : 0
  const avgRttMs = rttValues.length ? rttValues.reduce((a, b) => a + b, 0) / rttValues.length : null
  const rttJitterMs = rttValues.length >= 3 ? stdDev(rttValues) : null
  const latestProbeMbps = latest?.probeMbps ?? (probeValues.length ? probeValues[probeValues.length - 1] : null)
  const latestDownlinkMbps = latest?.downlinkMbps ?? null
  const utilizationPct =
    latestProbeMbps != null && latestDownlinkMbps != null && latestDownlinkMbps > 0
      ? Math.min(100, (latestProbeMbps / latestDownlinkMbps) * 100)
      : null

  const warnings: ClientNetworkWarning[] = []

  if (latest?.saveData) {
    warnings.push({
      kind: 'save-data',
      severity: 'info',
      message: 'Save-Data mode is on — the browser may limit background bandwidth.',
    })
  }

  const effective = latest?.effectiveType?.toLowerCase() ?? ''
  if (effective === 'slow-2g' || effective === '2g') {
    warnings.push({
      kind: 'slow-link',
      severity: 'critical',
      message: `Connection classified as ${effective.toUpperCase()} — expect buffering on high-bitrate streams.`,
    })
  } else if (effective === '3g') {
    warnings.push({
      kind: 'slow-link',
      severity: 'warn',
      message: 'Connection classified as 3G — 1080p relays may struggle.',
    })
  }

  if (utilizationPct != null && utilizationPct >= 85) {
    warnings.push({
      kind: 'high-utilization',
      severity: utilizationPct >= 95 ? 'critical' : 'warn',
      message: `Measured throughput is ${utilizationPct.toFixed(0)}% of the browser link estimate — you may be near your bandwidth cap.`,
    })
  }

  if (rttJitterMs != null && avgRttMs != null && rttJitterMs >= Math.max(25, avgRttMs * 0.35)) {
    warnings.push({
      kind: 'unstable-rtt',
      severity: rttJitterMs >= Math.max(60, avgRttMs * 0.6) ? 'critical' : 'warn',
      message: `RTT is unstable (±${rttJitterMs.toFixed(0)} ms jitter) — possible packet loss or Wi‑Fi congestion.`,
    })
  }

  if (probeFailureRate >= 0.2 && recent.length >= 5) {
    warnings.push({
      kind: 'probe-failures',
      severity: probeFailureRate >= 0.4 ? 'critical' : 'warn',
      message: `${Math.round(probeFailureRate * 100)}% of recent throughput probes failed — local routing or proxy may be saturated.`,
    })
  }

  return {
    latestProbeMbps,
    latestDownlinkMbps,
    utilizationPct,
    avgRttMs,
    rttJitterMs,
    probeFailureRate,
    warnings,
  }
}

export function formatMbps(value: number | null | undefined) {
  if (value == null || Number.isNaN(value)) return '—'
  if (value >= 100) return `${value.toFixed(0)} Mbps`
  if (value >= 10) return `${value.toFixed(1)} Mbps`
  return `${value.toFixed(2)} Mbps`
}
