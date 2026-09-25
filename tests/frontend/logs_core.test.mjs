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

