import {
  buildLogFilename,
  downloadTextAsFile,
  filterLogsText,
  normalizeLogSeverity,
  syncSeveritySelector,
} from "./logs.js";
import { escapeHTML, setText } from "../utils/dom.js";

const globalServiceIcons = {
  caddy: "shield",
  mail: "mail",
  pma: "database",
  portainer: "deployed_code",
  dnsmasq: "dns",
};

const ACTIVE_STATUSES = new Set([
  "running",
  "restarting",
  "starting",
  "healthy",
  "up",
]);
const PORT_CONFLICT_WARNING_PREFIX = "port conflict ";

const BULK_START_ENABLED_CLASS =
  "h-10 min-w-[118px] px-3 bg-primary text-background-dark rounded-xl text-xs font-bold uppercase tracking-[0.08em] hover:bg-primary/90 transition-all active:scale-95 inline-flex items-center justify-center gap-1.5 shadow-[0_8px_22px_rgba(13,242,89,0.18)] ring-1 ring-primary/30 whitespace-nowrap disabled:opacity-70 disabled:cursor-not-allowed disabled:active:scale-100";
const BULK_START_DISABLED_CLASS =
  "h-10 min-w-[118px] px-3 bg-background-secondary text-slate-500 dark:text-text-tertiary/70 border border-border-primary rounded-xl text-xs font-bold uppercase tracking-[0.08em] transition-all inline-flex items-center justify-center gap-1.5 whitespace-nowrap disabled:opacity-70 disabled:cursor-not-allowed";
const BULK_RESTART_ENABLED_CLASS =
  "h-10 min-w-[118px] px-3 bg-primary text-background-dark rounded-xl text-xs font-bold uppercase tracking-[0.08em] hover:bg-primary/90 transition-all active:scale-95 inline-flex items-center justify-center gap-1.5 shadow-[0_8px_22px_rgba(13,242,89,0.18)] ring-1 ring-primary/30 whitespace-nowrap disabled:opacity-70 disabled:cursor-not-allowed disabled:active:scale-100";
const BULK_RESTART_DISABLED_CLASS =
  "h-10 min-w-[118px] px-3 bg-background-secondary text-slate-500 dark:text-text-tertiary/70 border border-border-primary rounded-xl text-xs font-bold uppercase tracking-[0.08em] transition-all inline-flex items-center justify-center gap-1.5 whitespace-nowrap disabled:opacity-70 disabled:cursor-not-allowed";
const BULK_STOP_ENABLED_CLASS =
  "h-10 min-w-[118px] px-3 bg-red-600 text-white border border-red-500 rounded-xl text-xs font-bold uppercase tracking-[0.08em] hover:bg-red-500 transition-all active:scale-95 inline-flex items-center justify-center gap-1.5 shadow-[0_8px_24px_rgba(239,68,68,0.25)] ring-1 ring-red-400/30 whitespace-nowrap disabled:opacity-70 disabled:cursor-not-allowed disabled:active:scale-100";
const BULK_STOP_DISABLED_CLASS =
  "h-10 min-w-[118px] px-3 bg-red-500/10 text-red-700 dark:text-red-500/60 border border-red-500/20 rounded-xl text-xs font-bold uppercase tracking-[0.08em] transition-all inline-flex items-center justify-center gap-1.5 whitespace-nowrap disabled:opacity-70 disabled:cursor-not-allowed";
const BULK_PULL_CLASS =
  "h-10 min-w-[118px] px-3 bg-background-secondary text-text-primary border border-border-primary rounded-xl text-xs font-bold uppercase tracking-[0.08em] hover:bg-background-primary transition-all active:scale-95 inline-flex items-center justify-center gap-1.5 shadow-sm whitespace-nowrap disabled:opacity-70 disabled:cursor-not-allowed disabled:active:scale-100";
const BULK_ERROR_MESSAGE_MAX_LENGTH = 180;

const collapseWhitespace = (value = "") =>
  String(value || "")
    .replace(/\s+/g, " ")
    .trim();

const pickBulkErrorDetail = (rawError = "") => {
  const normalized = String(rawError || "").replace(/\r/g, "\n");
  const portMatch = normalized.match(
    /bind for [^:]+:(\d+)\s+failed:\s*port is already allocated/i,
  );
  if (portMatch?.[1]) {
    return `port ${portMatch[1]} is already in use`;
  }
  if (/port is already allocated/i.test(normalized)) {
    return "a required port is already in use";
  }

  const lines = normalized
    .split("\n")
    .map((line) => collapseWhitespace(line))
    .filter(Boolean);
  if (!lines.length) {
    return "";
  }

  const preferredLine = [...lines]
    .reverse()
    .find((line) =>
      /(error|failed|denied|cannot|unable|conflict|timeout|refused|already|not found)/i.test(
        line,
      ),
    );

  const rawDetail = preferredLine || lines[lines.length - 1];
  return collapseWhitespace(
    rawDetail
      .replace(/^error response from daemon:\s*/i, "")
      .replace(/^[a-z]+\s+global services:\s*/i, "")
      .replace(/^exit status \d+:\s*/i, ""),
  );
};

const formatBulkGlobalActionError = (action, err) => {
  const actionLabel = collapseWhitespace(action).toLowerCase() || "operation";
  const prefix = `Global ${actionLabel} failed`;
  const rawError = err instanceof Error ? err.message : String(err || "");
  const detail = pickBulkErrorDetail(rawError);

  let message = detail ? `${prefix}: ${detail}` : `${prefix}.`;
  if (message.length > BULK_ERROR_MESSAGE_MAX_LENGTH) {
    message = `${message.slice(0, BULK_ERROR_MESSAGE_MAX_LENGTH - 3).trimEnd()}...`;
  }
  return message;
};

export const formatBulkGlobalActionErrorForTest = (action, err) =>
  formatBulkGlobalActionError(action, err);

const isServiceActive = (service = {}) =>
  ACTIVE_STATUSES.has(
    String(service.status || "")
      .trim()
      .toLowerCase(),
  ) || Boolean(service.running);

