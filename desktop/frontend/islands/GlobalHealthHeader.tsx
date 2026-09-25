import { useEffect, useState, type CSSProperties } from "react";
import {
  bulkActionButtonStates,
  deriveHealthSummary,
  feedbackTone,
  formatStatusLabel,
  globalServiceIcon,
  statusStripTone,
} from "../modules/global-services.js";
import { useStore } from "../state/useStore.js";
import type { GlobalServiceRow } from "./GlobalServicesList.tsx";

export type GlobalServiceSnapshot = {
  active: number;
  total: number;
  summary: string;
  warnings: string[];
  services: GlobalServiceRow[];
};

export type GlobalBulkAction = "start" | "restart" | "stop" | "pull";

type Props = {
  onBulkAction(action: GlobalBulkAction): Promise<void>;
};

const INITIAL_BAR_CLASS =
  "h-full w-0 rounded-full bg-linear-to-r from-primary/70 via-primary to-primary-hover transition-all duration-500";
const INITIAL_LABEL_CLASS =
  "mt-2 inline-flex items-center gap-1.5 rounded-md border border-primary/25 bg-primary/10 px-2 py-1 text-[10px] font-semibold text-primary";

const BULK_BUTTONS: Array<{
  action: GlobalBulkAction;
  id: string;
  testid: string;
  icon: string;
  label: string;
  loadingLabel: string;
  filled?: boolean;
}> = [
  {
    action: "start",
    id: "globalBulkStart",
    testid: "global-bulk-start",
    icon: "play_arrow",
    label: "Start All",
    loadingLabel: "Starting All...",
  },
  {
    action: "restart",
    id: "globalBulkRestart",
    testid: "global-bulk-restart",
    icon: "restart_alt",
    label: "Restart All",
    loadingLabel: "Restarting All...",
  },
  {
    action: "stop",
    id: "globalBulkStop",
    testid: "global-bulk-stop",
    icon: "stop",
    label: "Stop All",
    loadingLabel: "Stopping All...",
    filled: true,
  },
  {
    action: "pull",
    id: "globalBulkPull",
    testid: "global-bulk-pull",
    icon: "download",
    label: "Pull All",
    loadingLabel: "Pulling All...",
  },
];

/**
 * The ops control deck: the health KPI, the four bulk actions, the service-mesh
 * status strip and the feedback line. The markup mirrors what index.html held
 * plus what renderSummary/renderStatusStrip/syncBulkActionButtons wrote into it,
 * class for class (no look change).
 *
 * Everything here is derived, never written imperatively: the controller
 * publishes the snapshot and the feedback line into the store, and this island
 * derives the KPI, the bar tone, the label, the strip, each bulk button's
 * enabled state and the feedback tone from them (D6). The two things the vanilla
 * code did with the document - withButtonLoading's innerHTML swap plus
 * `dataset.busy`, and the feedback strip's classList.remove + rAF re-add - are
 * this island's own state and a `seq`-keyed effect.
 */
