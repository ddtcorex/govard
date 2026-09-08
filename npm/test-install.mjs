import assert from 'node:assert/strict';
import { platformAsset } from './install.js';

const asset = platformAsset(process.platform, process.arch);
assert.match(
  asset,
  /^(Linux|Darwin|Windows)_(amd64|arm64)\.(tar\.gz|zip)$/,
  `unexpected asset: ${asset}`,
);
console.log(`OK: ${process.platform}/${process.arch} -> ${asset}`);
