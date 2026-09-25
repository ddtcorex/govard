import test from "node:test";
import assert from "node:assert/strict";
import { writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { withPreview } from "./support/preview-session.mjs";

const LOGS_TAB = '[data-action="switch-tab"][data-tab="logs"]';
const ISLAND = "#logsIsland";
const OUTPUT = "#logsIsland #logOutput";

// Page-side count of LogService.GetLogsForService calls in the preview log.
const CALL_COUNT_EXPR = `window.__govardPreview.getCalls().filter(
  (c) => c.service === "LogService" && c.method === "GetLogsForService",
).length`;

const callCount = (session) => session.evaluate(CALL_COUNT_EXPR);
const quiet = (session, ms) => session.evaluate(`new Promise((r) => setTimeout(r, ${ms}))`);

/**
 * Types into a React-controlled input the way a user does: the native value
 * setter plus a bubbling input event, because assigning `.value` alone never
 * reaches React's onChange.
 */
function typeInto(selector, text) {
  return `(() => {
    const el = document.querySelector('${selector}');
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value").set;
    setter.call(el, ${JSON.stringify(text)});
    el.dispatchEvent(new Event("input", { bubbles: true }));
    return el.value;
  })()`;
}

/**
 * Installs a fixture and opens the logs tab on a project.
 *
 * The dashboard refresh is what makes the fixture's first environment the
 * selected project; without it the logs island would legitimately render
 * "Select an environment to view logs." and never call the backend. Waiting on
 * the sidebar entry is the observable proof the refresh landed, so the tab
 * click cannot race it.
 */
async function openLogsWithFixture(session, fixture) {
  await session.evaluate(`window.__govardPreview.reset()`);
  await session.evaluate(`window.__govardPreview.installFixtures(${JSON.stringify(fixture)})`);
  await session.evaluate(`document.getElementById("refresh").click()`);
  await session.waitFor(
    `!!document.querySelector('#envList [data-testid="env-card"][data-env="sample-project"]')`,
    true,
    { timeoutMs: 10000 },
  );
  await session.evaluate(`document.querySelector('${LOGS_TAB}').click()`);
  await session.waitFor(`document.getElementById("tab-logs").classList.contains("active")`, true);
  // Prove the island is mounted before asserting anything about it: a count
  // assertion on an empty container passes vacuously.
  await session.waitFor(`document.querySelector('${OUTPUT}') !== null`, true, { timeoutMs: 10000 });
}

test("the logs island renders the fixture, owns its controls, and filters as the user types", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  // A desktop-sized viewport, so the capture below is the real tab and not a
  // corner of Chrome's 800x600 default with white page around it.
  const viewport = { width: 1440, height: 900 };
  await session.send("Emulation.setDeviceMetricsOverride", {
    ...viewport,
    deviceScaleFactor: 1,
    mobile: false,
  });

  await openLogsWithFixture(session, "logs");
  await session.waitFor(`document.getElementById("logOutput").textContent.includes("fixture line one")`, true);

  // The island owns its whole subtree now, so nothing in it may carry the
  // attribute main.js's document-wide delegate resolves (D5).
  assert.equal(
    await session.evaluate(`document.querySelectorAll('${ISLAND} [data-action]').length`),
    0,
    "a migrated subtree must carry no data-action for main.js's delegate",
  );

  // The service strip is built from the fixture environment's services, not
  // from the markup the vanilla tab used to inject.
  assert.equal(
    await session.evaluate(`document.querySelectorAll('#logServiceSelector [data-testid^="service-"]').length`),
    3,
    "all plus the environment's web and php targets",
  );

  // One user action, one call: the click must not be handled twice.
  await quiet(session, 600);
  const beforeRefresh = await callCount(session);
  await session.evaluate(`document.querySelector('[data-testid="refresh-logs"]').click()`);
  await session.waitFor(`${CALL_COUNT_EXPR} > ${beforeRefresh}`, true);
  assert.equal(
    await callCount(session) - beforeRefresh,
    1,
    "one click must produce exactly one GetLogsForService call",
  );

  // Review Focus 1: what the user typed survives the state change the click
  // causes, and it still drives the filter.
  assert.equal(await session.evaluate(typeInto("#logSearch", "line two")), "line two");
  await session.waitFor(`document.getElementById("logOutput").textContent`, "fixture line two");
  const beforeTypedRefresh = await callCount(session);
  await session.evaluate(`document.querySelector('[data-testid="refresh-logs"]').click()`);
  await session.waitFor(`${CALL_COUNT_EXPR} > ${beforeTypedRefresh}`, true);
  assert.equal(await session.evaluate(`document.getElementById("logSearch").value`), "line two");
  await session.waitFor(`document.getElementById("logOutput").textContent`, "fixture line two");

  // A severity chip filters the same buffer without touching the backend.
  const beforeSeverity = await callCount(session);
  await session.evaluate(`document.querySelector('[data-testid="severity-error"]').click()`);
  await session.waitFor(
    `document.getElementById("logOutput").textContent`,
    "No logs match the current filters.",
  );
  assert.equal(await callCount(session), beforeSeverity, "filtering is not a backend call");
  await session.evaluate(`document.querySelector('[data-testid="severity-all"]').click()`);

  // Selecting a service reloads that target, which is what the service strip is
  // for: the call must carry the service the user asked for.
  const beforeService = await callCount(session);
  await session.evaluate(`document.querySelector('[data-testid="service-php"]').click()`);
  await session.waitFor(`${CALL_COUNT_EXPR} > ${beforeService}`, true);
  const serviceCalls = await session.evaluate(`JSON.stringify(window.__govardPreview.getCalls()
    .filter((c) => c.service === "LogService" && c.method === "GetLogsForService")
    .slice(${beforeService}))`);
  assert.equal(JSON.parse(serviceCalls).length, 1, "one chip click, one call");
  assert.deepEqual(
    JSON.parse(serviceCalls)[0].args.slice(0, 2),
    ["sample-project", "php"],
    "the call targets the chip's service",
  );

  // The spec's Testing leg 3: one capture per scenario, read back by the agent
  // so the module is verified by looking at it, not only by DOM text. Taken
  // with the filters cleared and before the event pushes, so the toast the
  // status event raises does not cover the tab being verified.
  const shot = await session.screenshot({ x: 0, y: 0, ...viewport });
  assert.ok(shot.length > 0, "the scenario captures the rendered tab");
  writeFileSync(join(tmpdir(), "logs-island.png"), shot);

  // A pushed event reaches the island through services/events.js. The filter
  // box still holds the text typed above, so clear it first: the assertion is
  // about the push, not about the filter.
  await session.evaluate(`document.querySelector('[data-testid="severity-all"]').click()`);
  assert.equal(await session.evaluate(typeInto("#logSearch", "")), "");
  await session.waitFor(`document.getElementById("logOutput").textContent.includes("fixture line one")`, true);
  await session.evaluate(`window.__govardPreview.pushEvent("logs:line", "pushed line")`);
  await session.waitFor(`document.getElementById("logOutput").textContent.includes("pushed line")`, true);
  await session.evaluate(`window.__govardPreview.pushEvent("logs:status", "harness-logs-status")`);
  await session.waitFor(`document.getElementById("status").textContent`, "harness-logs-status");

  // Unmounting ends the island's own event subscriptions: a push afterwards
  // must reach nothing.
  await session.evaluate(`window.__govardLogsIsland.unmount()`);
  await session.evaluate(`window.__govardPreview.pushEvent("logs:status", "after-unmount")`);
  await quiet(session, 400);
  assert.equal(
    await session.evaluate(`document.getElementById("status").textContent`),
    "harness-logs-status",
    "the logs:status subscription outlived the island",
  );

  assert.deepEqual(session.consoleErrors, []);
});

test("the live poll stops when the logs island unmounts", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  // This fixture makes StartLogStreamForService fail, so the island takes the
  // polling fallback - the branch that owns a real interval and is therefore
  // the one whose cleanup can leak.
  await openLogsWithFixture(session, "logs-poll");
  await session.waitFor(`document.getElementById("logOutput").textContent.includes("poll line one")`, true);

  const beforeLive = await callCount(session);
  await session.evaluate(`document.querySelector('[data-testid="toggle-live"]').click()`);
  await session.waitFor(`document.getElementById("toggleLive").textContent`, "Live: On");
  // Two polls after the immediate one prove the interval is really running.
  await session.waitFor(`${CALL_COUNT_EXPR} >= ${beforeLive + 3}`, true, { timeoutMs: 8000 });

  await session.evaluate(`window.__govardLogsIsland.unmount()`);
  // Let an in-flight poll land before the count is taken.
  await quiet(session, 1500);
  const settled = await callCount(session);
  await quiet(session, 3000);
  assert.equal(
    await callCount(session),
    settled,
    "the poll kept calling the backend after unmount",
  );

  assert.deepEqual(session.consoleErrors, []);
});
