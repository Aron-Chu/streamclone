import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { assertBuildOnlyRuntimeBoundary, validateAuditPolicy, validateAuditSchema } from '../../scripts/frontend-npm-audit-disposition.mjs'

// The reviewed npm report shape, separate from the gate's internal constants.
function reviewedFixture() {
  const audit: any = {
    auditReportVersion: 2,
    vulnerabilities: {
      braces: { name: 'braces', severity: 'high', nodes: ['node_modules/braces'], via: [{
        source: 1240992, name: 'braces', dependency: 'braces',
        title: 'braces vulnerable to stack-exhaustion denial of service through deeply nested patterns',
        url: 'https://github.com/advisories/GHSA-vfj7-8cjw-p6xm', severity: 'high', range: '<=3.0.3',
      }] },
      chokidar: { name: 'chokidar', severity: 'high', nodes: ['node_modules/chokidar'], via: ['braces'] },
      'fast-glob': { name: 'fast-glob', severity: 'high', nodes: ['node_modules/fast-glob'], via: ['micromatch'] },
      micromatch: { name: 'micromatch', severity: 'high', nodes: ['node_modules/micromatch'], via: ['braces'] },
      tailwindcss: { name: 'tailwindcss', severity: 'high', nodes: ['node_modules/tailwindcss'], via: ['chokidar', 'fast-glob', 'micromatch'] },
    },
    metadata: { vulnerabilities: { info: 0, low: 0, moderate: 0, high: 5, critical: 0, total: 5 } },
  }
  const lock: any = {
    name: 'streamclone-frontend',
    packages: {
      '': { name: 'streamclone-frontend' },
      'node_modules/braces': { version: '3.0.3', dev: true },
      'node_modules/chokidar': { version: '3.6.0', dev: true, dependencies: { braces: '~3.0.2' } },
      'node_modules/fast-glob': { version: '3.3.3', dev: true, dependencies: { micromatch: '^4.0.8' } },
      'node_modules/micromatch': { version: '4.0.8', dev: true, dependencies: { braces: '^3.0.3' } },
      'node_modules/tailwindcss': { version: '3.4.19', dev: true, dependencies: { chokidar: '^3.6.0', 'fast-glob': '^3.3.2', micromatch: '^4.0.8' } },
    },
  }
  return { audit, lock }
}

function cleanAudit(): any {
  return { auditReportVersion: 2, vulnerabilities: {}, metadata: { vulnerabilities: { info: 0, low: 0, moderate: 0, high: 0, critical: 0, total: 0 } } }
}