export function GlobalHealthHeader({ onBulkAction }: Props) {
  // The controller publishes the snapshot and the feedback line into the store,
  // and the island derives everything it shows from them (D6); main.js creates
  // this element once, so a prop captured at mount would freeze the deck.
  const state = useStore();
  const services = state.globalServices as GlobalServiceRow[];
  const snapshot = state.globalServicesSnapshot as GlobalServiceSnapshot | null;
  const feedback = state.globalActionFeedback;
  const [busy, setBusy] = useState<GlobalBulkAction | null>(null);
  const [ping, setPing] = useState(false);

  // The feedback line announces itself again on every raise: drop the class,
  // then re-add it on the next frame, which is what restarts the CSS animation.
  useEffect(() => {
    if (!feedback.seq) {
      return undefined;
    }
    setPing(false);
    const frame = requestAnimationFrame(() => setPing(true));
    return () => cancelAnimationFrame(frame);
  }, [feedback.seq]);

  const runBulk = async (action: GlobalBulkAction) => {
    if (busy) {
      return;
    }
    setBusy(action);
    try {
      await onBulkAction(action);
    } finally {
      setBusy(null);
    }
  };

  const health = snapshot ? deriveHealthSummary(snapshot) : null;
  const buttons = bulkActionButtonStates(snapshot ?? {});
  const tone = feedbackTone(feedback.tone);

  return (
    <>
      <div className="flex items-center justify-between gap-2">
        <p className="text-[11px] uppercase tracking-[0.18em] text-primary font-bold flex items-center gap-1.5 dark:drop-shadow-[0_1px_3px_rgba(0,0,0,0.8)]">
          <span className="material-symbols-outlined text-[14px]">space_dashboard</span>
          Ops Control Deck
        </p>
        <span className="text-[11px] uppercase tracking-[0.12em] text-text-tertiary font-bold">
          Global Services
        </span>
      </div>

      <div className="grid gap-3 xl:grid-cols-[260px_minmax(0,1fr)]">
        <section className="rounded-xl border border-border-primary bg-surface-primary/80 p-3 shadow-[0_8px_30px_rgba(0,0,0,0.25)] global-control-card">
          <p className="text-[11px] uppercase tracking-[0.16em] text-slate-600 dark:text-white/80 font-bold flex items-center gap-1.5">
            <span className="material-symbols-outlined text-[13px]">monitoring</span>
            Global Health
          </p>
          <div className="mt-2 flex items-end gap-2">
            <span
              id="globalServiceHealthPercent"
              className="text-4xl font-black text-text-primary dark:text-white leading-none ops-kpi-value"
            >
              {health ? health.percentLabel : "--%"}
            </span>
            <span id="globalServiceCount" className="text-xs font-semibold text-primary">
              {health ? health.countLabel : "0/0 running"}
            </span>
          </div>
          <div className="mt-3 h-2 rounded-full bg-slate-200 dark:bg-white/5 relative">
            <div
              id="globalServiceHealthBar"
              className={health ? health.barClass : INITIAL_BAR_CLASS}
              style={health ? { width: `${health.percent}%` } : undefined}
            ></div>
          </div>
          <div
            id="globalServiceHealthLabel"
            className={health ? health.labelClass : INITIAL_LABEL_CLASS}
          >
            <span id="globalServiceHealthLabelIcon" className="material-symbols-outlined text-[12px]">
              {health ? health.labelIcon : "monitor_heart"}
            </span>
            <span id="globalServiceHealthLabelText">
              {health ? health.labelText : "Health check pending"}
            </span>
          </div>
        </section>

        <section className="rounded-xl border border-border-primary bg-surface-secondary/75 p-3 shadow-[0_8px_24px_rgba(0,0,0,0.2)] global-control-card">
          <div className="flex items-center justify-between gap-2 mb-2">
            <p className="text-[11px] uppercase tracking-[0.16em] text-primary font-semibold flex items-center gap-1.5">
              <span className="material-symbols-outlined text-[13px]">tune</span>
              All Actions
            </p>
            <span className="text-[10px] text-text-tertiary">
              Applies to every global service
            </span>
          </div>
          <div className="grid grid-cols-2 lg:grid-cols-4 gap-2">
            {BULK_BUTTONS.map((button) => {
              const state = buttons[button.action];
              const isBusy = busy === button.action;
              return (
                <button
                  key={button.action}
                  type="button"
                  id={button.id}
                  data-testid={button.testid}
                  data-loading-label={button.loadingLabel}
                  className={state.className}
                  aria-busy={isBusy ? "true" : undefined}
                  disabled={!state.enabled || isBusy}
                  onClick={() => void runBulk(button.action)}
                >
                  {isBusy ? (
                    <>
                      <span className="material-symbols-outlined text-[18px] animate-spin">
                        progress_activity
                      </span>
                      {button.loadingLabel}
                    </>
                  ) : (
                    <>
                      <span
                        className={`material-symbols-outlined text-[16px]${button.filled ? " fill-1" : ""}`}
                        style={button.filled ? { fontVariationSettings: '"FILL" 1' } : undefined}
                      >
                        {button.icon}
                      </span>
                      {button.label}
                    </>
                  )}
                </button>
              );
            })}
          </div>
        </section>
      </div>

      <section className="rounded-xl border border-border-primary bg-background-secondary/75 p-2.5 global-control-card">
        <div className="flex items-center gap-1.5 mb-2">
          <span className="material-symbols-outlined text-[14px] text-primary">lan</span>
          <p className="text-[11px] uppercase tracking-[0.12em] text-primary font-bold">
            Service Mesh Snapshot
          </p>
        </div>
        <div id="globalServiceStatusStrip" className="flex flex-wrap gap-1.5">
          {services.length === 0 ? (
            <span className="inline-flex items-center gap-1 rounded-md border border-border-primary bg-background-secondary px-2 py-1 text-[10px] text-text-tertiary">
              Loading services...
            </span>
          ) : (
            services.map((service, index) => {
              const stripTone = statusStripTone(service);
              return (
                <span
                  key={service.id}
                  className={`global-status-chip inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-[10px] font-medium shadow-xs ${stripTone.chip}`}
                  style={{ "--chip-order": index } as CSSProperties}
                >
                  <span className={`w-1.5 h-1.5 rounded-full ${stripTone.dot}`}></span>
                  <span className="material-symbols-outlined text-[11px] leading-none opacity-90">
                    {globalServiceIcon(service.id)}
                  </span>
                  <span className="text-text-primary/95">{service.name}</span>
                  <span className="opacity-80">{formatStatusLabel(service.status)}</span>
                </span>
              );
            })
          )}
        </div>
      </section>

      <section
        id="globalActionFeedback"
        className={`${tone.textClass}${ping ? " global-feedback-ping" : ""}`}
      >
        <span id="globalActionFeedbackIcon" className={tone.iconClass}>
          {tone.icon}
        </span>
        <span
          id="globalActionFeedbackText"
          className="min-w-0 whitespace-pre-line leading-relaxed"
        >
          {feedback.message || "Ready for global operations."}
        </span>
      </section>
    </>
  );
}
