import { useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { getHostNetworkDiagnostics, getOpsNetworkSnapshot } from '../api'
import { SETUP_CONTROL_AVAILABLE } from '../config'

export function useNetworkMonitor(options: { highlightChannel?: string; paused?: boolean } = {}) {
  const queryClient = useQueryClient()
  const [tabHidden, setTabHidden] = useState(
    () => typeof document !== 'undefined' && document.visibilityState === 'hidden',
  )
  useEffect(() => {
    const onVisibility = () => setTabHidden(document.visibilityState === 'hidden')
    document.addEventListener('visibilitychange', onVisibility)
    return () => document.removeEventListener('visibilitychange', onVisibility)
  }, [])

  const paused = (options.paused ?? false) || tabHidden

  const snapshot = useQuery({
    queryKey: ['ops-network-snapshot'],
    queryFn: getOpsNetworkSnapshot,
    staleTime: 4_000,
    refetchInterval: paused ? false : 5_000,
    retry: false,
  })

  const hostNetwork = useQuery({
    queryKey: ['host-network-diagnostics'],
    queryFn: getHostNetworkDiagnostics,
    enabled: SETUP_CONTROL_AVAILABLE && !paused,
    staleTime: 4_000,
    refetchInterval: paused ? false : 5_000,
    retry: false,
  })

  const refreshAll = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['ops-network-snapshot'] }),
      SETUP_CONTROL_AVAILABLE
        ? queryClient.invalidateQueries({ queryKey: ['host-network-diagnostics'] })
        : Promise.resolve(),
    ])
  }

  const activeAnalyticsSyncs = snapshot.data?.activeAnalyticsSyncs?.jobs ?? []
  const trackingSnapshot = snapshot.data?.trackingSnapshot

  const totals = useMemo(() => {
    const containers = hostNetwork.data?.containers ?? []
    let rxBytes = 0
    let txBytes = 0
    for (const row of containers) {
      rxBytes += row.rxBytes ?? 0
      txBytes += row.txBytes ?? 0
    }
    const activeStreams = snapshot.data?.activeStreams ?? []
    const relayCount = activeStreams.length
    const chatConnections = snapshot.data?.prometheus?.chatConnections?.value ?? null
    const medianDelay = activeStreams.length
      ? activeStreams.reduce((sum, stream) => {
        const segment = Number.parseFloat(String(stream.targetDuration ?? '2')) || 2
        return sum + (stream.liveEdge ?? 0) * segment
      }, 0) / activeStreams.length
      : null
    const analyticsSyncBytes = activeAnalyticsSyncs.reduce((sum, job) => {
      const network = job.network
      if (!network) return sum
      return sum + (network.totalBytes ?? (
        (network.trackerScrapeBytes ?? 0)
        + (network.gqlFetchBytes ?? 0)
        + (network.emotePreloadBytes ?? 0)
        + (network.helixBytes ?? 0)
      ))
    }, 0)
    const analyticsSyncRateBps = activeAnalyticsSyncs.reduce(
      (sum, job) => sum + (job.network?.lastRateBps ?? 0),
      0,
    )
    return {
      rxBytes,
      txBytes,
      hasBytes: containers.length > 0,
      relayCount,
      chatConnections,
      medianDelay,
      analyticsSyncCount: activeAnalyticsSyncs.length,
      trackedChannelCount: trackingSnapshot?.tracked?.length ?? 0,
      analyticsSyncBytes,
      analyticsSyncRateBps,
    }
  }, [
    activeAnalyticsSyncs,
    hostNetwork.data?.containers,
    snapshot.data?.activeStreams,
    snapshot.data?.prometheus,
    trackingSnapshot?.tracked,
  ])

  const diagnosticsBundle = useMemo(() => ({
    generatedAt: new Date().toISOString(),
    snapshot: snapshot.data,
    hostNetwork: hostNetwork.data,
  }), [hostNetwork.data, snapshot.data])

  return {
    snapshot,
    hostNetwork,
    refreshAll,
    totals,
    diagnosticsBundle,
    highlightChannel: options.highlightChannel?.trim().toLowerCase() || '',
    pulseReady: snapshot.data?.pulseReady ?? false,
    paused,
    activeAnalyticsSyncs,
    trackingSnapshot,
  }
}
