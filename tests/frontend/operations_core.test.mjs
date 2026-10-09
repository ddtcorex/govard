import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  beginOperation,
  endOperation,
  isOperationInFlight,
  operationLabel,
} from "../../desktop/frontend/modules/operations.js";

test("beginOperation hands out one token per key", () => {
  const token = beginOperation("env-start:sample-project");
  try {
    assert.ok(typeof token === "string" && token.length > 0);
    assert.equal(isOperationInFlight("env-start:sample-project"), true);
    assert.equal(beginOperation("env-start:sample-project"), null);
  } finally {
    endOperation(token);
  }
  assert.equal(isOperationInFlight("env-start:sample-project"), false);
  const again = beginOperation("env-start:sample-project");
  assert.ok(again !== null);
  endOperation(again);
});

test("different keys are independent and unknown tokens are no-ops", () => {
  const a = beginOperation("env-start:one");
  const b = beginOperation("env-stop:one");
  try {
    assert.ok(a !== null && b !== null && a !== b);
  } finally {
    endOperation(a);
    endOperation(b);
  }
  assert.doesNotThrow(() => endOperation("no-such-token"));
  assert.equal(isOperationInFlight("env-start:one"), false);
});

test("the feedback sequence lives in the store, not a module global", async () => {
  const src = await readFile(
    new URL("../../desktop/frontend/modules/global-services.js", import.meta.url),
    "utf8",
  );
  assert.equal(
    src.includes("feedbackSeq"),
    false,
    "the module-global counter must be gone; the store owns the sequence",
  );
});

test("the sync preset default resolver is defined exactly once", async () => {
  const [mainJS, remotesJS] = await Promise.all(
    ["../../desktop/frontend/main.js", "../../desktop/frontend/modules/remotes.js"].map(
      (rel) => readFile(new URL(rel, import.meta.url), "utf8"),
    ),
  );
  const definitions = (text) =>
    [...text.matchAll(/(?:const|function)\s+resolveSyncPresetConfig\s*=/g)].length +
    [...text.matchAll(/function\s+resolveSyncPresetConfig\s*\(/g)].length;
  assert.equal(definitions(remotesJS), 1);
  assert.equal(definitions(mainJS), 0);
  assert.ok(
    mainJS.includes("resolveSyncPresetConfig"),
    "main.js must delegate to the single resolver",
  );
});

test("one mutating operation per project, labeled", () => {
  const token = beginOperation("env:sample-project", "env-start");
  try {
    assert.ok(typeof token === "string" && token.length > 0);
    assert.equal(beginOperation("env:sample-project", "env-stop"), null);
    assert.equal(operationLabel("env:sample-project"), "env-start");
  } finally {
    endOperation(token);
  }
  assert.equal(operationLabel("env:sample-project"), "");
  assert.equal(isOperationInFlight("env:sample-project"), false);
});

test("releaseWhenSettled holds the key until the backend settles", async () => {
  const { beginOperation, isOperationInFlight, releaseWhenSettled } = await import(
    "../../desktop/frontend/modules/operations.js"
  );
  let releaseBackend;
  const backend = new Promise((resolve) => {
    releaseBackend = resolve;
  });
  const token = beginOperation("env:slow-project", "env-start");
  assert.ok(token);
  releaseWhenSettled(token, backend);
  assert.equal(isOperationInFlight("env:slow-project"), true);
  assert.equal(beginOperation("env:slow-project", "env-stop"), null);
  releaseBackend("started");
  await backend;
  // Let the release microtask run.
  await Promise.resolve();
  await Promise.resolve();
  assert.equal(isOperationInFlight("env:slow-project"), false);
});

test("releaseWhenSettled releases on rejection too", async () => {
  const { beginOperation, isOperationInFlight, releaseWhenSettled } = await import(
    "../../desktop/frontend/modules/operations.js"
  );
  let rejectBackend;
  const backend = new Promise((_, reject) => {
    rejectBackend = reject;
  });
  const token = beginOperation("env:fail-project", "env-start");
  releaseWhenSettled(token, backend);
  rejectBackend(new Error("gone"));
  await backend.catch(() => {});
  await Promise.resolve();
  await Promise.resolve();
  assert.equal(isOperationInFlight("env:fail-project"), false);
});
