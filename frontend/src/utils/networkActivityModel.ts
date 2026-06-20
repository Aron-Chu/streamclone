import type {
  ActiveAnalyticsSyncJob,
  HostNetworkContainerStats,
  MetadataDiagnosticsServices,
  OpsActiveStream,
  OpsNetworkPrometheus,
  TrackingSnapshot,
} from '../api'
import { formatMbps } from './clientNetworkProbe.ts'
import type { NetworkTaskDisableAction } from './networkTaskManager'

export type NetworkActivityCategory =
  | 'stream'
  | 'analytics'
  | 'analytics-op'
  | 'tracking'
  | 'core'
  | 'optional'
  | 'page'
  | 'browser'

export type NetworkActivityImpact = 'high' | 'medium' | 'low' | 'unknown'

export type NetworkActivityTab = 'overview' | 'streams' | 'analytics' | 'stack' | 'client'

export type BandwidthSeriesKey = 'hls' | 'analytics' | 'chat' | 'core' | 'browser'

export interface NetworkActivityNode {
  id: string
  parentId?: string
  category: NetworkActivityCategory
  name: string
  subActivity?: string
  channel?: string
  phase?: string
  status: 'active' | 'idle' | 'offline'
  bytesTotal?: number
  bytesPerSec?: number
  sparkline?: number[]
  impact: NetworkActivityImpact
  detail?: string
  throughputHint?: string
  canDisable?: boolean
  disableLabel?: string
  disableWarning?: string
  disableAction?: NetworkTaskDisableAction
}

export interface BuildNetworkActivityNodesInput {
  activeStreams: OpsActiveStream[]
  activeAnalyticsSyncs?: ActiveAnalyticsSyncJob[]
  trackingSnapshot?: TrackingSnapshot
  services?: MetadataDiagnosticsServices
  pulseReady: boolean
  containers: HostNetworkContainerStats[]
  prometheus?: OpsNetworkPrometheus
  pageMonitoringPaused: boolean
  clientProbeMbps: number | null
  setupControlAvailable: boolean
}

export const BANDWIDTH_SERIES_META: Record<
  BandwidthSeriesKey,
  { label: string; color: string; fill: string }
> = {
  hls: { label: 'HLS relays', color: '#a78bfa', fill: 'rgba(167,139,250,0.35)' },
  analytics: { label: 'Analytics sync', color: '#f472b6', fill: 'rgba(244,114,182,0.35)' },
  chat: { label: 'Chat / tracking', color: '#34d399', fill: 'rgba(52,211,153,0.35)' },
  core: { label: 'Core stack', color: '#60a5fa', fill: 'rgba(96,165,250,0.35)' },
  browser: { label: 'Browser / page', color: '#fbbf24', fill: 'rgba(251,191,36,0.35)' },
}

export const NETWORK_ACTIVITY_TABS: Array<{ id: NetworkActivityTab; label: string }> = [
  { id: 'overview', label: 'Overview' },
  { id: 'streams', label: 'Streams' },
  { id: 'analytics', label: 'Analytics' },
  { id: 'stack', label: 'Stack' },
  { id: 'client', label: 'Client' },
]

const TAB_CATEGORIES: Record<NetworkActivityTab, NetworkActivityCategory[] | 'all'> = {
  overview: 'all',
  streams: ['stream'],
  analytics: ['analytics', 'analytics-op', 'tracking'],
  stack: ['core', 'optional'],
  client: ['page', 'browser'],
}

export function nodeToBandwidthSeries(category: NetworkActivityCategory): BandwidthSeriesKey {
  switch (category) {
    case 'stream':
      return 'hls'
    case 'analytics':
    case 'analytics-op':
      return 'analytics'
    case 'tracking':
      return 'chat'
    case 'core':
    case 'optional':
      return 'core'
    case 'page':
    case 'browser':
      return 'browser'
    default:
      return 'core'
  }
}

export function filterNodesByTab(nodes: NetworkActivityNode[], tab: NetworkActivityTab): NetworkActivityNode[] {
  const allowed = TAB_CATEGORIES[tab]
  if (allowed === 'all') return nodes
  const allowedSet = new Set(allowed)
  const visibleIds = new Set<string>()
  for (const node of nodes) {
    if (allowedSet.has(node.category)) visibleIds.add(node.id)
  }
  for (const node of nodes) {
    if (node.parentId && visibleIds.has(node.parentId)) visibleIds.add(node.id)
  }
  return nodes.filter(node => visibleIds.has(node.id))
}

export function filterNodesBySeries(nodes: NetworkActivityNode[], series: BandwidthSeriesKey | null): NetworkActivityNode[] {
  if (!series) return nodes
  const visibleIds = new Set<string>()
  for (const node of nodes) {
    if (nodeToBandwidthSeries(node.category) === series) visibleIds.add(node.id)
  }
  for (const node of nodes) {
    if (node.parentId && visibleIds.has(node.parentId)) visibleIds.add(node.id)
  }
  return nodes.filter(node => visibleIds.has(node.id))
}

