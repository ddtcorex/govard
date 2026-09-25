import { useEffect } from "react";
import { byId } from "../utils/dom.js";

type SettingsController = {
  load(): Promise<void>;
  save(): Promise<void>;
  reset(): Promise<void>;
  checkForUpdates(options?: { silent?: boolean }): Promise<unknown>;
  installLatestUpdate(): Promise<unknown>;
  setUpdateChannel(channel: string): Promise<unknown>;
  toggleDrawer(open: boolean): void;
  updateRefs(refs: Record<string, unknown>): void;
};
type Props = {
  bridge: { quit(): Promise<unknown> };
  controller: SettingsController;
  onOpenChange(open: boolean): void;
  onResetSettings(): Promise<void>;
  registerRefs(refs: Record<string, unknown>): void;
};

/** Every element the settings controller writes into, resolved once it exists. */
const collectRefs = () => ({
  settingsDrawer: byId("settingsDrawer"),
  closeSettings: byId("closeSettings"),
  themeSelect: byId("themeSelect"),
  proxyTarget: byId("proxyTarget"),
  codeEditor: byId("codeEditor"),
  preferredBrowser: byId("preferredBrowser"),
  dbClientPreference: byId("dbClientPreference"),
  runInBackgroundToggle: byId("runInBackgroundToggle"),
  trayUnavailableHint: byId("trayUnavailableHint"),
  settingsUpdateBadge: byId("settingsUpdateBadge"),
  settingsUpdateStatus: byId("settingsUpdateStatus"),
  settingsUpdateChangelog: byId("settingsUpdateChangelog"),
  updateChannelSelect: byId("updateChannelSelect"),
  checkUpdatesButton: byId("checkUpdatesButton"),
  installUpdateButton: byId("installUpdateButton"),
});

/**
 * The settings drawer. The markup mirrors what renderSettingsDrawer injected,
 * element for element and class for class (no look change), minus the
 * data-action attributes the global delegate resolved (D5).
 *
 * The drawer is the one island that keeps its controller rather than porting it
 * (spec, island contract): createSettingsController owns a small update state
 * machine - badge, changelog, install-button visibility, channel resync after a
 * failed switch, and the tray hint - that is covered by twelve unit tests
 * against fake refs. The island therefore renders the structure once and hands
 * the controller the real elements through the seam the controller already has
 * (updateRefs), exactly as main.js used to. It deliberately holds no state of
 * its own: React must not re-render the nodes the controller mutates, and the
 * drawer's `hidden` class is toggled synchronously by the controller, which
 * main.js's isSettingsDrawerOpen probe and the update prompt read.
 */