describe('frontend npm audit disposition', () => {
  it('accepts only the reviewed full development chain, keeping every finding visible', () => {
    const { audit, lock } = reviewedFixture()
    assert.deepEqual(validateAuditSchema(audit), [])
    const findings = validateAuditPolicy(audit, lock, 1)
    assert.equal(findings.length, 5)
    assert.ok(findings.every((finding) => finding.includes('GHSA-vfj7-8cjw-p6xm')))
  })

  it('allows a clean full or production audit without any disposition', () => {
    assert.deepEqual(validateAuditPolicy(cleanAudit(), null, 0), [])
    assert.deepEqual(validateAuditPolicy(cleanAudit(), null, 0, true), [])
  })

  it('does not disposition production high findings, even when the full chain matches', () => {
    const { audit, lock } = reviewedFixture()
    assert.throws(() => validateAuditPolicy(audit, lock, 1, true), /Production audit/)
  })

  it('keeps moderate findings reported under the existing high threshold', () => {
    const audit = cleanAudit()
    audit.vulnerabilities['react-router'] = { name: 'react-router', severity: 'moderate', nodes: ['node_modules/react-router'], via: ['example'] }
    audit.metadata.vulnerabilities.moderate = 1
    audit.metadata.vulnerabilities.total = 1
    assert.deepEqual(validateAuditPolicy(audit, null, 0, true), ['moderate react-router: remains reported'])
  })

  for (const field of ['source', 'name', 'dependency', 'title', 'url', 'severity', 'range']) {
    it(`rejects advisory ${field} drift`, () => {
      const { audit, lock } = reviewedFixture()
      audit.vulnerabilities.braces.via[0][field] = 'changed'
      assert.throws(() => validateAuditPolicy(audit, lock, 1), /Undispositioned/)
    })
  }

  for (const [description, mutate] of Object.entries({
    'unknown high': (a: any) => { a.vulnerabilities.other = { name: 'other', severity: 'high', nodes: ['node_modules/other'], via: ['braces'] }; a.metadata.vulnerabilities.high++; a.metadata.vulnerabilities.total++ },
    'critical chain': (a: any) => { a.vulnerabilities.braces.severity = 'critical'; a.metadata.vulnerabilities.high--; a.metadata.vulnerabilities.critical++ },
    'extra advisory': (a: any) => a.vulnerabilities.braces.via.push({ source: 9, url: 'https://example.invalid/advisory' }),
    'missing ancestor': (a: any) => { delete a.vulnerabilities.micromatch; a.metadata.vulnerabilities.high--; a.metadata.vulnerabilities.total-- },
    'redirected edge': (a: any) => { a.vulnerabilities.chokidar.via = ['micromatch'] },
    'duplicate edge': (a: any) => { a.vulnerabilities.tailwindcss.via = ['chokidar', 'chokidar', 'micromatch'] },
    'cycle': (a: any) => { a.vulnerabilities.chokidar.via = ['tailwindcss'] },
    'additional reported node': (a: any) => a.vulnerabilities.braces.nodes.push('node_modules/other/node_modules/braces'),
    'changed reported path': (a: any) => { a.vulnerabilities.braces.nodes = ['node_modules/other/node_modules/braces'] },
    'npm report error': (a: any) => { a.error = { code: 'EAUDITNOLOCK' } },
    'wrong report version': (a: any) => { a.auditReportVersion = 1 },
    'missing vulnerabilities': (a: any) => { delete a.vulnerabilities },
    'missing metadata': (a: any) => { delete a.metadata },
    'wrong package count': (a: any) => { a.metadata.vulnerabilities.high = 0 },
    'wrong total': (a: any) => { a.metadata.vulnerabilities.total = 0 },
    'name mismatch': (a: any) => { a.vulnerabilities.braces.name = 'other' },
    'missing via': (a: any) => { delete a.vulnerabilities.braces.via },
    'invalid nodes': (a: any) => { a.vulnerabilities.braces.nodes = [null] },
  })) {
    it(`rejects ${description}`, () => {
      const { audit, lock } = reviewedFixture()
      mutate(audit)
      assert.throws(() => validateAuditPolicy(audit, lock, 1))
    })
  }

  for (const [description, mutate] of Object.entries({
    'wrong package identity': (l: any) => { l.name = 'other' },
    'wrong root identity': (l: any) => { l.packages[''].name = 'other' },
    'changed version': (l: any) => { l.packages['node_modules/braces'].version = '3.0.4' },
    'production node': (l: any) => { l.packages['node_modules/braces'].dev = false },
    'missing dev evidence': (l: any) => { delete l.packages['node_modules/micromatch'].dev },
    'missing installed node': (l: any) => { delete l.packages['node_modules/chokidar'] },
    'nested installed node': (l: any) => { l.packages['node_modules/other/node_modules/braces'] = { version: '3.0.3', dev: true } },
    'changed installed dependency': (l: any) => { l.packages['node_modules/chokidar'].dependencies.braces = '^3.0.3' },
  })) {
    it(`rejects lock ${description}`, () => {
      const { audit, lock } = reviewedFixture()
      mutate(lock)
      assert.throws(() => validateAuditPolicy(audit, lock, 1), /Undispositioned/)
    })
  }

  for (const status of [null, 2, -1, 127]) {
    it(`rejects npm command failure status ${status}`, () => assert.throws(() => validateAuditPolicy(cleanAudit(), null, status), /command failed/))
  }
  it('rejects inconsistent npm exit status', () => {
    const { audit, lock } = reviewedFixture()
    assert.throws(() => validateAuditPolicy(audit, lock, 0), /disagrees/)
    assert.throws(() => validateAuditPolicy(cleanAudit(), null, 1), /disagrees/)
  })
  for (const report of [null, [], 'not JSON', { error: {} }]) {
    it(`rejects invalid report ${JSON.stringify(report)}`, () => assert.throws(() => validateAuditPolicy(report, null, 0), /Invalid/))
  }
})

describe('development-only browser output boundary', () => {
  it('accepts normal application chunks and CSS assets', () => {
    assert.doesNotThrow(() => assertBuildOnlyRuntimeBoundary({
      'app.js': { type: 'chunk', modules: { '/repo/src/main.tsx': {}, '/repo/node_modules/react/index.js': {} } },
      'styles.css': { type: 'asset', source: '@tailwind is only text here' },
    }))
  })
  for (const name of ['braces', 'chokidar', 'fast-glob', 'micromatch', 'tailwindcss']) {
    for (const path of [`/repo/node_modules/${name}/index.js`, `C:\\repo\\node_modules\\${name}\\index.js`, `/repo/node_modules/other/node_modules/${name}/index.js`]) {
      it(`rejects a lazy chunk containing ${path}`, () => {
        assert.throws(() => assertBuildOnlyRuntimeBoundary({
          'entry.js': { type: 'chunk', isEntry: true, modules: { '/repo/src/main.tsx': {} } },
          'lazy.js': { type: 'chunk', isEntry: false, modules: { [path]: {} } },
        }), /Development-only audit dependency/)
      })
    }
  }
  it('rejects chunks without module evidence', () => assert.throws(() => assertBuildOnlyRuntimeBoundary({ 'app.js': { type: 'chunk' } }), /modules are required/))
})
