import test from "node:test";
import assert from "node:assert/strict";
import { writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { withPreview } from "./support/preview-session.mjs";

const VIEWPORT = { width: 1440, height: 900 };
const CARD = "#globalServicesList [data-testid='global-service-card']";
const DECK = "#globalHealthIsland";
const LIST = "#globalServicesList";

const text = (session, selector) =>
  session.evaluate(`document.querySelector(${JSON.stringify(selector)}).textContent.trim()`);

const calls = async (session) =>
  JSON.parse(await session.evaluate(`JSON.stringify(window.__govardPreview.getCalls())`));

/**
 * Installs the fixture and refreshes.
 *
 * The global-services tab is the one the boot path leaves visible, so no tab
 * click is needed; but the fixture is installed after boot, so the app's own
 * refresh control is what publishes it to the store the islands read.
 */
async function loadGlobalServices(session) {
  await session.evaluate(`window.__govardPreview.reset()`);
  await session.evaluate(`window.__govardPreview.installFixtures("global-services")`);
  await session.evaluate(`document.getElementById("refresh").click()`);
  await session.waitFor(`document.querySelector("${CARD}") !== null`, true, {
    timeoutMs: 10000,
  });
}

test("the ops deck and the card list render the fixture through the islands", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.send("Emulation.setDeviceMetricsOverride", {
    ...VIEWPORT,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await loadGlobalServices(session);
  await session.waitFor(`document.getElementById("globalServiceHealthPercent").textContent`, "67%");

  // Both migrated containers are React's now, so nothing in either may carry the
  // attribute main.js's document-wide delegate resolves (D5).
  for (const container of [DECK, LIST]) {
    assert.equal(
      await session.evaluate(`document.querySelectorAll('${container} [data-action]').length`),
      0,
      `${container} must carry no data-action for main.js's delegate`,
    );
  }

  // The health KPI is derived from the snapshot: 2 of 3 services are active.
  assert.equal(await text(session, "#globalServiceCount"), "2/3 running");
  assert.equal(await text(session, "#globalServiceHealthLabelText"), "1 service need attention");
  assert.equal(
    await session.evaluate(`document.getElementById("globalServiceHealthBar").style.width`),
    "67%",
  );
  assert.equal(
    await session.evaluate(
      `document.getElementById("globalServiceHealthBar").className.includes("from-amber-500")`,
    ),
    true,
    "a partially healthy mesh keeps the amber bar",
  );

  // The mesh strip lists every service with its own tone.
  assert.equal(
    await session.evaluate(`document.querySelectorAll("#globalServiceStatusStrip .global-status-chip").length`),
    3,
  );
  assert.equal(
    await session.evaluate(
      `document.querySelector("#globalServiceStatusStrip .global-status-chip").textContent.includes("Caddy Proxy")`,
    ),
    true,
    "the strip names the service",
  );
  assert.equal(
    await session.evaluate(
      `document.querySelector("#globalServiceStatusStrip .global-status-chip").textContent.includes("Running")`,
    ),
    true,
    "the strip carries the status label beside the name",
  );

  // The cards follow the snapshot: the stopped service can be started, the
  // running one restarted, and a service that cannot be opened says so.
  assert.equal(await session.evaluate(`document.querySelectorAll("${CARD}").length`), 3);
  assert.equal(
    await session.evaluate(
      `document.querySelector("${CARD}[data-service='portainer'] [data-testid='global-service-primary']").dataset.operation`,
    ),
    "start",
  );
  assert.equal(
    await session.evaluate(
      `document.querySelector("${CARD}[data-service='portainer'] [data-testid='global-service-stop']").disabled`,
    ),
    true,
    "a stopped service cannot be stopped again",
  );
  assert.equal(
    await session.evaluate(
      `document.querySelector("${CARD}[data-service='caddy'] [data-testid='global-service-primary']").dataset.operation`,
    ),
    "restart",
  );
  assert.equal(
    await session.evaluate(
      `document.querySelector("${CARD}[data-service='dnsmasq'] [data-testid='global-service-open']").disabled`,
    ),
    true,
    "a service that declares itself unopenable keeps the disabled open button",
  );

  // The bulk buttons are derived too: two services run, so Restart/Stop are on.
  for (const id of ["globalBulkStart", "globalBulkRestart", "globalBulkStop", "globalBulkPull"]) {
    assert.equal(
      await session.evaluate(`document.getElementById(${JSON.stringify(id)}).disabled`),
      false,
      `${id} should be enabled for a partially running mesh`,
    );
  }

  assert.equal(
    await text(session, "#globalActionFeedbackText"),
    "1 service are offline. Start All can recover quickly.",
  );

  const shot = await session.screenshot({ x: 0, y: 0, ...VIEWPORT });
  assert.ok(shot.length > 0, "the scenario captures the rendered ops deck");
  writeFileSync(join(tmpdir(), "global-services-island.png"), shot);

  assert.deepEqual(session.consoleErrors, []);
});

test("a bulk action and a card action each reach the bridge exactly once", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.send("Emulation.setDeviceMetricsOverride", {
    ...VIEWPORT,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await loadGlobalServices(session);

  // Pull All is never gated, so it is the one bulk action a fixture can drive
  // without the routing-settle loop.
  const before = (await calls(session)).length;
  await session.evaluate(`document.getElementById("globalBulkPull").click()`);
  await session.waitFor(
    `window.__govardPreview.getCalls().some((c) => c.method === "PullGlobalServices")`,
    true,
  );
  const afterPull = await calls(session);
  assert.equal(
    afterPull.slice(before).filter((c) => c.method === "PullGlobalServices").length,
    1,
    "one click must reach the bridge exactly once",
  );
  // The action's own message lands in the feedback strip, summarised to one line.
  await session.waitFor(`document.getElementById("globalActionFeedbackText").textContent`, "Global services restarted.");

  // A card's primary button: the stopped service starts, once, with its id.
  await session.evaluate(
    `document.querySelector("${CARD}[data-service='portainer'] [data-testid='global-service-primary']").click()`,
  );
  await session.waitFor(
    `window.__govardPreview.getCalls().some((c) => c.method === "StartGlobalService")`,
    true,
  );
  const startCalls = (await calls(session)).filter((c) => c.method === "StartGlobalService");
  assert.equal(startCalls.length, 1, "the card button must not double-fire");
  assert.deepEqual(startCalls[0].args, ["portainer"]);

  // Selecting a card still drives the Logs panel beside it, which is vanilla
  // markup in this change: the store write plus main.js's own call is what keeps
  // the two halves in step while only one of them is React.
  await session.evaluate(`document.querySelector("${CARD}[data-service='portainer']").click()`);
  await session.waitFor(`document.getElementById("globalLogServiceName").textContent`, "Portainer");
  assert.equal(
    await session.evaluate(
      `document.querySelector("${CARD}[data-service='portainer']").className.includes("border-primary/40")`,
    ),
    true,
    "the selected card keeps its selected ring",
  );

  assert.deepEqual(session.consoleErrors, []);
});

test("the log pane renders, filters and opens one stream, and stops on unmount", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.send("Emulation.setDeviceMetricsOverride", {
    ...VIEWPORT,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await loadGlobalServices(session);

  // The whole tab is React's after this change, not just the two deck containers.
  assert.equal(
    await session.evaluate(`document.querySelectorAll('#tab-global-services [data-action]').length`),
    0,
    "the migrated tab must carry no data-action for main.js's delegate",
  );

  // The pane's own Refresh loads the selected service's logs.
  await session.evaluate(`document.querySelector("[data-testid='refresh-global-logs']").click()`);
  await session.waitFor(
    `document.getElementById("globalLogOutput").textContent.includes("serving sample-project.test")`,
    true,
  );

  // The severity strip is derived from the store, so Error hides the one line
  // the fixture returns and All brings it back.
  await session.evaluate(`document.querySelector("[data-testid='global-severity-error']").click()`);
  await session.waitFor(
    `document.getElementById("globalLogOutput").textContent`,
    "No logs match the current filters.",
  );
  await session.evaluate(`document.querySelector("[data-testid='global-severity-all']").click()`);
  await session.waitFor(
    `document.getElementById("globalLogOutput").textContent.includes("serving sample-project.test")`,
    true,
  );

  // Live: the pane prefers the backend stream, so one click opens exactly one.
  const before = (await calls(session)).length;
  await session.evaluate(`document.querySelector("[data-testid='global-toggle-live']").click()`);
  await session.waitFor(`document.getElementById("globalToggleLive").textContent`, "Live: On");
  await session.waitFor(
    `window.__govardPreview.getCalls().some((c) => c.method === "StartGlobalServiceLogStream")`,
    true,
  );
  const after = await calls(session);
  assert.equal(
    after.slice(before).filter((c) => c.method === "StartGlobalServiceLogStream").length,
    1,
    "one click must open exactly one stream",
  );

  const shot = await session.screenshot({ x: 0, y: 0, ...VIEWPORT });
  assert.ok(shot.length > 0, "the scenario captures the rendered log pane");
  writeFileSync(join(tmpdir(), "global-logs-island.png"), shot);

  // Unmounting the island is what stops the pane: neither live path outlives it,
  // where the vanilla controller's interval and subscriptions had no owner.
  const logCalls = async () =>
    (await calls(session)).filter(
      (c) => c.method === "GetGlobalServiceLogs" || c.method.includes("LogStream"),
    ).length;
  await session.evaluate(`window.__govardGlobalLogsIsland.unmount()`);
  const settled = await logCalls();
  await new Promise((resolve) => setTimeout(resolve, 2500));
  assert.equal(await logCalls(), settled, "the log pane kept calling after unmount");

  assert.deepEqual(session.consoleErrors, []);
});

test("a live pane whose stream is unavailable falls back to the poll, and unmounting stops it", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.send("Emulation.setDeviceMetricsOverride", {
    ...VIEWPORT,
    deviceScaleFactor: 1,
    mobile: false,
  });
  // This bundle answers the stream route with an error, which is the one way a
  // scenario can reach the polling fallback: the preview runs with an event
  // runtime, so the stream path is otherwise always available.
  await session.evaluate(`window.__govardPreview.reset()`);
  await session.evaluate(`window.__govardPreview.installFixtures("global-logs-poll")`);
  await session.evaluate(`document.getElementById("refresh").click()`);
  await session.waitFor(`document.querySelector("${CARD}") !== null`, true, {
    timeoutMs: 10000,
  });

  await session.evaluate(`document.querySelector("[data-testid='global-toggle-live']").click()`);
  await session.waitFor(`document.getElementById("globalToggleLive").textContent`, "Live: On");
  const beforePoll = (await calls(session)).filter(
    (c) => c.method === "GetGlobalServiceLogs",
  ).length;
  // Two polls after the immediate load prove the interval is really running.
  await session.waitFor(
    `window.__govardPreview.getCalls().filter((c) => c.method === "GetGlobalServiceLogs").length >= ${beforePoll + 3}`,
    true,
    { timeoutMs: 8000 },
  );

  // Let an in-flight poll land before the count is taken. Only the log route is
  // counted: the footer's version and metrics loops run whatever this pane does.
  await new Promise((resolve) => setTimeout(resolve, 1200));
  await session.evaluate(`window.__govardGlobalLogsIsland.unmount()`);
  const settled = (await calls(session)).filter(
    (c) => c.method === "GetGlobalServiceLogs",
  ).length;
  await new Promise((resolve) => setTimeout(resolve, 3000));
  assert.equal(
    (await calls(session)).filter((c) => c.method === "GetGlobalServiceLogs").length,
    settled,
    "the global logs poll survived unmount",
  );

  assert.deepEqual(session.consoleErrors, []);
});

test("an unmounted island's registered API cannot still reach the bridge", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.send("Emulation.setDeviceMetricsOverride", {
    ...VIEWPORT,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await loadGlobalServices(session);

  // The log pane registers an API that main.js keeps calling - two refreshes and
  // the stop-on-leave. Its own timers die with the island, but the closures stay
  // reachable through main.js unless registering can be undone.
  await session.evaluate(`window.__govardGlobalLogsIsland.unmount()`);
  const before = (await calls(session)).filter(
    (c) => c.method === "GetGlobalServiceLogs",
  ).length;

  // The sidebar's own row takes the path that calls that API.
  await session.evaluate(`document.querySelector("[data-testid='global-services-row']").click()`);
  await new Promise((resolve) => setTimeout(resolve, 1500));

  assert.equal(
    (await calls(session)).filter((c) => c.method === "GetGlobalServiceLogs").length,
    before,
    "an unmounted pane's API must not still call the bridge",
  );
  assert.deepEqual(session.consoleErrors, []);
});
