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

test("a failed open during the close window cannot strand the sync modal in closing", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.send("Emulation.setDeviceMetricsOverride", {
    ...VIEWPORT,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await openRemotes(session);
  await openModal(session, "db");

  // open() cancels the close timer, so while its option load is in flight the
  // close can only finish through transitionend. Removing the transition makes
  // that event never fire, which is the worst case a throttled or reduced-motion
  // run can hit: if the failed open then leaves the dialog in `closing`, a
  // transparent full-screen backdrop is left over the whole app.
  await session.evaluate(`(() => {
    const style = document.createElement("style");
    style.textContent = "#${MODAL}, #${MODAL} * { transition: none !important; }";
    document.head.appendChild(style);
  })()`);
  await session.evaluate(`window.__govardPreview.appendFixtures([
    { service: "RemoteService", method: "GetSyncOptions", args: [${JSON.stringify("sample-project")}, "media"], error: "options backend down" },
  ])`);
  await session.evaluate(`window.__govardPreviewRouteDelayMs = 600`);
  await session.evaluate(`(() => {
    document.querySelector("[data-testid='close-sync-modal']").click();
    document.querySelector("#remotesList [data-testid='open-sync-modal'][data-preset='media']").click();
  })()`);

  await session.waitFor(
    `document.getElementById("toastContainer").textContent.includes("options backend down")`,
    true,
    { timeoutMs: 5000 },
  );
  await session.waitFor(
    `document.getElementById("${MODAL}").classList.contains("hidden")`,
    true,
    { timeoutMs: 3000 },
  );
  await session.evaluate(`window.__govardPreviewRouteDelayMs = 0`);
});

const quiet = (session, ms) => session.evaluate(`new Promise((r) => setTimeout(r, ${ms}))`);
const PROJECT = "sample-project";
const PLAN = "#syncPlanOutput";
const CONFIRM = "document.getElementById('confirmSyncBtn')";

/** Clicks Preview Plan, leaving the plan call in flight. */
const clickPreview = (session) =>
  session.evaluate(`document.getElementById("previewSyncPlanBtn").click()`);

test("sync modal shows an error and stays closed when preset options fail", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.send("Emulation.setDeviceMetricsOverride", {
    ...VIEWPORT,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await openRemotes(session);

  // A good open first, so the previous preset's toggles are what a failed open
  // would have to leave behind.
  await openModal(session, "db");
  await session.waitFor(
    `document.querySelectorAll("#syncModalOptionsContainer input").length`,
    2,
  );
  await session.evaluate(`document.querySelector("[data-testid='close-sync-modal']").click()`);
  await session.waitFor(
    `document.getElementById("${MODAL}").classList.contains("hidden")`,
    true,
  );

  await session.evaluate(`window.__govardPreview.appendFixtures([
    { service: "RemoteService", method: "GetSyncOptions", args: [${JSON.stringify(PROJECT)}, "media"], error: "options backend down" },
  ])`);
  await session.evaluate(
    `document.querySelector("#remotesList [data-testid='open-sync-modal'][data-preset='media']").click()`,
  );
  await session.waitFor(
    `document.getElementById("toastContainer").textContent.includes("options backend down")`,
    true,
  );
  await quiet(session, 500);
  assert.equal(
    await session.evaluate(`document.getElementById("${MODAL}").classList.contains("hidden")`),
    true,
    "a failed option load must not open the dialog",
  );
  assert.equal(
    await session.evaluate(`document.querySelectorAll("#syncModalOptionsContainer input").length`),
    0,
    "the previous preset's toggles must not survive a failed open",
  );
  assert.equal(
    await session.evaluate(`document.querySelector("#toastContainer .toast--error") !== null`),
    true,
    "the failure surfaces as an error toast",
  );

  // The failure is not a latch: the next open works.
  await openModal(session, "db");
  await session.waitFor(
    `document.querySelectorAll("#syncModalOptionsContainer input").length`,
    2,
  );
});

test("a stale plan preview never overwrites a newer one", async (t) => {
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
    `document.querySelectorAll("#syncModalOptionsContainer input").length`,
    2,
  );

  // The plan call is keyed by the config, so one toggle makes the second preview
  // a different request: the first is slow, the second instant.
  const first = { noNoise: false, noPii: false };
  const second = { noNoise: true, noPii: false };
  await session.evaluate(`window.__govardPreview.appendFixtures([
    { service: "RemoteService", method: "RunRemoteSyncPreset", args: [${JSON.stringify(PROJECT)}, "staging", "db", ${JSON.stringify(first)}], result: "PLAN-STALE", delayMs: 1200 },
    { service: "RemoteService", method: "RunRemoteSyncPreset", args: [${JSON.stringify(PROJECT)}, "staging", "db", ${JSON.stringify(second)}], result: "PLAN-NEWEST" },
  ])`);

  await clickPreview(session);
  await session.evaluate(
    `document.querySelector("[data-testid='back-to-sync-options']").click()`,
  );
  await session.evaluate(`document.querySelector("#syncModalOptionsContainer input").click()`);
  await clickPreview(session);
  await session.waitFor(
    `document.querySelector("${PLAN}").textContent.includes("PLAN-NEWEST")`,
    true,
  );

  // Let the slow first response land.
  await quiet(session, 1600);
  const text = await session.evaluate(`document.querySelector("${PLAN}").textContent`);
  assert.ok(text.includes("PLAN-NEWEST"), `the newest plan was replaced: ${text}`);
  assert.ok(!text.includes("PLAN-STALE"), `a stale plan leaked in: ${text}`);
  assert.equal(
    await session.evaluate(`document.getElementById("syncPlanLoading").classList.contains("hidden")`),
    true,
  );
});

test("execute is disabled until a plan exists", async (t) => {
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
    `document.querySelectorAll("#syncModalOptionsContainer input").length`,
    2,
  );
  await session.evaluate(`window.__govardPreview.appendFixtures([
    { service: "RemoteService", method: "RunRemoteSyncPreset", args: [${JSON.stringify(PROJECT)}, "staging", "db", { noNoise: false, noPii: false }], result: "PLAN-LATE", delayMs: 1000 },
  ])`);

  await clickPreview(session);
  await session.waitFor(
    `document.getElementById("syncPlanLoading").classList.contains("hidden")`,
    false,
  );
  assert.equal(
    await session.evaluate(`${CONFIRM}.disabled`),
    true,
    "Execute must be disabled while the plan is generating",
  );
  await session.evaluate(`${CONFIRM}.click()`);
  await quiet(session, 200);
  assert.equal(
    await session.evaluate(
      `window.__govardPreview.getCalls().some((c) => c.method === "RunRemoteSync")`,
    ),
    false,
    "a disabled Execute must not start a sync",
  );

  await session.waitFor(
    `document.querySelector("${PLAN}").textContent.includes("PLAN-LATE")`,
    true,
  );
  assert.equal(
    await session.evaluate(`${CONFIRM}.disabled`),
    false,
    "Execute is available once the plan is shown",
  );
});

