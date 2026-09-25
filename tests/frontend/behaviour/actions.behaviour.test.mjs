import test from "node:test";
import assert from "node:assert/strict";
import { withPreview } from "./support/preview-session.mjs";

// Pins today's actions dispatch path (hero button -> main.js delegate ->
// actions controller -> bridge) so the later dashboard migration, which moves
// these triggers into React, has a regression net.
test("the hero stop button stops the selected project once and reports it", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  // reset() clears the fixtures and the call log, so it runs first.
  await session.evaluate(`window.__govardPreview.reset()`);
  await session.evaluate(`window.__govardPreview.installFixtures("actions")`);

  // Installing fixtures does not trigger a call. The dashboard refresh control
  // re-reads GetDashboard so the sidebar lists the fixture's environment, and
  // selecting it from the sidebar is the user's path to the hero.
  await session.evaluate(`document.getElementById("refresh").click()`);
  const envButton = `document.querySelector('#envList [data-action="select-environment"][data-env="sample-project"]')`;
  await session.waitFor(`!!${envButton}`, true);
  await session.evaluate(`${envButton}.click()`);
  await session.waitFor(
    `(() => { const b = document.getElementById("heroStopBtn"); return b.dataset.env === "sample-project" && !b.disabled; })()`,
    true,
  );

  await session.evaluate(`document.getElementById("heroStopBtn").click()`);

  // The loading toast opens with its title and "Please wait..." and stays at
  // least MIN_LOADING_TOAST_MS (700 ms) before it turns into the result, so a
  // 50 ms poll sees the loading state before the success text replaces it.
  const toast = `[...document.querySelectorAll("#toastContainer .toast")].find((el) => el.querySelector(".toast-message")?.textContent.trim() === "Stopping sample-project...")`;
  await session.waitFor(
    `(() => { const el = ${toast}; return el ? el.querySelector(".toast-stream-line").textContent.trim() : null; })()`,
    "Please wait...",
    { intervalMs: 50 },
  );
  await session.waitFor(
    `(() => { const el = ${toast}; return el ? el.querySelector(".toast-stream-line").textContent.trim() : null; })()`,
    "Stopped sample-project",
  );
  assert.equal(
    await session.evaluate(`${toast}.classList.contains("toast--success")`),
    true,
    "the loading toast must close as a success",
  );

  const calls = JSON.parse(await session.evaluate(`JSON.stringify(window.__govardPreview.getCalls())`));
  const stopCalls = calls.filter(
    (c) => c.service === "EnvironmentService" && c.method === "StopEnvironment",
  );
  assert.equal(stopCalls.length, 1, "one click must produce exactly one StopEnvironment call");
  assert.deepEqual(stopCalls[0].args, ["sample-project"]);

  assert.deepEqual(session.consoleErrors, [], "no console error during the scenario");
});
