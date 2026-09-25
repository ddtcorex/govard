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
  // attribute main.js's document-wide delegate resolves (D5). The Logs panel
  // beside them is still vanilla markup and keeps its own data-action.
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
