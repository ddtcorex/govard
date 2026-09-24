import test from "node:test";
import assert from "node:assert/strict";
import {
  ROUTES,
  desktopBridge,
  __setBackendForTest,
  __setBindingsLoaderForTest,
} from "../../desktop/frontend/services/bridge.js";

test("every route targets a known service", () => {
  const services = new Set([
    "SettingsService", "OnboardingService", "EnvironmentService", "RemoteService",
    "SystemService", "LogService", "GlobalServiceService", "UpdateService",
  ]);
  for (const [name, [service, method]] of Object.entries(ROUTES)) {
    assert.ok(services.has(service), `${name} routes to unknown service ${service}`);
    assert.match(method, /^[A-Z][A-Za-z]+$/, `${name} has a bad method name`);
  }
  assert.equal(Object.keys(ROUTES).length, 55);
});

test("renamed routes keep their frontend names", () => {
  assert.deepEqual(ROUTES.OpenEnvironment, ["EnvironmentService", "OpenEnvironment"]);
  assert.deepEqual(ROUTES.RunRemoteSyncBackground, ["RemoteService", "RunRemoteSync"]);
  assert.deepEqual(ROUTES.GetSyncPresetOptions, ["RemoteService", "GetSyncOptions"]);
});

test("bridge methods call the routed service method with their arguments", async () => {
  const calls = [];
  const restore = __setBackendForTest(async (service, method, args) => {
    calls.push([service, method, args]);
    return "ok";
  });
  try {
    assert.equal(await desktopBridge.startEnvironment("sample-project"), "ok");
    assert.deepEqual(calls, [["EnvironmentService", "StartEnvironment", ["sample-project"]]]);
  } finally {
    restore();
  }
});

test("a missing backend rejects with a readable error", async () => {
  const restore = __setBackendForTest(async () => {
    throw new Error("Desktop bridge not available: EnvironmentService.GetDashboard");
  });
  try {
    await assert.rejects(desktopBridge.getDashboard(), /Desktop bridge not available/);
  } finally {
    restore();
  }
});

test("bindings backend calls the generated service function", async () => {
  const restore = __setBindingsLoaderForTest(async () => ({
    EnvironmentService: { StartEnvironment: async (p) => `started ${p}` },
  }));
  try {
    assert.equal(await desktopBridge.startEnvironment("sample-project"), "started sample-project");
  } finally {
    restore();
  }
});

test("bindings that fail to load reject with a readable error", async () => {
  const restore = __setBindingsLoaderForTest(async () => {
    throw new Error("Failed to fetch dynamically imported module");
  });
  try {
    await assert.rejects(desktopBridge.getDashboard(), /Desktop bridge not available/);
  } finally {
    restore();
  }
});