export function SettingsDrawer({ bridge, controller, onOpenChange, onResetSettings, registerRefs }: Props) {
  useEffect(() => {
    registerRefs(collectRefs());
    // main.js's bootstrap load can run before React has committed this markup,
    // so the drawer loads itself as well; whichever call finds no refs is a
    // no-op inside the controller.
    void controller.load();
  }, [controller, registerRefs]);

  return (
    <div
      className="drawer hidden fixed inset-0 z-100 bg-slate-900/40 dark:bg-background-primary/80 backdrop-blur-md transition-all duration-500"
      id="settingsDrawer"
      aria-hidden="true"
      onClick={(event) => {
        if (event.target === event.currentTarget) onOpenChange(false);
      }}
    >
      <div className="bg-white dark:bg-[#112217] border-l border-slate-200 dark:border-white/5 h-full ml-auto w-[460px] shadow-[0_0_80px_rgba(0,0,0,0.6)] flex flex-col relative overflow-hidden">
        <div className="absolute -top-40 -right-40 w-96 h-96 bg-primary/10 blur-[120px] pointer-events-none rounded-full"></div>
        <div className="absolute bottom-20 -left-20 w-64 h-64 bg-primary/5 blur-[100px] pointer-events-none rounded-full"></div>

        <header className="flex items-center justify-between p-8 border-b border-slate-200 dark:border-white/5 relative z-10 bg-slate-50/80 dark:bg-black/10 backdrop-blur-xs">
          <div className="flex items-center gap-4">
            <div className="size-12 rounded-2xl bg-primary/10 border border-primary/20 flex items-center justify-center text-primary shadow-lg shadow-primary/5">
              <span className="material-symbols-outlined text-[28px]">settings</span>
            </div>
            <div>
              <h3 className="text-text-primary text-xl font-bold tracking-tight">Application Settings</h3>
              <p className="text-text-tertiary text-xs mt-0.5 font-medium">Customize your workspace environment</p>
            </div>
          </div>
          <button
            className="group p-2.5 rounded-xl hover:bg-slate-200 dark:hover:bg-white/10 text-slate-500 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white transition-all flex items-center justify-center h-10 w-10 border border-slate-200 dark:border-white/5 hover:border-slate-300 dark:hover:border-white/20 active:scale-90"
            id="closeSettings"
            data-testid="close-settings"
            type="button"
            onClick={() => onOpenChange(false)}
          >
            <span className="material-symbols-outlined group-hover:rotate-90 transition-transform">close</span>
          </button>
        </header>

        <div className="flex-1 overflow-y-auto custom-scrollbar p-8 flex flex-col gap-10 relative z-10">
          <section className="flex flex-col gap-6">
            <div className="flex items-center gap-2.5 px-1">
              <div className="w-1.5 h-4 bg-primary rounded-full"></div>
              <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-text-tertiary">Environment &amp; Workspace</p>
            </div>

            <div className="grid grid-cols-1 gap-5">
              <div className="flex flex-col gap-2 group">
                <label htmlFor="themeSelect" className="text-[11px] font-bold text-text-tertiary uppercase tracking-wider ml-1 group-focus-within:text-primary transition-colors">Interface Theme</label>
                <div className="relative">
                  <select
                    id="themeSelect"
                    className="w-full bg-slate-50 dark:bg-[#162a1d] border border-slate-200 dark:border-white/10 rounded-xl px-4 py-3.5 text-slate-900 dark:text-white outline-hidden focus:border-primary/50 focus:ring-4 focus:ring-primary/10 appearance-none cursor-pointer transition-all hover:bg-slate-100 dark:hover:bg-surface-primary font-medium"
                    onChange={() => void controller.save()}
                  >
                    <option value="system">System Default</option>
                    <option value="light">Light Mode</option>
                    <option value="dark">Dark Mode</option>
                  </select>
                  <span className="material-symbols-outlined absolute right-4 top-1/2 -translate-y-1/2 text-text-tertiary pointer-events-none text-xl">unfold_more</span>
                </div>
              </div>
              <div className="flex flex-col gap-2 group">
                <label htmlFor="proxyTarget" className="text-[11px] font-bold text-text-tertiary uppercase tracking-wider ml-1 group-focus-within:text-primary transition-colors">Proxy Gateway URL</label>
                <div className="relative">
                  <input
                    id="proxyTarget"
                    type="text"
                    placeholder="govard.test"
                    className="w-full bg-surface-secondary dark:bg-[#162a1d] border border-border-primary dark:border-white/10 rounded-xl px-4 py-3.5 text-text-primary dark:text-white outline-hidden focus:border-primary/50 focus:ring-4 focus:ring-primary/10 transition-all placeholder:text-text-tertiary dark:placeholder:text-slate-600 hover:bg-slate-100 dark:hover:bg-surface-primary font-medium"
                    onChange={() => void controller.save()}
                  />
                  <span className="material-symbols-outlined absolute right-4 top-1/2 -translate-y-1/2 text-text-tertiary dark:text-slate-600 pointer-events-none text-xl">dns</span>
                </div>
              </div>

              <div className="flex flex-col gap-2 group">
                <label htmlFor="codeEditor" className="text-[11px] font-bold text-text-tertiary uppercase tracking-wider ml-1 group-focus-within:text-primary transition-colors">Favorite IDE Command</label>
                <div className="relative">
                  <input
                    id="codeEditor"
                    type="text"
                    placeholder="code"
                    className="w-full bg-surface-secondary dark:bg-[#162a1d] border border-border-primary dark:border-white/10 rounded-xl px-4 py-3.5 text-text-primary dark:text-white outline-hidden focus:border-primary/50 focus:ring-4 focus:ring-primary/10 transition-all placeholder:text-text-tertiary dark:placeholder:text-slate-600 hover:bg-slate-100 dark:hover:bg-surface-primary font-medium"
                    onChange={() => void controller.save()}
                  />
                  <span className="material-symbols-outlined absolute right-4 top-1/2 -translate-y-1/2 text-text-tertiary dark:text-slate-600 pointer-events-none text-xl">code</span>
                </div>
              </div>

              <div className="flex flex-col gap-2 group">
                <label htmlFor="preferredBrowser" className="text-[11px] font-bold text-text-tertiary uppercase tracking-wider ml-1 group-focus-within:text-primary transition-colors">Primary Web Browser</label>
                <div className="relative">
                  <input
                    id="preferredBrowser"
                    type="text"
                    placeholder="firefox"
                    className="w-full bg-surface-secondary dark:bg-[#162a1d] border border-border-primary dark:border-white/10 rounded-xl px-4 py-3.5 text-text-primary dark:text-white outline-hidden focus:border-primary/50 focus:ring-4 focus:ring-primary/10 transition-all placeholder:text-text-tertiary dark:placeholder:text-slate-600 hover:bg-slate-100 dark:hover:bg-surface-primary font-medium"
                    onChange={() => void controller.save()}
                  />
                  <span className="material-symbols-outlined absolute right-4 top-1/2 -translate-y-1/2 text-text-tertiary dark:text-slate-600 pointer-events-none text-xl">open_in_new</span>
                </div>
              </div>

              <div className="flex flex-col gap-2 group">
                <label htmlFor="dbClientPreference" className="text-[11px] font-bold text-text-tertiary uppercase tracking-wider ml-1 group-focus-within:text-primary transition-colors">Database Access Method</label>
                <div className="relative">
                  <select
                    id="dbClientPreference"
                    className="w-full bg-slate-50 dark:bg-[#162a1d] border border-slate-200 dark:border-white/10 rounded-xl px-4 py-3.5 text-slate-900 dark:text-white outline-hidden focus:border-primary/50 focus:ring-4 focus:ring-primary/10 appearance-none cursor-pointer transition-all hover:bg-slate-100 dark:hover:bg-surface-primary font-medium"
                    onChange={() => void controller.save()}
                  >
                    <option value="pma">Built-in PHPMyAdmin</option>
                    <option value="desktop">Local App (TablePlus/BeeKeeper)</option>
                  </select>
                  <span className="material-symbols-outlined absolute right-4 top-1/2 -translate-y-1/2 text-text-tertiary pointer-events-none text-xl">database</span>
                </div>
              </div>
            </div>
          </section>

          <section className="flex flex-col gap-6">
            <div className="flex items-center gap-2.5 px-1">
              <div className="w-1.5 h-4 bg-primary rounded-full"></div>
              <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-text-tertiary">System Behavior</p>
            </div>

            <div className="bg-slate-100/70 dark:bg-[#162a1d]/40 border border-slate-200 dark:border-white/5 rounded-2xl p-6 flex items-center justify-between hover:bg-slate-200/60 dark:hover:bg-[#162a1d]/60 transition-all cursor-default group hover:border-primary/10">
              <div className="flex items-center gap-4">
                <div className="size-10 rounded-xl bg-primary/5 flex items-center justify-center text-primary/60 group-hover:text-primary group-hover:bg-primary/10 transition-all">
                  <span className="material-symbols-outlined text-[20px]">background_replace</span>
                </div>
                <div className="flex flex-col gap-0.5">
                  <span className="text-sm font-bold text-text-primary dark:text-slate-200">Run in background</span>
                  <span className="text-[11px] text-text-tertiary font-medium leading-relaxed max-w-[200px]">Keep services active in tray when closing window</span>
                </div>
              </div>
              <label className="relative inline-flex items-center cursor-pointer group/toggle">
                <input
                  id="runInBackgroundToggle"
                  type="checkbox"
                  className="sr-only peer"
                  onChange={() => void controller.save()}
                />
                <div className="w-11 h-6 bg-slate-700 peer-focus:outline-hidden rounded-full peer peer-checked:after:translate-x-full peer-checked:rtl:after:-translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:inset-s-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary"></div>
              </label>
            </div>
            <p
              id="trayUnavailableHint"
              data-testid="tray-unavailable-hint"
              className="hidden px-1 text-[11px] font-medium leading-relaxed text-amber-600 dark:text-amber-300"
            >
              No system tray detected. Closing the window will quit Govard. On GNOME, enable the AppIndicator extension.
            </p>
          </section>

          <section className="flex flex-col gap-6">
            <div className="flex items-center gap-2.5 px-1">
              <div className="w-1.5 h-4 bg-primary rounded-full"></div>
              <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-text-tertiary">Maintenance &amp; Updates</p>
            </div>

            <div className="relative overflow-hidden rounded-2xl border border-primary/20 bg-primary/5 p-6 pb-5">
              <div className="pointer-events-none absolute -right-8 -top-8 h-32 w-32 rounded-full bg-primary/10 blur-3xl opacity-50"></div>
              <div className="relative z-10">
                <div className="flex items-center justify-between mb-5">
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 rounded-xl bg-primary/10 border border-primary/20 flex items-center justify-center text-primary">
                      <span className="material-symbols-outlined text-[22px]">auto_awesome</span>
                    </div>
                    <div>
                      <h4 className="text-sm font-bold text-slate-900 dark:text-white tracking-tight">Software Updates</h4>
                      <p className="text-[11px] text-text-tertiary font-medium mt-0.5">Automated version control</p>
                    </div>
                  </div>
                  <span
                    id="settingsUpdateBadge"
                    className="px-2.5 py-1 rounded-full border border-slate-200 dark:border-white/5 bg-slate-100 dark:bg-black/40 text-[10px] font-black uppercase tracking-wider text-primary shadow-xs"
                  >
                    Idle
                  </span>
                </div>

                <div className="flex items-center justify-between mb-5 -mt-1">
                  <label htmlFor="updateChannelSelect" className="text-[11px] font-bold text-text-tertiary uppercase tracking-wider">Update channel</label>
                  <select
                    id="updateChannelSelect"
                    className="bg-slate-50 dark:bg-[#162a1d] border border-slate-200 dark:border-white/10 rounded-lg px-3 py-1.5 text-xs font-semibold text-slate-900 dark:text-white outline-hidden focus:border-primary/50 cursor-pointer"
                    onChange={(event) => void controller.setUpdateChannel(event.target.value)}
                  >
                    <option value="stable">Stable</option>
                    <option value="beta">Beta</option>
                  </select>
                </div>

                <div className="mt-4 mb-6 p-5 bg-white dark:bg-black/20 rounded-2xl border border-slate-200 dark:border-white/5 flex flex-col gap-4">
                  <p
                    className="update-message-text text-[14px] font-medium text-slate-700 dark:text-slate-200 leading-snug flex items-center gap-3"
                    id="settingsUpdateStatus"
                    aria-live="polite"
                  >
                    <span className="material-symbols-outlined text-[18px] text-primary/60">info</span>
                    <span>Version check has not been run yet.</span>
                  </p>
                  <div
                    id="settingsUpdateChangelog"
                    className="hidden text-[13px] leading-relaxed text-slate-600 dark:text-slate-400 max-h-[160px] overflow-y-auto font-sans whitespace-pre-wrap select-text custom-scrollbar pr-2"
                  ></div>
                </div>

                <div className="flex flex-col gap-2.5">
                  <button
                    className="flex w-full items-center justify-center gap-2.5 rounded-xl border border-slate-200 dark:border-white/10 bg-slate-50 dark:bg-white/5 px-4 py-3 text-xs font-bold text-slate-600 dark:text-slate-300 transition-all hover:bg-slate-100 dark:hover:bg-white/10 hover:text-slate-900 dark:hover:text-white active:scale-[0.98] disabled:opacity-50"
                    data-testid="check-updates"
                    id="checkUpdatesButton"
                    type="button"
                    onClick={() => void controller.checkForUpdates()}
                  >
                    <span className="material-symbols-outlined text-[18px]">sync</span>
                    <span>Scan for updates</span>
                  </button>
                  <button
                    className="hidden flex w-full items-center justify-center gap-2.5 rounded-xl bg-primary px-4 py-3 text-[13px] font-black text-slate-900 shadow-[0_12px_24px_rgba(13,242,89,0.2)] transition-all hover:scale-[1.02] active:scale-[0.98] disabled:opacity-50"
                    data-testid="install-update"
                    id="installUpdateButton"
                    type="button"
                    onClick={() => void controller.installLatestUpdate()}
                  >
                    <span className="material-symbols-outlined text-[20px]">download_for_offline</span>
                    <span>Update Govard Now</span>
                  </button>
                </div>
              </div>
            </div>
          </section>
        </div>

        <footer className="p-8 bg-slate-50/90 dark:bg-black/40 border-t border-slate-200 dark:border-white/5 backdrop-blur-xl relative z-20 shadow-[0_-4px_20px_rgba(0,0,0,0.06)] dark:shadow-[0_-10px_40px_rgba(0,0,0,0.3)]">
          <div className="grid grid-cols-2 gap-4">
            <button
              className="flex items-center justify-center gap-2.5 px-4 py-3.5 bg-surface-secondary dark:bg-white/5 border border-border-primary dark:border-white/10 rounded-xl text-[13px] font-bold text-text-secondary dark:text-slate-400 hover:bg-slate-200 dark:hover:bg-white/10 hover:text-text-primary dark:hover:text-white hover:border-slate-300 dark:hover:border-white/20 transition-all active:scale-[0.98] group"
              data-testid="reset-settings"
              type="button"
              onClick={() => void onResetSettings()}
            >
              <span className="material-symbols-outlined text-[18px] group-hover:rotate-180 transition-transform duration-500">restart_alt</span>
              <span>Reset Settings</span>
            </button>
            <button
              className="flex items-center justify-center gap-2.5 px-4 py-3.5 bg-red-500/5 border border-red-500/20 rounded-xl text-[13px] font-bold text-red-400 hover:bg-red-500/15 hover:text-red-300 hover:border-red-500/40 transition-all active:scale-[0.98] group"
              data-testid="quit-app"
              type="button"
              onClick={() => void bridge.quit().catch(() => {})}
            >
              <span className="material-symbols-outlined text-[18px] group-hover:scale-110 transition-transform">power_settings_new</span>
              <span>Quit Govard</span>
            </button>
          </div>
        </footer>
      </div>
    </div>
  );
}
