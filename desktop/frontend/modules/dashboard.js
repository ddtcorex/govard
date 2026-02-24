import { clearChildren, escapeHTML, setText } from "../utils/dom.js";

export const normalizeDashboardPayload = (data = {}) => ({
  active: data.ActiveEnvironments ?? data.active ?? 0,
  services: data.RunningServices ?? data.services ?? 0,
  queued: data.QueuedTasks ?? data.queued ?? 0,
  activeSummary: data.ActiveSummary ?? data.activeSummary ?? "",
  servicesSummary: data.ServicesSummary ?? data.servicesSummary ?? "",
  queueSummary: data.QueueSummary ?? data.queueSummary ?? "",
  environments: Array.isArray(data.Environments)
    ? data.Environments
    : Array.isArray(data.environments)
      ? data.environments
      : [],
  warnings: Array.isArray(data.Warnings)
    ? data.Warnings
    : Array.isArray(data.warnings)
      ? data.warnings
      : [],
});

export const projectKey = (env = {}) =>
  env.Project || env.project || env.Name || env.name || "";

export const domainLabel = (env = {}) =>
  env.Domain || env.domain || env.Name || env.name || projectKey(env);

export const serviceTargets = (env = {}) => {
  const values = Array.isArray(env.ServiceTargets)
    ? env.ServiceTargets
    : Array.isArray(env.serviceTargets)
      ? env.serviceTargets
      : [];
  return values.length ? values : ["web"];
};

export const renderMetricSkeletons = (refs) => {
  const skeleton = `<div class="h-6 w-12 skeleton mb-1"></div>`;
  const hintSkeleton = `<div class="h-3 w-24 skeleton"></div>`;
  if (refs.statActive) refs.statActive.innerHTML = skeleton;
  if (refs.statServices) refs.statServices.innerHTML = skeleton;
  if (refs.statQueue) refs.statQueue.innerHTML = skeleton;
  if (refs.statActiveHint) refs.statActiveHint.innerHTML = hintSkeleton;
  if (refs.statServicesHint) refs.statServicesHint.innerHTML = hintSkeleton;
  if (refs.statQueueHint) refs.statQueueHint.innerHTML = hintSkeleton;
};

export const setMetricText = (
  { active, services, queued, activeSummary, servicesSummary, queueSummary },
  refs,
) => {
  setText(refs.statActive, String(active));
  setText(refs.statServices, String(services));
  setText(refs.statQueue, String(queued));
  setText(refs.statActiveHint, activeSummary || "No environments detected");
  setText(refs.statServicesHint, servicesSummary || "Waiting for service data");
  setText(refs.statQueueHint, queueSummary || "Queue idle");
};

export const renderWarnings = (warningList, warnings = []) => {
  if (!warningList) {
    return;
  }
  clearChildren(warningList);
  warnings.forEach((warning) => {
    const item = document.createElement("li");
    item.textContent = String(warning);
    warningList.appendChild(item);
  });
};

export const renderEnvironmentSkeletons = (container) => {
  if (!container) return;
  const header = `<div class="px-3 mb-2 text-xs font-semibold text-[#90cba4] uppercase tracking-wider">Environments</div>`;
  const items = Array(3)
    .fill(0)
    .map(
      () => `
    <div class="w-full mb-1 flex items-center gap-3 px-3 py-2.5 rounded-lg border border-transparent">
      <div class="h-6 w-6 rounded-full skeleton"></div>
      <div class="flex-1 space-y-2">
        <div class="h-3 w-24 skeleton"></div>
        <div class="h-2 w-12 skeleton"></div>
      </div>
    </div>
  `,
    )
    .join("");
  container.innerHTML = header + items;
};

