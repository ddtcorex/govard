// Records where the frontend build ran, so the deploy's own verification can
// assert it happened inside the theme's Tailwind directory and not somewhere
// else in the release.
//
// The record goes to the directory named by stub-state-dir.txt at the project
// root: the build runs in the release and the verification runs in the *served*
// path, which on an in-place target is a different directory, so a record kept
// under either tree's `var/` is invisible to the other. Without that file the
// record goes to the release's `var/`, which is what a symlink target needs.
const fs = require('fs');
const path = require('path');

const root = path.resolve(__dirname, '../../../../../../..');
const named = (() => {
  try {
    return fs.readFileSync(path.join(root, 'stub-state-dir.txt'), 'utf8').trim();
  } catch {
    return '';
  }
})();

const marker = path.join(named === '' ? path.join(root, 'var') : named, 'frontend-built.txt');
fs.mkdirSync(path.dirname(marker), { recursive: true });
fs.appendFileSync(marker, process.cwd() + '\n');
