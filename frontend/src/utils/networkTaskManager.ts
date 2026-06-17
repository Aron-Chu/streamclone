import type {
  HostNetworkContainerStats,
  MetadataDiagnosticsServices,
  OpsActiveStream,
  OpsNetworkPrometheus,
} from '../api'
import { formatMbps } from './clientNetworkProbe'

export type NetworkTaskCategory = 'stream' | 'optional' | 'core' | 'page' | 'browser'
export type NetworkTaskImpact = 'high' | 'medium' | 'low' | 'unknown'

export type NetworkTaskDisableAction =
  | { kind: 'stop-relay'; channel: string }
  | { kind: 'stop-optional'; service: 'scraper' | 'clipper' | 'pulse' }
  | { kind: 'pause-page-monitoring' }

export interface NetworkTask {
  id: string
  name: string
  category: NetworkTaskCategory
  status: 'active' | 'idle' | 'offline'
  impact: NetworkTaskImpact
  detail: string
  throughputHint?: string
  canDisable: boolean
  disableLabel?: string
  disableWarning?: string
  disableAction?: NetworkTaskDisableAction
}

export interface BuildNetworkTasksInput {
  activeStreams: OpsActiveStream[]
  services: MetadataDiagnosticsServices | undefined
  pulseReady: boolean
  containers: HostNetworkContainerStats[]
  prometheus?: OpsNetworkPrometheus
  pageMonitoringPaused: boolean
  clientProbeMbps: number | null
  setupControlAvailable: boolean
}

const CATEGORY_LABELS: Record<NetworkTaskCategory, string> = {
  stream: 'Live stream',
  optional: 'Optional service',
  core: 'Core stack',
  page: 'This page',
  browser: 'Browser',
}

export function networkTaskCategoryLabel(category: NetworkTaskCategory) {
  return CATEGORY_LABELS[category]
}

function renditionMbps(bandwidth?: number) {
  if (!bandwidth || bandwidth <= 0) return null
  return bandwidth / 1_000_000
}

function containerRole(name: string): { label: string; detail: string; optional: boolean } {
  const lower = name.toLowerCase()
  if (lower.includes('video')) return { label: 'Video / HLS relay', detail: 'Pulls Twitch HLS segments and serves playback to the player.', optional: false }
  if (lower.includes('chat')) return { label: 'Chat bridge', detail: 'IRC ingest and WebSocket fan-out for live chat.', optional: false }
  if (lower.includes('metadata')) return { label: 'Metadata API', detail: 'Directory, channel pages, and ops snapshots.', optional: false }
  if (lower.includes('analytics')) return { label: 'Analytics worker', detail: 'Chat/emote rollups, sync jobs, and Pulse export when enabled.', optional: false }
  if (lower.includes('emote')) return { label: 'Emote pipeline', detail: '7TV / Twitch / FFZ dictionary sync and image serving.', optional: false }
  if (lower.includes('scraper')) return { label: 'TwitchTracker scraper', detail: 'Viewer minute charts and tracker-backed analytics sync.', optional: true }
  if (lower.includes('grafana') || lower.includes('influx')) return { label: 'Pulse metrics', detail: 'Grafana dashboards and Influx time-series for ops graphs.', optional: true }
  if (lower.includes('postgres')) return { label: 'Postgres', detail: 'Primary database for streams, chat logs, and emotes.', optional: false }
  if (lower.includes('redis')) return { label: 'Redis cache', detail: 'Hot emote dictionaries and chat cache.', optional: false }
  if (lower.includes('frontend') || lower.includes('proxy') || lower.includes('caddy')) {
    return { label: 'Web UI / proxy', detail: 'Serves the React app and routes API traffic on :8090.', optional: false }
  }
  if (lower.includes('mediamtx')) return { label: 'MediaMTX', detail: 'Local media router for relay paths.', optional: false }
  return { label: name, detail: 'Docker container in the Streamclone stack.', optional: false }
}

function impactFromBytes(txBytes: number, rxBytes: number): NetworkTaskImpact {
  const total = txBytes + rxBytes
  if (total >= 500_000_000) return 'high'
  if (total >= 50_000_000) return 'medium'
  if (total > 0) return 'low'
  return 'unknown'
}

