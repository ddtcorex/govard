import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  localEnvironmentURL,
  normalizeDashboardPayload,
  renderProjectHero,
} from "../../desktop/frontend/modules/dashboard.js";

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
  assert.equal(
    html.includes("Edit .env"),
    false,
    "legacy Edit .env action should not be rendered",
  );
});

test("footer exposes system metrics refresh control", async () => {
  const html = await readFile(
    new URL("../../desktop/frontend/index.html", import.meta.url),
    "utf8",
  );
  assert.equal(
    html.includes('data-action="refresh-metrics"'),
    true,
    "missing refresh-metrics action in footer",
  );
  assert.equal(
    html.includes('id="footerCPU"'),
    true,
    "missing footer CPU metric field",
  );
  assert.equal(
    html.includes('id="footerMemory"'),
    true,
    "missing footer memory metric field",
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

test("active services cards expose service actions and always-visible controls", async () => {
  const dashboardJS = await readFile(
    new URL("../../desktop/frontend/modules/dashboard.js", import.meta.url),
    "utf8",
  );

  assert.equal(
    dashboardJS.includes('data-action="open-service-logs"'),
    true,
    "missing service logs action",
  );
  assert.equal(
    dashboardJS.includes('data-action="open-service-shell"'),
    true,
    "missing service shell action",
  );
  assert.equal(
    dashboardJS.includes("group-hover:opacity-100"),
    false,
    "service controls should not depend on hover visibility",
  );
});

test("project hero uses Start action when environment is not running", async () => {
  const dashboardJS = await readFile(
    new URL("../../desktop/frontend/modules/dashboard.js", import.meta.url),
    "utf8",
  );
  assert.equal(
    dashboardJS.includes('const action = isStopped ? "env-start" : "env-restart";'),
    true,
    "project hero should switch restart action to env-start when stopped",
  );
  assert.equal(
    dashboardJS.includes('const icon = isStopped ? "play_arrow" : "restart_alt";'),
    true,
    "project hero should use play icon for stopped environments",
  );
});

test("localEnvironmentURL resolves local domain to HTTPS URL", () => {
  const url = localEnvironmentURL({
    Domain: "magento2-test-instance.test",
  });
  assert.equal(url, "https://magento2-test-instance.test");
});

test("renderProjectHero unhides and sets local URL under environment title", () => {
  const classValues = new Set(["hidden"]);
  const classList = {
    add: (...tokens) => tokens.forEach((token) => classValues.add(token)),
    remove: (...tokens) => tokens.forEach((token) => classValues.delete(token)),
    contains: (token) => classValues.has(token),
  };

  const refs = {
    projectUrl: {
      href: "#",
      dataset: {},
      classList,
    },
    projectUrlText: {
      textContent: "",
    },
  };

  const previousDocument = globalThis.document;
  globalThis.document = {
    getElementById: () => null,
  };

  try {
    renderProjectHero(
      refs,
      [
        {
          Project: "magento2-test-instance",
          Domain: "magento2-test-instance.test",
          Status: "running",
        },
      ],
      "magento2-test-instance",
    );
  } finally {
    globalThis.document = previousDocument;
  }

  assert.equal(
    refs.projectUrl.href,
    "https://magento2-test-instance.test",
    "expected hero URL to use local environment domain",
  );
  assert.equal(
    refs.projectUrlText.textContent,
    "https://magento2-test-instance.test",
    "expected hero URL text to match local URL",
  );
  assert.equal(
    refs.projectUrl.classList.contains("hidden"),
    false,
    "expected project URL link to be visible",
  );
  assert.equal(
    refs.projectUrl.dataset.action,
    "env-open",
    "expected project URL to trigger env-open action",
  );
  assert.equal(
    refs.projectUrl.dataset.env,
    "magento2-test-instance",
    "expected project URL action to target selected environment",
  );
});
