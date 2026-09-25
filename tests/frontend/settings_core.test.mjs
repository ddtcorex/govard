import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  applyTheme,
  createSettingsController,
  normalizeSettingsPayload,
} from "../../desktop/frontend/modules/settings.js";

test("normalizeSettingsPayload maps settings payload", () => {
  const value = normalizeSettingsPayload({
    theme: "dark",
    proxyTarget: "govard.test",
    preferredBrowser: "firefox",
  });
  assert.deepEqual(value, {
    theme: "dark",
    proxyTarget: "govard.test",
    preferredBrowser: "firefox",
    codeEditor: "",
    dbClientPreference: "pma",
    runInBackground: true,
  });
});

test("normalizeSettingsPayload falls back to defaults", () => {
  const value = normalizeSettingsPayload({});
  assert.deepEqual(value, {
    theme: "system",
    proxyTarget: "",
    preferredBrowser: "",
    codeEditor: "",
    dbClientPreference: "pma",
    runInBackground: true,
  });
});

test("the settings island draws the update controls the controller writes into", async () => {
  // The drawer's markup moved into a React island, so these assertions read the
  // island source: the [data-action] routing the global delegate used is gone
  // (spec D5) and the controls are matched by data-testid instead.
  const island = await readFile(
    new URL("../../desktop/frontend/islands/SettingsDrawer.tsx", import.meta.url),
    "utf8",
  );

  assert.equal(
    island.includes('data-testid="check-updates"'),
    true,
    "expected check-updates control in settings drawer",
  );
  assert.equal(
    island.includes('data-testid="install-update"'),
    true,
    "expected install-update control in settings drawer",
  );
  assert.equal(
    island.includes('id="settingsUpdateStatus"'),
    true,
    "expected settingsUpdateStatus element in settings drawer",
  );
  assert.equal(
    island.includes("update-message-text"),
    true,
    "expected shared update message style class in settings drawer",
  );
  assert.equal(
    /data-action\s*=/.test(island),
    false,
    "the migrated subtree must carry no data-action for main.js's delegate",
  );
});

const createClassList = () => {
  const set = new Set();
  return {
    add(value) {
      set.add(value);
    },
    remove(value) {
      set.delete(value);
    },
    toggle(value, force) {
      if (force === undefined) {
        if (set.has(value)) {
          set.delete(value);
        } else {
          set.add(value);
        }
        return;
      }
      if (force) {
        set.add(value);
      } else {
        set.delete(value);
      }
    },
    contains(value) {
      return set.has(value);
    },
  };
};

test("checkForUpdates normalizes redundant update message in settings", async () => {
  const refs = {
    settingsUpdateStatus: { textContent: "" },
    settingsUpdateBadge: { textContent: "", className: "" },
    checkUpdatesButton: { disabled: false, innerHTML: "" },
    installUpdateButton: {
      disabled: false,
      innerHTML: "",
      classList: createClassList(),
    },
  };

  const bridge = {
    checkForUpdates: async () => ({
      outdated: true,
      currentVersion: "v1.16.0",
      latestVersion: "v1.15.0",
      message: "Update available: v1.16.0 -> v1.15.0",
    }),
  };

  const controller = createSettingsController({
    bridge,
    refs,
    onStatus: () => {},
    onToast: () => {},
  });

  const result = await controller.checkForUpdates({ silent: true });

  assert.equal(result.outdated, true);
  assert.equal(
    result.message,
    "A new Govard Desktop version is ready to install (v1.16.0 -> v1.15.0).",
  );
  assert.equal(
    refs.settingsUpdateStatus.textContent,
    "A new Govard Desktop version is ready to install (v1.16.0 -> v1.15.0).",
  );
});

/* ---------- applyTheme tests ---------- */

test("applyTheme adds dark class for theme=dark", () => {
  const classList = createClassList();
  globalThis.document = { documentElement: { classList } };
  applyTheme("dark");
  assert.equal(classList.contains("dark"), true, "dark class should be present");
  delete globalThis.document;
});

test("applyTheme removes dark class for theme=light", () => {
  const classList = createClassList();
  classList.add("dark"); // start in dark mode
  globalThis.document = { documentElement: { classList } };
  applyTheme("light");
  assert.equal(classList.contains("dark"), false, "dark class should be removed");
  delete globalThis.document;
});

