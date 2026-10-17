import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  classifyLogSeverity,
  filterLogsText,
  resolveLogTarget,
  resolveServiceTargets,
} from "../../desktop/frontend/modules/logs.js";

test("resolveLogTarget returns selected project and service", () => {
  const value = resolveLogTarget({
    project: "demo",
    service: "php",
  });
  assert.equal(value.project, "demo");
  assert.equal(value.service, "php");
});

test("resolveLogTarget applies defaults", () => {
  const value = resolveLogTarget({});
  assert.equal(value.project, "");
  assert.equal(value.service, "all");
  assert.equal(value.severity, "all");
  assert.equal(value.query, "");
});

test("classifyLogSeverity detects severity keywords", () => {
  assert.equal(classifyLogSeverity("Fatal: exception occurred"), "error");
  assert.equal(classifyLogSeverity("WARN cache warmup is slow"), "warn");
  assert.equal(classifyLogSeverity("Info: queue drained"), "info");
  assert.equal(classifyLogSeverity("just a regular line"), "info");
});

test("filterLogsText returns unfiltered logs when filters are default", () => {
  const logs = [
    "Info service started",
    "WARN cache slow",
    "Fatal exception",
  ].join("\n");
  assert.equal(filterLogsText(logs, "all", ""), logs);
});

test("filterLogsText filters by severity and search query", () => {
  const logs = [
    "Info worker ready",
    "WARN cache slow",
    "Fatal: php error",
    "Info retry success",
  ].join("\n");

  assert.equal(filterLogsText(logs, "warn", ""), "WARN cache slow");
  assert.equal(filterLogsText(logs, "error", ""), "Fatal: php error");
  assert.equal(filterLogsText(logs, "all", "retry"), "Info retry success");
  assert.equal(filterLogsText(logs, "info", "worker"), "Info worker ready");
  assert.equal(filterLogsText(logs, "error", "worker"), "");
});

test("resolveServiceTargets lists all plus the environment's targets", () => {
  const environments = [
    {
      project: "sample-project",
      services: [{ name: "nginx" }, { name: "php" }],
    },
  ];
  const { targets, service } = resolveServiceTargets(environments, "sample-project", "all");
  assert.deepEqual(targets, ["all", "web", "php"]);
  assert.equal(service, "all");
});

test("resolveServiceTargets keeps a selection the environment still offers", () => {
  const environments = [{ project: "sample-project", serviceTargets: ["php", "db"] }];
  const { targets, service } = resolveServiceTargets(environments, "sample-project", "db");
  assert.deepEqual(targets, ["all", "php", "db"]);
  assert.equal(service, "db");
});

test("resolveServiceTargets falls back to all for an unknown project", () => {
  const { targets, service } = resolveServiceTargets([], "missing-project", "php");
  assert.deepEqual(targets, ["all", "web"]);
  assert.equal(service, "all");
});

test("resolveServiceTargets drops a selection the environment no longer offers", () => {
  const environments = [{ project: "sample-project", serviceTargets: ["php"] }];
  const { service } = resolveServiceTargets(environments, "sample-project", "redis");
  assert.equal(service, "all");
});


test("appendCappedLogText keeps the last N lines", async () => {
  const { appendCappedLogText, maxLogLines } = await import(
    "../../desktop/frontend/modules/logs.js"
  );
  assert.equal(maxLogLines, 2000);
  assert.equal(appendCappedLogText("", "a\nb", 2), "a\nb");
  assert.equal(appendCappedLogText("a\nb", "c", 2), "b\nc");
  assert.equal(appendCappedLogText("a", "", 2), "a");
  assert.equal(appendCappedLogText("", "", 2), "");
  // Default cap applies without an explicit max.
  const big = Array.from({ length: maxLogLines + 5 }, (_, i) => `l${i}`).join("\n");
  assert.equal(appendCappedLogText("", big).split("\n").length, maxLogLines);
});

