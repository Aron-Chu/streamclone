import assert from 'node:assert/strict'
import test from 'node:test'
import type { NetworkActivityNode } from '../src/utils/networkActivityModel.ts'
import {
  computeCategoryRates,
  computeNodeRates,
  MAX_SAMPLE_BPS,
  sampleNodeRate,
  sumPromAnalyticsBytesPerSec,
  sumPromChatBytesPerSec,
} from '../src/utils/networkRateSampling.ts'

function node(partial: Partial<NetworkActivityNode> & Pick<NetworkActivityNode, 'id' | 'category' | 'name'>): NetworkActivityNode {
  return {
    status: 'active',
    impact: 'low',
    ...partial,
  }
}

test('sumPromAnalyticsBytesPerSec sums analyticsBytesByChannelOp series values', () => {
  const total = sumPromAnalyticsBytesPerSec({
    analyticsBytesByChannelOp: {
      query: 'sum(rate(analytics_sync_bytes_total[1m])) by (channel, op)',
      series: [
        { labels: { channel: 'a', op: 'gql' }, value: 1_200 },
        { labels: { channel: 'b', op: 'tracker' }, value: 800 },
      ],
    },
  })
  assert.equal(total, 2_000)
})

test('computeCategoryRates does not double-count parent and child analytics-op', () => {
  const nodes = [
    node({ id: 'analytics-sync-1', category: 'analytics', name: 'Sync', bytesPerSec: 1_000 }),
    node({
      id: 'analytics-sync-1-gql',
      parentId: 'analytics-sync-1',
      category: 'analytics-op',
      name: 'GQL',
      bytesPerSec: 600,
    }),
  ]
  const rates = computeCategoryRates(nodes, { nodeBytes: {} }, 5, 0, false)
  assert.equal(rates.analytics, 1_000)
})

test('computeCategoryRates prefers Prometheus analytics when pulse is ready', () => {
  const nodes = [
    node({ id: 'analytics-sync-1', category: 'analytics', name: 'Sync', bytesPerSec: 1_000 }),
  ]
  const rates = computeCategoryRates(nodes, { nodeBytes: {} }, 5, 4_500, true)
  assert.equal(rates.analytics, 4_500)
})

test('sampleNodeRate clamps counter-derived rates to MAX_SAMPLE_BPS', () => {
  const hugeNode = node({
    id: 'container-big',
    category: 'core',
    name: 'Big container',
    bytesTotal: 2_000_000_000,
  })
  const rate = sampleNodeRate(hugeNode, 0, 4)
  assert.equal(rate, MAX_SAMPLE_BPS)
})

test('sampleNodeRate returns 0 for counter delta when elapsed is below 4s', () => {
  const counterNode = node({
    id: 'container-small',
    category: 'core',
    name: 'Small container',
    bytesTotal: 40_000,
  })
  assert.equal(sampleNodeRate(counterNode, 0, 2), 0)
})

test('computeCategoryRates excludes browser child from category sum', () => {
  const nodes = [
    node({ id: 'page-network-monitor', category: 'page', name: 'Page poll', bytesPerSec: 100 }),
    node({
      id: 'browser-probe',
      parentId: 'page-network-monitor',
      category: 'browser',
      name: 'Browser probe',
      bytesPerSec: 50,
    }),
  ]
  const rates = computeCategoryRates(nodes, { nodeBytes: {} }, 5, 0, false)
  assert.equal(rates.browser, 100)
})

test('computeNodeRates includes child analytics-op sparkline rates', () => {
  const nodes = [
    node({ id: 'analytics-sync-1', category: 'analytics', name: 'Sync', bytesPerSec: 1_000 }),
    node({
      id: 'analytics-sync-1-gql',
      parentId: 'analytics-sync-1',
      category: 'analytics-op',
      name: 'GQL',
      bytesPerSec: 600,
    }),
  ]
  const rates = computeNodeRates(nodes, { nodeBytes: {} }, 5)
  assert.equal(rates['analytics-sync-1'], 1_000)
  assert.equal(rates['analytics-sync-1-gql'], 600)
})

test('computeCategoryRates excludes estimated HLS bitrate from measured history', () => {
  const nodes = [
    node({
      id: 'relay-x',
      category: 'stream',
      name: 'HLS relay',
      bytesPerSec: 500_000,
      rateIsEstimated: true,
    }),
  ]
  const rates = computeCategoryRates(nodes, { nodeBytes: {} }, 5, 0, false, 0)
  assert.equal(rates.hls, 0)
})

test('sumPromChatBytesPerSec converts message rate to bytes per second', () => {
  const bps = sumPromChatBytesPerSec({
    chatMessagesOutPerSec: { query: 'sum(rate(chat_messages_out_total[1m]))', value: 10 },
  })
  assert.equal(bps, 2_200)
})

test('computeCategoryRates uses Prometheus chat throughput when pulse is ready', () => {
  const rates = computeCategoryRates([], { nodeBytes: {} }, 5, 0, true, 2_200)
  assert.equal(rates.chat, 2_200)
})
