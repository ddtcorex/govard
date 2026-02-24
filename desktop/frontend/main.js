import { createActionsController } from "./modules/actions.js";
import {
  normalizeDashboardPayload,
  projectKey,
  renderEnvironmentList,
  renderProjectHero,
  renderWarnings,
  setMetricText,
  syncProjectSelectors,
  renderEnvVars,
  renderMetricSkeletons as renderDashboardSkeletons,
  renderEnvironmentSkeletons,
} from "./modules/dashboard.js";
import {
  createLogsController,
  resolveLogTarget,
  syncServiceSelector,
  syncSeveritySelector,
} from "./modules/logs.js";
import {
  createMetricsController,
  renderMetricSkeletons as renderServiceSkeletons,
} from "./modules/metrics.js";
import { createOnboardingController } from "./modules/onboarding.js";
import { createRemotesController } from "./modules/remotes.js";
import { createSettingsController } from "./modules/settings.js";
import { createShellController } from "./modules/shell.js";
import { desktopBridge } from "./services/bridge.js";
import { getState, setState } from "./state/store.js";
import { createToast } from "./ui/toast.js";
import { byId, setText } from "./utils/dom.js";

const refs = {
  status: byId("status"),
  refresh: byId("refresh"),
  statActive: byId("statActive"),
  statServices: byId("statServices"),
  statQueue: byId("statQueue"),
  statActiveHint: byId("statActiveHint"),
  statServicesHint: byId("statServicesHint"),
  statQueueHint: byId("statQueueHint"),
  metricActiveProjects: byId("metricActiveProjects"),
  metricCPU: byId("metricCPU"),
  metricMemory: byId("metricMemory"),
  metricNetRx: byId("metricNetRx"),
  metricNetTx: byId("metricNetTx"),
  metricOOM: byId("metricOOM"),
  metricsList: byId("metricsList"),
  metricsWarnings: byId("metricsWarnings"),
  remotesList: byId("remotesList"),
  remotesWarnings: byId("remotesWarnings"),
  envVarsList: byId("envVarsList"),
  projectPath: byId("projectPath"),
  projectDomain: byId("projectDomain"),
  displayProjectPath: byId("displayProjectPath"),
  projectRecipe: byId("projectRecipe"),
  onboardVarnish: byId("onboardVarnish"),
  onboardRedis: byId("onboardRedis"),
  onboardRabbitMQ: byId("onboardRabbitMQ"),
  onboardElasticsearch: byId("onboardElasticsearch"),
  syncToggleSanitize: byId("syncToggleSanitize"),
  syncToggleExcludeLogs: byId("syncToggleExcludeLogs"),
  syncToggleCompress: byId("syncToggleCompress"),
  remoteName: byId("remoteName"),
  remoteHost: byId("remoteHost"),
  remoteUser: byId("remoteUser"),
  remotePath: byId("remotePath"),
  remotePort: byId("remotePort"),
  remoteEnvironment: byId("remoteEnvironment"),
  remoteCapabilities: byId("remoteCapabilities"),
  remoteAuthMethod: byId("remoteAuthMethod"),
  remoteProtected: byId("remoteProtected"),
  envList: byId("envList"),
  envSelector: byId("envSelector"),
  logSelector: byId("logSelector"),
  logServiceSelector: byId("logServiceSelector"),
  logSeverity: byId("logSeverity"),
  logSearch: byId("logSearch"),
  logOutput: byId("logOutput"),
  toggleLive: byId("toggleLive"),
  shellUser: byId("shellUser"),
  shellCommand: byId("shellCommand"),
  warningList: byId("warningList"),
  openSettings: byId("openSettings"),
  closeSettings: byId("closeSettings"),
  settingsDrawer: byId("settingsDrawer"),
  themeSelect: byId("themeSelect"),
  proxyTarget: byId("proxyTarget"),
  preferredBrowser: byId("preferredBrowser"),
  codeEditor: byId("codeEditor"),
  userAvatar: byId("userAvatar"),
  userName: byId("userName"),
  terminalContainer: byId("terminalContainer"),
  toastContainer: byId("toastContainer"),
  onboardingModal: byId("onboardingModal"),
  projectTitle: byId("projectTitle"),
  projectStatusBadge: byId("projectStatusBadge"),
  projectStatusText: byId("projectStatusText"),
  projectUrl: byId("projectUrl"),
  projectUrlText: byId("projectUrlText"),
  projectTechnologies: byId("projectTechnologies"),
  heroRestartBtn: byId("heroRestartBtn"),
  heroStopBtn: byId("heroStopBtn"),
  footerCPU: byId("footerCPU"),
  footerMemory: byId("footerMemory"),
};

