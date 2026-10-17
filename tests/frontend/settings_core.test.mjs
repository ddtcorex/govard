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

/* ---------- Task 8: explicit Save/Discard + proxy validation ---------- */

const makeSettingsFieldRefs = (overrides = {}) => ({
  themeSelect: { value: "system" },
  proxyTarget: {
    value: "",
    setAttribute() {},
    removeAttribute() {},
  },
  preferredBrowser: { value: "" },
  codeEditor: { value: "" },
  dbClientPreference: { value: "pma" },
  runInBackgroundToggle: { checked: true },
  proxyTargetError: { textContent: "", classList: createClassList() },
  settingsDirtyDot: { classList: createClassList() },
  saveSettingsButton: { disabled: true },
  discardSettingsButton: { disabled: true },
  ...overrides,
});

test("typing in settings fields fires no settings RPC until Save", async () => {
  const island = await readFile(
    new URL("../../desktop/frontend/islands/SettingsDrawer.tsx", import.meta.url),
    "utf8",
  );
  const autoSaves = (island.match(/onChange=\{\(\) => void controller\.save\(\)\}/g) || []).length;
  assert.equal(autoSaves, 0, `expected no onChange autosave, found ${autoSaves}`);
});

test("settings drawer renders explicit Save/Discard plus a dirty indicator", async () => {
  const island = await readFile(
    new URL("../../desktop/frontend/islands/SettingsDrawer.tsx", import.meta.url),
    "utf8",
  );
  for (const marker of [
    'data-testid="save-settings"',
    'data-testid="discard-settings"',
    'data-testid="settings-dirty-dot"',
    'data-testid="proxy-target-error"',
  ]) {
    assert.equal(island.includes(marker), true, `expected ${marker} in settings drawer`);
  }
});

test("settings drawer fits a 400px window", async () => {
  const island = await readFile(
    new URL("../../desktop/frontend/islands/SettingsDrawer.tsx", import.meta.url),
    "utf8",
  );
  assert.equal(island.includes("w-[460px]"), true, "drawer keeps its 460px width on wide windows");
  assert.equal(
    island.includes("max-w-full"),
    true,
    "drawer panel must cap at the viewport width so a 400px window fits",
  );
});

test("settings module exports a proxy URL validator", async () => {
  const mod = await import("../../desktop/frontend/modules/settings.js");
  assert.equal(
    typeof mod.validateProxyTarget,
    "function",
    "expected a validateProxyTarget export",
  );
  assert.equal(mod.validateProxyTarget(""), "", "empty proxy URL means unset, not invalid");
  assert.equal(mod.validateProxyTarget("govard.test"), "", "bare hostname is valid");
  assert.equal(mod.validateProxyTarget("https://govard.test:8443"), "", "full URL is valid");
  assert.equal(mod.validateProxyTarget("localhost"), "", "localhost is valid");
  assert.notEqual(mod.validateProxyTarget("not a url"), "", "spaces must be rejected");
  assert.notEqual(mod.validateProxyTarget("ftp://govard.test"), "", "non-http scheme must be rejected");
  assert.notEqual(mod.validateProxyTarget("http://"), "", "missing host must be rejected");
});

test("save with an invalid proxy URL fires no RPC and reports a field error", async () => {
  const refs = makeSettingsFieldRefs();
  let calls = 0;
  const bridge = {
    updateSettings: async () => {
      calls += 1;
      return "ok";
    },
  };
  const controller = createSettingsController({
    bridge,
    refs,
    onStatus: () => {},
    onToast: () => {},
  });

  refs.proxyTarget.value = "not a url with spaces";
  const result = await controller.save();

  assert.equal(calls, 0, "invalid proxy URL must block the settings RPC");
  assert.equal(result.ok, false, "save must report failure for an invalid proxy URL");
  assert.match(refs.proxyTargetError.textContent, /hostname|URL/i, "field error must explain the problem");
  assert.equal(
    refs.proxyTargetError.classList.contains("hidden"),
    false,
    "field error must be visible",
  );
});

test("editing a field marks the form dirty without any RPC", () => {
  const refs = makeSettingsFieldRefs();
  let calls = 0;
  const bridge = {
    updateSettings: async () => {
      calls += 1;
      return "ok";
    },
  };
  const controller = createSettingsController({
    bridge,
    refs,
    onStatus: () => {},
    onToast: () => {},
  });

  assert.equal(controller.isDirty(), false, "fresh form must read clean");
  refs.proxyTarget.value = "edited.test";
  controller.handleFieldChange();

  assert.equal(calls, 0, "editing a field must not fire updateSettings");
  assert.equal(controller.isDirty(), true, "edited form must read dirty");
  assert.equal(refs.settingsDirtyDot.classList.contains("hidden"), false, "dirty dot must show");
  assert.equal(refs.saveSettingsButton.disabled, false, "Save must enable when dirty");
  assert.equal(refs.discardSettingsButton.disabled, false, "Discard must enable when dirty");
});