const isStopLikeState = (service = {}) => {
  const status = String(service.status || "")
    .trim()
    .toLowerCase();
  const state = String(service.state || "")
    .trim()
    .toLowerCase();
  return (
    status === "stopped" ||
    status === "exited" ||
    status === "created" ||
    status === "dead" ||
    state.includes("stopped") ||
    state.includes("exited") ||
    state.includes("created") ||
    state.includes("dead")
  );
};

const hasRoutingImpact = (service = {}) =>
  (service.id === "caddy" || service.id === "dnsmasq") &&
  !isServiceActive(service) &&
  isStopLikeState(service);

const hasRoutingWarningInSnapshot = (snapshot = {}) => {
  const services = Array.isArray(snapshot.services) ? snapshot.services : [];
  const warnings = Array.isArray(snapshot.warnings) ? snapshot.warnings : [];
  return (
    services.some((service) => hasRoutingImpact(service)) ||
    warnings.some((warning) =>
      String(warning || "")
        .trim()
        .toLowerCase()
        .startsWith(PORT_CONFLICT_WARNING_PREFIX),
    )
  );
};

const delay = (ms) =>
  new Promise((resolve) => {
    setTimeout(resolve, ms);
  });

export const summarizeActionMessage = (message, fallback = "") => {
  const normalizedFallback = String(fallback || "").trim();
  const text = String(message || "").trim();
  if (!text) {
    return normalizedFallback;
  }
  const [firstLineRaw] = text.split(/\r?\n/u);
  const firstLine = String(firstLineRaw || "").trim();
  return firstLine || normalizedFallback;
};

const summarizeRoutingConflicts = (warnings = []) => {
  const conflicts = Array.isArray(warnings)
    ? warnings
        .map((warning) => String(warning || "").trim())
        .filter((warning) =>
          warning.toLowerCase().startsWith(PORT_CONFLICT_WARNING_PREFIX),
        )
        .map((warning) => warning.slice(PORT_CONFLICT_WARNING_PREFIX.length))
    : [];

  if (conflicts.length === 0) {
    return {
      hasConflicts: false,
      missingLine: "",
      occupiedLine: "",
      notesLine: "",
    };
  }

  const missingPortsByService = new Map();
  const ownerPortsByName = new Map();
  const otherEntries = [];

  for (const conflict of conflicts) {
    const missingMatch = conflict.match(
      /^(\d+\/[a-z]+):\s+(.+?) is running but .+ is not published on host$/i,
    );
    if (missingMatch) {
      const port = String(missingMatch[1] || "")
        .trim()
        .toLowerCase();
      const serviceName = String(missingMatch[2] || "").trim();
      if (!missingPortsByService.has(serviceName)) {
        missingPortsByService.set(serviceName, new Set());
      }
      missingPortsByService.get(serviceName).add(port);
      continue;
    }

    const dockerOwnerMatch = conflict.match(
      /^(\d+\/[a-z]+):\s+docker container\s+(.+)$/i,
    );
    if (dockerOwnerMatch) {
      const port = String(dockerOwnerMatch[1] || "")
        .trim()
        .toLowerCase();
      const ownerRaw = String(dockerOwnerMatch[2] || "").trim();
      const ownerName = ownerRaw.replace(/\s+\([^)]*\)\s*$/, "").trim();
      if (!ownerPortsByName.has(ownerName)) {
        ownerPortsByName.set(ownerName, new Set());
      }
      ownerPortsByName.get(ownerName).add(port);
      continue;
    }

    const hostOwnerMatch = conflict.match(
      /^(\d+\/[a-z]+):\s+host process\s+(.+)$/i,
    );
    if (hostOwnerMatch) {
      const port = String(hostOwnerMatch[1] || "")
        .trim()
        .toLowerCase();
      const ownerRaw = String(hostOwnerMatch[2] || "").trim();
      const ownerName = ownerRaw.replace(/\s+\([^)]*\)\s*$/, "").trim();
      if (!ownerPortsByName.has(ownerName)) {
        ownerPortsByName.set(ownerName, new Set());
      }
      ownerPortsByName.get(ownerName).add(port);
      continue;
    }

    otherEntries.push(conflict);
  }

  let missingLine = "";
  let occupiedLine = "";
  let notesLine = "";

  if (missingPortsByService.size > 0) {
    const missingSummaryList = Array.from(missingPortsByService.entries()).map(
      ([serviceName, ports]) => {
        const portList = Array.from(ports).sort().join(", ");
        return `${serviceName} (${portList})`;
      },
    );
    const cappedMissing = missingSummaryList.slice(0, 2);
    const remainingMissing = missingSummaryList.length - cappedMissing.length;
    missingLine = `Missing bindings: ${cappedMissing.join("; ")}${remainingMissing > 0 ? `; +${remainingMissing} more` : ""}.`;
  }

  if (ownerPortsByName.size > 0) {
    const ownerSummaryList = Array.from(ownerPortsByName.entries()).map(
      ([ownerName, ports]) =>
        `${ownerName} (${Array.from(ports).sort().join(", ")})`,
    );
    const cappedOwners = ownerSummaryList.slice(0, 2);
    const remaining = ownerSummaryList.length - cappedOwners.length;
    occupiedLine = `Occupied by: ${cappedOwners.join("; ")}${remaining > 0 ? `; +${remaining} more` : ""}.`;
  }

  if (otherEntries.length > 0) {
    const cappedOthers = otherEntries.slice(0, 2);
    const remaining = otherEntries.length - cappedOthers.length;
    notesLine = `Notes: ${cappedOthers.join(" | ")}${remaining > 0 ? ` | +${remaining} more` : ""}.`;
  }

  return {
    hasConflicts: true,
    missingLine:
      missingLine || "Missing bindings: could not verify published ports.",
    occupiedLine: occupiedLine || "Occupied by: not detected.",
    notesLine,
  };
};