const toast = createToast(refs.toastContainer);

const setStatus = (message) => {
  setText(refs.status, message);
};

const loadUser = async () => {
  try {
    const user = await desktopBridge.getCurrentUser();
    if (refs.userName) setText(refs.userName, user.name || user.username);
    if (refs.userAvatar) {
      const initials = (user.name || user.username || "??")
        .split(" ")
        .map((n) => n[0])
        .join("")
        .toUpperCase()
        .slice(0, 2);
      setText(refs.userAvatar, initials);
    }
  } catch (_err) {
    if (refs.userName) setText(refs.userName, "Unknown User");
  }
};

const showToast = (message, type = "success") => {
  toast.show(message, type);
};

const switchTab = (tabId) => {
  const tabLinks = document.querySelectorAll('[data-action="switch-tab"]');
  const tabContents = document.querySelectorAll(".tab-content");

  tabLinks.forEach((l) => {
    l.classList.remove("border-primary", "text-primary");
    l.classList.add("border-transparent", "text-[#90cba4]");
    if (l instanceof HTMLElement && l.dataset.tab === tabId) {
      l.classList.remove("border-transparent", "text-[#90cba4]");
      l.classList.add("border-primary", "text-primary");
    }
  });

  tabContents.forEach((c) => {
    c.classList.remove("active", "flex", "block");
    c.classList.add("hidden");
  });

  const content = byId("tab-" + tabId);
  if (content) {
    content.classList.remove("hidden");
    content.classList.add("active");
    if (tabId === "logs") content.classList.add("flex");
    else content.classList.add("block");
  }
};

const showSystemNotification = (title, body) => {
  if (
    typeof window === "undefined" ||
    typeof window.Notification === "undefined"
  ) {
    return;
  }
  if (window.Notification.permission === "granted") {
    new window.Notification(title, { body });
    return;
  }
  if (window.Notification.permission === "default") {
    window.Notification.requestPermission().then((permission) => {
      if (permission === "granted") {
        new window.Notification(title, { body });
      }
    });
  }
};

if (desktopBridge.runtime?.EventsOn) {
  desktopBridge.runtime.EventsOn("operations:notification", (payload = {}) => {
    const title = String(payload.title || "Govard operation update");
    const body = String(payload.body || "").trim();
    let level = payload.level || "success";
    if (
      body.toLowerCase().includes("failed") ||
      body.toLowerCase().includes("error")
    ) {
      level = "error";
    } else if (
      body.toLowerCase().includes("unable") ||
      body.toLowerCase().includes("warning")
    ) {
      level = "warning";
    }
    showToast(body || title, level);
    showSystemNotification(title, body || title);
  });
}

const readSelection = () =>
  resolveLogTarget({
    project: getState().selectedProject,
    service: getState().selectedService,
    severity: getState().selectedSeverity,
    query: getState().logQuery,
  });

