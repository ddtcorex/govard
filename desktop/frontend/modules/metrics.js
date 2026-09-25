const asNumber = (value) => {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
};

export const formatMetricPercent = (value = 0) =>
  `${asNumber(value).toFixed(1)}%`;
export const formatMetricMB = (value = 0) => `${asNumber(value).toFixed(1)} MB`;

export const normalizeMetricsPayload = (payload = {}) => {
  return {
    systemCPU: asNumber(
      payload.systemCPU ?? payload.cpuUsage ?? payload.CPUUsage ?? 0,
    ),
    systemMemory: asNumber(
      payload.systemMemory ?? payload.memoryUsage ?? payload.MemoryUsage ?? 0,
    ),
  };
};
