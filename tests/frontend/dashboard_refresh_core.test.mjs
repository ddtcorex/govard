import test from "node:test";
import assert from "node:assert/strict";

import { createDashboardRefresher } from "../../desktop/frontend/modules/dashboard-refresh.js";

// Runtime behavioral tests for the dashboard refresh orchestration. The
// source-grep tests in dashboard_core.test.mjs pin the shape; these prove the
// behavior with controllable deferred promises: concurrency means every panel
// is in flight before any of them resolves, and isolation means one panel's
// rejection still leaves the other four plus the dashboard state rendered.
// Either property must fail if the implementation reverts to sequential
// awaits or to a single try/catch that blanks the dashboard.

const deferred = () => {
  let resolve;
  let reject;
  const promise = new Promise((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
};

const flush = async (rounds = 5) => {
  for (let i = 0; i < rounds; i += 1) {
    await new Promise((r) => setTimeout(r, 0));
  }
};

const PANEL_NAMES = [
  "refreshMetrics",
  "refreshRemotes",
  "refreshGlobalServices",
  "refreshLogs",
  "loadFooterVersion",
];

const setup = (overrides = {}) => {
  const state = {
    environments: [{ Name: "stale-project", Status: "running" }],
    selectedProject: "stale-project",
    envListLoading: true,
    dashboardError: "",
    lastRefreshed: 0,
  };
  const statuses = [];
  const skeletons = [];
  const panels = {};
  for (const name of PANEL_NAMES) {
    panels[name] = { started: false, gate: deferred() };
  }
  const panelFn = (name) => {
    const entry = panels[name];
    return async () => {
      entry.started = true;
      return entry.gate.promise;
    };
  };
  const refreshDashboard = createDashboardRefresher({
    loadDashboard: async () => ({
      environments: [{ Name: "sample-project", Status: "running" }],
    }),
    getState: () => state,
    setState: (patch) => Object.assign(state, patch || {}),
    projectKey: (env = {}) => env.Project || env.project || env.Name || env.name || "",
    refreshServiceSelector: () => {},
    renderAllSkeletons: () => {
      skeletons.push(true);
    },
    setStatus: (message) => {
      statuses.push(message);
    },
    panels: {
      refreshMetrics: panelFn("refreshMetrics"),
      refreshRemotes: panelFn("refreshRemotes"),
      refreshGlobalServices: panelFn("refreshGlobalServices"),
      refreshLogs: panelFn("refreshLogs"),
      loadFooterVersion: panelFn("loadFooterVersion"),
    },
    now: () => 1729000000000,
    ...overrides,
  });
  return { state, statuses, skeletons, panels, refreshDashboard };
};

test("all five panel refreshes are in flight concurrently", async () => {
  const { panels, refreshDashboard } = setup();
  const pending = refreshDashboard({ silent: true });
  await flush();

  // Every panel must have STARTED while every gate is still closed: with
  // sequential awaits only the first panel could have started by now.
  for (const name of PANEL_NAMES) {
    assert.equal(
      panels[name].started,
      true,
      `${name} must be in flight before any panel resolves`,
    );
  }

  for (const name of PANEL_NAMES) {
    panels[name].gate.resolve(null);
  }
  await pending;
});

test("one rejecting panel still leaves the other four and the dashboard rendered", async () => {
  const { state, statuses, panels, refreshDashboard } = setup();
  panels.refreshRemotes.gate.reject(new Error("remotes backend down"));
  for (const name of PANEL_NAMES) {
    if (name !== "refreshRemotes") {
      panels[name].gate.resolve(null);
    }
  }
  await refreshDashboard({ silent: true });

  for (const name of PANEL_NAMES) {
    assert.equal(panels[name].started, true, `${name} must still have run`);
  }
  assert.equal(state.environments.length, 1);
  assert.equal(state.environments[0].Name, "sample-project");
  assert.equal(state.dashboardError, "");
  assert.equal(state.lastRefreshed, 1729000000000);
  assert.equal(statuses.at(-1), "Status: Ready");
});

test("a failing dashboard fetch keeps real state and still refreshes every panel", async () => {
  const failure = new Error("Desktop bridge not available");
  const { state, statuses, panels, refreshDashboard } = setup({
    loadDashboard: async () => {
      throw failure;
    },
  });
  for (const name of PANEL_NAMES) {
    panels[name].gate.resolve(null);
  }
  await refreshDashboard({ silent: true });

  // Never fake projects, never blank the screen: the stale real environments
  // stay untouched and the error is published for the offline state.
  assert.equal(state.environments.length, 1);
  assert.equal(state.environments[0].Name, "stale-project");
  assert.equal(state.dashboardError, failure.message);
  assert.equal(state.envListLoading, false);
  assert.equal(state.lastRefreshed, 0);
  for (const name of PANEL_NAMES) {
    assert.equal(panels[name].started, true, `${name} must still have run`);
  }
  assert.equal(statuses.at(-1), "Status: Error");
});
