import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  TOOL_COMPLETED_EVENT,
  TOOL_FAILED_EVENT,
  TOOL_OUTPUT_EVENT,
  NO_PROJECT_GUIDANCE,
  maxToolOutputLines,
  isToolRunEnabled,
  buildToolRunPayload,
  currentToolGeneration,
  applyToolOutputBatch,
  applyToolTerminalEvent,
  toolTerminalOutcome,
} from "../../desktop/frontend/modules/tool-palette.js";
import {
  ROUTES,
  desktopBridge,
  __setBackendForTest,
} from "../../desktop/frontend/services/bridge.js";

// The palette never sends argv/command lines: only a preset ID plus an args
// map. Exec argv is backend-owned, so a hostile frontend cannot upgrade to a
// shell (controller binding decision 1).

test("run is disabled without a project", () => {
  assert.equal(isToolRunEnabled(""), false);
  assert.equal(isToolRunEnabled("   "), false);
  assert.equal(isToolRunEnabled("sample-project"), true);
});

test("no-project state carries selection guidance", () => {
  assert.equal(typeof NO_PROJECT_GUIDANCE, "string");
  assert.ok(NO_PROJECT_GUIDANCE.length > 0);
  assert.match(NO_PROJECT_GUIDANCE, /project/i);
});

test("run payload carries preset ID plus args map only", () => {
  const payload = buildToolRunPayload("laravel-migrate", {
    database: "forge",
    force: "",
  });
  assert.equal(payload.presetId, "laravel-migrate");
  assert.deepEqual(payload.args, { database: "forge" });
  assert.ok(!("argv" in payload), "payload must never carry argv");
  assert.ok(!("command" in payload), "payload must never carry a command line");
});

test("run payload trims the preset ID and drops blank args", () => {
  const payload = buildToolRunPayload("  npm-install  ", { extra: "  " });
  assert.equal(payload.presetId, "npm-install");
  assert.deepEqual(payload.args, {});
});

test("current generation is the newest history gen for the op", () => {
  const history = [
    { opId: "tool:/a", gen: 1 },
    { opId: "tool:/a", gen: 2 },
    { opId: "tool:/b", gen: 9 },
  ];
  assert.equal(currentToolGeneration(history, "tool:/a"), 2);
  assert.equal(currentToolGeneration(history, "tool:/b"), 9);
  assert.equal(currentToolGeneration(history, "tool:/missing"), 0);
  assert.equal(currentToolGeneration([], "tool:/a"), 0);
});

test("terminal event for the current generation finishes the run", () => {
  const run = { opId: "tool:/a", gen: 2, finished: false, outcome: "" };
  const next = applyToolTerminalEvent(run, {
    opId: "tool:/a",
    gen: 2,
    event: TOOL_COMPLETED_EVENT,
  });
  assert.equal(next.finished, true);
  assert.equal(next.outcome, "success");
});

test("terminal event for a stale generation is ignored", () => {
  const run = { opId: "tool:/a", gen: 3, finished: false, outcome: "" };
  const next = applyToolTerminalEvent(run, {
    opId: "tool:/a",
    gen: 2,
    event: TOOL_FAILED_EVENT,
  });
  assert.equal(next.finished, false);
  assert.equal(next.outcome, "");
});

test("terminal event adopts the generation while the run has none yet", () => {
  const run = { opId: "tool:/a", gen: 0, finished: false, outcome: "" };
  const next = applyToolTerminalEvent(run, {
    opId: "tool:/a",
    gen: 4,
    event: TOOL_COMPLETED_EVENT,
  });
  assert.equal(next.finished, true);
  assert.equal(next.gen, 4);
  assert.equal(next.outcome, "success");
});

test("a finished run never flips back to running", () => {
  const run = { opId: "tool:/a", gen: 2, finished: true, outcome: "success" };
  const next = applyToolTerminalEvent(run, {
    opId: "tool:/a",
    gen: 2,
    event: TOOL_FAILED_EVENT,
  });
  assert.equal(next.finished, true);
  assert.equal(next.outcome, "success");
});

test("terminal event for another op is ignored", () => {  const run = { opId: "tool:/a", gen: 2, finished: false, outcome: "" };
  const next = applyToolTerminalEvent(run, {
    opId: "tool:/b",
    gen: 2,
    event: TOOL_COMPLETED_EVENT,
  });
  assert.equal(next.finished, false);
});

test("terminal outcome names the event", () => {
  assert.equal(toolTerminalOutcome(TOOL_COMPLETED_EVENT), "success");
  assert.equal(toolTerminalOutcome(TOOL_FAILED_EVENT), "failed");
});

