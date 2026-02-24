import { clearChildren, escapeHTML, setText } from "../utils/dom.js";

const asNumber = (value) => {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
};

export const formatMetricPercent = (value = 0) =>
  `${asNumber(value).toFixed(1)}%`;
export const formatMetricMB = (value = 0) => `${asNumber(value).toFixed(1)} MB`;

const normalizeProjectMetric = (project = {}) => ({
  project: String(project.project || project.Project || "").trim(),
  status: String(project.status || project.Status || "stopped")
    .trim()
    .toLowerCase(),
  cpuPercent: asNumber(project.cpuPercent ?? project.CPUPercent),
  memoryMB: asNumber(project.memoryMB ?? project.MemoryMB),
  memoryPercent: asNumber(project.memoryPercent ?? project.MemoryPercent),
  netRxMB: asNumber(project.netRxMB ?? project.NetRxMB),
  netTxMB: asNumber(project.netTxMB ?? project.NetTxMB),
  oomKilled: Boolean(project.oomKilled ?? project.OOMKilled),
});

export const normalizeMetricsPayload = (payload = {}) => {
  const summary = payload.summary || payload.Summary || {};
  const projectsRaw = Array.isArray(payload.projects)
    ? payload.projects
    : Array.isArray(payload.Projects)
      ? payload.Projects
      : [];
  const warnings = Array.isArray(payload.warnings)
    ? payload.warnings
    : Array.isArray(payload.Warnings)
      ? payload.Warnings
      : [];

  return {
    updatedAt: String(payload.updatedAt || payload.UpdatedAt || "").trim(),
    systemCPU: asNumber(payload.systemCPU ?? payload.SystemCPU),
    systemMemory: asNumber(payload.systemMemory ?? payload.SystemMemory),
    summary: {
      activeProjects: asNumber(
        summary.activeProjects ?? summary.ActiveProjects,
      ),
      cpuPercent: asNumber(summary.cpuPercent ?? summary.CPUPercent),
      memoryMB: asNumber(summary.memoryMB ?? summary.MemoryMB),
      netRxMB: asNumber(summary.netRxMB ?? summary.NetRxMB),
      netTxMB: asNumber(summary.netTxMB ?? summary.NetTxMB),
      oomProjects: asNumber(summary.oomProjects ?? summary.OOMProjects),
    },
    projects: projectsRaw.map(normalizeProjectMetric),
    warnings: warnings.map((item) => String(item)),
  };
};

const renderMetricSummary = (refs, summary = {}) => {
  setText(refs.metricActiveProjects, String(summary.activeProjects ?? 0));
  setText(refs.metricCPU, formatMetricPercent(summary.cpuPercent));
  setText(refs.metricMemory, formatMetricMB(summary.memoryMB));
  setText(refs.metricNetRx, formatMetricMB(summary.netRxMB));
  setText(refs.metricNetTx, formatMetricMB(summary.netTxMB));
  setText(refs.metricOOM, String(summary.oomProjects ?? 0));
};

const renderMetricWarnings = (container, warnings = []) => {
  if (!container) {
    return;
  }
  clearChildren(container);
  warnings.forEach((warning) => {
    const item = document.createElement("li");
    item.textContent = warning;
    container.appendChild(item);
  });
};

export const renderMetricSkeletons = (container) => {
  if (!container) return;
  container.innerHTML = Array(3)
    .fill(0)
    .map(
      () => `
    <div class="glass-panel p-4 rounded-xl border border-[#2e573a] flex items-center justify-between">
      <div class="flex items-center gap-4">
        <div class="p-2 rounded bg-[#1a3322] border border-[#2e573a] h-10 w-10 skeleton"></div>
        <div class="space-y-2">
          <div class="h-4 w-32 skeleton"></div>
          <div class="h-3 w-48 skeleton"></div>
        </div>
      </div>
    </div>
  `,
    )
    .join("");
};

