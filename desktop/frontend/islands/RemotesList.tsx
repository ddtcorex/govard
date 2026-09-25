import { useCallback, useEffect, useRef, useState } from "react";
import { hasEventRuntime, onEvent } from "../services/events.js";
import {
  applySyncOutputBatch,
  canUseSyncPreset,
  formatAuthMethodLabel,
  getSyncPresetLabel,
  normalizeRemotesPayload,
  protectedWarningCopy,
  safeRemotes,
  sanitizeSyncToastLine,
  syncInProgressReason,
  syncPresets,
} from "../modules/remotes.js";
import { getState, setState } from "../state/store.js";
import { useStore } from "../state/useStore.js";

type RemoteRow = {
  name: string;
  host: string;
  user: string;
  path: string;
  port: number;
  protected: boolean;
  authMethod: string;
  lastSync: string;
  capabilities: string[];
};
type RemotesBridge = {
  getRemotes(project: string): Promise<Record<string, unknown>>;
  testRemote(project: string, remote: string): Promise<string>;
  openRemoteShell(project: string, remote: string): Promise<string>;
  openRemoteDB(project: string, remote: string): Promise<string>;
  openRemoteSFTP(project: string, remote: string): Promise<string>;
  runRemoteSyncBackground(
    project: string,
    remote: string,
    preset: string,
    config: Record<string, unknown>,
  ): Promise<string>;
};
type ToastKind = "info" | "success" | "error" | "warning";
/** What main.js keeps calling after the tab moved into React. */
export type RemotesIslandApi = {
  refresh(options?: { silent?: boolean }): Promise<void>;
  runSync(input: {
    project: string;
    remoteName: string;
    preset: string;
    config?: Record<string, unknown>;
  }): Promise<void>;
};
type Props = {
  bridge: RemotesBridge;
  onStatus(message: string): void;
  onToast(message: string, kind?: ToastKind): void;
  onOpenSyncModal(remote: string, preset: string): void;
  onSyncSettled(): void;
  registerApi(api: RemotesIslandApi): void;
};

/** How long an Open SSH/DB/SFTP button stays in its loading state at minimum. */
const MIN_REMOTE_OPEN_LOADING_MS = 1400;
/** The copy that shows before the first refresh answers. */
const IDLE_NOTICE = "Select an environment to load remotes.";
const EMPTY_NOTICE = "No remotes configured for this project.";

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));
const busyKey = (remote: string, action: string) => `${remote}:${action}`;

const presetButtonClass = (enabled: boolean) =>
  enabled
    ? "flex-1 px-4 py-2.5 bg-background-secondary hover:bg-(--border-primary) border border-border-primary dark:border-[#366b47] rounded-lg text-sm text-text-primary dark:text-white font-medium transition-all flex items-center justify-center gap-2 group/btn"
    : "flex-1 px-4 py-2.5 bg-background-secondary dark:bg-[#13231a] border border-border-primary dark:border-[#2b3d31] rounded-lg text-sm text-slate-500 font-medium transition-all flex items-center justify-center gap-2 opacity-70 cursor-not-allowed";

const openButtonClass =
  "flex-1 min-h-[42px] px-4 py-2.5 bg-background-secondary hover:bg-surface-primary border border-border-primary rounded-lg text-sm text-text-secondary dark:text-slate-300 hover:text-text-primary dark:hover:text-white font-medium transition-all flex items-center justify-center gap-2 group/btn";

type ProgressState = {
  visible: boolean;
  entered: boolean;
  title: string;
  lines: string[];
  failed: boolean;
  running: boolean;
};

const IDLE_PROGRESS: ProgressState = {
  visible: false,
  entered: false,
  title: "Sync Terminal",
  lines: ["Initializing connection..."],
  failed: false,
  running: false,
};

