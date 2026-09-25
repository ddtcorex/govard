import test from "node:test";
import assert from "node:assert/strict";
import { withPreview } from "./support/preview-session.mjs";

// Page-side count of SystemService.GetSystemMetrics calls in the preview log.
const COUNT_EXPR = `window.__govardPreview.getCalls().filter(
  (c) => c.service === "SystemService" && c.method === "GetSystemMetrics",
).length`;

const metricsCalls = (session) => session.evaluate(COUNT_EXPR);

// Resolves right after the island's poll fires once, page-side, so the caller
// starts a measurement at the beginning of a full polling period. The preview
// polls every second (preview/bootstrap.js), far longer than a click plus its
// render takes, so a delta measured from here cannot include a stray tick.
const AFTER_NEXT_POLL_EXPR = `new Promise((resolve, reject) => {
  const start = ${COUNT_EXPR};
  const deadline = Date.now() + 3000;
  const check = () => {
    if (${COUNT_EXPR} > start) return resolve(true);
    if (Date.now() > deadline) return reject(new Error("the metrics island never polled"));
    setTimeout(check, 5);
  };
  check();
})`;

test("metrics island renders the fixture and refreshes once per click", async (t) => {
  const session = await withPreview(t);
  if (!session) return;
  await session.evaluate(`window.__govardPreview.reset()`);
  await session.evaluate(`window.__govardPreview.installFixtures("metrics")`);

  // The island must actually be mounted, or the data-action count below would
  // pass vacuously on an empty container.
  assert.equal(
    await session.evaluate(`document.querySelector("#metricsIsland #footerCPU") !== null`),
    true,
    "the footer readout renders inside #metricsIsland",
  );
  assert.equal(
    await session.evaluate(`document.querySelectorAll("#metricsIsland [data-action]").length`),
    0,
    "D5: the island subtree carries no data-action",
  );

  await session.evaluate(AFTER_NEXT_POLL_EXPR);
  const before = await metricsCalls(session);
  await session.evaluate(`document.querySelector('[data-testid="refresh-metrics"]').click()`);
  await session.waitFor(`document.getElementById("status").textContent.includes("system metrics updated")`, true);
  const after = await metricsCalls(session);
  assert.equal(after, before + 1, "exactly one GetSystemMetrics per click");
  assert.equal(await session.evaluate(`document.getElementById("footerCPU").textContent`), "12.5%");
  assert.equal(await session.evaluate(`document.getElementById("footerMemory").textContent`), "2048.3 MB");

  assert.deepEqual(session.consoleErrors, []);
});

test("metrics island stops polling after unmount", async (t) => {
  const session = await withPreview(t);
  if (!session) return;
  await session.evaluate(`window.__govardPreview.reset()`);
  await session.evaluate(`window.__govardPreview.installFixtures("metrics")`);

  // Prove the interval is live first; otherwise "no call after unmount" would
  // also hold for an island that never polled at all. The silent poll renders
  // the fixture without a click.
  await session.evaluate(AFTER_NEXT_POLL_EXPR);
  await session.waitFor(`document.getElementById("footerCPU").textContent`, "12.5%");

  await session.evaluate(`window.__govardMetricsIsland.unmount()`);
  const after = await metricsCalls(session);
  // Two and a half polling periods.
  await session.evaluate(`new Promise((r) => setTimeout(r, 2500))`);
  assert.equal(await metricsCalls(session), after, "no GetSystemMetrics after unmount");
  assert.equal(await session.evaluate(`document.getElementById("metricsIsland").childElementCount`), 0);

  assert.deepEqual(session.consoleErrors, []);
});
