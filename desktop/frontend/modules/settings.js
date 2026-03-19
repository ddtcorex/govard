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
    "border-primary/35 bg-emerald-50 dark:bg-primary/15 text-primary";
  const UPDATE_BADGE_INSTALLED_CLASS =
    "border-primary/45 bg-emerald-100 dark:bg-primary/20 text-primary";
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
      refs.settingsDrawer.classList.remove("hidden");
      refs.settingsDrawer.setAttribute("aria-hidden", "false");
      return;
    }
    refs.settingsDrawer.classList.add("hidden");
    refs.settingsDrawer.setAttribute("aria-hidden", "true");
  };

  const load = async () => {
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
      renderUpdateSection();
    } catch (_err) {
      applyTheme();
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
        message: updateState.message,
      };
    } catch (_err) {
      updateState.checked = true;
      updateState.outdated = false;
      updateState.failed = true;
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

  return {
    toggleDrawer,
    load,
    save,
    reset,
    checkForUpdates,
    installLatestUpdate,
    updateRefs,
  };
};

export const renderSettingsDrawer = (container) => {
  if (!container) return;
  container.innerHTML = `
        <div
          class="drawer hidden fixed inset-0 z-[100] bg-slate-900/40 dark:bg-background-primary/80 backdrop-blur-md transition-all duration-500"
          id="settingsDrawer"
          aria-hidden="true"
        >
          <div
            class="bg-white dark:bg-[#112217] border-l border-slate-200 dark:border-white/5 h-full ml-auto w-[460px] shadow-[0_0_80px_rgba(0,0,0,0.6)] flex flex-col relative overflow-hidden"
          >
          <!-- Premium Background Accents -->
          <div class="absolute -top-40 -right-40 w-96 h-96 bg-primary/10 blur-[120px] pointer-events-none rounded-full"></div>
          <div class="absolute bottom-20 -left-20 w-64 h-64 bg-primary/5 blur-[100px] pointer-events-none rounded-full"></div>
          
          <header
            class="flex items-center justify-between p-8 border-b border-slate-200 dark:border-white/5 relative z-10 bg-slate-50/80 dark:bg-black/10 backdrop-blur-sm"
          >
            <div class="flex items-center gap-4">
              <div class="size-12 rounded-2xl bg-primary/10 border border-primary/20 flex items-center justify-center text-primary shadow-lg shadow-primary/5">
                <span class="material-symbols-outlined text-[28px]">settings</span>
              </div>
              <div>
                <h3 class="text-slate-900 dark:text-white text-xl font-bold tracking-tight">Application Settings</h3>
                <p class="text-slate-500 dark:text-slate-500 text-xs mt-0.5 font-medium">Customize your workspace environment</p>
              </div>
            </div>
            <button
              class="group p-2.5 rounded-xl hover:bg-slate-200 dark:hover:bg-white/10 text-slate-500 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white transition-all flex items-center justify-center h-10 w-10 border border-slate-200 dark:border-white/5 hover:border-slate-300 dark:hover:border-white/20 active:scale-90"
              id="closeSettings"
              type="button"
            >
              <span class="material-symbols-outlined group-hover:rotate-90 transition-transform">close</span>
            </button>
          </header>

          <div class="flex-1 overflow-y-auto custom-scrollbar p-8 flex flex-col gap-10 relative z-10">
            <!-- Section: Workspace -->
            <section class="flex flex-col gap-6">
              <div class="flex items-center gap-2.5 px-1">
                 <div class="w-1.5 h-4 bg-primary rounded-full"></div>
                 <p class="text-[11px] font-bold uppercase tracking-[0.15em] text-slate-400">Environment & Workspace</p>
              </div>
              
              <div class="grid grid-cols-1 gap-5">
                <div class="flex flex-col gap-2 group">
                  <label for="themeSelect" class="text-[11px] font-bold text-slate-500 uppercase tracking-wider ml-1 group-focus-within:text-primary transition-colors">Interface Theme</label>
                  <div class="relative">
                    <select
                      id="themeSelect"
                      class="w-full bg-slate-50 dark:bg-[#162a1d] border border-slate-200 dark:border-white/10 rounded-xl px-4 py-3.5 text-slate-900 dark:text-white outline-none focus:border-primary/50 focus:ring-4 focus:ring-primary/10 appearance-none cursor-pointer transition-all hover:bg-slate-100 dark:hover:bg-surface-primary font-medium"
                    >
                      <option value="system">System Default</option>
                      <option value="light">Light Mode</option>
                      <option value="dark">Dark Mode</option>
                    </select>
                    <span class="material-symbols-outlined absolute right-4 top-1/2 -translate-y-1/2 text-slate-500 pointer-events-none text-xl">unfold_more</span>
                  </div>
                </div>
                <div class="flex flex-col gap-2 group">
                  <label for="proxyTarget" class="text-[11px] font-bold text-slate-500 uppercase tracking-wider ml-1 group-focus-within:text-primary transition-colors">Proxy Gateway URL</label>
                  <div class="relative">
                    <input
                      id="proxyTarget"
                      type="text"
                      placeholder="govard.test"
                      class="w-full bg-slate-50 dark:bg-[#162a1d] border border-slate-200 dark:border-white/10 rounded-xl px-4 py-3.5 text-slate-900 dark:text-white outline-none focus:border-primary/50 focus:ring-4 focus:ring-primary/10 transition-all placeholder:text-slate-400 dark:placeholder:text-slate-600 hover:bg-slate-100 dark:hover:bg-surface-primary font-medium"
                     />
                     <span class="material-symbols-outlined absolute right-4 top-1/2 -translate-y-1/2 text-slate-400 dark:text-slate-600 pointer-events-none text-xl">dns</span>
                  </div>
                </div>

                <div class="flex flex-col gap-2 group">
                  <label for="codeEditor" class="text-[11px] font-bold text-slate-500 uppercase tracking-wider ml-1 group-focus-within:text-primary transition-colors">Favorite IDE Command</label>
                  <div class="relative">
                    <input
                      id="codeEditor"
                      type="text"
                      placeholder="code"
                      class="w-full bg-slate-50 dark:bg-[#162a1d] border border-slate-200 dark:border-white/10 rounded-xl px-4 py-3.5 text-slate-900 dark:text-white outline-none focus:border-primary/50 focus:ring-4 focus:ring-primary/10 transition-all placeholder:text-slate-400 dark:placeholder:text-slate-600 hover:bg-slate-100 dark:hover:bg-surface-primary font-medium"
                     />
                     <span class="material-symbols-outlined absolute right-4 top-1/2 -translate-y-1/2 text-slate-400 dark:text-slate-600 pointer-events-none text-xl">code</span>
                  </div>
                </div>

                <div class="flex flex-col gap-2 group">
                  <label for="preferredBrowser" class="text-[11px] font-bold text-slate-500 uppercase tracking-wider ml-1 group-focus-within:text-primary transition-colors">Primary Web Browser</label>
                  <div class="relative">
                    <input
                      id="preferredBrowser"
                      type="text"
                      placeholder="firefox"
                      class="w-full bg-slate-50 dark:bg-[#162a1d] border border-slate-200 dark:border-white/10 rounded-xl px-4 py-3.5 text-slate-900 dark:text-white outline-none focus:border-primary/50 focus:ring-4 focus:ring-primary/10 transition-all placeholder:text-slate-400 dark:placeholder:text-slate-600 hover:bg-slate-100 dark:hover:bg-surface-primary font-medium"
                     />
                     <span class="material-symbols-outlined absolute right-4 top-1/2 -translate-y-1/2 text-slate-400 dark:text-slate-600 pointer-events-none text-xl">open_in_new</span>
                  </div>
                </div>

                <div class="flex flex-col gap-2 group">
                  <label for="dbClientPreference" class="text-[11px] font-bold text-slate-500 uppercase tracking-wider ml-1 group-focus-within:text-primary transition-colors">Database Access Method</label>
                  <div class="relative">
                    <select
                      id="dbClientPreference"
                      class="w-full bg-slate-50 dark:bg-[#162a1d] border border-slate-200 dark:border-white/10 rounded-xl px-4 py-3.5 text-slate-900 dark:text-white outline-none focus:border-primary/50 focus:ring-4 focus:ring-primary/10 appearance-none cursor-pointer transition-all hover:bg-slate-100 dark:hover:bg-surface-primary font-medium"
                    >
                      <option value="pma">Built-in PHPMyAdmin</option>
                      <option value="desktop">Local App (TablePlus/BeeKeeper)</option>
                    </select>
                    <span class="material-symbols-outlined absolute right-4 top-1/2 -translate-y-1/2 text-slate-500 pointer-events-none text-xl">database</span>
                  </div>
                </div>
              </div>
            </section>

            <!-- Section: System Behavior -->
            <section class="flex flex-col gap-6">
              <div class="flex items-center gap-2.5 px-1">
                 <div class="w-1.5 h-4 bg-primary rounded-full"></div>
                 <p class="text-[11px] font-bold uppercase tracking-[0.15em] text-slate-400">System Behavior</p>
              </div>
              
              <div class="bg-slate-100/70 dark:bg-[#162a1d]/40 border border-slate-200 dark:border-white/5 rounded-2xl p-6 flex items-center justify-between hover:bg-slate-200/60 dark:hover:bg-[#162a1d]/60 transition-all cursor-default group hover:border-primary/10">
                <div class="flex items-center gap-4">
                  <div class="size-10 rounded-xl bg-primary/5 flex items-center justify-center text-primary/60 group-hover:text-primary group-hover:bg-primary/10 transition-all">
                    <span class="material-symbols-outlined text-[20px]">background_replace</span>
                  </div>
                  <div class="flex flex-col gap-0.5">
                     <span class="text-sm font-bold text-slate-800 dark:text-slate-200">Run in background</span>
                    <span class="text-[11px] text-slate-500 font-medium leading-relaxed max-w-[200px]">Keep services active in tray when closing window</span>
                  </div>
                </div>
                <label class="relative inline-flex items-center cursor-pointer group/toggle">
                  <input
                    id="runInBackgroundToggle"
                    type="checkbox"
                    class="sr-only peer"
                  />
                  <div class="w-11 h-6 bg-slate-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full rtl:peer-checked:after:-translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:start-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary"></div>
                </label>
              </div>
            </section>

            <!-- Section: Maintenance -->
            <section class="flex flex-col gap-6">
              <div class="flex items-center gap-2.5 px-1">
                 <div class="w-1.5 h-4 bg-primary rounded-full"></div>
                 <p class="text-[11px] font-bold uppercase tracking-[0.15em] text-slate-400">Maintenance & Updates</p>
              </div>

              <div class="relative overflow-hidden rounded-2xl border border-primary/20 bg-primary/5 p-6 pb-5">
                <div class="pointer-events-none absolute -right-8 -top-8 h-32 w-32 rounded-full bg-primary/10 blur-3xl opacity-50"></div>
                <div class="relative z-10">
                  <div class="flex items-center justify-between mb-5">
                    <div class="flex items-center gap-3">
                      <div class="w-10 h-10 rounded-xl bg-primary/10 border border-primary/20 flex items-center justify-center text-primary">
                        <span class="material-symbols-outlined text-[22px]">auto_awesome</span>
                      </div>
                      <div>
                        <h4 class="text-sm font-bold text-slate-900 dark:text-white tracking-tight">Software Updates</h4>
                        <p class="text-[11px] text-slate-500 font-medium mt-0.5">Automated version control</p>
                      </div>
                    </div>
                    <span
                      id="settingsUpdateBadge"
                      class="px-2.5 py-1 rounded-full border border-slate-200 dark:border-white/5 bg-slate-100 dark:bg-black/40 text-[10px] font-black uppercase tracking-wider text-primary shadow-sm"
                    >
                      Idle
                    </span>
                  </div>
                  
                  <div class="bg-slate-100 dark:bg-black/30 rounded-xl p-4 mb-4 border border-slate-200 dark:border-white/5">
                    <p
                      class="text-[12px] text-slate-600 dark:text-slate-300 leading-relaxed flex items-center gap-3 update-message-text"
                      id="settingsUpdateStatus"
                      aria-live="polite"
                    >
                      <span class="material-symbols-outlined text-[16px] text-primary/60">info</span>
                      <span class="font-medium">Version check has not been run yet.</span>
                    </p>
                  </div>

                  <div class="flex flex-col gap-2.5">
                    <button
                      class="flex w-full items-center justify-center gap-2.5 rounded-xl border border-slate-200 dark:border-white/10 bg-slate-50 dark:bg-white/5 px-4 py-3 text-xs font-bold text-slate-600 dark:text-slate-300 transition-all hover:bg-slate-100 dark:hover:bg-white/10 hover:text-slate-900 dark:hover:text-white active:scale-[0.98] disabled:opacity-50"
                      data-action="check-updates"
                      id="checkUpdatesButton"
                      type="button"
                    >
                      <span class="material-symbols-outlined text-[18px]">sync</span>
                      <span>Scan for updates</span>
                    </button>
                    <button
                      class="hidden w-full items-center justify-center gap-2.5 rounded-xl bg-primary px-4 py-3 text-[13px] font-black text-background-dark shadow-[0_12px_24px_rgba(13,242,89,0.2)] transition-all hover:scale-[1.02] active:scale-[0.98] disabled:opacity-50"
                      data-action="install-update"
                      id="installUpdateButton"
                      type="button"
                    >
                      <span class="material-symbols-outlined text-[20px]">download_for_offline</span>
                      <span>Update Govard Now</span>
                    </button>
                  </div>
                </div>
              </div>
            </section>
          </div>

          <footer class="p-8 bg-slate-50/90 dark:bg-black/40 border-t border-slate-200 dark:border-white/5 backdrop-blur-xl relative z-20 shadow-[0_-4px_20px_rgba(0,0,0,0.06)] dark:shadow-[0_-10px_40px_rgba(0,0,0,0.3)]">
            <div class="grid grid-cols-2 gap-4">
              <button
                class="flex items-center justify-center gap-2.5 px-4 py-3.5 bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/10 rounded-xl text-[13px] font-bold text-slate-600 dark:text-slate-400 hover:bg-slate-200 dark:hover:bg-white/10 hover:text-slate-900 dark:hover:text-white hover:border-slate-300 dark:hover:border-white/20 transition-all active:scale-[0.98] group"
                data-action="reset-settings"
                type="button"
              >
                <span class="material-symbols-outlined text-[18px] group-hover:rotate-180 transition-transform duration-500">restart_alt</span>
                <span>Reset Settings</span>
              </button>
              <button
                class="flex items-center justify-center gap-2.5 px-4 py-3.5 bg-red-500/5 border border-red-500/20 rounded-xl text-[13px] font-bold text-red-400 hover:bg-red-500/15 hover:text-red-300 hover:border-red-500/40 transition-all active:scale-[0.98] group"
                data-action="quit-app"
                type="button"
              >
                <span class="material-symbols-outlined text-[18px] group-hover:scale-110 transition-transform">power_settings_new</span>
                <span>Quit Govard</span>
              </button>
            </div>
          </footer>
        </div>
      </div>

  `;
};