/**
 * The remotes tab. The island owns the whole subtree the vanilla renderer
 * injected into #tab-remotes: the header's Refresh control, the warning list,
 * the cards and the hover "visual" panel, element for element and class for
 * class (no look change), minus the data-action attributes the global delegate
 * resolved (D5).
 *
 * Three things the vanilla controller did through the document are React state
 * here, and each was a real defect class rather than a preference:
 * createRemotesController.refresh() wrote innerHTML into #remotesList, so the
 * list could never be React-owned while the controller existed; testRemote found
 * its own button with document.querySelector, which breaks the moment two
 * remotes share a name or the card is re-rendered; and the two card hover
 * listeners were added on every re-render and never removed. The sync runner
 * (previously main.js's runRemoteSyncWithProgressToast) lives here too, because
 * every node it streamed into is inside this subtree.
 */
export function RemotesList({
  bridge,
  onStatus,
  onToast,
  onOpenSyncModal,
  onSyncSettled,
  registerApi,
}: Props) {
  const state = useStore();
  const [remotes, setRemotes] = useState<RemoteRow[]>([]);
  const [warnings, setWarnings] = useState<string[]>([]);
  const [notice, setNotice] = useState(IDLE_NOTICE);
  const [hovered, setHovered] = useState<string | null>(null);
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const [progress, setProgress] = useState<ProgressState>(IDLE_PROGRESS);

  const progressRef = useRef<HTMLDivElement | null>(null);
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const pendingActions = useRef<Set<string>>(new Set());
  const subscriptions = useRef<Array<() => void>>([]);
  // The settled callback reaches the event handlers below through a ref, so a
  // run started before main.js handed a new callback still calls the newest one.
  const settled = useRef(onSyncSettled);
  useEffect(() => {
    settled.current = onSyncSettled;
  }, [onSyncSettled]);

  const releaseSubscriptions = useCallback(() => {
    subscriptions.current.forEach((off) => off());
    subscriptions.current = [];
  }, []);
  useEffect(() => () => releaseSubscriptions(), [releaseSubscriptions]);

  const refresh = useCallback(
    async (options: { silent?: boolean } = {}) => {
      const silent = Boolean(options.silent);
      // Read the selection from the store rather than a prop: main.js calls
      // refresh() in the same synchronous block as the setState that selects a
      // project, before React has re-rendered with it.
      const project = String(getState().selectedProject || "").trim();
      if (!project) {
        setRemotes([]);
        setWarnings(["Select a project to load remotes."]);
        setNotice(EMPTY_NOTICE);
        return;
      }

      if (!silent) {
        onStatus(`Status: loading remotes for ${project}...`);
      }

      try {
        const raw = await bridge.getRemotes(project);
        const payload = normalizeRemotesPayload(raw);
        setRemotes(payload.remotes);
        setWarnings(payload.warnings);
        setNotice(EMPTY_NOTICE);
        if (!silent) {
          onStatus(`Status: remotes loaded for ${project}`);
        }
      } catch (err) {
        console.error("[Remotes] Failed to load remotes:", err);

        if (hasEventRuntime()) {
          setRemotes([]);
          setWarnings(["Failed to load remotes from backend."]);
          setNotice(EMPTY_NOTICE);
          if (!silent) {
            onStatus("Status: failed to load remotes");
          }
          return;
        }

        const payload = normalizeRemotesPayload(safeRemotes);
        setRemotes(payload.remotes);
        setWarnings(payload.warnings);
        setNotice(EMPTY_NOTICE);
        if (!silent) {
          onStatus("Status: showing local remotes fallback");
        }
      }
    },
    [bridge, onStatus],
  );

  const testRemote = useCallback(
    async (remoteName: string) => {
      const project = String(getState().selectedProject || "").trim();
      if (!project || !remoteName) {
        return;
      }

      setBusy((prev) => ({ ...prev, [busyKey(remoteName, "test")]: true }));
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
        setBusy((prev) => {
          const next = { ...prev };
          delete next[busyKey(remoteName, "test")];
          return next;
        });
      }
    },
    [bridge, onStatus, onToast],
  );

  // One in-flight action per project+remote+action, and a floor on how long the
  // button spins so a fast local answer does not flash the loading state.
  const runOpenRemoteAction = useCallback(
    async (
      remoteName: string,
      actionName: string,
      runner: (project: string) => Promise<void>,
    ) => {
      const project = String(getState().selectedProject || "").trim();
      if (!project || !remoteName) {
        return;
      }
      const key = `${project}:${remoteName}:${actionName}`;
      if (pendingActions.current.has(key)) {
        return;
      }

      pendingActions.current.add(key);
      const startedAt = Date.now();
      setBusy((prev) => ({ ...prev, [busyKey(remoteName, actionName)]: true }));
      try {
        await runner(project);
      } finally {
        const remaining = MIN_REMOTE_OPEN_LOADING_MS - (Date.now() - startedAt);
        if (remaining > 0) {
          await wait(remaining);
        }
        setBusy((prev) => {
          const next = { ...prev };
          delete next[busyKey(remoteName, actionName)];
          return next;
        });
        pendingActions.current.delete(key);
      }
    },
    [],
  );

  const openRemoteShell = useCallback(
    async (remoteName: string) => {
      await runOpenRemoteAction(remoteName, "ssh", async (project) => {
        const message = await bridge.openRemoteShell(project, remoteName);
        onStatus(message || `Opened SSH for ${remoteName}`);
        onToast(message || `Opened SSH for ${remoteName}`, "success");
      }).catch((err) => {
        onStatus(`Failed to open SSH for ${remoteName}: ${err}`);
        onToast(`Failed to open SSH for ${remoteName}.`, "error");
      });
    },
    [bridge, onStatus, onToast, runOpenRemoteAction],
  );

  const openRemoteDB = useCallback(
    async (remoteName: string) => {
      await runOpenRemoteAction(remoteName, "db", async (project) => {
        const message = await bridge.openRemoteDB(project, remoteName);
        onStatus(message || `Opening remote database for ${remoteName}`);
        onToast(message || `Opening remote database for ${remoteName}.`, "info");
      }).catch((err) => {
        onStatus(`Failed to open remote DB for ${remoteName}: ${err}`);
        onToast(`Failed to open remote DB for ${remoteName}.`, "error");
      });
    },
    [bridge, onStatus, onToast, runOpenRemoteAction],
  );

  const openRemoteSFTP = useCallback(
    async (remoteName: string) => {
      await runOpenRemoteAction(remoteName, "sftp", async (project) => {
        const message = await bridge.openRemoteSFTP(project, remoteName);
        onStatus(message || `Opening SFTP for ${remoteName}`);
        onToast(message || `Opening SFTP for ${remoteName}.`, "info");
      }).catch((err) => {
        onStatus(`Failed to open SFTP for ${remoteName}: ${err}`);
        onToast(`Failed to open SFTP for ${remoteName}.`, "error");
      });
    },
    [bridge, onStatus, onToast, runOpenRemoteAction],
  );

  const runSync = useCallback(
    async ({
      project,
      remoteName,
      preset,
      config = {},
    }: {
      project: string;
      remoteName: string;
      preset: string;
      config?: Record<string, unknown>;
    }) => {
      const normalizedProject = String(project || "").trim();
      const normalizedRemote = String(remoteName || "").trim();
      const normalizedPreset = String(preset || "").trim();
      if (!normalizedProject || !normalizedRemote || !normalizedPreset) {
        return;
      }

      const title = getSyncPresetLabel(normalizedPreset, normalizedRemote);
      setState({
        syncingProject: normalizedProject,
        syncingRemote: normalizedRemote,
        syncingPreset: normalizedPreset,
      });
      setProgress({
        visible: true,
        entered: false,
        title,
        failed: false,
        running: true,
        lines: [`   ${title}`, `   ${"-".repeat(title.length)}`, ""],
      });

      releaseSubscriptions();
      if (hasEventRuntime()) {
        subscriptions.current = [
          onEvent("sync:output", (payload) => {
            setProgress((prev) => ({
              ...prev,
              lines: applySyncOutputBatch(prev.lines, String(payload ?? "")),
            }));
          }),
          onEvent("sync:completed", (message) => {
            const finalMessage = sanitizeSyncToastLine(message) || "Sync completed ✔";
            setProgress((prev) => ({
              ...prev,
              running: false,
              lines: [...prev.lines, "", `[SUCCESS] ${finalMessage}`, ""],
            }));
            setState({ syncingProject: "", syncingRemote: "", syncingPreset: "" });
            settled.current?.();
            releaseSubscriptions();
          }),
          onEvent("sync:failed", (message) => {
            const finalMessage = sanitizeSyncToastLine(message) || "Sync failed";
            setProgress((prev) => ({
              ...prev,
              failed: true,
              running: false,
              lines: [...prev.lines, "", `[FAILED] ${finalMessage}`, ""],
            }));
            setState({ syncingProject: "", syncingRemote: "", syncingPreset: "" });
            settled.current?.();
            releaseSubscriptions();
          }),
        ];
      }

      try {
        const result = await bridge.runRemoteSyncBackground(
          normalizedProject,
          normalizedRemote,
          normalizedPreset,
          config,
        );
        if (
          typeof result === "string" &&
          result.startsWith("Remote sync background process failed:")
        ) {
          const failure = sanitizeSyncToastLine(result) || "Sync failed";
          setProgress((prev) => ({
            ...prev,
            failed: true,
            running: false,
            lines: [...prev.lines, "", `[FAILED] ${failure}`, ""],
          }));
          releaseSubscriptions();
        }
      } catch (err) {
        console.error("Sync failed to start:", err);
        setState({ syncingProject: "", syncingRemote: "", syncingPreset: "" });
        releaseSubscriptions();
      }
    },
    [bridge, releaseSubscriptions],
  );

  useEffect(() => {
    registerApi({ refresh, runSync });
  }, [registerApi, refresh, runSync]);

  // The first value never waits for main.js's first refreshDashboard, which can
  // run before React has mounted at all.
  useEffect(() => {
    void refresh({ silent: true });
  }, [refresh]);

  // The panel starts hidden and enters: read a layout property to force the
  // reflow the vanilla code got from `void container.offsetWidth`, then take the
  // transition classes off so the 500ms slide actually plays.
  useEffect(() => {
    if (!progress.visible || progress.entered) {
      return;
    }
    if (progressRef.current) {
      void progressRef.current.offsetWidth;
    }
    setProgress((prev) => ({ ...prev, entered: true }));
  }, [progress.visible, progress.entered]);

  useEffect(() => {
    const viewport = viewportRef.current;
    if (viewport) {
      viewport.scrollTop = viewport.scrollHeight;
    }
  }, [progress.lines]);

  const defaultRemote = remotes.length ? remotes[0] : null;
  const visual = remotes.find((remote) => remote.name === hovered) ?? defaultRemote;
  const visualName = visual ? visual.name : "No Remotes";
  const visualHost = visual
    ? hovered
      ? visual.host
      : visual.host || visual.name
    : "Configure in Govard";
  const visualProtected = Boolean(visual?.protected);

  const sourceBorderColor = visualProtected ? "border-amber-500/40" : "border-blue-500/30";
  const sourceBadgeBg = visualProtected
    ? "bg-amber-600 dark:bg-amber-500"
    : "bg-blue-600 dark:bg-blue-500";
  const sourceBadgeBorder = visualProtected ? "border-amber-400" : "border-blue-400";
  const centerRingColor = visualProtected
    ? "border-amber-500/50 shadow-[0_0_15px_rgba(245,158,11,0.2)]"
    : "border-blue-500/30 shadow-[0_0_15px_rgba(59,130,246,0.2)]";
  const centerIconColor = visualProtected ? "text-amber-500" : "text-blue-500";
  const centerIcon = visualProtected ? "gpp_bad" : "cloud_sync";

  const syncingRemote = String(state.syncingRemote || "");
  const syncingPreset = String(state.syncingPreset || "");
  const anySyncing = syncingRemote !== "";

  const cards = remotes.map((remote) => {
    const isSyncing = syncingRemote === remote.name;
    const isProtected = Boolean(remote.protected);
    const themeColor = isSyncing ? "emerald" : isProtected ? "amber" : "blue";
    const themeIcon = isSyncing ? "sync" : isProtected ? "rocket_launch" : "science";
    const borderColor = isSyncing
      ? "border-emerald-500/50 shadow-[0_0_15px_rgba(16,185,129,0.1)]"
      : isProtected
        ? "border-amber-500/20"
        : "border-(--bg-secondary)";
    const lastSyncText = String(remote.lastSync || "never").trim().toLowerCase();
    const lastSyncTone =
      lastSyncText === "never"
        ? "text-amber-600 dark:text-amber-300"
        : "text-text-secondary dark:text-slate-200";
    // This card's own button, never a document lookup: two remotes may share a
    // name, and the card re-renders.
    const testIcon = `material-symbols-outlined text-[18px] ${busy[busyKey(remote.name, "test")] ? "animate-spin" : ""}`;

    return (
      <div
        key={remote.name}
        data-testid="remote-card"
        data-remote-name={remote.name}
        data-remote-host={remote.host}
        data-remote-protected={isProtected ? "true" : "false"}
        className={`remote-card-hover glass-card rounded-xl p-0 overflow-hidden group mb-6 border ${borderColor} dark:bg-card-bg cursor-pointer transition-all hover:scale-[1.01] hover:border-primary/50`}
        onMouseEnter={() => setHovered(remote.name)}
        onMouseLeave={() => setHovered(null)}
      >
        <div className="p-6 pb-4 border-b border-border-primary dark:border-(--bg-secondary) bg-linear-to-r from-surface-primary to-surface-primary/50 dark:from-(--surface-primary) dark:to-(--surface-primary)/50 relative overflow-hidden">
          <div className="relative z-10 flex flex-col gap-4">
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0 flex items-center gap-4">
                <div
                  className={`h-14 w-14 shrink-0 rounded-xl bg-${themeColor}-500/10 border border-${themeColor}-500/30 text-${themeColor}-400 flex items-center justify-center shadow-[0_0_0_1px_rgba(63,122,82,0.45)]`}
                >
                  <span className="material-symbols-outlined text-[26px]">{themeIcon}</span>
                </div>
                <div className="min-w-0">
                  <div className="flex items-center flex-wrap gap-2">
                    <h3 className="text-text-primary dark:text-white text-[1.4rem] leading-none font-semibold">
                      {remote.name}
                    </h3>
                    {isProtected ? (
                      <span className="px-2 py-0.5 rounded-sm text-[10px] font-bold bg-amber-500/20 text-amber-400 border border-amber-500/30 uppercase tracking-wide">
                        Protected
                      </span>
                    ) : null}
                    {isSyncing ? (
                      <span className="px-2 py-0.5 rounded-sm text-[10px] font-bold bg-emerald-500/20 text-emerald-400 border border-emerald-500/30 uppercase tracking-wide animate-pulse">
                        Syncing...
                      </span>
                    ) : null}
                  </div>
                  <p className="mt-1 text-[11px] uppercase tracking-[0.08em] text-primary/70">
                    Auth: {formatAuthMethodLabel(remote.authMethod)}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-1.5 p-1 rounded-lg border border-border-primary bg-surface-secondary/60 backdrop-blur-xs shadow-[0_0_15px_rgba(13,242,89,0.1)]">
                <button
                  type="button"
                  data-testid="open-remote-url"
                  className="h-8 w-8 flex items-center justify-center text-slate-500 dark:text-slate-300 hover:text-slate-900 dark:hover:text-white hover:bg-slate-200 dark:hover:bg-background-secondary rounded-md transition-all"
                  title="Open Remote URL"
                >
                  <span className="material-symbols-outlined text-[20px]">open_in_new</span>
                </button>
                <button
                  type="button"
                  data-testid="remote-test"
                  className="h-8 w-8 flex items-center justify-center text-slate-500 dark:text-slate-300 hover:text-slate-900 dark:hover:text-white hover:bg-slate-200 dark:hover:bg-background-secondary rounded-md transition-all"
                  title="Test Connection"
                  disabled={Boolean(busy[busyKey(remote.name, "test")])}
                  onClick={() => void testRemote(remote.name)}
                >
                  <span className={testIcon}>
                    {busy[busyKey(remote.name, "test")] ? "sync" : "wifi_tethering"}
                  </span>
                </button>
              </div>
            </div>
            <div className="flex flex-col lg:flex-row gap-2.5">
              <div className="min-w-0 flex items-center gap-2 px-3 py-2 rounded-lg border border-border-primary bg-surface-secondary/50 lg:flex-1">
                <span className="material-symbols-outlined text-[16px] text-slate-400">dns</span>
                <span className="text-[11px] uppercase tracking-wide text-slate-500">Host</span>
                <span
                  className="ml-auto min-w-0 truncate text-xs font-mono text-text-primary dark:text-slate-200"
                  title={remote.host}
                >
                  {remote.host}
                </span>
              </div>
              <div className="flex items-center gap-2 px-3 py-2 rounded-lg border border-border-primary bg-surface-secondary/50 lg:flex-1 min-w-0 overflow-hidden">
                <span className="material-symbols-outlined text-[16px] text-slate-400">history</span>
                <span className="text-[11px] uppercase tracking-wide text-slate-500 shrink-0">
                  Last sync
                </span>
                <span className={`ml-auto text-xs font-mono ${lastSyncTone} truncate ml-2`}>
                  {remote.lastSync || "never"}
                </span>
              </div>
            </div>
          </div>
        </div>
        <div className="p-6 pt-4">
          <div className="flex flex-col gap-3">
            <div className="flex flex-wrap gap-3">
              {syncPresets.map((entry) => {
                const canUse = canUseSyncPreset(remote, entry.capability);
                const enabled = canUse && (!anySyncing || isSyncing);
                const spinner = isSyncing && (syncingPreset === entry.preset || !syncingPreset);
                const title = enabled
                  ? entry.label
                  : anySyncing
                    ? syncInProgressReason
                    : entry.disabledReason;
                return (
                  <button
                    key={entry.preset}
                    type="button"
                    data-testid="open-sync-modal"
                    data-remote={remote.name}
                    data-preset={entry.preset}
                    className={presetButtonClass(enabled)}
                    title={title}
                    disabled={!enabled}
                    aria-disabled={!enabled || undefined}
                    onClick={() => onOpenSyncModal(remote.name, entry.preset)}
                  >
                    <span
                      className={
                        enabled
                          ? `material-symbols-outlined text-[18px] ${entry.iconHoverClass} transition-colors ${spinner ? "animate-spin" : ""}`
                          : "material-symbols-outlined text-[18px] text-slate-500"
                      }
                    >
                      {spinner ? "sync" : entry.icon}
                    </span>
                    {spinner ? "Syncing..." : entry.label}
                  </button>
                );
              })}
            </div>
            <div className="flex flex-wrap gap-3">
              <button
                type="button"
                data-testid="open-remote-shell"
                data-remote={remote.name}
                data-loading-label="Opening SSH..."
                className={openButtonClass}
                onClick={() => void openRemoteShell(remote.name)}
              >
                <span className="material-symbols-outlined inline-flex h-[18px] w-[18px] items-center justify-center text-[18px] opacity-70 group-hover/btn:opacity-100">
                  terminal
                </span>
                <span data-role="label">Open SSH</span>
              </button>
              <button
                type="button"
                data-testid="open-remote-db"
                data-remote={remote.name}
                data-loading-label="Opening Database..."
                className={openButtonClass}
                onClick={() => void openRemoteDB(remote.name)}
              >
                <span className="material-symbols-outlined inline-flex h-[18px] w-[18px] items-center justify-center text-[18px] opacity-70 group-hover/btn:opacity-100">
                  database
                </span>
                <span data-role="label">Open Database</span>
              </button>
              <button
                type="button"
                data-testid="open-remote-sftp"
                data-remote={remote.name}
                data-loading-label="Opening SFTP..."
                className={openButtonClass}
                onClick={() => void openRemoteSFTP(remote.name)}
              >
                <span className="material-symbols-outlined inline-flex h-[18px] w-[18px] items-center justify-center text-[18px] opacity-70 group-hover/btn:opacity-100">
                  folder_open
                </span>
                <span data-role="label">Open SFTP</span>
              </button>
            </div>
          </div>
          {isProtected ? (
            <div className="mt-4 flex items-center gap-2 p-2 bg-amber-900/10 border border-amber-900/30 rounded-sm text-amber-500/80 text-xs">
              <span className="material-symbols-outlined text-[16px]">info</span>
              {protectedWarningCopy}
            </div>
          ) : null}
        </div>
      </div>
    );
  });

  return (
    <div className="px-6 lg:px-10 py-6 max-w-[1248px] w-full mx-auto" id="remotes">
      <div className="space-y-8">
        <div className="flex items-center justify-between">
          <h2 className="text-slate-900 dark:text-white text-xl font-semibold flex items-center gap-2">
            <span className="material-symbols-outlined text-primary">cloud_sync</span>
            Remotes
          </h2>
          <div className="flex items-center gap-3">
            <button
              type="button"
              data-testid="refresh-remotes"
              className="px-4 py-2 bg-background-secondary text-text-primary dark:text-white rounded-lg text-sm font-medium hover:bg-primary/20 dark:hover:bg-[#2e573a] transition-colors border border-border-primary dark:border-[#366b47] flex items-center gap-2"
              onClick={() => void refresh()}
            >
              <span className="material-symbols-outlined text-lg">refresh</span>
              Refresh
            </button>
          </div>
        </div>
        <div>
          <div className="space-y-4">
            <ul id="remotesWarnings" className="text-red-500 px-4 text-sm list-disc">
              {warnings.map((warning, index) => (
                <li key={`${index}-${warning}`}>{warning}</li>
              ))}
            </ul>
            <div id="remotesList" className="space-y-6">
              {remotes.length > 0 ? (
                <div className="grid grid-cols-1 lg:grid-cols-5 gap-8 items-start">
                  <div className="lg:col-span-3 space-y-6">
                    <div className="flex items-center justify-between pb-2">
                      <h3 className="text-text-primary dark:text-white text-lg font-semibold flex items-center gap-2">
                        Connected Remotes
                      </h3>
                    </div>
                    {cards}
                  </div>
                  <div className="lg:col-span-2">
                    <div className="sticky top-6 flex flex-col items-center justify-center bg-white dark:bg-background-primary border border-border-primary rounded-xl overflow-hidden shadow-xl py-10">
                      <div
                        className="absolute inset-0 z-0 opacity-10"
                        style={{
                          backgroundImage:
                            "radial-gradient(var(--primary) 1px, transparent 1px)",
                          backgroundSize: "20px 20px",
                        }}
                      ></div>
                      <div className="relative z-10 w-full max-w-[200px]">
                        <div
                          id="visual-source-box"
                          className={`bg-surface-primary border ${sourceBorderColor} rounded-lg p-4 shadow-lg shadow-blue-500/5 relative transition-colors duration-300`}
                        >
                          <div
                            id="visual-source-badge"
                            className={`absolute -top-3 left-1/2 -translate-x-1/2 ${sourceBadgeBg} px-3 py-0.5 text-[10px] text-white border ${sourceBadgeBorder} rounded-full uppercase font-black tracking-wider shadow-xs transition-colors duration-300`}
                          >
                            Source
                          </div>
                          <div className="flex items-center justify-center gap-3">
                            <span className="material-symbols-outlined text-blue-400 text-3xl shrink-0">
                              cloud
                            </span>
                            <div className="text-left min-w-0 flex-1">
                              <div
                                id="visual-source-name"
                                className="text-slate-900 dark:text-white text-sm font-black truncate"
                                title={visualName}
                              >
                                {visualName}
                              </div>
                              <div
                                id="visual-source-host"
                                className="text-slate-600 dark:text-slate-500 text-[10px] truncate"
                                title={visualHost}
                              >
                                {visualHost}
                              </div>
                            </div>
                          </div>
                        </div>
                      </div>
                      <div className="h-8 w-px relative my-1 dashed-line bg-linear-to-b from-transparent via-border-primary to-transparent">
                        <div className="absolute top-0 left-1/2 -translate-x-1/2 ml-[-2px] w-[5px] h-6 bg-primary rounded-full animate-[bounce_1.5s_infinite]"></div>
                      </div>
                      <div className="relative z-10">
                        <div
                          id="visual-center-ring"
                          className={`bg-surface-secondary border rounded-full h-12 w-12 flex items-center justify-center transition-all duration-300 ${centerRingColor}`}
                        >
                          <span
                            id="visual-center-icon"
                            className={`material-symbols-outlined text-2xl transition-colors duration-300 ${centerIconColor}`}
                          >
                            {centerIcon}
                          </span>
                        </div>
                      </div>
                      <div className="h-8 w-px relative my-1 dashed-line bg-linear-to-b from-transparent via-border-primary to-transparent">
                        <div className="absolute top-0 left-1/2 -translate-x-1/2 ml-[-2px] w-[5px] h-6 bg-primary rounded-full animate-[bounce_1.5s_infinite] delay-300"></div>
                      </div>
                      <div className="relative z-10 w-full max-w-[200px] group/dest">
                        <div className="bg-background-secondary border border-primary/40 rounded-lg p-4 shadow-lg shadow-primary/10 relative">
                          <div className="absolute inset-0 bg-primary/5 opacity-0 group-hover/dest:opacity-100 animate-pulse transition-opacity rounded-lg"></div>
                          <div className="absolute -top-3 left-1/2 -translate-x-1/2 bg-emerald-500 dark:bg-primary px-3 py-0.5 text-[10px] text-slate-900 border border-emerald-400 dark:border-transparent rounded-full uppercase font-black tracking-wider shadow-xs">
                            Destination
                          </div>
                          <div className="flex items-center justify-center gap-3">
                            <span className="material-symbols-outlined text-primary text-3xl px-1 relative z-10 shrink-0">
                              laptop_mac
                            </span>
                            <div className="text-left relative z-10 min-w-0 flex-1">
                              <div
                                className="text-slate-900 dark:text-white text-sm font-black truncate"
                                title="local"
                              >
                                local
                              </div>
                              <div
                                className="text-slate-600 dark:text-slate-500 text-[10px] truncate"
                                title="Your Machine"
                              >
                                Your Machine
                              </div>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>

                    <div
                      id="visual-sync-progress-container"
                      ref={progressRef}
                      className={`${progress.visible ? "" : "hidden "}mt-8 w-full h-[500px] bg-slate-50 dark:bg-slate-950 rounded-xl overflow-hidden shadow-2xl border border-slate-200 dark:border-white/5 flex flex-col transform-gpu transition-all duration-500 ${progress.entered ? "" : "opacity-0 translate-y-4"}`}
                    >
                      <div className="h-8 flex items-center px-4 bg-white dark:bg-slate-900 border-b border-slate-200 dark:border-white/5 shrink-0 z-10 shadow-xs">
                        <div className="flex gap-1.5 shrink-0">
                          <div className="w-2.5 h-2.5 rounded-full bg-[#ff5f56] shadow-[0_0_8px_rgba(255,95,86,0.15)]"></div>
                          <div className="w-2.5 h-2.5 rounded-full bg-[#ffbd2e] shadow-[0_0_8px_rgba(255,189,46,0.1)]"></div>
                          <div className="w-2.5 h-2.5 rounded-full bg-[#27c93f] shadow-[0_0_8px_rgba(39,201,63,0.15)]"></div>
                        </div>
                        <div className="mx-auto flex items-center gap-2 px-6">
                          <span className="visual-sync-title text-[9px] font-medium text-slate-400 dark:text-slate-500 uppercase tracking-[0.25em] pointer-events-none truncate max-w-[240px]">
                            {progress.title}
                          </span>
                          <span
                            className={`visual-sync-indicator flex h-1 w-1 rounded-full ${progress.failed ? "bg-rose-500" : "bg-primary"} ${progress.running ? "animate-pulse" : ""}`}
                          ></span>
                        </div>
                        <div className="w-[50px] shrink-0"></div>
                      </div>
                      <div
                        id="visual-sync-scroll-viewport"
                        ref={viewportRef}
                        className="flex-1 overflow-y-auto p-4 bg-white dark:bg-black/95 custom-scrollbar selection:bg-primary/20"
                      >
                        <pre
                          id="visual-sync-progress-line"
                          className={`m-0 font-mono text-[11px] leading-relaxed ${progress.failed ? "text-rose-500" : "text-emerald-700 dark:text-emerald-400/90"} whitespace-pre-wrap break-all`}
                        >
                          {progress.lines.join("\n")}
                        </pre>
                      </div>
                    </div>
                  </div>
                </div>
              ) : (
                <div className="p-8 text-center text-slate-500 border border-dashed border-(--bg-secondary) rounded-xl">
                  {notice}
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
