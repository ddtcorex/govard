import { confirm } from "../ui/modal.js";
import { escapeHTML } from "../utils/dom.js";
import { beginOperation, endOperation } from "./operations.js";

// The backend owns per-operation timeouts (up to 15 minutes for `up`);
// this is only a hard backstop so a lost RPC can never hang the UI.
// It must stay above the backend maximum, and its message must defer to
// the backend's instead of inventing a second timeout story.
export const ACTION_BACKSTOP_MS = 16 * 60 * 1000;

export const ACTION_BACKSTOP_MESSAGE =
  "The backend stopped responding after 16 minutes; the operation may have been interrupted. Check the dashboard for partial state.";

export const buildDeleteConfirmMessage = (project) =>
  `Are you sure you want to PERMANENTLY delete project <span class="text-primary font-bold">"${escapeHTML(project)}"</span>?<br><br>
                  This will remove all Docker containers and <span class="text-red-500 font-bold uppercase underline">VOLUMES</span> (database data).<br><br>
                  The project source code directory will <span class="font-bold">NOT</span> be deleted.<br><br>
                  <span class="text-red-500 font-bold">THIS ACTION CANNOT BE UNDONE.</span>`;

export const createActionsController = ({
  bridge,
  getProject,
  refreshDashboard,
  renderSkeletons,
  onStatus,
  onToast,
  onToastLoading,
}) => {
  const MIN_LOADING_TOAST_MS = 700;

  const runEnvironmentAction = async (
    fn,
    project,
    fallbackMessage,
    loadingLabel = "Processing environment...",
    opAction = "",
  ) => {
    if (!project) {
      onStatus("Please select an environment first.");
      return;
    }
    // One in-flight operation per action and project: a double-click or a
    // Start-then-Stop race reports instead of running `up` twice.
    let opToken = null;
    if (opAction) {
      opToken = beginOperation(`${opAction}:${project}`);
      if (opToken === null) {
        onStatus(`${opAction} is already running for ${project}.`);
        return;
      }
    }
    let loadingToast = null;
    let loadingStartedAt = 0;
    let backstopTimer = null;
    const waitForToastVisibility = async () => {
      if (!loadingToast || loadingStartedAt <= 0) {
        return;
      }
      const elapsed = Date.now() - loadingStartedAt;
      const remaining = Math.max(0, MIN_LOADING_TOAST_MS - elapsed);
      if (remaining > 0) {
        await new Promise((resolve) => setTimeout(resolve, remaining));
      }
    };

    try {
      onStatus(`Processing ${project}...`);
      renderSkeletons();
      loadingToast = onToastLoading?.(loadingLabel, "info", "Please wait...");
      loadingStartedAt = Date.now();

      const backstopPromise = new Promise((_, reject) => {
        backstopTimer = setTimeout(
          () => reject(new Error(ACTION_BACKSTOP_MESSAGE)),
          ACTION_BACKSTOP_MS,
        );
      });

      const message = await Promise.race([fn(project), backstopPromise]);
      onStatus(message || fallbackMessage);
      if (loadingToast) {
        await waitForToastVisibility();
        loadingToast.close(message || fallbackMessage, "success");
      } else {
        onToast(message || fallbackMessage, "success");
      }
      await refreshDashboard({ silent: true });
    } catch (err) {
      const message = `${fallbackMessage}: ${err}`;
      onStatus(message);
      if (loadingToast) {
        await waitForToastVisibility();
        loadingToast.close(message, "error");
      } else {
        onToast(message, "error");
      }
      // The refresh is what closes the loading frame raised above, and a failed
      // action can still have changed containers half way, so the list has to
      // be re-read on this path too.
      await refreshDashboard({ silent: true });
    } finally {
      if (backstopTimer !== null) {
        clearTimeout(backstopTimer);
      }
      if (opToken !== null) {
        endOperation(opToken);
      }
    }
  };

  const handle = async (action, explicitProject = "") => {
    const project = explicitProject || getProject();

    if (action === "env-start") {
      await runEnvironmentAction(
        bridge.startEnvironment,
        project,
        `Started ${project} successfully`,
        `Starting ${project}...`,
        action,
      );
      return;
    }
    if (action === "env-restart") {
      await runEnvironmentAction(
        bridge.restartEnvironment,
        project,
        `Restarted ${project} successfully`,
        `Restarting ${project}...`,
        action,
      );
      return;
    }
    if (action === "env-stop") {
      await runEnvironmentAction(
        bridge.stopEnvironment,
        project,
        `Stopped ${project} successfully`,
        `Stopping ${project}...`,
        action,
      );
      return;
    }
    if (action === "env-pull") {
      await runEnvironmentAction(
        bridge.pullEnvironment,
        project,
        `Pulled images for ${project}`,
        `Pulling images for ${project}...`,
        action,
      );
      return;
    }
    if (action === "env-delete") {
      const confirmed = await confirm({
        title: "Delete Project",
        message: buildDeleteConfirmMessage(project),
        icon: "delete_forever",
        confirmLabel: "Delete Project",
        cancelLabel: "Cancel",
      });
      if (!confirmed) return;

      await runEnvironmentAction(
        bridge.deleteProject,
        project,
        `Deleted ${project} from Govard`,
        `Deleting ${project}...`,
        action,
      );
      return;
    }
    if (action === "env-open") {
      await runEnvironmentAction(
        bridge.openEnvironment,
        project,
        `Opened ${project} in browser`,
      );
      return;
    }
    if (action === "toggle-env") {
      await runEnvironmentAction(
        bridge.toggleEnvironment,
        project,
        `Toggled ${project} state`,
        "Processing environment...",
        action,
      );
      return;
    }
    if (action === "open-env") {
      await runEnvironmentAction(
        bridge.openEnvironment,
        project,
        `Opened ${project} in browser`,
      );
      return;
    }

    if (
      [
        "open-pma",
        "toggle-xdebug",
        "check-health",
        "open-folder",
        "open-ide",
        "open-db-client",
        "open-mail-client",
      ].includes(action)
    ) {
      try {
        const message = await bridge.quickActionForProject(action, project);
        onStatus(message);
        onToast(message, "success");
        await refreshDashboard();
      } catch (err) {
        const message = `Action failed: ${err}`;
        onStatus(message);
        onToast(message, "error");
      }
    }
  };

  return { handle };
};
