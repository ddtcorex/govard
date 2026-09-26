import { useCallback, useEffect, useRef, useState } from "react";
import {
  formatSyncPlanDetails,
  resolveSyncPresetConfig,
  sanitizeSyncPlanText,
} from "../modules/remotes.js";
import { getState, setState } from "../state/store.js";

type SyncOptionDef = {
  key: string;
  label: string;
  description: string;
  defaultValue: boolean;
};
type SyncModalBridge = {
  getSyncPresetOptions(project: string, preset: string): Promise<Record<string, unknown>>;
  runRemoteSyncPreset(
    project: string,
    remote: string,
    preset: string,
    config: Record<string, unknown>,
  ): Promise<string>;
};
type ToastKind = "info" | "success" | "error" | "warning";
/** What main.js keeps calling after the modal moved into React. */
export type SyncModalApi = {
  open(remote: string, preset: string): Promise<void>;
  close(): void;
};
type Props = {
  bridge: SyncModalBridge;
  onStatus(message: string): void;
  onToast(message: string, kind?: ToastKind): void;
  onModalBlur(visible: boolean): void;
  onConfirmSync(input: {
    remote: string;
    preset: string;
    config: Record<string, unknown>;
  }): void;
  registerApi(api: SyncModalApi | null): void;
};

/**
 * The dialog's four states. The vanilla code drove the same four with class
 * writes and two timeouts (10ms in, 300ms out); naming them is what keeps the
 * transition identical while React owns the nodes.
 */
type Phase = "closed" | "opening" | "open" | "closing";

const OPEN_DELAY_MS = 10;
const CLOSE_DELAY_MS = 300;

/**
 * The sync modal. The markup mirrors what renderSyncModal injected, element for
 * element and class for class (no look change), minus the data-action attributes
 * the global delegate used to resolve (D5).
 *
 * Two-step machine: step 1 is the option list the backend declares for the
 * preset, step 2 is the plan it renders for the current configuration. The
 * document lookups the delegate did are gone with it - remotes.js:603's
 * document.querySelector for a button outside the controller's container is a
 * ref to this island's own confirm button, and the Escape/backdrop close paths
 * are a listener this island registers while it is open and drops with it.
 */
