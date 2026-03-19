import { clearChildren, escapeHTML } from "../utils/dom.js";

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

const formatAuthMethodLabel = (authMethod) => {
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

const renderWarnings = (container, warnings = []) => {
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

const canUseSyncPreset = (remote, preset) => {
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

const renderSyncPresetButton = ({
  remoteName,
  preset,
  icon,
  label,
  iconHoverClass,
  enabled,
  disabledReason,
}) => {
  const buttonClasses = enabled
    ? "flex-1 px-4 py-2.5 bg-background-secondary hover:bg-[var(--border-primary)] border border-border-primary dark:border-[#366b47] rounded-lg text-sm text-text-primary dark:text-white font-medium transition-all flex items-center justify-center gap-2 group/btn"
    : "flex-1 px-4 py-2.5 bg-background-secondary dark:bg-[#13231a] border border-border-primary dark:border-[#2b3d31] rounded-lg text-sm text-slate-500 font-medium transition-all flex items-center justify-center gap-2 opacity-70 cursor-not-allowed";
  const iconClasses = enabled
    ? `material-symbols-outlined text-[18px] ${iconHoverClass} transition-colors`
    : "material-symbols-outlined text-[18px] text-slate-500";
  const title = enabled ? label : disabledReason;
  const disabledAttr = enabled ? "" : ' disabled aria-disabled="true"';

  return `
                <button data-action="open-sync-modal" data-remote="${escapeHTML(remoteName)}" data-preset="${escapeHTML(preset)}" class="${buttonClasses}" title="${escapeHTML(title)}"${disabledAttr}>
                    <span class="${iconClasses}">${icon}</span>
                    ${escapeHTML(label)}
                </button>
  `;
};

export const renderRemotes = (container, remotes = []) => {
  if (!container) {
    return;
  }

  if (!remotes.length) {
    container.innerHTML = `
      <div class="p-8 text-center text-slate-500 border border-dashed border-[var(--bg-secondary)] rounded-xl">
        No remotes configured for this project.
      </div>`;
    return;
  }

  const cardsHtml = remotes
    .map((remote) => {
      const isProtected = Boolean(remote.protected);
      const themeColor = isProtected ? "amber" : "blue";
      const themeIcon = isProtected ? "rocket_launch" : "science";
      const borderColor = isProtected
        ? "border-amber-500/20"
        : "border-[var(--bg-secondary)]";
      const lastSyncText = String(remote.lastSync || "never")
        .trim()
        .toLowerCase();
      const lastSyncTone =
        lastSyncText === "never" ? "text-amber-600 dark:text-amber-300" : "text-text-secondary dark:text-slate-200";
      const canPullDB = canUseSyncPreset(remote, "db");
      const canPullMedia = canUseSyncPreset(remote, "media");
      const dbDisabledReason =
        "Database sync is disabled for this remote (capability: db).";
      const mediaDisabledReason =
        "Media sync is disabled for this remote (capability: media).";
      const authMethodLabel = formatAuthMethodLabel(remote.authMethod);

      return `
      <div class="glass-card rounded-xl p-0 overflow-hidden group mb-6 border ${borderColor} dark:bg-card-bg">
        <div class="p-6 border-b border-border-primary dark:border-[var(--bg-secondary)] bg-gradient-to-r from-surface-primary to-surface-primary/50 dark:from-[var(--surface-primary)] dark:to-[var(--surface-primary)]/50 relative overflow-hidden">
          ${isProtected ? `<div class="absolute top-0 right-0 w-16 h-16 bg-gradient-to-bl from-amber-500/10 to-transparent pointer-events-none"></div>` : ""}
          <div class="relative z-10 flex flex-col gap-4">
            <div class="flex items-start justify-between gap-4">
              <div class="min-w-0 flex items-center gap-4">
                <div class="h-14 w-14 shrink-0 rounded-xl bg-${themeColor}-500/10 border border-${themeColor}-500/30 text-${themeColor}-400 flex items-center justify-center shadow-[0_0_0_1px_rgba(63,122,82,0.45)]">
                  <span class="material-symbols-outlined text-[26px]">${themeIcon}</span>
                </div>
                <div class="min-w-0">
                  <div class="flex items-center flex-wrap gap-2">
                    <h3 class="text-text-primary dark:text-white text-[1.4rem] leading-none font-semibold">
                      ${escapeHTML(remote.name)}
                    </h3>
                    ${isProtected ? `<span class="px-2 py-0.5 rounded text-[10px] font-bold bg-${themeColor}-500/20 text-${themeColor}-400 border border-${themeColor}-500/30 uppercase tracking-wide">Protected</span>` : ""}
                  </div>
                  <p class="mt-1 text-[11px] uppercase tracking-[0.08em] text-primary/70">Auth: ${escapeHTML(authMethodLabel)}</p>
                </div>
              </div>
              <div class="flex items-center gap-1.5 p-1 rounded-lg border border-border-primary bg-surface-secondary/60 backdrop-blur-sm shadow-[0_0_15px_rgba(13,242,89,0.1)]">
                <button data-action="open-remote-url" data-remote="${escapeHTML(remote.name)}" class="h-8 w-8 flex items-center justify-center text-slate-500 dark:text-slate-300 hover:text-slate-900 dark:hover:text-white hover:bg-slate-200 dark:hover:bg-background-secondary rounded-md transition-all" title="Open Remote URL">
                  <span class="material-symbols-outlined text-[18px]">open_in_new</span>
                </button>
                <button data-action="remote-test" data-remote="${escapeHTML(remote.name)}" class="h-8 w-8 flex items-center justify-center text-slate-500 dark:text-slate-300 hover:text-slate-900 dark:hover:text-white hover:bg-slate-200 dark:hover:bg-background-secondary rounded-md transition-all" title="Test Connection">
                  <span class="material-symbols-outlined text-[18px]">wifi_tethering</span>
                </button>
              </div>
            </div>
            <div class="flex flex-col lg:flex-row gap-2.5">
              <div class="min-w-0 flex items-center gap-2 px-3 py-2 rounded-lg border border-border-primary bg-surface-secondary/50 lg:flex-1">
                <span class="material-symbols-outlined text-[16px] text-slate-400">dns</span>
                <span class="text-[11px] uppercase tracking-wide text-slate-500">Host</span>
                <span class="ml-auto min-w-0 truncate text-xs font-mono text-text-primary dark:text-slate-200" title="${escapeHTML(remote.host)}">${escapeHTML(remote.host)}</span>
              </div>
              <div class="flex items-center gap-2 px-3 py-2 rounded-lg border border-border-primary bg-surface-secondary/50 lg:flex-1 min-w-0 overflow-hidden">
                <span class="material-symbols-outlined text-[16px] text-slate-400">history</span>
                <span class="text-[11px] uppercase tracking-wide text-slate-500 shrink-0">Last sync</span>
                <span class="ml-auto text-xs font-mono ${lastSyncTone} truncate ml-2">${escapeHTML(remote.lastSync || "never")}</span>
              </div>
            </div>

          </div>
        </div>
        <div class="p-6">
          <div class="flex flex-col gap-3">
            <div class="flex flex-wrap gap-3">
                ${renderSyncPresetButton({
                  remoteName: remote.name,
                  preset: "full",
                  icon: "all_inclusive",
                  label: "Pull Everything",
                  iconHoverClass: "group-hover/btn:text-purple-400",
                  enabled: true,
                  disabledReason: "",
                })}
                ${renderSyncPresetButton({
                  remoteName: remote.name,
                  preset: "db",
                  icon: "database",
                  label: "Pull Database",
                  iconHoverClass: "group-hover/btn:text-primary",
                  enabled: canPullDB,
                  disabledReason: dbDisabledReason,
                })}
                ${renderSyncPresetButton({
                  remoteName: remote.name,
                  preset: "media",
                  icon: "perm_media",
                  label: "Pull Media",
                  iconHoverClass: "group-hover/btn:text-blue-400",
                  enabled: canPullMedia,
                  disabledReason: mediaDisabledReason,
                })}
            </div>
            <div class="flex flex-wrap gap-3">
                <button data-action="open-remote-shell" data-remote="${escapeHTML(remote.name)}" data-loading-label="Opening SSH..." class="flex-1 min-h-[42px] px-4 py-2.5 bg-background-secondary hover:bg-surface-primary border border-border-primary rounded-lg text-sm text-text-secondary dark:text-slate-300 hover:text-text-primary dark:hover:text-white font-medium transition-all flex items-center justify-center gap-2 group/btn">
                    <span class="material-symbols-outlined inline-flex h-[18px] w-[18px] items-center justify-center text-[18px] opacity-70 group-hover/btn:opacity-100">terminal</span>
                    <span data-role="label">Open SSH</span>
                </button>
                <button data-action="open-remote-db" data-remote="${escapeHTML(remote.name)}" data-loading-label="Opening Database..." class="flex-1 min-h-[42px] px-4 py-2.5 bg-background-secondary hover:bg-surface-primary border border-border-primary rounded-lg text-sm text-text-secondary dark:text-slate-300 hover:text-text-primary dark:hover:text-white font-medium transition-all flex items-center justify-center gap-2 group/btn">
                    <span class="material-symbols-outlined inline-flex h-[18px] w-[18px] items-center justify-center text-[18px] opacity-70 group-hover/btn:opacity-100">database</span>
                    <span data-role="label">Open Database</span>
                </button>
                <button data-action="open-remote-sftp" data-remote="${escapeHTML(remote.name)}" data-loading-label="Opening SFTP..." class="flex-1 min-h-[42px] px-4 py-2.5 bg-background-secondary hover:bg-surface-primary border border-border-primary rounded-lg text-sm text-text-secondary dark:text-slate-300 hover:text-text-primary dark:hover:text-white font-medium transition-all flex items-center justify-center gap-2 group/btn">
                    <span class="material-symbols-outlined inline-flex h-[18px] w-[18px] items-center justify-center text-[18px] opacity-70 group-hover/btn:opacity-100">folder_open</span>
                    <span data-role="label">Open SFTP</span>
                </button>
            </div>
            <!-- Inline sync config removed in favor of modal -->
          </div>
          ${remote.protected ? `<div class="mt-4 flex items-center gap-2 p-2 bg-amber-900/10 border border-amber-900/30 rounded text-amber-500/80 text-xs"><span class="material-symbols-outlined text-[16px]">info</span>Syncing from a protected remote can overwrite local data. Consider creating a snapshot before syncing.</div>` : ""}
        </div>
      </div>
    `;
    })
    .join("");

  container.innerHTML = `
    <div class="grid grid-cols-1 lg:grid-cols-5 gap-8 items-start">
      <div class="lg:col-span-3 space-y-6">
        <div class="flex items-center justify-between pb-2">
          <h3 class="text-text-primary dark:text-white text-lg font-semibold flex items-center gap-2">
            Connected Remotes
          </h3>
        </div>
        ${cardsHtml}
      </div>
      <div class="lg:col-span-2">
        <div class="sticky top-6 flex flex-col items-center justify-center bg-white dark:bg-background-primary border border-border-primary rounded-xl overflow-hidden shadow-xl py-8">
            <div class="absolute inset-0 z-0 opacity-10" style="background-image: radial-gradient(var(--primary) 1px, transparent 1px); background-size: 20px 20px;"></div>
            <div class="relative z-10 w-full max-w-[200px]">
              <div class="bg-surface-primary border border-blue-500/30 rounded-lg p-4 shadow-lg shadow-blue-500/5 relative">
                <div class="absolute -top-3 left-1/2 -translate-x-1/2 bg-blue-600 dark:bg-blue-500 px-3 py-0.5 text-[10px] text-white border border-blue-400 rounded-full uppercase font-black tracking-wider shadow-sm">Source</div>
                <div class="flex items-center justify-center gap-3">
                  <span class="material-symbols-outlined text-blue-400 text-3xl">cloud</span>
                  <div class="text-left">
                    <div class="text-slate-900 dark:text-white text-sm font-black">Remote</div>
                    <div class="text-slate-600 dark:text-slate-500 text-xs">Dev/Staging/Prod</div>
                  </div>
                </div>
              </div>
            </div>
            <div class="h-12 w-px dashed-line relative my-2">
              <div class="absolute top-0 left-1/2 -translate-x-1/2 -ml-[2px] w-1 h-3 bg-primary rounded-full animate-[bounce_2s_infinite]"></div>
            </div>
            <div class="relative z-10">
              <div class="bg-surface-secondary border border-[#366b47] rounded-full h-12 w-12 flex items-center justify-center shadow-[0_0_15px_rgba(13,242,89,0.2)]">
                <span class="material-symbols-outlined text-primary animate-pulse">lock_open</span>
              </div>
            </div>
            <div class="h-12 w-px dashed-line relative my-2">
              <div class="absolute bottom-0 left-1/2 -translate-x-1/2 -ml-[2px] w-1 h-3 bg-primary rounded-full animate-[bounce_2s_infinite_reverse]"></div>
            </div>
            <div class="relative z-10 w-full max-w-[200px]">
              <div class="bg-background-secondary border border-primary/40 rounded-lg p-4 shadow-lg shadow-primary/10 relative">
                <div class="absolute -top-3 left-1/2 -translate-x-1/2 bg-emerald-600 dark:bg-primary px-3 py-0.5 text-[10px] text-white border border-emerald-400 rounded-full uppercase font-black tracking-wider shadow-sm">Destination</div>
                <div class="flex items-center justify-center gap-3">
                  <span class="material-symbols-outlined text-primary text-3xl">laptop_mac</span>
                  <div class="text-left">
                    <div class="text-slate-900 dark:text-white text-sm font-black">Local App</div>
                    <div class="text-slate-600 dark:text-slate-500 text-xs">Your Machine</div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  `;
};

const safeRemotes = {
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

export const createRemotesController = ({
  bridge,
  refs,
  getProject,
  getSyncConfig,
  onStatus,
  onToast,
  onOpenRemoteShellFallback,
}) => {
  const MIN_REMOTE_OPEN_LOADING_MS = 1400;
  const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const updateRefs = (newRefs) => {
    refs = newRefs;
  };
  const pendingRemoteActions = new Set();

  const remoteActionKey = (project, remoteName, actionName) =>
    `${project}:${remoteName}:${actionName}`;

  const setButtonLoading = (button, isLoading) => {
    if (!(button instanceof HTMLElement)) {
      return;
    }
    const icon = button.querySelector(".material-symbols-outlined");
    if (isLoading) {
      button.disabled = true;
      button.setAttribute("aria-disabled", "true");
      button.setAttribute("aria-busy", "true");
      button.classList.add("opacity-70", "cursor-not-allowed");
      if (icon) {
        icon.dataset.previousIcon = icon.textContent;
        icon.textContent = "progress_activity";
        icon.classList.add("animate-spin");
      }
      return;
    }

    button.disabled = false;
    button.setAttribute("aria-disabled", "false");
    button.removeAttribute("aria-busy");
    button.classList.remove("opacity-70", "cursor-not-allowed");
    if (icon) {
      const previousIcon = icon.dataset.previousIcon;
      if (previousIcon) {
        icon.textContent = previousIcon;
      }
      icon.classList.remove("animate-spin");
    }
  };

  const runOpenRemoteAction = async (
    remoteName,
    actionName,
    button,
    runner,
  ) => {
    const project = String(getProject?.() || "").trim();
    if (!project || !remoteName) {
      return;
    }
    const key = remoteActionKey(project, remoteName, actionName);
    if (pendingRemoteActions.has(key)) {
      return;
    }

    pendingRemoteActions.add(key);
    const startedAt = Date.now();
    setButtonLoading(button, true);
    try {
      await runner(project);
    } finally {
      const elapsed = Date.now() - startedAt;
      const remaining = MIN_REMOTE_OPEN_LOADING_MS - elapsed;
      if (remaining > 0) {
        await wait(remaining);
      }
      setButtonLoading(button, false);
      pendingRemoteActions.delete(key);
    }
  };
  const refresh = async ({ silent = false } = {}) => {
    const project = String(getProject?.() || "").trim();
    if (!project) {
      renderRemotes(refs.remotesList, []);
      renderWarnings(refs.remotesWarnings, [
        "Select a project to load remotes.",
      ]);
      return null;
    }

    try {
      const payload = normalizeRemotesPayload(await bridge.getRemotes(project));
      renderRemotes(refs.remotesList, payload.remotes);
      renderWarnings(refs.remotesWarnings, payload.warnings);
      if (!silent) {
        onStatus(`Status: remotes loaded for ${project}`);
      }
      return payload;
    } catch (err) {
      const payload = normalizeRemotesPayload(safeRemotes);
      renderRemotes(refs.remotesList, payload.remotes);
      renderWarnings(refs.remotesWarnings, payload.warnings);
      if (!silent) {
        onStatus("Status: showing local remotes fallback");
      }
      return payload;
    }
  };

  const testRemote = async (remoteName) => {
    const project = String(getProject?.() || "").trim();
    if (!project || !remoteName) {
      return;
    }

    const btn = document.querySelector(
      `[data-action="remote-test"][data-remote="${remoteName}"]`,
    );
    const icon = btn?.querySelector(".material-symbols-outlined");

    if (icon) {
      icon.classList.add("animate-spin");
      icon.textContent = "sync";
    }
    if (btn) btn.disabled = true;

    onToast(`Checking connection to ${remoteName}...`, "info");
    onStatus(`Testing connection to ${remoteName}...`);

    try {
      const message = await bridge.testRemote(project, remoteName);
      onStatus(`Remote ${remoteName} connection successful`);
      onToast(message || `Connection to ${remoteName} successful!`, "success");
    } catch (err) {
      onStatus(`Remote ${remoteName} connection failed: ${err}`);
      onToast(`Connection to ${remoteName} failed.`, "error");
    } finally {
      if (icon) {
        icon.classList.remove("animate-spin");
        icon.textContent = "wifi_tethering";
      }
      if (btn) btn.disabled = false;
    }
  };

  const openRemoteShell = async (remoteName, button) => {
    await runOpenRemoteAction(remoteName, "ssh", button, async (project) => {
      try {
        const message = await bridge.openRemoteShell(project, remoteName);
        onStatus(message || `Opened SSH for ${remoteName}`);
        onToast(message || `Opened SSH for ${remoteName}`, "success");
      } catch (err) {
        const normalizedError = String(err || "")
          .trim()
          .toLowerCase();
        if (
          typeof onOpenRemoteShellFallback === "function" &&
          normalizedError.includes("fallback to embedded terminal")
        ) {
          onStatus(`Opening SSH for ${remoteName} in embedded terminal...`);
          await onOpenRemoteShellFallback(remoteName);
          onToast(`Opened SSH for ${remoteName}`, "success");
          return;
        }
        throw err;
      }
    }).catch((err) => {
      onStatus(`Failed to open SSH for ${remoteName}: ${err}`);
      onToast(`Failed to open SSH for ${remoteName}.`, "error");
    });
  };

  const openRemoteDB = async (remoteName, button) => {
    await runOpenRemoteAction(remoteName, "db", button, async (project) => {
      const message = await bridge.openRemoteDB(project, remoteName);
      onStatus(message || `Opening remote database for ${remoteName}`);
      onToast(message || `Opening remote database for ${remoteName}.`, "info");
    }).catch((err) => {
      onStatus(`Failed to open remote DB for ${remoteName}: ${err}`);
      onToast(`Failed to open remote DB for ${remoteName}.`, "error");
    });
  };

  const openRemoteSFTP = async (remoteName, button) => {
    await runOpenRemoteAction(remoteName, "sftp", button, async (project) => {
      const message = await bridge.openRemoteSFTP(project, remoteName);
      onStatus(message || `Opening SFTP for ${remoteName}`);
      onToast(message || `Opening SFTP for ${remoteName}.`, "info");
    }).catch((err) => {
      onStatus(`Failed to open SFTP for ${remoteName}: ${err}`);
      onToast(`Failed to open SFTP for ${remoteName}.`, "error");
    });
  };

  const runSyncPreset = async (remoteName, preset) => {
    const normalizedPreset = normalizeRemotePreset(preset);
    const project = String(getProject?.() || "").trim();
    if (!project || !remoteName || !normalizedPreset) {
      return;
    }

    const syncConfig = getSyncConfig?.() || {};
    try {
      const message = await bridge.runRemoteSyncPreset(
        project,
        remoteName,
        normalizedPreset,
        {
          sanitize: Boolean(syncConfig.sanitize),
          excludeLogs: Boolean(syncConfig.excludeLogs),
          compress: Boolean(syncConfig.compress),
        },
      );
      onStatus(`Sync plan for ${remoteName} is ready`);
      onToast(message || `Sync plan for ${remoteName} prepared.`, "success");
    } catch (err) {
      onStatus(`Failed to generate sync plan for ${remoteName}: ${err}`);
      onToast(`Failed to prepare sync for ${remoteName}.`, "error");
    }
  };

  const renderSyncOptions = (container, preset, optionsDef, currentConfig) => {
    if (!container) return;

    const html = (optionsDef || [])
      .map((opt) => {
        const isChecked =
          currentConfig[opt.key] !== undefined
            ? currentConfig[opt.key]
            : opt.defaultValue;

        return `
        <label class="flex items-center justify-between cursor-pointer group p-3 rounded-lg border border-border-primary bg-background-secondary/30 hover:bg-background-secondary/50 transition-all">
          <div class="flex-1">
            <div class="text-xs font-bold text-slate-800 dark:text-white">${escapeHTML(opt.label)}</div>
            <div class="text-[10px] text-slate-500 dark:text-slate-400">${escapeHTML(opt.description)}</div>
          </div>
          <div class="relative inline-block w-10 h-6 align-middle select-none transition duration-200 ease-in">
            <input
              type="checkbox"
              data-action="toggle-sync-config"
              data-preset="${escapeHTML(preset)}"
              data-config="${escapeHTML(opt.key)}"
              class="toggle-checkbox absolute block w-4 h-4 rounded-full bg-white border-4 border-slate-600 appearance-none cursor-pointer transition-all duration-300 top-1 left-1 checked:left-5 checked:bg-white checked:border-white/0"
              ${isChecked ? "checked" : ""}
            />
            <span class="toggle-label block overflow-hidden h-6 rounded-full bg-slate-700 cursor-pointer transition-colors duration-300 group-hover:bg-slate-600 ${isChecked ? "bg-primary" : ""}"></span>
          </div>
        </label>
      `;
      })
      .join("");

    container.innerHTML = html;
  };

  const toggleSyncConfig = (preset, key, currentState, onUpdate) => {
    const nextValue = !currentState[key];
    const nextState = { ...currentState, [key]: nextValue };
    onUpdate(nextState);
    onStatus(`Option "${key}" ${nextValue ? "enabled" : "disabled"}.`);
    return nextState;
  };

  return {
    refresh,
    testRemote,
    openRemoteShell,
    openRemoteDB,
    openRemoteSFTP,
    runSyncPreset,
    renderSyncOptions,
    toggleSyncConfig,
  };
};

export const renderSyncModal = (container) => {
  if (!container) return;
  container.innerHTML = `
      <div
        id="syncOptionsModal"
        class="hidden fixed inset-0 z-[150] bg-background-primary/60 backdrop-blur-sm flex items-center justify-center p-4 opacity-0 transition-opacity duration-300"
      >
        <div
          class="bg-surface-primary border border-border-primary rounded-xl w-full max-w-lg shadow-2xl flex flex-col overflow-hidden scale-95 transition-transform duration-300"
        >
          <div
            class="px-6 py-4 border-b border-border-primary flex justify-between items-center bg-surface-secondary/50"
          >
            <h3 class="text-white text-lg font-bold flex items-center gap-2">
              <span
                class="material-symbols-outlined text-primary"
                id="syncModalIcon"
                >sync</span
              >
              <span id="syncModalTitle">Sync Options</span>
            </h3>
            <button
              class="text-slate-500 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white transition-colors"
              data-action="close-sync-modal"
            >
              <span class="material-symbols-outlined">close</span>
            </button>
          </div>

          <!-- Step 1: Options -->
          <div id="syncModalStep1" class="p-6 space-y-4">
            <p class="text-slate-300 text-sm">
              You are about to sync data from the
              <strong id="syncModalRemoteName" class="text-white"></strong>
              environment. Configure your sync options below:
            </p>

            <div id="syncModalOptionsContainer" class="space-y-4">
              <!-- Options injected dynamically based on preset -->
            </div>

            <div
              class="px-0 pt-4 flex gap-3 justify-end items-center border-t border-border-primary"
            >
              <button
                class="px-4 py-2 rounded-lg text-sm text-slate-300 font-medium hover:bg-white/5 transition-colors"
                data-action="close-sync-modal"
              >
                Cancel
              </button>
              <button
                data-action="preview-sync-plan"
                id="previewSyncPlanBtn"
                class="px-5 py-2 bg-slate-700 hover:bg-slate-600 border border-slate-500 rounded-lg text-sm text-white font-medium transition-all group flex items-center gap-2"
              >
                <span
                  class="material-symbols-outlined text-[16px] group-hover:text-primary transition-colors"
                  >preview</span
                >
                <span>Preview Plan</span>
              </button>
            </div>
          </div>

          <!-- Step 2: Plan Preview -->
          <div id="syncModalStep2" class="hidden p-6 space-y-4">
            <div class="flex items-center gap-2 text-sm text-slate-300">
              <span class="material-symbols-outlined text-[18px] text-primary"
                >fact_check</span
              >
              Review the actions below, then confirm to proceed:
            </div>

            <!-- Plan output -->
            <div
              id="syncPlanOutput"
              class="bg-background-primary border border-border-primary/60 rounded-lg p-4 font-mono text-xs text-slate-300 max-h-64 overflow-y-auto leading-relaxed whitespace-pre-wrap"
            >
              <!-- Plan output injected here -->
            </div>

            <div
              id="syncPlanLoading"
              class="hidden flex items-center gap-3 text-sm text-slate-400 py-2"
            >
              <span
                class="inline-block w-4 h-4 rounded-full border-2 border-primary border-t-transparent animate-spin flex-shrink-0"
              ></span>
              Generating plan...
            </div>

            <div
              class="pt-4 flex gap-3 justify-between items-center border-t border-border-primary"
            >
              <button
                data-action="back-to-sync-options"
                class="px-4 py-2 rounded-lg text-sm text-slate-300 font-medium hover:bg-white/5 transition-colors flex items-center gap-1"
              >
                <span class="material-symbols-outlined text-[16px]"
                  >arrow_back</span
                >
                Back
              </button>
              <div class="flex gap-3">
                <button
                  class="px-4 py-2 rounded-lg text-sm text-slate-300 font-medium hover:bg-white/5 transition-colors"
                  data-action="close-sync-modal"
                >
                  Cancel
                </button>
                <button
                  data-action="confirm-sync"
                  id="confirmSyncBtn"
                  class="px-5 py-2 bg-primary hover:bg-primary/80 border border-primary/50 rounded-lg text-sm text-white font-bold transition-all group flex items-center gap-2"
                >
                  <span
                    class="material-symbols-outlined text-[16px] group-hover:text-white transition-colors"
                    >play_arrow</span
                  >
                  <span>Execute Sync</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
  `;
};
