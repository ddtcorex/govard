console.log("==> main.js top level loaded! <==");
import { createActionsController } from "./modules/actions.js?v=20260301";
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
  renderLogsTab,
} from "./modules/logs.js";
import { createMetricsController } from "./modules/metrics.js";
import {
  createOnboardingController,
  renderOnboardingModal,
} from "./modules/onboarding.js?v=20260302";
import {
  createRemotesController,
  renderRemotes,
  renderSyncModal,
} from "./modules/remotes.js";
import {
  createSettingsController,
  renderSettingsDrawer,
} from "./modules/settings.js";
import { createShellController } from "./modules/shell.js";
import { createTerminalController } from "./modules/terminal.js";
import { desktopBridge } from "./services/bridge.js";
import { getState, setState } from "./state/store.js";
import { createToast } from "./ui/toast.js?v=20260301";
import { byId, setText } from "./utils/dom.js";
console.log("==> Finished imports <==");

const initUI = () => {
  renderLogsTab(byId("tab-logs"));
  renderOnboardingModal(byId("onboardingModalMount"));
  // NOTE: do NOT call renderRemotes(tab-remotes) here — it wipes the remotesList/remotesWarnings
  // containers. The remotesController.refresh() handles rendering when the tab is opened.
  renderSyncModal(byId("syncOptionsModalMount"));
  renderSettingsDrawer(byId("settingsDrawerMount"));
  refreshRefs();
};

const getLiveRefs = () => ({
  refresh: byId("refresh"),
  status: byId("status"),
  envList: byId("envList"),
  envSelector: byId("envSelector"),
  logSelector: byId("logSelector"),
  logServiceSelector: byId("logServiceSelector"),
  logSeverity: byId("logSeverity"),
  logSearch: byId("logSearch"),
  logOutput: byId("logOutput"),
  toggleLive: byId("toggleLive"),
  warningList: byId("warningList"),
  openSettings: byId("openSettings"),
  closeSettings: byId("closeSettings"),
  settingsDrawer: byId("settingsDrawer"),
  themeSelect: byId("themeSelect"),
  proxyTarget: byId("proxyTarget"),
  preferredBrowser: byId("preferredBrowser"),
  codeEditor: byId("codeEditor"),
  shellUser: byId("shellUser"),
  userAvatar: byId("userAvatar"),
  userName: byId("userName"),
  terminalContainer: byId("terminalContainer"),
  terminalPanel: byId("terminalPanel"),
  terminalBackdrop: byId("terminalBackdrop"),
  terminalExpandIcon: byId("terminalExpandIcon"),
  toastContainer: byId("toastContainer"),
  onboardingModal: byId("onboardingModal"),
  projectPath: byId("projectPath"),
  displayProjectPath: byId("displayProjectPath"),
  projectPathHint: byId("projectPathHint"),
  projectDomain: byId("projectDomain"),
  projectDomainHint: byId("projectDomainHint"),
  projectFramework: byId("projectFramework"),
  onboardingSummaryProject: byId("onboardingSummaryProject"),
  onboardingSummaryFramework: byId("onboardingSummaryFramework"),
  onboardingSummaryDomain: byId("onboardingSummaryDomain"),
  onboardingSubmitHint: byId("onboardingSubmitHint"),
  onboardingSubmit: byId("onboardingSubmit"),
  onboardVarnish: byId("onboardVarnish"),
  onboardRedis: byId("onboardRedis"),
  onboardRabbitMQ: byId("onboardRabbitMQ"),
  onboardElasticsearch: byId("onboardElasticsearch"),
  newRemoteModal: byId("newRemoteModal"),
  projectTitle: byId("projectTitle"),
  projectStatusBadge: byId("projectStatusBadge"),
  projectStatusText: byId("projectStatusText"),
  projectUrl: byId("projectUrl"),
  projectUrlText: byId("projectUrlText"),
  projectTechnologies: byId("projectTechnologies"),
  heroRestartBtn: byId("heroRestartBtn"),
  heroStopBtn: byId("heroStopBtn"),
  footerVersion: byId("footerVersion"),
  footerCPU: byId("footerCPU"),
  footerMemory: byId("footerMemory"),
  envVarsList: byId("envVarsList"),
  remotesList: byId("remotesList"),
  remotesWarnings: byId("remotesWarnings"),
  syncOptionsModal: byId("syncOptionsModal"),
  syncModalStep1: byId("syncModalStep1"),
  syncModalStep2: byId("syncModalStep2"),
  syncModalTitle: byId("syncModalTitle"),
  syncModalIcon: byId("syncModalIcon"),
  syncModalRemoteName: byId("syncModalRemoteName"),
  syncPlanOutput: byId("syncPlanOutput"),
  syncPlanLoading: byId("syncPlanLoading"),
});

