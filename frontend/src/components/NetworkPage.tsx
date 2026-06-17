import { useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { SETUP_CONTROL_AVAILABLE } from '../config'
import { getLiveDelayBreakdown } from '../playbackMath'
import { useClientNetworkSamples } from '../hooks/useClientNetworkSamples'
import { useNetworkMonitor } from '../hooks/useNetworkMonitor'
import { useNetworkTimeSeries } from '../hooks/useNetworkTimeSeries'
import { useOptionalServices } from '../hooks/useOptionalServices'
import {
  attachSparklines,
  buildNetworkActivityNodes,
  filterNodesBySeries,
  filterNodesByTab,
  type BandwidthSeriesKey,
  type NetworkActivityTab,
} from '../utils/networkActivityModel'
import ActiveRelaysPanel from './network/ActiveRelaysPanel'
import ClientNetworkPanel from './network/ClientNetworkPanel'
import ContainerNetworkTable from './network/ContainerNetworkTable'
import LiveDelayBreakdownPanel from './network/LiveDelayBreakdown'
import NetworkActivityTable from './network/NetworkActivityTable'
import NetworkBandwidthOverview from './network/NetworkBandwidthOverview'
import NetworkCategoryTabs from './network/NetworkCategoryTabs'
import NetworkSummaryCards from './network/NetworkSummaryCards'
import ServiceThroughputPanel from './network/ServiceThroughputPanel'

import { PULSE_OPS_DASHBOARD_URL } from '../utils/pulseDashboard.ts'

export default function NetworkPage() {
  const [searchParams] = useSearchParams()
  const highlightChannel = searchParams.get('channel') ?? ''
  const [monitoringPaused, setMonitoringPaused] = useState(false)
  const [activeTab, setActiveTab] = useState<NetworkActivityTab>('overview')
  const [selectedSeries, setSelectedSeries] = useState<BandwidthSeriesKey | null>(null)

  const monitor = useNetworkMonitor({ highlightChannel, paused: monitoringPaused })
  const clientNetwork = useClientNetworkSamples(!monitoringPaused && activeTab === 'client')
  const optional = useOptionalServices({ probeControl: true })
  const [copied, setCopied] = useState(false)

  const baseNodes = useMemo(() => buildNetworkActivityNodes({
    activeStreams: monitor.snapshot.data?.activeStreams ?? [],
    activeAnalyticsSyncs: monitor.activeAnalyticsSyncs,
    trackingSnapshot: monitor.trackingSnapshot,
    services: monitor.snapshot.data?.services,
    pulseReady: monitor.pulseReady,
    containers: monitor.hostNetwork.data?.containers ?? [],
    prometheus: monitor.snapshot.data?.prometheus,
    pageMonitoringPaused: monitoringPaused,
    clientProbeMbps: clientNetwork.analysis.latestProbeMbps,
    setupControlAvailable: SETUP_CONTROL_AVAILABLE,
  }), [
    clientNetwork.analysis.latestProbeMbps,
    monitor.activeAnalyticsSyncs,
    monitor.hostNetwork.data?.containers,
    monitor.pulseReady,
    monitor.snapshot.data?.activeStreams,
    monitor.snapshot.data?.prometheus,
    monitor.snapshot.data?.services,
    monitor.trackingSnapshot,
    monitoringPaused,
  ])

  const timeSeries = useNetworkTimeSeries({
    nodes: baseNodes,
    containerRxBytes: monitor.totals.rxBytes,
    containerTxBytes: monitor.totals.txBytes,
    hasContainerBytes: monitor.totals.hasBytes,
    enabled: !monitoringPaused,
  })

  const activityNodes = useMemo(() => {
    let nodes = attachSparklines(baseNodes, timeSeries.nodeSparklines)
    if (selectedSeries) nodes = filterNodesBySeries(nodes, selectedSeries)
    return nodes
  }, [baseNodes, selectedSeries, timeSeries.nodeSparklines])

  const tabCounts = useMemo(() => ({
    overview: baseNodes.filter(n => !n.parentId).length,
    streams: filterNodesByTab(baseNodes, 'streams').filter(n => !n.parentId).length,
    analytics: filterNodesByTab(baseNodes, 'analytics').filter(n => !n.parentId).length,
    stack: filterNodesByTab(baseNodes, 'stack').filter(n => !n.parentId).length,
    client: filterNodesByTab(baseNodes, 'client').filter(n => !n.parentId).length,
  }), [baseNodes])

  const sampleBreakdown = useMemo(() => {
    const stream = monitor.snapshot.data?.activeStreams?.find(
      row => highlightChannel && row.channel.toLowerCase() === highlightChannel.toLowerCase(),
    ) ?? monitor.snapshot.data?.activeStreams?.[0]
    if (!stream) return null
    return getLiveDelayBreakdown(
      { latencyToLiveSec: null, targetLatencySec: 5, behindLiveSec: 0 },
      { liveEdge: stream.liveEdge, hlsProbe: { targetDuration: stream.targetDuration } },
    )
  }, [highlightChannel, monitor.snapshot.data?.activeStreams])

  const copyDiagnostics = async () => {
    try {
      await navigator.clipboard.writeText(JSON.stringify(monitor.diagnosticsBundle, null, 2))
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    } catch {
      setCopied(false)
    }
  }

  return (
    <main className="min-h-screen bg-[#07070a] text-zinc-100">
      <div className="mx-auto flex w-full max-w-6xl flex-col gap-5 px-4 py-6 sm:px-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div className="text-xs font-black uppercase tracking-wide text-zinc-500">
              <Link to="/" className="text-zinc-500 transition hover:text-zinc-300">Home</Link>
              <span className="mx-2 text-zinc-700">→</span>
              Network
            </div>
            <h1 className="text-2xl font-black text-white">Network activity monitor</h1>
            <p className="mt-1 text-sm font-semibold text-zinc-500">
              Measured bandwidth by stream, analytics sync phase, and stack service
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={() => void monitor.refreshAll()}
              className="rounded border border-white/10 px-3 py-2 text-xs font-black text-zinc-300 transition hover:bg-white/10"
            >
              Refresh
            </button>
            <button
              type="button"
              onClick={() => void copyDiagnostics()}
              className="rounded border border-white/10 px-3 py-2 text-xs font-black text-zinc-300 transition hover:bg-white/10"
            >
              {copied ? 'Copied' : 'Copy diagnostics'}
            </button>
            {monitor.pulseReady ? (
              <a
                href={PULSE_OPS_DASHBOARD_URL}
                target="_blank"
                rel="noreferrer"
                className="rounded border border-violet-400/30 bg-violet-500/10 px-3 py-2 text-xs font-black text-violet-100 transition hover:bg-violet-500/20"
              >
                Open Grafana ops
              </a>
            ) : null}
          </div>
        </div>

        {monitor.snapshot.isError ? (
          <div className="rounded-xl border border-amber-300/20 bg-amber-500/10 px-4 py-3 text-sm font-semibold text-amber-50">
            Ops snapshot unavailable. Metadata `GET /v1/ops/network` may not be deployed yet.
          </div>
        ) : null}

        <NetworkSummaryCards
          rxBytes={monitor.totals.rxBytes}
          txBytes={monitor.totals.txBytes}
          hasBytes={monitor.totals.hasBytes}
          relayCount={monitor.totals.relayCount}
          chatConnections={monitor.totals.chatConnections}
          medianDelay={monitor.totals.medianDelay}
          loading={monitor.snapshot.isLoading || monitor.hostNetwork.isLoading}
        />

        <NetworkBandwidthOverview
          categorySeries={timeSeries.categorySeries}
          latestRates={timeSeries.latestCategoryRates}
          selectedSeries={selectedSeries}
          onSelectSeries={setSelectedSeries}
          loading={monitoringPaused || monitor.snapshot.isLoading}
        />

        <NetworkCategoryTabs
          activeTab={activeTab}
          onTabChange={setActiveTab}
          counts={tabCounts}
        />

        <NetworkActivityTable
          nodes={activityNodes}
          activeTab={activeTab}
          onPauseMonitoring={() => setMonitoringPaused(true)}
          onActionComplete={() => {
            void monitor.refreshAll()
            void optional.refreshStatus()
          }}
        />

        {monitoringPaused ? (
          <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-white/10 bg-white/[0.03] px-4 py-3 text-sm font-semibold text-zinc-400">
            Network monitoring is paused on this tab.
            <button
              type="button"
              onClick={() => setMonitoringPaused(false)}
              className="rounded border border-violet-400/30 bg-violet-500/10 px-3 py-1.5 text-xs font-black uppercase text-violet-100 transition hover:bg-violet-500/20"
            >
              Resume monitoring
            </button>
          </div>
        ) : null}

        {activeTab === 'streams' && sampleBreakdown ? (
          <LiveDelayBreakdownPanel
            breakdown={sampleBreakdown}
            latencyMode={monitor.snapshot.data?.activeStreams?.[0]?.latencyMode}
            title={highlightChannel ? `Delay estimate · ${highlightChannel}` : 'Sample relay delay estimate'}
          />
        ) : null}

        {activeTab === 'stack' ? (
          <>
            <ContainerNetworkTable
              containers={monitor.hostNetwork.data?.containers ?? []}
              loading={monitor.hostNetwork.isLoading}
            />
            <ActiveRelaysPanel
              streams={monitor.snapshot.data?.activeStreams ?? []}
              highlightChannel={monitor.highlightChannel}
              loading={monitor.snapshot.isLoading}
            />
            <ServiceThroughputPanel
              prometheus={monitor.snapshot.data?.prometheus}
              pulseReady={monitor.pulseReady}
              loading={monitor.snapshot.isLoading}
              onStartPulse={() => void optional.startService('pulse')}
              startingPulse={optional.isStarting('pulse')}
            />
          </>
        ) : null}

        {activeTab === 'client' ? (
          <ClientNetworkPanel
            samples={clientNetwork.samples}
            analysis={clientNetwork.analysis}
            paused={monitoringPaused}
          />
        ) : null}
      </div>
    </main>
  )
}
