import test from "node:test";
import assert from "node:assert/strict";

import {
  formatMetricMB,
  formatMetricPercent,
  normalizeMetricsPayload,
} from "../../desktop/frontend/modules/metrics.js";

test("normalizeMetricsPayload maps system metrics", () => {
  const payload = normalizeMetricsPayload({
    CPUUsage: 24.5,
    MemoryUsage: 1536.2,
  });

  assert.equal(payload.systemCPU, 24.5);
  assert.equal(payload.systemMemory, 1536.2);
});

test("normalizeMetricsPayload supports lowercase metric keys", () => {
  const payload = normalizeMetricsPayload({
    cpuUsage: 7.6,
    memoryUsage: 4096.4,
  });

  assert.equal(payload.systemCPU, 7.6);
  assert.equal(payload.systemMemory, 4096.4);
});

test("normalizeMetricsPayload falls back to safe defaults", () => {
  const payload = normalizeMetricsPayload({});
  assert.equal(payload.systemCPU, 0);
  assert.equal(payload.systemMemory, 0);
});

test("formatMetric helpers render stable strings", () => {
  assert.equal(formatMetricPercent(12.345), "12.3%");
  assert.equal(formatMetricPercent(0), "0.0%");
  assert.equal(formatMetricMB(1024.08), "1024.1 MB");
  assert.equal(formatMetricMB(0), "0.0 MB");
});