test("appendCappedLogLines keeps the last N entries", async () => {
  const { appendCappedLogLines, maxLogLines } = await import(
    "../../desktop/frontend/modules/logs.js"
  );
  assert.deepEqual(appendCappedLogLines(["a", "b"], "c", 2), ["b", "c"]);
  assert.deepEqual(appendCappedLogLines([], "a", 2), ["a"]);
  const big = Array.from({ length: maxLogLines + 5 }, (_, i) => `l${i}`);
  assert.equal(appendCappedLogLines(big, "x").length, maxLogLines);
  assert.equal(appendCappedLogLines(big, "x")[maxLogLines - 1], "x");
  assert.deepEqual(appendCappedLogLines(["a"], "b", 0), []);
});

test("isViewportAtBottom tells a pinned pane from a scrolled-up one", async () => {
  const { isViewportAtBottom } = await import(
    "../../desktop/frontend/modules/logs.js"
  );
  assert.equal(
    typeof isViewportAtBottom,
    "function",
    "the autoscroll gate must be an exported helper",
  );
  // Pinned to the bottom: no unread lines.
  assert.equal(
    isViewportAtBottom({ scrollTop: 976, scrollHeight: 1076, clientHeight: 100 }),
    true,
    "a pane within tolerance of the bottom must read as pinned",
  );
  assert.equal(
    isViewportAtBottom({ scrollTop: 1000, scrollHeight: 1100, clientHeight: 100 }),
    true,
    "an exactly-bottom pane must read as pinned",
  );
  // Scrolled up: new lines must pile into the pill, not yank the pane.
  assert.equal(
    isViewportAtBottom({ scrollTop: 400, scrollHeight: 1076, clientHeight: 100 }),
    false,
    "a pane far from the bottom must read as scrolled up",
  );
  assert.equal(
    isViewportAtBottom({ scrollTop: 0, scrollHeight: 5000, clientHeight: 100 }),
    false,
    "a pane at the top must read as scrolled up",
  );
});

test("shouldStickToBottom needs the toggle on and the pane pinned", async () => {
  const { shouldStickToBottom } = await import(
    "../../desktop/frontend/modules/logs.js"
  );
  assert.equal(
    typeof shouldStickToBottom,
    "function",
    "the stick decision must be an exported helper",
  );
  assert.equal(
    shouldStickToBottom({ autoscroll: true, atBottom: true }),
    true,
    "toggle on plus pinned must stick",
  );
  assert.equal(
    shouldStickToBottom({ autoscroll: true, atBottom: false }),
    false,
    "a scrolled-up pane must not yank even with the toggle on",
  );
  assert.equal(
    shouldStickToBottom({ autoscroll: false, atBottom: true }),
    false,
    "an opted-out toggle must not stick even when pinned",
  );
  assert.equal(
    shouldStickToBottom({ autoscroll: false, atBottom: false }),
    false,
    "an opted-out scrolled-up pane must not stick",
  );
});

test("formatPendingLines renders the pill copy", async () => {
  const { formatPendingLines } = await import(
    "../../desktop/frontend/modules/logs.js"
  );
  assert.equal(
    typeof formatPendingLines,
    "function",
    "the pill copy must come from an exported helper",
  );
  assert.equal(formatPendingLines(1), "1 new line");
  assert.equal(formatPendingLines(3), "3 new lines");
  assert.equal(formatPendingLines(2000), "2000 new lines");
});

test("liveModeLabel names the stream-vs-poll state", async () => {
  const { liveModeLabel } = await import(
    "../../desktop/frontend/modules/logs.js"
  );
  assert.equal(
    typeof liveModeLabel,
    "function",
    "the Live button copy must come from an exported helper",
  );
  assert.equal(liveModeLabel("off"), "Live: Off");
  assert.equal(liveModeLabel("stream"), "Live: Stream");
  assert.equal(liveModeLabel("poll"), "Live: Poll");
});

