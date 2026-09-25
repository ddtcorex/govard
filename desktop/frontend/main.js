console.log("==> main.js top level loaded! <==");
import { createActionsController } from "./modules/actions.js?v=20260301";
import { normalizeDashboardPayload, projectKey } from "./modules/dashboard.js";
import {
  createGlobalServicesController,
  raiseActionFeedback,
} from "./modules/global-services.js";
import { resolveServiceTargets } from "./modules/logs.js";
import {
  createOnboardingController,
  renderOnboardingModal,
} from "./modules/onboarding.js?v=20260302";
import { createSettingsController } from "./modules/settings.js";
import { createUpdateNotifierModel } from "./modules/update-notifier.js";
import { createElement } from "react";
import { mountIsland } from "./islands/mount.js";
import { LogsTab } from "./islands/LogsTab.tsx";
import { MetricsFooter } from "./islands/MetricsFooter.tsx";
import { SettingsDrawer } from "./islands/SettingsDrawer.tsx";
import { ActiveServices } from "./islands/ActiveServices.tsx";
import { EnvVars } from "./islands/EnvVars.tsx";
import { EnvironmentList } from "./islands/EnvironmentList.tsx";
import { ProjectHero } from "./islands/ProjectHero.tsx";
import { GlobalHealthHeader } from "./islands/GlobalHealthHeader.tsx";
import { GlobalLogsPanel } from "./islands/GlobalLogsPanel.tsx";
import { GlobalServicesList } from "./islands/GlobalServicesList.tsx";
import { RemotesList } from "./islands/RemotesList.tsx";
import { SyncModal } from "./islands/SyncModal.tsx";
import { UpdatePrompt } from "./islands/UpdatePrompt.tsx";
import { desktopBridge } from "./services/bridge.js";
import { hasEventRuntime, onEvent } from "./services/events.js";
import { getState, setState } from "./state/store.js";
import { createToast } from "./ui/toast.js?v=20260301";
import { byId, setText } from "./utils/dom.js";
console.log("==> Finished imports <==");

const initUI = () => {
  renderOnboardingModal(byId("onboardingModalMount"));
  refreshRefs();
};

const getLiveRefs = () => ({
  refresh: byId("refresh"),
  status: byId("status"),
  envList: byId("envList"),
  sidebarPanelEnvironments: byId("sidebarPanel-environments"),
  sidebarEnvActions: byId("sidebarEnvActions"),
  openSettings: byId("openSettings"),
  hardReset: byId("hardReset"),
  userAvatar: byId("userAvatar"),
  userName: byId("userName"),
  toastContainer: byId("toastContainer"),
  confirmModal: byId("confirmModal"),
  confirmIcon: byId("confirmIcon"),
  confirmTitle: byId("confirmTitle"),
  confirmMessage: byId("confirmMessage"),
  confirmCancelBtn: byId("confirmCancelBtn"),
  confirmConfirmBtn: byId("confirmConfirmBtn"),
  onboardingModal: byId("onboardingModal"),
  projectPath: byId("projectPath"),
  displayProjectPath: byId("displayProjectPath"),
  projectPathHint: byId("projectPathHint"),
  projectDomain: byId("projectDomain"),
  projectDomainHint: byId("projectDomainHint"),
  projectFramework: byId("projectFramework"),
  projectFrameworkVersion: byId("projectFrameworkVersion"),
  projectFrameworkVersionHint: byId("projectFrameworkVersionHint"),
  onboardFromGit: byId("onboardFromGit"),
  gitCloneFields: byId("gitCloneFields"),
  gitProtocol: byId("gitProtocol"),
  gitUrl: byId("gitUrl"),
  gitUrlHint: byId("gitUrlHint"),
  gitConfirmContainer: byId("gitConfirmContainer"),
  gitConfirmOverride: byId("gitConfirmOverride"),
  gitConfirmHint: byId("gitConfirmHint"),
  onboardingSummaryProject: byId("onboardingSummaryProject"),
  onboardingSummaryFramework: byId("onboardingSummaryFramework"),
  onboardingSummaryDomain: byId("onboardingSummaryDomain"),
  onboardingSubmitSpinner: byId("onboardingSubmitSpinner"),
  onboardingSubmitHint: byId("onboardingSubmitHint"),
  onboardingSubmit: byId("onboardingSubmit"),
  onboardVarnish: byId("onboardVarnish"),
  onboardRedis: byId("onboardRedis"),
  onboardRabbitMQ: byId("onboardRabbitMQ"),
  onboardElasticsearch: byId("onboardElasticsearch"),
  confirmModal: byId("confirmModal"),
  confirmIcon: byId("confirmIcon"),
  confirmTitle: byId("confirmTitle"),
  confirmMessage: byId("confirmMessage"),
  confirmCancelBtn: byId("confirmCancelBtn"),
  confirmConfirmBtn: byId("confirmConfirmBtn"),
  projectTitle: byId("projectTitle"),
  projectStatusBadge: byId("projectStatusBadge"),
  projectStatusText: byId("projectStatusText"),
  projectUrl: byId("projectUrl"),
  projectUrlText: byId("projectUrlText"),
  projectTechnologies: byId("projectTechnologies"),
  heroRestartBtn: byId("heroRestartBtn"),
  heroStopBtn: byId("heroStopBtn"),
  heroPullBtn: byId("heroPullBtn"),
  footerVersion: byId("footerVersion"),
  envVarsList: byId("envVarsList"),
});

