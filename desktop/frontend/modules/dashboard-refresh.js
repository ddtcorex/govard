/**
 * Dashboard refresh orchestration, dependency-injected so node tests can drive
 * it with deferred promises (see tests/frontend/dashboard_refresh_core.test.mjs).
 * main.js wires the production collaborators; the logic lives here, in one
 * place, so a revert to sequential awaits or to a blanking catch breaks the
 * runtime tests, not just a source grep.
 *
 * @param {object} deps
 * @param {() => Promise<{ environments: Array<any> }>} deps.loadDashboard
 *   Normalized dashboard payload; rejects when the bridge is unreachable.
 * @param {() => any} deps.getState
 * @param {(patch: Record<string, any>) => any} deps.setState
 * @param {(env: any) => string} deps.projectKey
 * @param {() => void} deps.refreshServiceSelector
 * @param {() => void} deps.renderAllSkeletons
 * @param {(message: string) => void} deps.setStatus
 * @param {object} deps.panels The five follow-up refreshes, each read through
 *   a closure by the caller when islands can re-register them after mount.
 * @param {(opts: { silent: boolean }) => Promise<any>} deps.panels.refreshMetrics
 * @param {(opts: { silent: boolean }) => Promise<any>} deps.panels.refreshRemotes
 * @param {(opts: { silent: boolean }) => Promise<any>} deps.panels.refreshGlobalServices
 * @param {() => Promise<any>} deps.panels.refreshLogs
 * @param {() => Promise<any>} deps.panels.loadFooterVersion
 * @param {() => number} [deps.now]
 */
export const createDashboardRefresher = ({
  loadDashboard,
  getState,
  setState,
  projectKey,
  refreshServiceSelector,
  renderAllSkeletons,
  setStatus,
  panels,
  now = () => Date.now(),
}) => {
  const refreshDashboard = async (options = {}) => {
    try {
      setStatus("Status: syncing dashboard...");
      if (!options.silent) {
        renderAllSkeletons();
      }

      console.log("[refreshDashboard] Fetching dashboard...");
      // A failed fetch must never render fake projects: the error is published
      // to the store, and the hero and the sidebar list render their offline
      // state (with its own Retry) when they have nothing real to show. Whatever
      // the store already holds stays on screen untouched.
      let dashboard = null;
      let dashboardError = "";
      try {
        dashboard = await loadDashboard();
        console.log(
          "[refreshDashboard] Got dashboard, environments:",
          dashboard.environments?.length,
        );
      } catch (err) {
        dashboardError = err instanceof Error ? err.message : String(err);
        console.error("[refreshDashboard] dashboard fetch failed:", err);
      }

      // The sidebar list, the hero, the service cards and the env vars all read
      // this store now (spec D6), so publishing the environments is what renders
      // them - there is nothing left to push into the DOM from here.
      if (dashboard) {
        const selectedProject = getState().selectedProject || "";
        // The list has its data now, so the loading frame is over either way: a
        // silent refresh never showed one, and this is what closes the non-silent one
        // and the boot frame the store starts with.
        setState({
          environments: dashboard.environments,
          selectedProject,
          envListLoading: false,
          dashboardError: "",
          lastRefreshed: now(),
        });
        if (!selectedProject && dashboard.environments.length > 0) {
          setState({ selectedProject: projectKey(dashboard.environments[0]) });
        }

        try {
          refreshServiceSelector();
        } catch (e) {
          console.error("[refreshDashboard] refreshServiceSelector error:", e);
        }
      } else {
        setState({ envListLoading: false, dashboardError });
      }

      // Every panel refreshes concurrently: one failing panel settles rejected
      // while the others still render, so a single failure never blanks the rest.
      await Promise.allSettled([
        panels.refreshMetrics({ silent: true }),
        panels.refreshRemotes({ silent: true }),
        panels.refreshGlobalServices({ silent: true }),
        panels.refreshLogs(),
        panels.loadFooterVersion(),
      ]);

      setStatus(dashboardError ? `Status: Error` : `Status: Ready`);
    } catch (e) {
      console.error("[refreshDashboard] error:", e);
      setStatus(`Status: Error`);
    }
  };

  return refreshDashboard;
};
