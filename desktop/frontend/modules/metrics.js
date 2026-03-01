import { clearChildren, escapeHTML, setText } from "../utils/dom.js";

const asNumber = (value) => {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
};

export const formatMetricPercent = (value = 0) =>
  `${asNumber(value).toFixed(1)}%`;
export const formatMetricMB = (value = 0) => `${asNumber(value).toFixed(1)} MB`;

export const normalizeMetricsPayload = (payload = {}) => {
  return {
    systemCPU: asNumber(payload.systemCPU ?? payload.CPUUsage ?? 0),
    systemMemory: asNumber(payload.systemMemory ?? payload.MemoryUsage ?? 0),
  };
};

export const createMetricsController = ({
  bridge,
  refs,
  onStatus,
  getProject,
}) => {
  const updateRefs = (newRefs) => {
    refs = newRefs;
  };
  let refreshTimer = null;

  const renderPayload = (payload) => {
    const metrics = normalizeMetricsPayload(payload);

    // Footer always shows system metrics
    if (refs.footerCPU)
      setText(refs.footerCPU, formatMetricPercent(metrics.systemCPU));
    if (refs.footerMemory)
      setText(refs.footerMemory, formatMetricMB(metrics.systemMemory));

    return metrics;
  };

  const refresh = async ({ silent = false } = {}) => {
    try {
      // Use the lightweight GetSystemMetrics instead of the heavyweight GetResourceMetrics
      const payload = await bridge.system.GetSystemMetrics();
      const metrics = renderPayload(payload);
      if (!silent) {
        onStatus(
          `Status: system metrics updated at ${new Date().toLocaleTimeString()}`,
        );
      }
      return metrics;
    } catch (err) {
      if (!silent) {
        onStatus("Status: system metrics unavailable");
      }
      return null;
    }
  };

  const startAutoRefresh = () => {
    if (refreshTimer) {
      clearInterval(refreshTimer);
    }
    refreshTimer = setInterval(() => {
      refresh({ silent: true });
    }, 15000);
  };

  const stopAutoRefresh = () => {
    if (refreshTimer) {
      clearInterval(refreshTimer);
      refreshTimer = null;
    }
  };

  return {
    refresh,
    startAutoRefresh,
    stopAutoRefresh,
  };
};

export const renderMetricSkeletons = () => {};