let refs = getLiveRefs();

const refreshRefs = () => {
  const newRefs = getLiveRefs();
  Object.assign(refs, newRefs);
  // Propagate updated refs to controllers if they don't hold the object by reference
  // (Most do, but we keep this for safety and explicit update triggers)
  if (settingsController?.updateRefs) settingsController.updateRefs(refs);
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

const loadFooterVersion = async () => {
  const footerVersionEl = byId("footerVersion");
  if (!footerVersionEl) return;
  const maxAttempts = 15;

  for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
    try {
      const version = String(await desktopBridge.getVersion()).trim();
      if (version) {
        const normalized = version.startsWith("v") ? version : `v${version}`;
        setText(footerVersionEl, normalized);
        if (refs.footerVersion && refs.footerVersion !== footerVersionEl) {
          setText(refs.footerVersion, normalized);
        }
        return;
      }
    } catch (_err) {
      // Bridge may not be fully ready during early bootstrap.
    }

    if (attempt < maxAttempts - 1) {
      await new Promise((resolve) => setTimeout(resolve, 300));
    }
  }

  setText(footerVersionEl, "v--");
};

const showToast = (message, type = "success") => {
  toast.show(message, type);
};

export const toggleModalBlur = (isVisible) => {
  const mainContent = byId("mainContent");
  if (!mainContent) return;
  if (isVisible) {
    mainContent.classList.add("blur-xs", "pointer-events-none", "select-none", "opacity-50");
  } else {
    mainContent.classList.remove("blur-xs", "pointer-events-none", "select-none", "opacity-50");
  }
};

window.addEventListener("govard:blur", (e) => {
  toggleModalBlur(!!e.detail?.isVisible);
});

const showConfirm = ({
  title,
  message,
  icon = "help",
  confirmText = "Confirm",
  cancelText = "Cancel",
}) => {
  return new Promise((resolve) => {
    if (
      !refs.confirmModal ||
      !refs.confirmTitle ||
      !refs.confirmMessage ||
      !refs.confirmConfirmBtn ||
      !refs.confirmCancelBtn
    ) {
      // Fallback if elements are missing
      resolve(confirm(message || "Are you sure?"));
      return;
    }

    refs.confirmTitle.textContent = title || "Confirm Action";
    refs.confirmMessage.innerHTML =
      message || "Are you sure you want to proceed?";
    if (refs.confirmIcon) refs.confirmIcon.textContent = icon;
    refs.confirmConfirmBtn.textContent = confirmText;
    refs.confirmCancelBtn.textContent = cancelText;

    const onConfirm = () => {
      cleanup();
      resolve(true);
    };
    const onCancel = () => {
      cleanup();
      resolve(false);
    };
    const cleanup = () => {
      toggleModalBlur(false);
      refs.confirmModal.classList.remove("is-visible");
      refs.confirmConfirmBtn.removeEventListener("click", onConfirm);
      refs.confirmCancelBtn.removeEventListener("click", onCancel);
    };

    refs.confirmConfirmBtn.addEventListener("click", onConfirm);
    refs.confirmCancelBtn.addEventListener("click", onCancel);
    toggleModalBlur(true);
    refs.confirmModal.classList.add("is-visible");
  });
};

