import test from "node:test";
import assert from "node:assert/strict";

import { createVisiblePoll } from "../../desktop/frontend/utils/visible-interval.js";

const stubDoc = (visibilityState = "visible") => {
  const listeners = new Map();
  return {
    visibilityState,
    listeners,
    addEventListener: (type, fn) => {
      listeners.set(type, [...(listeners.get(type) || []), fn]);
    },
    removeEventListener: (type, fn) => {
      listeners.set(
        type,
        (listeners.get(type) || []).filter((f) => f !== fn),
      );
    },
    fire: (type) => {
      for (const fn of listeners.get(type) || []) fn();
    },
  };
};

const stubTimers = () => {
  const scheduled = new Map();
  let nextId = 1;
  return {
    scheduled,
    setTimer: (fn, ms) => {
      const id = nextId++;
      scheduled.set(id, { fn, ms });
      return id;
    },
    clearTimer: (id) => {
      scheduled.delete(id);
    },
  };
};

test("no timer is scheduled while the document starts hidden", () => {
  const doc = stubDoc("hidden");
  const timers = stubTimers();
  let ticks = 0;
  const poll = createVisiblePoll({
    intervalMs: 2000,
    onTick: () => {
      ticks += 1;
    },
    doc,
    ...timers,
  });
  assert.equal(timers.scheduled.size, 0);
  assert.equal(ticks, 0);
  poll.dispose();
});

test("returning to visible fires once and restarts the timer", () => {
  const doc = stubDoc("hidden");
  const timers = stubTimers();
  let ticks = 0;
  const poll = createVisiblePoll({
    intervalMs: 2000,
    onTick: () => {
      ticks += 1;
    },
    doc,
    ...timers,
  });
  doc.visibilityState = "visible";
  doc.fire("visibilitychange");
  assert.equal(ticks, 1);
  assert.equal(timers.scheduled.size, 1);
  const [[, { ms }]] = [...timers.scheduled];
  assert.equal(ms, 2000);
  // The restarted timer ticks without another visibility event.
  const [[, { fn }]] = [...timers.scheduled];
  fn();
  assert.equal(ticks, 2);
  poll.dispose();
});

test("hiding clears the timer and dispose removes the listener", () => {
  const doc = stubDoc("visible");
  const timers = stubTimers();
  let ticks = 0;
  const poll = createVisiblePoll({
    intervalMs: 2000,
    onTick: () => {
      ticks += 1;
    },
    doc,
    ...timers,
  });
  assert.equal(timers.scheduled.size, 1);
  doc.visibilityState = "hidden";
  doc.fire("visibilitychange");
  assert.equal(timers.scheduled.size, 0);
  assert.equal(ticks, 0);
  poll.dispose();
  assert.equal(doc.listeners.get("visibilitychange").length, 0);
});
