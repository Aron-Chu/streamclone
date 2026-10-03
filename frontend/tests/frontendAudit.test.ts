import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { assertBuildOnlyRuntimeBoundary, validateAuditPolicy, validateAuditSchema } from '../../scripts/frontend-npm-audit-disposition.mjs'

/** Reviewed audit evidence and complete regular lock edges, independent of gate constants. */
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
      'node_modules/braces': { version: '3.0.3', dev: true, dependencies: { 'fill-range': '^7.1.1' } },
      'node_modules/chokidar': { version: '3.6.0', dev: true, dependencies: {
        anymatch: '~3.1.2', braces: '~3.0.2', 'glob-parent': '~5.1.2', 'is-binary-path': '~2.1.0',
        'is-glob': '~4.0.1', 'normalize-path': '~3.0.0', readdirp: '~3.6.0',
      } },
      'node_modules/fast-glob': { version: '3.3.3', dev: true, dependencies: {
        '@nodelib/fs.stat': '^2.0.2', '@nodelib/fs.walk': '^1.2.3', 'glob-parent': '^5.1.2', merge2: '^1.3.0', micromatch: '^4.0.8',
      } },
      'node_modules/micromatch': { version: '4.0.8', dev: true, dependencies: { braces: '^3.0.3', picomatch: '^2.3.1' } },
      'node_modules/tailwindcss': { version: '3.4.19', dev: true, dependencies: {
        '@alloc/quick-lru': '^5.2.0', arg: '^5.0.2', chokidar: '^3.6.0', didyoumean: '^1.2.2',
        dlv: '^1.1.3', 'fast-glob': '^3.3.2', 'glob-parent': '^6.0.2', 'is-glob': '^4.0.3', jiti: '^1.21.7',
        lilconfig: '^3.1.3', micromatch: '^4.0.8', 'normalize-path': '^3.0.0', 'object-hash': '^3.0.0',
        picocolors: '^1.1.1', postcss: '^8.4.47', 'postcss-import': '^15.1.0', 'postcss-js': '^4.0.1',
        'postcss-load-config': '^4.0.2 || ^5.0 || ^6.0', 'postcss-nested': '^6.2.0',
        'postcss-selector-parser': '^6.1.2', resolve: '^1.22.8', sucrase: '^3.35.0',
      } },
    },
  }
  return { audit, lock }
}

/** Clean report used to test failures independently of the disposition graph. */
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

  for (const name of ['braces', 'chokidar', 'fast-glob', 'micromatch', 'tailwindcss']) {
    for (const mutation of ['added', 'missing', 'changed range', 'missing map']) {
      it(`rejects ${mutation} regular dependency on ${name}`, () => {
        const { audit, lock } = reviewedFixture()
        const installed = lock.packages[`node_modules/${name}`]
        const first = Object.keys(installed.dependencies)[0]
        if (mutation === 'added') installed.dependencies.unreviewed = '^1.0.0'
        if (mutation === 'missing') delete installed.dependencies[first]
        if (mutation === 'changed range') installed.dependencies[first] = '^999.0.0'
        if (mutation === 'missing map') delete installed.dependencies
        assert.throws(() => validateAuditPolicy(audit, lock, 1), /Undispositioned/)
      })
    }
  }

  it('accepts complete regular dependency maps with reordered keys', () => {
    const { audit, lock } = reviewedFixture()
    for (const installed of Object.values(lock.packages) as any[]) {
      if (installed.dependencies) installed.dependencies = Object.fromEntries(Object.entries(installed.dependencies).reverse())
    }
    assert.equal(validateAuditPolicy(audit, lock, 1).length, 5)
  })

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
