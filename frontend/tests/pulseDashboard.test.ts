import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import { it } from 'node:test'
import { fileURLToPath } from 'node:url'

const testDir = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(testDir, '../..')
const dashboardPath = path.resolve(repoRoot, 'deploy/grafana/dashboards/emote-pulse.json')
const opsDashboardPath = path.resolve(repoRoot, 'deploy/grafana/dashboards/streamclone-ops.json')
const prometheusDatasourcePath = path.resolve(repoRoot, 'deploy/grafana/provisioning/datasources/prometheus.yml')
const prometheusScrapePath = path.resolve(repoRoot, 'deploy/prometheus/prometheus.yml')

it('release Pulse dashboard is rendered for Compose', () => {
  const raw = readFileSync(dashboardPath, 'utf8')
  assert.doesNotMatch(raw, /__[A-Z0-9_]+__/)
  assert.match(raw, /"datasource":\s*"InfluxDB"/)
  assert.match(raw, /http:\/\/localhost:8090\/emotes\//)
  assert.match(raw, /range\(start: -90d\)/)
  assert.match(raw, /strings\.substring\(v: string\(v: r\._time\)/)

  const dashboard = JSON.parse(raw) as {
    uid?: string
    version?: number
    panels?: Array<{ title?: string }>
    templating?: { list?: Array<{ name?: string }> }
  }
  assert.equal(dashboard.uid, 'streamclone-emote-pulse')
  assert.equal(dashboard.version, 10)

  const variableNames = new Set((dashboard.templating?.list ?? []).map(variable => variable.name))
  for (const required of ['channel', 'stream', 'category', 'stream_start', 'stream_end']) {
    assert.equal(variableNames.has(required), true, `missing Grafana variable: ${required}`)
  }

  const panelTitles = new Set((dashboard.panels ?? []).map(panel => panel.title))
  for (const required of ['Emote/chat ratio', 'Chat spikes', 'Viewer peak vs avg', 'Unique emotes/min']) {
    assert.equal(panelTitles.has(required), true, `missing Grafana panel: ${required}`)
  }

  assert.match(raw, /stream_category/)
  assert.match(raw, /schema\.tagValues\(bucket: v\.bucket, tag: \\"stream_category\\"\)/)
  assert.match(raw, /http:\/\/localhost:8090\/analytics\?channel=\$\{channel\}/)
})

it('Pulse dashboard URLs include Emote Pulse and Ops', async () => {
  const { PULSE_DASHBOARD_URL, PULSE_OPS_DASHBOARD_URL, PULSE_DASHBOARD_LINKS } = await import('../src/utils/pulseDashboard.ts')
  assert.match(PULSE_DASHBOARD_URL, /streamclone-emote-pulse/)
  assert.match(PULSE_OPS_DASHBOARD_URL, /streamclone-ops/)
  assert.equal(PULSE_DASHBOARD_LINKS.length, 2)
})

it('ops dashboard and Prometheus provisioning ship with Pulse', () => {
  const opsRaw = readFileSync(opsDashboardPath, 'utf8')
  const opsDashboard = JSON.parse(opsRaw) as { uid?: string }
  assert.equal(opsDashboard.uid, 'streamclone-ops')
  assert.match(opsRaw, /"datasource":\s*"Prometheus"/)
  assert.match(opsRaw, /timeseries_write_attempts_total/)
  assert.match(opsRaw, /http_requests_total/)

  const datasourceRaw = readFileSync(prometheusDatasourcePath, 'utf8')
  assert.match(datasourceRaw, /name:\s*Prometheus/)
  assert.match(datasourceRaw, /http:\/\/prometheus:9090/)

  const scrapeRaw = readFileSync(prometheusScrapePath, 'utf8')
  assert.match(scrapeRaw, /analytics:8080/)
  assert.match(scrapeRaw, /scrape_interval:\s*15s/)
})
