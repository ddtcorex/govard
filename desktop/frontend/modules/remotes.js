/**
 * The remotes tab's data shape and its pure helpers.
 *
 * #tab-remotes and the sync modal are React islands now (spec D5/D9), so
 * everything that touched a document moved with them: this module used to build
 * markup with innerHTML, own the two-card hover listeners and hold the
 * createRemotesController state machine, and none of that is left. What stays is
 * the part that has no element in it - the payload normalizers, the capability
 * gate, the preset table, the sync-plan text builders and the progress buffer -
 * which is why the module no longer imports a DOM helper at all.
 */

const asNumber = (value, fallback = 0) => {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
};

const normalizeCapabilities = (value) => {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((item) =>
      String(item || "")
        .trim()
        .toLowerCase(),
    )
    .filter((item) => item !== "");
};

export const formatAuthMethodLabel = (authMethod) => {
  const normalized = String(authMethod || "")
    .trim()
    .toLowerCase();
  if (normalized === "ssh-agent") {
    return "SSH Agent";
  }
  if (normalized === "keyfile") {
    return "Key File";
  }
  if (normalized === "keychain") {
    return "Keychain";
  }
  if (!normalized) {
    return "Keychain";
  }
  return normalized;
};

const normalizeRemote = (remote = {}) => ({
  name: String(remote.name || remote.Name || "").trim(),
  host: String(remote.host || remote.Host || "").trim(),
  user: String(remote.user || remote.User || "").trim(),
  path: String(remote.path || remote.Path || "").trim(),
  port: asNumber(remote.port ?? remote.Port, 22),
  protected: Boolean(remote.protected ?? remote.Protected),
  authMethod: String(remote.authMethod || remote.AuthMethod || "keychain")
    .trim()
    .toLowerCase(),
  lastSync: String(remote.lastSync || remote.LastSync || "").trim(),
  capabilities: normalizeCapabilities(
    remote.capabilities || remote.Capabilities,
  ),
});

export const normalizeRemotesPayload = (payload = {}) => {
  const remotesRaw = Array.isArray(payload.remotes)
    ? payload.remotes
    : Array.isArray(payload.Remotes)
      ? payload.Remotes
      : [];

  const warningsRaw = Array.isArray(payload.warnings)
    ? payload.warnings
    : Array.isArray(payload.Warnings)
      ? payload.Warnings
      : [];

  return {
    project: String(payload.project || payload.Project || "").trim(),
    remotes: remotesRaw.map(normalizeRemote),
    warnings: warningsRaw
      .map((item) => String(item || "").trim())
      .filter((item) => item !== ""),
  };
};

export const normalizeRemotePreset = (preset = "") => {
  const normalized = String(preset || "")
    .trim()
    .toLowerCase();
  if (["file", "files", "source", "code"].includes(normalized)) {
    return "files";
  }
  if (["media", "assets"].includes(normalized)) {
    return "media";
  }
  if (["db", "database"].includes(normalized)) {
    return "db";
  }
  if (["full", "all"].includes(normalized)) {
    return "full";
  }
  return "";
};

/**
 * One string, so the notice the card renders and the assertion that guards its
 * wording can never drift apart (it must not name a production environment: the
 * same notice shows for every protected remote).
 */
export const protectedWarningCopy =
  "Syncing from a protected remote can overwrite local data. Consider creating a snapshot before syncing.";

/** The sync reason a disabled pull button carries while another one runs. */
export const syncInProgressReason = "Another sync is currently in progress.";

/**
 * A remote that declares no capabilities keeps every pull enabled; one that
 * declares a list is gated on the preset's own capability. Pull-everything has
 * no capability of its own and is therefore never gated.
 */
export const canUseSyncPreset = (remote, preset) => {
  const capabilities = Array.isArray(remote?.capabilities)
    ? remote.capabilities
    : [];
  if (capabilities.length === 0) {
    return true;
  }
  if (preset === "db") {
    return capabilities.includes("db");
  }
  if (preset === "media") {
    return capabilities.includes("media");
  }
  return true;
};

/**
 * The three pull buttons, in the order the card renders them. `capability` is
 * what canUseSyncPreset gates on; `disabledReason` is what the button's title
 * says when the gate - rather than a running sync - is what disabled it.
 */
export const syncPresets = [
  {
    preset: "full",
    icon: "all_inclusive",
    label: "Pull Everything",
    iconHoverClass: "group-hover/btn:text-purple-400",
    capability: "",
    disabledReason: "",
  },
  {
    preset: "db",
    icon: "database",
    label: "Pull Database",
    iconHoverClass: "group-hover/btn:text-primary",
    capability: "db",
    disabledReason:
      "Database sync is disabled for this remote (capability: db).",
  },
  {
    preset: "media",
    icon: "perm_media",
    label: "Pull Media",
    iconHoverClass: "group-hover/btn:text-blue-400",
    capability: "media",
    disabledReason: "Media sync is disabled for this remote (capability: media).",
  },
];