function optionalServiceTask(
  service: 'scraper' | 'clipper' | 'pulse',
  name: string,
  ready: boolean,
  detail: string,
  disableWarning: string,
): NetworkTask {
  return {
    id: `optional-${service}`,
    name,
    category: 'optional',
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

export function buildNetworkTasks(input: BuildNetworkTasksInput): NetworkTask[] {
  const tasks: NetworkTask[] = []

  for (const stream of input.activeStreams) {
    const mbps = renditionMbps(stream.bandwidth)
    tasks.push({
      id: `relay-${stream.channel}`,
      name: `HLS relay · ${stream.channel}`,
      category: 'stream',
      status: 'active',
      impact: mbps != null && mbps >= 4 ? 'high' : mbps != null && mbps >= 1 ? 'medium' : 'medium',
      detail: `Worker pulls Twitch HLS for ${stream.channel}. ${stream.listeners} listener(s), ${stream.quality || 'auto'} quality.`,
      throughputHint: mbps != null ? `~${formatMbps(mbps)} downstream to player` : 'Bitrate unknown',
      canDisable: true,
      disableLabel: 'Stop relay',
      disableWarning: `Playback for ${stream.channel} will stop and the channel player will lose its live stream until you reopen it.`,
      disableAction: { kind: 'stop-relay', channel: stream.channel },
    })
  }

  const svc = input.services
  if (svc) {
    tasks.push(optionalServiceTask(
      'scraper',
      'Analytics scraper',
      svc.scraper === 'ready',
      'TwitchTracker viewer charts, scraper-backed sync, and Cloudflare-heavy fetches.',
      'Minute-level viewer charts and new TwitchTracker sync will stop. Existing synced analytics data remains readable.',
    ))
    tasks.push(optionalServiceTask(
      'pulse',
      'Pulse dashboards',
      input.pulseReady || svc.pulse === 'ready',
      'Prometheus scrape, Grafana ops dashboards, and Influx rollups for historical throughput.',
      'Ops throughput graphs and Grafana dashboards will go offline. Core playback and chat are unaffected.',
    ))
    tasks.push(optionalServiceTask(
      'clipper',
      'Clip Studio',
      svc.clipper === 'ready',
      'ReplayForge / clipper API for exports, captions, and clip jobs.',
      'Clip Studio exports and caption rendering will be unavailable until Clip Studio is started again.',
    ))
  }

  const sortedContainers = [...input.containers].sort(
    (a, b) => (b.txBytes + b.rxBytes) - (a.txBytes + a.rxBytes),
  )
  for (const row of sortedContainers.slice(0, 8)) {
    const role = containerRole(row.name)
    tasks.push({
      id: `container-${row.name}`,
      name: role.label,
      category: role.optional ? 'optional' : 'core',
      status: 'active',
      impact: impactFromBytes(row.txBytes, row.rxBytes),
      detail: `${role.detail} Container: ${row.name}.`,
      throughputHint: `${row.rxHuman || '—'} ↓ / ${row.txHuman || '—'} ↑ total`,
      canDisable: false,
      disableWarning: role.optional
        ? undefined
        : 'Core containers cannot be stopped from this page — use Stop Streamclone to shut down the whole stack.',
    })
  }

  const chatClients = input.prometheus?.chatConnections?.value
  if (chatClients != null && chatClients > 0) {
    tasks.push({
      id: 'chat-ws-clients',
      name: 'Chat WebSocket clients',
      category: 'core',
      status: 'active',
      impact: chatClients >= 5 ? 'medium' : 'low',
      detail: 'Browser tabs connected to the chat WebSocket (live channels and chat logs).',
      throughputHint: `${Math.round(chatClients)} connected client(s)`,
      canDisable: false,
      disableWarning: 'Close live channel tabs to reduce chat socket traffic.',
    })
  }

  tasks.push({
    id: 'page-network-monitor',
    name: 'Network monitor polling',
    category: 'page',
    status: input.pageMonitoringPaused ? 'idle' : 'active',
    impact: 'low',
    detail: 'This tab polls /v1/ops/network every 5s and runs a small throughput probe every 2s.',
    throughputHint: input.clientProbeMbps != null ? `Probe ~${formatMbps(input.clientProbeMbps)}` : undefined,
    canDisable: !input.pageMonitoringPaused,
    disableLabel: 'Pause monitoring',
    disableWarning: 'Live graphs on this page will freeze until you resume monitoring or refresh.',
    disableAction: input.pageMonitoringPaused ? undefined : { kind: 'pause-page-monitoring' },
  })

  if (!input.setupControlAvailable) {
    tasks.push({
      id: 'setup-control-offline',
      name: 'Install helper offline',
      category: 'core',
      status: 'offline',
      impact: 'unknown',
      detail: 'setup-control on :9191 is not reachable — container task breakdown and Docker stop actions are limited.',
      canDisable: false,
    })
  }

  return tasks.sort((a, b) => {
    const impactRank: Record<NetworkTaskImpact, number> = { high: 0, medium: 1, low: 2, unknown: 3 }
    const diff = impactRank[a.impact] - impactRank[b.impact]
    if (diff !== 0) return diff
    if (a.status === 'active' && b.status !== 'active') return -1
    if (b.status === 'active' && a.status !== 'active') return 1
    return a.name.localeCompare(b.name)
  })
}

export function summarizeNetworkTasks(tasks: NetworkTask[]) {
  const active = tasks.filter(t => t.status === 'active')
  return {
    activeCount: active.length,
    highImpact: active.filter(t => t.impact === 'high').length,
    stoppable: tasks.filter(t => t.canDisable && t.disableAction).length,
  }
}