export const buildRoutingWarningMessage = (services = [], warnings = []) => {
  const conflictSummary = summarizeRoutingConflicts(warnings);
  const appendGuidance = (baseMessage) =>
    conflictSummary.hasConflicts
      ? `${baseMessage}\n${conflictSummary.missingLine}\n${conflictSummary.occupiedLine}${conflictSummary.notesLine ? ` ${conflictSummary.notesLine}` : ""} Resolve conflicts, then click Restart All or Start All.`
      : `${baseMessage} Check Docker/host processes using ports 80/443/53, then click Start All.`;

  if (!Array.isArray(services) || services.length === 0) {
    return appendGuidance(
      "Routing guard triggered: Caddy Proxy or DNSMasq is stopped.",
    );
  }

  const caddy = services.find((service) => service.id === "caddy");
  const dnsmasq = services.find((service) => service.id === "dnsmasq");
  const caddyStopped = Boolean(caddy) && hasRoutingImpact(caddy);
  const dnsmasqStopped = Boolean(dnsmasq) && hasRoutingImpact(dnsmasq);
  const hasConflicts = conflictSummary.hasConflicts;

  if (caddyStopped && dnsmasqStopped) {
    return appendGuidance("Caddy Proxy and DNSMasq are stopped.");
  }
  if (caddyStopped) {
    return appendGuidance("Caddy Proxy is stopped.");
  }
  if (dnsmasqStopped) {
    return appendGuidance("DNSMasq is stopped.");
  }

  if (hasConflicts) {
    return appendGuidance(
      "Routing services are running but port bindings are degraded.",
    );
  }

  return appendGuidance(
    "Routing guard triggered: Caddy Proxy or DNSMasq is stopped.",
  );
};

const normalizeGlobalService = (service = {}) => ({
  id: String(service.id || service.ID || "")
    .trim()
    .toLowerCase(),
  name: String(service.name || service.Name || "").trim() || "Unknown",
  composeService: String(
    service.composeService || service.ComposeService || "",
  ).trim(),
  containerName: String(
    service.containerName || service.ContainerName || "",
  ).trim(),
  status: String(service.status || service.Status || "missing")
    .trim()
    .toLowerCase(),
  state: String(service.state || service.State || "unknown").trim(),
  health: String(service.health || service.Health || "unknown")
    .trim()
    .toLowerCase(),
  statusText: String(service.statusText || service.StatusText || "").trim(),
  running: Boolean(service.running ?? service.Running),
  openable: Boolean(service.openable ?? service.Openable),
  url: String(service.url || service.URL || "").trim(),
});

const placeDnsmasqAfterCaddy = (services = []) => {
  const caddyIndex = services.findIndex((item) => item.id === "caddy");
  const dnsmasqIndex = services.findIndex((item) => item.id === "dnsmasq");
  if (caddyIndex < 0 || dnsmasqIndex < 0 || dnsmasqIndex === caddyIndex + 1) {
    return services;
  }

  const reordered = [...services];
  const [dnsmasq] = reordered.splice(dnsmasqIndex, 1);
  const nextCaddyIndex = reordered.findIndex((item) => item.id === "caddy");
  reordered.splice(nextCaddyIndex + 1, 0, dnsmasq);
  return reordered;
};

export const normalizeGlobalServicesSnapshot = (payload = {}) => {
  const servicesRaw = Array.isArray(payload.services)
    ? payload.services
    : Array.isArray(payload.Services)
      ? payload.Services
      : [];
  return {
    active: Number(payload.active ?? payload.Active ?? 0) || 0,
    total:
      Number(payload.total ?? payload.Total ?? servicesRaw.length) ||
      servicesRaw.length,
    summary:
      String(payload.summary || payload.Summary || "").trim() ||
      "Global services status unavailable",
    warnings: Array.isArray(payload.warnings)
      ? payload.warnings
      : Array.isArray(payload.Warnings)
        ? payload.Warnings
        : [],
    services: placeDnsmasqAfterCaddy(
      servicesRaw.map(normalizeGlobalService).filter((item) => item.id),
    ),
  };
};

const statusChipClass = (status = "missing") => {
  if (status === "running") {
    return "bg-primary/20 border-primary/30 text-primary";
  }
  if (status === "restarting") {
    return "bg-amber-500/20 border-amber-500/30 text-amber-400";
  }
  if (status === "paused") {
    return "bg-amber-500/20 border-amber-500/30 text-amber-500";
  }
  if (status === "missing") {
    return "bg-slate-500/20 border-slate-500/30 text-slate-600 dark:text-slate-400";
  }
  return "bg-red-500/20 border-red-500/30 text-red-600 dark:text-red-400";
};

const formatStatusLabel = (status = "missing") => {
  const normalized = String(status || "missing")
    .trim()
    .toLowerCase();
  return normalized.charAt(0).toUpperCase() + normalized.slice(1);
};

