
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

  if (status === "running" || status === "healthy" || status === "up") {
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

  if (
    status === "restarting" ||
    status === "starting" ||
    status === "booting" ||
    status === "syncing"
  ) {
    meta.iconName = "sync";
    meta.iconClass = "text-blue-400";
    meta.detailClass = "text-blue-400";
    meta.dotClass = "bg-blue-400";
    meta.detailText = status === "starting" ? "Starting..." : "Restarting...";
    return meta;
  }

  if (status === "warning") {
    meta.iconName = "warning";
    meta.iconClass = "text-amber-500";
    meta.detailClass = "text-amber-500";
    meta.dotClass = "bg-amber-500";
    meta.detailText = "Warning";
    return meta;
  }

  return meta;
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

