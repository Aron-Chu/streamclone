/**
 * Exact development-only braces disposition. The production audit has no
 * dispositions. New advisories, graph drift, report errors, and browser imports
 * fail the gate. See docs/security.md; the vulnerable dependency is not patched.
 * Adapted from streamclone-pulse's reviewed npm audit and runtime boundary gate.
 */
import { spawnSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

const SEVERITIES = ['info', 'low', 'moderate', 'high', 'critical']
// Complete regular dependency maps from the reviewed 86e7014c frontend lock.
// Optional/peer dependencies are audited but are not frozen by this comparison.
const CHAIN = {
  braces: { version: '3.0.3', via: [], dependencies: { 'fill-range': '^7.1.1' } },
  chokidar: { version: '3.6.0', via: ['braces'], dependencies: {
    anymatch: '~3.1.2', braces: '~3.0.2', 'glob-parent': '~5.1.2',
    'is-binary-path': '~2.1.0', 'is-glob': '~4.0.1', 'normalize-path': '~3.0.0', readdirp: '~3.6.0',
  } },
  'fast-glob': { version: '3.3.3', via: ['micromatch'], dependencies: {
    '@nodelib/fs.stat': '^2.0.2', '@nodelib/fs.walk': '^1.2.3', 'glob-parent': '^5.1.2',
    merge2: '^1.3.0', micromatch: '^4.0.8',
  } },
  micromatch: { version: '4.0.8', via: ['braces'], dependencies: { braces: '^3.0.3', picomatch: '^2.3.1' } },
  tailwindcss: { version: '3.4.19', via: ['chokidar', 'fast-glob', 'micromatch'], dependencies: {
    '@alloc/quick-lru': '^5.2.0', arg: '^5.0.2', chokidar: '^3.6.0', didyoumean: '^1.2.2',
    dlv: '^1.1.3', 'fast-glob': '^3.3.2', 'glob-parent': '^6.0.2', 'is-glob': '^4.0.3',
    jiti: '^1.21.7', lilconfig: '^3.1.3', micromatch: '^4.0.8', 'normalize-path': '^3.0.0',
    'object-hash': '^3.0.0', picocolors: '^1.1.1', postcss: '^8.4.47', 'postcss-import': '^15.1.0',
    'postcss-js': '^4.0.1', 'postcss-load-config': '^4.0.2 || ^5.0 || ^6.0', 'postcss-nested': '^6.2.0',
    'postcss-selector-parser': '^6.1.2', resolve: '^1.22.8', sucrase: '^3.35.0',
  } },
}
const ADVISORY = {
  source: 1240992,
  name: 'braces',
  dependency: 'braces',
  title: 'braces vulnerable to stack-exhaustion denial of service through deeply nested patterns',
  url: 'https://github.com/advisories/GHSA-vfj7-8cjw-p6xm',
  severity: 'high',
  range: '<=3.0.3',
}
/** Distinguish object-shaped npm evidence from arrays and null. */
const isRecord = (value) => value !== null && typeof value === 'object' && !Array.isArray(value)

/** Validate npm report v2 evidence and package counts before any disposition. */
export function validateAuditSchema(audit) {
  if (!isRecord(audit)) return ['report must be an object']
  const errors = []
  if (Object.hasOwn(audit, 'error')) errors.push('npm audit reported an error')
  if (audit.auditReportVersion !== 2) errors.push('auditReportVersion must be 2')
  if (!isRecord(audit.vulnerabilities) || !isRecord(audit.metadata?.vulnerabilities)) {
    return [...errors, 'vulnerabilities and metadata.vulnerabilities are required']
  }
  const actual = Object.fromEntries(SEVERITIES.map((severity) => [severity, 0]))
  for (const [name, info] of Object.entries(audit.vulnerabilities)) {
    if (!isRecord(info) || info.name !== name || !SEVERITIES.includes(info.severity)) {
      errors.push(`invalid vulnerability entry: ${name}`)
      continue
    }
    actual[info.severity] += 1
    if (!Array.isArray(info.via) || info.via.length === 0) errors.push(`missing via: ${name}`)
    if (!Array.isArray(info.nodes) || info.nodes.length === 0 || info.nodes.some((node) => typeof node !== 'string')) {
      errors.push(`invalid nodes: ${name}`)
    }
  }
  for (const severity of SEVERITIES) {
    const count = audit.metadata.vulnerabilities[severity]
    if (!Number.isInteger(count) || count < 0 || count !== actual[severity]) errors.push(`incorrect ${severity} count`)
  }
  if (audit.metadata.vulnerabilities.total !== Object.keys(audit.vulnerabilities).length) errors.push('incorrect total count')
  return errors
}

/** Compare every regular dependency key/range without depending on key order. */
function exactRegularDependencies(actual, expected) {
  return isRecord(actual) && Object.keys(actual).length === Object.keys(expected).length &&
    Object.entries(expected).every(([name, range]) => Object.hasOwn(actual, name) && actual[name] === range)
}

/** Require reviewed node identity, complete regular dependencies and advisory edges. */
function exactDevelopmentChain(vulnerabilities, lock) {
  if (lock?.name !== 'streamclone-frontend' || lock?.packages?.['']?.name !== 'streamclone-frontend' || !isRecord(lock.packages)) {
    return false
  }
  // A second occurrence, including a nested production occurrence, is outside
  // this review even when npm reports only the top-level affected node.
  for (const name of Object.keys(CHAIN)) {
    const paths = Object.keys(lock.packages).filter((path) => path === `node_modules/${name}` || path.endsWith(`/node_modules/${name}`))
    if (paths.length !== 1 || paths[0] !== `node_modules/${name}`) return false
  }
  /** Follow only the reviewed vulnerability path, rejecting missing nodes or cycles. */
  function matches(name, seen) {
    const expected = Object.hasOwn(CHAIN, name) ? CHAIN[name] : undefined
    const info = vulnerabilities[name]
    const path = `node_modules/${name}`
    const installed = lock.packages[path]
    if (!expected || seen.has(name) || !isRecord(info) || info.severity !== 'high' ||
        !Array.isArray(info.nodes) || info.nodes.length !== 1 || info.nodes[0] !== path ||
        installed?.dev !== true || installed?.version !== expected.version || !Array.isArray(info.via) ||
        !exactRegularDependencies(installed.dependencies, expected.dependencies)) return false
    if (name === 'braces') {
      return info.via.length === 1 && isRecord(info.via[0]) &&
        Object.entries(ADVISORY).every(([key, value]) => info.via[0][key] === value)
    }
    const nextSeen = new Set(seen).add(name)
    return info.via.length === expected.via.length && new Set(info.via).size === info.via.length &&
      info.via.every((parent) => typeof parent === 'string' && expected.via.includes(parent) && matches(parent, nextSeen))
  }
  return matches('tailwindcss', new Set())
}

/** Keep unknown high/critical findings fatal; production has no dispositions. */
export function validateAuditPolicy(audit, lock, npmStatus, production = false) {
  if (npmStatus !== 0 && npmStatus !== 1) throw new Error(`npm audit command failed: status ${npmStatus}`)
  const errors = validateAuditSchema(audit)
  if (errors.length) throw new Error(`Invalid npm audit report: ${errors.join('; ')}`)
  const highs = Object.entries(audit.vulnerabilities).filter(([, info]) => info.severity === 'high' || info.severity === 'critical')
  if ((highs.length === 0 ? 0 : 1) !== npmStatus) throw new Error('npm audit status disagrees with high/critical findings')
  if (production && highs.length) throw new Error(`Production audit has high/critical findings: ${highs.map(([name]) => name).join(', ')}`)
  if (!production && highs.length &&
      (highs.some(([name]) => !Object.hasOwn(CHAIN, name)) || !exactDevelopmentChain(audit.vulnerabilities, lock))) {
    throw new Error('Undispositioned high/critical finding or drift from the exact development-only braces chain')
  }
  return Object.entries(audit.vulnerabilities).map(([name, info]) =>
    `${info.severity} ${name}${highs.some(([high]) => high === name) ? ': exact development-only GHSA-vfj7-8cjw-p6xm disposition' : ': remains reported'}`,
  )
}

/** All chunks are inspected, including dynamic imports and non-entry chunks. */
export function assertBuildOnlyRuntimeBoundary(bundle) {
  if (!isRecord(bundle)) throw new Error('Build output bundle is required')
  const forbidden = new Set()
  for (const chunk of Object.values(bundle)) {
    if (chunk.type !== 'chunk') continue
    if (!isRecord(chunk.modules)) throw new Error('Emitted chunk modules are required')
    for (const id of Object.keys(chunk.modules)) {
      const normalized = id.replaceAll('\\', '/')
      if (Object.keys(CHAIN).some((name) => normalized.includes(`/node_modules/${name}/`))) forbidden.add(id)
    }
  }
  if (forbidden.size) throw new Error(`Development-only audit dependency in browser output:\n${[...forbidden].join('\n')}`)
}

/** Collect full and production evidence with explicit dependency inclusion scope. */
function runAudits() {
  const reports = process.env.STREAMCLONE_AUDIT_REPORT_DIR
    ? resolve(process.env.STREAMCLONE_AUDIT_REPORT_DIR)
    : mkdtempSync(join(tmpdir(), 'streamclone-frontend-audit-'))
  mkdirSync(reports, { recursive: true })
  console.log(`Raw npm audit reports: ${reports}`)
  const lock = JSON.parse(readFileSync('package-lock.json', 'utf8'))
  const results = []
  // Preserve both raw reports before considering the only permitted disposition.
  for (const production of [false, true]) {
    const name = production ? 'production' : 'full'
    // CLI include overrides inherited omit/defaults; the explicit production
    // include list replaces an inherited include=dev before omit=dev is applied.
    const args = ['audit', '--json', '--audit-level=high', '--include=prod', '--include=optional', '--include=peer',
      ...(production ? ['--omit=dev'] : ['--include=dev'])]
    // Windows command text consists only of the constant arguments above.
    const result = spawnSync(process.platform === 'win32' ? 'cmd.exe' : 'npm',
      process.platform === 'win32' ? ['/d', '/s', '/c', ['npm', ...args].join(' ')] : args, {
      encoding: 'utf8', timeout: 60_000, maxBuffer: 16 * 1024 * 1024,
    })
    writeFileSync(join(reports, `${name}.json`), result.stdout ?? '', { flag: 'wx' })
    writeFileSync(join(reports, `${name}.stderr.txt`), result.stderr ?? '', { flag: 'wx' })
    writeFileSync(join(reports, `${name}.command.json`), JSON.stringify({ args, status: result.status, signal: result.signal, error: result.error?.message }, null, 2), { flag: 'wx' })
    results.push({ name, production, result })
  }
  for (const { name, production, result } of results) {
    if (result.error || result.signal) throw new Error(`${name} audit could not complete: ${result.error?.message ?? result.signal}`)
    let report
    try { report = JSON.parse(result.stdout) } catch { throw new Error(`${name} audit returned invalid JSON; raw output retained`) }
    const findings = validateAuditPolicy(report, lock, result.status, production)
    console.log(`${name} audit: ${findings.length} reported vulnerability entries`)
    for (const finding of findings) console.log(`  ${finding}`)
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try { runAudits() } catch (error) {
    console.error(error.message)
    process.exitCode = 1
  }
}
