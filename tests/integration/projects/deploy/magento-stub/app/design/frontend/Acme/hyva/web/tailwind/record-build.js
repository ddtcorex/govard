// Records where the frontend build ran, so the deploy's own verification can
// assert it happened inside the theme's Tailwind directory and not somewhere
// else in the release.
const fs = require('fs');
const path = require('path');

const marker = path.join(__dirname, '../../../../../../../var/frontend-built.txt');
fs.mkdirSync(path.dirname(marker), { recursive: true });
fs.appendFileSync(marker, process.cwd() + '\n');
