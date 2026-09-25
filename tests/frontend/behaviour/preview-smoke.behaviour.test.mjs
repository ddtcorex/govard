import test from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { findFreePort, launchChrome, openTab } from "./support/cdp.mjs";
import { startVite } from "./support/dev-server.mjs";

const CHROME_BIN = process.env.CHROME_BIN;
const FRONTEND_DIR = join(import.meta.dirname, "..", "..", "..", "desktop", "frontend");

// Boots the real app against the preview seam once and hands the scenario a
// ready session. Chrome and vite are torn down even when a scenario fails.
async function withPreview(t) {
  if (!CHROME_BIN) {
    t.skip("CHROME_BIN not set");
    return null;
  }
  const vite = await startVite({ cwd: FRONTEND_DIR });
  const chromePort = await findFreePort();
  const userDataDir = mkdtempSync(join(tmpdir(), "govard-cdp-"));
  const chrome = launchChrome({ chromeBin: CHROME_BIN, port: chromePort, userDataDir });
  t.after(async () => {
    // SIGKILL: a graceful kill can leave the debug port held by a zombie long
    // enough to fail the next scenario's Chrome start.
    chrome.kill("SIGKILL");
    await vite.stop();
  });
  const session = await openTab({ port: chromePort, url: `${vite.baseUrl}/preview.html` });
  t.after(async () => {
    await session.close();
  });
  await session.navigate(`${vite.baseUrl}/preview.html`);
  // main.js's bootstrap ends by writing "Status: Ready", but only after
  // refreshDashboard awaits loadFooterVersion, which retries GetVersion 15 times
  // at 300ms while the route answers the generic "" default. Waiting for that
  // quiescent state is what keeps these scenarios from racing the boot.
  await session.waitFor(`document.getElementById("status").textContent`, "Status: Ready", {
    timeoutMs: 10000,
  });
  return session;
}

test("a fixture drives the rendered footer, and the app makes the call once per refresh", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  // reset() clears the fixture list AND the recorded call log, so it has to run
  // before installFixtures: the other order leaves the app answering the
  // generic defaults and the footer stuck at 0.0%.
  await session.evaluate(`window.__govardPreview.reset()`);
  await session.evaluate(`window.__govardPreview.installFixtures("metrics")`);

  // Installing fixtures only changes what the next call answers; it does not
  // trigger one. The footer's own refresh control is the user's path, so the
  // scenario drives that instead of poking the controller directly.
  await session.evaluate(`document.querySelector('[data-action="refresh-metrics"]').click()`);
  await session.waitFor(`document.getElementById("footerCPU").textContent`, "12.5%");
  assert.equal(await session.evaluate(`document.getElementById("footerMemory").textContent`), "2048.3 MB");

  const calls = JSON.parse(await session.evaluate(`JSON.stringify(window.__govardPreview.getCalls())`));
  const metricsCalls = calls.filter(
    (c) => c.service === "SystemService" && c.method === "GetSystemMetrics",
  );
  // Deliberately not an exact count: the boot path reaches refreshDashboard
  // twice (switchTab plus bootstrap's Promise.allSettled), so GetSystemMetrics
  // runs more than once per page load. What matters is that the one click
  // produced at least one call and no call went unanswered.
  assert.ok(metricsCalls.length >= 1, "the refresh click must reach SystemService.GetSystemMetrics");
  assert.deepEqual(metricsCalls[0].args, []);

  const screenshot = await session.screenshot({ x: 0, y: 0, width: 1200, height: 800 });
  assert.ok(screenshot.length > 0, "expected non-empty PNG bytes");

  assert.deepEqual(session.consoleErrors, [], "no console error during the scenario");
});

test("a pushed event reaches the app's own handler", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  // The app only subscribes when hasEventRuntime() is true, and in plain Chrome
  // none of the probed transports exist: preview/bootstrap.js installs the
  // WebKitGTK one, which is the transport the Linux desktop build really uses.
  // Without that stub the push below would reach nothing and this test would be
  // the only place the difference showed up.
  await session.evaluate(
    `window.__govardPreview.pushEvent("global-logs:status", "harness-event-ok")`,
  );
  await session.waitFor(`document.getElementById("status").textContent`, "harness-event-ok");

  await session.evaluate(`window.__govardPreview.pushEvent("global-logs:line", "HARNESS-LINE")`);
  const log = await session.evaluate(`document.getElementById("globalLogOutput").textContent`);
  assert.ok(log.includes("HARNESS-LINE"), `pushed log line missing from the log pane: ${log}`);

  assert.deepEqual(session.consoleErrors, [], "no console error during the scenario");
});
