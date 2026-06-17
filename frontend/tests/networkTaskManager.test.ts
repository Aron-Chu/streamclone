import assert from 'node:assert/strict'
import test from 'node:test'
import { buildNetworkTasks, summarizeNetworkTasks } from '../src/utils/networkTaskManager.ts'

test('buildNetworkTasks includes active stream relay with stop action', () => {
  const tasks = buildNetworkTasks({
    activeStreams: [{
      channel: 'sodapoppin',
      listeners: 1,
      quality: '720p',
      bandwidth: 4_500_000,
    }],
    services: {
      metadata: 'ready',
      chat: 'ready',
      video: 'ready',
      emote: 'ready',
      analytics: 'ready',
      scraper: 'offline',
      clipper: 'offline',
      pulse: 'offline',
    },
    pulseReady: false,
    containers: [],
    pageMonitoringPaused: false,
    clientProbeMbps: 2,
    setupControlAvailable: true,
  })
  const relay = tasks.find(t => t.id === 'relay-sodapoppin')
  assert.ok(relay)
  assert.equal(relay?.canDisable, true)
  assert.equal(relay?.disableAction?.kind, 'stop-relay')
})

test('buildNetworkTasks marks optional scraper stoppable when ready', () => {
  const tasks = buildNetworkTasks({
    activeStreams: [],
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
    clientProbeMbps: null,
    setupControlAvailable: false,
  })
  const scraper = tasks.find(t => t.id === 'optional-scraper')
  assert.ok(scraper?.canDisable)
  assert.match(scraper?.disableWarning ?? '', /viewer charts/i)
})

test('summarizeNetworkTasks counts high impact tasks', () => {
  const summary = summarizeNetworkTasks([
    { id: 'a', name: 'A', category: 'stream', status: 'active', impact: 'high', detail: '', canDisable: true },
    { id: 'b', name: 'B', category: 'core', status: 'active', impact: 'low', detail: '', canDisable: false },
  ] as never)
  assert.equal(summary.highImpact, 1)
  assert.equal(summary.activeCount, 2)
})
