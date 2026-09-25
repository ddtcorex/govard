import test from "node:test";
import assert from "node:assert/strict";
import { desktopBridge } from "../../../desktop/frontend/services/bridge.js";
import { setFixtures, resetFixtures, getCalls, resetCalls } from "../../../desktop/frontend/preview/fixture-store.js";
import { installPreviewSeam, PREVIEW_SEAM } from "../../../desktop/frontend/preview/seam.js";

// The whole playback path with no browser: an app-facing bridge route, through
// the generated binding and $Call.ByID, into the active preview seam, out
// through the fixture lookup and the real model constructor.

test("the active seam is the Phase 0 winner", () => {
  assert.equal(PREVIEW_SEAM, "transport");
});

test("a fixture with the wire field names reaches the rendered values end to end", async () => {
  const restore = await installPreviewSeam();
  resetCalls();
  setFixtures([
    {
      service: "SystemService",
      method: "GetSystemMetrics",
      args: [],
      result: { cpuUsage: 12.5, memoryUsage: 2048.3 },
    },
  ]);
  try {
    const metrics = await desktopBridge.getSystemMetrics();
    assert.equal(metrics.cpuUsage, 12.5);
    assert.equal(metrics.memoryUsage, 2048.3);
    assert.deepEqual(getCalls(), [
      { service: "SystemService", method: "GetSystemMetrics", args: [] },
    ]);
  } finally {
    resetFixtures();
    restore();
  }
});

test("a fixture keyed on the Go struct field name never reaches the model field", async () => {
  const restore = await installPreviewSeam();
  setFixtures([
    {
      service: "SystemService",
      method: "GetSystemMetrics",
      args: [],
      result: { CPUUsage: 12.5, MemoryUsage: 2048.3 },
    },
  ]);
  try {
    const metrics = await desktopBridge.getSystemMetrics();
    // This is the Phase 0 finding as an executable pin: the Go struct tags emit
    // cpuUsage/memoryUsage, so a struct-name fixture is silently ignored by the
    // model constructor and the fields keep their defaults. A loader-level fake
    // (Candidate B) cannot reproduce this, which is why the gate chose the
    // transport seam.
    assert.equal(metrics.cpuUsage, 0);
    assert.equal(metrics.memoryUsage, 0);
  } finally {
    resetFixtures();
    restore();
  }
});

test("with no fixture installed every model route still answers with model defaults", async () => {
  const restore = await installPreviewSeam();
  resetFixtures();
  try {
    const metrics = await desktopBridge.getSystemMetrics();
    assert.equal(metrics.constructor.name, "SystemMetrics");
    assert.equal(metrics.cpuUsage, 0);
  } finally {
    restore();
  }
});

test("a fixture carrying an error rejects the bridge call with that message", async () => {
  const restore = await installPreviewSeam();
  setFixtures([
    {
      service: "EnvironmentService",
      method: "GetDashboard",
      args: [],
      error: "ssh: connection refused",
    },
  ]);
  try {
    await assert.rejects(() => desktopBridge.getDashboard(), /ssh: connection refused/);
  } finally {
    resetFixtures();
    restore();
  }
});
