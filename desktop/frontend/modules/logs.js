import { projectKey, serviceTargets } from "./dashboard.js";

const errorPattern = /\b(error|critical|fail|failed|exception|fatal|panic)\b/i;
const warnPattern = /\b(warn|warning|deprecated)\b/i;

const sanitizeLogFilenameToken = (value, fallback) =>
  String(value || "")
    .trim()
    .replace(/[^a-zA-Z0-9._-]+/g, "-")
    .replace(/^-+|-+$/g, "") || fallback;

export const buildLogFilename = ({
  scope = "logs",
  project = "",
  service = "all",
} = {}) => {
  const stamp = new Date()
    .toISOString()
    .replace(/[:.]/g, "-")
    .replace("T", "_")
    .replace("Z", "");
  return `govard-${sanitizeLogFilenameToken(scope, "logs")}-${sanitizeLogFilenameToken(project, "project")}-${sanitizeLogFilenameToken(service, "all")}-${stamp}.log`;
};

export const downloadTextAsFile = (
  content = "",
  filename = "govard-logs.log",
) => {
  const output = String(content || "");
  if (!output.trim()) {
    return false;
  }
  if (typeof document === "undefined" || typeof URL === "undefined") {
    return false;
  }
  try {
    const blob = new Blob([output.endsWith("\n") ? output : `${output}\n`], {
      type: "text/plain;charset=utf-8",
    });
    const href = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = href;
    anchor.download =
      String(filename || "govard-logs.log").trim() || "govard-logs.log";
    anchor.style.display = "none";
    document.body.appendChild(anchor);
    anchor.click();

    const cleanup = () => {
      try {
        URL.revokeObjectURL(href);
      } catch (_err) {
        // Ignore cleanup errors.
      }
      anchor.remove();
    };
    if (
      typeof window !== "undefined" &&
      typeof window.setTimeout === "function"
    ) {
      window.setTimeout(cleanup, 1500);
    } else {
      cleanup();
    }
    return true;
  } catch (_err) {
    return false;
  }
};

export const normalizeLogSeverity = (severity = "all") => {
  const normalized = String(severity || "all")
    .trim()
    .toLowerCase();
  if (["all", "error", "warn", "info"].includes(normalized)) {
    return normalized;
  }
  return "all";
};

export const classifyLogSeverity = (line = "") => {
  const text = String(line || "");
  if (errorPattern.test(text)) {
    return "error";
  }
  if (warnPattern.test(text)) {
    return "warn";
  }
  return "info";
};

export const filterLogsText = (raw = "", severity = "all", query = "") => {
  const selectedSeverity = normalizeLogSeverity(severity);
  const normalizedQuery = String(query || "")
    .trim()
    .toLowerCase();

  const lines = String(raw || "").split("\n");
  const filtered = lines.filter((line) => {
    if (
      selectedSeverity !== "all" &&
      classifyLogSeverity(line) !== selectedSeverity
    ) {
      return false;
    }
    if (
      normalizedQuery !== "" &&
      !line.toLowerCase().includes(normalizedQuery)
    ) {
      return false;
    }
    return true;
  });
  return filtered.join("\n").trim();
};

export const resolveLogTarget = ({
  project = "",
  service = "all",
  severity = "all",
  query = "",
} = {}) => ({
  project: String(project || "").trim(),
  service: String(service || "all").trim() || "all",
  severity: normalizeLogSeverity(severity),
  query: String(query || "").trim(),
});