export function formatBytes(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value) || value <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = value
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit += 1
  }
  return `${size >= 10 || unit === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[unit]}`
}

export function formatRate(bytesPerSec: number | null | undefined): string {
  if (bytesPerSec == null || !Number.isFinite(bytesPerSec) || bytesPerSec <= 0) return '—'
  const units = ['B/s', 'KB/s', 'MB/s', 'GB/s']
  let size = bytesPerSec
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit += 1
  }
  return `${size >= 10 || unit === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[unit]}`
}

function renditionBytesPerSec(bandwidth?: number): number | undefined {
  if (!bandwidth || bandwidth <= 0) return undefined
  return bandwidth / 8
}

function impactFromBytesPerSec(bps: number | undefined): NetworkActivityImpact {
  if (bps == null || bps <= 0) return 'unknown'
  if (bps >= 1_000_000) return 'high'
  if (bps >= 250_000) return 'medium'
  return 'low'
}

function impactFromTotals(txBytes: number, rxBytes: number): NetworkActivityImpact {
  const total = txBytes + rxBytes
  if (total >= 500_000_000) return 'high'
  if (total >= 50_000_000) return 'medium'
  if (total > 0) return 'low'
  return 'unknown'
}

function containerRole(name: string): { label: string; detail: string; optional: boolean } {
  const lower = name.toLowerCase()
  if (lower.includes('video')) return { label: 'Video / HLS relay', detail: 'Pulls Twitch HLS segments and serves playback.', optional: false }
  if (lower.includes('chat')) return { label: 'Chat bridge', detail: 'IRC ingest and WebSocket fan-out.', optional: false }
  if (lower.includes('metadata')) return { label: 'Metadata API', detail: 'Directory, channel pages, and ops snapshots.', optional: false }
  if (lower.includes('analytics')) return { label: 'Analytics worker', detail: 'Chat/emote rollups and sync jobs.', optional: false }
  if (lower.includes('emote')) return { label: 'Emote pipeline', detail: '7TV / Twitch / FFZ dictionary sync.', optional: false }
  if (lower.includes('scraper')) return { label: 'TwitchTracker scraper', detail: 'Viewer charts and tracker-backed sync.', optional: true }
  if (lower.includes('grafana') || lower.includes('influx')) return { label: 'Pulse metrics', detail: 'Grafana dashboards and Influx time-series.', optional: true }
  if (lower.includes('postgres')) return { label: 'Postgres', detail: 'Primary database.', optional: false }
  if (lower.includes('redis')) return { label: 'Redis cache', detail: 'Hot emote dictionaries and chat cache.', optional: false }
  if (lower.includes('frontend') || lower.includes('proxy') || lower.includes('caddy')) {
    return { label: 'Web UI / proxy', detail: 'Serves the React app on :8090.', optional: false }
  }
  if (lower.includes('mediamtx')) return { label: 'MediaMTX', detail: 'Local media router for relay paths.', optional: false }
  return { label: name, detail: 'Docker container in the Streamclone stack.', optional: false }
}

function optionalServiceNode(
  service: 'scraper' | 'clipper' | 'pulse',
  name: string,
  ready: boolean,
  detail: string,
  disableWarning: string,
): NetworkActivityNode {
  return {
    id: `optional-${service}`,
    category: 'optional',
    name,
    status: ready ? 'active' : 'offline',
    impact: ready ? (service === 'scraper' ? 'high' : 'medium') : 'unknown',
    detail,
    throughputHint: ready ? 'Background sync / metrics' : undefined,
    canDisable: ready,
    disableLabel: 'Stop service',
    disableWarning,
    disableAction: ready ? { kind: 'stop-optional', service } : undefined,
  }
}

const ANALYTICS_OP_ROWS: Array<{
  key: keyof NonNullable<ActiveAnalyticsSyncJob['network']>
  suffix: string
  label: string
  phaseMatch?: string
}> = [
  { key: 'trackerScrapeBytes', suffix: 'tracker', label: 'TwitchTracker scrape', phaseMatch: 'scraping_tracker' },
  { key: 'gqlFetchBytes', suffix: 'gql', label: 'VOD GQL fetch', phaseMatch: 'fetching_comments' },
  { key: 'emotePreloadBytes', suffix: 'emote', label: 'Emote preload' },
  { key: 'helixBytes', suffix: 'helix', label: 'Helix / VOD resolve' },
]

