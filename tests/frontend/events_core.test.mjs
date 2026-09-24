import test from "node:test";
import assert from "node:assert/strict";

test("onEvent returns a no-op unsubscribe when the runtime is absent", async () => {
  delete globalThis.window;
  const { onEvent, hasEventRuntime } = await import(
    "../../desktop/frontend/services/events.js?absent"
  );
  assert.equal(hasEventRuntime(), false);
  const off = onEvent("logs:line", () => {});
  assert.equal(typeof off, "function");
  off();
});

test("onEvent subscribes through the runtime and returns its unsubscribe", async () => {
  const calls = [];
  let unsubscribed = false;
  globalThis.window = {
    runtime: {
      EventsOn(name, handler) {
        calls.push(name);
        handler("payload");
        return () => {
          unsubscribed = true;
        };
      },
    },
  };
  const { onEvent, hasEventRuntime } = await import(
    "../../desktop/frontend/services/events.js?present"
  );
  let received;
  const off = onEvent("sync:output", (data) => {
    received = data;
  });
  assert.equal(hasEventRuntime(), true);
  assert.deepEqual(calls, ["sync:output"]);
  assert.equal(received, "payload");
  off();
  assert.equal(unsubscribed, true);
  delete globalThis.window;
});
