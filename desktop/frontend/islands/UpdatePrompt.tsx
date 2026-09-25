import { useSyncExternalStore } from "react";

type Snapshot = {
  visible: boolean;
  installing: boolean;
  currentVersion: string;
  latestVersion: string;
  message: string;
  changelog: string;
};

type Model = {
  subscribe(listener: () => void): () => void;
  getSnapshot(): Snapshot;
  dismissPrompt(): void;
  installLatestUpdateFromPrompt(): Promise<unknown>;
};

type Props = { model: Model };

/**
 * The floating "Update available" prompt. It renders the headless model from
 * modules/update-notifier.js, which owns the background checks, the dismissed
 * version and the settings-drawer rule. The markup mirrors the static block it
 * replaced, element for element (no look change), minus the action attributes
 * the global click delegate used to route (D5). The install button shows
 * "Download & Install" from the start: the legacy controller overwrote the
 * static "Update Now" label on its first render, before the prompt could show.
 */
export function UpdatePrompt({ model }: Props) {
  const snapshot = useSyncExternalStore(model.subscribe, model.getSnapshot);
  const hasChangelog = snapshot.changelog.trim() !== "";

  return (
    <div
      id="updatePrompt"
      className={`${snapshot.visible ? "" : "hidden "}fixed z-140 right-4 bottom-[56px] w-[min(420px,calc(100%-32px))] md:right-8 md:bottom-[64px]`}
      aria-hidden={snapshot.visible ? "false" : "true"}
    >
      <div className="update-prompt-panel p-6 rounded-[32px] shadow-[0_24px_60px_rgba(0,0,0,0.4)] border border-white/10 backdrop-blur-2xl">
        <div className="flex items-center gap-4 mb-5">
          <div className="h-12 w-12 rounded-2xl bg-primary/10 border border-primary/20 text-primary inline-flex items-center justify-center shrink-0 shadow-lg shadow-primary/10">
            <span className="material-symbols-outlined text-[22px]">system_update_alt</span>
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-slate-900 dark:text-white text-lg font-bold tracking-tight">Update available</p>
          </div>
          <button
            onClick={() => model.dismissPrompt()}
            className="h-10 w-10 rounded-xl text-slate-500 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white hover:bg-slate-200 dark:hover:bg-white/10 inline-flex items-center justify-center transition-all active:scale-95"
            type="button"
            aria-label="Dismiss update prompt"
          >
            <span className="material-symbols-outlined text-[22px]">close</span>
          </button>
        </div>

        <div className="bg-slate-50 dark:bg-black/30 rounded-2xl border border-slate-200 dark:border-white/5 p-5 flex flex-col gap-4">
          <p
            id="updatePromptMessage"
            className="update-message-text text-[14px] font-medium text-slate-700 dark:text-slate-200 leading-snug"
          >
            {snapshot.message || "A new Govard Desktop version is available."}
          </p>
          <div
            id="updatePromptChangelog"
            className={`${hasChangelog ? "" : "hidden "}text-[13px] leading-relaxed text-slate-600 dark:text-slate-400 max-h-[320px] overflow-y-auto font-sans whitespace-pre-wrap select-text custom-scrollbar pr-3`}
          >
            {snapshot.changelog}
          </div>
          <div className="pt-4 border-t border-slate-200 dark:border-white/5 text-[12px] font-mono text-slate-500 dark:text-slate-400 flex items-center justify-between">
            <div className="flex items-center gap-2">
              <span className="opacity-60 text-[10px] uppercase font-black tracking-widest">Current</span>
              <span id="updatePromptCurrent" className="text-slate-700 dark:text-slate-200 font-bold">
                {snapshot.currentVersion || "-"}
              </span>
            </div>
            <div className="flex items-center gap-3">
              <span className="text-primary font-black">→</span>
              <span id="updatePromptLatest" className="text-primary font-black text-sm">
                {snapshot.latestVersion || "-"}
              </span>
            </div>
          </div>
        </div>

        <div className="mt-5 flex items-center justify-end gap-3">
          <button
            onClick={() => model.dismissPrompt()}
            className="px-4 py-2.5 text-xs font-bold rounded-xl border border-slate-200 dark:border-white/10 text-slate-500 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white hover:bg-slate-100 dark:hover:bg-white/5 transition-all active:scale-95"
            type="button"
          >
            Later
          </button>
          <button
            id="installUpdatePromptButton"
            onClick={() => void model.installLatestUpdateFromPrompt()}
            disabled={snapshot.installing}
            className="px-5 py-2.5 text-xs font-black rounded-xl bg-primary text-slate-900 hover:scale-[1.02] active:scale-[0.98] inline-flex items-center justify-center gap-2 disabled:opacity-50 transition-all shadow-[0_12px_24px_rgba(var(--primary-rgb),0.2)]"
            type="button"
          >
            <span className="material-symbols-outlined text-[18px]">
              {snapshot.installing ? "install_desktop" : "download"}
            </span>
            <span>{snapshot.installing ? "Installing..." : "Download & Install"}</span>
          </button>
        </div>
      </div>
    </div>
  );
}
