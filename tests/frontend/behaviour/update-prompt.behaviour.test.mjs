import test from "node:test";
import assert from "node:assert/strict";
import { withPreview } from "./support/preview-session.mjs";

const callCount = (session, method) =>
  session.evaluate(`window.__govardPreview.getCalls().filter(
    (c) => c.service === "UpdateService" && c.method === ${JSON.stringify(method)},
  ).length`);

const promptVisible = `(() => {
  const el = document.getElementById("updatePrompt");
  return !!el && !el.classList.contains("hidden") && el.getAttribute("aria-hidden") === "false";
})()`;

// main.js schedules the production background check 12 s after boot. The
// preview exposes the model (preview/bootstrap.js), so a scenario stops that
// schedule and runs each check itself: nothing but the scenario decides when a
// check happens, and a check against the generic "not outdated" default cannot
// land between two assertions.
const prepare = async (session) => {
  await session.evaluate(`window.__govardUpdatePromptModel.clearTimers()`);
  await session.evaluate(`window.__govardPreview.reset()`);
  await session.evaluate(`window.__govardPreview.installFixtures("update-prompt")`);
};

const runCheck = (session) =>
  session.evaluate(`window.__govardUpdatePromptModel.checkForUpdatesInBackground().then(() => true)`);

test("update prompt shows the fixture, hides behind settings and stays dismissed", async (t) => {
  const session = await withPreview(t);
  if (!session) return;
  await prepare(session);

  assert.equal(await session.evaluate(promptVisible), false, "hidden before any check");
  await runCheck(session);
  await session.waitFor(promptVisible, true);
  assert.equal(await callCount(session, "CheckForUpdates"), 1, "one bridge call per check");

  assert.equal(await session.evaluate(`document.getElementById("updatePromptCurrent").textContent`), "1.0.0");
  assert.equal(await session.evaluate(`document.getElementById("updatePromptLatest").textContent`), "1.1.0");
  assert.equal(
    await session.evaluate(`document.getElementById("updatePromptMessage").textContent`),
    "A new Govard Desktop version is ready to install.",
  );
  assert.equal(
    await session.evaluate(`document.getElementById("updatePromptChangelog").textContent`),
    "- Faster project switching in the sidebar.",
  );
  assert.equal(
    await session.evaluate(`document.getElementById("updatePromptChangelog").classList.contains("hidden")`),
    false,
  );
  assert.equal(
    await session.evaluate(`document.getElementById("installUpdatePromptButton").textContent`),
    "downloadDownload & Install",
  );
  assert.equal(
    await session.evaluate(`document.querySelectorAll("#updatePromptIsland [data-action]").length`),
    0,
    "D5: the island subtree carries no data-action",
  );

  // Review focus 3: hidden while the settings drawer is open, back when it closes.
  await session.evaluate(`document.getElementById("openSettings").click()`);
  await session.waitFor(`document.getElementById("settingsDrawer").classList.contains("hidden")`, false);
  await session.waitFor(promptVisible, false);
  await session.evaluate(`document.getElementById("closeSettings").click()`);
  await session.waitFor(`document.getElementById("settingsDrawer").classList.contains("hidden")`, true);
  await session.waitFor(promptVisible, true);

  const laterButton = `[...document.querySelectorAll("#updatePrompt button")].find((b) => b.textContent.trim() === "Later")`;
  await session.evaluate(`${laterButton}.click()`);
  await session.waitFor(promptVisible, false);

  // The same version never comes back: not from a new check, not from the drawer.
  await runCheck(session);
  assert.equal(await callCount(session, "CheckForUpdates"), 2);
  await session.evaluate(`document.getElementById("openSettings").click()`);
  await session.evaluate(`document.getElementById("closeSettings").click()`);
  await session.evaluate(`new Promise((r) => setTimeout(r, 200))`);
  assert.equal(await session.evaluate(promptVisible), false, "a dismissed version stays hidden");

  assert.deepEqual(session.consoleErrors, []);
});

test("update prompt installs once per click and hides on success", async (t) => {
  const session = await withPreview(t);
  if (!session) return;
  await prepare(session);

  await runCheck(session);
  await session.waitFor(promptVisible, true);

  assert.equal(await callCount(session, "InstallLatestUpdate"), 0);
  await session.evaluate(`document.getElementById("installUpdatePromptButton").click()`);
  await session.waitFor(promptVisible, false);
  assert.equal(await callCount(session, "InstallLatestUpdate"), 1, "exactly one install call per click");
  assert.equal(
    await session.evaluate(`document.getElementById("installUpdatePromptButton").disabled`),
    false,
    "the button is re-enabled once the install settles",
  );

  assert.deepEqual(session.consoleErrors, []);
});
