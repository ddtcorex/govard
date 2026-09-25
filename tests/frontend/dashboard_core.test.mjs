import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  localEnvironmentURL,
  normalizeDashboardPayload,
  serviceTargets,
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

test("serviceTargets prefers env services over stale serviceTargets payload", () => {
  const value = serviceTargets({
    ServiceTargets: ["web", "php", "db", "redis", "rabbitmq"],
    Services: [
      { Name: "Nginx", Target: "web" },
      { Name: "PHP", Target: "php" },
      { Name: "MariaDB", Target: "db" },
      { Name: "Redis", Target: "redis" },
    ],
  });

  assert.deepEqual(value, ["web", "php", "db", "redis"]);
});

test("serviceTargets infers common targets when target field is missing", () => {
  const value = serviceTargets({
    Services: [{ Name: "Nginx" }, { Name: "PHP" }, { Name: "MariaDB" }],
  });

  assert.deepEqual(value, ["web", "php", "db"]);
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

test("sidebar mode switch drives global services panel on the right", async () => {
  const html = await readFile(
    new URL("../../desktop/frontend/index.html", import.meta.url),
    "utf8",
  );
  // The sidebar list moved into an island, so its rows are asserted there; the
  // shell containers it switches between stay in the static markup.
  const island = await readFile(
    new URL("../../desktop/frontend/islands/EnvironmentList.tsx", import.meta.url),
    "utf8",
  );
  for (const id of [
    "tab-global-services",
    "globalServicesList",
    "globalLogOutput",
    "sidebarPanel-environments",
  ]) {
    assert.equal(html.includes(`id="${id}"`), true, `missing ${id}`);
  }
  assert.equal(
    html.includes('id="sidebarPanel-global-services"'),
    false,
    "legacy sidebar global services panel should be removed",
  );
  assert.equal(
    island.includes('data-testid="global-services-row"'),
    true,
    "missing global services sidebar row",
  );
  assert.equal(
    island.includes('"Active Environments"'),
    true,
    "missing active environments section",
  );
  assert.equal(
    island.includes('"Inactive Environments"'),
    true,
    "missing inactive environments section",
  );
  assert.equal(
    island.includes("onSwitchSidebarMode"),
    true,
    "missing sidebar mode switch action",
  );
});
test("inactive environments label uses the same primary styling as active environments", async () => {
  // The list is an island now, so the label contract is asserted on its source:
  // both group headers share one tone class, and the inactive group keeps its
  // extra top margin.
  const island = await readFile(
    new URL("../../desktop/frontend/islands/EnvironmentList.tsx", import.meta.url),
    "utf8",
  );
  assert.equal(
    island.includes("text-primary/70 uppercase tracking-[0.12em]"),
    true,
    "group labels should reuse the primary header styling",
  );
  assert.equal(
    island.includes('mt="mt-8"'),
    true,
    "the inactive group should keep its top margin",
  );
  assert.equal(
    island.includes('title="Active Environments"') &&
      island.includes('title="Inactive Environments"'),
    true,
    "both groups should render through the same component",
  );
});
test("global services right panel includes per-service and log actions", async () => {
  const html = await readFile(
    new URL("../../desktop/frontend/index.html", import.meta.url),
    "utf8",
  );
  for (const id of [
    "globalServiceHealthPercent",
    "globalServiceHealthBar",
    "globalServiceStatusStrip",
    "globalActionFeedback",
    "globalLogSearch",
    "globalLogSeverity",
  ]) {
    assert.equal(
      html.includes(`id="${id}"`),
      true,
      `missing global services deck element ${id}`,
    );
  }
  for (const action of [
    "global-bulk-start",
    "global-bulk-stop",
    "global-bulk-restart",
    "global-bulk-pull",
    "toggle-global-live",
    "refresh-global-logs",
    "clear-global-logs",
    "download-global-logs",
    "filter-global-severity",
  ]) {
    assert.equal(
      html.includes(`data-action="${action}"`),
      true,
      `missing global services action ${action}`,
    );
  }
  for (const loadingLabel of [
    'data-loading-label="Starting All..."',
    'data-loading-label="Restarting All..."',
    'data-loading-label="Stopping All..."',
    'data-loading-label="Pulling All..."',
  ]) {
    assert.equal(
      html.includes(loadingLabel),
      true,
      `missing loading label contract ${loadingLabel}`,
    );
  }
});

test("global service card actions provide loading state contracts", async () => {
  const globalServicesJS = await readFile(
    new URL("../../desktop/frontend/modules/global-services.js", import.meta.url),
    "utf8",
  );

  for (const loadingLabel of [
    'data-loading-label="${primaryAction === "restart" ? "Restarting..." : "Starting..."}"',
    'data-loading-label="Stopping..."',
    'data-loading-label="Opening..."',
  ]) {
    assert.equal(
      globalServicesJS.includes(loadingLabel),
      true,
      `missing per-service loading label ${loadingLabel}`,
    );
  }

  assert.equal(
    globalServicesJS.includes("progress_activity"),
    true,
    "missing loading spinner icon for global service actions",
  );
  assert.equal(
    globalServicesJS.includes('button.setAttribute("aria-busy", "true")'),
    true,
    "missing aria-busy state for loading buttons",
  );
});

test("footer exposes system metrics refresh control", async () => {
  const html = await readFile(
    new URL("../../desktop/frontend/index.html", import.meta.url),
    "utf8",
  );
  const island = await readFile(
    new URL("../../desktop/frontend/islands/MetricsFooter.tsx", import.meta.url),
    "utf8",
  );
  assert.equal(
    html.includes('id="metricsIsland"'),
    true,
    "missing metrics island container in footer",
  );
  assert.equal(
    island.includes('data-testid="refresh-metrics"'),
    true,
    "missing refresh-metrics control in the metrics island",
  );
  assert.equal(
    island.includes('id="footerCPU"'),
    true,
    "missing footer CPU metric field",
  );
  assert.equal(
    island.includes('id="footerMemory"'),
    true,
    "missing footer memory metric field",
  );
});

test("logs section exposes filtering and streaming controls", async () => {
  const html = await readFile(
    new URL("../../desktop/frontend/index.html", import.meta.url),
    "utf8",
  );
  // The tab's markup moved into a React island, so its controls are asserted on
  // the island and by data-testid: the [data-action] routing the global
  // delegate used is gone (spec D5).
  const island = await readFile(
    new URL("../../desktop/frontend/islands/LogsTab.tsx", import.meta.url),
    "utf8",
  );

  assert.equal(html.includes('id="tab-logs"'), true, "missing logs tab");
  assert.equal(
    html.includes('id="logsIsland"'),
    true,
    "missing logs island container",
  );
  assert.equal(
    island.includes("logServiceSelector"),
    true,
    "missing log service selector",
  );
  assert.equal(
    island.includes('data-testid="refresh-logs"'),
    true,
    "missing refresh logs control",
  );
  assert.equal(
    island.includes('data-testid="download-logs"'),
    true,
    "missing download logs control",
  );
  assert.equal(
    island.includes('data-testid="toggle-live"'),
    true,
    "missing toggle live control",
  );
  assert.equal(
    /data-action\s*=/.test(island),
    false,
    "the migrated subtree must carry no data-action for main.js's delegate",
  );
});

test("active services cards expose service actions and always-visible controls", async () => {
  const island = await readFile(
    new URL("../../desktop/frontend/islands/ActiveServices.tsx", import.meta.url),
    "utf8",
  );

  assert.equal(
    island.includes('data-testid="service-logs"'),
    true,
    "missing service logs control",
  );
  assert.equal(
    island.includes('data-testid="service-terminal"'),
    true,
    "missing service terminal control",
  );
  assert.equal(
    island.includes("group-hover:opacity-100"),
    false,
    "service controls should not depend on hover visibility",
  );
});
test("project hero uses Start action when environment is not running", async () => {
  const island = await readFile(
    new URL("../../desktop/frontend/islands/ProjectHero.tsx", import.meta.url),
    "utf8",
  );
  assert.equal(
    island.includes('isStopped ? "env-start" : "env-restart"'),
    true,
    "project hero should switch restart action to env-start when stopped",
  );
  assert.equal(
    island.includes('isStopped ? "play_arrow" : "restart_alt"'),
    true,
    "project hero should use play icon for stopped environments",
  );
  assert.equal(
    island.includes('data-testid="hero-pull"') && island.includes("data-env={project}"),
    true,
    "project hero should bind the pull action to the selected environment",
  );
});
test("project hero exposes pull button contract", async () => {
  const island = await readFile(
    new URL("../../desktop/frontend/islands/ProjectHero.tsx", import.meta.url),
    "utf8",
  );
  assert.equal(island.includes('id="heroPullBtn"'), true, "missing hero pull button");
  assert.equal(
    island.includes('onAction("env-pull", project)'),
    true,
    "hero pull button should trigger env-pull action",
  );
  assert.equal(island.includes('id="projectUrl"'), true, "missing project URL link");
  assert.equal(
    island.includes(
      "mb-2 text-emerald-700 dark:text-primary hover:text-emerald-800 dark:hover:text-primary/80 transition-colors text-sm font-bold flex items-center gap-1 leading-none",
    ),
    true,
    "project URL link should keep bottom spacing before technology badges",
  );
});
test("localEnvironmentURL resolves local domain to HTTPS URL", () => {
  const url = localEnvironmentURL({
    Domain: "sample-project.test",
  });
  assert.equal(url, "https://sample-project.test");
});

test("the hero hides the project URL link when the environment has no URL", async () => {
  const island = await readFile(
    new URL("../../desktop/frontend/islands/ProjectHero.tsx", import.meta.url),
    "utf8",
  );
  assert.equal(
    island.includes("localEnvironmentURL(env)"),
    true,
    "the hero should resolve the local URL from the environment",
  );
  assert.equal(
    island.includes('href={url || "#"}'),
    true,
    "the URL link should fall back to a dead href when the environment has none",
  );
  assert.equal(
    island.includes('${url ? "" : " hidden"}'),
    true,
    "the URL link should only be hidden when there is no URL",
  );
});

