
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

/**
 * Only a stopped (or unknown) environment offers Start: degraded, warning
 * and syncing are still running, so Stop stays enabled and the primary
 * action stays Restart.
 */
export const isStoppedStatus = (status) => {
  const normalized = String(status ?? "").trim().toLowerCase();
  return normalized === "stopped" || normalized === "";
};

/**
 * The single grouping behind every status presentation: hero badge tones,
 * dots and the environment-list classification all branch on this, so a
 * status can never read green in one view and amber in another.
 */
export const statusGroup = (status) => {
  const normalized = String(status ?? "").trim().toLowerCase();
  if (
    normalized === "running" ||
    normalized === "healthy" ||
    normalized === "up"
  )
    return "healthy";
  if (
    normalized === "syncing" ||
    normalized === "starting" ||
    normalized === "restarting" ||
    normalized === "booting"
  )
    return "transition";
  if (normalized === "warning" || normalized === "degraded") return "warn";
  return "stopped";
};

/**
 * Hero badge and dot tones, mirroring classifyEnvironmentStatus below:
 * green while healthy, blue while transitioning, amber while degraded.
 */
export const statusTone = (status) => {
  switch (statusGroup(status)) {
    case "healthy":
      return "bg-primary/20 border-primary/30 text-primary";
    case "transition":
      return "bg-blue-400/20 border-blue-400/30 text-blue-400";
    case "warn":
      return "bg-amber-500/20 border-amber-500/30 text-amber-500";
    default:
      return "bg-slate-500/10 border-slate-500/20 text-slate-600 dark:text-slate-400";
  }
};

export const dotTone = (status) => {
  switch (statusGroup(status)) {
    case "healthy":
      return "bg-primary shadow-[0_0_12px_rgba(13,242,89,0.9)] animate-pulse";
    case "transition":
      return "bg-blue-400";
    case "warn":
      return "bg-amber-500";
    default:
      return "bg-slate-500";
  }
};

const withScheme = (value) => {
  const raw = String(value || "").trim();
  if (!raw) {
    return "";
  }
  if (/^https?:\/\//i.test(raw)) {
    return raw;
  }

  const host = raw.split("/")[0].trim();
  const isLoopback = /^(localhost|127\.0\.0\.1|\[::1\])(?::\d+)?$/i.test(host);
  const scheme = isLoopback ? "http" : "https";
  return `${scheme}://${raw.replace(/^\/+/, "")}`;
};

export const localEnvironmentURL = (env = {}) => {
  const explicitURL =
    env.LocalURL || env.localURL || env.URL || env.Url || env.url || "";
  const explicitResolved = withScheme(explicitURL);
  if (explicitResolved) {
    return explicitResolved;
  }

  const candidate = String(
    env.Domain ||
    env.domain ||
    env.Name ||
    env.name ||
    env.Project ||
    env.project ||
    "",
  ).trim();
  if (!candidate) {
    return "";
  }

  let host = candidate;
  if (
    !/^https?:\/\//i.test(host) &&
    !host.includes(".") &&
    !host.includes(":")
  ) {
    host = `${host}.test`;
  }

  return withScheme(host);
};

const SERVICE_TARGET_ORDER = [
  "web",
  "php",
  "db",
  "redis",
  "valkey",
  "elasticsearch",
  "opensearch",
  "varnish",
  "rabbitmq",
  "mail",
  "pma",
];

const serviceListForTargets = (env = {}) => {
  if (Array.isArray(env.Services)) return env.Services;
  if (Array.isArray(env.services)) return env.services;
  return [];
};

const inferServiceTargetForFilter = (service = {}) => {
  const explicit = String(service.Target || service.target || "")
    .trim()
    .toLowerCase();
  if (explicit) {
    return explicit;
  }

  const name = String(service.Name || service.name || "")
    .trim()
    .toLowerCase();
  if (!name) {
    return "";
  }

  if (name === "web" || name === "nginx" || name === "apache") return "web";
  if (name === "php") return "php";
  if (
    name === "db" ||
    name === "database" ||
    name === "mariadb" ||
    name === "mysql" ||
    name === "postgresql" ||
    name === "postgres"
  ) {
    return "db";
  }
  if (name === "redis") return "redis";
  if (name === "valkey") return "valkey";
  if (name === "elasticsearch") return "elasticsearch";
  if (name === "opensearch") return "opensearch";
  if (name === "varnish") return "varnish";
  if (name === "rabbitmq") return "rabbitmq";
  if (name === "mailhog" || name === "mailpit" || name === "mail")
    return "mail";
  if (name === "pma" || name === "phpmyadmin") return "pma";
  return "";
};

