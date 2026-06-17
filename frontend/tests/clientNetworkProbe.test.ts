import assert from 'node:assert/strict'
import test from 'node:test'
import { analyzeClientNetwork, type ClientNetworkSample } from '../src/utils/clientNetworkProbe.ts'

function sample(partial: Partial<ClientNetworkSample> & Pick<ClientNetworkSample, 'at'>): ClientNetworkSample {
  return {
    downlinkMbps: 10,
    rttMs: 50,
    effectiveType: '4g',
    saveData: false,
    probeMbps: 2,
    probeFailed: false,
    ...partial,
  }
}

test('analyzeClientNetwork flags high utilization', () => {
  const analysis = analyzeClientNetwork([
    sample({ at: 1, downlinkMbps: 10, probeMbps: 9.5 }),
  ])
  assert.ok(analysis.utilizationPct != null && analysis.utilizationPct >= 90)
  assert.ok(analysis.warnings.some(w => w.kind === 'high-utilization'))
})

test('analyzeClientNetwork flags unstable RTT jitter', () => {
  const analysis = analyzeClientNetwork([
    sample({ at: 1, rttMs: 40 }),
    sample({ at: 2, rttMs: 120 }),
    sample({ at: 3, rttMs: 45 }),
    sample({ at: 4, rttMs: 130 }),
  ])
  assert.ok(analysis.rttJitterMs != null && analysis.rttJitterMs > 20)
  assert.ok(analysis.warnings.some(w => w.kind === 'unstable-rtt'))
})

test('analyzeClientNetwork flags probe failures', () => {
  const analysis = analyzeClientNetwork([
    sample({ at: 1, probeFailed: true, probeMbps: null }),
    sample({ at: 2, probeFailed: true, probeMbps: null }),
    sample({ at: 3, probeFailed: false, probeMbps: 3 }),
    sample({ at: 4, probeFailed: true, probeMbps: null }),
    sample({ at: 5, probeFailed: true, probeMbps: null }),
  ])
  assert.ok(analysis.probeFailureRate >= 0.6)
  assert.ok(analysis.warnings.some(w => w.kind === 'probe-failures'))
})