let refs = getLiveRefs();

const refreshRefs = () => {
  const newRefs = getLiveRefs();
  Object.assign(refs, newRefs);
  // Propagate updated refs to controllers if they don't hold the object by reference
  // (Most do, but we keep this for safety and explicit update triggers)
  if (logsController?.updateRefs) logsController.updateRefs(refs);
  if (shellController?.updateRefs) shellController.updateRefs(refs);
  if (metricsController?.updateRefs) metricsController.updateRefs(refs);
  if (remotesController?.updateRefs) remotesController.updateRefs(refs);
  if (settingsController?.updateRefs) settingsController.updateRefs(refs);
  if (embeddedTerminalController?.updateRefs)
    embeddedTerminalController.updateRefs(refs);
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
      const getVersion = window.go?.desktop?.App?.GetVersion;
      if (!getVersion) {
        throw new Error("version bridge not ready");
      }
      const version = String(await getVersion()).trim();
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
    if (tabId !== "logs" && getState().terminalModalOpen) {
      toggleTerminalModal(false);
    }

    if (tabId === "dashboard") {
      scrollContainer.classList.add("overflow-y-auto");
      scrollContainer.classList.remove("overflow-hidden");
      refreshDashboard();
    } else if (tabId === "remotes") {
      scrollContainer.classList.add("overflow-y-auto");
      scrollContainer.classList.remove("overflow-hidden");
      remotesController.refresh();
    } else if (tabId === "logs") {
      scrollContainer.classList.remove("overflow-y-auto");
      scrollContainer.classList.add("overflow-hidden");
      setState({ selectedService: "all" });
      refreshServiceSelector();
      logsController.refresh();
    }

    // Standardize header styling across all tabs
    tabs.className =
      "border-b border-[#22492f] shrink-0 bg-background-dark sticky top-0 z-10 w-full";
    tabsInner.className = "w-full";
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

const openServiceContext = async (project, service, mode = "logs") => {
  const selectedProject = String(project || getState().selectedProject || "")
    .trim();
  if (!selectedProject) {
    setStatus("Select an environment first.");
    return;
  }

  setSelectedProject(selectedProject);
  switchTab("logs");

  const selectedService = String(service || "all")
    .trim()
    .toLowerCase() || "all";
  setState({ selectedService });
  refreshServiceSelector();

  if (mode === "shell") {
    await shellController.openShell();
    return;
  }
  await logsController.refresh();
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

const embeddedTerminalController = createTerminalController({
  bridge: desktopBridge,
  runtime: desktopBridge.runtime,
  refs,
  onStatus: setStatus,
  onToast: showToast,
  readSelection,
});

const TERMINAL_MODAL_ANIMATION_MS = 360;
let terminalModalAnimationTimer = null;
let terminalModalAnimationId = 0;
let terminalModalTransitionHandler = null;

const clearTerminalPanelAnimation = (panel) => {
  if (terminalModalAnimationTimer) {
    clearTimeout(terminalModalAnimationTimer);
    terminalModalAnimationTimer = null;
  }
  if (panel && terminalModalTransitionHandler) {
    panel.removeEventListener("transitionend", terminalModalTransitionHandler);
    terminalModalTransitionHandler = null;
  }
};

const setTerminalModalLayout = (panel, isOpen) => {
  if (!panel) return;
  if (isOpen) {
    panel.classList.remove("h-1/3", "relative", "z-10");
    panel.classList.add(
      "fixed",
      "inset-4",
      "h-auto",
      "z-[140]",
      "border-primary/40",
      "shadow-2xl",
    );
    return;
  }

  panel.classList.remove(
    "fixed",
    "inset-4",
    "h-auto",
    "z-[140]",
    "border-primary/40",
    "shadow-2xl",
  );
  panel.classList.add("h-1/3", "relative", "z-10");
};

const animateTerminalPanelLayout = (panel, isOpen) => {
  if (!panel) return;

  clearTerminalPanelAnimation(panel);
  const animationId = ++terminalModalAnimationId;
  const firstRect = panel.getBoundingClientRect();
  setTerminalModalLayout(panel, isOpen);
  const lastRect = panel.getBoundingClientRect();

  const width = Math.max(lastRect.width, 1);
  const height = Math.max(lastRect.height, 1);
  const deltaX = firstRect.left - lastRect.left;
  const deltaY = firstRect.top - lastRect.top;
  const scaleX = firstRect.width / width;
  const scaleY = firstRect.height / height;

  panel.style.willChange = "transform, box-shadow, border-color";
  panel.style.transformOrigin = "top left";
  panel.style.transition = "none";
  panel.style.transform = `translate(${deltaX}px, ${deltaY}px) scale(${scaleX}, ${scaleY})`;
  // Force reflow so the browser applies the inverted transform before animation.
  void panel.offsetWidth;
  panel.style.transition =
    `transform ${TERMINAL_MODAL_ANIMATION_MS}ms cubic-bezier(0.22, 1, 0.36, 1), ` +
    `box-shadow ${TERMINAL_MODAL_ANIMATION_MS}ms cubic-bezier(0.22, 1, 0.36, 1), ` +
    `border-color ${TERMINAL_MODAL_ANIMATION_MS}ms cubic-bezier(0.22, 1, 0.36, 1)`;
  panel.style.transform = "translate(0px, 0px) scale(1, 1)";

  const finishAnimation = () => {
    if (animationId !== terminalModalAnimationId) return;
    clearTerminalPanelAnimation(panel);
    panel.style.transition = "";
    panel.style.transform = "";
    panel.style.transformOrigin = "";
    panel.style.willChange = "";
    embeddedTerminalController.resize();
    requestAnimationFrame(() => embeddedTerminalController.resize());
  };

  terminalModalTransitionHandler = (event) => {
    if (event.target !== panel || event.propertyName !== "transform") return;
    finishAnimation();
  };
  panel.addEventListener("transitionend", terminalModalTransitionHandler);
  terminalModalAnimationTimer = setTimeout(
    finishAnimation,
    TERMINAL_MODAL_ANIMATION_MS + 80,
  );
};

const toggleTerminalModal = (forceOpen) => {
  const panel = refs.terminalPanel;
  const backdrop = refs.terminalBackdrop;
  const icon = refs.terminalExpandIcon;
  if (!panel) return;

  const current = Boolean(getState().terminalModalOpen);
  const next = typeof forceOpen === "boolean" ? forceOpen : !current;
  if (next === current) return;

  setState({ terminalModalOpen: next });

  if (next) {
    if (backdrop) {
      backdrop.classList.remove("hidden");
      requestAnimationFrame(() => backdrop.classList.remove("opacity-0"));
    }
    if (icon) icon.textContent = "close_fullscreen";
    animateTerminalPanelLayout(panel, true);
  } else {
    if (backdrop) {
      backdrop.classList.add("opacity-0");
      setTimeout(() => {
        if (!getState().terminalModalOpen) {
          backdrop.classList.add("hidden");
        }
      }, TERMINAL_MODAL_ANIMATION_MS);
    }
    if (icon) icon.textContent = "open_in_full";
    animateTerminalPanelLayout(panel, false);
  }
};

const renderAllSkeletons = () => {
  renderDashboardSkeletons(refs);
  renderEnvironmentSkeletons(refs.envList);
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

    try {
      setMetricText(dashboard, refs);
    } catch (e) {
      console.error("[refreshDashboard] setMetricText error:", e);
    }
    try {
      renderWarnings(refs.warningList, dashboard.warnings);
    } catch (e) {
      console.error("[refreshDashboard] renderWarnings error:", e);
    }
    try {
      renderEnvironmentList(
        refs.envList,
        dashboard.environments,
        getState().selectedProject,
      );
      console.log(
        "[refreshDashboard] renderEnvironmentList called with",
        dashboard.environments?.length,
        "envs",
      );
    } catch (e) {
      console.error("[refreshDashboard] renderEnvironmentList error:", e);
    }

    const previousProject = getState().selectedProject;
    try {
      syncProjectSelectors(
        { envSelector: refs.envSelector, logSelector: refs.logSelector },
        dashboard.environments,
        previousProject,
      );
    } catch (e) {
      console.error("[refreshDashboard] syncProjectSelectors error:", e);
    }

    const selectedProject =
      refs.envSelector?.value || getState().selectedProject || "";
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

    try {
      refreshServiceSelector();
    } catch (e) {
      console.error("[refreshDashboard] refreshServiceSelector error:", e);
    }
    try {
      refreshSeveritySelector();
    } catch (e) {
      console.error("[refreshDashboard] refreshSeveritySelector error:", e);
    }
    try {
      syncLogFiltersState();
    } catch (e) {
      console.error("[refreshDashboard] syncLogFiltersState error:", e);
    }
    try {
      renderEnvironmentList(
        refs.envList,
        dashboard.environments,
        getState().selectedProject,
      );
    } catch (e) {
      console.error(
        "[refreshDashboard] second renderEnvironmentList error:",
        e,
      );
    }
    try {
      const { environments, selectedProject: project } = getState();
      const env = environments.find((item) => projectKey(item) === project);

      // Sync "Active Services" block as well
      const servicesContainer = document.getElementById("activeServicesList");
      if (servicesContainer && env) {
        import("./modules/dashboard.js").then((mod) => {
          mod.renderActiveServices(servicesContainer, env);
        });
      }

      renderProjectHero(refs, environments, project);
    } catch (e) {
      console.error("[refreshDashboard] renderProjectHero error:", e);
    }
    await metricsController.refresh({ silent: true });
    await remotesController.refresh({ silent: true });
    await shellController.loadShellUser();
    await logsController.refresh();
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
  getExistingDomains: () =>
    (getState().environments || [])
      .map((item) => ({
        domain: String(item?.domain || item?.Domain || "").trim().toLowerCase(),
        project: String(
          item?.project ||
            item?.Project ||
            item?.name ||
            item?.Name ||
            "",
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

  if (action === "select-environment") {
    const project = targetElement.dataset.env || "";
    setState({ selectedProject: project });
    if (refs.envSelector) refs.envSelector.value = project;
    if (refs.logSelector) refs.logSelector.value = project;

    const currentTab = document.querySelector(".tab-content.active")?.id;
    if (!currentTab || currentTab === "tab-dashboard") {
      switchTab("dashboard");
    }

    await syncProjectSelectorsFrom("env");
    await refreshDashboard({ silent: true });

    // Ensure controllers refresh if they are on the active tab
    const activeTabId = document
      .querySelector(".tab-content.active")
      ?.id?.replace("tab-", "");
    if (activeTabId === "logs") {
      logsController.refresh();
    } else if (activeTabId === "remotes") {
      remotesController.refresh();
    }
    return;
  }

  if (action === "copy-text") {
    const text = targetElement.dataset.text || "";
    if (text) {
      try {
        await navigator.clipboard.writeText(text);
        showToast("Copied to clipboard!", "success");
      } catch (err) {
        showToast(`Failed to copy: ${err}`, "error");
      }
    }
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
  if (action === "filter-severity") {
    const sev = targetElement.dataset.severity;
    if (sev) {
      setState({ selectedSeverity: sev });
      logsController.applyFilters();
      refreshSeveritySelector();
    }
    return;
  }
  if (action === "filter-service") {
    const svc = targetElement.dataset.service;
    if (svc) {
      setState({ selectedService: svc });
      refreshServiceSelector();
      if (logsController.isLiveEnabled()) {
        await logsController.stopLive();
        await logsController.toggleLive();
      } else {
        await logsController.refresh();
      }
    }
    return;
  }
  if (action === "open-service-logs") {
    await openServiceContext(
      targetElement.dataset.project,
      targetElement.dataset.service,
      "logs",
    );
    return;
  }
  if (action === "open-service-shell") {
    await openServiceContext(
      targetElement.dataset.project,
      targetElement.dataset.service,
      "shell",
    );
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
    await remotesController.testRemote(
      String(targetElement.dataset.remote || ""),
    );
    return;
  }
  if (action === "open-remote-url") {
    console.log("[Main] open-remote-url", targetElement.dataset.remote);
    await shellController.openRemoteURL(
      String(targetElement.dataset.remote || ""),
    );
    return;
  }
  if (action === "open-remote-shell") {
    console.log("[Main] open-remote-shell", targetElement.dataset.remote);
    switchTab("logs");
    await shellController.openRemoteShell(
      String(targetElement.dataset.remote || ""),
    );
    return;
  }
  if (action === "open-remote-db") {
    console.log("[Main] open-remote-db", targetElement.dataset.remote);
    switchTab("logs");
    await shellController.openRemoteDB(
      String(targetElement.dataset.remote || ""),
    );
    return;
  }
  if (action === "open-remote-sftp") {
    console.log("[Main] open-remote-sftp", targetElement.dataset.remote);
    switchTab("logs");
    await shellController.openRemoteSFTP(
      String(targetElement.dataset.remote || ""),
    );
    return;
  }
  if (action === "open-settings") {
    settingsController.toggleDrawer(true);
    return;
  }
  if (action === "close-settings") {
    settingsController.toggleDrawer(false);
    return;
  }
  if (action === "reset-settings") {
    await settingsController.reset();
    return;
  }
  if (action === "switch-tab") {
    const tab = targetElement.dataset.tab;
    if (tab) {
      switchTab(tab);
    }
    return;
  }
  if (action === "open-sync-modal") {
    const remote = String(targetElement.dataset.remote || "");
    const preset = String(targetElement.dataset.preset || "");
    if (!remote || !preset) return;

    setState({ currentSyncRemote: remote, currentSyncPreset: preset });

    if (refs.syncModalRemoteName) {
      refs.syncModalRemoteName.textContent = remote;
    }

    try {
      const payload = await desktopBridge.getSyncPresetOptions(preset);
      const optionsDef = payload.options || [];

      const state = getState();
      const presetConfigs = state.syncConfigs || {};
      let config = presetConfigs[preset] || {};
      let changed = false;

      optionsDef.forEach((opt) => {
        if (config[opt.key] === undefined) {
          config[opt.key] = opt.defaultValue;
          changed = true;
        }
      });

      if (changed) {
        setState({
          syncConfigs: { ...presetConfigs, [preset]: config },
          currentSyncPresetDefs: optionsDef,
        });
      } else {
        setState({ currentSyncPresetDefs: optionsDef });
      }

      const container = document.getElementById("syncModalOptionsContainer");
      if (container) {
        remotesController.renderSyncOptions(
          container,
          preset,
          optionsDef,
          getState().syncConfigs[preset],
        );
      }
    } catch (err) {
      console.error("Failed to load sync options", err);
    }

    if (refs.syncOptionsModal) {
      refs.syncOptionsModal.classList.remove("hidden");
      // Always reset back to step 1 when opening
      if (refs.syncModalStep1) refs.syncModalStep1.classList.remove("hidden");
      if (refs.syncModalStep2) refs.syncModalStep2.classList.add("hidden");
      if (refs.syncModalTitle) refs.syncModalTitle.textContent = "Sync Options";
      if (refs.syncModalIcon) refs.syncModalIcon.textContent = "sync";
      setTimeout(() => {
        refs.syncOptionsModal.classList.remove("opacity-0");
        refs.syncOptionsModal.firstElementChild.classList.remove("scale-95");
      }, 10);
    }
    return;
  }
  if (action === "close-sync-modal") {
    if (refs.syncOptionsModal) {
      refs.syncOptionsModal.classList.add("opacity-0");
      refs.syncOptionsModal.firstElementChild.classList.add("scale-95");
      setTimeout(() => {
        refs.syncOptionsModal.classList.add("hidden");
        // Reset to step 1 after close animation
        if (refs.syncModalStep1) refs.syncModalStep1.classList.remove("hidden");
        if (refs.syncModalStep2) refs.syncModalStep2.classList.add("hidden");
        if (refs.syncModalTitle)
          refs.syncModalTitle.textContent = "Sync Options";
        if (refs.syncModalIcon) refs.syncModalIcon.textContent = "sync";
      }, 300);
    }
    return;
  }
  if (action === "back-to-sync-options") {
    if (refs.syncModalStep1) refs.syncModalStep1.classList.remove("hidden");
    if (refs.syncModalStep2) refs.syncModalStep2.classList.add("hidden");
    if (refs.syncModalTitle) refs.syncModalTitle.textContent = "Sync Options";
    if (refs.syncModalIcon) refs.syncModalIcon.textContent = "sync";
    return;
  }
  if (action === "preview-sync-plan") {
    const { currentSyncRemote, currentSyncPreset } = getState();
    if (!currentSyncRemote || !currentSyncPreset) return;

    const config = (getState().syncConfigs || {})[currentSyncPreset] || {};

    // Show step 2 with loading state
    if (refs.syncModalStep1) refs.syncModalStep1.classList.add("hidden");
    if (refs.syncModalStep2) refs.syncModalStep2.classList.remove("hidden");
    if (refs.syncModalTitle) refs.syncModalTitle.textContent = "Sync Preview";
    if (refs.syncModalIcon) refs.syncModalIcon.textContent = "fact_check";
    if (refs.syncPlanOutput) refs.syncPlanOutput.textContent = "";
    if (refs.syncPlanLoading) refs.syncPlanLoading.classList.remove("hidden");
    if (refs.syncPlanOutput) refs.syncPlanOutput.classList.add("hidden");

    const currentProject = getState().selectedProject;
    if (!currentProject) return;

    try {
      const plan = await desktopBridge.runRemoteSyncPreset(
        currentProject,
        currentSyncRemote,
        currentSyncPreset,
        config,
      );
      if (refs.syncPlanOutput) {
        refs.syncPlanOutput.textContent = String(
          plan || "No plan details returned.",
        );
        refs.syncPlanOutput.classList.remove("hidden");
      }
    } catch (err) {
      if (refs.syncPlanOutput) {
        refs.syncPlanOutput.textContent = `Failed to generate plan: ${err}`;
        refs.syncPlanOutput.classList.remove("hidden");
      }
    } finally {
      if (refs.syncPlanLoading) refs.syncPlanLoading.classList.add("hidden");
    }
    return;
  }
  if (action === "toggle-sync-config") {
    const configKey = targetElement.dataset.config;
    const preset = targetElement.dataset.preset;
    if (configKey && preset) {
      const currentConfigs = getState().syncConfigs || {};
      const currentConfig = currentConfigs[preset] || {};

      const nextConfig = remotesController.toggleSyncConfig(
        preset,
        configKey,
        currentConfig,
        (cfg) =>
          setState({ syncConfigs: { ...currentConfigs, [preset]: cfg } }),
      );

      const container = document.getElementById("syncModalOptionsContainer");
      if (container) {
        remotesController.renderSyncOptions(
          container,
          preset,
          getState().currentSyncPresetDefs,
          nextConfig,
        );
      }
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
  if (action === "start-embedded-terminal") {
    await embeddedTerminalController.startSession();
    return;
  }
  if (action === "toggle-terminal-modal") {
    toggleTerminalModal();
    return;
  }
  if (action === "close-terminal-modal") {
    toggleTerminalModal(false);
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
  // (filter-service and filter-severity are handled above)
  if (action === "reset-settings") {
    await settingsController.reset();
    return;
  }

  if (action === "switch-tab") {
    const tabId = targetElement.dataset.tab;
    if (tabId) switchTab(tabId);
    return;
  }
  if (action === "confirm-sync") {
    const { currentSyncRemote, currentSyncPreset } = getState();
    if (!currentSyncRemote || !currentSyncPreset) return;

    const config = (getState().syncConfigs || {})[currentSyncPreset] || {};

    // Close the modal
    if (refs.syncOptionsModal) {
      refs.syncOptionsModal.classList.add("opacity-0");
      refs.syncOptionsModal.firstElementChild.classList.add("scale-95");
      setTimeout(() => {
        refs.syncOptionsModal.classList.add("hidden");
      }, 300);
    }

    const currentProject = getState().selectedProject;
    if (!currentProject) return;

    // Show a persistent streaming toast
    const presetLabel =
      currentSyncPreset === "full" || currentSyncPreset === "bootstrap"
        ? `Setting up from ${currentSyncRemote}...`
        : currentSyncPreset === "db"
          ? `Pulling database from ${currentSyncRemote}...`
          : currentSyncPreset === "media"
            ? `Pulling media from ${currentSyncRemote}...`
            : `Syncing from ${currentSyncRemote}...`;
    const streamingToast = toast.showStreaming(presetLabel, "info");

    // Setup one-time streaming event listeners
    let offStream, offCompleted, offFailed;
    if (desktopBridge.runtime?.EventsOn) {
      offStream = desktopBridge.runtime.EventsOn("sync:output", (line) => {
        if (streamingToast) streamingToast.update(String(line || ""));
      });
      offCompleted = desktopBridge.runtime.EventsOn("sync:completed", (msg) => {
        if (streamingToast)
          streamingToast.close(String(msg || "Sync completed ✔"), "success");
        if (offStream) offStream();
        if (offFailed) offFailed();
      });
      offFailed = desktopBridge.runtime.EventsOn("sync:failed", (msg) => {
        if (streamingToast)
          streamingToast.close(String(msg || "Sync failed"), "error");
        if (offStream) offStream();
        if (offCompleted) offCompleted();
      });
    }

    // Trigger the background sync command via the bridge
    const result = await desktopBridge.runRemoteSyncBackground(
      currentProject,
      currentSyncRemote,
      currentSyncPreset,
      config,
    );
    if (result && result.startsWith("Remote sync background process failed:")) {
      if (streamingToast) streamingToast.close(result, "error");
      if (offStream) offStream();
      if (offCompleted) offCompleted();
      if (offFailed) offFailed();
    }

    return;
  }

  await actionsController.handle(action, targetElement.dataset.env || "");
});

const bindRuntimeListeners = () => {
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

const bindDynamicControlListeners = () => {
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
    });

    // Run core loads in parallel
    await Promise.allSettled([
      loadUser(),
      loadFooterVersion(),
      settingsController.load(),
      refreshDashboard(),
    ]).catch((e) => console.error("Parallel bootstrap error:", e));
    await loadFooterVersion();
    setTimeout(() => {
      loadFooterVersion();
    }, 1500);

    metricsController.startAutoRefresh();
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
  metricsController.stopAutoRefresh();
});
