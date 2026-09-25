import test from "node:test";
import assert from "node:assert/strict";
import { getState, getVersion, setState, subscribe } from "../../desktop/frontend/state/store.js";

test("subscribe is notified after setState", () => {
  let calls = 0;
  const off = subscribe(() => { calls++; });
  setState({ selectedProject: "sample-project" });
  assert.equal(calls, 1);
  off();
  setState({ selectedProject: "" });
  assert.equal(calls, 1, "unsubscribed listener must not fire again");
});

test("getState returns the same object identity across setState calls", () => {
  const before = getState();
  setState({ selectedProject: "sample-project" });
  assert.equal(getState(), before, "the store mutates in place; useSyncExternalStore relies on this");
});

// A stable snapshot object is what the vanilla modules need, but
// useSyncExternalStore re-renders only when the snapshot it reads CHANGES, so an
// in-place store needs a separate revision number for React to watch. Without it
// an island renders once and then never again.
test("getVersion changes on every setState", () => {
  const before = getVersion();
  setState({ logQuery: "sample" });
  const after = getVersion();
  assert.notEqual(after, before);
  setState({ logQuery: "sample-2" });
  assert.notEqual(getVersion(), after);
});

test("getVersion does not change when nothing is set", () => {
  const before = getVersion();
  setState({});
  assert.equal(getVersion(), before);
});

// Review finding (2026-09-25): setState notified listeners even for a call that
// carried no patch, so `setState()`, `setState(null)` and `setState({})` all woke
// every subscriber across the pre-existing vanilla modules that call setState
// liberally. Nothing changed, so nothing should be told.
test("setState with no patch does not notify listeners", () => {
  let calls = 0;
  const off = subscribe(() => { calls++; });
  setState();
  setState(null);
  setState({});
  assert.equal(calls, 0, "a call that changed nothing must not wake every subscriber");
  off();
});