const safeDashboard = {
  ActiveEnvironments: 2,
  RunningServices: 5,
  QueuedTasks: 0,
  ActiveSummary: "2 environments running",
  ServicesSummary: "All systems healthy",
  QueueSummary: "Queue idle",
  Environments: [
    {
      Name: "Project Alpha",
      Status: "running",
      Domain: "project-alpha.test",
      Url: "http://project-alpha.test",
      Technologies: ["PHP 8.2", "MySQL 8.0", "Redis"],
      EnvVars: {
        APP_ENV: "local",
        APP_DEBUG: "true",
        DB_CONNECTION: "mysql",
      },
    },
    {
      Name: "Project Beta",
      Status: "stopped",
      Domain: "project-beta.test",
      Url: "http://project-beta.test",
      Technologies: ["Python 3.11", "Postgres 15", "RabbitMQ"],
    },
    {
      Name: "Project Gamma",
      Status: "warning",
      Domain: "project-gamma.test",
      Url: "http://project-gamma.test",
      Technologies: ["Node.js 20", "MongoDB 6.0"],
    },
  ],
  Warnings: ["Desktop bridge unavailable. Showing local fallback view."],
};

const loadDashboard = async () => {
  try {
    const data = await desktopBridge.getDashboard();
    return normalizeDashboardPayload(data);
  } catch (_err) {
    return normalizeDashboardPayload(safeDashboard);
  }
};

const syncProjectState = () => {
  const state = getState();
  const selectedProject =
    refs.envSelector?.value || refs.logSelector?.value || state.selectedProject || "";
  setState({ selectedProject });
};

const syncServiceState = () => {
  const container = refs.logServiceSelector;
  const activeBtn = container?.querySelector("button.bg-\\[\\#2e573a\\]");
  const selectedService = activeBtn?.dataset.service || "all";
  setState({ selectedService });
};

const syncLogFiltersState = () => {
  const container = refs.logSeverity;
  const activeBtn = container?.querySelector("button.bg-\\[\\#2e573a\\]");
  const selectedSeverity = activeBtn?.dataset.severity || "all";
  const logQuery = refs.logSearch?.value || "";
  setState({ selectedSeverity, logQuery });
};

const refreshSeveritySelector = () => {
  const state = getState();
  syncSeveritySelector(refs.logSeverity, state.selectedSeverity);
};

const refreshServiceSelector = () => {
  const state = getState();
  const selectedService = syncServiceSelector(
    refs.logServiceSelector,
    state.environments,
    state.selectedProject,
    state.selectedService,
  );
  setState({ selectedService });
};

const logsController = createLogsController({
  bridge: desktopBridge,
  runtime: desktopBridge.runtime,
  refs,
  readSelection,
  onStatus: setStatus,
  onToast: showToast,
});

const shellController = createShellController({
  bridge: desktopBridge,
  refs,
  readSelection,
  onStatus: setStatus,
  onToast: showToast,
});

const metricsController = createMetricsController({
  bridge: desktopBridge,
  refs,
  onStatus: setStatus,
  getProject: () => getState().selectedProject,
});

const remotesController = createRemotesController({
  bridge: desktopBridge,
  refs,
  getProject: () => getState().selectedProject,
  getSyncConfig: () => getState().syncConfig,
  onStatus: setStatus,
  onToast: showToast,
});

const renderAllSkeletons = () => {
  renderDashboardSkeletons(refs);
  renderEnvironmentSkeletons(refs.envList);
  renderServiceSkeletons(refs.metricsList);
};