test("applyTheme respects prefers-color-scheme for theme=system", () => {
  const classList = createClassList();
  globalThis.document = { documentElement: { classList } };
  globalThis.window = {
    matchMedia: (query) => ({
      matches: query === "(prefers-color-scheme: dark)",
    }),
  };
  applyTheme("system");
  assert.equal(classList.contains("dark"), true, "should detect dark from matchMedia");
  delete globalThis.document;
  delete globalThis.window;
});

test("the settings island draws the update channel select", async () => {
  const island = await readFile(
    new URL("../../desktop/frontend/islands/SettingsDrawer.tsx", import.meta.url),
    "utf8",
  );

  assert.equal(
    island.includes('id="updateChannelSelect"'),
    true,
    "expected update channel select in settings drawer",
  );
});

test("load reads update channel from bridge", async () => {
  globalThis.document = { documentElement: { classList: createClassList() } };
  globalThis.window = { matchMedia: () => ({ matches: false }) };

  const refs = {
    updateChannelSelect: { value: "" },
  };
  const bridge = {
    getSettings: async () => ({}),
    getUpdateChannel: async () => "beta",
  };

  const controller = createSettingsController({
    bridge,
    refs,
    onStatus: () => {},
    onToast: () => {},
  });

  await controller.load();

  assert.equal(refs.updateChannelSelect.value, "beta");
  delete globalThis.document;
  delete globalThis.window;
});

test("load reads update channel even when getSettings fails", async () => {
  globalThis.document = { documentElement: { classList: createClassList() } };
  globalThis.window = { matchMedia: () => ({ matches: false }) };

  const refs = {
    updateChannelSelect: { value: "" },
  };
  const bridge = {
    getSettings: async () => {
      throw new Error("boom");
    },
    getUpdateChannel: async () => "beta",
  };

  const controller = createSettingsController({
    bridge,
    refs,
    onStatus: () => {},
    onToast: () => {},
  });

  await controller.load();

  assert.equal(refs.updateChannelSelect.value, "beta");
  delete globalThis.document;
  delete globalThis.window;
});

test("setUpdateChannel persists channel via bridge and updates status", async () => {
  let statusMessage = "";
  const bridge = {
    setUpdateChannel: async (channel) => channel,
  };

  const controller = createSettingsController({
    bridge,
    refs: {},
    onStatus: (msg) => {
      statusMessage = msg;
    },
    onToast: () => {},
  });

  const result = await controller.setUpdateChannel("beta");

  assert.equal(result.ok, true);
  assert.equal(result.channel, "beta");
  assert.equal(statusMessage, "Update channel set to beta.");
});

test("setUpdateChannel surfaces bridge errors", async () => {
  const bridge = {
    setUpdateChannel: async () => {
      throw new Error("invalid update channel");
    },
  };

  const controller = createSettingsController({
    bridge,
    refs: {},
    onStatus: () => {},
    onToast: () => {},
  });

  const result = await controller.setUpdateChannel("nightly");

  assert.equal(result.ok, false);
  assert.equal(result.message, "invalid update channel");
});

test("setUpdateChannel resyncs select to last-known-good channel on failure", async () => {
  const refs = {
    updateChannelSelect: { value: "stable" },
  };
  const bridge = {
    setUpdateChannel: async () => {
      throw new Error("invalid update channel");
    },
  };

  const controller = createSettingsController({
    bridge,
    refs,
    onStatus: () => {},
    onToast: () => {},
  });

  // Simulate the user picking "beta" in the <select> before the rejected call.
  refs.updateChannelSelect.value = "beta";

  const result = await controller.setUpdateChannel("beta");

  assert.equal(result.ok, false);
  assert.equal(
    refs.updateChannelSelect.value,
    "stable",
    "select should snap back to the last persisted channel after a rejected update",
  );
});

test("settings shows a hint when no tray host is available", async () => {
  const { readFile } = await import("node:fs/promises");
  const settingsJS = await readFile(
    new URL("../../desktop/frontend/modules/settings.js", import.meta.url),
    "utf8",
  );
  // The query stays in the controller; the hint's markup moved into the island.
  const island = await readFile(
    new URL("../../desktop/frontend/islands/SettingsDrawer.tsx", import.meta.url),
    "utf8",
  );
  assert.equal(settingsJS.includes("getTrayStatus"), true, "settings must query the tray status");
  assert.equal(
    island.includes('data-testid="tray-unavailable-hint"'),
    true,
    "settings must render the tray hint",
  );
});
