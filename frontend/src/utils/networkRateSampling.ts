import type { OpsNetworkPrometheus } from '../api'
import type { BandwidthSeriesKey, NetworkActivityNode } from './networkActivityModel.ts'
import { nodeToBandwidthSeries } from './networkActivityModel.ts'

export const MAX_SAMPLE_BPS = 100 * 1024 * 1024
const MIN_COUNTER_SAMPLE_SEC = 4
const AVG_CHAT_MESSAGE_BYTES = 220

type SampledNode = NetworkActivityNode & { rateIsEstimated?: boolean }

export interface NetworkSampleState {
  nodeBytes: Record<string, number>
}

function finitePositive(value: number | null | undefined): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : 0
}

export function sumPromAnalyticsBytesPerSec(prometheus?: OpsNetworkPrometheus): number {
  const metric = prometheus?.analyticsBytesByChannelOp
  if (!metric) return 0
  const series = metric.series ?? []
  if (series.length > 0) {
    return series.reduce((sum, row) => sum + finitePositive(row.value), 0)
  }
  return finitePositive(metric.value)
}

export function sumPromChatBytesPerSec(prometheus?: OpsNetworkPrometheus): number {
  const messagesPerSec = finitePositive(prometheus?.chatMessagesOutPerSec?.value)
  return messagesPerSec * AVG_CHAT_MESSAGE_BYTES
}

export function sampleNodeRate(
  node: SampledNode,
  previousBytesTotal = 0,
  elapsedSec = 0,
): number {
  const directRate = finitePositive(node.bytesPerSec)
  if (directRate > 0) return Math.min(directRate, MAX_SAMPLE_BPS)

  const currentTotal = finitePositive(node.bytesTotal)
  if (currentTotal <= 0 || elapsedSec < MIN_COUNTER_SAMPLE_SEC) return 0

  const delta = currentTotal - finitePositive(previousBytesTotal)
  if (delta <= 0) return 0
  return Math.min(delta / elapsedSec, MAX_SAMPLE_BPS)
}

export function computeNodeRates(
  nodes: SampledNode[],
  sampleState: NetworkSampleState,
  elapsedSec: number,
): Record<string, number> {
  const rates: Record<string, number> = {}
  for (const node of nodes) {
    rates[node.id] = sampleNodeRate(node, sampleState.nodeBytes[node.id] ?? 0, elapsedSec)
  }
  return rates
}

export function computeCategoryRates(
  nodes: SampledNode[],
  sampleState: NetworkSampleState,
  elapsedSec: number,
  prometheusAnalyticsBytesPerSec: number,
  pulseReady: boolean,
  prometheusChatBytesPerSec = 0,
): Record<BandwidthSeriesKey, number> {
  const rates = computeNodeRates(nodes, sampleState, elapsedSec)
  const totals: Record<BandwidthSeriesKey, number> = {
    hls: 0,
    analytics: 0,
    chat: 0,
    core: 0,
    browser: 0,
  }

  const nodesById = new Map(nodes.map(node => [node.id, node]))
  for (const node of nodes) {
    const series = nodeToBandwidthSeries(node.category)
    const parent = node.parentId ? nodesById.get(node.parentId) : undefined
    if (parent && nodeToBandwidthSeries(parent.category) === series) {
      continue
    }
    if (series === 'hls' && node.rateIsEstimated) {
      continue
    }
    totals[series] += rates[node.id] ?? 0
  }

  if (pulseReady) {
    const analyticsRate = finitePositive(prometheusAnalyticsBytesPerSec)
    if (analyticsRate > 0) totals.analytics = analyticsRate

    const chatRate = finitePositive(prometheusChatBytesPerSec)
    if (chatRate > 0) totals.chat = chatRate
  }

  return totals
}
