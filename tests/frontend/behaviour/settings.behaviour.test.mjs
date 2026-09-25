import test from "node:test";
import assert from "node:assert/strict";
import { writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { withPreview } from "./support/preview-session.mjs";

const DRAWER = "#settingsDrawer";
const GEAR = "#openSettings";
const CLOSE = "#closeSettings";

const callCount = (session, method) =>
  session.evaluate(`window.__govardPreview.getCalls().filter(
    (c) => c.method === ${JSON.stringify(method)},
  ).length`);

const callArgs = (session, method) =>
  session.evaluate(`JSON.stringify(window.__govardPreview.getCalls()
    .filter((c) => c.method === ${JSON.stringify(method)})
    .map((c) => c.args))`);

const quiet = (session, ms) => session.evaluate(`new Promise((r) => setTimeout(r, ${ms}))`);

/** Sets a React-controlled <select> the way a user does: native setter + change event. */
const selectValue = (selector, value) => `(() => {
  const el = document.querySelector('${selector}');
  const setter = Object.getOwnPropertyDescriptor(window.HTMLSelectElement.prototype, "value").set;
  setter.call(el, ${JSON.stringify(value)});
  el.dispatchEvent(new Event("change", { bubbles: true }));
  return el.value;
})()`;

/**
 * Installs the fixture, stops the production background update schedule, and
 * opens the drawer. The preview exposes the update-notifier model
 * (preview/bootstrap.js), so nothing but this scenario decides when a check
 * runs and a background check cannot land between two assertions.
 */
async function openDrawer(session) {
  await session.evaluate(`window.__govardUpdatePromptModel.clearTimers()`);
  await session.evaluate(`window.__govardPreview.reset()`);
  await session.evaluate(`window.__govardPreview.installFixtures("settings")`);
  await session.evaluate(`document.querySelector('${GEAR}').click()`);
  await session.waitFor(`document.querySelector('${DRAWER}').classList.contains("hidden")`, false);
}

/**
 * The app's own settings reload path: `main.js` re-runs the controller's load()
 * when the OS colour scheme flips while the drawer sits on "system". Between
 * them, `Emulation.setEmulatedMedia` calls fire that listener, which is the only
 * way to make load() write into the drawer after the scenario installed its
 * fixture - and therefore the assertion that proves the controller's refs now
 * point at the island's DOM rather than at nothing.
 */
async function reloadSettings(session) {
  const media = (value) =>
    session.send("Emulation.setEmulatedMedia", {
      features: [{ name: "prefers-color-scheme", value }],
    });
  await media("light");
  await media("dark");
}

test("the settings drawer renders the fixture and saves exactly one change per edit", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  const viewport = { width: 1440, height: 900 };
  await session.send("Emulation.setDeviceMetricsOverride", {
    ...viewport,
    deviceScaleFactor: 1,
    mobile: false,
  });

  await openDrawer(session);

  // Prove the island is mounted before asserting anything about it: the vanilla
  // drawer injected its own markup, so the id alone proves nothing.
  assert.equal(
    await session.evaluate(`document.querySelector('[data-testid="check-updates"]') !== null`),
    true,
    "the settings island renders its own controls",
  );
  // The island owns its subtree now, so nothing in it may carry the attribute
  // main.js's document-wide delegate resolves (D5).
  assert.equal(
    await session.evaluate(`document.querySelectorAll('${DRAWER} [data-action]').length`),
    0,
    "a migrated subtree must carry no data-action for main.js's delegate",
  );

  await reloadSettings(session);
  // The fixture's values, not the boot defaults: load() wrote into the island.
  await session.waitFor(`document.getElementById("themeSelect").value`, "dark", { timeoutMs: 8000 });
  assert.equal(await session.evaluate(`document.getElementById("proxyTarget").value`), "govard.test");
  assert.equal(await session.evaluate(`document.getElementById("preferredBrowser").value`), "firefox");
  assert.equal(await session.evaluate(`document.getElementById("codeEditor").value`), "code");
  assert.equal(await session.evaluate(`document.getElementById("dbClientPreference").value`), "desktop");
  assert.equal(await session.evaluate(`document.getElementById("runInBackgroundToggle").checked`), false);
  assert.equal(await session.evaluate(`document.getElementById("updateChannelSelect").value`), "beta");
  // The fixture reports a tray host, so the hint stays hidden.
  assert.equal(
    await session.evaluate(`document.getElementById("trayUnavailableHint").classList.contains("hidden")`),
    true,
  );

  // One edit, one call - and the payload must be read from the island's own
  // fields, which is what makes save() work at all after the migration.
  await quiet(session, 300);
  const beforeSave = await callCount(session, "UpdateSettings");
  await session.evaluate(selectValue("#themeSelect", "light"));
  await session.waitFor(
    `window.__govardPreview.getCalls().some((c) => c.method === "UpdateSettings")`,
    true,
  );
  assert.equal(
    await callCount(session, "UpdateSettings") - beforeSave,
    1,
    "one edit must produce exactly one UpdateSettings call",
  );
  const payloads = JSON.parse(await callArgs(session, "UpdateSettings"));
  assert.deepEqual(payloads.at(-1), [
    {
      theme: "light",
      proxyTarget: "govard.test",
      preferredBrowser: "firefox",
      codeEditor: "code",
      dbClientPreference: "desktop",
      runInBackground: false,
    },
  ]);

  const shot = await session.screenshot({ x: 0, y: 0, ...viewport });
  assert.ok(shot.length > 0, "the scenario captures the rendered drawer");
  writeFileSync(join(tmpdir(), "settings-island.png"), shot);

  assert.deepEqual(session.consoleErrors, []);
});

