export const normalizeSettingsPayload = (settings = {}) => ({
  theme: settings.theme || settings.Theme || "system",
  proxyTarget: settings.proxyTarget || settings.ProxyTarget || "",
  preferredBrowser:
    settings.preferredBrowser || settings.PreferredBrowser || "",
  codeEditor: settings.codeEditor || settings.CodeEditor || "",
  dbClientPreference:
    settings.dbClientPreference || settings.DBClientPreference || "desktop",
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
  const updateRefs = (newRefs) => {
    refs = newRefs;
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
      applyTheme(settings.theme);
    } catch (_err) {
      applyTheme("system");
    }
  };

  const save = async () => {
    const theme = refs.themeSelect?.value || "system";
    const proxyTarget = refs.proxyTarget?.value || "";
    const preferredBrowser = refs.preferredBrowser?.value || "";
    const codeEditor = refs.codeEditor?.value || "";
    const dbClientPreference = refs.dbClientPreference?.value || "desktop";
    try {
      const message = await bridge.updateSettings({
        theme,
        proxyTarget,
        preferredBrowser,
        codeEditor,
        dbClientPreference,
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

  return { toggleDrawer, load, save, reset };
};

export const renderSettingsDrawer = (container) => {
  if (!container) return;
  container.innerHTML = `
      <div
        class="drawer hidden fixed inset-0 z-[100] bg-[#0c1810]/40 backdrop-blur-sm"
        id="settingsDrawer"
        aria-hidden="true"
      >
        <div
          class="bg-[#1a3322] border-l border-[#2e573a] h-full ml-auto w-[420px] p-8 flex flex-col gap-6 shadow-2xl"
        >
          <div
            class="flex justify-between items-center mb-4 border-b border-[#2e573a] pb-4"
          >
            <h3 class="text-white text-lg font-bold">Govard Settings</h3>
            <button
              class="p-2 rounded hover:bg-white/5 text-slate-400 hover:text-white flex items-center justify-center h-9 w-9"
              id="closeSettings"
              type="button"
            >
              <span class="material-symbols-outlined">close</span>
            </button>
          </div>
          <!-- Settings fields -->
          <div class="flex flex-col gap-6 overflow-y-auto pr-2">
            <label
              class="flex flex-col gap-2 text-sm font-medium text-slate-300"
            >
              Theme
              <select
                id="themeSelect"
                class="bg-[#102316] border border-[#2e573a] rounded-lg px-4 py-3 text-white outline-none focus:border-primary/50"
              >
                <option value="system">System</option>
                <option value="light">Light</option>
                <option value="dark">Dark</option>
              </select>
            </label>
            <label
              class="flex flex-col gap-2 text-sm font-medium text-slate-300"
            >
              Proxy target
              <input
                id="proxyTarget"
                type="text"
                placeholder="govard.test"
                class="bg-[#102316] border border-[#2e573a] rounded-lg px-4 py-3 text-white outline-none focus:border-primary/50"
              />
            </label>
            <label
              class="flex flex-col gap-2 text-sm font-medium text-slate-300"
            >
              Code Editor (IDE)
              <input
                id="codeEditor"
                type="text"
                placeholder="code"
                class="bg-[#102316] border border-[#2e573a] rounded-lg px-4 py-3 text-white outline-none focus:border-primary/50"
              />
            </label>
            <label
              class="flex flex-col gap-2 text-sm font-medium text-slate-300"
            >
              Preferred browser
              <input
                id="preferredBrowser"
                type="text"
                placeholder="firefox"
                class="bg-[#102316] border border-[#2e573a] rounded-lg px-4 py-3 text-white outline-none focus:border-primary/50"
              />
            </label>
            <label
              class="flex flex-col gap-2 text-sm font-medium text-slate-300"
            >
              Database Client
              <select
                id="dbClientPreference"
                class="bg-[#102316] border border-[#2e573a] rounded-lg px-4 py-3 text-white outline-none focus:border-primary/50 appearance-none cursor-pointer"
              >
                <option value="desktop">Local Client (e.g. TablePlus)</option>
                <option value="pma">PHPMyAdmin (Proxy Container)</option>
              </select>
            </label>
          </div>
          <div class="mt-auto">
            <button
              class="w-full px-5 py-3 bg-[#22492f] border border-[#366b47] rounded-lg text-sm text-white hover:bg-[#2e573a] transition-all"
              data-action="reset-settings"
            >
              Reset Settings
            </button>
          </div>
        </div>
      </div>
  `;
};
