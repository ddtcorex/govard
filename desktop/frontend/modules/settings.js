import { formatUpdateMessage } from "./update-message.js";
import {
  focusFirstElement,
  rememberFocus,
} from "../utils/focus-trap.js";

export const normalizeSettingsPayload = (settings = {}) => ({
  theme: settings.theme || settings.Theme || "system",
  proxyTarget: settings.proxyTarget || settings.ProxyTarget || "",
  preferredBrowser:
    settings.preferredBrowser || settings.PreferredBrowser || "",
  codeEditor: settings.codeEditor || settings.CodeEditor || "",
  dbClientPreference:
    settings.dbClientPreference || settings.DBClientPreference || "pma",
  runInBackground:
    settings.runInBackground === undefined
      ? Boolean(settings.RunInBackground ?? true)
      : Boolean(settings.runInBackground),
});

/**
 * Validate the proxy gateway URL field. Returns "" when the value is usable
 * and a human-readable field error otherwise. Empty means unset (the backend
 * default applies), so it validates clean.
 */
export const validateProxyTarget = (value) => {
  const trimmed = String(value ?? "").trim();
  if (trimmed === "") {
    return "";
  }
  if (/\s/.test(trimmed)) {
    return "Enter a valid hostname or URL, e.g. govard.test.";
  }
  const candidate = /^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(trimmed)
    ? trimmed
    : `http://${trimmed}`;
  let parsed;
  try {
    parsed = new URL(candidate);
  } catch (_parseErr) {
    return "Enter a valid hostname or URL, e.g. govard.test.";
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    return "Proxy URL must start with http:// or https://, e.g. https://govard.test.";
  }
  const host = parsed.hostname;
  if (!/^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$/.test(host)) {
    return "Enter a valid hostname or URL, e.g. govard.test.";
  }
  if (host !== "localhost" && !host.includes(".")) {
    return "Enter a valid hostname or URL, e.g. govard.test.";
  }
  return "";
};

export const applyTheme = (theme) => {
  const root = document.documentElement;
  if (!root) {
    return;
  }
  const setDarkClass = (darkEnabled) => {
    if (darkEnabled) {
      root.classList.add("dark");
    } else {
      root.classList.remove("dark");
    }
  };
  if (theme === "system") {
    const prefersDark =
      window.matchMedia &&
      window.matchMedia("(prefers-color-scheme: dark)").matches;
    setDarkClass(prefersDark);
    return;
  }
  setDarkClass(theme === "dark");
};