const showLoadingToast = (
  title = "Processing...",
  type = "info",
  initialLine = "Please wait...",
) => {
  const loadingToast = toast.showStreaming(title, type, { dedupeKey: false });
  if (loadingToast && initialLine) {
    loadingToast.update(initialLine);
    return loadingToast;
  }

  const container = refs.toastContainer;
  if (!container) {
    return null;
  }

  const item = document.createElement("div");
  item.className = `toast toast--${type} group`;
  item.innerHTML = `
    <div class="toast-indicator"></div>
    <div class="toast-icon-wrapper">
      <span class="material-symbols-outlined toast-icon">info</span>
    </div>
    <div class="toast-content">
      <div style="display:flex; align-items:center; gap:8px;">
        <p class="toast-message" style="font-weight:600; margin:0;">${String(title)}</p>
        <span class="toast-spinner inline-block w-3 h-3 border-2 border-white/30 border-t-white rounded-full animate-spin"></span>
      </div>
      <p class="toast-stream-line text-xs font-mono opacity-80 mt-1">${String(initialLine || "Please wait...")}</p>
    </div>
    <button class="toast-close" aria-label="Close">
      <span class="material-symbols-outlined">close</span>
    </button>
  `;
  container.appendChild(item);
  requestAnimationFrame(() => item.classList.add("is-visible"));

  const lineEl = item.querySelector(".toast-stream-line");
  const spinnerEl = item.querySelector(".toast-spinner");
  const iconEl = item.querySelector(".toast-icon");
  const closeBtn = item.querySelector(".toast-close");

  let removed = false;
  const remove = () => {
    if (removed || item.classList.contains("is-removing")) return;
    removed = true;
    item.classList.add("is-removing");
    item.classList.remove("is-visible");
    setTimeout(() => {
      item.remove();
    }, 500);
  };

  if (closeBtn) {
    closeBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      remove();
    });
  }

  return {
    update: (line) => {
      if (lineEl && line) {
        lineEl.textContent = String(line);
      }
    },
    close: (finalLabel, finalType = "success") => {
      if (spinnerEl) spinnerEl.style.display = "none";
      if (lineEl && finalLabel) {
        lineEl.textContent = String(finalLabel);
      }
      const iconByType = {
        success: "check_circle",
        error: "report",
        warning: "warning",
        info: "info",
      };
      if (iconEl) {
        iconEl.textContent = iconByType[finalType] || "check_circle";
      }
      item.className = `toast toast--${finalType} group is-visible`;
      setTimeout(remove, 4000);
    },
  };
};

const resolveSyncConfigForPreset = async (preset) => {
  const state = getState();
  const project = state.selectedProject;
  const payload = await desktopBridge.getSyncPresetOptions(project || "", preset);
  const optionsDef = Array.isArray(payload?.options) ? payload.options : [];

  const currentConfigs = state.syncConfigs || {};
  const currentConfig = { ...(currentConfigs[preset] || {}) };

  optionsDef.forEach((option) => {
    if (currentConfig[option.key] === undefined) {
      currentConfig[option.key] = Boolean(option.defaultValue);
    }
  });

  setState({
    syncConfigs: { ...currentConfigs, [preset]: currentConfig },
  });

  return currentConfig;
};

const selectProject = async (project) => {
  if (!project) return;
  // Normalize: if it's a path, use basename
  const normalized = project.includes("/") || project.includes("\\")
    ? project.split(/[\\/]+/).filter(Boolean).pop()
    : project;

  setState({ selectedProject: normalized });
  if (refs.envSelector) refs.envSelector.value = normalized;
  if (refs.logSelector) refs.logSelector.value = normalized;

  await switchSidebarMode("environments", { silent: true, skipLogs: true });

  const currentTab = document.querySelector(".tab-content.active")?.id;
  if (!currentTab || currentTab === "tab-global-services") {
    switchTab("dashboard");
  }

  await syncProjectSelectorsFrom("env");
  await refreshDashboard({ silent: true });

  const activeTabId = document
    .querySelector(".tab-content.active")
    ?.id?.replace("tab-", "");

  if (activeTabId === "logs") {
    void logsApi.refresh();
  } else if (activeTabId === "remotes") {
    remotesApi.refresh();
  }
};

const switchTab = (tabId) => {
  const tabLinks = document.querySelectorAll('[data-action="switch-tab"]');
  const tabContents = document.querySelectorAll(".tab-content");

  tabLinks.forEach((l) => {
    l.className = "border-transparent text-slate-500 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white hover:border-slate-300 dark:hover:border-slate-500 whitespace-nowrap py-4 px-4 border-b-2 font-bold text-sm flex items-center gap-2 transition-all";
    if (l instanceof HTMLElement && l.dataset.tab === tabId) {
      l.className = "border-primary text-primary dark:text-white whitespace-nowrap py-4 px-4 border-b-2 font-bold text-sm flex items-center gap-2 transition-all active";
    }
  });

  tabContents.forEach((c) => {
    c.classList.remove("active");
    c.classList.add("hidden");
  });

  const content = byId("tab-" + tabId);
  if (content) {
    content.classList.remove("hidden");
    content.classList.add("active");
  }

  const scrollContainer = byId("unifiedScrollContainer");
  const hero = byId("projectHero");
  const tabs = byId("tabsHeader");

  const tabsInner = byId("tabsHeaderInner");

  if (scrollContainer && hero && tabs && tabsInner) {
    const showHero = ["dashboard", "remotes", "logs"].includes(tabId);
    hero.classList.toggle("hidden", !showHero);
    const showPrimaryTabs = tabId !== "global-services";

    if (tabId === "dashboard") {
      scrollContainer.classList.add("overflow-y-auto");
      scrollContainer.classList.remove("overflow-hidden");
      refreshDashboard();
    } else if (tabId === "remotes") {
      scrollContainer.classList.add("overflow-y-auto");
      scrollContainer.classList.remove("overflow-hidden");
      remotesApi.refresh();
    } else if (tabId === "logs") {
      scrollContainer.classList.remove("overflow-y-auto");
      scrollContainer.classList.add("overflow-hidden");
      setState({ selectedService: "all" });
      refreshServiceSelector();
      void logsApi.refresh();
    } else if (tabId === "global-services") {
      scrollContainer.classList.add("overflow-y-auto");
      scrollContainer.classList.remove("overflow-hidden");
    }

    // Standardize header styling across all tabs
    tabs.className = showPrimaryTabs
      ? "border-b border-[#22492f] shrink-0 bg-background-dark sticky top-0 z-10 w-full"
      : "hidden";
    tabsInner.className = "w-full";
  }
};

