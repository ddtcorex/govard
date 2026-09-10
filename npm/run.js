#!/usr/bin/env node
// Bin shim for @ddtcorex/govard: forwards to the downloaded binary.
'use strict';

const { spawnSync } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');

const binaryName = process.platform === 'win32' ? 'govard.exe' : 'govard';
const binaryPath = path.join(__dirname, 'bin', binaryName);

if (!fs.existsSync(binaryPath)) {
  console.error(
    `@ddtcorex/govard: binary not found at ${binaryPath}. The postinstall download may have been skipped or failed — reinstall with: npm i -g @ddtcorex/govard@latest`,
  );
  process.exit(1);
}

const result = spawnSync(binaryPath, process.argv.slice(2), { stdio: 'inherit' });
process.exit(result.status === null ? 1 : result.status);
