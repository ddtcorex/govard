export const createActionsController = ({
  bridge,
  getProject,
  refreshDashboard,
  renderSkeletons,
  onStatus,
  onToast,
}) => {
  const runEnvironmentAction = async (fn, project, fallbackMessage) => {
    if (!project) {
      onStatus("Select an environment first.");
      return;
    }
    try {
      onStatus(`Processing ${project}...`);
      renderSkeletons();
      const message = await fn(project);
      onStatus(message || fallbackMessage);
      onToast(message || fallbackMessage, "success");
      await refreshDashboard({ silent: true });
    } catch (err) {
      const message = `${fallbackMessage}: ${err}`;
      onStatus(message);
      onToast(message, "error");
    }
  };

  const handle = async (action, explicitProject = "") => {
    const project = explicitProject || getProject();

    if (action === "env-start") {
      await runEnvironmentAction(
        bridge.startEnvironment,
        project,
        `Failed to start ${project}`,
      );
      return;
    }
    if (action === "env-restart") {
      await runEnvironmentAction(
        bridge.restartEnvironment,
        project,
        `Failed to restart ${project}`,
      );
      return;
    }
    if (action === "env-stop") {
      await runEnvironmentAction(
        bridge.stopEnvironment,
        project,
        `Failed to stop ${project}`,
      );
      return;
    }
    if (action === "env-open") {
      await runEnvironmentAction(
        bridge.openEnvironment,
        project,
        `Failed to open ${project}`,
      );
      return;
    }
    if (action === "toggle-env") {
      await runEnvironmentAction(
        bridge.toggleEnvironment,
        project,
        `Failed to toggle ${project}`,
      );
      return;
    }
    if (action === "open-env") {
      await runEnvironmentAction(
        bridge.openEnvironment,
        project,
        `Failed to open ${project}`,
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
