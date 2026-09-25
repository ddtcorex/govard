#!/usr/bin/env node
// @ts-check
// Captures the same four app views before and after a look-changing build, so
// the Tailwind v4 migration can be gated on a pixel diff instead of on "looks
// close enough". Dev-only tooling: nothing imports it, and it writes PNGs into a
// gitignored directory.
import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import {
  findFreePort,
  launchChrome,
  openTab,
} from "../../../tests/frontend/behaviour/support/cdp.mjs";
import { startVite } from "../../../tests/frontend/behaviour/support/dev-server.mjs";

const __dirname = dirname(fileURLToPath(import.meta.url));
const FRONTEND_DIR = join(__dirname, "..");
const outDir = process.argv[2];
if (!outDir) {
  console.error("usage: node capture-tab-screenshots.mjs <outDir>");
  process.exit(1);
}
mkdirSync(outDir, { recursive: true });

const CHROME_BIN = process.env.CHROME_BIN;
if (!CHROME_BIN) {
  console.error("CHROME_BIN must be set");
  process.exit(1);
}

const VIEWPORT = { x: 0, y: 0, width: 1440, height: 900 };

/**
 * Deterministic state is what makes the before/after diff meaningful, so this
 * deliberately does three things the obvious version does not:
 *
 * 1. No fixtures are installed. Installing them changes nothing by itself, but
 *    the app's 15s metrics auto-refresh would then flip the footer from 0.0% to
 *    12.5% at an arbitrary moment, so two runs could differ for a reason that has
 *    nothing to do with the build. The generic defaults keep every view stable.
 * 2. It waits for the boot to quiesce, for fonts to load, and for the tab to
 *    actually become active, instead of sleeping a fixed 300ms. The footer's
 *    version text only settles about 4.5s in, and a screenshot taken before that
 *    is a screenshot of a different state.
 * 3. It disables animations and transitions: a spinner or a transition captured
 *    half-way would differ between two runs of identical code.
 */
const TABS = [
  { id: "global-services", panel: "tab-global-services", selector: null },
  { id: "dashboard", panel: "tab-dashboard", selector: '[data-action="switch-tab"][data-tab="dashboard"]' },
  { id: "remotes", panel: "tab-remotes", selector: '[data-action="switch-tab"][data-tab="remotes"]' },
  { id: "logs", panel: "tab-logs", selector: '[data-action="switch-tab"][data-tab="logs"]' },
];

const vite = await startVite({ cwd: FRONTEND_DIR });
const port = await findFreePort();
const userDataDir = mkdtempSync(join(tmpdir(), "govard-shot-"));
const chrome = launchChrome({ chromeBin: CHROME_BIN, port, userDataDir });
try {
  const session = await openTab({ port, url: `${vite.baseUrl}/preview.html` });
  await session.navigate(`${vite.baseUrl}/preview.html`);
  await session.waitFor(`document.getElementById("status").textContent`, "Status: Ready", {
    timeoutMs: 15000,
  });
  await session.evaluate(`document.fonts.ready.then(() => true)`);
  await session.evaluate(`(() => {
    const style = document.createElement("style");
    style.textContent = "*, *::before, *::after { animation: none !important; transition: none !important; }";
    document.head.appendChild(style);
    return true;
  })()`);

  for (const tab of TABS) {
    if (tab.selector) {
      await session.evaluate(`document.querySelector('${tab.selector}').click()`);
      await session.waitFor(
        `document.getElementById("${tab.panel}").classList.contains("active")`,
        true,
      );
    }
    const png = await session.screenshot(VIEWPORT);
    writeFileSync(join(outDir, `${tab.id}.png`), png);
    console.log(`captured ${tab.id}`);
  }
  await session.close();
} finally {
  chrome.kill("SIGKILL");
  await vite.stop();
}
