// RF-P5-013 — Retire browser-visible clipper mutation token.
// Guard test: the ReplayForge clipper mutation Auth_Token must never be shipped
// to or read by the browser. The token is injected server-side by the
// same-origin /v1/clipper/* proxy (Caddy header_up from CLIPPER_WEBHOOK_TOKEN).
//
// These are static source scans (no runtime), asserting the browser bundle
// source carries no clipper token symbol and clipperHeaders() no longer injects
// a browser-visible Authorization bearer.
import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const here = dirname(fileURLToPath(import.meta.url))
const srcDir = join(here, '../src')

function collectSourceFiles(dir: string): string[] {
  const out: string[] = []
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name)
    if (entry.isDirectory()) {
      out.push(...collectSourceFiles(full))
    } else if (/\.(ts|tsx|js|jsx)$/.test(entry.name)) {
      out.push(full)
    }
  }
  return out
}

const sourceFiles = collectSourceFiles(srcDir)

// The browser must not reference a clipper mutation token by any known name.
const FORBIDDEN_TOKEN_SYMBOLS = ['VITE_CLIPPER_TOKEN', 'CLIPPER_TOKEN', 'clipperToken']

test('frontend/src contains no clipper mutation token symbol', () => {
  const offenders: string[] = []
  for (const file of sourceFiles) {
    const text = readFileSync(file, 'utf8')
    for (const symbol of FORBIDDEN_TOKEN_SYMBOLS) {
      if (text.includes(symbol)) {
        offenders.push(`${file.slice(srcDir.length + 1)} → ${symbol}`)
      }
    }
  }
  assert.deepEqual(
    offenders,
    [],
    `clipper token symbols must not appear in the browser bundle source:\n${offenders.join('\n')}`,
  )
})

test('config.ts does not export a clipper token', () => {
  const configSource = readFileSync(join(srcDir, 'config.ts'), 'utf8')
  assert.ok(
    !/export\s+const\s+CLIPPER_TOKEN/.test(configSource),
    'config.ts must not export CLIPPER_TOKEN',
  )
})

test('clipperHeaders does not inject a browser-visible Authorization bearer', () => {
  const apiSource = readFileSync(join(srcDir, 'api.ts'), 'utf8')
  const start = apiSource.indexOf('function clipperHeaders')
  assert.ok(start >= 0, 'clipperHeaders function should still exist')
  // Grab the function body (up to the next top-level declaration).
  const rawBody = apiSource.slice(start, start + 400)
  // Strip line/block comments so prose mentioning the retired header does not
  // trip the scan — we only care about executable code.
  const body = rawBody
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/\/\/[^\n]*/g, '')
  assert.ok(
    !/\.Authorization\s*=/.test(body),
    'clipperHeaders must not assign an Authorization header in the browser',
  )
  assert.ok(
    !/Bearer\s*\$\{/.test(body) && !/['"`]Bearer/.test(body),
    'clipperHeaders must not build a Bearer token in the browser',
  )
})