test("shouldStopEnvLiveOnTabSwitch stops env live only when leaving logs", async () => {
  const { shouldStopEnvLiveOnTabSwitch } = await import(
    "../../desktop/frontend/modules/logs.js"
  );
  assert.equal(
    typeof shouldStopEnvLiveOnTabSwitch,
    "function",
    "the tab-hide stop decision must be an exported helper",
  );
  assert.equal(
    shouldStopEnvLiveOnTabSwitch({ nextTab: "dashboard", activeTab: "tab-logs" }),
    true,
    "leaving the logs tab must stop env live",
  );
  assert.equal(
    shouldStopEnvLiveOnTabSwitch({ nextTab: "remotes", activeTab: "tab-logs" }),
    true,
    "leaving the logs tab for remotes must stop env live",
  );
  assert.equal(
    shouldStopEnvLiveOnTabSwitch({ nextTab: "logs", activeTab: "tab-logs" }),
    false,
    "staying on the logs tab must not stop env live",
  );
  assert.equal(
    shouldStopEnvLiveOnTabSwitch({ nextTab: "dashboard", activeTab: "tab-dashboard" }),
    false,
    "a switch that never showed logs must not stop anything",
  );
});

test("the env logs island wires autoscroll, the pill, and the stream/poll label", async () => {
  const island = await readFile(
    new URL("../../desktop/frontend/islands/LogsTab.tsx", import.meta.url),
    "utf8",
  );
  assert.equal(
    island.includes('data-testid="autoscroll-toggle"'),
    true,
    "missing the autoscroll toggle control",
  );
  assert.equal(
    island.includes('data-testid="new-lines-pill"'),
    true,
    'missing the "N new lines" pill',
  );
  assert.equal(
    island.includes("data-live-mode"),
    true,
    "the Live button must expose the stream/poll mode for tests",
  );
  assert.equal(
    island.includes("liveModeLabel"),
    true,
    "the Live button copy must go through the stream/poll helper",
  );
  assert.equal(
    island.includes("shouldStickToBottom") || island.includes("isViewportAtBottom"),
    true,
    "the scroll effect must go through the autoscroll helpers, not an unconditional yank",
  );
  assert.equal(
    island.includes("stopLive"),
    true,
    "the island must own a stopLive main.js can call on tab hide",
  );
  assert.equal(
    island.includes("registerController({ refresh: load, selectionChanged, stopLive })") ||
      (island.includes("stopLive") && island.includes("registerController")),
    true,
    "the registered controller must expose stopLive so tab hide stops the backend stream",
  );
});

test("the global logs island wires autoscroll, the pill, and the stream/poll label", async () => {
  const island = await readFile(
    new URL("../../desktop/frontend/islands/GlobalLogsPanel.tsx", import.meta.url),
    "utf8",
  );
  assert.equal(
    island.includes('data-testid="autoscroll-toggle"'),
    true,
    "missing the autoscroll toggle control",
  );
  assert.equal(
    island.includes('data-testid="new-lines-pill"'),
    true,
    'missing the "N new lines" pill',
  );
  assert.equal(
    island.includes("data-live-mode"),
    true,
    "the Live button must expose the stream/poll mode for tests",
  );
  assert.equal(
    island.includes("stopGlobalServiceLogStream"),
    true,
    "hiding the pane must stop the backend global stream",
  );
});

test("leaving the logs tab stops env live, including the backend stream", async () => {
  const main = await readFile(
    new URL("../../desktop/frontend/main.js", import.meta.url),
    "utf8",
  );
  assert.equal(
    main.includes("shouldStopEnvLiveOnTabSwitch"),
    true,
    "the tab switch must go through the stop-on-hide helper",
  );
  assert.equal(
    main.includes("logsApi.stopLive"),
    true,
    "leaving the logs tab must call the island stopLive so the backend stream dies with it",
  );
});
