import test from "node:test";
import assert from "node:assert/strict";
import { onEvent, hasEventRuntime, __setRuntimeLoaderForTest } from "../../desktop/frontend/services/events.js";

const tick = () => new Promise((r) => setTimeout(r, 0));

test("hasEventRuntime is false outside a browser", () => {
  assert.equal(hasEventRuntime(), false);
});

// The runtime module loads in a plain browser too, so a DOM alone does not mean
// a backend. The transports below are the ones @wailsio/runtime itself probes.
test("hasEventRuntime is false in a plain browser tab", () => {
  globalThis.window = { document: {} };
  try {
    assert.equal(hasEventRuntime(), false);
  } finally {
    delete globalThis.window;
  }
});

test("hasEventRuntime is true when the WebKit host transport is present", () => {
  globalThis.window = {
    webkit: { messageHandlers: { external: { postMessage() {} } } },
  };
  try {
    assert.equal(hasEventRuntime(), true);
  } finally {
    delete globalThis.window;
  }
});

test("hasEventRuntime is true when the Windows host transport is present", () => {
  globalThis.window = { chrome: { webview: { postMessage() {} } } };
  try {
    assert.equal(hasEventRuntime(), true);
  } finally {
    delete globalThis.window;
  }
});

test("onEvent subscribes through the runtime and unwraps event data", async () => {
  const seen = [];
  let unsubscribed = false;
  const restore = __setRuntimeLoaderForTest(async () => ({
    Events: {
      On(name, cb) {
        seen.push(name);
        cb({ name, data: "payload" });
        return () => {
          unsubscribed = true;
        };
      },
    },
  }));
  try {
    let received;
    const off = onEvent("sync:output", (data) => {
      received = data;
    });
    await tick();
    assert.deepEqual(seen, ["sync:output"]);
    assert.equal(received, "payload");
    off();
    assert.equal(unsubscribed, true);
  } finally {
    restore();
  }
});

test("unsubscribing before the runtime loads prevents the subscription", async () => {
  let subscribed = false;
  const restore = __setRuntimeLoaderForTest(async () => ({
    Events: { On() { subscribed = true; return () => {}; } },
  }));
  try {
    const off = onEvent("logs:line", () => {});
    off();
    await tick();
    assert.equal(subscribed, false);
  } finally {
    restore();
  }
});

test("a runtime that fails to load yields a working no-op unsubscribe", async () => {
  const restore = __setRuntimeLoaderForTest(async () => {
    throw new Error("no runtime");
  });
  try {
    const off = onEvent("logs:line", () => {});
    await tick();
    off();
  } finally {
    restore();
  }
});