test("the drawer's update controls call the backend once and keep the hidden-class contract", async (t) => {
  const session = await withPreview(t);
  if (!session) return;
  await openDrawer(session);

  // The update-control buttons are the island's now: one click, one call.
  await quiet(session, 300);
  assert.equal(await callCount(session, "CheckForUpdates"), 0);
  await session.evaluate(`document.querySelector('[data-testid="check-updates"]').click()`);
  await session.waitFor(
    `window.__govardPreview.getCalls().some((c) => c.method === "CheckForUpdates")`,
    true,
  );
  assert.equal(await callCount(session, "CheckForUpdates"), 1, "one click, one check");
  // The result reached the drawer, so the controller's refs are the island's.
  await session.waitFor(
    `document.getElementById("settingsUpdateStatus").textContent`,
    "Govard Desktop is up to date (1.0.0).",
  );
  assert.equal(await session.evaluate(`document.getElementById("settingsUpdateBadge").textContent`), "Current");
  // A not-outdated check keeps the install button hidden.
  assert.equal(
    await session.evaluate(`document.getElementById("installUpdateButton").classList.contains("hidden")`),
    true,
  );

  // The channel select is the island's too; the fixture answers "stable", so the
  // status message proves the response was consumed rather than the input echoed.
  const beforeChannel = await callCount(session, "SetUpdateChannel");
  await session.evaluate(selectValue("#updateChannelSelect", "beta"));
  await session.waitFor(
    `window.__govardPreview.getCalls().some((c) => c.method === "SetUpdateChannel")`,
    true,
  );
  assert.equal(
    await callCount(session, "SetUpdateChannel") - beforeChannel,
    1,
    "one change must produce exactly one SetUpdateChannel call",
  );
  assert.deepEqual(JSON.parse(await callArgs(session, "SetUpdateChannel")).at(-1), ["beta"]);
  await session.waitFor(
    `document.getElementById("status").textContent.includes("Update channel set to stable.")`,
    true,
  );

  // Review Focus: the drawer's own `hidden` class is what main.js's
  // isSettingsDrawerOpen probe and the update prompt read, so closing through
  // the island's own controls must keep toggling it.
  assert.equal(await session.evaluate(`document.querySelector('${DRAWER}').getAttribute("aria-hidden")`), "false");
  await session.evaluate(`document.querySelector('${CLOSE}').click()`);
  await session.waitFor(`document.querySelector('${DRAWER}').classList.contains("hidden")`, true);
  assert.equal(await session.evaluate(`document.querySelector('${DRAWER}').getAttribute("aria-hidden")`), "true");

  // And the backdrop, which is also the island's own element now.
  await session.evaluate(`document.querySelector('${GEAR}').click()`);
  await session.waitFor(`document.querySelector('${DRAWER}').classList.contains("hidden")`, false);
  await session.evaluate(`document.querySelector('${DRAWER}').click()`);
  await session.waitFor(`document.querySelector('${DRAWER}').classList.contains("hidden")`, true);

  assert.deepEqual(session.consoleErrors, []);
});