const orderedUniqueTargets = (values = []) => {
  const seen = new Set();
  const extras = [];
  const known = new Set(SERVICE_TARGET_ORDER);

  values.forEach((value) => {
    const normalized = String(value || "")
      .trim()
      .toLowerCase();
    if (!normalized || seen.has(normalized)) {
      return;
    }
    seen.add(normalized);
    if (!known.has(normalized)) {
      extras.push(normalized);
    }
  });

  const ordered = SERVICE_TARGET_ORDER.filter((target) => seen.has(target));
  return ordered.concat(extras);
};

export const serviceTargets = (env = {}) => {
  const fromServices = orderedUniqueTargets(
    serviceListForTargets(env).map((service) =>
      inferServiceTargetForFilter(service),
    ),
  );
  if (fromServices.length) {
    return fromServices;
  }

  const values = Array.isArray(env.ServiceTargets)
    ? env.ServiceTargets
    : Array.isArray(env.serviceTargets)
      ? env.serviceTargets
      : [];
  const fromTargets = orderedUniqueTargets(values);
  return fromTargets.length ? fromTargets : ["web"];
};

const ACTIVE_ENVIRONMENT_STATUSES = new Set([
  "running",
  "warning",
  "degraded",
  "healthy",
  "up",
  "starting",
  "restarting",
  "booting",
  "syncing",
]);

const environmentServices = (env = {}) => {
  if (Array.isArray(env.Services)) return env.Services;
  if (Array.isArray(env.services)) return env.services;
  return [];
};

export const classifyEnvironmentStatus = (env = {}) => {
  const status = String(env.Status || env.status || "stopped").toLowerCase();
  const active = ACTIVE_ENVIRONMENT_STATUSES.has(status);
  const services = environmentServices(env);
  const serviceCount = services.length;

  const meta = {
    status,
    active,
    iconName: "stop_circle",
    iconClass: "text-text-tertiary",
    fill: false,
    detailClass: "text-text-tertiary",
    dotClass: "bg-text-tertiary",
    detailText: "Stopped",
    showPulseDot: false,
  };

  // Tone branches follow statusGroup: the list view keeps its own icon and
  // copy vocabulary, but a status can never land in a different color group
  // than the hero badge shows for it.
  switch (statusGroup(status)) {
    case "healthy": {
      meta.iconName = "play_circle";
      meta.iconClass = "text-primary fill-1";
      meta.fill = true;
      meta.detailClass = "text-primary";
      meta.dotClass = "bg-primary";
      meta.detailText =
        serviceCount > 0 ? `Running • ${serviceCount} services` : "Running";
      meta.showPulseDot = true;
      return meta;
    }

    case "transition": {
      meta.iconName = "sync";
      meta.iconClass = "text-blue-400";
      meta.detailClass = "text-blue-400";
      meta.dotClass = "bg-blue-400";
      meta.detailText =
        status === "starting"
          ? "Starting..."
          : status === "syncing"
            ? "Syncing..."
            : "Restarting...";
      return meta;
    }

    case "warn": {
      meta.iconName = "warning";
      meta.iconClass = "text-amber-500";
      meta.detailClass = "text-amber-500";
      meta.dotClass = "bg-amber-500";
      meta.detailText = status === "warning" ? "Warning" : "Degraded";
      return meta;
    }

    default:
      return meta;
  }
};

export const inferServiceTarget = (service = {}) => {
  const explicit = String(service.Target || service.target || "")
    .trim()
    .toLowerCase();
  if (explicit) {
    return explicit;
  }

  const name = String(service.Name || service.name || "")
    .trim()
    .toLowerCase();

  if (name.includes("php")) return "php";
  if (
    name.includes("maria") ||
    name.includes("mysql") ||
    name.includes("postgres") ||
    name.includes("database") ||
    name.includes("db")
  ) {
    return "db";
  }
  if (name.includes("opensearch")) return "opensearch";
  if (name.includes("elastic")) return "elasticsearch";
  if (name.includes("redis")) return "redis";
  if (name.includes("valkey")) return "valkey";
  if (name.includes("rabbit")) return "rabbitmq";
  if (name.includes("varnish")) return "varnish";
  if (
    name.includes("nginx") ||
    name.includes("apache") ||
    name.includes("proxy") ||
    name.includes("web")
  ) {
    return "web";
  }

  return "web";
};

