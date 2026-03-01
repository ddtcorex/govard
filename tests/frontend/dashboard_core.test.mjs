import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import { normalizeDashboardPayload } from "../../desktop/frontend/modules/dashboard.js";

test("normalizeDashboardPayload maps core values", () => {
  const value = normalizeDashboardPayload({
    ActiveEnvironments: 2,
    RunningServices: 9,
    QueuedTasks: 1,
    ActiveSummary: "demo",
    Environments: [{ Name: "demo.test" }],
    Warnings: ["warn"],
  });

  assert.equal(value.active, 2);
  assert.equal(value.services, 9);
  assert.equal(value.queued, 1);
  assert.equal(value.activeSummary, "demo");
  assert.equal(value.environments.length, 1);
  assert.equal(value.warnings.length, 1);
});

test("normalizeDashboardPayload uses safe defaults", () => {
  const value = normalizeDashboardPayload({});
  assert.equal(value.active, 0);
  assert.equal(value.services, 0);
  assert.equal(value.queued, 0);
  assert.deepEqual(value.environments, []);
  assert.deepEqual(value.warnings, []);
});

test("core layout excludes removed heavyweight sections", async () => {
  const html = await readFile(
    new URL("../../desktop/frontend/index.html", import.meta.url),
    "utf8",
  );
  for (const id of [
    "operations",
    "onboardingChecks",
    "workflowSync",
    "activityList",
  ]) {
    assert.equal(
      html.includes(`id="${id}"`),
      false,
      `expected removed section ${id}`,
    );
  }
});

test("quick actions exposes desktop action contracts", async () => {
  const html = await readFile(
    new URL("../../desktop/frontend/index.html", import.meta.url),
    "utf8",
  );
  for (const action of ["open-folder", "open-ide", "open-db-client"]) {
    assert.equal(
      html.includes(`data-action="${action}"`),
      true,
      `missing quick action ${action}`,
    );
  }
  assert.equal(
    html.includes('data-action="open-mail-client"'),
    true,
    "quick action open-mail-client should be present",
  );
});

test("project management workspace keeps core IDs for controllers", async () => {
  const html = await readFile(
    new URL("../../desktop/frontend/index.html", import.meta.url),
    "utf8",
  );
  assert.equal(
    html.includes('id="envList"'),
    true,
    "missing environment list container",
  );
  assert.equal(
    html.includes('id="metricsList"'),
    false,
    "metrics list container should be removed from dashboard tab",
  );
  assert.equal(
    html.includes('id="onboardingModalMount"'),
    true,
    "missing onboarding mount point",
  );
  assert.equal(
    html.includes('data-action="open-onboarding"'),
    true,
    "missing onboarding open action",
  );
});

test("logs section exposes filtering and streaming controls", async () => {
  const html = await readFile(
    new URL("../../desktop/frontend/index.html", import.meta.url),
    "utf8",
  );
  const logsJS = await readFile(
    new URL("../../desktop/frontend/modules/logs.js", import.meta.url),
    "utf8",
  );
  const combined = html + logsJS;

  assert.equal(combined.includes('id="tab-logs"'), true, "missing logs tab");
  assert.equal(
    combined.includes('id="logServiceSelector"'),
    true,
    "missing log service selector",
  );
  assert.equal(
    combined.includes('data-action="refresh-logs"'),
    true,
    "missing refresh logs action",
  );
  assert.equal(
    combined.includes('data-action="toggle-live"'),
    true,
    "missing toggle live action",
  );
});
