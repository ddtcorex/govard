import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  OPERATIONS_NOTIFICATION_EVENT,
  SYNC_PROGRESS_EVENT,
  isRetryable,
  normalizeHistoryList,
  normalizeOpList,
  rerunHintFor,
} from "../../desktop/frontend/modules/operation-center.js";
import {
  ROUTES,
  desktopBridge,
  __setBackendForTest,
} from "../../desktop/frontend/services/bridge.js";

const readIsland = () =>
  readFile(
    new URL("../../desktop/frontend/islands/OperationCenter.tsx", import.meta.url),
    "utf8",
  );

// A running op normalizes with its identity, scope, and start time, whatever
// casing the bindings produced.
test("normalizeOpList keeps running ops with scope labels", () => {
  const { ops } = normalizeOpList({
    Ops: [
      { ID: "sync:proj-a", Project: "proj-a", Kind: "sync", StartedAt: "2026-10-17T10:00:00Z" },
    ],
  });
  assert.equal(ops.length, 1);
  assert.equal(ops[0].id, "sync:proj-a");
  assert.equal(ops[0].project, "proj-a");
  assert.equal(ops[0].kind, "sync");
  assert.equal(ops[0].startedAt, "2026-10-17T10:00:00Z");
});

test("normalizeOpList tolerates a bare array and an empty payload", () => {
  assert.deepEqual(normalizeOpList([{ id: "up:proj-b", project: "proj-b", kind: "up" }]).ops.length, 1);
  assert.deepEqual(normalizeOpList().ops, []);
  assert.deepEqual(normalizeOpList({}).ops, []);
});

// History normalizes newest-first with the status the retry gate reads.
test("normalizeHistoryList keeps terminal records with status and message", () => {
  const { records } = normalizeHistoryList({
    Records: [
      { id: "sync:proj-a", project: "proj-a", kind: "sync", status: "failed", message: "boom" },
      { id: "up:proj-a", project: "proj-a", kind: "up", status: "completed", message: "" },
    ],
  });
  assert.equal(records.length, 2);
  assert.equal(records[0].status, "failed");
  assert.equal(records[0].message, "boom");
  assert.equal(records[1].status, "completed");
});

// Only failed ops show Retry: succeeded, cancelled, and interrupted records
// must never offer a replay.
test("isRetryable is true for failed ops only", () => {
  assert.equal(isRetryable({ status: "failed" }), true);
  assert.equal(isRetryable({ status: "completed" }), false);
  assert.equal(isRetryable({ status: "cancelled" }), false);
  assert.equal(isRetryable({ status: "interrupted" }), false);
  assert.equal(isRetryable({}), false);
});

// A restart-interrupted entry shows a re-run hint (a fresh run, not a
// replay); every other status shows no hint.
test("rerunHintFor guides interrupted entries to a fresh run", () => {
  const hint = rerunHintFor({ status: "interrupted" });
  assert.match(hint, /fresh run/i);
  assert.equal(rerunHintFor({ status: "failed" }), "");
  assert.equal(rerunHintFor({ status: "completed" }), "");
  assert.equal(rerunHintFor({ status: "cancelled" }), "");
});

// The island is the visible home for long ops: the running list carries a
// real cancel per op, history carries retry gated on failed-only, and every
// row carries its per-project scope label.
test("the island renders cancel for running ops and gated retry for history", async () => {
  const island = await readIsland();
  assert.ok(island.includes("operation-cancel"), "missing the cancel control for running ops");
  assert.ok(island.includes("cancelOp"), "cancel must call the bridge, not a shortcut");
  assert.ok(island.includes("operation-retry"), "missing the retry control for failed ops");
  assert.ok(island.includes("isRetryable"), "retry must be gated by the module helper, not inline status checks");
  assert.ok(island.includes("rerunHintFor"), "interrupted entries must show the re-run hint");
  assert.ok(island.includes("listOps"), "the running list must load through the bridge");
  assert.ok(island.includes("listHistory"), "history must load through the bridge");
});

// The center stays live: progress and notification events refresh both lists.
// The literals live once in the module; the island subscribes through them.
test("the island refreshes on sync progress and operation notifications", async () => {
  assert.equal(SYNC_PROGRESS_EVENT, "sync:progress");
  assert.equal(OPERATIONS_NOTIFICATION_EVENT, "operations:notification");
  const island = await readIsland();
  assert.ok(island.includes("SYNC_PROGRESS_EVENT"), "missing the sync progress subscription");
  assert.ok(island.includes("OPERATIONS_NOTIFICATION_EVENT"), "missing the operations notification subscription");
  assert.ok(island.includes("onEvent("), "missing the event subscription call");
});

// The bridge reaches the new backend through ROUTES, like every other call.
test("operation center routes reach the OperationService", () => {
  assert.deepEqual(ROUTES.ListOps, ["OperationService", "ListOps"]);
  assert.deepEqual(ROUTES.ListHistory, ["OperationService", "ListHistory"]);
  assert.deepEqual(ROUTES.CancelOp, ["OperationService", "CancelOp"]);
  assert.deepEqual(ROUTES.RetryOp, ["OperationService", "RetryOp"]);
});

test("bridge methods call the routed service method with their arguments", async () => {
  const calls = [];
  const restore = __setBackendForTest(async (service, method, args) => {
    calls.push([service, method, args]);
    return "ok";
  });
  try {
    assert.equal(await desktopBridge.cancelOp("sync:proj-a"), "ok");
    assert.equal(await desktopBridge.retryOp("sync:proj-a"), "ok");
    assert.deepEqual(calls, [
      ["OperationService", "CancelOp", ["sync:proj-a"]],
      ["OperationService", "RetryOp", ["sync:proj-a"]],
    ]);
  } finally {
    restore();
  }
});