function analyticsSyncNodes(job: ActiveAnalyticsSyncJob): NetworkActivityNode[] {
  const parentId = `analytics-sync-${job.streamId}`
  const network = job.network
  const totalBytes = network?.totalBytes ?? sumNetworkBytes(network)
  const bytesPerSec = network?.lastRateBps != null ? network.lastRateBps / 8 : undefined

  const nodes: NetworkActivityNode[] = [{
    id: parentId,
    category: 'analytics',
    name: `Analytics sync · ${job.channel}`,
    channel: job.channel,
    phase: job.phase,
    status: 'active',
    bytesTotal: totalBytes || undefined,
    bytesPerSec,
    impact: impactFromBytesPerSec(bytesPerSec ?? (totalBytes ? totalBytes / 60 : undefined)),
    detail: job.phase
      ? `Phase ${job.phase}${job.chat?.gqlPages ? ` · ${job.chat.gqlPages} GQL pages` : ''}`
      : 'Historical chat/emote sync in progress.',
    throughputHint: bytesPerSec != null ? formatRate(bytesPerSec) : totalBytes ? formatBytes(totalBytes) : undefined,
    canDisable: false,
  }]

  for (const op of ANALYTICS_OP_ROWS) {
    const bytes = network?.[op.key]
    if (typeof bytes !== 'number' || bytes <= 0) continue
    const phase = op.phaseMatch && job.phase?.includes(op.phaseMatch) ? job.phase : job.tracker?.active && op.suffix === 'tracker' ? 'scraping_tracker' : undefined
    nodes.push({
      id: `${parentId}-${op.suffix}`,
      parentId,
      category: 'analytics-op',
      name: op.label,
      channel: job.channel,
      phase,
      status: 'active',
      bytesTotal: bytes,
      impact: impactFromBytesPerSec(bytes / 30),
      detail: phase ? `Active phase: ${phase}` : 'Measured download bytes for this sync step.',
      throughputHint: formatBytes(bytes),
    })
  }

  return nodes
}

function sumNetworkBytes(network?: ActiveAnalyticsSyncJob['network']): number {
  if (!network) return 0
  return (network.trackerScrapeBytes ?? 0)
    + (network.gqlFetchBytes ?? 0)
    + (network.emotePreloadBytes ?? 0)
    + (network.helixBytes ?? 0)
}

