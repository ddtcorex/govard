import { projectKey, serviceTargets } from "./dashboard.js";

const errorPattern = /\b(error|critical|fail|failed|exception|fatal|panic)\b/i;
const warnPattern = /\b(warn|warning|deprecated)\b/i;

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
        "bg-[#2e573a] text-white border-[#3f7a52] shadow-[0_0_0_1px_rgba(63,122,82,0.45)]";
      const inactiveClass =
        "text-[#90cba4] border-transparent hover:text-white hover:bg-[#2e573a]/60";

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
    const activeClass = "bg-[#2e573a] text-primary border-[#3f7a52]";
    const inactiveClass =
      "bg-[#102316] text-[#90cba4] border-transparent hover:bg-[#1a3322] hover:text-white";

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

  const buildLogFilename = (project, service) => {
    const sanitize = (value, fallback) =>
      String(value || "")
        .trim()
        .replace(/[^a-zA-Z0-9._-]+/g, "-")
        .replace(/^-+|-+$/g, "") || fallback;
    const stamp = new Date()
      .toISOString()
      .replace(/[:.]/g, "-")
      .replace("T", "_")
      .replace("Z", "");
    return `govard-${sanitize(project, "project")}-${sanitize(service, "all")}-${stamp}.log`;
  };

  const renderFilteredOutput = () => {
    if (!refs.logOutput) {
      return;
    }
    const { severity, query } = readSelection();
    const filtered = filterLogsText(rawLogOutput, severity, query);
    refs.logOutput.textContent =
      filtered || "No logs match the current filters.";
  };

  const appendLogLine = (line) => {
    rawLogOutput = rawLogOutput
      ? `${rawLogOutput}\n${line}`
      : String(line || "");
    renderFilteredOutput();
    if (refs.logOutput) {
      refs.logOutput.scrollTop = refs.logOutput.scrollHeight;
    }
  };

  const refresh = async () => {
    const { project, service } = readSelection();
    if (!project) {
      if (refs.logOutput) {
        refs.logOutput.textContent = "Select an environment to view logs.";
      }
      rawLogOutput = "";
      return;
    }
    if (refs.logOutput) {
      refs.logOutput.textContent = "Loading logs...";
    }
    try {
      const logs = await bridge.getLogsForService(project, service);
      rawLogOutput = String(logs || "");
      renderFilteredOutput();
    } catch (err) {
      rawLogOutput = "";
      if (refs.logOutput) {
        refs.logOutput.textContent = `Failed to load logs: ${err}`;
      }
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
    renderFilteredOutput();
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
    const blob = new Blob([output + "\n"], {
      type: "text/plain;charset=utf-8",
    });
    const href = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = href;
    anchor.download = buildLogFilename(project, service);
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    URL.revokeObjectURL(href);
    onStatus("Logs downloaded successfully.");
    onToast("Logs downloaded successfully.", "success");
  };

  if (runtime?.EventsOn) {
    runtime.EventsOn("logs:line", appendLogLine);
    runtime.EventsOn("logs:status", (message) => {
      onStatus(message);
      onToast(message, "success");
    });
    runtime.EventsOn("logs:error", (message) => {
      onStatus(message);
      onToast(message, "error");
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
            class="flex-1 flex flex-col rounded-xl border border-[#2e573a] bg-[#0c1810] overflow-hidden shadow-lg relative"
          >
            <div
              class="p-3 border-b border-[#2e573a] bg-[#1a3322]"
            >
              <div class="flex flex-wrap items-center justify-between gap-3">
                <div class="flex items-center gap-2 min-w-0">
                  <span class="material-symbols-outlined text-primary text-lg"
                    >receipt_long</span
                  >
                  <h3 class="text-sm font-semibold text-white">Logs</h3>
                  <span
                    class="text-[10px] uppercase tracking-wide text-[#90cba4] bg-[#102316] border border-[#2e573a] px-2 py-0.5 rounded-full"
                    >Live Stream</span
                  >
                </div>
                <div class="flex items-center gap-2 ml-auto">
                  <div class="relative">
                    <span class="absolute inset-y-0 left-2 flex items-center">
                      <span
                        class="material-symbols-outlined text-[#5d856b] text-base"
                        >search</span
                      >
                    </span>
                    <input
                      class="bg-[#102316] text-xs text-white pl-8 pr-3 py-2 rounded-md border border-[#2e573a] focus:border-primary/50 focus:outline-none placeholder-[#5d856b] w-44 md:w-56"
                      placeholder="Filter logs..."
                      type="text"
                      id="logSearch"
                    />
                  </div>
                  <button
                    data-action="clear-logs"
                    class="p-2 rounded-md text-[#90cba4] hover:text-primary hover:bg-[#2e573a]/50 transition-colors"
                    title="Clear Logs"
                  >
                    <span class="material-symbols-outlined text-lg"
                      >block</span
                    >
                  </button>
                  <button
                    data-action="download-logs"
                    class="p-2 rounded-md text-[#90cba4] hover:text-primary hover:bg-[#2e573a]/50 transition-colors"
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
                    class="flex items-center gap-2 bg-[#102316] rounded-lg p-1.5 border border-[#2e573a] min-w-0 flex-1"
                  >
                    <span class="text-[10px] uppercase tracking-wide text-[#5d856b] px-1 shrink-0"
                      >Service</span
                    >
                    <div
                      id="logServiceSelector"
                      class="flex gap-1 min-w-0 flex-1 overflow-x-auto"
                    >
                      <!-- Service buttons will be rendered here -->
                      <button
                        data-action="filter-service"
                        data-service="all"
                        class="h-7 px-3 rounded-md text-xs font-semibold whitespace-nowrap border bg-[#2e573a] text-white border-[#3f7a52]"
                      >
                        all
                      </button>
                    </div>
                  </div>
                  <div
                    class="flex items-center gap-2 bg-[#102316] rounded-lg p-1.5 border border-[#2e573a] shrink-0"
                  >
                    <span class="text-[10px] uppercase tracking-wide text-[#5d856b] px-1 shrink-0"
                      >Severity</span
                    >
                    <div
                      id="logSeverity"
                      class="flex gap-1"
                    >
                      <button
                        data-action="filter-severity"
                        data-severity="all"
                        class="h-7 px-3 text-[10px] font-bold uppercase tracking-wide rounded-md border transition-colors bg-[#2e573a] text-primary border-[#3f7a52]"
                      >
                        All
                      </button>
                      <button
                        data-action="filter-severity"
                        data-severity="error"
                        class="h-7 px-3 text-[10px] font-bold uppercase tracking-wide rounded-md border transition-colors bg-[#102316] text-[#90cba4] border-transparent hover:bg-[#1a3322] hover:text-white"
                      >
                        Error
                      </button>
                      <button
                        data-action="filter-severity"
                        data-severity="warn"
                        class="h-7 px-3 text-[10px] font-bold uppercase tracking-wide rounded-md border transition-colors bg-[#102316] text-[#90cba4] border-transparent hover:bg-[#1a3322] hover:text-white"
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
                    class="h-8 px-3 rounded-md text-xs font-semibold bg-[#2e573a] text-white hover:bg-[#366b47] transition-colors"
                  >
                    Live: Off
                  </button>
                  <button
                    data-action="refresh-logs"
                    class="h-8 px-3 rounded-md text-xs font-semibold text-[#90cba4] hover:text-white hover:bg-[#2e573a]/50 transition-colors"
                  >
                    Refresh
                  </button>
                </div>
              </div>
            </div>
            <div
              class="flex-1 overflow-y-auto px-4 pb-4 pt-2 terminal-text text-xs bg-[#0c1810] custom-scrollbar"
            >
              <pre id="logOutput" class="m-0 font-mono whitespace-pre-wrap">Select an environment to view logs.</pre>
            </div>
          </div>
          <div
            class="h-1.5 bg-[#1a3322] hover:bg-primary/50 cursor-row-resize flex items-center justify-center rounded transition-colors group/resizer"
          >
            <div
              class="w-10 h-1 bg-[#2e573a] rounded-full group-hover/resizer:bg-white/50"
            ></div>
          </div>
          <div
            id="terminalPanel"
            class="h-1/3 min-h-0 flex flex-col rounded-xl border border-[#2e573a] bg-[#0c1810] overflow-hidden shadow-lg relative z-10 transform-gpu"
          >
            <div
              class="flex items-center justify-between p-2 pl-3 border-b border-[#2e573a] bg-[#1a3322]"
            >
              <div class="flex items-center gap-2">
                <span
                  class="material-symbols-outlined text-slate-400 text-sm"
                  >terminal</span
                >
                <span class="text-xs font-semibold text-slate-300"
                  >Terminal — bash</span
                >
              </div>
              <div class="flex items-center gap-2">
                <button
                  data-action="start-embedded-terminal"
                  class="px-2.5 py-1 rounded text-xs font-medium bg-[#22492f] text-slate-100 hover:bg-[#2e573a] transition-colors border border-[#366b47]"
                  title="Open Shell"
                >
                  Open Shell
                </button>
                <button
                  data-action="toggle-terminal-modal"
                  class="p-1.5 rounded hover:bg-white/10 text-slate-400 hover:text-white transition-colors"
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
                  <option value="bash">bash</option>
                </select>
              </div>
            </div>
            <div
              class="flex-1 min-h-0 overflow-hidden bg-[#0c1810] p-4 pb-6"
            >
              <div
                id="terminalContainer"
                class="h-full min-h-0 w-full overflow-hidden bg-[#0c1810]"
              ></div>
            </div>
          </div>
        </div>
      </div>
  `;
};
