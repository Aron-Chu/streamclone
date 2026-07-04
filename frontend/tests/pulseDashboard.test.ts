import assert from 'node:assert/strict'
import { existsSync } from 'node:fs'
import path from 'node:path'
import { it } from 'node:test'
import { fileURLToPath } from 'node:url'

const testDir = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(testDir, '../..')

const removedHostedOpsPaths = [
  'deploy/grafana/dashboards/emote-pulse.json',
  'deploy/grafana/dashboards/streamclone-ops.json',
  'deploy/grafana/provisioning/datasources/prometheus.yml',
  'deploy/prometheus/prometheus.yml',
  'charts/pulse/Chart.yaml',
]

it('public repo does not ship hosted Grafana/Prometheus deploy trees', () => {
  for (const rel of removedHostedOpsPaths) {
    assert.equal(
      existsSync(path.resolve(repoRoot, rel)),
      false,
      `legacy hosted ops path should be removed from public streamclone: ${rel}`,
    )
  }
})

it('Pulse dashboard URLs include Emote Pulse and Ops', async () => {
  const { PULSE_DASHBOARD_URL, PULSE_OPS_DASHBOARD_URL, PULSE_DASHBOARD_LINKS } = await import('../src/utils/pulseDashboard.ts')
  assert.match(PULSE_DASHBOARD_URL, /streamclone-emote-pulse/)
  assert.match(PULSE_OPS_DASHBOARD_URL, /streamclone-ops/)
  assert.equal(PULSE_DASHBOARD_LINKS.length, 2)
})