test("save with a valid proxy URL fires one RPC and clears dirty", async () => {
  globalThis.document = { documentElement: { classList: createClassList() } };
  globalThis.window = { matchMedia: () => ({ matches: false }) };
  try {
    const refs = makeSettingsFieldRefs();
    let calls = 0;
    const bridge = {
      updateSettings: async () => {
        calls += 1;
        return "ok";
      },
    };
    const controller = createSettingsController({
      bridge,
      refs,
      onStatus: () => {},
      onToast: () => {},
    });

    refs.proxyTarget.value = "govard.test";
    controller.handleFieldChange();
    const result = await controller.save();

    assert.equal(result.ok, true, "valid settings must save");
    assert.equal(calls, 1, "one save must produce exactly one UpdateSettings call");
    assert.equal(controller.isDirty(), false, "saved form must read clean");
    assert.equal(refs.settingsDirtyDot.classList.contains("hidden"), true, "dirty dot must hide after save");
    assert.equal(refs.saveSettingsButton.disabled, true, "Save must disable after save");
    assert.equal(
      refs.proxyTargetError.classList.contains("hidden"),
      true,
      "field error must stay hidden for valid input",
    );
  } finally {
    delete globalThis.document;
    delete globalThis.window;
  }
});

test("discard restores saved values and clears dirty", async () => {
  globalThis.document = { documentElement: { classList: createClassList() } };
  globalThis.window = { matchMedia: () => ({ matches: false }) };
  try {
    const refs = makeSettingsFieldRefs();
    const bridge = {
      getSettings: async () => ({ proxyTarget: "saved.test", theme: "dark" }),
      getUpdateChannel: async () => "stable",
    };
    const controller = createSettingsController({
      bridge,
      refs,
      onStatus: () => {},
      onToast: () => {},
    });

    await controller.load();
    assert.equal(refs.proxyTarget.value, "saved.test", "load must populate the proxy field");

    refs.proxyTarget.value = "edited.test";
    controller.handleFieldChange();
    assert.equal(controller.isDirty(), true, "edited form must read dirty");

    controller.discard();

    assert.equal(refs.proxyTarget.value, "saved.test", "discard must restore the saved proxy URL");
    assert.equal(controller.isDirty(), false, "discarded form must read clean");
    assert.equal(
      refs.settingsDirtyDot.classList.contains("hidden"),
      true,
      "dirty dot must hide after discard",
    );
  } finally {
    delete globalThis.document;
    delete globalThis.window;
  }
});

/* ---------- Task 10: lastChecked + restart retry ---------- */

test("checkForUpdates records lastChecked and renders it in settings", async () => {
  const refs = {
    settingsUpdateStatus: { textContent: "" },
    settingsUpdateLastChecked: { textContent: "" },
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
      outdated: false,
      currentVersion: "v1.1.0",
      latestVersion: "v1.1.0",
      message: "Govard Desktop is up to date (v1.1.0).",
    }),
  };

  const controller = createSettingsController({
    bridge,
    refs,
    onStatus: () => {},
    onToast: () => {},
  });

  const result = await controller.checkForUpdates({ silent: true });

  assert.ok(
    typeof result.lastChecked === "string" && result.lastChecked !== "",
    "the check result must carry a lastChecked timestamp",
  );
  assert.match(
    refs.settingsUpdateLastChecked.textContent,
    /Last checked/,
    "settings must render the last-checked timestamp",
  );
});

test("restart failure shows Retry Restart and retry clears it on success", async () => {
  let restartCalls = 0;
  let failRestart = true;
  const refs = {
    settingsUpdateStatus: { textContent: "" },
    settingsUpdateBadge: { textContent: "", className: "" },
    checkUpdatesButton: { disabled: false, innerHTML: "" },
    installUpdateButton: {
      disabled: false,
      innerHTML: "",
      classList: createClassList(),
    },
    retryRestartButton: { classList: createClassList() },
  };
  refs.retryRestartButton.classList.add("hidden");

  const bridge = {
    installLatestUpdate: async () => {},
    restartDesktopApp: async () => {
      restartCalls += 1;
      if (failRestart) {
        throw new Error("restart denied");
      }
    },
  };

  const controller = createSettingsController({
    bridge,
    refs,
    onStatus: () => {},
    onToast: () => {},
  });

  const outcome = await controller.installLatestUpdate();
  assert.equal(outcome.restartFailed, true, "restart failure must be reported");
  assert.equal(
    refs.retryRestartButton.classList.contains("hidden"),
    false,
    "Retry Restart must show after a restart failure",
  );

  assert.equal(
    typeof controller.retryRestart,
    "function",
    "expected a retryRestart method on the settings controller",
  );
  failRestart = false;
  const retry = await controller.retryRestart();
  assert.equal(retry.ok, true);
  assert.equal(restartCalls, 2, "retry must call restartDesktopApp again");
  assert.equal(
    refs.retryRestartButton.classList.contains("hidden"),
    true,
    "a successful retry hides Retry Restart",
  );
});

test("settings drawer renders retry-restart and last-checked elements", async () => {
  const island = await readFile(
    new URL("../../desktop/frontend/islands/SettingsDrawer.tsx", import.meta.url),
    "utf8",
  );
  for (const marker of [
    'id="retryRestartButton"',
    'data-testid="retry-restart"',
    'id="settingsUpdateLastChecked"',
    "retryRestart",
  ]) {
    assert.equal(island.includes(marker), true, `expected ${marker} in settings drawer`);
  }
});
