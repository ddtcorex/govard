import test from "node:test";
import assert from "node:assert/strict";
import { mountIsland } from "../../desktop/frontend/islands/mount.js";
import { ErrorBoundary } from "../../desktop/frontend/islands/ErrorBoundary.js";

test("mountIsland returns null when the container is missing", () => {
  globalThis.document = { getElementById: () => null };
  try {
    assert.equal(mountIsland("missing-container", null), null);
  } finally {
    delete globalThis.document;
  }
});

test("ErrorBoundary reports caught render errors to the backend log", async () => {
  const { __setBackendForTest } = await import(
    "../../desktop/frontend/services/bridge.js"
  );
  const calls = [];
  let seenConsoleError = false;
  const restore = __setBackendForTest(async (service, method, args) => {
    calls.push([service, method, args]);
    return "Frontend error reported";
  });
  try {
    const boundary = Object.create(ErrorBoundary.prototype);
    const orig = console.error;
    console.error = () => {
      seenConsoleError = true;
    };
    try {
      boundary.componentDidCatch(new Error("boom"), { componentStack: "in Panel" });
    } finally {
      console.error = orig;
    }
    await new Promise((resolve) => setTimeout(resolve, 0));
  } finally {
    restore();
  }
  assert.deepEqual(
    calls.map(([service, method]) => `${service}.${method}`),
    ["SystemService.ReportFrontendError"],
  );
  assert.equal(
    String(calls[0][2][0]).includes("boom"),
    true,
    "the reported message must carry the error",
  );
  assert.equal(
    seenConsoleError,
    true,
    "componentDidCatch must still log to the console",
  );
});
