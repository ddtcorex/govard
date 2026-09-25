import { formatUpdateMessage } from "./update-message.js";

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
    message: "Version check has not been run yet.",
    changelog: "",
    channel: "stable",
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
  };

  const updateRefs = (newRefs) => {
    refs = newRefs;
    renderUpdateSection();
  };
  const toggleDrawer = (open) => {
    if (!refs.settingsDrawer) {
      return;
    }
    if (open) {
      window.dispatchEvent(new CustomEvent("govard:blur", { detail: { isVisible: true } }));
      refs.settingsDrawer.classList.remove("hidden");
      refs.settingsDrawer.setAttribute("aria-hidden", "false");
      return;
    }
    window.dispatchEvent(new CustomEvent("govard:blur", { detail: { isVisible: false } }));
    refs.settingsDrawer.classList.add("hidden");
    refs.settingsDrawer.setAttribute("aria-hidden", "true");
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
    const theme = refs.themeSelect?.value || "system";
    const proxyTarget = refs.proxyTarget?.value || "";
    const preferredBrowser = refs.preferredBrowser?.value || "";
    const codeEditor = refs.codeEditor?.value || "";
    const dbClientPreference = refs.dbClientPreference?.value || "pma";
    const runInBackground = refs.runInBackgroundToggle
      ? Boolean(refs.runInBackgroundToggle.checked)
      : true;
    try {
      const message = await bridge.updateSettings({
        theme,
        proxyTarget,
        preferredBrowser,
        codeEditor,
        dbClientPreference,
        runInBackground,
      });
      applyTheme(theme);
      onStatus("Settings saved successfully.");
      onToast("Settings saved successfully.", "success");
    } catch (err) {
      const message = "Could not save settings.";
      onStatus(message);
      onToast(message, "error");
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
      updateState.outdated = result.outdated;
      updateState.installCompleted = false;
      updateState.failed = false;
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
      };
    } catch (_err) {
      updateState.checked = true;
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
    checkForUpdates,
    installLatestUpdate,
    setUpdateChannel,
    updateRefs,
  };
};

