import test from "node:test";
import assert from "node:assert/strict";
import {
  __setTransportForTest,
  loadGeneratedModules,
  CANCEL_CALL_OBJECT_ID,
} from "../../../desktop/frontend/services/bridge.js";

// The preview's Candidate A seam lives in bridge.js rather than in a preview
// file: the frontend runtime guard allows exactly two modules to touch the
// runtime or the generated bindings, and a preview file importing
// "@wailsio/runtime" fails TestDesktopFrontendUsesBridgeOnly. These tests pin
// the seam's behaviour from outside, through the real generated bindings.

test("a transport installed by the seam answers a real generated binding call", async () => {
  const calls = [];
  const restore = await __setTransportForTest({
    call: async (objectID, method, windowName, args) => {
      calls.push({ objectID, method, windowName, args });
      return { cpuUsage: 7.5 };
    },
  });
  try {
    const bindings = await loadGeneratedModules();
    const metrics = await bindings.SystemService.GetSystemMetrics();
    assert.equal(calls.length, 1);
    assert.equal(calls[0].objectID, 0, "binding calls arrive as objectNames.Call");
    assert.deepEqual(calls[0].args.args, []);
    // The generated binding runs createFrom on the transport's raw JSON, so a
    // field the fake omitted comes back as the model's own default. This is the
    // property that makes Candidate A faithful: a fixture keyed on the Go struct
    // field name instead of the JSON tag lands in cpuUsage's default, not here.
    assert.equal(metrics.cpuUsage, 7.5);
    assert.equal(metrics.memoryUsage, 0);
  } finally {
    restore();
  }
});

test("restoring a seam puts the previous transport back", async () => {
  const first = [];
  const second = [];
  const restoreFirst = await __setTransportForTest({
    call: async () => {
      first.push(1);
      return { cpuUsage: 1 };
    },
  });
  const restoreSecond = await __setTransportForTest({
    call: async () => {
      second.push(1);
      return { cpuUsage: 2 };
    },
  });
  try {
    const bindings = await loadGeneratedModules();
    assert.equal((await bindings.SystemService.GetSystemMetrics()).cpuUsage, 2);
    restoreSecond();
    assert.equal((await bindings.SystemService.GetSystemMetrics()).cpuUsage, 1);
    assert.equal(first.length, 1);
    assert.equal(second.length, 1);
  } finally {
    restoreFirst();
  }
});

test("cancelling a pending binding call reaches the transport as CancelCall", async () => {
  const seen = [];
  const restore = await __setTransportForTest({
    call: (objectID) => {
      seen.push(objectID);
      if (objectID === CANCEL_CALL_OBJECT_ID) {
        return Promise.resolve(true);
      }
      // Never settles, so the call is still running when it is cancelled.
      return new Promise(() => {});
    },
  });
  try {
    const bindings = await loadGeneratedModules();
    const pending = bindings.SystemService.GetSystemMetrics();
    await pending.cancel("test");
    assert.deepEqual(seen, [0, CANCEL_CALL_OBJECT_ID]);
    assert.equal(CANCEL_CALL_OBJECT_ID, 10, "objectNames.CancelCall is 10 in the v3 runtime");
  } finally {
    restore();
  }
});