test("output batch appends lines for the current op only", () => {
  const lines = applyToolOutputBatch(["a"], { opId: "tool:/a", text: "b\nc" }, "tool:/a");
  assert.deepEqual(lines, ["a", "b", "c"]);
  const untouched = applyToolOutputBatch(["a"], { opId: "tool:/b", text: "x" }, "tool:/a");
  assert.deepEqual(untouched, ["a"]);
});

test("output buffer is capped", () => {
  assert.ok(maxToolOutputLines > 0);
  const big = Array.from({ length: maxToolOutputLines + 5 }, (_, i) => `l${i}`);
  const capped = applyToolOutputBatch(big, { opId: "tool:/a", text: "new" }, "tool:/a");
  assert.equal(capped.length, maxToolOutputLines);
  assert.equal(capped[capped.length - 1], "new");
});

test("tool event names match the backend contract", () => {
  assert.equal(TOOL_OUTPUT_EVENT, "tool:output");
  assert.equal(TOOL_COMPLETED_EVENT, "tool:completed");
  assert.equal(TOOL_FAILED_EVENT, "tool:failed");
});

test("bridge routes the four tool calls to ToolService", () => {
  assert.deepEqual(ROUTES.ListToolPresets, ["ToolService", "ListToolPresets"]);
  assert.deepEqual(ROUTES.RunTool, ["ToolService", "RunTool"]);
  assert.deepEqual(ROUTES.CancelToolRun, ["ToolService", "CancelToolRun"]);
  assert.deepEqual(ROUTES.GetToolHistory, ["ToolService", "GetToolHistory"]);
});

test("bridge exposes the four tool methods", () => {
  for (const name of ["listToolPresets", "runTool", "cancelToolRun", "getToolHistory"]) {
    assert.equal(
      typeof desktopBridge[name],
      "function",
      `desktopBridge.${name} must exist`,
    );
  }
});

test("bridge runTool sends preset ID plus args map, never argv", async () => {
  const calls = [];
  const restore = __setBackendForTest(async (service, method, args) => {
    calls.push([service, method, args]);
    return "tool:/a";
  });
  try {
    const opId = await desktopBridge.runTool("sample-project", "laravel-migrate", {
      database: "forge",
    });
    assert.equal(opId, "tool:/a");
    assert.deepEqual(calls, [
      ["ToolService", "RunTool", ["sample-project", "laravel-migrate", { database: "forge" }]],
    ]);
    const sentArgs = calls[0][2];
    for (const arg of sentArgs) {
      if (arg && typeof arg === "object") {
        assert.ok(!("argv" in arg), "bridge must never send argv");
        assert.ok(!("command" in arg), "bridge must never send a command line");
      }
    }
  } finally {
    restore();
  }
});

test("the palette island renders the picker, args, history, and output pane", async () => {
  const island = await readFile(
    new URL("../../desktop/frontend/islands/ToolPalette.tsx", import.meta.url),
    "utf8",
  );
  for (const id of [
    'data-testid="tool-palette"',
    'data-testid="tool-preset-picker"',
    'data-testid="tool-run"',
    'data-testid="tool-cancel"',
    'data-testid="tool-output-pane"',
    'data-testid="tool-history"',
  ]) {
    assert.ok(island.includes(id), `missing ${id}`);
  }
});

test("the palette island goes through the bridge and the tool events", async () => {
  const island = await readFile(
    new URL("../../desktop/frontend/islands/ToolPalette.tsx", import.meta.url),
    "utf8",
  );
  for (const token of [
    "listToolPresets",
    "runTool",
    "cancelToolRun",
    "getToolHistory",
    "tool:output",
    "tool:completed",
    "tool:failed",
  ]) {
    assert.ok(island.includes(token), `island never touches ${token}`);
  }
});

test("the palette island attributes terminal events by generation", async () => {
  const island = await readFile(
    new URL("../../desktop/frontend/islands/ToolPalette.tsx", import.meta.url),
    "utf8",
  );
  assert.ok(
    island.includes("applyToolTerminalEvent"),
    "terminal events must go through the (op, gen) attribution helper",
  );
  assert.ok(
    island.includes("currentToolGeneration"),
    "the island must learn the current generation before applying terminal events",
  );
});

test("the palette island guides the no-project state and hands back its API", async () => {
  const island = await readFile(
    new URL("../../desktop/frontend/islands/ToolPalette.tsx", import.meta.url),
    "utf8",
  );
  assert.ok(
    island.includes("NO_PROJECT_GUIDANCE") || island.includes("Select a project"),
    "no-project state must guide toward selecting a project",
  );
  assert.ok(
    island.includes("registerApi(null)"),
    "the island must hand its API back on unmount",
  );
});

test("the palette island never mentions argv", async () => {
  const island = await readFile(
    new URL("../../desktop/frontend/islands/ToolPalette.tsx", import.meta.url),
    "utf8",
  );
  assert.ok(!island.includes("argv"), "the palette must never send argv");
});
