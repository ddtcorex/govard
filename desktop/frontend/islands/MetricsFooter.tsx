import { useCallback, useEffect, useState } from "react";
import { formatMetricMB, formatMetricPercent, normalizeMetricsPayload } from "../modules/metrics.js";

type RefreshOptions = { silent?: boolean };
type Metrics = { systemCPU: number; systemMemory: number };

type Props = {
  bridge: { getSystemMetrics(): Promise<unknown> };
  onStatus(message: string): void;
  registerRefresh(fn: (opts?: RefreshOptions) => Promise<Metrics | null>): void;
  intervalMs?: number;
};

/**
 * Footer CPU and memory readout plus its refresh control. The island owns the
 * polling interval, so unmounting it is what stops the background refresh; the
 * markup mirrors the static footer it replaced, element for element (no look
 * change), minus the data-action the global delegate used to route (D5).
 *
 * main.js keeps triggering a silent refresh from refreshDashboard through the
 * function handed to registerRefresh.
 */
export function MetricsFooter({ bridge, onStatus, registerRefresh, intervalMs = 15000 }: Props) {
  const [metrics, setMetrics] = useState<Metrics>({ systemCPU: 0, systemMemory: 0 });

  const refresh = useCallback(
    async ({ silent = false }: RefreshOptions = {}) => {
      try {
        // The lightweight GetSystemMetrics, not the heavyweight GetResourceMetrics.
        const next: Metrics = normalizeMetricsPayload((await bridge.getSystemMetrics()) ?? {});
        setMetrics(next);
        if (!silent) onStatus(`Status: system metrics updated at ${new Date().toLocaleTimeString()}`);
        return next;
      } catch {
        if (!silent) onStatus("Status: system metrics unavailable");
        return null;
      }
    },
    [bridge, onStatus],
  );

  useEffect(() => {
    registerRefresh(refresh);
    // React mounts asynchronously, so main.js's first refreshDashboard can run
    // before this effect registers the function; fetch once here so the
    // readout never waits a full interval for its first value.
    void refresh({ silent: true });
    const timer = setInterval(() => void refresh({ silent: true }), intervalMs);
    return () => clearInterval(timer);
  }, [refresh, registerRefresh, intervalMs]);

  return (
    <>
      <div className="flex items-center gap-1 text-[10px] text-text-tertiary font-mono">
        <span className="material-symbols-outlined text-[14px]">memory</span>
        <span id="footerCPU">{formatMetricPercent(metrics.systemCPU)}</span>
      </div>
      <div className="flex items-center gap-1 text-[10px] text-text-tertiary font-mono">
        <span className="material-symbols-outlined text-[14px]">hard_drive</span>
        <span id="footerMemory">{formatMetricMB(metrics.systemMemory)}</span>
      </div>
      <div className="h-8 w-px bg-border-primary"></div>
      <button
        type="button"
        data-testid="refresh-metrics"
        aria-label="Refresh system metrics"
        onClick={() => void refresh()}
        className="h-6 w-6 inline-flex items-center justify-center text-primary hover:text-primary-hover dark:hover:text-white transition-colors shrink-0"
      >
        <span className="material-symbols-outlined text-[16px]">sync</span>
      </button>
    </>
  );
}