const renderServiceCard = (service, selectedService) => {
  const selected = service.id === selectedService;
  const icon = globalServiceIcons[service.id] || "widgets";
  const statusClass = statusChipClass(service.status);
  const isActive = isServiceActive(service);
  const primaryAction = isActive ? "restart" : "start";
  const primaryLabel = isActive ? "Restart" : "Start";
  const primaryIcon = isActive ? "restart_alt" : "play_arrow";
  const showRoutingWarning = hasRoutingImpact(service);
  const routingWarning = showRoutingWarning
    ? `<div class="mt-2 rounded-md border border-amber-500/40 bg-amber-500/10 px-2 py-1.5 text-[10px] text-amber-700 dark:text-amber-300 font-medium flex items-start gap-1.5">
          <span class="material-symbols-outlined text-[13px] leading-none mt-[1px]">warning</span>
          <span>Routing warning: ${escapeHTML(service.name)} is stopped. Proxy/domain routing may fail.</span>
        </div>`
    : "";
  const rowClass = selected
    ? "bg-primary/5 dark:bg-primary/10 border border-primary/40 shadow-[0_0_0_1px_rgba(13,242,89,0.25)]"
    : "bg-white dark:bg-transparent border border-slate-200 dark:border-border-primary hover:border-primary/30 hover:bg-slate-50 dark:hover:bg-background-secondary/20";
  const serviceName = escapeHTML(service.name);
  const containerName = escapeHTML(service.containerName);
  const statusLabel = escapeHTML(formatStatusLabel(service.status));

  return `
    <article
      data-action="global-service-select-log"
      data-service="${service.id}"
      class="rounded-xl border ${rowClass} p-3 transition-all cursor-pointer"
      title="Select ${serviceName} logs"
    >
      <div class="flex items-start justify-between gap-3">
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <span class="material-symbols-outlined text-primary text-[18px]">${icon}</span>
            <h4 class="text-sm font-semibold text-slate-900 dark:text-white truncate">${serviceName}</h4>
          </div>
          <p class="text-[11px] text-slate-600 dark:text-slate-400 mt-1 truncate font-medium">${containerName}</p>
        </div>
        <span class="px-2 py-1 rounded-md border text-[10px] font-bold uppercase tracking-wide shrink-0 ${statusClass}">
          ${statusLabel}
        </span>
      </div>
      ${routingWarning}
      <div class="mt-3 grid grid-cols-3 gap-1.5">
        <button
          data-action="global-service-primary"
          data-service="${service.id}"
          data-operation="${primaryAction}"
          data-loading-label="${primaryAction === "restart" ? "Restarting..." : "Starting..."}"
          class="h-9 rounded-lg text-[10px] font-bold bg-primary text-background-secondary hover:bg-primary-hover transition-all active:scale-95 flex items-center justify-center gap-1 disabled:opacity-70 disabled:cursor-not-allowed disabled:active:scale-100 shadow-sm"
        >
          <span class="material-symbols-outlined text-[14px]">${primaryIcon}</span>
          ${primaryLabel}
        </button>
        <button
          data-action="global-service-stop"
          data-service="${service.id}"
          data-loading-label="Stopping..."
          class="${isActive ? "h-9 rounded-lg text-[10px] font-bold bg-red-600 text-white border border-red-500 hover:bg-red-500 transition-all active:scale-95 flex items-center justify-center gap-1 disabled:opacity-70 disabled:cursor-not-allowed disabled:active:scale-100 shadow-sm" : "h-9 rounded-lg text-[10px] font-bold bg-background-secondary text-slate-500 dark:text-text-tertiary border border-border-primary opacity-90 dark:opacity-60 flex items-center justify-center gap-1"}"
          ${isActive ? "" : "disabled"}
        >
          <span class="material-symbols-outlined text-[14px] fill-1" style="font-variation-settings: &quot;FILL&quot; 1">stop</span>
          Stop
        </button>
        <button
          data-action="global-service-open"
          data-service="${service.id}"
          data-loading-label="Opening..."
          class="${service.openable ? "h-9 rounded-lg text-[10px] font-bold bg-background-primary text-text-primary border border-border-primary hover:bg-background-secondary transition-all active:scale-95 flex items-center justify-center gap-1 disabled:opacity-70 disabled:cursor-not-allowed disabled:active:scale-100 shadow-sm" : "h-9 rounded-lg text-[10px] font-bold border border-border-primary text-slate-500 dark:text-text-tertiary bg-background-secondary opacity-90 dark:opacity-60 flex items-center justify-center gap-1"}"
          ${service.openable ? "" : "disabled"}
        >
          <span class="material-symbols-outlined text-[14px]">open_in_new</span>
          Open
        </button>
      </div>
    </article>
  `;
};

export const renderGlobalServices = (
  container,
  services = [],
  selectedService = "",
) => {
  if (!container) {
    return;
  }
  if (!Array.isArray(services) || services.length === 0) {
    container.innerHTML = `
      <div class="rounded-xl border border-dashed border-border-primary bg-background-secondary p-4 text-sm text-text-tertiary">
        Global services data unavailable.
      </div>
    `;
    return;
  }

  container.innerHTML = services
    .map((service) => renderServiceCard(service, selectedService))
    .join("");
};

const statusStripClass = (service = {}) => {
  const status = String(service.status || "")
    .trim()
    .toLowerCase();
  if (isServiceActive(service)) {
    return {
      chip: "border-primary/30 bg-primary/10 text-emerald-700 dark:text-primary",
      dot: "bg-primary shadow-[0_0_8px_rgba(13,242,89,0.85)]",
    };
  }
  if (status === "restarting" || status === "starting" || status === "paused") {
    return {
      chip: "border-amber-500/35 bg-amber-500/15 text-amber-300",
      dot: "bg-amber-400",
    };
  }
  if (status === "missing") {
    return {
      chip: "border-slate-500/30 bg-slate-500/10 text-slate-700 dark:text-slate-300",
      dot: "bg-slate-400",
    };
  }
  return {
    chip: "border-red-500/35 bg-red-500/10 text-red-700 dark:text-red-300",
    dot: "bg-red-400",
  };
};

const renderStatusStrip = (container, services = []) => {
  if (!container) {
    return;
  }
  if (!Array.isArray(services) || services.length === 0) {
    container.innerHTML =
      '<span class="inline-flex items-center gap-1 rounded-md border border-border-primary bg-background-secondary px-2 py-1 text-[10px] text-text-tertiary">Loading services...</span>';
    return;
  }
  container.innerHTML = services
    .map((service, index) => {
      const tone = statusStripClass(service);
      const name = escapeHTML(service.name);
      const label = escapeHTML(formatStatusLabel(service.status));
      const icon = globalServiceIcons[service.id] || "widgets";
      return `<span class="global-status-chip inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-[10px] font-medium shadow-sm ${tone.chip}" style="--chip-order:${index}">
          <span class="w-1.5 h-1.5 rounded-full ${tone.dot}"></span>
          <span class="material-symbols-outlined text-[11px] leading-none opacity-90">${icon}</span>
          <span class="text-text-primary/95">${name}</span>
          <span class="opacity-80">${label}</span>
        </span>`;
    })
    .join("");
};

const applyButtonState = (button, className, enabled) => {
  if (!(button instanceof HTMLElement)) {
    return;
  }
  button.className = className;
  button.disabled = !enabled;
};