const refreshDashboard = async (options = {}) => {
  setStatus("Status: syncing dashboard...");
  if (!options.silent) {
    renderAllSkeletons();
  }
  const dashboard = await loadDashboard();
  setMetricText(dashboard, refs);
  renderWarnings(refs.warningList, dashboard.warnings);
  renderEnvironmentList(
    refs.envList,
    dashboard.environments,
    getState().selectedProject,
  );

  const previousProject = getState().selectedProject;
  syncProjectSelectors(
    { envSelector: refs.envSelector, logSelector: refs.logSelector },
    dashboard.environments,
    previousProject,
  );

  const selectedProject = refs.envSelector?.value || getState().selectedProject || "";
  setState({ environments: dashboard.environments, selectedProject });
  if (!selectedProject && dashboard.environments.length > 0) {
    setState({ selectedProject: projectKey(dashboard.environments[0]) });
  }

  if (
    refs.logSelector &&
    refs.logSelector.value !== getState().selectedProject
  ) {
    refs.logSelector.value = getState().selectedProject;
  }
  if (
    refs.envSelector &&
    refs.envSelector.value !== getState().selectedProject
  ) {
    refs.envSelector.value = getState().selectedProject;
  }

  refreshServiceSelector();
  refreshSeveritySelector();
  syncLogFiltersState();
  renderEnvironmentList(
    refs.envList,
    dashboard.environments,
    getState().selectedProject,
  );
  renderProjectHero(refs, dashboard.environments, getState().selectedProject);
  await metricsController.refresh({ silent: true });
  await remotesController.refresh({ silent: true });
  remotesController.syncSyncConfigUI(getState().syncConfig);
  await shellController.loadShellUser();
  await logsController.refresh();
  setStatus(`Status: Ready`);
};

const onboardingController = createOnboardingController({
  bridge: desktopBridge,
  refs,
  onStatus: setStatus,
  onToast: showToast,
  onProjectAdded: refreshDashboard,
});

const actionsController = createActionsController({
  bridge: desktopBridge,
  getProject: () => getState().selectedProject,
  refreshDashboard,
  renderSkeletons: renderAllSkeletons,
  onStatus: setStatus,
  onToast: showToast,
});

const settingsController = createSettingsController({
  bridge: desktopBridge,
  refs,
  onStatus: setStatus,
  onToast: showToast,
});

document.addEventListener("click", async (event) => {
  const target = event.target;
  if (!(target instanceof HTMLElement)) {
    return;
  }

  const action = target.closest("[data-action]")?.dataset.action;
  const targetElement = target.closest("[data-action]");
  if (!action) {
    return;
  }

  if (action === "select-environment") {
    const project = targetElement.dataset.env || "";
    setState({ selectedProject: project });
    if (refs.envSelector) refs.envSelector.value = project;
    if (refs.logSelector) refs.logSelector.value = project;
    switchTab("dashboard");
    await syncProjectSelectorsFrom("env");
    await refreshDashboard({ silent: true });
    return;
  }

  if (action === "refresh-logs") {
    await logsController.refresh();
    return;
  }
  if (action === "refresh-metrics") {
    await metricsController.refresh();
    return;
  }
  if (action === "browse-project") {
    await onboardingController.browseProject();
    return;
  }
  if (action === "add-project") {
    await onboardingController.addProject();
    return;
  }
  if (action === "refresh-remotes") {
    await remotesController.refresh();
    return;
  }
  if (action === "save-remote") {
    await remotesController.saveRemote();
    return;
  }
  if (action === "open-onboarding") {
    onboardingController.toggleModal(true);
    return;
  }
  if (action === "close-onboarding") {
    onboardingController.toggleModal(false);
    return;
  }
  if (action === "remote-test") {
    await remotesController.testRemote(String(target.dataset.remote || ""));
    return;
  }
  if (action === "remote-sync") {
    await remotesController.runSyncPreset(
      String(target.dataset.remote || ""),
      String(target.dataset.preset || ""),
    );
    return;
  }
  if (action === "toggle-sync-config") {
    const configKey = targetElement.dataset.config;
    if (configKey) {
      const currentConfig = getState().syncConfig;
      await remotesController.toggleSyncConfig(
        configKey,
        currentConfig,
        (nextConfig) => setState({ syncConfig: nextConfig }),
      );
    }
    return;
  }
  if (action === "toggle-live") {
    await logsController.toggleLive();
    return;
  }
  if (action === "clear-logs") {
    await logsController.clearLogs();
    return;
  }
  if (action === "download-logs") {
    logsController.downloadLogs();
    return;
  }
  if (action === "open-shell") {
    await shellController.openShell();
    return;
  }
  if (action === "reset-shell-users") {
    await shellController.resetShellUsers();
    return;
  }
  if (action === "filter-service") {
    const service = targetElement.dataset.service || "all";
    setState({ selectedService: service });
    refreshServiceSelector();
    await logsController.refresh();
    return;
  }
  if (action === "filter-severity") {
    const severity = targetElement.dataset.severity || "all";
    setState({ selectedSeverity: severity });
    refreshSeveritySelector();
    await logsController.refresh();
    return;
  }
  if (action === "reset-settings") {
    await settingsController.reset();
    return;
  }

  if (action === "switch-tab") {
    const tabId = targetElement.dataset.tab;
    if (tabId) switchTab(tabId);
    return;
  }

  await actionsController.handle(action, targetElement.dataset.env || "");
});