export const renderEnvironmentList = (container, environments = []) => {
  if (!container) {
    return;
  }
  if (!environments.length) {
    container.innerHTML = `<div class="px-3 mb-2 text-xs font-semibold text-[#90cba4] uppercase tracking-wider">Environments</div><div class="px-3 text-slate-400 text-sm">No environments detected.</div>`;
    return;
  }

  const header = `<div class="px-3 mb-2 text-xs font-semibold text-[#90cba4] uppercase tracking-wider">Environments</div>`;

  container.innerHTML =
    header +
    environments
      .map((env) => {
        const key = projectKey(env);
        const domain = domainLabel(env);
        const status = String(
          env.Status || env.status || "stopped",
        ).toLowerCase();
        const running = status === "running";
        const warning = status === "warning";
        const statusText = warning
          ? "Warning"
          : running
            ? "Running"
            : "Stopped";

        if (running) {
          return `
          <button data-action="env-open" data-env="${escapeHTML(key)}" class="w-full mb-1 flex items-center gap-3 px-3 py-2.5 rounded-lg bg-[#22492f] border border-[#366b47]/50 group transition-all relative overflow-hidden" title="Click to open, or double-click to toggle">
            <div class="absolute inset-y-0 left-0 w-1 bg-primary"></div>
            <span data-action="toggle-env" data-env="${escapeHTML(key)}" class="material-symbols-outlined text-primary fill-1 hover:text-white transition-colors z-10" style="font-variation-settings: 'FILL' 1;">play_circle</span>
            <div class="flex flex-col items-start min-w-0" data-action="env-open" data-env="${escapeHTML(key)}">
              <span class="text-white text-sm font-medium truncate w-full text-left" data-action="env-open" data-env="${escapeHTML(key)}">${escapeHTML(domain)}</span>
              <span class="text-xs text-primary" data-action="env-open" data-env="${escapeHTML(key)}">${statusText}</span>
            </div>
          </button>
         `;
        }

        if (warning) {
          return `
          <button data-action="env-open" data-env="${escapeHTML(key)}" class="w-full mb-1 flex items-center gap-3 px-3 py-2.5 rounded-lg hover:bg-[#22492f]/50 transition-colors group" title="Click to open, or double-click to toggle">
            <span data-action="toggle-env" data-env="${escapeHTML(key)}" class="material-symbols-outlined text-amber-500 hover:text-primary transition-colors z-10">warning</span>
            <div class="flex flex-col items-start min-w-0" data-action="env-open" data-env="${escapeHTML(key)}">
              <span class="text-slate-300 text-sm font-medium group-hover:text-white truncate w-full text-left" data-action="env-open" data-env="${escapeHTML(key)}">${escapeHTML(domain)}</span>
              <span class="text-xs text-amber-500" data-action="env-open" data-env="${escapeHTML(key)}">${statusText}</span>
            </div>
          </button>
          `;
        }

        return `
        <button data-action="env-open" data-env="${escapeHTML(key)}" class="w-full mb-1 flex items-center gap-3 px-3 py-2.5 rounded-lg hover:bg-[#22492f]/50 transition-colors group" title="Click to open, or double-click to toggle">
          <span data-action="toggle-env" data-env="${escapeHTML(key)}" class="material-symbols-outlined text-slate-400 hover:text-primary transition-colors z-10">stop_circle</span>
          <div class="flex flex-col items-start min-w-0" data-action="env-open" data-env="${escapeHTML(key)}">
            <span class="text-slate-300 text-sm font-medium group-hover:text-white truncate w-full text-left" data-action="env-open" data-env="${escapeHTML(key)}">${escapeHTML(domain)}</span>
            <span class="text-xs text-slate-500" data-action="env-open" data-env="${escapeHTML(key)}">${statusText}</span>
          </div>
        </button>
      `;
      })
      .join("");
};

const syncSingleSelector = (selector, environments, selectedProject) => {
  if (!selector) {
    return;
  }
  const previous = selectedProject || selector.value;
  selector.innerHTML = "";
  environments.forEach((env) => {
    const option = document.createElement("option");
    option.value = projectKey(env);
    option.textContent = domainLabel(env);
    selector.appendChild(option);
  });
  const exists = environments.some((env) => projectKey(env) === previous);
  selector.value = exists
    ? previous
    : environments.length
      ? projectKey(environments[0])
      : "";
};

