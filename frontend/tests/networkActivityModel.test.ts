import assert from 'node:assert/strict'
import test from 'node:test'
import {
  attachSparklines,
  buildNetworkActivityNodes,
  filterNodesBySeries,
  filterNodesByTab,
  formatBytes,
  formatRate,
  nodeToBandwidthSeries,
  sortActivityNodes,
  summarizeActivityNodes,
} from '../src/utils/networkActivityModel.ts'

test('buildNetworkActivityNodes creates analytics sync hierarchy with child ops', () => {
  const nodes = buildNetworkActivityNodes({
    activeStreams: [],
    activeAnalyticsSyncs: [{
      streamId: '123',
      channel: 'sodapoppin',
      phase: 'fetching_comments',
      network: {
        trackerScrapeBytes: 1_800_000,
        gqlFetchBytes: 12_400_000,
        emotePreloadBytes: 340_000,
        totalBytes: 14_540_000,
        lastRateBps: 6_800_000,
      },
      chat: { gqlPages: 42, segmentsDone: 3, segmentsTotal: 8 },
    }],
    services: {
      metadata: 'ready',
      chat: 'ready',
      video: 'ready',
      emote: 'ready',
      analytics: 'ready',
      scraper: 'ready',
      clipper: 'offline',
      pulse: 'offline',
    },
    pulseReady: false,
    containers: [],
    pageMonitoringPaused: false,
    clientProbeMbps: 5,
    setupControlAvailable: true,
  })

  const parent = nodes.find(n => n.id === 'analytics-sync-123')
  assert.ok(parent)
  assert.equal(parent?.channel, 'sodapoppin')
  assert.equal(parent?.bytesPerSec, 850_000)

  const tracker = nodes.find(n => n.id === 'analytics-sync-123-tracker')
  const gql = nodes.find(n => n.id === 'analytics-sync-123-gql')
  assert.ok(tracker)
  assert.ok(gql)
  assert.equal(tracker?.parentId, parent?.id)
  assert.equal(gql?.parentId, parent?.id)
  assert.equal(tracker?.bytesTotal, 1_800_000)
  assert.equal(gql?.bytesTotal, 12_400_000)
})

test('buildNetworkActivityNodes includes stream relay with estimated rate from bandwidth', () => {
  const nodes = buildNetworkActivityNodes({
    activeStreams: [{
      channel: 'sodapoppin',
      listeners: 1,
      quality: '720p',
      bandwidth: 4_000_000,
    }],
    pulseReady: false,
    containers: [],
    pageMonitoringPaused: false,
    clientProbeMbps: null,
    setupControlAvailable: false,
  })
  const relay = nodes.find(n => n.id === 'relay-sodapoppin')
  assert.ok(relay)
  assert.equal(relay?.bytesPerSec, 500_000)
  assert.equal(relay?.disableAction?.kind, 'stop-relay')
})

test('filterNodesByTab keeps analytics child rows with parent', () => {
  const nodes = buildNetworkActivityNodes({
    activeStreams: [{ channel: 'a', listeners: 1 }],
    activeAnalyticsSyncs: [{
      streamId: '1',
      channel: 'b',
      network: { gqlFetchBytes: 1000, totalBytes: 1000, lastRateBps: 50 },
    }],
    pulseReady: false,
    containers: [],
    pageMonitoringPaused: false,
    clientProbeMbps: null,
    setupControlAvailable: false,
  })
  const analyticsTab = filterNodesByTab(nodes, 'analytics')
  assert.ok(analyticsTab.some(n => n.category === 'analytics'))
  assert.ok(analyticsTab.some(n => n.category === 'analytics-op'))
  const streamsTab = filterNodesByTab(nodes, 'streams')
  assert.ok(streamsTab.every(n => n.category === 'stream' || n.parentId?.startsWith('relay-')))
})

test('filterNodesBySeries maps stream nodes to hls series', () => {
  const nodes = buildNetworkActivityNodes({
    activeStreams: [{ channel: 'x', listeners: 1, bandwidth: 1_000_000 }],
    pulseReady: false,
    containers: [],
    pageMonitoringPaused: false,
    clientProbeMbps: null,
    setupControlAvailable: false,
  })
  const filtered = filterNodesBySeries(nodes, 'hls')
  assert.ok(filtered.some(n => n.id === 'relay-x'))
})

test('nodeToBandwidthSeries maps categories to stacked chart keys', () => {
  assert.equal(nodeToBandwidthSeries('stream'), 'hls')
  assert.equal(nodeToBandwidthSeries('analytics-op'), 'analytics')
  assert.equal(nodeToBandwidthSeries('tracking'), 'chat')
  assert.equal(nodeToBandwidthSeries('core'), 'core')
  assert.equal(nodeToBandwidthSeries('page'), 'browser')
})

test('sortActivityNodes prefers higher bytesPerSec', () => {
  const sorted = sortActivityNodes([
    { id: 'a', category: 'stream', name: 'A', status: 'active', impact: 'low', bytesPerSec: 100 },
    { id: 'b', category: 'stream', name: 'B', status: 'active', impact: 'high', bytesPerSec: 900 },
  ] as never)
  assert.equal(sorted[0]?.id, 'b')
})

test('attachSparklines merges series into nodes', () => {
  const nodes = attachSparklines(
    [{ id: 'a', category: 'stream', name: 'A', status: 'active', impact: 'low' } as never],
    { a: [1, 2, 3] },
  )
  assert.deepEqual(nodes[0]?.sparkline, [1, 2, 3])
})

test('summarizeActivityNodes counts analytics syncs and tracking', () => {
  const nodes = buildNetworkActivityNodes({
    activeStreams: [],
    activeAnalyticsSyncs: [{ streamId: '1', channel: 'c', network: { totalBytes: 1 } }],
    trackingSnapshot: { tracked: ['c', 'd'] },
    pulseReady: false,
    containers: [],
    pageMonitoringPaused: false,
    clientProbeMbps: null,
    setupControlAvailable: false,
  })
  const summary = summarizeActivityNodes(nodes)
  assert.equal(summary.analyticsSyncs, 1)
  assert.equal(summary.trackedChannels, 2)
  assert.ok(summary.stoppable >= 2)
})

test('tracking nodes expose untrack action', () => {
  const nodes = buildNetworkActivityNodes({
    activeStreams: [],
    trackingSnapshot: { tracked: ['xqc'] },
    pulseReady: false,
    containers: [],
    pageMonitoringPaused: false,
    clientProbeMbps: null,
    setupControlAvailable: false,
  })
  const tracking = nodes.find(node => node.id === 'tracking-xqc')
  assert.equal(tracking?.canDisable, true)
  assert.equal(tracking?.disableLabel, 'Untrack')
  assert.deepEqual(tracking?.disableAction, { kind: 'untrack-channel', channel: 'xqc' })
})

test('formatBytes and formatRate render human units', () => {
  assert.equal(formatBytes(1536), '1.5 KB')
  assert.match(formatRate(2048), /KB\/s/)
})