export const syncServiceSelector = (
  container,
  environments,
  project,
  selectedService = "all",
) => {
  if (!container) {
    return "all";
  }
  const env = environments.find((item) => projectKey(item) === project);
  const targets = env ? serviceTargets(env) : ["web"];
  const mergedTargets = [
    "all",
    ...targets.filter((target) => target !== "all"),
  ];

  container.innerHTML = mergedTargets
    .map((target) => {
      const isActive = target === selectedService;
      const baseClass =
        "h-7 px-3 rounded-md text-xs font-semibold whitespace-nowrap border transition-colors";
      const activeClass =
        "bg-primary/20 text-primary border-primary/30 shadow-[0_0_12px_rgba(13,242,89,0.1)]";
      const inactiveClass =
        "text-slate-500 dark:text-primary border-transparent hover:text-primary dark:hover:text-white hover:bg-slate-100 dark:hover:bg-[var(--border-primary)]/60";

      return `<button 
      data-action="filter-service" 
      data-service="${target}" 
      class="${baseClass} ${isActive ? activeClass : inactiveClass}"
    >
      ${target}
    </button>`;
    })
    .join("");

  return mergedTargets.includes(selectedService)
    ? selectedService
    : mergedTargets[0];
};

export const syncSeveritySelector = (container, selectedSeverity = "all") => {
  if (!container) {
    return "all";
  }
  const severities = ["all", "error", "warn"];
  const buttons = container.querySelectorAll("button[data-severity]");

  buttons.forEach((btn) => {
    const sev = btn.dataset.severity;
    const isActive = sev === selectedSeverity;
    const baseClass =
      "h-7 px-3 text-[10px] font-bold uppercase tracking-wide rounded-md border transition-colors";
    const activeClass = "bg-primary/20 text-primary border-primary/30";
    const inactiveClass =
      "bg-slate-50 dark:bg-surface-secondary text-slate-500 dark:text-primary border-transparent hover:bg-slate-100 dark:hover:bg-surface-primary hover:text-primary dark:hover:text-white";

    btn.className = `${baseClass} ${isActive ? activeClass : inactiveClass}`;
  });

  return selectedSeverity;
};