const switchSidebarMode = async (mode, options = {}) => {
  const normalized =
    mode === "environments" ? "environments" : "global-services";
  setState({ sidebarMode: normalized });

  if (refs.sidebarPanelEnvironments) {
    refs.sidebarPanelEnvironments.classList.remove("hidden");
  }
  if (refs.sidebarEnvActions) {
    refs.sidebarEnvActions.classList.remove("hidden");
  }
  if (normalized === "global-services") {
    switchTab("global-services");
    await globalServicesController.refresh({ silent: Boolean(options.silent) });
    if (!options.skipLogs) {
      await globalLogsApi.refreshLogs();
    }
    return;
  }

  await globalLogsApi.stopLive();
  const activeTabId = document
    .querySelector(".tab-content.active")
    ?.id?.replace("tab-", "");
  if (!activeTabId || activeTabId === "global-services") {
    switchTab("dashboard");
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

if (hasEventRuntime()) {
  onEvent("operations:notification", (payload = {}) => {
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
    console.error("Dashboard fetch error caught!", _err);
    const result = normalizeDashboardPayload(safeDashboard);
    console.log("Returning safe payload:", result);
    return result;
  }
};

const syncProjectState = () => {
  const state = getState();
  const selectedProject =
    refs.envSelector?.value ||
    refs.logSelector?.value ||
    state.selectedProject ||
    "";
  setState({ selectedProject });
};

// The service strip lives inside the logs island now, so this resolves the
// selection against whatever the refreshed environment list still offers and
// writes it to the store, where the island reads it (spec D6). main.js keeps
// calling it because the selection is shared state: switching to the logs tab,
// opening a service's logs, and a dashboard refresh all go through here.
const refreshServiceSelector = () => {
  const state = getState();
  const { service } = resolveServiceTargets(
    state.environments,
    state.selectedProject,
    state.selectedService,
  );
  setState({ selectedService: service });
};

const setSelectedProject = (project) => {
  const value = String(project || "").trim();
  if (!value) return;
  setState({ selectedProject: value });
  if (refs.logSelector && refs.logSelector.value !== value) {
    refs.logSelector.value = value;
  }
  if (refs.envSelector && refs.envSelector.value !== value) {
    refs.envSelector.value = value;
  }
};

const openServiceContext = async (project, service) => {
  const selectedProject = String(
    project || getState().selectedProject || "",
  ).trim();
  if (!selectedProject) {
    setStatus("Select an environment first.");
    return;
  }

  setSelectedProject(selectedProject);
  switchTab("logs");

  const selectedService =
    String(service || "all")
      .trim()
      .toLowerCase() || "all";
  setState({ selectedService });
  refreshServiceSelector();

  await logsApi.refresh();
};

// The footer readout is a React island that owns its own polling interval;
// refreshDashboard reaches it through the function it registers. The preview
// (preview/bootstrap.js) sets a short interval so a behaviour test can observe
// the polling stop on unmount; production never defines that global.
let refreshMetrics = async () => null;
const metricsIsland = mountIsland(
  "metricsIsland",
  createElement(MetricsFooter, {
    bridge: desktopBridge,
    onStatus: setStatus,
    registerRefresh: (fn) => {
      refreshMetrics = fn;
    },
    intervalMs: window.__govardPreviewMetricsIntervalMs ?? 15000,
  }),
);
if (window.__govardPreviewMetricsIntervalMs) window.__govardMetricsIsland = metricsIsland;

// The logs tab is an island too: it owns the markup the vanilla tab used to
// inject, plus the live interval and the logs:* subscriptions the controller
// could never clean up after. main.js keeps its own triggers (tab switch,
// dashboard refresh, project change) through the controller the island
// registers, so no path fetches the same buffer twice. The preview
// (preview/bootstrap.js) shortens the poll and exposes the island so a
// behaviour test can watch it stop on unmount; production defines no such
// global.
let logsApi = {
  refresh: async () => null,
  selectionChanged: async () => {},
};
const logsIsland = mountIsland(
  "logsIsland",
  createElement(LogsTab, {
    bridge: desktopBridge,
    onStatus: setStatus,
    onToast: showToast,
    registerController: (api) => {
      logsApi = api;
    },
    livePollMs: window.__govardPreviewLogsPollMs ?? 2000,
  }),
);
if (window.__govardPreviewLogsPollMs) window.__govardLogsIsland = logsIsland;

// The remotes tab and its sync dialog are islands over the same seam the logs
// tab uses: each registers the imperative API main.js still calls, and every
// control they own calls back into main.js. The card buttons that open the
// dialog live inside the list island and the dialog lives in its own mount
// point, so the list hands the open request up and main.js routes it down.
let remotesApi = {
  refresh: async () => {},
  runSync: async () => {},
};
let syncModalApi = {
  open: async () => {},
  close: () => {},
};

const remotesIsland = mountIsland(
  "remotesIsland",
  createElement(RemotesList, {
    bridge: desktopBridge,
    onStatus: setStatus,
    onToast: showToast,
    onOpenSyncModal: (remote, preset) => void syncModalApi.open(remote, preset),
    // A finished or failed sync refreshes the dashboard, which is what reloads
    // the remotes list itself - the island is the reader of the same store.
    onSyncSettled: () => refreshDashboard({ silent: true }),
    registerApi: (api) => {
      remotesApi = api;
    },
  }),
);

const syncModalIsland = mountIsland(
  "syncOptionsModalMount",
  createElement(SyncModal, {
    bridge: desktopBridge,
    onStatus: setStatus,
    onToast: showToast,
    onModalBlur: toggleModalBlur,
    onConfirmSync: ({ remote, preset, config }) => {
      void remotesApi.runSync({
        project: getState().selectedProject,
        remoteName: remote,
        preset,
        config,
      });
    },
    registerApi: (api) => {
      syncModalApi = api;
    },
  }),
);





// The sidebar's loading frame belongs to its island now: main.js asks the island
// to show it instead of writing skeleton markup into a subtree React owns. The
// metric skeletons went with the footer tiles (#425).
let envSkeleton = { show: () => {}, hide: () => {} };
const renderAllSkeletons = () => {
  envSkeleton.show();
};

const refreshDashboard = async (options = {}) => {
  try {
    setStatus("Status: syncing dashboard...");
    if (!options.silent) {
      renderAllSkeletons();
    }

    console.log("[refreshDashboard] Fetching dashboard...");
    const dashboard = await loadDashboard();
    console.log(
      "[refreshDashboard] Got dashboard, environments:",
      dashboard.environments?.length,
    );

    // The sidebar list, the hero, the service cards and the env vars all read
    // this store now (spec D6), so publishing the environments is what renders
    // them - there is nothing left to push into the DOM from here.
    const selectedProject = getState().selectedProject || "";
    setState({ environments: dashboard.environments, selectedProject });
    if (!selectedProject && dashboard.environments.length > 0) {
      setState({ selectedProject: projectKey(dashboard.environments[0]) });
    }
    // The list has its data now, so the loading frame is over either way: a
    // silent refresh never showed one, and this is what closes the non-silent one.
    envSkeleton.hide();

    try {
      refreshServiceSelector();
    } catch (e) {
      console.error("[refreshDashboard] refreshServiceSelector error:", e);
    }
    await refreshMetrics({ silent: true });
    await remotesApi.refresh({ silent: true });
    await globalServicesController.refresh({ silent: true });
    await logsApi.refresh();
    await loadFooterVersion();

    setStatus(`Status: Ready`);
  } catch (e) {
    console.error("[refreshDashboard] error:", e);
    setStatus(`Status: Error`);
  }
};

const onboardingController = createOnboardingController({
  bridge: desktopBridge,
  refs,
  onStatus: setStatus,
  onToast: showToast,
  onProjectAdded: refreshDashboard,
  onSelectProject: selectProject,
  onRunBootstrapSync: async ({ projectPath, remoteName, preset, config }) => {
    const normalizedProjectPath = String(projectPath || "").trim();
    const normalizedRemote = String(remoteName || "").trim();
    const normalizedPreset = String(preset || "full")
      .trim()
      .toLowerCase();
    if (!normalizedProjectPath || !normalizedRemote) {
      return;
    }

    // 1. Select the project (normalized name)
    const projectName = normalizedProjectPath.split(/[\\/]+/).filter(Boolean).pop();
    await selectProject(projectName || normalizedProjectPath);

    // 2. Clear onboarding modal if still open
    onboardingController.toggleModal(false);

    // 3. Switch to Remotes tab (this also calls remotesApi.refresh() internally)
    switchTab("remotes");

    // 4. Force refresh to finish before exposing progress UI
    await remotesApi.refresh();

    const resolvedConfig =
      config && Object.keys(config).length > 0
        ? { ...config }
        : await resolveSyncConfigForPreset(normalizedPreset);

    // 5. Run sync which will unhide the progress container in Remotes tab
    await remotesApi.runSync({
      project: projectName || normalizedProjectPath,
      remoteName: normalizedRemote,
      preset: normalizedPreset,
      config: resolvedConfig,
    });
  },
  getExistingDomains: () =>
    (getState().environments || [])
      .map((item) => ({
        domain: String(item?.domain || item?.Domain || "")
          .trim()
          .toLowerCase(),
        project: String(
          item?.project || item?.Project || item?.name || item?.Name || "",
        )
          .trim()
          .toLowerCase(),
      }))
      .filter((entry) => entry.domain),
});

const actionsController = createActionsController({
  bridge: desktopBridge,
  getProject: () => getState().selectedProject,
  refreshDashboard,
  renderSkeletons: renderAllSkeletons,
  onStatus: setStatus,
  onToast: showToast,
  onToastLoading: showLoadingToast,
});

const settingsController = createSettingsController({
  bridge: desktopBridge,
  refs,
  onStatus: setStatus,
  onToast: showToast,
});

// The update prompt is a React island over a headless model; the model owns the
// background check timers and the dismissed-version and settings-drawer rules.
// The preview (preview/bootstrap.js) asks for the model on window so a
// behaviour test can drive checks itself; production never defines that global.
const updateNotifierModel = createUpdateNotifierModel({
  settingsController,
  onStatus: setStatus,
  isSettingsDrawerOpen: () =>
    Boolean(refs.settingsDrawer) && !refs.settingsDrawer.classList.contains("hidden"),
});
const updatePromptIsland = mountIsland(
  "updatePromptIsland",
  createElement(UpdatePrompt, { model: updateNotifierModel }),
);
if (window.__govardPreviewExposeUpdatePrompt) window.__govardUpdatePromptModel = updateNotifierModel;

const setSettingsDrawerOpen = (open) => {
  settingsController.toggleDrawer(open);
  updateNotifierModel.syncWithSettingsDrawer();
};

// The settings drawer is an island that keeps its controller rather than porting
// it (spec, island contract): the controller's update state machine is covered by
// unit tests against fake refs, so the island renders the structure once and
// hands the controller the real elements through the seam it already had
// (updateRefs). main.js still creates the controller because the update-notifier
// model holds that exact object, and it keeps the open/close contract below,
// which isSettingsDrawerOpen and the update prompt both read.
const resetSettings = async () => {
  const confirmed = await showConfirm({
    title: "Reset Settings",
    message:
      "Are you sure you want to reset all settings to defaults?<br><small class='text-text-tertiary opacity-70'>This will overwrite your proxy, IDE, and UI preferences.</small>",
    icon: "restart_alt",
    confirmText: "Reset Defaults",
    cancelText: "Cancel",
  });

  if (confirmed) {
    await settingsController.reset();
  }
};

const settingsIsland = mountIsland(
  "settingsDrawerMount",
  createElement(SettingsDrawer, {
    bridge: desktopBridge,
    controller: settingsController,
    onOpenChange: setSettingsDrawerOpen,
    onResetSettings: resetSettings,
    registerRefs: (islandRefs) => {
      Object.assign(refs, islandRefs);
      settingsController.updateRefs(refs);
    },
  }),
);

// The dashboard's four surfaces are islands over the same store. Every control
// they own calls back into the code that already handled it: the environment
// actions live in actions.js (main.js only forwards), selectProject and
// openServiceContext are main.js's own, and the terminal/copy bodies are the
// delegate branches deleted with this change.
const environmentListIsland = mountIsland(
  "envList",
  createElement(EnvironmentList, {
    onSelect: (project) => void selectProject(project),
    onToggle: (project) => void actionsController.handle("toggle-env", project),
    onSwitchSidebarMode: () => void switchSidebarMode("global-services"),
    registerSkeleton: (api) => {
      envSkeleton = api;
    },
  }),
);
const projectHeroIsland = mountIsland(
  "projectHero",
  createElement(ProjectHero, {
    onAction: (action, project) => void actionsController.handle(action, project),
  }),
);
const activeServicesIsland = mountIsland(
  "activeServicesList",
  createElement(ActiveServices, {
    onOpenLogs: (project, service) =>
      void openServiceContext(project, service, "logs"),
    onOpenTerminal: (project, service) => {
      if (!project || !service) return;
      desktopBridge
        .startServiceTerminalInOS(project, service, "", "sh")
        .catch((err) => showToast(`Failed to launch OS Terminal: ${err}`, "error"));
    },
  }),
);
const envVarsIsland = mountIsland(
  "envVarsList",
  createElement(EnvVars, {
    onCopy: (text) => {
      if (!text) return;
      navigator.clipboard
        .writeText(text)
        .then(() => showToast("Copied to clipboard!", "success"))
        .catch((err) => showToast(`Failed to copy: ${err}`, "error"));
    },
  }),
);

const globalServicesController = createGlobalServicesController({
  bridge: desktopBridge,
  getState,
  setState,
  onStatus: setStatus,
  onToast: showToast,
});

// The ops deck and the card list are islands over that controller: it publishes
// the snapshot, the feedback line and the selected service into the store, and
// each island derives what it renders from them (D6). Their controls call back
// into the controller, which stays the only thing that talks to the bridge for
// these actions, so the routing-settle loop and the refresh cadence are
// unchanged. The Logs panel is still vanilla markup in this change.
const globalHealthIsland = mountIsland(
  "globalHealthIsland",
  createElement(GlobalHealthHeader, {
    onBulkAction: (action) => globalServicesController.runBulkAction(action),
  }),
);

const globalServicesListIsland = mountIsland(
  "globalServicesList",
  createElement(GlobalServicesList, {
    onSelectService: (serviceId) =>
      globalServicesController.selectService(serviceId),
    onServiceAction: (action, serviceId) =>
      globalServicesController.runServiceAction(action, serviceId),
  }),
);

// The log pane registers the two things main.js still drives in it: the two
// refreshes and the stop-on-leave. Its poll and its three event subscriptions
// belong to the island, so unmounting is what stops them.
let globalLogsApi = {
  refreshLogs: async () => {},
  stopLive: async () => {},
};

const globalLogsIsland = mountIsland(
  "globalLogsIsland",
  createElement(GlobalLogsPanel, {
    bridge: desktopBridge,
    onStatus: setStatus,
    onToast: showToast,
    onFeedback: (message, tone) => raiseActionFeedback(setState, message, tone),
    registerApi: (api) => {
      globalLogsApi = api;
    },
    pollMs: window.__govardPreviewGlobalLogsPollMs ?? 2000,
  }),
);
if (window.__govardPreviewGlobalLogsPollMs) {
  window.__govardGlobalLogsIsland = globalLogsIsland;
}

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
  event.preventDefault();

  if (action === "switch-sidebar-mode") {
    const mode = String(targetElement.dataset.mode || "").trim();
    await switchSidebarMode(mode);
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
  if (action === "confirm-onboarding-bootstrap") {
    await onboardingController.confirmBootstrapPrompt();
    return;
  }
  if (action === "skip-onboarding-bootstrap") {
    onboardingController.skipBootstrapPrompt();
    return;
  }
  if (action === "toggle-onboarding-bootstrap-option") {
    onboardingController.toggleBootstrapOption(
      String(targetElement.dataset.option || ""),
    );
    return;
  }
  if (action === "open-service-shell") {
    // Redirect to OS Terminal
    const project = targetElement.dataset.project || "";
    const service = targetElement.dataset.service || "";
    if (project && service) {
      try {
        await desktopBridge.startServiceTerminalInOS(project, service, "", "sh");
      } catch (err) {
        showToast(`Failed to launch OS Terminal: ${err}`, "error");
      }
    }
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
  if (action === "open-settings") {
    setSettingsDrawerOpen(true);
    return;
  }
  if (action === "close-settings") {
    setSettingsDrawerOpen(false);
    return;
  }
  if (action === "switch-tab") {
    const tab = targetElement.dataset.tab;
    if (tab) {
      switchTab(tab);
    }
    return;
  }
  if (action === "open-shell") {
    // Redirect to OS Terminal for the whole project
    const project = getState().selectedProject;
    if (project) {
      try {
        await desktopBridge.startServiceTerminalInOS(project, "web", "", "sh");
      } catch (err) {
        showToast(`Failed to launch OS Terminal: ${err}`, "error");
      }
    }
    return;
  }


  await actionsController.handle(action, targetElement.dataset.env || "");
});

const bindRuntimeListeners = () => {
  if (hasEventRuntime()) {
    onEvent("onboarding:progress", (payload = {}) => {
      onboardingController.handleProgress(payload);
    });
  }

  if (refs.refresh) {
    refs.refresh.addEventListener("click", () => {
      refreshDashboard();
    });
  }

  if (refs.openSettings) {
    refs.openSettings.addEventListener("click", () =>
      setSettingsDrawerOpen(true),
    );
  }

  if (refs.hardReset) {
    refs.hardReset.addEventListener("click", async () => {
      const confirmed = await showConfirm({
        title: "Restart Govard Desktop",
        message:
          "Are you sure you want to restart Govard Desktop?<br><small class='text-text-tertiary opacity-70'>This will reload the application process.</small>",
        icon: "restart_alt",
        confirmText: "Restart",
        cancelText: "Cancel",
      });

      if (!confirmed) {
        return;
      }
      showToast("Restarting application...", "info");
      try {
        await desktopBridge.restartDesktopApp();
      } catch (err) {
        showToast(`Restart failed: ${err}`, "error");
      }
    });
  }
};

document.addEventListener("keydown", (event) => {
  const target = event.target;
  if (target instanceof HTMLElement) {
    const isNativeControl = Boolean(
      target.closest("button, input, select, textarea, a"),
    );
    if (!isNativeControl && (event.key === "Enter" || event.key === " ")) {
      const actionTarget = target.closest('[data-action][role="button"]');
      if (actionTarget instanceof HTMLElement) {
        event.preventDefault();
        actionTarget.click();
        return;
      }
    }
  }

  if (event.key === "Escape") {
    setSettingsDrawerOpen(false);
  }
  if ((event.ctrlKey || event.metaKey) && event.key === ",") {
    event.preventDefault();
    setSettingsDrawerOpen(true);
  }
});

const syncProjectSelectorsFrom = async (source) => {
  if (source === "env") {
    // envSelector is the source of truth if logSelector is gone
  }
  syncProjectState();
  refreshServiceSelector();
  // The island decides whether a changed selection means "restart the stream"
  // or "reload the buffer" - the live mode lives there now.
  await logsApi.selectionChanged();
  await remotesApi.refresh({ silent: true });
};

const bindDynamicControlListeners = () => {
  if (refs.envSelector) {
    refs.envSelector.addEventListener("change", async () => {
      await syncProjectSelectorsFrom("env");
    });
  }



  if (refs.projectDomain) {
    refs.projectDomain.addEventListener("input", () => {
      onboardingController.handleInputChange();
    });
  }

  if (refs.projectFramework) {
    refs.projectFramework.addEventListener("change", () => {
      onboardingController.handleInputChange();
    });
  }

  if (refs.projectFrameworkVersion) {
    refs.projectFrameworkVersion.addEventListener("input", () => {
      onboardingController.handleInputChange();
    });
  }

  if (refs.onboardFromGit) {
    refs.onboardFromGit.addEventListener("change", () => {
      onboardingController.handleInputChange();
    });
  }

  if (refs.gitProtocol) {
    refs.gitProtocol.addEventListener("change", () => {
      onboardingController.handleInputChange();
    });
  }

  if (refs.gitUrl) {
    refs.gitUrl.addEventListener("input", () => {
      onboardingController.handleInputChange();
    });
  }

  if (refs.gitConfirmOverride) {
    refs.gitConfirmOverride.addEventListener("change", () => {
      onboardingController.handleInputChange();
    });
  }
};

if (window.matchMedia) {
  window
    .matchMedia("(prefers-color-scheme: dark)")
    .addEventListener("change", () => {
      if ((refs.themeSelect?.value || "system") === "system") {
        settingsController.load();
      }
    });
}

const bootstrap = async () => {
  try {
    setStatus("Status: Initializing...");
    setState({
      selectedService: "all",
      selectedSeverity: "all",
      logQuery: "",
      globalLogSeverity: "all",
      globalLogQuery: "",
    });

    // Run core loads in parallel
    await Promise.allSettled([
      loadUser(),
      loadFooterVersion(),
      settingsController.load(),
      refreshDashboard(),
      onboardingController.loadFrameworkOptions(),
    ]).catch((e) => console.error("Parallel bootstrap error:", e));
    if (getState().sidebarMode === "global-services") {
      await globalLogsApi.refreshLogs();
    }
    await loadFooterVersion();
    setTimeout(() => {
      loadFooterVersion();
    }, 1500);

    updateNotifierModel.scheduleBackgroundChecks();
    setStatus("Status: Ready");
  } catch (err) {
    console.error("Bootstrap fatal error:", err);
    setStatus("Status: Error");
  }
};

const initApp = () => {
  console.log("==> App Initializing! <==");
  try {
    initUI();
    bindRuntimeListeners();
    bindDynamicControlListeners();
    switchTab("dashboard");
    switchSidebarMode(getState().sidebarMode, {
      silent: true,
      skipLogs: true,
    }).catch((err) => {
      console.error("Failed to initialize sidebar mode:", err);
    });
    bootstrap();
  } catch (err) {
    console.error("Error in app initialization:", err);
  }
};

if (document.readyState === "loading") {
  window.addEventListener("DOMContentLoaded", initApp);
} else {
  initApp();
}

window.addEventListener("beforeunload", () => {
  updateNotifierModel.clearTimers();
  updatePromptIsland?.unmount();
  metricsIsland?.unmount();
  logsIsland?.unmount();
  settingsIsland?.unmount();
  environmentListIsland?.unmount();
  projectHeroIsland?.unmount();
  activeServicesIsland?.unmount();
  envVarsIsland?.unmount();
  remotesIsland?.unmount();
  syncModalIsland?.unmount();
  globalHealthIsland?.unmount();
  globalServicesListIsland?.unmount();
  globalLogsIsland?.unmount();
});
