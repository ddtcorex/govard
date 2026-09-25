const state = {
  sidebarMode: "global-services",
  environments: [],
  selectedProject: "",
  selectedService: "all",
  selectedSeverity: "all",
  logQuery: "",
  globalServices: [],
  selectedGlobalService: "caddy",
  globalLogSeverity: "all",
  globalLogQuery: "",
  liveLogsEnabled: false,
  globalLiveLogsEnabled: false,
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