const withButtonLoading = async (buttonLike, fallbackLabel, operation) => {
  const hasHTMLElement = typeof HTMLElement !== "undefined";
  const hasHTMLButtonElement = typeof HTMLButtonElement !== "undefined";
  const isButtonElement =
    hasHTMLButtonElement && buttonLike instanceof HTMLButtonElement;
  const isHTMLElement = hasHTMLElement && buttonLike instanceof HTMLElement;
  const button = isButtonElement
    ? buttonLike
    : isHTMLElement
      ? buttonLike.closest("button")
      : null;

  if (!(hasHTMLButtonElement && button instanceof HTMLButtonElement)) {
    return operation();
  }

  if (button.dataset.busy === "true") {
    return null;
  }

  const previousHTML = button.innerHTML;
  const previousDisabled = button.disabled;
  const previousAriaBusy = button.getAttribute("aria-busy");
  const loadingLabel =
    String(
      button.dataset.loadingLabel || fallbackLabel || "Processing...",
    ).trim() || "Processing...";

  button.dataset.busy = "true";
  button.disabled = true;
  button.setAttribute("aria-busy", "true");
  button.innerHTML = `<span class="material-symbols-outlined text-[14px] animate-spin">progress_activity</span>${loadingLabel}`;

  try {
    return await operation();
  } finally {
    delete button.dataset.busy;
    if (!button.isConnected) {
      return;
    }
    button.disabled = previousDisabled;
    if (previousAriaBusy === null) {
      button.removeAttribute("aria-busy");
    } else {
      button.setAttribute("aria-busy", previousAriaBusy);
    }
    button.innerHTML = previousHTML;
  }
};