export const getSyncPresetName = (preset) => {
  const normalized = String(preset || "")
    .trim()
    .toLowerCase();
  if (normalized === "full" || normalized === "bootstrap") {
    return "Pull Everything";
  }
  if (normalized === "db") {
    return "Pull Database";
  }
  if (normalized === "media") {
    return "Pull Media";
  }
  if (normalized === "files") {
    return "Pull Files";
  }
  return normalized || "Sync";
};

export const getSyncPresetLabel = (preset, remoteName) => {
  if (preset === "full" || preset === "bootstrap") {
    return `Setting up from ${remoteName}...`;
  }
  if (preset === "db") {
    return `Pulling database from ${remoteName}...`;
  }
  if (preset === "media") {
    return `Pulling media from ${remoteName}...`;
  }
  return `Syncing from ${remoteName}...`;
};

/**
 * The "Selected Pull Configuration" block the preview step shows above the
 * backend's plan text.
 * @param {{
 *   remoteName?: string,
 *   preset?: string,
 *   config?: Record<string, unknown>,
 *   optionDefs?: Array<{key?: string, label?: string}>,
 * }} input
 * @returns {string}
 */
export const formatSyncPlanDetails = ({
  remoteName,
  preset,
  config = {},
  optionDefs = [],
}) => {
  const selectedOptions = (optionDefs || [])
    .filter((option) => option && option.key && Boolean(config[option.key]))
    .map((option) => String(option.label || option.key));

  const selectedBlock = selectedOptions.length
    ? selectedOptions.map((option) => `- ${option}`).join("\n")
    : "- None";

  return [
    "Selected Pull Configuration",
    `Preset: ${getSyncPresetName(preset)}`,
    `Remote: ${String(remoteName || "").trim() || "-"}`,
    "Enabled options:",
    selectedBlock,
  ].join("\n");
};

/**
 * The backend's option defaults, laid over whatever the user already chose.
 * @param {Array<{key?: string, defaultValue?: unknown}>} [optionsDef]
 * @param {Record<string, unknown>} [currentConfig]
 * @returns {Record<string, unknown>}
 */
export const resolveSyncPresetConfig = (optionsDef = [], currentConfig = {}) => {
  const config = { ...(currentConfig || {}) };
  (optionsDef || []).forEach((option) => {
    if (!option || !option.key) {
      return;
    }
    if (config[option.key] === undefined) {
      config[option.key] = Boolean(option.defaultValue);
    }
  });
  return config;
};

const ansiSequencePattern = /\u001b\[[0-?]*[ -/]*[@-~]/g;
const orphanAnsiStylePattern = /\[(?:\d{1,3}(?:;\d{1,3})*)m/g;

export const sanitizeSyncToastLine = (value) => {
  const raw = String(value ?? "");
  if (!raw) {
    return "";
  }
  return raw
    .replace(ansiSequencePattern, "")
    .replace(orphanAnsiStylePattern, "")
    .replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g, "")
    .trim();
};

export const sanitizeSyncPlanText = (value) => {
  const raw = String(value ?? "");
  if (!raw) {
    return "";
  }

  const lines = raw.split(/\r?\n/).map((line) => sanitizeSyncToastLine(line));
  const compact = [];
  let previousWasBlank = false;

  lines.forEach((line) => {
    const blank = line === "";
    if (blank) {
      if (!previousWasBlank) {
        compact.push("");
      }
      previousWasBlank = true;
      return;
    }
    compact.push(line);
    previousWasBlank = false;
  });

  return compact.join("\n").trim();
};

/** Keeps a runaway sync from growing the terminal buffer without bound. */
export const maxSyncProgressLines = 300;

/**
 * One batch of `sync:output` folded into the terminal buffer. The backend sends
 * batched lines joined by \n to throttle IPC events, and a line written with a
 * carriage return overwrites the one already on screen (that is how the sync
 * binary draws progress), so this is a pure function rather than a DOM write.
 */
export const applySyncOutputBatch = (
  lines,
  rawBatch,
  maxLines = maxSyncProgressLines,
) => {
  let next = Array.isArray(lines) ? lines.slice() : [];

  for (const raw of String(rawBatch ?? "").split("\n")) {
    const crParts = raw.split("\r");
    const normalized = sanitizeSyncToastLine(crParts[crParts.length - 1]);
    if (!normalized) {
      continue;
    }

    if (crParts.length > 1 && next.length > 0) {
      next[next.length - 1] = normalized;
      continue;
    }

    next.push(normalized);
    if (next.length > maxLines) {
      next = next.slice(-maxLines);
    }
  }

  return next;
};

/**
 * What the remotes list falls back to when the backend answers nothing and no
 * event runtime exists (the preview and the browser-only dev loop).
 */
export const safeRemotes = {
  Remotes: [
    {
      Name: "Staging",
      Host: "192.168.1.45",
      LastSync: "2m ago",
      DbSize: "458 MB",
      MediaSize: "1.2 GB",
    },
    {
      Name: "Production",
      Host: "203.0.113.15",
      LastSync: "1h ago",
      DbSize: "2.4 GB",
      MediaSize: "8.5 GB",
      Protected: true,
    },
  ],
};