test("a rejected background sync clears the progress card and syncing flags", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.send("Emulation.setDeviceMetricsOverride", {
    ...VIEWPORT,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await openRemotes(session);

  const CARD_STAGING = `document.querySelector("#remotesList [data-testid='remote-card'][data-remote-name='staging']")`;
  const indicator = `document.querySelector("#remotesList .visual-sync-indicator")`;
  const progressText = `document.getElementById("visual-sync-progress-line").textContent`;

  // The two failures use different configs because lookup answers with the first
  // entry whose args match exactly.
  async function runFailingSync(config, fixtureEntry, wantText) {
    await session.evaluate(
      `window.__govardPreview.appendFixtures([${JSON.stringify({ ...fixtureEntry, args: [PROJECT, "staging", "db", config] })}])`,
    );
    await openModal(session, "db");
    await session.waitFor(
      `document.querySelectorAll("#syncModalOptionsContainer input").length`,
      2,
    );
    if (config.noNoise) {
      await session.evaluate(`document.querySelector("#syncModalOptionsContainer input").click()`);
    }
    await clickPreview(session);
    await session.waitFor(
      `document.querySelector("${PLAN}").textContent.includes("full snapshot")`,
      true,
    );
    await session.evaluate(`${CONFIRM}.click()`);
    await session.waitFor(`${progressText}.includes("[FAILED]")`, true);
    assert.ok(
      (await session.evaluate(progressText)).includes(wantText),
      "the failure reason reaches the progress card",
    );
    assert.equal(
      await session.evaluate(`${indicator}.className.includes("animate-pulse")`),
      false,
      "a failed sync must stop the running indicator",
    );
    assert.equal(
      await session.evaluate(`${CARD_STAGING}.className.includes("border-emerald-500/50")`),
      false,
      "a failed sync must clear the syncing flags",
    );
    assert.equal(
      await session.evaluate(`document.querySelectorAll("#toastContainer .toast--error").length`),
      1,
      "a failed sync raises exactly one error toast",
    );
    await session.evaluate(
      `document.querySelectorAll("#toastContainer .toast").forEach((el) => el.remove())`,
    );
  }

  // The bridge call rejects.
  await runFailingSync(
    { noNoise: false, noPii: false },
    { service: "RemoteService", method: "RunRemoteSync", error: "rejected by backend" },
    "rejected by backend",
  );
  // The bridge call resolves with the failure string.
  await runFailingSync(
    { noNoise: true, noPii: false },
    {
      service: "RemoteService",
      method: "RunRemoteSync",
      result: "Remote sync background process failed: exit status 7",
    },
    "exit status 7",
  );
});

test("escape closes the sync modal the moment it appears (#484)", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.send("Emulation.setDeviceMetricsOverride", {
    ...VIEWPORT,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await openRemotes(session);

  // openModal returns as soon as the dialog is no longer hidden, which is the
  // commit that shows it. A key sent right then used to be dropped about one run
  // in three, because the listener attached only in an effect after that commit,
  // so the scenario looked like a modal stuck open. Twelve rounds make a dropped
  // key all but certain to show.
  for (let round = 0; round < 12; round += 1) {
    await openModal(session, "full");
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
      { timeoutMs: 3000 },
    );
  }

  assert.deepEqual(session.consoleErrors, []);
});

test("execute stays disabled after a failed plan preview", async (t) => {
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
    `document.querySelectorAll("#syncModalOptionsContainer input").length`,
    2,
  );
  // Only the untouched config fails; the toggled one falls through to the
  // fixture's good plan.
  await session.evaluate(`window.__govardPreview.appendFixtures([
    { service: "RemoteService", method: "RunRemoteSyncPreset", args: [${JSON.stringify(PROJECT)}, "staging", "db", { noNoise: false, noPii: false }], error: "plan backend down" },
  ])`);

  await clickPreview(session);
  await session.waitFor(
    `document.querySelector("${PLAN}").textContent.includes("Failed to generate plan")`,
    true,
  );
  assert.equal(
    await session.evaluate(`${CONFIRM}.disabled`),
    true,
    "a failure message is not a plan: Execute must stay disabled",
  );
  await session.evaluate(`${CONFIRM}.click()`);
  await quiet(session, 200);
  assert.equal(
    await session.evaluate(
      `window.__govardPreview.getCalls().some((c) => c.method === "RunRemoteSync")`,
    ),
    false,
    "no sync may start from a failed plan",
  );

  await session.evaluate(
    `document.querySelector("[data-testid='back-to-sync-options']").click()`,
  );
  await session.evaluate(`document.querySelector("#syncModalOptionsContainer input").click()`);
  await clickPreview(session);
  await session.waitFor(
    `document.querySelector("${PLAN}").textContent.includes("full snapshot")`,
    true,
  );
  assert.equal(
    await session.evaluate(`${CONFIRM}.disabled`),
    false,
    "a later successful preview enables Execute",
  );
});

