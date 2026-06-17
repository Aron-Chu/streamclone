import { useEffect, useMemo, useRef, useState } from 'react'
import {
  BANDWIDTH_SERIES_META,
  nodeToBandwidthSeries,
  type BandwidthSeriesKey,
  type NetworkActivityNode,
} from '../utils/networkActivityModel'

export const NETWORK_TIME_SERIES_MAX_POINTS = 60

export interface NetworkTimeSeriesInput {
  nodes: NetworkActivityNode[]
  containerRxBytes: number
  containerTxBytes: number
  hasContainerBytes: boolean
  enabled?: boolean
}

export interface NetworkTimeSeriesState {
  categorySeries: Record<BandwidthSeriesKey, number[]>
  nodeSparklines: Record<string, number[]>
  latestCategoryRates: Record<BandwidthSeriesKey, number>
}

const EMPTY_SERIES = (): Record<BandwidthSeriesKey, number[]> => ({
  hls: [],
  analytics: [],
  chat: [],
  core: [],
  browser: [],
})

const EMPTY_RATES = (): Record<BandwidthSeriesKey, number> => ({
  hls: 0,
  analytics: 0,
  chat: 0,
  core: 0,
  browser: 0,
})

export function useNetworkTimeSeries({
  nodes,
  containerRxBytes,
  containerTxBytes,
  hasContainerBytes,
  enabled = true,
}: NetworkTimeSeriesInput): NetworkTimeSeriesState {
  const prevRef = useRef<{
    at: number
    nodeBytes: Record<string, number>
    containerTotal: number
  } | null>(null)

  const [categorySeries, setCategorySeries] = useState(EMPTY_SERIES)
  const [nodeSparklines, setNodeSparklines] = useState<Record<string, number[]>>({})
  const [latestCategoryRates, setLatestCategoryRates] = useState(EMPTY_RATES)

  useEffect(() => {
    if (!enabled) return

    const now = Date.now()
    const nodeBytes: Record<string, number> = {}
    for (const node of nodes) {
      if (node.bytesTotal != null) nodeBytes[node.id] = node.bytesTotal
    }
    const containerTotal = hasContainerBytes ? containerRxBytes + containerTxBytes : 0
    const prev = prevRef.current

    if (!prev) {
      prevRef.current = { at: now, nodeBytes, containerTotal }
      return
    }

    const elapsedSec = Math.max((now - prev.at) / 1000, 0.001)
    const categoryRates = EMPTY_RATES()
    const nodeRates: Record<string, number> = {}

    for (const node of nodes) {
      let bps = node.bytesPerSec ?? 0
      if (node.bytesTotal != null) {
        const delta = Math.max(0, (nodeBytes[node.id] ?? 0) - (prev.nodeBytes[node.id] ?? 0))
        if (delta > 0) bps = Math.max(bps, delta / elapsedSec)
      }
      if (bps <= 0) continue
      nodeRates[node.id] = bps
      categoryRates[nodeToBandwidthSeries(node.category)] += bps
    }

    if (hasContainerBytes) {
      const delta = Math.max(0, containerTotal - prev.containerTotal)
      const containerBps = delta / elapsedSec
      if (containerBps > categoryRates.core) {
        categoryRates.core = containerBps
      }
    }

    setLatestCategoryRates(categoryRates)
    setCategorySeries(current => {
      const next = { ...current }
      for (const key of Object.keys(BANDWIDTH_SERIES_META) as BandwidthSeriesKey[]) {
        next[key] = [...(current[key] ?? []), categoryRates[key]].slice(-NETWORK_TIME_SERIES_MAX_POINTS)
      }
      return next
    })
    setNodeSparklines(current => {
      const next = { ...current }
      for (const [id, rate] of Object.entries(nodeRates)) {
        next[id] = [...(current[id] ?? []), rate].slice(-NETWORK_TIME_SERIES_MAX_POINTS)
      }
      return next
    })

    prevRef.current = { at: now, nodeBytes, containerTotal }
  }, [nodes, containerRxBytes, containerTxBytes, hasContainerBytes, enabled])

  return useMemo(() => ({
    categorySeries,
    nodeSparklines,
    latestCategoryRates,
  }), [categorySeries, nodeSparklines, latestCategoryRates])
}