export const createLogsController = ({
  bridge,
  runtime,
  refs,
  readSelection,
  onStatus,
  onToast,
}) => {
  const updateRefs = (newRefs) => {
    refs = newRefs;
  };
  let livePoll = null;
  let liveEnabled = false;
  let rawLogOutput = "";

  const resolveOutputViewport = () =>
    refs.logOutputViewport || refs.logOutput?.parentElement || null;

  const scrollToLatest = (force = false) => {
    if (!force && !liveEnabled) {
      return;
    }
    const viewport = resolveOutputViewport();
    if (!viewport) {
      return;
    }
    viewport.scrollTop = viewport.scrollHeight;
  };

  const renderFilteredOutput = ({ forceScroll = false } = {}) => {
    if (!refs.logOutput) {
      return;
    }
    const { severity, query } = readSelection();
    const filtered = filterLogsText(rawLogOutput, severity, query);
    refs.logOutput.textContent =
      filtered || "No logs match the current filters.";
    scrollToLatest(forceScroll);
  };

  const appendLogLine = (line) => {
    rawLogOutput = rawLogOutput
      ? `${rawLogOutput}\n${line}`
      : String(line || "");
    renderFilteredOutput();
  };

  const refresh = async () => {
    const { project, service } = readSelection();
    if (!project) {
      if (refs.logOutput) {
        refs.logOutput.textContent = "Select an environment to view logs.";
      }
      rawLogOutput = "";
      scrollToLatest(true);
      return;
    }
    if (refs.logOutput) {
      refs.logOutput.textContent = "Loading logs...";
    }
    try {
      const logs = await bridge.getLogsForService(project, service);
      rawLogOutput = String(logs || "");
      renderFilteredOutput({ forceScroll: true });
    } catch (err) {
      rawLogOutput = "";
      if (refs.logOutput) {
        refs.logOutput.textContent = `Failed to load logs: ${err}`;
      }
      scrollToLatest(true);
    }
  };

  const stopLive = async () => {
    liveEnabled = false;
    if (refs.toggleLive) {
      refs.toggleLive.textContent = "Live: Off";
    }
    if (livePoll) {
      clearInterval(livePoll);
      livePoll = null;
    }
    try {
      await bridge.stopLogStream();
    } catch (_err) {
      // Fallback polling mode may not have a stream to stop.
    }
  };

  const startLive = async () => {
    const { project, service } = readSelection();
    if (!project) {
      onStatus("Select an environment to stream logs.");
      return;
    }

    liveEnabled = true;
    if (refs.toggleLive) {
      refs.toggleLive.textContent = "Live: On";
    }

    if (bridge.startLogStreamForService && runtime?.EventsOn) {
      try {
        await bridge.startLogStreamForService(project, service);
        return;
      } catch (_err) {
        // Fall back to polling.
      }
    }
    await refresh();
    livePoll = setInterval(refresh, 2000);
  };

  const toggleLive = async () => {
    if (liveEnabled) {
      await stopLive();
      return;
    }
    await startLive();
  };

  const clearLogs = async () => {
    await stopLive();
    rawLogOutput = "";
    renderFilteredOutput({ forceScroll: true });
    onStatus("Logs cleared.");
    onToast("Logs cleared successfully.", "success");
  };

  const downloadLogs = async () => {
    const output = String(rawLogOutput || "").trim();
    if (!output) {
      onStatus("No logs available to download.");
      onToast("No logs available to download.", "warning");
      return;
    }
    const { project, service } = readSelection();
    const filename = buildLogFilename({
      scope: "environment",
      project,
      service,
    });

    let nativeExportError = null;
    if (bridge?.saveLogsToFile) {
      try {
        const response = await bridge.saveLogsToFile(output, filename);
        const message = String(response || "").trim();
        if (message.toLowerCase().includes("cancelled")) {
          onStatus(message || "Log export cancelled.");
          return;
        }
        onStatus(message || "Logs downloaded successfully.");
        onToast("Logs downloaded successfully.", "success");
        return;
      } catch (err) {
        nativeExportError = err;
      }
    }

    const downloaded = downloadTextAsFile(output, filename);
    if (!downloaded) {
      const details =
        nativeExportError !== null
          ? `Failed to download logs: ${nativeExportError}`
          : "Failed to download logs.";
      onStatus(details);
      onToast("Failed to download logs.", "error");
      return;
    }
    onStatus("Logs downloaded successfully.");
    onToast("Logs downloaded successfully.", "success");
  };

  if (runtime?.EventsOn) {
    runtime.EventsOn("logs:line", appendLogLine);
    runtime.EventsOn("logs:status", (message) => {
      const text = String(message || "").trim();
      if (!text) {
        return;
      }
      onStatus(text);
      onToast(text, "success");
    });
    runtime.EventsOn("logs:error", (message) => {
      const text =
        message && typeof message === "object"
          ? String(message.message || "")
          : String(message || "").trim();
      if (!text) {
        return;
      }
      onStatus(text);
      onToast(text, "error");
    });
  }

  return {
    refresh,
    applyFilters: renderFilteredOutput,
    toggleLive,
    clearLogs,
    downloadLogs,
    stopLive,
    isLiveEnabled: () => liveEnabled,
  };
};

