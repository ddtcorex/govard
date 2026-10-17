const state = {
  sidebarMode: "global-services",
  environments: [],
  // The sidebar list shows its loading frame until main.js publishes the first
  // dashboard, and again for every non-silent refresh. It lives here and not in
  // the island so a list that mounts after the data arrived still renders it.
  envListLoading: true,
  selectedProject: "",
  selectedService: "all",
  // The dashboard fetch outcome: a non-empty dashboardError means the last
  // refresh failed, and the hero and the sidebar list render their offline
  // state (with its own Retry) instead of fake projects. lastRefreshed is the
  // epoch ms of the last successful fetch, shown in the footer.
  dashboardError: "",
  lastRefreshed: 0,
  selectedSeverity: "all",
  logQuery: "",
  globalServices: [],
  selectedGlobalService: "caddy",
  globalLogSeverity: "all",
  globalLogQuery: "",
  liveLogsEnabled: false,
  globalLiveLogsEnabled: false,
  // The ops deck's derived inputs plus the feedback line: the controller
  // publishes both and the two deck/list islands render from them (D6).
  globalServicesSnapshot: null,
  globalServicesLoading: false,
  globalServicesError: "",
  globalActionFeedback: {
    message: "Ready for global operations.",
    tone: "info",
    seq: 0,
  },
  terminalModalOpen: false,
  /**
   * The per-preset sync options the user has toggled, keyed by preset. Two
   * readers share it: the remotes list island (the modal's configuration) and
   * main.js's onboarding flow, which resolves a preset's config from it.
   * @type {Record<string, Record<string, unknown>>}
   */
  syncConfigs: {},
  /**
   * The option definitions the backend declared for the open preset.
   * @type {Array<{key: string; label?: string; description?: string; defaultValue?: boolean}>}
   */
  currentSyncPresetDefs: [],
  currentSyncRemote: "",
  currentSyncPreset: "",
  // The remotes island owns these now: it is the only reader of "a sync is
  // running", and main.js's delegate no longer writes them. Declared here so the
  // store keeps naming its own shape ("" rather than null: the island compares
  // them against a remote name).
  syncingProject: "",
  syncingRemote: "",
  syncingPreset: "",
  /**
   * Project-scoped env-operation locks, keyed by project to the label holding
   * it (`env-start`, `env-stop`, ...). Published by modules/operations.js on
   * every begin/end so the hero and the sidebar can disable visibly (with a
   * spinner) instead of refusing through the status line. Other projects stay
   * actionable while one entry is held.
   * @type {Record<string, string>}
   */
  busyEnvs: {},
  // Doctor + audit health UX (Task 3): the health controller publishes here
  // and GlobalHealthHeader/HealthPanels derive banners and panels from it.
  // Banner state always derives from the latest report/result, never cached
  // independently, so a clean re-run clears stale banners. Doctor and audit
  // entries are keyed by normalized project scope with a current-scope
  // pointer: switching projects shows that scope's loading/empty state,
  // never another scope's banner. The preview slice carries no token: the
  // preview id lives in module-local controller state only.
  healthDoctor: { byProject: {}, scope: "" },
  healthAudit: { byProject: {}, scope: "" },
  healthPreview: { open: false, checkId: "", summary: "", actions: [] },
};

const listeners = new Set();
let version = 0;

export const getState = () => state;

/**
 * A revision number, bumped on every setState that carries a patch. The store
 * mutates in place, so `getState()` always returns the same object: that identity
 * is what the vanilla modules want, but it is invisible to
 * useSyncExternalStore, which re-renders only when the snapshot it reads changes.
 * React watches this number and reads the state through getState() (spec D6).
 */
export const getVersion = () => version;

/**
 * The subscriber seam React islands read through (spec D6).
 */
export const subscribe = (listener) => {
  listeners.add(listener);
  return () => listeners.delete(listener);
};

export const setState = (patch) => {
  const hasPatch = Boolean(patch) && Object.keys(patch).length > 0;
  Object.assign(state, patch || {});
  // A call that changed nothing must not wake every subscriber: the vanilla
  // modules call setState liberally, and React only re-renders when the version
  // it watches changes, so notifying anyway was a pure side effect on no-ops.
  if (hasPatch) {
    version += 1;
    listeners.forEach((l) => l());
  }
  return state;
};