test("the sync modal closes on transitionend, not only on the fallback timer", async (t) => {
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
    `document.getElementById("${MODAL}").className.includes("opacity-0")`,
    false,
  );

  // Park the 300 ms fallback far away and switch the CSS transitions off, so the
  // browser fires no transitionend of its own: only the events dispatched below
  // can settle the close. Timers set while it is patched are the only ones
  // affected.
  await session.evaluate(`(() => {
    const style = document.createElement("style");
    style.id = "no-transitions";
    style.textContent = "#${MODAL}, #${MODAL} * { transition: none !important; }";
    document.head.append(style);
    window.__realSetTimeout = window.setTimeout;
    window.setTimeout = (fn, ms, ...rest) =>
      window.__realSetTimeout(fn, ms === 300 ? 600000 : ms, ...rest);
    document.querySelector("[data-testid='close-sync-modal']").click();
  })()`);
  await session.waitFor(
    `document.getElementById("${MODAL}").className.includes("opacity-0")`,
    true,
  );
  await session.evaluate(`window.setTimeout = window.__realSetTimeout`);

  // The dialog card's own transform transition bubbles up and must not close it.
  await session.evaluate(`document.getElementById("${MODAL}").firstElementChild.dispatchEvent(
    new TransitionEvent("transitionend", { propertyName: "transform", bubbles: true }))`);
  await quiet(session, 300);
  assert.equal(
    await session.evaluate(`document.getElementById("${MODAL}").classList.contains("hidden")`),
    false,
    "a bubbled transform transition must not settle the close",
  );

  // The backdrop's opacity transition ending does.
  await session.evaluate(`document.getElementById("${MODAL}").dispatchEvent(
    new TransitionEvent("transitionend", { propertyName: "opacity", bubbles: true }))`);
  await session.waitFor(
    `document.getElementById("${MODAL}").classList.contains("hidden")`,
    true,
    { timeoutMs: 2000 },
  );

  assert.deepEqual(session.consoleErrors, []);
});
