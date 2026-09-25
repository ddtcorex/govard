// @ts-check
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { findFreePort, launchChrome, openTab } from "./cdp.mjs";
import { startVite } from "./dev-server.mjs";

export const FRONTEND_DIR = join(
  import.meta.dirname,
  "..",
  "..",
  "..",
  "..",
  "desktop",
  "frontend",
);

/**
 * Boots the real app against the preview seam once and hands the scenario a
 * ready session: throwaway dev server on an OS-allocated port, headless Chrome
 * on another, and Chrome and vite torn down even when the scenario fails.
 *
 * Every behaviour file shares this so the boot contract lives in one place. The
 * quiescence wait is part of it: main.js's bootstrap writes "Status: Ready" only
 * after refreshDashboard awaits loadFooterVersion, which retries GetVersion 15
 * times at 300ms while the route answers the generic "" default, so a scenario
 * that starts earlier races the boot.
 *
 * @param {import("node:test").TestContext} t
 * @returns {Promise<import("./cdp.mjs").Session | null>} null when skipped
 */
export async function withPreview(t) {
  const chromeBin = process.env.CHROME_BIN;
  if (!chromeBin) {
    t.skip("CHROME_BIN not set");
    return null;
  }
  const vite = await startVite({ cwd: FRONTEND_DIR });
  const chromePort = await findFreePort();
  const userDataDir = mkdtempSync(join(tmpdir(), "govard-cdp-"));
  const chrome = launchChrome({ chromeBin, port: chromePort, userDataDir });
  t.after(async () => {
    // SIGKILL: a graceful kill can leave the debug port held by a zombie long
    // enough to fail the next scenario's Chrome start.
    chrome.kill("SIGKILL");
    await vite.stop();
  });
  const session = await openTab({ port: chromePort, url: `${vite.baseUrl}/preview.html`, chrome });
  t.after(async () => {
    await session.close();
  });
  await session.navigate(`${vite.baseUrl}/preview.html`);
  await session.waitFor(`document.getElementById("status").textContent`, "Status: Ready", {
    timeoutMs: 10000,
  });
  return session;
}