export const createGlobalServicesController = ({
  bridge,
  runtime,
  refs,
  getState,
  setState,
  onStatus,
  onToast,
}) => {
  const updateRefs = (nextRefs) => {
    refs = nextRefs;
  };

  let liveEnabled = false;
  let pollTimer = null;
  let rawLogOutput = "";

  const resolveLogViewport = () =>
    refs.globalLogViewport || refs.globalLogOutput?.parentElement || null;

  const scrollToLatest = (force = false) => {
    if (!force && !liveEnabled) {
      return;
    }
    const viewport = resolveLogViewport();
    if (!viewport) {
      return;
    }
    viewport.scrollTop = viewport.scrollHeight;
  };

  const readLogFilters = () => {
    const state = getState();
    return {
      severity: normalizeLogSeverity(state.globalLogSeverity || "all"),
      query: String(state.globalLogQuery || "").trim(),
    };
  };

  const syncLogFilterControls = () => {
    const { severity, query } = readLogFilters();
    if (refs.globalLogSearch && refs.globalLogSearch.value !== query) {
      refs.globalLogSearch.value = query;
    }
    syncSeveritySelector(refs.globalLogSeverity, severity);
  };

  const buildEmptyLogMessage = () => {
    const state = getState();
    const selectedID = String(state.selectedGlobalService || "")
      .trim()
      .toLowerCase();
    if (!selectedID) {
      return "Select a global service to view logs.";
    }
    if (selectedID === "dnsmasq") {
      return "DNSMasq is running but does not emit stdout logs by default.";
    }
    const selectedService = (state.globalServices || []).find(
      (item) => item.id === selectedID,
    );
    const serviceName = selectedService?.name || selectedID;
    return `No logs available for ${serviceName}.`;
  };

  const clearPoll = () => {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  };

  const renderLogOutput = ({ forceScroll = false } = {}) => {
    if (!refs.globalLogOutput) {
      return;
    }
    const rawTrimmed = String(rawLogOutput || "").trim();
    const { severity, query } = readLogFilters();
    const filtered = filterLogsText(rawLogOutput, severity, query);
    const filteredTrimmed = String(filtered || "").trim();

    if (!rawTrimmed) {
      refs.globalLogOutput.textContent = buildEmptyLogMessage();
    } else {
      refs.globalLogOutput.textContent =
        filteredTrimmed || "No logs match the current filters.";
    }
    scrollToLatest(forceScroll);
  };

  const setActionFeedback = (message, tone = "info") => {
    const toneMap = {
      success: {
        icon: "check_circle",
        iconClass:
          "material-symbols-outlined text-[15px] leading-none self-start mt-px text-primary shadow-sm",
        textClass:
          "rounded-xl border border-primary/25 bg-primary/10 px-3 py-2.5 grid grid-cols-[auto_minmax(0,1fr)] items-start gap-x-2.5 text-xs text-primary dark:text-primary/95",
      },
      warning: {
        icon: "warning",
        iconClass:
          "material-symbols-outlined text-[15px] leading-none self-start mt-px text-amber-600 dark:text-amber-300",
        textClass:
          "rounded-xl border border-amber-500/30 bg-amber-500/10 px-3 py-2.5 grid grid-cols-[auto_minmax(0,1fr)] items-start gap-x-2.5 text-xs text-amber-700 dark:text-amber-200",
      },
      error: {
        icon: "error",
        iconClass:
          "material-symbols-outlined text-[15px] leading-none self-start mt-px text-red-600 dark:text-red-300",
        textClass:
          "rounded-xl border border-red-500/30 bg-red-500/10 px-3 py-2.5 grid grid-cols-[auto_minmax(0,1fr)] items-start gap-x-2.5 text-xs text-red-700 dark:text-red-200",
      },
      info: {
        icon: "info",
        iconClass:
          "material-symbols-outlined text-[15px] leading-none self-start mt-px text-slate-600 dark:text-primary",
        textClass:
          "rounded-xl border border-slate-200 dark:border-border-primary bg-slate-100 dark:bg-[#0f2015]/80 px-3 py-2.5 grid grid-cols-[auto_minmax(0,1fr)] items-start gap-x-2.5 text-xs text-slate-700 dark:text-slate-300",
      },
    };
    const toneConfig = toneMap[tone] || toneMap.info;

    if (refs.globalActionFeedback) {
      refs.globalActionFeedback.className = toneConfig.textClass;
      refs.globalActionFeedback.classList.remove("global-feedback-ping");
      if (typeof requestAnimationFrame === "function") {
        requestAnimationFrame(() => {
          if (refs.globalActionFeedback) {
            refs.globalActionFeedback.classList.add("global-feedback-ping");
          }
        });
      } else {
        refs.globalActionFeedback.classList.add("global-feedback-ping");
      }
    }
    if (refs.globalActionFeedbackIcon) {
      refs.globalActionFeedbackIcon.className = toneConfig.iconClass;
      refs.globalActionFeedbackIcon.textContent = toneConfig.icon;
    }
    setText(
      refs.globalActionFeedbackText,
      message || "Ready for global operations.",
    );
  };

  const syncBulkActionButtons = (snapshot) => {
    const total = Number(snapshot.total || 0);
    const active = Number(snapshot.active || 0);
    const allRunning = total > 0 && active >= total;
    const hasRoutingWarning = hasRoutingWarningInSnapshot(snapshot);
    const anyRunning = active > 0;
    const canStart = !allRunning || hasRoutingWarning;

    applyButtonState(
      refs.globalBulkStart,
      canStart ? BULK_START_ENABLED_CLASS : BULK_START_DISABLED_CLASS,
      canStart,
    );
    applyButtonState(
      refs.globalBulkRestart,
      anyRunning ? BULK_RESTART_ENABLED_CLASS : BULK_RESTART_DISABLED_CLASS,
      anyRunning,
    );
    applyButtonState(
      refs.globalBulkStop,
      anyRunning ? BULK_STOP_ENABLED_CLASS : BULK_STOP_DISABLED_CLASS,
      anyRunning,
    );
    applyButtonState(refs.globalBulkPull, BULK_PULL_CLASS, true);
  };

  const renderSummary = (snapshot) => {
    const services = Array.isArray(snapshot.services) ? snapshot.services : [];
    const warningList = Array.isArray(snapshot.warnings)
      ? snapshot.warnings
      : [];
    const total = Number(snapshot.total || services.length);
    const active = Number(
      snapshot.active ||
        services.filter((service) => isServiceActive(service)).length,
    );
    const runningSafe = Math.max(0, Math.min(active, total || active));
    const percent = total > 0 ? Math.round((runningSafe / total) * 100) : 0;
    const hasRoutingWarning = hasRoutingWarningInSnapshot(snapshot);
    const offlineServices = Math.max(total - runningSafe, 0);
    const defaultSummary =
      total > 0
        ? `${runningSafe}/${total} global services running`
        : "No global services detected";

    setText(refs.globalServicesSummary, snapshot.summary || defaultSummary);
    setText(refs.globalServiceCount, `${runningSafe}/${total} running`);
    setText(refs.globalServiceHealthPercent, `${percent}%`);

    if (refs.globalServiceHealthBar instanceof HTMLElement) {
      refs.globalServiceHealthBar.style.width = `${percent}%`;
      refs.globalServiceHealthBar.className =
        hasRoutingWarning || percent < 35
          ? "h-full rounded-full bg-gradient-to-r from-red-500 via-red-400 to-amber-300 transition-all duration-500"
          : percent < 100
            ? "h-full rounded-full bg-gradient-to-r from-amber-500 via-amber-300 to-primary transition-all duration-500"
            : "h-full rounded-full bg-gradient-to-r from-primary/70 via-primary to-[#7dffad] transition-all duration-500";
    }

    if (refs.globalServiceHealthLabel instanceof HTMLElement) {
      if (hasRoutingWarning) {
        refs.globalServiceHealthLabel.className =
          "mt-2 inline-flex items-center gap-1.5 rounded-md border border-amber-500/35 bg-amber-500/10 px-2 py-1 text-[10px] font-bold text-amber-600 dark:text-amber-200";
        setText(refs.globalServiceHealthLabelIcon, "warning");
        setText(refs.globalServiceHealthLabelText, "Routing degraded");
      } else if (percent >= 100 && total > 0) {
        refs.globalServiceHealthLabel.className =
          "mt-2 inline-flex items-center gap-1.5 rounded-md border border-primary/25 bg-primary/10 px-2 py-1 text-[10px] font-bold text-emerald-600 dark:text-primary";
        setText(refs.globalServiceHealthLabelIcon, "task_alt");
        setText(refs.globalServiceHealthLabelText, "All systems nominal");
      } else if (runningSafe === 0 && total > 0) {
        refs.globalServiceHealthLabel.className =
          "mt-2 inline-flex items-center gap-1.5 rounded-md border border-red-500/35 bg-red-500/10 px-2 py-1 text-[10px] font-bold text-red-600 dark:text-red-200";
        setText(refs.globalServiceHealthLabelIcon, "error");
        setText(refs.globalServiceHealthLabelText, "Service mesh offline");
      } else {
        refs.globalServiceHealthLabel.className =
          "mt-2 inline-flex items-center gap-1.5 rounded-md border border-amber-500/35 bg-amber-500/10 px-2 py-1 text-[10px] font-bold text-amber-600 dark:text-amber-200";
        setText(refs.globalServiceHealthLabelIcon, "monitor_heart");
        setText(
          refs.globalServiceHealthLabelText,
          `${offlineServices} service${offlineServices === 1 ? "" : "s"} need attention`,
        );
      }
    }

    renderStatusStrip(refs.globalServiceStatusStrip, services);
    syncBulkActionButtons({ active: runningSafe, total });

    if (hasRoutingWarning) {
      setActionFeedback(
        buildRoutingWarningMessage(services, snapshot.warnings),
        "warning",
      );
    } else if (percent >= 100 && total > 0) {
      setActionFeedback(
        "All global services are healthy. Use Restart All for safe rolling refresh.",
        "success",
      );
    } else if (total > 0) {
      setActionFeedback(
        `${offlineServices} service${offlineServices === 1 ? "" : "s"} are offline. Start All can recover quickly.`,
        "warning",
      );
    } else {
      setActionFeedback("Global services are not available yet.", "info");
    }
  };

  const renderSnapshot = () => {
    const state = getState();
    const services = state.globalServices || [];
    const selected = state.selectedGlobalService || "";
    renderGlobalServices(refs.globalServicesList, services, selected);
    const selectedService = services.find((item) => item.id === selected);
    setText(
      refs.globalLogServiceName,
      selectedService ? selectedService.name : "Select service",
    );
    syncLogFilterControls();
  };

  const ensureSelectedService = () => {
    const state = getState();
    const services = state.globalServices || [];
    if (!services.length) {
      setState({ selectedGlobalService: "" });
      return "";
    }
    const selected = state.selectedGlobalService || "";
    if (services.some((item) => item.id === selected)) {
      return selected;
    }
    const preferred =
      services.find((item) => isServiceActive(item)) || services[0];
    setState({ selectedGlobalService: preferred.id });
    return preferred.id;
  };

  const refresh = async ({ silent = false } = {}) => {
    if (!silent && refs.globalServicesList) {
      refs.globalServicesList.innerHTML = `
        <div class="rounded-xl border border-dashed border-border-primary bg-surface-secondary p-4 text-sm text-slate-400">Loading global services...</div>
      `;
    }
    if (!silent) {
      setActionFeedback("Refreshing global services snapshot...", "info");
    }
    try {
      const snapshot = normalizeGlobalServicesSnapshot(
        await bridge.getGlobalServices(),
      );
      setState({ globalServices: snapshot.services });
      ensureSelectedService();
      renderSummary(snapshot);
      renderSnapshot();
      if (snapshot.warnings?.length) {
        onStatus(`Global services warnings: ${snapshot.warnings.join(" | ")}`);
      }
      return snapshot;
    } catch (err) {
      onStatus(`Failed to load global services: ${err}`);
      setActionFeedback(`Failed to load global services: ${err}`, "error");
      if (refs.globalServicesList) {
        refs.globalServicesList.innerHTML = `
          <div class="rounded-xl border border-dashed border-red-500/40 bg-red-500/10 p-4 text-sm text-red-300">
            Failed to load global services.
          </div>
        `;
      }
      return null;
    }
  };

  const appendLogLine = (line) => {
    const value = String(line || "").trim();
    if (!value) {
      return;
    }
    rawLogOutput = rawLogOutput ? `${rawLogOutput}\n${value}` : value;
    renderLogOutput();
  };

  const refreshLogs = async () => {
    const serviceID = String(getState().selectedGlobalService || "").trim();
    if (!serviceID) {
      rawLogOutput = "";
      renderLogOutput({ forceScroll: true });
      return;
    }
    if (refs.globalLogOutput) {
      refs.globalLogOutput.textContent = "Loading logs...";
    }
    try {
      rawLogOutput = String(
        (await bridge.getGlobalServiceLogs(serviceID, 300)) || "",
      );
      renderLogOutput({ forceScroll: true });
    } catch (err) {
      rawLogOutput = `Failed to load logs: ${err}`;
      renderLogOutput({ forceScroll: true });
    }
  };

  const stopLive = async ({ skipBridge = false } = {}) => {
    liveEnabled = false;
    setState({ globalLiveLogsEnabled: false });
    clearPoll();
    setText(refs.globalToggleLive, "Live: Off");
    setActionFeedback("Live log stream paused.", "info");
    if (skipBridge) {
      return;
    }
    try {
      await bridge.stopGlobalServiceLogStream();
    } catch (_err) {
      // Ignore: polling fallback mode may not hold stream state.
    }
  };

  const startLive = async () => {
    const serviceID = String(getState().selectedGlobalService || "").trim();
    if (!serviceID) {
      onStatus("Select a global service to stream logs.");
      setActionFeedback("Select a service first to stream logs.", "warning");
      return;
    }
    liveEnabled = true;
    setState({ globalLiveLogsEnabled: true });
    setText(refs.globalToggleLive, "Live: On");
    const serviceName =
      getState().globalServices.find((item) => item.id === serviceID)?.name ||
      serviceID;
    setActionFeedback(`Streaming live logs for ${serviceName}.`, "info");

    if (bridge.startGlobalServiceLogStream && runtime?.EventsOn) {
      try {
        await bridge.startGlobalServiceLogStream(serviceID);
        return;
      } catch (_err) {
        // Fall through to polling fallback.
      }
    }

    await refreshLogs();
    clearPoll();
    pollTimer = setInterval(refreshLogs, 2000);
  };

  const toggleLive = async () => {
    if (liveEnabled) {
      await stopLive();
      return;
    }
    await startLive();
  };

  const selectService = async (serviceID) => {
    const normalized = String(serviceID || "")
      .trim()
      .toLowerCase();
    if (!normalized) {
      return;
    }
    const state = getState();
    if (!state.globalServices.some((item) => item.id === normalized)) {
      return;
    }
    setState({ selectedGlobalService: normalized });
    renderSnapshot();
    if (liveEnabled) {
      await stopLive();
      await startLive();
      return;
    }
    await refreshLogs();
  };

  const runServiceAction = async (action, serviceID, triggerButton = null) => {
    const normalized = String(serviceID || "")
      .trim()
      .toLowerCase();
    if (!normalized) {
      return;
    }

    const actions = {
      start: bridge.startGlobalService,
      stop: bridge.stopGlobalService,
      restart: bridge.restartGlobalService,
      open: bridge.openGlobalService,
    };
    const fn = actions[action];
    if (!fn) {
      return;
    }

    const loadingLabelByAction = {
      start: "Starting...",
      stop: "Stopping...",
      restart: "Restarting...",
      open: "Opening...",
    };
    const actionVerbByAction = {
      start: "Starting",
      stop: "Stopping",
      restart: "Restarting",
      open: "Opening",
    };
    const serviceName =
      getState().globalServices.find((item) => item.id === normalized)?.name ||
      normalized;

    const settleRoutingIfNeeded = async (snapshot) => {
      if (action !== "start" && action !== "restart") {
        return snapshot;
      }

      const isRoutingService =
        normalized === "caddy" || normalized === "dnsmasq";
      if (!isRoutingService) {
        return snapshot;
      }

      let nextSnapshot = snapshot;
      const maxAttempts = 4;
      for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
        if (!nextSnapshot || !hasRoutingWarningInSnapshot(nextSnapshot)) {
          return nextSnapshot;
        }
        setActionFeedback(
          `Waiting for routing bindings to stabilize (${attempt + 1}/${maxAttempts})...`,
          "info",
        );
        await delay(1200);
        nextSnapshot = await refresh({ silent: true });
      }
      return nextSnapshot;
    };

    await withButtonLoading(
      triggerButton,
      loadingLabelByAction[action],
      async () => {
        setActionFeedback(
          `${actionVerbByAction[action] || "Processing"} ${serviceName}...`,
          "info",
        );
        try {
          const message = await fn(normalized);
          const compactMessage = summarizeActionMessage(
            message,
            `${serviceName} ${action} completed.`,
          );
          onStatus(compactMessage);
          onToast(compactMessage, "success");
          const snapshot = await settleRoutingIfNeeded(
            await refresh({ silent: true }),
          );
          if (snapshot && hasRoutingWarningInSnapshot(snapshot)) {
            setActionFeedback(
              buildRoutingWarningMessage(snapshot.services, snapshot.warnings),
              "warning",
            );
          } else {
            setActionFeedback(compactMessage, "success");
          }
        } catch (err) {
          onStatus(`${action} ${normalized} failed: ${err}`);
          onToast(`${action} ${normalized} failed: ${err}`, "error");
          setActionFeedback(
            `${actionVerbByAction[action] || "Action"} ${serviceName} failed: ${err}`,
            "error",
          );
        }
      },
    );
  };

  const runBulkAction = async (action, triggerButton = null) => {
    const actions = {
      start: bridge.startGlobalServices,
      stop: bridge.stopGlobalServices,
      restart: bridge.restartGlobalServices,
      pull: bridge.pullGlobalServices,
    };
    const fn = actions[action];
    if (!fn) {
      return;
    }

    const loadingLabelByAction = {
      start: "Starting All...",
      stop: "Stopping All...",
      restart: "Restarting All...",
      pull: "Pulling All...",
    };
    const actionVerbByAction = {
      start: "Starting",
      stop: "Stopping",
      restart: "Restarting",
      pull: "Pulling",
    };

    const settleRoutingIfNeeded = async (snapshot) => {
      if (action !== "start" && action !== "restart") {
        return snapshot;
      }

      let nextSnapshot = snapshot;
      const maxAttempts = 6;
      for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
        if (!nextSnapshot || !hasRoutingWarningInSnapshot(nextSnapshot)) {
          return nextSnapshot;
        }
        setActionFeedback(
          `Waiting for routing bindings to stabilize (${attempt + 1}/${maxAttempts})...`,
          "info",
        );
        await delay(1200);
        nextSnapshot = await refresh({ silent: true });
      }
      return nextSnapshot;
    };

    await withButtonLoading(
      triggerButton,
      loadingLabelByAction[action],
      async () => {
        setActionFeedback(
          `${actionVerbByAction[action] || "Processing"} all global services...`,
          "info",
        );
        try {
          const message = await fn();
          const compactMessage = summarizeActionMessage(
            message,
            `Global ${action} completed successfully.`,
          );
          onStatus(compactMessage);
          onToast(compactMessage, "success");
          const snapshot = await settleRoutingIfNeeded(
            await refresh({ silent: true }),
          );
          if (snapshot && hasRoutingWarningInSnapshot(snapshot)) {
            setActionFeedback(
              buildRoutingWarningMessage(snapshot.services, snapshot.warnings),
              "warning",
            );
          } else {
            setActionFeedback(compactMessage, "success");
          }
        } catch (err) {
          const compactError = formatBulkGlobalActionError(action, err);
          onStatus(`Global ${action} failed: ${err}`);
          onToast(compactError, "error");
          setActionFeedback(compactError, "error");
        }
      },
    );
  };

  const clearLogs = async () => {
    await stopLive();
    rawLogOutput = "";
    renderLogOutput({ forceScroll: true });
    onStatus("Global service logs cleared.");
    setActionFeedback("Global service logs cleared.", "info");
  };

  const downloadLogs = async () => {
    const output = String(rawLogOutput || "").trim();
    if (!output) {
      onStatus("No global logs available to download.");
      onToast("No global logs available to download.", "warning");
      return;
    }

    const state = getState();
    const selectedID = String(state.selectedGlobalService || "").trim();
    const selectedService = (state.globalServices || []).find(
      (item) => item.id === selectedID,
    );
    const filename = buildLogFilename({
      scope: "global",
      project: "services",
      service: selectedService?.name || selectedID || "all",
    });

    let nativeExportError = null;
    if (bridge?.saveLogsToFile) {
      try {
        const response = await bridge.saveLogsToFile(output, filename);
        const message = String(response || "").trim();
        if (message.toLowerCase().includes("cancelled")) {
          onStatus(message || "Global log export cancelled.");
          setActionFeedback(message || "Global log export cancelled.", "info");
          return;
        }
        onStatus(message || "Global logs downloaded successfully.");
        onToast("Global logs downloaded successfully.", "success");
        setActionFeedback(
          message || "Global logs downloaded successfully.",
          "success",
        );
        return;
      } catch (err) {
        nativeExportError = err;
      }
    }

    const downloaded = downloadTextAsFile(output, filename);
    if (!downloaded) {
      const details =
        nativeExportError !== null
          ? `Failed to download global logs: ${nativeExportError}`
          : "Failed to download global logs.";
      onStatus(details);
      onToast("Failed to download global logs.", "error");
      setActionFeedback("Failed to download global logs.", "error");
      return;
    }

    onStatus("Global logs downloaded successfully.");
    onToast("Global logs downloaded successfully.", "success");
    setActionFeedback("Global logs downloaded successfully.", "success");
  };

  const applyFilters = () => {
    syncLogFilterControls();
    renderLogOutput();
  };

  if (runtime?.EventsOn) {
    runtime.EventsOn("global-logs:line", appendLogLine);
    runtime.EventsOn("global-logs:status", (message) => {
      onStatus(String(message || "").trim());
    });
    runtime.EventsOn("global-logs:error", (message) => {
      const text = String(message || "").trim();
      if (!text) {
        return;
      }
      onStatus(text);
      onToast(text, "error");
    });
  }

  return {
    refresh,
    refreshLogs,
    applyFilters,
    stopLive,
    toggleLive,
    clearLogs,
    downloadLogs,
    selectService,
    runServiceAction,
    runBulkAction,
    updateRefs,
  };
};
