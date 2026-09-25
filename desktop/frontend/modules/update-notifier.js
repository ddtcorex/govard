import { formatUpdateMessage } from "./update-message.js";

const DEFAULT_STARTUP_DELAY_MS = 12000;
const DEFAULT_CHECK_INTERVAL_MS = 6 * 60 * 60 * 1000;
const DEFAULT_INTERVAL_JITTER_RATIO = 0.15;

/**
 * Headless update-prompt model. The React island (islands/UpdatePrompt.tsx)
 * reads it through useSyncExternalStore: every change publishes a new frozen
 * snapshot and notifies subscribers. Timers and the dismiss and settings-drawer
 * rules live here so they stay testable without a DOM.
 */
export const createUpdateNotifierModel = ({
  settingsController,
  onStatus,
  isSettingsDrawerOpen,
}) => {
  const state = {
    checking: false,
    installing: false,
    visible: false,
    currentVersion: "",
    latestVersion: "",
    changelog: "",
    message: "",
    dismissedVersion: "",
    startupTimerID: null,
    intervalTimerID: null,
  };

  const hasUndismissedUpdate = () => {
    const latestVersion = String(state.latestVersion || "").trim();
    if (!latestVersion) {
      return false;
    }
    return latestVersion !== state.dismissedVersion;
  };

  const listeners = new Set();
  let snapshot = Object.freeze({
    visible: false,
    installing: false,
    currentVersion: "",
    latestVersion: "",
    message: "",
    changelog: "",
  });

  const render = () => {
    snapshot = Object.freeze({
      visible: state.visible,
      installing: state.installing,
      currentVersion: state.currentVersion,
      latestVersion: state.latestVersion,
      message: state.message,
      changelog: state.changelog,
    });
    for (const listener of listeners) listener();
  };

  const setPromptVisibility = (visible) => {
    state.visible = Boolean(visible);
    render();
  };

  const syncWithSettingsDrawer = () => {
    const shouldShowPrompt = hasUndismissedUpdate() && !isSettingsDrawerOpen();
    setPromptVisibility(shouldShowPrompt);
    return shouldShowPrompt;
  };

  const clearTimers = () => {
    if (state.startupTimerID !== null) {
      clearTimeout(state.startupTimerID);
      state.startupTimerID = null;
    }
    if (state.intervalTimerID !== null) {
      clearTimeout(state.intervalTimerID);
      state.intervalTimerID = null;
    }
  };

  const dismissPrompt = () => {
    if (state.latestVersion) {
      state.dismissedVersion = state.latestVersion;
    }
    setPromptVisibility(false);
  };

  const checkForUpdatesInBackground = async () => {
    if (state.checking || state.installing) {
      return { skipped: true, reason: "busy" };
    }

    state.checking = true;
    try {
      const result = await settingsController.checkForUpdates({ silent: true });
      if (!result || result.skipped || result.failed || !result.outdated) {
        if (result && result.outdated === false) {
          setPromptVisibility(false);
        }
        return result;
      }

      const latestVersion = String(result.latestVersion || "").trim();
      if (latestVersion && latestVersion === state.dismissedVersion) {
        return result;
      }

      state.currentVersion = String(result.currentVersion || "").trim();
      state.latestVersion = latestVersion;
      state.changelog = String(result.changelog || "").trim();
      state.message = formatUpdateMessage(result);
      const promptVisible = syncWithSettingsDrawer();
      if (promptVisible) {
        onStatus("Update available.");
      }
      return {
        ...result,
        promptVisible,
      };
    } finally {
      state.checking = false;
      render();
    }
  };

  const installLatestUpdateFromPrompt = async () => {
    if (state.installing) {
      return { ok: false, skipped: true, reason: "busy" };
    }

    state.installing = true;
    render();

    try {
      const outcome = await settingsController.installLatestUpdate();
      if (outcome?.ok) {
        dismissPrompt();
        return outcome;
      }

      if (outcome?.message) {
        state.message = outcome.message;
      }
      render();
      return outcome || { ok: false, skipped: false };
    } finally {
      state.installing = false;
      render();
    }
  };

  const scheduleBackgroundChecks = ({
    startupDelayMs = DEFAULT_STARTUP_DELAY_MS,
    intervalMs = DEFAULT_CHECK_INTERVAL_MS,
    intervalJitterRatio = DEFAULT_INTERVAL_JITTER_RATIO,
  } = {}) => {
    clearTimers();

    const scheduleNextIntervalCheck = () => {
      const boundedJitter = Math.max(
        0,
        Math.min(0.5, Number(intervalJitterRatio) || 0),
      );
      const jitterWindowMs = intervalMs * boundedJitter;
      const jitterOffsetMs =
        jitterWindowMs > 0 ? (Math.random() * 2 - 1) * jitterWindowMs : 0;
      const nextDelayMs = Math.max(
        60 * 1000,
        Math.round(intervalMs + jitterOffsetMs),
      );

      state.intervalTimerID = setTimeout(async () => {
        await checkForUpdatesInBackground();
        scheduleNextIntervalCheck();
      }, nextDelayMs);
    };

    state.startupTimerID = setTimeout(async () => {
      await checkForUpdatesInBackground();
      scheduleNextIntervalCheck();
    }, startupDelayMs);
  };

  return {
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    getSnapshot: () => snapshot,
    dismissPrompt,
    syncWithSettingsDrawer,
    checkForUpdatesInBackground,
    installLatestUpdateFromPrompt,
    scheduleBackgroundChecks,
    clearTimers,
  };
};
