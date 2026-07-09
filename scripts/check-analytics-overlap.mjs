#!/usr/bin/env node
/**
 * Streamclone wrapper — runs streampulse-web overlap guard (sibling checkout).
 * From repo root: node scripts/check-analytics-overlap.mjs
 */
import { spawnSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const repoRoot = join(here, '..')
const pulseWeb = join(repoRoot, '..', 'streamclone-pulse', 'streampulse-web')
const script = join(pulseWeb, 'scripts', 'check-analytics-overlap.mjs')

if (!existsSync(script)) {
  console.error(`check-analytics-overlap: missing ${script}`)
  console.error('Clone streamclone-pulse beside streamclone and keep streampulse-web in sync.')
  process.exit(1)
}

const result = spawnSync(process.execPath, [script], {
  cwd: pulseWeb,
  stdio: 'inherit',
})
process.exit(result.status ?? 1)
