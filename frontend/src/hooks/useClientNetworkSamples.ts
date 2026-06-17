import { useEffect, useMemo, useState } from 'react'
import {
  CLIENT_NETWORK_MAX_SAMPLES,
  CLIENT_NETWORK_PROBE_INTERVAL_MS,
  analyzeClientNetwork,
  measureProbeThroughput,
  type ClientNetworkSample,
} from '../utils/clientNetworkProbe'

interface NetworkInformation {
  downlink?: number
  effectiveType?: string
  rtt?: number
  saveData?: boolean
  addEventListener?: (type: string, listener: () => void) => void
  removeEventListener?: (type: string, listener: () => void) => void
}

export function useClientNetworkSamples(enabled = true) {
  const [samples, setSamples] = useState<ClientNetworkSample[]>([])

  useEffect(() => {
    if (!enabled || typeof window === 'undefined') return

    let cancelled = false

    const readConnection = (): Omit<ClientNetworkSample, 'at' | 'probeMbps' | 'probeFailed'> => {
      const nav = navigator as Navigator & { connection?: NetworkInformation }
      const conn = nav.connection
      return {
        downlinkMbps: conn?.downlink ?? null,
        rttMs: conn?.rtt ?? null,
        effectiveType: conn?.effectiveType ?? null,
        saveData: Boolean(conn?.saveData),
      }
    }

    const tick = async () => {
      const probe = await measureProbeThroughput()
      if (cancelled) return
      setSamples(prev => {
        const next: ClientNetworkSample = {
          at: Date.now(),
          ...readConnection(),
          probeMbps: probe.mbps,
          probeFailed: probe.failed,
        }
        return [...prev, next].slice(-CLIENT_NETWORK_MAX_SAMPLES)
      })
    }

    void tick()
    const timer = window.setInterval(() => void tick(), CLIENT_NETWORK_PROBE_INTERVAL_MS)

    const nav = navigator as Navigator & { connection?: NetworkInformation }
    const conn = nav.connection
    const onChange = () => {
      setSamples(prev => {
        if (!prev.length) return prev
        const last = prev[prev.length - 1]!
        const updated: ClientNetworkSample = { ...last, ...readConnection(), at: Date.now() }
        return [...prev.slice(0, -1), updated]
      })
    }
    if (conn && typeof conn.addEventListener === 'function') {
      conn.addEventListener('change', onChange)
    }

    return () => {
      cancelled = true
      window.clearInterval(timer)
      if (conn && typeof conn.removeEventListener === 'function') {
        conn.removeEventListener('change', onChange)
      }
    }
  }, [enabled])

  const analysis = useMemo(() => analyzeClientNetwork(samples), [samples])

  return { samples, analysis }
}
