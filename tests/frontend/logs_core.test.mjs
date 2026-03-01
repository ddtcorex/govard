import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  classifyLogSeverity,
  filterLogsText,
  resolveLogTarget,
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

test("logs tab exposes shell controls contract", async () => {
  const html = await readFile(
    new URL("../../desktop/frontend/index.html", import.meta.url),
    "utf8",
  );
  const logsJS = await readFile(
    new URL("../../desktop/frontend/modules/logs.js", import.meta.url),
    "utf8",
  );
  const combined = html + logsJS;

  for (const id of ["shellUser", "shellCommand"]) {
    assert.equal(
      combined.includes(`id="${id}"`),
      true,
      `missing shell control ${id}`,
    );
  }
  assert.equal(
    combined.includes('data-action="start-embedded-terminal"'),
    true,
    "missing shell action start-embedded-terminal",
  );
  assert.equal(
    combined.includes('data-action="toggle-terminal-modal"'),
    true,
    "missing shell action toggle-terminal-modal",
  );
  assert.equal(
    combined.includes('data-action="reset-shell-users"'),
    false,
    "shell reset/settings action should not be rendered",
  );
});
