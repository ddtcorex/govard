// Postinstall downloader for @ddtcorex/govard.
// Downloads the matching GoReleaser asset from GitHub Releases, verifies its
// sha256 against checksums.txt, extracts it to ./bin, and records the npm
// install-source marker. Stdlib only, no dependencies.
'use strict';

const { execFileSync } = require('node:child_process');
const crypto = require('node:crypto');
const fs = require('node:fs');
const https = require('node:https');
const os = require('node:os');
const path = require('node:path');

const REPO = 'ddtcorex/govard';
const BIN_DIR = path.join(__dirname, 'bin');
const BINARY_NAME = process.platform === 'win32' ? 'govard.exe' : 'govard';

// Maps Node platform/arch to the GoReleaser asset infix
// (name_template: "{{ .ProjectName }}_{{ .Version }}_{{- title .Os }}_{{ .Arch }}").
function platformAsset(platform, arch) {
  const normalizedArch = arch === 'x64' ? 'amd64' : arch === 'arm64' ? 'arm64' : null;
  if (normalizedArch === null) {
    throw new Error(`Unsupported architecture for @ddtcorex/govard: ${arch}. See https://github.com/${REPO} for manual install via install.sh.`);
  }
  switch (platform) {
    case 'linux':
      return `Linux_${normalizedArch}.tar.gz`;
    case 'darwin':
      return `Darwin_${normalizedArch}.tar.gz`;
    case 'win32':
      return `Windows_${normalizedArch}.zip`;
    default:
      throw new Error(`Unsupported platform for @ddtcorex/govard: ${platform}/${arch}. See https://github.com/${REPO} for manual install via install.sh.`);
  }
}

function packageVersion() {
  const pkg = JSON.parse(fs.readFileSync(path.join(__dirname, 'package.json'), 'utf8'));
  const version = String(pkg.version || '').trim();
  if (version === '0.0.0-dev') {
    throw new Error('Refusing to download: wrapper version is still the unstamped 0.0.0-dev placeholder (the release job stamps it from the git tag).');
  }
  if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version)) {
    throw new Error(`Refusing to download: package version ${JSON.stringify(version)} is not a release version (the release job stamps it from the git tag).`);
  }
  return version;
}

function downloadWithCurl(url, dest) {
  execFileSync('curl', ['-fsSL', '--retry', '3', '-o', dest, url], { stdio: 'inherit' });
}

function downloadWithNode(url, dest, redirects = 5) {
  return new Promise((resolve, reject) => {
    https
      .get(url, { headers: { 'User-Agent': '@ddtcorex/govard-installer' } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          if (redirects <= 0) {
            reject(new Error(`Too many redirects downloading ${url}`));
            return;
          }
          res.resume();
          resolve(downloadWithNode(res.headers.location, dest, redirects - 1));
          return;
        }
        if (res.statusCode !== 200) {
          reject(new Error(`Download failed (${res.statusCode}): ${url}`));
          return;
        }
        const out = fs.createWriteStream(dest);
        res.pipe(out);
        out.on('finish', () => out.close(resolve));
        out.on('error', reject);
      })
      .on('error', reject);
  });
}

async function download(url, dest) {
  try {
    execFileSync('curl', ['--version'], { stdio: 'ignore' });
    downloadWithCurl(url, dest);
  } catch {
    await downloadWithNode(url, dest);
  }
}

function sha256File(filePath) {
  const hash = crypto.createHash('sha256');
  hash.update(fs.readFileSync(filePath));
  return hash.digest('hex');
}

function extract(archivePath, destDir) {
  fs.mkdirSync(destDir, { recursive: true });
  if (archivePath.endsWith('.zip')) {
    execFileSync(
      'powershell',
      ['-NoProfile', '-Command', `Expand-Archive -Force '${archivePath}' '${destDir}'`],
      { stdio: 'inherit' },
    );
    return;
  }
  execFileSync('tar', ['-xzf', archivePath, '-C', destDir], { stdio: 'inherit' });
}

async function main() {
  if (process.env.GOVARD_SKIP_POSTINSTALL) {
    console.log('@ddtcorex/govard: GOVARD_SKIP_POSTINSTALL set, skipping binary download.');
    return;
  }
  const version = packageVersion();
  const assetInfix = platformAsset(process.platform, process.arch);
  const mirror = (process.env.GOVARD_MIRROR || '').replace(/\/+$/, '');
  const base = mirror || `https://github.com/${REPO}/releases/download/v${version}`;
  const archiveName = `govard_${version}_${assetInfix}`;
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'govard-npm-'));
  try {
    const archivePath = path.join(tmpDir, archiveName);
    console.log(`@ddtcorex/govard: downloading ${archiveName} ...`);
    await download(`${base}/${archiveName}`, archivePath);

    console.log('@ddtcorex/govard: verifying checksum ...');
    const checksumsPath = path.join(tmpDir, 'checksums.txt');
    await download(`${base}/checksums.txt`, checksumsPath);
    const line = fs
      .readFileSync(checksumsPath, 'utf8')
      .split('\n')
      .find((l) => l.trim().endsWith(`  ${archiveName}`) || l.trim().endsWith(` *${archiveName}`));
    if (!line) {
      throw new Error(`Checksum entry for ${archiveName} not found in checksums.txt.`);
    }
    const expected = line.trim().split(/\s+/)[0];
    const actual = sha256File(archivePath);
    if (expected !== actual) {
      throw new Error(`Checksum mismatch for ${archiveName}: expected ${expected}, got ${actual}.`);
    }

    console.log('@ddtcorex/govard: extracting ...');
    extract(archivePath, BIN_DIR);
    const binaryPath = path.join(BIN_DIR, BINARY_NAME);
    if (!fs.existsSync(binaryPath)) {
      throw new Error(`Archive ${archiveName} does not contain ${BINARY_NAME}.`);
    }
    if (process.platform !== 'win32') {
      fs.chmodSync(binaryPath, 0o755);
    }
    fs.writeFileSync(path.join(BIN_DIR, '.install-source'), 'npm\n');
    console.log(`@ddtcorex/govard: installed ${BINARY_NAME} ${version}.`);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

if (require.main === module) {
  main().catch((err) => {
    console.error(`@ddtcorex/govard postinstall failed: ${err.message}`);
    process.exit(1);
  });
}

module.exports = { platformAsset };