export const renderLogsTab = (container) => {
  if (!container) return;
  container.innerHTML = `
      <div
        class="px-6 lg:px-10 py-6 max-w-[1248px] w-full mx-auto flex-1 flex flex-col gap-6 overflow-hidden h-full"
      >
        <div
          id="terminalBackdrop"
          data-action="close-terminal-modal"
          class="hidden fixed inset-0 bg-black/60 backdrop-blur-[2px] z-[135] opacity-0 transition-opacity duration-300"
        ></div>
        <div
          class="flex-1 flex flex-col gap-4 overflow-hidden h-full min-h-0"
        >
          <div
            class="flex-1 flex flex-col rounded-xl border border-slate-200 dark:border-border-primary bg-white dark:bg-background-primary overflow-hidden shadow-lg relative"
          >
            <div
              class="p-3 border-b border-slate-200 dark:border-border-primary bg-slate-50 dark:bg-surface-primary"
            >
              <div class="flex flex-wrap items-center justify-between gap-3">
                <div class="flex items-center gap-4 min-w-0">
                  <span class="material-symbols-outlined text-primary text-lg"
                    >receipt_long</span
                  >
                  <h3 class="text-sm font-semibold text-slate-800 dark:text-white">Logs</h3>
                  <span class="text-[10px] uppercase tracking-wide text-slate-600 dark:text-primary bg-slate-100 dark:bg-surface-secondary border border-slate-200 dark:border-border-primary px-3 py-1 rounded-full font-bold">Live Stream</span>
                </div>
                <div class="flex items-center gap-2 ml-auto">
                  <div class="relative">
                    <span class="absolute inset-y-0 left-2 flex items-center">
                      <span
                        class="material-symbols-outlined text-text-tertiary text-base"
                        >search</span
                      >
                    </span>
                    <input
                      class="bg-slate-50 dark:bg-surface-secondary text-xs text-slate-900 dark:text-white pl-8 pr-3 py-2 rounded-md border border-slate-200 dark:border-border-primary focus:border-primary/50 focus:outline-none placeholder-slate-400 dark:placeholder-[var(--text-tertiary)] w-44 md:w-56"
                      placeholder="Filter logs..."
                      type="text"
                      id="logSearch"
                    />
                  </div>
                  <button
                    data-action="clear-logs"
                    class="p-2 rounded-md text-primary hover:text-primary hover:bg-[var(--border-primary)]/50 transition-colors"
                    title="Clear Logs"
                  >
                    <span class="material-symbols-outlined text-lg"
                      >block</span
                    >
                  </button>
                  <button
                    data-action="download-logs"
                    class="p-2 rounded-md text-slate-500 dark:text-primary hover:text-primary hover:bg-slate-100 dark:hover:bg-[var(--border-primary)]/50 transition-colors"
                    title="Download Logs"
                  >
                    <span class="material-symbols-outlined text-lg"
                      >download</span
                    >
                  </button>
                </div>
              </div>
              <div class="mt-3 flex flex-col gap-2 xl:flex-row xl:items-center xl:justify-between">
                <div class="flex flex-col gap-2 min-w-0 flex-1 lg:flex-row lg:items-center">
                  <div
                    class="flex items-center gap-2 bg-slate-50 dark:bg-surface-secondary rounded-lg p-1.5 border border-slate-200 dark:border-border-primary min-w-0 flex-1"
                  >
                    <span class="h-7 inline-flex items-center leading-none text-[10px] uppercase tracking-wide text-text-tertiary px-1 shrink-0"
                      >Service</span
                    >
                    <div
                      id="logServiceSelector"
                      class="flex items-center gap-1 min-w-0 flex-1 overflow-x-auto service-strip-scroll"
                    >
                      <!-- Service buttons will be rendered here -->
                      <button
                        data-action="filter-service"
                        data-service="all"
                        class="h-7 px-3 rounded-md text-xs font-semibold whitespace-nowrap border bg-primary/20 text-primary border-primary/30"
                      >
                        all
                      </button>
                    </div>
                  </div>
                  <div
                    class="flex items-center gap-2 bg-slate-50 dark:bg-surface-secondary rounded-lg p-1.5 border border-slate-200 dark:border-border-primary shrink-0"
                  >
                    <span class="text-[10px] uppercase tracking-wide text-text-tertiary px-1 shrink-0"
                      >Severity</span
                    >
                    <div
                      id="logSeverity"
                      class="flex gap-1"
                    >
                      <button
                        data-action="filter-severity"
                        data-severity="all"
                        class="h-7 px-3 text-[10px] font-bold uppercase tracking-wide rounded-md border transition-colors bg-primary/20 text-primary border-primary/30"
                      >
                        All
                      </button>
                      <button
                        data-action="filter-severity"
                        data-severity="error"
                        class="h-7 px-3 text-[10px] font-bold uppercase tracking-wide rounded-md border transition-colors bg-slate-50 dark:bg-surface-secondary text-slate-500 dark:text-primary border-transparent hover:bg-slate-100 dark:hover:bg-surface-primary hover:text-primary dark:hover:text-white"
                      >
                        Error
                      </button>
                      <button
                        data-action="filter-severity"
                        data-severity="warn"
                        class="h-7 px-3 text-[10px] font-bold uppercase tracking-wide rounded-md border transition-colors bg-slate-50 dark:bg-surface-secondary text-slate-500 dark:text-primary border-transparent hover:bg-slate-100 dark:hover:bg-surface-primary hover:text-primary dark:hover:text-white"
                      >
                        Warn
                      </button>
                    </div>
                  </div>
                </div>
                <div class="flex items-center gap-2 shrink-0">
                  <button
                    id="toggleLive"
                    data-action="toggle-live"
                    class="h-8 px-3 rounded-md text-xs font-semibold bg-primary text-background-dark hover:bg-primary/90 transition-colors"
                  >
                    Live: Off
                  </button>
                  <button
                    data-action="refresh-logs"
                    class="h-8 px-3 rounded-md text-xs font-semibold text-slate-600 dark:text-primary hover:text-primary dark:hover:text-white hover:bg-slate-100 dark:hover:bg-[var(--border-primary)]/50 transition-colors"
                  >
                    Refresh
                  </button>
                </div>
              </div>
            </div>
            <div
              id="logOutputViewport"
              class="flex-1 overflow-y-auto px-4 pb-4 pt-2 terminal-text text-xs bg-slate-900 dark:bg-background-primary custom-scrollbar log-pane-scroll text-slate-300 dark:text-slate-300"
            >
              <pre id="logOutput" class="m-0 font-mono whitespace-pre-wrap">Select an environment to view logs.</pre>
            </div>
          </div>
          <div
            class="h-1.5 bg-slate-200 dark:bg-surface-primary hover:bg-primary/50 cursor-row-resize flex items-center justify-center rounded transition-colors group/resizer"
          >
            <div
              class="w-10 h-1 bg-slate-400 dark:bg-[var(--border-primary)] rounded-full group-hover/resizer:bg-white/50"
            ></div>
          </div>
          <div
            id="terminalPanel"
            class="h-1/3 min-h-0 flex flex-col rounded-xl border border-slate-200 dark:border-border-primary bg-slate-900 dark:bg-background-primary overflow-hidden shadow-lg relative z-10 transform-gpu"
          >
            <div
              class="flex items-center justify-between p-2 pl-3 border-b border-slate-200 dark:border-border-primary bg-slate-50 dark:bg-surface-primary"
            >
              <div class="flex items-center gap-2">
                <span
                  class="material-symbols-outlined text-slate-400 text-sm"
                  >terminal</span
                >
                <span class="text-xs font-semibold text-slate-800 dark:text-slate-300"
                  >Terminal</span
                >
              </div>
              <div class="flex items-center gap-2">
                <button
                  data-action="start-embedded-terminal"
                  class="px-2.5 py-1 rounded text-xs font-medium bg-primary text-background-dark hover:bg-primary/90 transition-colors border border-primary/30"
                  title="Open Shell"
                >
                  Open Shell
                </button>
                <button
                  data-action="restart-terminal-session"
                  class="px-2.5 py-1 rounded text-xs font-medium bg-slate-100 dark:bg-surface-secondary text-slate-800 dark:text-slate-100 hover:bg-slate-200 dark:hover:bg-background-secondary transition-colors border border-slate-200 dark:border-border-primary"
                  title="Terminate current session and open a new one"
                >
                  Restart Session
                </button>
                <button
                  data-action="toggle-terminal-modal"
                  class="p-1.5 rounded hover:bg-slate-200 dark:hover:bg-white/10 text-slate-500 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white transition-colors"
                  title="Expand Terminal"
                >
                  <span
                    id="terminalExpandIcon"
                    class="material-symbols-outlined text-sm"
                    >open_in_full</span
                  >
                </button>
                <select id="shellUser" class="hidden">
                  <option value="">Auto</option>
                </select>
                <select id="shellCommand" class="hidden">
                  <option value="sh">sh</option>
                </select>
              </div>
            </div>
            <div
              class="flex-1 min-h-0 overflow-hidden bg-slate-900 dark:bg-background-primary"
            >
              <div
                id="terminalContainer"
                class="h-full min-h-0 w-full overflow-hidden bg-background-primary"
              ></div>
            </div>
          </div>
        </div>
      </div>
  `;
};
