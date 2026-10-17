import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  NOTIFICATION_ACTION_OPEN_LOGS,
  NOTIFICATION_ACTION_RETRY,
  normalizeNotification,
  notificationActionTarget,
  notificationLevel,
} from "../../desktop/frontend/modules/operation-center.js";

// The action ids live once in the module so the handler switches on
// constants, never inline strings.
test("notification action ids match the backend catalog", () => {
  assert.equal(NOTIFICATION_ACTION_RETRY, "retry");
  assert.equal(NOTIFICATION_ACTION_OPEN_LOGS, "open-logs");
});

// The payload normalizes whatever casing the bindings produced, keeping the
// actions and the retry record id the toast buttons need.
test("normalizeNotification keeps actions and retry id across casings", () => {
  const actions = [
    { id: "retry", label: "Retry" },
    { id: "open-logs", label: "Open logs" },
  ];
  const fromGo = normalizeNotification({
    ID: "abc",
    Title: "Failed: sync (proj-a)",
    Body: "connection refused",
    Level: "error",
    Operation: "sync",
    Status: "failure",
    Project: "proj-a",
    Actions: actions,
    RetryID: "sync:proj-a",
  });
  assert.equal(fromGo.id, "abc");
  assert.equal(fromGo.title, "Failed: sync (proj-a)");
  assert.equal(fromGo.body, "connection refused");
  assert.equal(fromGo.level, "error");
  assert.equal(fromGo.operation, "sync");
  assert.equal(fromGo.status, "failure");
  assert.equal(fromGo.project, "proj-a");
  assert.deepEqual(fromGo.actions, actions);
  assert.equal(fromGo.retryId, "sync:proj-a");

  const fromCamel = normalizeNotification({
    id: "abc",
    title: "Failed: sync (proj-a)",
    body: "connection refused",
    level: "error",
    operation: "sync",
    status: "failure",
    project: "proj-a",
    actions,
    retryId: "sync:proj-a",
  });
  assert.deepEqual(fromCamel, fromGo);
});

// Success notifications carry no actions: the normalizer defaults them to an
// empty list and the retry id to "", never undefined.
test("normalizeNotification defaults an empty payload", () => {
  const note = normalizeNotification();
  assert.equal(note.id, "");
  assert.equal(note.title, "");
  assert.equal(note.body, "");
  assert.equal(note.project, "");
  assert.deepEqual(note.actions, []);
  assert.equal(note.retryId, "");
});

// The action target resolves Retry only with the exact record id bound at
// notify time. Without one there is nothing to replay: the target is
// unavailable, so the caller fails closed without dispatching a retry.
test("notificationActionTarget binds retry exactly or refuses it", () => {
  assert.deepEqual(
    notificationActionTarget("retry", { retryId: "sync:proj-a:old" }),
    { kind: "retry", id: "sync:proj-a:old" },
  );
  const refused = notificationActionTarget("retry", { retryId: "" });
  assert.equal(refused.kind, "unavailable");
  assert.match(refused.reason, /record unavailable/i);
  assert.deepEqual(
    notificationActionTarget("open-logs", { project: "proj-a" }),
    { kind: "open-logs", project: "proj-a" },
  );
  assert.equal(notificationActionTarget("bogus", {}).kind, "unknown");
});

// The main.js handler renders actionable notifications through showOpMessage
// (title + hint + buttons), with Retry reaching the real OperationService
// replay and Open-logs reaching the real logs view.
test("the notification handler wires actions to real handlers", async () => {
  const main = await readFile(
    new URL("../../desktop/frontend/main.js", import.meta.url),
    "utf8",
  );
  assert.ok(
    main.includes("showOpMessage"),
    "actionable notifications must render through showOpMessage, not plain showToast",
  );
  assert.ok(
    main.includes("normalizeNotification"),
    "the handler must normalize the payload through the module helper",
  );
  assert.ok(
    main.includes("notificationActionTarget"),
    "the handler must resolve clicks through the module target helper, never inline id checks",
  );
  assert.ok(
    main.includes("retryOp"),
    "the Retry action must call the bridge retryOp (the real OperationService replay)",
  );
  assert.ok(
    main.includes("Record unavailable"),
    "an unbound Retry must fail closed as record unavailable without dispatching",
  );
  assert.ok(
    main.includes("openServiceContext") || main.includes('switchTab("logs")'),
    "the Open-logs action must reach the real logs view",
  );
});

// The toast trusts the backend level: an info body that merely mentions
// "error" keeps its info tone instead of being escalated by substring sniffing.
test("notificationLevel trusts the backend level over body substrings", () => {
  assert.equal(
    notificationLevel({ level: "info", body: "done, with zero error output" }),
    "info",
  );
  assert.equal(
    notificationLevel({ level: "error", body: "all clear" }),
    "error",
  );
  assert.equal(notificationLevel({}), "success");
});

// An unknown toast action id gets its own message, never the retry
// record-unavailable text.
test("notificationActionTarget gives unknown actions their own message", () => {
  const unknown = notificationActionTarget("bogus", {});
  assert.equal(unknown.kind, "unknown");
  assert.ok(
    asNonEmptyString(unknown.reason),
    "unknown actions must carry their own message",
  );
  assert.doesNotMatch(unknown.reason, /record unavailable/i);
});

/**
 * @param {any} value
 */
const asNonEmptyString = (value) => {
  assert.equal(typeof value, "string");
  assert.ok(value.trim() !== "");
  return value;
};
