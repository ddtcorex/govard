import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import { createUpdateNotifierModel } from "../../desktop/frontend/modules/update-notifier.js";

// A drawer-open flag each test flips, standing in for the settings drawer's
// "hidden" class that the old controller read from the DOM.
const createDrawer = (open = false) => {
  const drawer = { open };
  return { drawer, isSettingsDrawerOpen: () => drawer.open };
};

test("checkForUpdatesInBackground shows prompt when update is available", async () => {
  const { isSettingsDrawerOpen } = createDrawer();
  const statuses = [];
  const settingsController = {
    async checkForUpdates() {
      return {
        skipped: false,
        failed: false,
        outdated: true,
        currentVersion: "v1.0.0",
        latestVersion: "v1.1.0",
        message: "Update available: v1.0.0 -> v1.1.0",
      };
    },
    async installLatestUpdate() {
      return { ok: true };
    },
  };

  const controller = createUpdateNotifierModel({
    settingsController,
    isSettingsDrawerOpen,
    onStatus: (message) => statuses.push(message),
  });

  await controller.checkForUpdatesInBackground();

  assert.equal(controller.getSnapshot().visible, true);
  assert.equal(controller.getSnapshot().currentVersion, "v1.0.0");
  assert.equal(controller.getSnapshot().latestVersion, "v1.1.0");
  assert.equal(
    controller.getSnapshot().message,
    "A new Govard Desktop version is ready to install.",
  );
  assert.deepEqual(statuses, ["Update available."]);
});

test("checkForUpdatesInBackground preserves custom non-redundant prompt message", async () => {
  const { isSettingsDrawerOpen } = createDrawer();
  const settingsController = {
    async checkForUpdates() {
      return {
        skipped: false,
        failed: false,
        outdated: true,
        currentVersion: "v1.0.0",
        latestVersion: "v1.1.0",
        message: "Security fixes and performance improvements are included.",
      };
    },
    async installLatestUpdate() {
      return { ok: true };
    },
  };

  const controller = createUpdateNotifierModel({
    settingsController,
    isSettingsDrawerOpen,
    onStatus: () => {},
  });

  await controller.checkForUpdatesInBackground();

  assert.equal(
    controller.getSnapshot().message,
    "Security fixes and performance improvements are included.",
  );
});

test("checkForUpdatesInBackground keeps prompt hidden when no update", async () => {
  const { isSettingsDrawerOpen } = createDrawer();
  const settingsController = {
    async checkForUpdates() {
      return {
        skipped: false,
        failed: false,
        outdated: false,
        currentVersion: "v1.1.0",
        latestVersion: "v1.1.0",
        message: "Govard Desktop is up to date (v1.1.0).",
      };
    },
    async installLatestUpdate() {
      return { ok: true };
    },
  };

  const controller = createUpdateNotifierModel({
    settingsController,
    isSettingsDrawerOpen,
    onStatus: () => {},
  });

  await controller.checkForUpdatesInBackground();

  assert.equal(controller.getSnapshot().visible, false);
});

test("dismissPrompt suppresses repeated prompt for same latest version", async () => {
  const { isSettingsDrawerOpen } = createDrawer();
  const settingsController = {
    async checkForUpdates() {
      return {
        skipped: false,
        failed: false,
        outdated: true,
        currentVersion: "v1.0.0",
        latestVersion: "v1.2.0",
        message: "Update available: v1.0.0 -> v1.2.0",
      };
    },
    async installLatestUpdate() {
      return { ok: true };
    },
  };

  const controller = createUpdateNotifierModel({
    settingsController,
    isSettingsDrawerOpen,
    onStatus: () => {},
  });

  await controller.checkForUpdatesInBackground();
  assert.equal(controller.getSnapshot().visible, true);

  controller.dismissPrompt();
  assert.equal(controller.getSnapshot().visible, false);

  await controller.checkForUpdatesInBackground();
  assert.equal(controller.getSnapshot().visible, false);
});

test("installLatestUpdateFromPrompt delegates to settings installer and hides prompt on success", async () => {
  const { isSettingsDrawerOpen } = createDrawer();
  let installCalled = 0;
  const settingsController = {
    async checkForUpdates() {
      return {
        skipped: false,
        failed: false,
        outdated: true,
        currentVersion: "v1.0.0",
        latestVersion: "v1.3.0",
        message: "Update available: v1.0.0 -> v1.3.0",
      };
    },
    async installLatestUpdate() {
      installCalled += 1;
      return { ok: true, skipped: false };
    },
  };

  const controller = createUpdateNotifierModel({
    settingsController,
    isSettingsDrawerOpen,
    onStatus: () => {},
  });

  await controller.checkForUpdatesInBackground();
  assert.equal(controller.getSnapshot().visible, true);

  const outcome = await controller.installLatestUpdateFromPrompt();
  assert.equal(Boolean(outcome?.ok), true);
  assert.equal(installCalled, 1);
  assert.equal(controller.getSnapshot().visible, false);
});

