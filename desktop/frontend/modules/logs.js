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

/**
 * The service strip's two halves as data: which targets the selected project
 * offers and which one of them is really selected. This is the pure part of the
 * selector that used to be rendered straight into the DOM; the logs island
 * renders `targets` and main.js keeps `service` in the store, so both worlds
 * resolve a selection the same way (spec D5).
 *
 * @param {Array<Record<string, unknown>>} environments
 * @param {string} project
 * @param {string} selectedService
 * @returns {{targets: string[], service: string}}
 */
export const resolveServiceTargets = (
  environments = [],
  project = "",
  selectedService = "all",
) => {
  const list = Array.isArray(environments) ? environments : [];
  const env = list.find((item) => projectKey(item) === project);
  const targets = env ? serviceTargets(env) : ["web"];
  const merged = ["all", ...targets.filter((target) => target !== "all")];
  return {
    targets: merged,
    service: merged.includes(selectedService) ? selectedService : merged[0],
  };
};

/**
 * The severity chip classes, shared by the two log panes now that both render
 * their strip from state instead of syncing a container's buttons by hand.
 */
export const severityChipClass = (active = false) =>
  `h-7 px-3 text-[10px] font-bold uppercase tracking-wide rounded-md border transition-colors ${
    active
      ? "bg-primary/20 text-primary dark:text-white border-primary/30"
      : "bg-surface-secondary dark:bg-surface-secondary text-text-tertiary dark:text-slate-400 border-transparent hover:bg-slate-100 dark:hover:bg-surface-primary hover:text-text-primary dark:hover:text-white transition-all"
  }`;