if (refs.refresh) {
  refs.refresh.addEventListener("click", () => {
    refreshDashboard();
  });
}

if (refs.openSettings) {
  refs.openSettings.addEventListener("click", () =>
    settingsController.toggleDrawer(true),
  );
}

if (refs.closeSettings) {
  refs.closeSettings.addEventListener("click", () =>
    settingsController.toggleDrawer(false),
  );
}

if (refs.settingsDrawer) {
  refs.settingsDrawer.addEventListener("click", (event) => {
    if (event.target === refs.settingsDrawer) {
      settingsController.toggleDrawer(false);
    }
  });
}

document.addEventListener("keydown", (event) => {
  if (event.key === "Escape") {
    settingsController.toggleDrawer(false);
  }
  if ((event.ctrlKey || event.metaKey) && event.key === ",") {
    event.preventDefault();
    settingsController.toggleDrawer(true);
  }
});

const syncProjectSelectorsFrom = async (source) => {
  if (source === "env") {
    // envSelector is the source of truth if logSelector is gone
  }
  syncProjectState();
  refreshServiceSelector();
  await shellController.loadShellUser();
  if (logsController.isLiveEnabled()) {
    await logsController.stopLive();
    await logsController.toggleLive();
  } else {
    await logsController.refresh();
  }
  await remotesController.refresh({ silent: true });
};

if (refs.envSelector) {
  refs.envSelector.addEventListener("change", async () => {
    await syncProjectSelectorsFrom("env");
  });
}

if (refs.logSeverity) {
  refs.logSeverity.addEventListener("change", () => {
    syncLogFiltersState();
    logsController.applyFilters();
  });
}

if (refs.logSearch) {
  refs.logSearch.addEventListener("input", () => {
    syncLogFiltersState();
    logsController.applyFilters();
  });
}

if (refs.shellUser) {
  refs.shellUser.addEventListener("change", () => {
    shellController.saveShellUser();
  });
}

if (refs.themeSelect) {
  refs.themeSelect.addEventListener("change", () => {
    settingsController.save();
  });
}

if (refs.codeEditor) {
  refs.codeEditor.addEventListener("change", () => {
    settingsController.save();
  });
}

if (refs.proxyTarget) {
  refs.proxyTarget.addEventListener("change", async () => {
    await settingsController.save();
  });
}

if (refs.preferredBrowser) {
  refs.preferredBrowser.addEventListener("change", () => {
    settingsController.save();
  });
}

if (window.matchMedia) {
  window
    .matchMedia("(prefers-color-scheme: dark)")
    .addEventListener("change", () => {
      if ((refs.themeSelect?.value || "system") === "system") {
        settingsController.load();
      }
    });
}

setStatus("Status: Ready");
setState({ selectedService: "all", selectedSeverity: "all", logQuery: "" });
await loadUser();
await settingsController.load();
await refreshDashboard();
metricsController.startAutoRefresh();

window.addEventListener("beforeunload", () => {
  metricsController.stopAutoRefresh();
});
