#!/usr/bin/env node
'use strict';

const { spawnSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const VERSION = '0.0.15';
const PKG = `@gethopp/figma-mcp-bridge@${VERSION}`;
const depRoot = path.join(__dirname, '.figma-bridge-deps');
const main = path.join(
  depRoot,
  'node_modules',
  '@gethopp',
  'figma-mcp-bridge',
  'dist',
  'index.js',
);

function ensureDeps() {
  if (fs.existsSync(main)) {
    return main;
  }

  fs.mkdirSync(depRoot, { recursive: true });
  const npmCli = path.join(path.dirname(process.execPath), 'node_modules', 'npm', 'bin', 'npm-cli.js');
  if (!fs.existsSync(npmCli)) {
    console.error(`FAIL: npm-cli.js not found at ${npmCli}`);
    process.exit(1);
  }
  const result = spawnSync(
    process.execPath,
    [npmCli, 'install', '--prefix', depRoot, '--no-fund', '--no-audit', PKG],
    { stdio: 'inherit', windowsHide: true },
  );

  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
  if (!fs.existsSync(main)) {
    console.error(`FAIL: expected entrypoint missing after install: ${main}`);
    process.exit(1);
  }
  return main;
}

if (require.main === module) {
  const entry = ensureDeps();
  console.log(entry);
}

module.exports = { ensureDeps, main, depRoot, VERSION };
