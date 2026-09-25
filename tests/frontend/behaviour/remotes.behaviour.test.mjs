import test from "node:test";
import assert from "node:assert/strict";
import { writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { withPreview } from "./support/preview-session.mjs";

const REMOTES_TAB = '[data-action="switch-tab"][data-tab="remotes"]';
const CARD = "#remotesList [data-testid='remote-card']";
const MODAL = "syncOptionsModal";

const VIEWPORT = { width: 1440, height: 900 };

/**
 * Installs the fixture, selects a project and opens the tab.
 *
 * The remotes tab is project-scoped: its refresh returns early while no project
 * is selected, and the only control that selects one is the dashboard fetch -
 * which the app has already made at boot, before the fixture exists in a
 * scenario. So the scenario refreshes the dashboard, exactly as the app's own
 * control does, and only then opens the tab.
 */
async function openRemotes(session) {
  await session.evaluate(`window.__govardPreview.reset()`);
  await session.evaluate(`window.__govardPreview.installFixtures("remotes")`);
  await session.evaluate(`document.getElementById("refresh").click()`);
  await session.waitFor(
    `document.querySelector("#envList [data-testid='env-card']") !== null`,
    true,
    { timeoutMs: 10000 },
  );
  await session.evaluate(`document.querySelector('${REMOTES_TAB}').click()`);
  await session.waitFor(
    `document.getElementById("tab-remotes").classList.contains("active")`,
    true,
  );
  await session.waitFor(`document.querySelector("${CARD}") !== null`, true, {
    timeoutMs: 10000,
  });
}

async function openModal(session, preset) {
  await session.evaluate(
    `document.querySelector("#remotesList [data-testid='open-sync-modal'][data-preset='${preset}']").click()`,
  );
  await session.waitFor(
    `document.getElementById("${MODAL}") !== null && !document.getElementById("${MODAL}").classList.contains("hidden")`,
    true,
  );
}

test("the remotes tab renders the fixture through the island, one click one call", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.send("Emulation.setDeviceMetricsOverride", {
    ...VIEWPORT,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await openRemotes(session);

  // The island owns the whole tab now, so nothing in it may carry the attribute
  // main.js's document-wide delegate resolves (D5) - including the header's
  // Refresh button, which used to be served by the refresh-remotes branch.
  assert.equal(
    await session.evaluate(`document.querySelectorAll('#tab-remotes [data-action]').length`),
    0,
    "a migrated subtree must carry no data-action for main.js's delegate",
  );

  // Every fixture remote reached the list, in order, and the protected one keeps
  // the badge that says so.
  assert.equal(await session.evaluate(`document.querySelectorAll("${CARD}").length`), 3);
  assert.equal(
    await session.evaluate(
      `document.querySelector("#remotesList [data-testid='remote-card'][data-remote-name='production']").textContent.includes("Protected")`,
    ),
    true,
    "the protected remote keeps its badge",
  );
  assert.equal(
    await session.evaluate(
      `document.querySelector("#remotesList [data-testid='remote-card'][data-remote-name='staging']").textContent.includes("Auth: Keychain")`,
    ),
    true,
    "the auth summary still renders",
  );

  // One click, one call: the delegate used to own remote-test.
  await session.evaluate(
    `document.querySelector("#remotesList [data-testid='remote-test']").click()`,
  );
  await session.waitFor(
    `JSON.stringify(window.__govardPreview.getCalls()).includes("TestRemote")`,
    true,
  );
  const calls = JSON.parse(
    await session.evaluate(`JSON.stringify(window.__govardPreview.getCalls())`),
  );
  const testCalls = calls.filter((c) => c.method === "TestRemote");
  assert.equal(testCalls.length, 1, "one click must reach the bridge exactly once");
  assert.deepEqual(testCalls[0].args, ["sample-project", "staging"]);

  // Capability gating survives the port: the remote that declares only files
  // cannot pull the database, and one that declares nothing stays enabled.
  assert.equal(
    await session.evaluate(
      `document.querySelector("#remotesList [data-testid='remote-card'][data-remote-name='legacy'] [data-testid='open-sync-modal'][data-preset='db']").disabled`,
    ),
    true,
    "an undeclared capability disables the matching pull button",
  );
  assert.equal(
    await session.evaluate(
      `document.querySelector("#remotesList [data-testid='remote-card'][data-remote-name='staging'] [data-testid='open-sync-modal'][data-preset='media']").disabled`,
    ),
    false,
    "a declared capability keeps the matching pull button enabled",
  );

  const shot = await session.screenshot({ x: 0, y: 0, ...VIEWPORT });
  assert.ok(shot.length > 0, "the scenario captures the rendered remotes tab");
  writeFileSync(join(tmpdir(), "remotes-island.png"), shot);

  assert.deepEqual(session.consoleErrors, []);
});

test("the sync modal is a two-step machine whose confirm starts the sync", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.send("Emulation.setDeviceMetricsOverride", {
    ...VIEWPORT,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await openRemotes(session);
  await openModal(session, "db");
  await session.waitFor(
    `document.querySelectorAll("#syncModalOptionsContainer input").length > 0`,
    true,
  );

  // The modal is an island too: its own subtree routes through React, not the
  // delegate, and it opened on step 1 with the chosen remote and preset.
  assert.equal(
    await session.evaluate(`document.querySelectorAll('#${MODAL} [data-action]').length`),
    0,
    "the migrated modal must carry no data-action for main.js's delegate",
  );
  assert.equal(
    await session.evaluate(`document.getElementById("syncModalRemoteName").textContent`),
    "staging",
  );
  assert.equal(
    await session.evaluate(`document.getElementById("syncModalStep2").classList.contains("hidden")`),
    true,
    "the modal opens on step 1",
  );
  assert.equal(
    await session.evaluate(`document.querySelectorAll("#syncModalOptionsContainer input").length`),
    2,
    "every option the backend declares renders as a toggle",
  );

  // Toggling an option stays on step 1 and rewrites the config the plan uses.
  await session.evaluate(`document.querySelector("#syncModalOptionsContainer input").click()`);
  assert.equal(
    await session.evaluate(`document.getElementById("syncModalStep1").classList.contains("hidden")`),
    false,
    "toggling an option does not advance the machine",
  );

  // Step 1 -> step 2: the plan the bridge returned is rendered, loading is over.
  await session.evaluate(`document.getElementById("previewSyncPlanBtn").click()`);
  await session.waitFor(
    `document.getElementById("syncModalStep2").classList.contains("hidden") === false`,
    true,
  );
  await session.waitFor(
    `document.getElementById("syncPlanOutput").textContent.includes("Selected Pull Configuration")`,
    true,
  );
  assert.equal(
    await session.evaluate(`document.getElementById("syncPlanOutput").textContent.includes("db: full snapshot of 42 tables")`),
    true,
    "the plan text comes from the backend",
  );
  assert.equal(
    await session.evaluate(`document.getElementById("syncPlanLoading").classList.contains("hidden")`),
    true,
  );

  // Step 2 -> back, then step 2 again, then confirm: the sync starts and the
  // modal closes behind the same animation.
  await session.evaluate(
    `document.querySelector("[data-testid='back-to-sync-options']").click()`,
  );
  assert.equal(
    await session.evaluate(`document.getElementById("syncModalStep1").classList.contains("hidden")`),
    false,
    "Back returns to step 1",
  );
  await session.evaluate(`document.getElementById("previewSyncPlanBtn").click()`);
  await session.waitFor(
    `document.getElementById("syncPlanOutput").textContent.includes("Selected Pull Configuration")`,
    true,
  );
  await session.evaluate(`document.getElementById("confirmSyncBtn").click()`);
  await session.waitFor(
    `window.__govardPreview.getCalls().some((c) => c.method === "RunRemoteSync")`,
    true,
  );
  await session.waitFor(
    `document.getElementById("${MODAL}").classList.contains("hidden")`,
    true,
    { timeoutMs: 5000 },
  );
  const syncCall = JSON.parse(
    await session.evaluate(`JSON.stringify(window.__govardPreview.getCalls().filter(
      (c) => c.method === "RunRemoteSync",
    ))`),
  ).at(-1);
  assert.deepEqual(syncCall.args.slice(0, 3), ["sample-project", "staging", "db"]);
  // The option the user toggled travels with the sync.
  assert.equal(syncCall.args[3].noNoise, true, "the toggled option reaches the backend");

  // Escape closes the modal from outside it, and reopening resets to step 1.
  await openModal(session, "full");
  assert.equal(
    await session.evaluate(`document.getElementById("syncModalStep2").classList.contains("hidden")`),
    true,
    "a reopened modal starts at step 1 again",
  );
  await session.send("Input.dispatchKeyEvent", {
    type: "keyDown",
    key: "Escape",
    code: "Escape",
    windowsVirtualKeyCode: 27,
    nativeVirtualKeyCode: 27,
  });
  await session.waitFor(
    `document.getElementById("${MODAL}").classList.contains("hidden")`,
    true,
    { timeoutMs: 5000 },
  );

  assert.deepEqual(session.consoleErrors, []);
});

test("closing the sync modal while it is still opening leaves it closed", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.send("Emulation.setDeviceMetricsOverride", {
    ...VIEWPORT,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await openRemotes(session);

  // Opening awaits the backend's option list before it reveals the dialog, so a
  // close that lands during that await has to win: otherwise the continuation
  // reopens a dialog the user already dismissed. The delay is what makes that
  // window reachable at all - with an instant fixture the 300 ms close animation
  // finishes first and the bug hides behind the timing.
  await session.evaluate(`window.__govardPreviewRouteDelayMs = 600`);
  await session.evaluate(`(() => {
    document.querySelector("#remotesList [data-testid='open-sync-modal'][data-preset='db']").click();
    document.querySelector("[data-testid='close-sync-modal']").click();
  })()`);

  await new Promise((resolve) => setTimeout(resolve, 1400));
  assert.equal(
    await session.evaluate(`document.getElementById("${MODAL}").classList.contains("hidden")`),
    true,
    "a close during the open must not be overridden",
  );
  await session.evaluate(`window.__govardPreviewRouteDelayMs = 0`);
  // And a later open still works, so the guard is not a latch.
  await openModal(session, "db");
  assert.equal(
    await session.evaluate(`document.getElementById("syncModalRemoteName").textContent`),
    "staging",
  );

  assert.deepEqual(session.consoleErrors, []);
});
