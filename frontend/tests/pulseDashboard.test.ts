import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import { it } from 'node:test'
import { fileURLToPath } from 'node:url'

const testDir = path.dirname(fileURLToPath(import.meta.url))
const dashboardPath = path.resolve(testDir, '../../deploy/grafana/dashboards/emote-pulse.json')

it('release Pulse dashboard is rendered for Compose', () => {
  const raw = readFileSync(dashboardPath, 'utf8')
  assert.doesNotMatch(raw, /__[A-Z0-9_]+__/)
  assert.match(raw, /"datasource":\s*"InfluxDB"/)
  assert.match(raw, /http:\/\/localhost:8090\/emotes\//)
  assert.match(raw, /range\(start: -90d\)/)

  const dashboard = JSON.parse(raw) as {
    uid?: string
    templating?: { list?: Array<{ name?: string }> }
  }
  assert.equal(dashboard.uid, 'streamclone-emote-pulse')

  const variableNames = new Set((dashboard.templating?.list ?? []).map(variable => variable.name))
  for (const required of ['channel', 'stream', 'stream_start', 'stream_end']) {
    assert.equal(variableNames.has(required), true, `missing Grafana variable: ${required}`)
  }
})