export const syncProjectSelectors = (
  selectors,
  environments = [],
  selectedProject = "",
) => {
  syncSingleSelector(selectors.envSelector, environments, selectedProject);
  syncSingleSelector(selectors.logSelector, environments, selectedProject);
};
export const renderProjectHero = (
  refs,
  environments = [],
  selectedProject = "",
) => {
  const env = environments.find((e) => projectKey(e) === selectedProject);
  if (!env) return;

  const title = domainLabel(env);
  const status = String(env.Status || env.status || "stopped").toLowerCase();
  const url = env.Url || env.url || `http://${title}.test`;

  if (refs.projectTitle) refs.projectTitle.textContent = title;
  if (refs.projectStatusText) {
    refs.projectStatusText.textContent =
      status.charAt(0).toUpperCase() + status.slice(1);
  }

  if (refs.projectStatusBadge) {
    const badge = refs.projectStatusBadge;
    badge.className =
      "px-3 py-1 rounded-full border text-xs font-bold uppercase tracking-wide neon-glow flex items-center gap-1.5";
    if (status === "running") {
      badge.classList.add("bg-primary/20", "border-primary/30", "text-primary");
    } else if (status === "warning") {
      badge.classList.add(
        "bg-amber-500/20",
        "border-amber-500/30",
        "text-amber-500",
      );
    } else {
      badge.classList.add(
        "bg-slate-500/20",
        "border-slate-500/30",
        "text-slate-400",
      );
    }
  }

  if (refs.projectUrl) {
    refs.projectUrl.href = url;
  }
  if (refs.projectUrlText) {
    refs.projectUrlText.textContent = url;
  }

  if (refs.projectTechnologies) {
    const techs = Array.isArray(env.Technologies)
      ? env.Technologies
      : Array.isArray(env.technologies)
        ? env.technologies
        : [];

    if (techs.length) {
      refs.projectTechnologies.innerHTML = techs
        .map((tech) => {
          let color = "bg-blue-500";
          let shadow = "rgba(59, 130, 246, 0.5)";
          if (
            tech.toLowerCase().includes("mysql") ||
            tech.toLowerCase().includes("maria")
          ) {
            color = "bg-yellow-500";
            shadow = "rgba(234, 179, 8, 0.5)";
          }
          if (
            tech.toLowerCase().includes("redis") ||
            tech.toLowerCase().includes("cache")
          ) {
            color = "bg-red-500";
            shadow = "rgba(239, 68, 68, 0.5)";
          }
          if (tech.toLowerCase().includes("python")) {
            color = "bg-green-600";
            shadow = "rgba(22, 163, 74, 0.5)";
          }
          if (tech.toLowerCase().includes("node")) {
            color = "bg-green-500";
            shadow = "rgba(34, 197, 94, 0.5)";
          }

          return `<span class="flex items-center gap-1 bg-[#1a3322] px-2 py-0.5 rounded border border-[#2e573a]">
          <span class="w-1.5 h-1.5 rounded-full ${color}" style="box-shadow: 0 0 8px ${shadow}"></span>
          ${escapeHTML(tech)}
        </span>`;
        })
        .join("");
    } else {
      refs.projectTechnologies.innerHTML = "";
    }
  }
  if (refs.heroRestartBtn) {
    refs.heroRestartBtn.dataset.env = selectedProject;
  }
  if (refs.heroStopBtn) {
    refs.heroStopBtn.dataset.env = selectedProject;
  }

  if (refs.envVarsList) {
    renderEnvVars(refs.envVarsList, env);
  }
};

export const renderEnvVars = (container, env) => {
  if (!container) return;

  const envVars = env?.EnvVars || env?.envVars || {};
  const keys = Object.keys(envVars);

  if (keys.length === 0) {
    container.innerHTML = `<div class="text-xs text-slate-500 italic">No environment variables defined</div>`;
    return;
  }

  container.innerHTML = keys
    .map((key) => {
      const value = envVars[key];
      return `
      <div class="flex justify-between items-center group cursor-pointer hover:bg-[#22492f]/50 p-1.5 -mx-1.5 rounded transition-colors" title="Click to copy">
        <span class="text-xs text-[#90cba4] font-mono">${escapeHTML(key)}</span>
        <span class="text-xs text-white font-mono bg-[#102316] px-2 py-0.5 rounded border border-[#2e573a] break-all max-w-[60%]">${escapeHTML(value)}</span>
      </div>`;
    })
    .join("");
};
