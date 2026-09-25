import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  localEnvironmentURL,
  normalizeDashboardPayload,
  renderEnvironmentList,
  renderProjectHero,
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
  const dashboardJS = await readFile(
    new URL("../../desktop/frontend/modules/dashboard.js", import.meta.url),
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
    dashboardJS.includes('data-mode="global-services"'),
    true,
    "missing global services sidebar row action",
  );
  assert.equal(
    dashboardJS.includes('"Active Environments",'),
    true,
    "missing active environments section",
  );
  assert.equal(
    dashboardJS.includes('"Inactive Environments",'),
    true,
    "missing inactive environments section",
  );
  assert.equal(
    dashboardJS.includes('data-action="switch-sidebar-mode"'),
    true,
    "missing sidebar mode switch action",
  );
});

test("inactive environments label uses the same primary styling as active environments", () => {
  const container = { innerHTML: "" };

  renderEnvironmentList(
    container,
    [
      {
        Project: "m2govard",
        Domain: "m2govard.test",
        Status: "running",
        Services: [{ Name: "Nginx" }],
      },
      {
        Project: "govard",
        Domain: "govard",
        Status: "stopped",
        Services: [],
      },
    ],
    "m2govard",
    { sidebarMode: "environments" },
  );

  assert.equal(
    container.innerHTML.includes(
      '<div class="px-1 mt-8 pb-2 text-[10px] font-bold text-primary/70 uppercase tracking-[0.12em]">Inactive Environments</div>',
    ),
    true,
    "inactive environments label should reuse the primary header styling",
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
    dashboardJS.includes('data-action="start-service-terminal-os"'),
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
  assert.equal(
    dashboardJS.includes("refs.heroPullBtn.dataset.env = selectedProject;"),
    true,
    "project hero should bind pull action to selected environment",
  );
});

test("project hero exposes pull button contract", async () => {
  const html = await readFile(
    new URL("../../desktop/frontend/index.html", import.meta.url),
    "utf8",
  );
  assert.equal(
    html.includes('id="heroPullBtn"'),
    true,
    "missing hero pull button",
  );
  assert.equal(
    html.includes('data-action="env-pull"'),
    true,
    "hero pull button should trigger env-pull action",
  );
  assert.equal(
    html.includes('id="projectUrl"'),
    true,
    "missing project URL link",
  );
  assert.equal(
    html.includes(
      'class="mb-2 text-emerald-700 dark:text-primary hover:text-emerald-800 dark:hover:text-primary/80 transition-colors text-sm font-bold flex items-center gap-1 leading-none"',
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
          Project: "sample-project",
          Domain: "sample-project.test",
          Status: "running",
        },
      ],
      "sample-project",
    );
  } finally {
    globalThis.document = previousDocument;
  }

  assert.equal(
    refs.projectUrl.href,
    "https://sample-project.test",
    "expected hero URL to use local environment domain",
  );
  assert.equal(
    refs.projectUrlText.textContent,
    "https://sample-project.test",
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
    "sample-project",
    "expected project URL action to target selected environment",
  );
});