export function SyncModal({
  bridge,
  onStatus,
  onToast,
  onModalBlur,
  onConfirmSync,
  registerApi,
}: Props) {
  const [phase, setPhase] = useState<Phase>("closed");
  const [remote, setRemote] = useState("");
  const [preset, setPreset] = useState("");
  const [options, setOptions] = useState<SyncOptionDef[]>([]);
  const [config, setConfig] = useState<Record<string, unknown>>({});
  const [step, setStep] = useState<"options" | "preview">("options");
  const [planText, setPlanText] = useState("");
  const [planLoading, setPlanLoading] = useState(false);
  const openTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  /** Bumped by every open and every close: only the newest open may reveal itself. */
  const openRequest = useRef(0);
  /** The phase as of the last commit, so close() can read it outside an updater. */
  const phaseRef = useRef<Phase>("closed");

  useEffect(() => {
    phaseRef.current = phase;
  }, [phase]);

  const clearTimers = useCallback(() => {
    if (openTimer.current !== null) {
      clearTimeout(openTimer.current);
      openTimer.current = null;
    }
    if (closeTimer.current !== null) {
      clearTimeout(closeTimer.current);
      closeTimer.current = null;
    }
  }, []);

  const close = useCallback(() => {
    clearTimers();
    // Closing supersedes an open that is still awaiting its option list, so that
    // continuation cannot reveal a dialog the user already dismissed.
    openRequest.current += 1;
    if (phaseRef.current !== "closed") {
      // Outside the updater on purpose: an updater must be pure, and this writes
      // to the DOM (main.js's modal blur).
      onModalBlur(false);
      setPhase("closing");
    }
    closeTimer.current = setTimeout(() => {
      closeTimer.current = null;
      setPhase("closed");
      // Reset to step 1 after the close animation, as the vanilla code did.
      setStep("options");
      setPlanText("");
      setPlanLoading(false);
    }, CLOSE_DELAY_MS);
  }, [clearTimers, onModalBlur]);

  const open = useCallback(
    async (remoteName: string, presetName: string) => {
      if (!remoteName || !presetName) {
        return;
      }
      clearTimers();
      const request = openRequest.current + 1;
      openRequest.current = request;
      setRemote(remoteName);
      setPreset(presetName);
      setStep("options");
      setPlanText("");
      setPlanLoading(false);
      setState({
        currentSyncRemote: remoteName,
        currentSyncPreset: presetName,
      });

      try {
        const project = String(getState().selectedProject || "");
        const payload = await bridge.getSyncPresetOptions(project, presetName);
        const optionsDef = Array.isArray(payload?.options)
          ? (payload.options as SyncOptionDef[])
          : [];
        const stored = (getState().syncConfigs || {})[presetName] || {};
        const nextConfig = resolveSyncPresetConfig(optionsDef, stored);
        setOptions(optionsDef);
        setConfig(nextConfig);
        // The store keeps the same shape it had before the port: the onboarding
        // flow resolves a preset's config from it, and two islands now read and
        // write this key through setState.
        setState({
          syncConfigs: { ...(getState().syncConfigs || {}), [presetName]: nextConfig },
          currentSyncPresetDefs: optionsDef,
        });
      } catch (err) {
        console.error("Failed to load sync options", err);
      }

      if (openRequest.current !== request) {
        return;
      }

      onModalBlur(true);
      setPhase("opening");
      openTimer.current = setTimeout(() => {
        openTimer.current = null;
        setPhase("open");
      }, OPEN_DELAY_MS);
    },
    [bridge, clearTimers, onModalBlur],
  );

  useEffect(() => {
    registerApi({ open, close });
    // Handing the API back on unmount is what stops main.js's own call
    // sites from reaching an island that no longer exists: the island's timers
    // die with it, but the closures main.js kept would not.
    return () => registerApi(null);
  }, [registerApi, open, close]);

  useEffect(() => () => clearTimers(), [clearTimers]);

  // Escape closes the modal from anywhere on the page, but only while it is up:
  // the vanilla listener lived in main.js's global keydown handler and is this
  // island's to register and drop now.
  useEffect(() => {
    if (phase === "closed") {
      return undefined;
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        close();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [phase, close]);

  const toggleOption = useCallback(
    (key: string) => {
      const nextValue = !config[key];
      const nextConfig = { ...config, [key]: nextValue };
      setConfig(nextConfig);
      setState({
        syncConfigs: { ...(getState().syncConfigs || {}), [preset]: nextConfig },
      });
      onStatus(`Option "${key}" ${nextValue ? "enabled" : "disabled"}.`);
    },
    [config, onStatus, preset],
  );

  const previewPlan = useCallback(async () => {
    const project = String(getState().selectedProject || "");
    if (!remote || !preset || !project) {
      return;
    }

    const optionDefs = getState().currentSyncPresetDefs || options;
    const planDetails = formatSyncPlanDetails({
      remoteName: remote,
      preset,
      config,
      optionDefs,
    });

    setStep("preview");
    setPlanText("");
    setPlanLoading(true);

    try {
      const plan = await bridge.runRemoteSyncPreset(project, remote, preset, config);
      const normalizedPlan = sanitizeSyncPlanText(plan) || "No plan details returned.";
      setPlanText(`${planDetails}\n\n${normalizedPlan}`);
    } catch (err) {
      const failure = sanitizeSyncPlanText(err) || "Unknown error";
      setPlanText(`${planDetails}\n\nFailed to generate plan: ${failure}`);
    } finally {
      setPlanLoading(false);
    }
  }, [bridge, config, options, preset, remote]);

  const confirm = useCallback(() => {
    if (!remote || !preset) {
      return;
    }
    close();
    onConfirmSync({ remote, preset, config });
  }, [close, config, onConfirmSync, preset, remote]);

  const hidden = phase === "closed";
  const scaled = phase !== "open";

  return (
    <div
      id="syncOptionsModal"
      className={`${hidden ? "hidden " : ""}fixed inset-0 z-150 bg-background-primary/60 backdrop-blur-md flex items-center justify-center p-4 transition-opacity duration-300 ${scaled ? "opacity-0" : ""}`}
      onClick={(event) => {
        if (event.target === event.currentTarget) {
          close();
        }
      }}
    >
      <div
        className={`bg-surface-primary border border-border-primary rounded-xl w-full max-w-lg shadow-2xl flex flex-col overflow-hidden transition-transform duration-300 ${scaled ? "scale-95" : ""}`}
      >
        <div className="px-6 py-4 border-b border-border-primary flex justify-between items-center bg-surface-secondary/50">
          <h3 className="text-text-primary dark:text-white text-lg font-bold flex items-center gap-2">
            <span className="material-symbols-outlined text-primary" id="syncModalIcon">
              {step === "preview" ? "fact_check" : "sync"}
            </span>
            <span id="syncModalTitle">
              {step === "preview" ? "Sync Preview" : "Sync Options"}
            </span>
          </h3>
          <button
            type="button"
            className="text-slate-500 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white transition-colors"
            data-testid="close-sync-modal"
            onClick={close}
          >
            <span className="material-symbols-outlined">close</span>
          </button>
        </div>

        <div id="syncModalStep1" className={`p-6 space-y-4 ${step === "preview" ? "hidden" : ""}`}>
          <p className="text-text-secondary dark:text-slate-300 text-sm">
            You are about to sync data from the{" "}
            <strong id="syncModalRemoteName" className="text-text-primary dark:text-white font-black">
              {remote}
            </strong>{" "}
            environment. Configure your sync options below:
          </p>

          <div id="syncModalOptionsContainer" className="space-y-4">
            {options.map((option) => {
              const checked = Boolean(config[option.key]);
              return (
                <label
                  key={option.key}
                  className="flex items-center justify-between cursor-pointer group p-3 rounded-lg border border-border-primary bg-background-secondary/30 hover:bg-background-secondary/50 transition-all"
                >
                  <div className="flex-1">
                    <div className="text-xs font-bold text-slate-800 dark:text-white">
                      {option.label}
                    </div>
                    <div className="text-[10px] text-slate-500 dark:text-slate-400">
                      {option.description}
                    </div>
                  </div>
                  <div className="relative inline-block w-10 h-6 align-middle select-none transition duration-200 ease-in">
                    <input
                      type="checkbox"
                      data-testid="toggle-sync-config"
                      data-preset={preset}
                      data-config={option.key}
                      className="toggle-checkbox absolute block w-4 h-4 rounded-full bg-white border-4 border-slate-600 appearance-none cursor-pointer transition-all duration-300 top-1 left-1 checked:left-5 checked:bg-white checked:border-white/0"
                      checked={checked}
                      onChange={() => toggleOption(option.key)}
                    />
                    <span
                      className={`toggle-label block overflow-hidden h-6 rounded-full bg-slate-700 cursor-pointer transition-colors duration-300 group-hover:bg-slate-600 ${checked ? "bg-primary" : ""}`}
                    ></span>
                  </div>
                </label>
              );
            })}
          </div>

          <div className="px-0 pt-4 flex gap-3 justify-end items-center border-t border-border-primary">
            <button
              type="button"
              className="px-4 py-2 rounded-lg text-sm text-text-secondary dark:text-slate-300 font-medium hover:bg-primary/10 transition-colors"
              data-testid="cancel-sync-options"
              onClick={close}
            >
              Cancel
            </button>
            <button
              type="button"
              data-testid="preview-sync-plan"
              id="previewSyncPlanBtn"
              className="px-5 py-2 bg-surface-secondary dark:bg-slate-700 hover:bg-primary/10 dark:hover:bg-slate-600 border border-border-primary dark:border-slate-500 rounded-lg text-sm text-text-primary dark:text-white font-medium transition-all group flex items-center gap-2"
              onClick={() => void previewPlan()}
            >
              <span className="material-symbols-outlined text-[16px] group-hover:text-primary transition-colors">
                preview
              </span>
              <span>Preview Plan</span>
            </button>
          </div>
        </div>

        <div id="syncModalStep2" className={`p-6 space-y-4 ${step === "preview" ? "" : "hidden"}`}>
          <div className="flex items-center gap-2 text-sm text-text-secondary dark:text-slate-300">
            <span className="material-symbols-outlined text-[18px] text-primary">fact_check</span>
            Review the actions below, then confirm to proceed:
          </div>

          <div
            id="syncPlanOutput"
            className={`bg-background-primary border border-border-primary/60 rounded-lg p-4 font-mono text-xs text-text-secondary dark:text-slate-300 max-h-64 overflow-y-auto leading-relaxed whitespace-pre-wrap ${planText ? "" : "hidden"}`}
          >
            {planText}
          </div>

          <div
            id="syncPlanLoading"
            className={`${planLoading ? "flex" : "hidden"} items-center gap-3 text-sm text-slate-400 py-2`}
          >
            <span className="inline-block w-4 h-4 rounded-full border-2 border-primary border-t-transparent animate-spin shrink-0"></span>
            Generating plan...
          </div>

          <div className="pt-4 flex gap-3 justify-between items-center border-t border-border-primary">
            <button
              type="button"
              data-testid="back-to-sync-options"
              className="px-4 py-2 rounded-lg text-sm text-text-secondary dark:text-slate-300 font-medium hover:bg-primary/10 transition-colors flex items-center gap-1"
              onClick={() => setStep("options")}
            >
              <span className="material-symbols-outlined text-[16px]">arrow_back</span>
              Back
            </button>
            <div className="flex gap-3">
              <button
                type="button"
                className="px-4 py-2 rounded-lg text-sm text-text-secondary dark:text-slate-300 font-medium hover:bg-primary/10 transition-colors"
                data-testid="cancel-sync-preview"
                onClick={close}
              >
                Cancel
              </button>
              <button
                type="button"
                data-testid="confirm-sync"
                id="confirmSyncBtn"
                className="px-5 py-2 bg-primary text-slate-900 rounded-lg text-sm font-bold hover:bg-primary/90 transition-all flex items-center gap-2 shadow-lg shadow-primary/10 active:scale-95"
                onClick={confirm}
              >
                <span className="material-symbols-outlined text-[16px] transition-colors">
                  play_arrow
                </span>
                <span>Execute Sync</span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