export const createSettingsController = ({
  bridge,
  refs,
  onStatus,
  onToast,
}) => {
  const UPDATE_BADGE_BASE_CLASS =
    "inline-flex items-center rounded-full border px-2.5 py-1 text-[10px] font-semibold uppercase tracking-[0.08em]";
  const UPDATE_BADGE_IDLE_CLASS =
    "border-slate-200 dark:border-border-primary bg-slate-50 dark:bg-[#173325]/70 text-slate-500 dark:text-primary";
  const UPDATE_BADGE_WORKING_CLASS =
    "border-blue-500/35 bg-blue-50 dark:bg-blue-500/15 text-blue-600 dark:text-blue-200";
  const UPDATE_BADGE_AVAILABLE_CLASS =
    "border-amber-500/35 bg-amber-50 dark:bg-amber-500/15 text-amber-600 dark:text-amber-200";
  const UPDATE_BADGE_CURRENT_CLASS =
    "border-emerald-500/30 bg-emerald-50 dark:bg-[#1a3a29] text-emerald-700 dark:text-[#0df259]";
  const UPDATE_BADGE_INSTALLED_CLASS =
    "border-emerald-500/40 bg-emerald-100 dark:bg-[#1e4631] text-emerald-700 dark:text-[#0df259]";
  const UPDATE_BADGE_ERROR_CLASS =
    "border-red-500/35 bg-red-50 dark:bg-red-500/15 text-red-600 dark:text-red-200";

  const updateState = {
    checked: false,
    checking: false,
    installing: false,
    outdated: false,
    installCompleted: false,
    failed: false,
    restartFailed: false,
    message: "Version check has not been run yet.",
    changelog: "",
    channel: "stable",
    lastChecked: "",
  };

  const normalizeUpdateResult = (payload = {}) => ({
    currentVersion: String(
      payload.currentVersion || payload.CurrentVersion || "",
    ).trim(),
    latestVersion: String(
      payload.latestVersion || payload.LatestVersion || "",
    ).trim(),
    outdated:
      payload.outdated === undefined
        ? Boolean(payload.Outdated)
        : Boolean(payload.outdated),
    message: String(payload.message || payload.Message || "").trim(),
    changelog: String(payload.changelog || payload.Changelog || "").trim(),
  });

  const normalizeErrorMessage = (err, fallback) => {
    const raw =
      typeof err?.message === "string"
        ? err.message
        : typeof err === "string"
          ? err
          : "";
    const message = raw.trim();
    return message ? message : fallback;
  };

  const normalizeCheckOptions = (options = {}) => ({
    silent:
      options && typeof options === "object" ? Boolean(options.silent) : false,
  });

  const formatLastChecked = (iso) => {
    const trimmed = String(iso || "").trim();
    if (!trimmed) {
      return "Last checked: never";
    }
    const parsed = new Date(trimmed);
    if (Number.isNaN(parsed.getTime())) {
      return "Last checked: never";
    }
    return `Last checked: ${parsed.toLocaleString()}`;
  };

  const renderUpdateSection = () => {
    if (refs.settingsUpdateStatus) {
      refs.settingsUpdateStatus.textContent = updateState.message;
    }

    if (refs.settingsUpdateChangelog) {
      const hasChangelog = String(updateState.changelog || "").trim() !== "";
      const shouldShowChangelog = updateState.outdated && hasChangelog;
      refs.settingsUpdateChangelog.classList.toggle("hidden", !shouldShowChangelog);
      if (shouldShowChangelog) {
        refs.settingsUpdateChangelog.textContent = updateState.changelog;
      }
    }

    if (refs.settingsUpdateLastChecked) {
      refs.settingsUpdateLastChecked.textContent = formatLastChecked(
        updateState.lastChecked,
      );
    }

    if (refs.settingsUpdateBadge) {
      let badgeText = "Idle";
      let badgeToneClass = UPDATE_BADGE_IDLE_CLASS;

      if (updateState.failed) {
        badgeText = "Failed";
        badgeToneClass = UPDATE_BADGE_ERROR_CLASS;
      } else if (updateState.checking || updateState.installing) {
        badgeText = "Working";
        badgeToneClass = UPDATE_BADGE_WORKING_CLASS;
      } else if (updateState.installCompleted) {
        badgeText = "Installed";
        badgeToneClass = UPDATE_BADGE_INSTALLED_CLASS;
      } else if (updateState.outdated && updateState.checked) {
        badgeText = "Available";
        badgeToneClass = UPDATE_BADGE_AVAILABLE_CLASS;
      } else if (updateState.checked) {
        badgeText = "Current";
        badgeToneClass = UPDATE_BADGE_CURRENT_CLASS;
      }

      refs.settingsUpdateBadge.textContent = badgeText;
      refs.settingsUpdateBadge.className = `${UPDATE_BADGE_BASE_CLASS} ${badgeToneClass}`;
    }

    if (refs.checkUpdatesButton) {
      refs.checkUpdatesButton.disabled =
        updateState.checking || updateState.installing;
      refs.checkUpdatesButton.innerHTML = updateState.checking
        ? '<span class="material-symbols-outlined text-[18px]">progress_activity</span><span>Checking...</span>'
        : '<span class="material-symbols-outlined text-[18px]">sync</span><span>Check for updates</span>';
    }

    if (refs.installUpdateButton) {
      const shouldShow =
        updateState.outdated &&
        updateState.checked &&
        !updateState.installCompleted;
      refs.installUpdateButton.classList.toggle("hidden", !shouldShow);
      refs.installUpdateButton.disabled =
        updateState.checking || updateState.installing;
      refs.installUpdateButton.innerHTML = updateState.installing
        ? '<span class="material-symbols-outlined text-[18px]">install_desktop</span><span>Installing...</span>'
        : '<span class="material-symbols-outlined text-[18px]">download</span><span>Download & Install Update</span>';
    }

    if (refs.retryRestartButton) {
      refs.retryRestartButton.classList.toggle(
        "hidden",
        !updateState.restartFailed,
      );
    }
  };

  const updateRefs = (newRefs) => {
    refs = newRefs;
    renderUpdateSection();
    refreshDirtyState();
  };
  // Baseline dialog focus contract: opening remembers who was focused and
  // moves focus into the drawer; closing hands focus back.
  let restoreDrawerFocus = null;
  const toggleDrawer = (open) => {
    if (!refs.settingsDrawer) {
      return;
    }
    if (open) {
      window.dispatchEvent(new CustomEvent("govard:blur", { detail: { isVisible: true } }));
      refs.settingsDrawer.classList.remove("hidden");
      refs.settingsDrawer.setAttribute("aria-hidden", "false");
      restoreDrawerFocus = rememberFocus();
      focusFirstElement(refs.settingsDrawer);
      return;
    }
    window.dispatchEvent(new CustomEvent("govard:blur", { detail: { isVisible: false } }));
    refs.settingsDrawer.classList.add("hidden");
    refs.settingsDrawer.setAttribute("aria-hidden", "true");
    if (restoreDrawerFocus) {
      restoreDrawerFocus();
      restoreDrawerFocus = null;
    }
  };

  // Explicit save discipline (Task 8): typing in a field must never fire an
  // RPC. The controller keeps the last saved snapshot, compares the live form
  // against it, and drives the drawer's dirty dot plus Save/Discard buttons.
  // save() is the only path that calls bridge.updateSettings, and it refuses
  // to call it while the proxy URL fails validation.
  const SETTINGS_FORM_KEYS = [
    "theme",
    "proxyTarget",
    "preferredBrowser",
    "codeEditor",
    "dbClientPreference",
    "runInBackground",
  ];

  const readForm = () => ({
    theme: refs.themeSelect?.value || "system",
    proxyTarget: refs.proxyTarget?.value || "",
    preferredBrowser: refs.preferredBrowser?.value || "",
    codeEditor: refs.codeEditor?.value || "",
    dbClientPreference: refs.dbClientPreference?.value || "pma",
    runInBackground: refs.runInBackgroundToggle
      ? Boolean(refs.runInBackgroundToggle.checked)
      : true,
  });

  let savedSnapshot = readForm();

  const setHidden = (el, hidden) => {
    if (!el || !el.classList || typeof el.classList.toggle !== "function") {
      return;
    }
    el.classList.toggle("hidden", hidden);
  };

  const renderProxyError = (message) => {
    if (refs.proxyTargetError) {
      refs.proxyTargetError.textContent = message || "";
    }
    setHidden(refs.proxyTargetError, !message);
    const input = refs.proxyTarget;
    if (
      input &&
      typeof input.setAttribute === "function" &&
      typeof input.removeAttribute === "function"
    ) {
      if (message) {
        input.setAttribute("aria-invalid", "true");
      } else {
        input.removeAttribute("aria-invalid");
      }
    }
  };

  const isDirty = () => {
    const form = readForm();
    return SETTINGS_FORM_KEYS.some((key) => form[key] !== savedSnapshot[key]);
  };

  const refreshDirtyState = () => {
    const dirty = isDirty();
    setHidden(refs.settingsDirtyDot, !dirty);
    if (refs.saveSettingsButton && "disabled" in refs.saveSettingsButton) {
      refs.saveSettingsButton.disabled = !dirty;
    }
    if (refs.discardSettingsButton && "disabled" in refs.discardSettingsButton) {
      refs.discardSettingsButton.disabled = !dirty;
    }
    return dirty;
  };

  const handleFieldChange = () => {
    if (!validateProxyTarget(readForm().proxyTarget)) {
      renderProxyError("");
    }
    return refreshDirtyState();
  };

  const discard = () => {
    if (refs.themeSelect) refs.themeSelect.value = savedSnapshot.theme;
    if (refs.proxyTarget) refs.proxyTarget.value = savedSnapshot.proxyTarget;
    if (refs.preferredBrowser)
      refs.preferredBrowser.value = savedSnapshot.preferredBrowser;
    if (refs.codeEditor) refs.codeEditor.value = savedSnapshot.codeEditor;
    if (refs.dbClientPreference)
      refs.dbClientPreference.value = savedSnapshot.dbClientPreference;
    if (refs.runInBackgroundToggle) {
      refs.runInBackgroundToggle.checked = savedSnapshot.runInBackground;
    }
    renderProxyError("");
    refreshDirtyState();
  };

  const load = async () => {
    try {
      const channel = await bridge.getUpdateChannel();
      updateState.channel = channel || "stable";
    } catch (_channelErr) {
      updateState.channel = "stable";
    }
    if (refs.updateChannelSelect) {
      refs.updateChannelSelect.value = updateState.channel;
    }

    try {
      const raw = await bridge.getSettings();
      const settings = normalizeSettingsPayload(raw);
      if (refs.themeSelect) refs.themeSelect.value = settings.theme;
      if (refs.proxyTarget) refs.proxyTarget.value = settings.proxyTarget;
      if (refs.preferredBrowser)
        refs.preferredBrowser.value = settings.preferredBrowser;
      if (refs.codeEditor) refs.codeEditor.value = settings.codeEditor;
      if (refs.dbClientPreference)
        refs.dbClientPreference.value = settings.dbClientPreference;
      if (refs.runInBackgroundToggle) {
        refs.runInBackgroundToggle.checked = settings.runInBackground;
      }
      applyTheme(settings.theme);
      savedSnapshot = { ...settings };
      renderProxyError("");
      refreshDirtyState();

      if (refs.trayUnavailableHint) {
        try {
          const trayStatus = await bridge.getTrayStatus();
          refs.trayUnavailableHint.classList.toggle(
            "hidden",
            trayStatus?.available !== false,
          );
        } catch (_trayErr) {
          refs.trayUnavailableHint.classList.add("hidden");
        }
      }

      renderUpdateSection();
    } catch (_err) {
      applyTheme();
      if (refs.trayUnavailableHint) {
        refs.trayUnavailableHint.classList.add("hidden");
      }
      renderUpdateSection();
    }
  };

  const save = async () => {
    const form = readForm();
    const proxyError = validateProxyTarget(form.proxyTarget);
    if (proxyError) {
      renderProxyError(proxyError);
      refreshDirtyState();
      onStatus("Fix the highlighted settings field before saving.");
      onToast(proxyError, "error");
      return { ok: false, error: proxyError };
    }
    renderProxyError("");
    try {
      const message = await bridge.updateSettings({ ...form });
      savedSnapshot = { ...form };
      applyTheme(form.theme);
      refreshDirtyState();
      onStatus("Settings saved successfully.");
      onToast("Settings saved successfully.", "success");
      return { ok: true, message };
    } catch (err) {
      const message = "Could not save settings.";
      onStatus(message);
      onToast(message, "error");
      return { ok: false, error: message };
    }
  };

  const reset = async () => {
    try {
      const message = await bridge.resetSettings();
      onStatus("Settings reset to defaults.");
      onToast("Settings reset to defaults.", "success");
      await load();
    } catch (err) {
      const message = "Could not reset settings.";
      onStatus(message);
      onToast(message, "error");
    }
  };

  const checkForUpdates = async (options = {}) => {
    if (updateState.checking || updateState.installing) {
      return { skipped: true, reason: "busy" };
    }

    const { silent } = normalizeCheckOptions(options);

    updateState.checking = true;
    updateState.message = "Checking for latest version...";
    renderUpdateSection();

    try {
      const raw = await bridge.checkForUpdates();
      const result = normalizeUpdateResult(raw);

      updateState.checked = true;
      updateState.lastChecked = new Date().toISOString();
      updateState.outdated = result.outdated;
      updateState.installCompleted = false;
      updateState.failed = false;
      updateState.restartFailed = false;
      updateState.changelog = result.changelog;

      if (result.outdated) {
        updateState.message = formatUpdateMessage(result, {
          includeVersionTransition: true,
        });
      } else if (result.message) {
        updateState.message = result.message;
      } else {
        updateState.message = `Govard Desktop is up to date (${result.currentVersion}).`;
      }

      if (!silent) {
        if (updateState.outdated) {
          onStatus("Update available.");
          onToast("A newer Govard version is available.", "info");
        } else {
          onStatus("Govard Desktop is up to date.");
          onToast("Govard Desktop is already up to date.", "success");
        }
      }
      return {
        skipped: false,
        failed: false,
        outdated: result.outdated,
        currentVersion: result.currentVersion,
        latestVersion: result.latestVersion,
        changelog: result.changelog,
        message: updateState.message,
        lastChecked: updateState.lastChecked,
      };
    } catch (_err) {
      updateState.checked = true;
      updateState.lastChecked = new Date().toISOString();
      updateState.outdated = false;
      updateState.failed = true;
      updateState.changelog = "";
      updateState.message = normalizeErrorMessage(
        _err,
        "Could not check for updates.",
      );
      if (!silent) {
        onStatus(updateState.message);
        onToast(updateState.message, "error");
      }
      return {
        skipped: false,
        failed: true,
        outdated: false,
        currentVersion: "",
        latestVersion: "",
        message: updateState.message,
        lastChecked: updateState.lastChecked,
      };
    } finally {
      updateState.checking = false;
      renderUpdateSection();
    }
  };

  const installLatestUpdate = async () => {
    if (updateState.checking || updateState.installing) {
      return { ok: false, skipped: true, reason: "busy" };
    }

    updateState.installing = true;
    updateState.message = "Downloading and installing update...";
    updateState.restartFailed = false;
    renderUpdateSection();
    let updateInstalled = false;
    let restartFailed = false;

    try {
      await bridge.installLatestUpdate();
      updateState.outdated = false;
      updateState.installCompleted = true;
      updateState.failed = false;
      updateInstalled = true;
      updateState.message =
        "Update installed. Restart Govard Desktop to run the new version.";
      onStatus("Update installed. Restarting Govard Desktop...");
      onToast("Update installed. Restarting Govard Desktop...", "success");

      try {
        await bridge.restartDesktopApp();
      } catch (restartErr) {
        restartFailed = true;
        updateState.restartFailed = true;
        updateState.failed = true;
        updateState.message = normalizeErrorMessage(
          restartErr,
          "Update installed but automatic restart failed. Please restart manually.",
        );
        onStatus(updateState.message);
        onToast(updateState.message, "warning");
      }
    } catch (_err) {
      updateState.failed = true;
      updateState.message = normalizeErrorMessage(
        _err,
        "Automatic update failed.",
      );
      onStatus(updateState.message);
      onToast(updateState.message, "error");
      return { ok: false, skipped: false, message: updateState.message };
    } finally {
      updateState.installing = false;
      renderUpdateSection();
    }

    return {
      ok: updateInstalled,
      skipped: false,
      restartFailed,
      message: updateState.message,
    };
  };

  const retryRestart = async () => {
    try {
      await bridge.restartDesktopApp();
      updateState.restartFailed = false;
      updateState.failed = false;
      updateState.message = "Restarting Govard Desktop...";
      onStatus(updateState.message);
      renderUpdateSection();
      return { ok: true, message: updateState.message };
    } catch (err) {
      updateState.restartFailed = true;
      updateState.failed = true;
      updateState.message = normalizeErrorMessage(
        err,
        "Restart failed. Use Retry Restart to try again.",
      );
      onStatus(updateState.message);
      onToast(updateState.message, "warning");
      renderUpdateSection();
      return { ok: false, message: updateState.message };
    }
  };

  const setUpdateChannel = async (channel) => {
    try {
      const applied = await bridge.setUpdateChannel(channel);
      updateState.channel = applied || channel;
      const message = `Update channel set to ${updateState.channel}.`;
      onStatus(message);
      onToast(message, "success");
      return { ok: true, channel: updateState.channel };
    } catch (_err) {
      const message = normalizeErrorMessage(
        _err,
        "Could not set update channel.",
      );
      if (refs.updateChannelSelect) {
        refs.updateChannelSelect.value = updateState.channel;
      }
      onStatus(message);
      onToast(message, "error");
      return { ok: false, channel: updateState.channel, message };
    }
  };

  return {
    toggleDrawer,
    load,
    save,
    reset,
    discard,
    isDirty,
    handleFieldChange,
    checkForUpdates,
    installLatestUpdate,
    retryRestart,
    setUpdateChannel,
    updateRefs,
  };
};