const renderMetricProjects = (container, projects = [], selectedProject = "") => {
  if (!container) {
    return;
  }

  const filtered = projects.filter(
    (p) => !selectedProject || p.project.startsWith(selectedProject + "-"),
  );

  if (!filtered.length) {
    container.innerHTML = `<div class="p-6 text-center text-slate-500 border border-dashed border-[#22492f] rounded-xl">No active services detected for this environment.</div>`;
    return;
  }

  container.innerHTML = filtered
    .map((project) => {
      const name = project.project || "unknown";
      const healthy = project.status === "running";

      let icon = "php";
      let iconColor = "text-blue-400";
      let iconBg = "bg-blue-500/10 border-blue-500/20";

      if (name.includes("mysql") || name.includes("db")) {
        icon = "database";
        iconColor = "text-yellow-400";
        iconBg = "bg-yellow-500/10 border-yellow-500/20";
      } else if (name.includes("redis") || name.includes("cache")) {
        icon = "bolt";
        iconColor = "text-red-400";
        iconBg = "bg-red-500/10 border-red-500/20";
      } else if (name.includes("nginx") || name.includes("web")) {
        icon = "language";
        iconColor = "text-emerald-400";
        iconBg = "bg-emerald-500/10 border-emerald-500/20";
      }

      return `
      <div class="glass-panel p-4 rounded-xl border border-[#2e573a] hover:border-primary/30 transition-all flex items-center justify-between group">
        <div class="flex items-center gap-4">
          <div class="p-2 rounded ${iconBg} ${iconColor} border">
            <span class="material-symbols-outlined">${icon}</span>
          </div>
          <div>
            <h4 class="text-white font-medium text-sm">${escapeHTML(name)}</h4>
            <div class="flex items-center gap-2 text-xs mt-1">
              <span class="text-slate-400">CPU: ${formatMetricPercent(project.cpuPercent)}</span>
              <span class="w-1 h-1 rounded-full bg-slate-600"></span>
              <span class="text-slate-400">Mem: ${formatMetricMB(project.memoryMB)}</span>
              <span class="w-1 h-1 rounded-full bg-slate-600"></span>
              <span class="${healthy ? "text-green-400" : "text-amber-500"}">${healthy ? "Healthy" : "Idle"}</span>
            </div>
          </div>
        </div>
        <div class="flex items-center gap-3 opacity-0 group-hover:opacity-100 transition-opacity">
          <button data-action="switch-tab" data-tab="logs" data-service="${escapeHTML(name)}" class="p-1.5 rounded hover:bg-[#22492f] text-slate-400 hover:text-white transition-colors" title="View Logs">
            <span class="material-symbols-outlined text-lg">list_alt</span>
          </button>
          <button data-action="switch-tab" data-tab="logs" data-service="${escapeHTML(name)}" class="p-1.5 rounded hover:bg-[#22492f] text-slate-400 hover:text-white transition-colors" title="Terminal">
            <span class="material-symbols-outlined text-lg">terminal</span>
          </button>
        </div>
      </div>
    `;
    })
    .join("");
};

export const createMetricsController = ({
  bridge,
  refs,
  onStatus,
  getProject,
}) => {
  let refreshTimer = null;

  const renderPayload = (payload) => {
    const metrics = normalizeMetricsPayload(payload);
    const selectedProject = getProject();

    let summary = metrics.summary;
    if (selectedProject) {
      const projectMetric = metrics.projects.find(
        (p) => p.project === selectedProject,
      );
      if (projectMetric) {
        summary = {
          ...summary,
          cpuPercent: projectMetric.cpuPercent,
          memoryMB: projectMetric.memoryMB,
        };
      }
    }

    renderMetricSummary(refs, summary);
    renderMetricWarnings(refs.metricsWarnings, metrics.warnings);
    renderMetricProjects(refs.metricsList, metrics.projects, selectedProject);

    // Footer always shows system metrics
    if (refs.footerCPU)
      setText(refs.footerCPU, formatMetricPercent(metrics.systemCPU));
    if (refs.footerMemory)
      setText(refs.footerMemory, formatMetricMB(metrics.systemMemory));

    return metrics;
  };

  const refresh = async ({ silent = false } = {}) => {
    try {
      const payload = await bridge.getResourceMetrics();
      const metrics = renderPayload(payload);
      if (!silent) {
        onStatus(
          `Status: metrics refreshed at ${new Date().toLocaleTimeString()}`,
        );
      }
      return metrics;
    } catch (err) {
      renderPayload({
        summary: {},
        projects: [],
        warnings: [`Metrics unavailable: ${err}`],
      });
      if (!silent) {
        onStatus("Status: metrics unavailable");
      }
      return null;
    }
  };

  const startAutoRefresh = () => {
    if (refreshTimer) {
      clearInterval(refreshTimer);
    }
    refreshTimer = setInterval(() => {
      refresh({ silent: true });
    }, 15000);
  };

  const stopAutoRefresh = () => {
    if (refreshTimer) {
      clearInterval(refreshTimer);
      refreshTimer = null;
    }
  };

  return {
    refresh,
    startAutoRefresh,
    stopAutoRefresh,
  };
};