export function buildNetworkActivityNodes(input: BuildNetworkActivityNodesInput): NetworkActivityNode[] {
  const nodes: NetworkActivityNode[] = []

  for (const stream of input.activeStreams) {
    const bps = renditionBytesPerSec(stream.bandwidth)
    nodes.push({
      id: `relay-${stream.channel}`,
      category: 'stream',
      name: `HLS relay · ${stream.channel}`,
      channel: stream.channel,
      status: 'active',
      bytesPerSec: bps,
      impact: impactFromBytesPerSec(bps),
      detail: `${stream.listeners} listener(s), ${stream.quality || 'auto'} quality.`,
      throughputHint: bps != null ? `~${formatMbps((bps * 8) / 1_000_000)} downstream` : 'Bitrate unknown',
      canDisable: true,
      disableLabel: 'Stop relay',
      disableWarning: `Playback for ${stream.channel} will stop until you reopen the channel.`,
      disableAction: { kind: 'stop-relay', channel: stream.channel },
    })
  }

  for (const job of input.activeAnalyticsSyncs ?? []) {
    nodes.push(...analyticsSyncNodes(job))
  }

  const tracked = input.trackingSnapshot?.tracked ?? []
  for (const channel of tracked) {
    nodes.push({
      id: `tracking-${channel}`,
      category: 'tracking',
      name: `Live tracking · ${channel}`,
      channel,
      status: 'active',
      impact: 'medium',
      detail: 'IRC ingest and minute rollups for analytics.',
      throughputHint: 'Chat ingest + rollups',
      canDisable: true,
      disableLabel: 'Untrack',
      disableWarning:
        `Remove ${channel} from analytics tracking. Offline channels stop Helix polling immediately. `
        + 'If they are live right now, chat collection continues until the broadcast ends. '
        + 'Channels in ALWAYS_TRACKED_CHANNELS (.env) may reappear after an analytics restart.',
      disableAction: { kind: 'untrack-channel', channel },
    })
  }

  const svc = input.services
  if (svc) {
    nodes.push(optionalServiceNode(
      'scraper',
      'Analytics scraper',
      svc.scraper === 'ready',
      'TwitchTracker viewer charts and scraper-backed sync.',
      'Minute-level viewer charts and new TwitchTracker sync will stop.',
    ))
    nodes.push(optionalServiceNode(
      'pulse',
      'Pulse dashboards',
      input.pulseReady || svc.pulse === 'ready',
      'Prometheus scrape, Grafana ops dashboards, and Influx rollups.',
      'Ops throughput graphs and Grafana dashboards will go offline.',
    ))
    nodes.push(optionalServiceNode(
      'clipper',
      'Clip Studio',
      svc.clipper === 'ready',
      'ReplayForge / clipper API for exports and clip jobs.',
      'Clip Studio exports will be unavailable until restarted.',
    ))
  }

  const sortedContainers = [...input.containers].sort(
    (a, b) => (b.txBytes + b.rxBytes) - (a.txBytes + a.rxBytes),
  )
  for (const row of sortedContainers.slice(0, 8)) {
    const role = containerRole(row.name)
    nodes.push({
      id: `container-${row.name}`,
      category: role.optional ? 'optional' : 'core',
      name: role.label,
      status: 'active',
      bytesTotal: row.rxBytes + row.txBytes,
      impact: impactFromTotals(row.txBytes, row.rxBytes),
      detail: `${role.detail} Container: ${row.name}.`,
      throughputHint: `${row.rxHuman || '—'} ↓ / ${row.txHuman || '—'} ↑ total`,
      canDisable: false,
    })
  }

  const chatClients = input.prometheus?.chatConnections?.value
  if (chatClients != null && chatClients > 0) {
    nodes.push({
      id: 'chat-ws-clients',
      category: 'core',
      name: 'Chat WebSocket clients',
      status: 'active',
      impact: chatClients >= 5 ? 'medium' : 'low',
      detail: 'Browser tabs connected to the chat WebSocket.',
      throughputHint: `${Math.round(chatClients)} connected client(s)`,
    })
  }

  nodes.push({
    id: 'page-network-monitor',
    category: 'page',
    name: 'Network monitor polling',
    status: input.pageMonitoringPaused ? 'idle' : 'active',
    bytesPerSec: input.clientProbeMbps != null ? (input.clientProbeMbps * 1_000_000) / 8 : undefined,
    impact: 'low',
    detail: 'This tab polls /v1/ops/network every 5s and runs a throughput probe every 2s.',
    throughputHint: input.clientProbeMbps != null ? `Probe ~${formatMbps(input.clientProbeMbps)}` : undefined,
    canDisable: !input.pageMonitoringPaused,
    disableLabel: 'Pause monitoring',
    disableWarning: 'Live graphs on this page will freeze until you resume monitoring.',
    disableAction: input.pageMonitoringPaused ? undefined : { kind: 'pause-page-monitoring' },
  })

  if (input.clientProbeMbps != null) {
    nodes.push({
      id: 'browser-probe',
      category: 'browser',
      parentId: 'page-network-monitor',
      name: 'Browser throughput probe',
      status: input.pageMonitoringPaused ? 'idle' : 'active',
      bytesPerSec: (input.clientProbeMbps * 1_000_000) / 8,
      impact: 'low',
      detail: 'Same-origin logo.svg fetch to estimate download throughput.',
      throughputHint: formatMbps(input.clientProbeMbps),
    })
  }

  if (!input.setupControlAvailable) {
    nodes.push({
      id: 'setup-control-offline',
      category: 'core',
      name: 'Install helper offline',
      status: 'offline',
      impact: 'unknown',
      detail: 'setup-control on :9191 is not reachable — container breakdown is limited.',
    })
  }

  return sortActivityNodes(nodes)
}

export function sortActivityNodes(nodes: NetworkActivityNode[]): NetworkActivityNode[] {
  const impactRank: Record<NetworkActivityImpact, number> = { high: 0, medium: 1, low: 2, unknown: 3 }
  return [...nodes].sort((a, b) => {
    const parentA = a.parentId ? 1 : 0
    const parentB = b.parentId ? 1 : 0
    if (parentA !== parentB) return parentA - parentB
    const rateA = a.bytesPerSec ?? 0
    const rateB = b.bytesPerSec ?? 0
    if (rateA !== rateB) return rateB - rateA
    const diff = impactRank[a.impact] - impactRank[b.impact]
    if (diff !== 0) return diff
    if (a.status === 'active' && b.status !== 'active') return -1
    if (b.status === 'active' && a.status !== 'active') return 1
    return a.name.localeCompare(b.name)
  })
}

export function summarizeActivityNodes(nodes: NetworkActivityNode[]) {
  const topLevel = nodes.filter(n => !n.parentId)
  const active = topLevel.filter(n => n.status === 'active')
  return {
    activeCount: active.length,
    highImpact: active.filter(n => n.impact === 'high').length,
    stoppable: nodes.filter(n => n.canDisable && n.disableAction).length,
    analyticsSyncs: nodes.filter(n => n.category === 'analytics').length,
    trackedChannels: nodes.filter(n => n.category === 'tracking').length,
  }
}

export function attachSparklines(
  nodes: NetworkActivityNode[],
  nodeSparklines: Record<string, number[]>,
): NetworkActivityNode[] {
  return nodes.map(node => ({
    ...node,
    sparkline: nodeSparklines[node.id] ?? node.sparkline,
  }))
}