test("checkForUpdatesInBackground suppresses prompt while settings drawer is open", async () => {
  const { drawer, isSettingsDrawerOpen } = createDrawer();
  drawer.open = true;
  const settingsController = {
    async checkForUpdates() {
      return {
        skipped: false,
        failed: false,
        outdated: true,
        currentVersion: "v1.6.0",
        latestVersion: "v1.7.0",
        message: "Update available: v1.6.0 -> v1.7.0",
      };
    },
    async installLatestUpdate() {
      return { ok: true };
    },
  };

  const controller = createUpdateNotifierModel({
    settingsController,
    isSettingsDrawerOpen,
    onStatus: () => {},
  });

  await controller.checkForUpdatesInBackground();
  assert.equal(controller.getSnapshot().visible, false);

  drawer.open = false;
  controller.syncWithSettingsDrawer();
  assert.equal(controller.getSnapshot().visible, true);
});

test("a dismissed version stays hidden across settings drawer toggles", async () => {
  const { drawer, isSettingsDrawerOpen } = createDrawer();
  const controller = createUpdateNotifierModel({
    settingsController: {
      async checkForUpdates() {
        return { outdated: true, currentVersion: "v1.6.0", latestVersion: "v1.7.0" };
      },
    },
    isSettingsDrawerOpen,
    onStatus: () => {},
  });

  await controller.checkForUpdatesInBackground();
  assert.equal(controller.getSnapshot().visible, true);

  drawer.open = true;
  assert.equal(controller.syncWithSettingsDrawer(), false);
  assert.equal(controller.getSnapshot().visible, false);

  drawer.open = false;
  assert.equal(controller.syncWithSettingsDrawer(), true);
  controller.dismissPrompt();

  drawer.open = true;
  controller.syncWithSettingsDrawer();
  drawer.open = false;
  assert.equal(controller.syncWithSettingsDrawer(), false);
  assert.equal(controller.getSnapshot().visible, false);

  await controller.checkForUpdatesInBackground();
  assert.equal(controller.getSnapshot().visible, false);
});

test("update prompt island renders the prompt elements without data-action", async () => {
  const tsx = await readFile(
    new URL("../../desktop/frontend/islands/UpdatePrompt.tsx", import.meta.url),
    "utf8",
  );

  for (const marker of [
    'id="updatePrompt"',
    'id="updatePromptMessage"',
    'id="updatePromptChangelog"',
    'id="updatePromptCurrent"',
    'id="updatePromptLatest"',
    'id="installUpdatePromptButton"',
    'aria-label="Dismiss update prompt"',
    "update-message-text",
  ]) {
    assert.equal(tsx.includes(marker), true, `missing ${marker} in UpdatePrompt.tsx`);
  }
  assert.equal(tsx.includes("data-action"), false, "D5: the island carries no data-action");
  assert.equal(
    tsx.includes("dangerouslySetInnerHTML"),
    false,
    "the prompt renders server text as text only",
  );

  const html = await readFile(
    new URL("../../desktop/frontend/index.html", import.meta.url),
    "utf8",
  );
  assert.equal(html.includes('id="updatePromptIsland"'), true, "missing island container");
  assert.equal(html.includes('id="updatePrompt"'), false, "static prompt markup left behind");
});

test("clearTimers cancels a scheduled check", async () => {
  let checks = 0;
  const model = createUpdateNotifierModel({
    settingsController: {
      checkForUpdates: async () => {
        checks++;
        return { outdated: false };
      },
    },
    onStatus: () => {},
    isSettingsDrawerOpen: () => false,
  });
  model.scheduleBackgroundChecks({ startupDelayMs: 20, intervalMs: 60_000 });
  model.clearTimers();
  await new Promise((r) => setTimeout(r, 60));
  assert.equal(checks, 0);
});

test("every change publishes a new snapshot object", async () => {
  const model = createUpdateNotifierModel({
    settingsController: {
      checkForUpdates: async () => ({
        outdated: true,
        currentVersion: "1.0.0",
        latestVersion: "1.1.0",
      }),
    },
    onStatus: () => {},
    isSettingsDrawerOpen: () => false,
  });
  const first = model.getSnapshot();
  assert.equal(Object.isFrozen(first), true);
  let notified = 0;
  const unsubscribe = model.subscribe(() => notified++);
  await model.checkForUpdatesInBackground();
  assert.notEqual(model.getSnapshot(), first);
  assert.equal(model.getSnapshot().visible, true);
  assert.ok(notified > 0);

  unsubscribe();
  const seen = notified;
  model.dismissPrompt();
  assert.equal(model.getSnapshot().visible, false);
  assert.equal(notified, seen, "an unsubscribed listener is not called");
});
