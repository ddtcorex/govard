import test from "node:test";
import assert from "node:assert/strict";
import { writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { withPreview } from "./support/preview-session.mjs";

const DASHBOARD_TAB = '[data-action="switch-tab"][data-tab="dashboard"]';
const ENV_CARD = "#envList [data-testid='env-card']";

const callCount = (session, method) =>
  session.evaluate(`window.__govardPreview.getCalls().filter(
    (c) => c.method === ${JSON.stringify(method)},
  ).length`);

const quiet = (session, ms) => session.evaluate(`new Promise((r) => setTimeout(r, ${ms}))`);

/**
 * Installs the fixture and refreshes the dashboard.
 *
 * The list renders from the store, and the store is filled by the dashboard
 * fetch, so the fixture needs one refresh - which is the app's own control, the
 * same one the actions scenario uses.
 */
async function loadDashboardFixture(session) {
  await session.evaluate(`window.__govardPreview.reset()`);
  await session.evaluate(`window.__govardPreview.installFixtures("dashboard")`);
  await session.evaluate(`document.getElementById("refresh").click()`);
  await session.waitFor(`document.querySelector("${ENV_CARD}") !== null`, true, {
    timeoutMs: 10000,
  });
}

test("the sidebar list, the hero and the dashboard cards render from the fixture", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  const viewport = { width: 1440, height: 900 };
  await session.send("Emulation.setDeviceMetricsOverride", {
    ...viewport,
    deviceScaleFactor: 1,
    mobile: false,
  });

  await loadDashboardFixture(session);

  // The island owns the list now, so nothing in it may carry the attribute
  // main.js's document-wide delegate resolves (D5).
  assert.equal(
    await session.evaluate(`document.querySelectorAll('#envList [data-action]').length`),
    0,
    "a migrated subtree must carry no data-action for main.js's delegate",
  );
  // Both environments reached the list, split by status.
  assert.equal(await session.evaluate(`document.querySelectorAll("#envList [data-testid='env-card']").length`), 2);
  assert.equal(
    await session.evaluate(`document.querySelector("#envList [data-testid='env-card'][data-env='second-project']").textContent.includes("Stopped")`),
    true,
    "the stopped environment keeps its status line",
  );

  // Selecting an environment still reaches the backend the way it did.
  const before = JSON.parse(await session.evaluate(`JSON.stringify(window.__govardPreview.getCalls())`)).length;
  await session.evaluate(`document.querySelector("#envList [data-testid='env-card']").click()`);
  await quiet(session, 800);
  const after = JSON.parse(await session.evaluate(`JSON.stringify(window.__govardPreview.getCalls())`)).length;
  assert.ok(after > before, "selecting an environment must still reach the backend");

  // The dashboard tab is rendered at boot and hidden by the global-services
  // switch, so prove the cards are mounted before asserting their contents.
  await session.evaluate(`document.querySelector('${DASHBOARD_TAB}').click()`);
  await session.waitFor(`document.getElementById("tab-dashboard").classList.contains("active")`, true);
  await session.waitFor(`document.querySelector("#activeServicesList [data-testid='service-card']") !== null`, true);
  // Only the two migrated subtrees are asserted: #tab-dashboard also holds the
  // Quick Actions grid, which belongs to another module and still routes
  // through the delegate, so a tab-wide count cannot be zero yet.
  for (const container of ["#activeServicesList", "#envVarsList"]) {
    assert.equal(
      await session.evaluate(`document.querySelectorAll('${container} [data-action]').length`),
      0,
      `${container} must carry no data-action for main.js's delegate`,
    );
  }
  assert.equal(
    await session.evaluate(`document.querySelectorAll("#activeServicesList [data-testid='service-card']").length`),
    2,
    "both services of the running environment render",
  );

  // The hero reads the same store: title, badge tone and the restart glyph.
  await session.waitFor(`document.getElementById("projectTitle").textContent`, "sample-project.test");
  assert.equal(await session.evaluate(`document.getElementById("projectStatusText").textContent`), "Running");
  assert.equal(
    await session.evaluate(`document.getElementById("projectStatusBadge").classList.contains("hidden")`),
    false,
    "a selected environment shows its status badge",
  );
  assert.equal(
    await session.evaluate(`document.getElementById("heroRestartBtn").textContent.trim()`),
    "restart_altRestart",
  );
  assert.equal(await session.evaluate(`document.getElementById("heroStopBtn").disabled`), false);
  assert.equal(await session.evaluate(`document.getElementById("projectGitBranchText").textContent`), "main");
  assert.equal(await session.evaluate(`document.querySelectorAll("#projectTechnologies span").length > 0`), true);
  assert.equal(
    await session.evaluate(`document.querySelectorAll("#envVarsList [data-testid='env-var-row']").length`),
    2,
    "the environment variables of the selected project render",
  );

  // Switching to the dashboard tab triggers a non-silent refresh, which shows
  // the sidebar's loading frame; wait for the list to come back so the capture
  // is the settled tab and not a skeleton mid-sync.
  await session.waitFor(`document.querySelector("${ENV_CARD}") !== null`, true, {
    timeoutMs: 10000,
  });

  const shot = await session.screenshot({ x: 0, y: 0, ...viewport });
  assert.ok(shot.length > 0, "the scenario captures the rendered dashboard");
  writeFileSync(join(tmpdir(), "dashboard-island.png"), shot);

  assert.deepEqual(session.consoleErrors, []);
});

test("a service card's logs button opens that service's logs", async (t) => {
  const session = await withPreview(t);
  if (!session) return;
  await loadDashboardFixture(session);

  const before = await callCount(session, "GetLogsForService");
  // The second card is php (nginx first), whose inferred target is "php".
  await session.evaluate(`document.querySelectorAll("#activeServicesList [data-testid='service-logs']")[1].click()`);
  await session.waitFor(`document.getElementById("tab-logs").classList.contains("active")`, true);
  await session.waitFor(
    `window.__govardPreview.getCalls().some((c) => c.method === "GetLogsForService")`,
    true,
  );
  const args = JSON.parse(
    await session.evaluate(`JSON.stringify(window.__govardPreview.getCalls()
      .filter((c) => c.method === "GetLogsForService")
      .map((c) => c.args))`),
  );
  assert.ok(args.length > before, "the card must load the service's logs");
  assert.deepEqual(args.at(-1).slice(0, 2), ["sample-project", "php"]);

  assert.deepEqual(session.consoleErrors, []);
});
