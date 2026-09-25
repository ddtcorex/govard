import { useState } from "react";
import {
  formatStatusLabel,
  globalServiceIcon,
  hasRoutingImpact,
  isServiceActive,
  statusChipClass,
} from "../modules/global-services.js";
import { useStore } from "../state/useStore.js";

export type GlobalServiceRow = {
  id: string;
  name: string;
  composeService: string;
  containerName: string;
  status: string;
  state: string;
  health: string;
  statusText: string;
  running: boolean;
  openable: boolean;
  url: string;
};

export type GlobalServiceAction = "start" | "stop" | "restart" | "open";

type Props = {
  onSelectService(serviceId: string): void;
  onServiceAction(action: GlobalServiceAction, serviceId: string): Promise<void>;
};

const LOADING_GLYPH_CLASS =
  "material-symbols-outlined text-[18px] animate-spin";

/**
 * The global-services card list. The markup mirrors renderServiceCard element
 * for element and class for class (no look change), minus the data-action
 * attributes the global delegate resolved (D5).
 *
 * The two things the vanilla code did through the document are React state here:
 * withButtonLoading swapped the clicked button's innerHTML and read
 * `button.dataset.busy` to ignore a double click, so a card whose button was
 * mid-action could be re-rendered out from under it; and the card's action
 * buttons relied on the delegate's `closest("[data-action]")` walk up to the
 * button rather than the card, which is an onClick + stopPropagation now.
 */
export function GlobalServicesList({
  onSelectService,
  onServiceAction,
}: Props) {
  // The list is published to the store by the controller's refresh, so the
  // island reads it there rather than taking it as a prop: main.js creates this
  // element once, and a prop captured at mount would freeze the list (D6).
  const state = useStore();
  const services = state.globalServices as GlobalServiceRow[];
  const selectedService = String(state.selectedGlobalService || "");
  const loading = Boolean(state.globalServicesLoading);
  const error = String(state.globalServicesError || "");
  /** serviceId -> the action its button is running. */
  const [busy, setBusy] = useState<Record<string, GlobalServiceAction>>({});

  const runAction = async (
    event: { stopPropagation(): void },
    serviceId: string,
    action: GlobalServiceAction,
  ) => {
    // The card itself selects the service; a button inside it must not.
    event.stopPropagation();
    if (busy[serviceId]) {
      return;
    }
    setBusy((prev) => ({ ...prev, [serviceId]: action }));
    try {
      await onServiceAction(action, serviceId);
    } finally {
      setBusy((prev) => {
        const next = { ...prev };
        delete next[serviceId];
        return next;
      });
    }
  };

  const renderGlyph = (serviceId: string, glyph: string) =>
    busy[serviceId] ? (
      <span className={LOADING_GLYPH_CLASS}>progress_activity</span>
    ) : (
      <span className="material-symbols-outlined text-[18px]">{glyph}</span>
    );

  if (error) {
    return (
      <div className="rounded-xl border border-dashed border-red-500/40 bg-red-500/10 p-4 text-sm text-red-300">
        {error}
      </div>
    );
  }
  if (loading) {
    return (
      <div className="rounded-xl border border-dashed border-border-primary bg-surface-secondary p-4 text-sm text-slate-400">
        Loading global services...
      </div>
    );
  }
  if (services.length === 0) {
    return (
      <div className="rounded-xl border border-dashed border-border-primary bg-background-secondary p-4 text-sm text-text-tertiary">
        Global services data unavailable.
      </div>
    );
  }

  return (
    <>
      {services.map((service) => {
        const selected = service.id === selectedService;
        const isActive = isServiceActive(service);
        const primaryAction: GlobalServiceAction = isActive ? "restart" : "start";
        const primaryLabel = isActive ? "Restart" : "Start";
        const primaryIcon = isActive ? "restart_alt" : "play_arrow";
        const rowClass = selected
          ? "bg-primary/5 dark:bg-primary/10 border border-primary/40 shadow-[0_0_0_1px_rgba(13,242,89,0.25)]"
          : "bg-white dark:bg-transparent border border-slate-200 dark:border-border-primary hover:border-primary/30 hover:bg-slate-50 dark:hover:bg-background-secondary/20";

        return (
          <article
            key={service.id}
            data-testid="global-service-card"
            data-service={service.id}
            className={`rounded-xl border ${rowClass} p-3 transition-all cursor-pointer`}
            title={`Select ${service.name} logs`}
            onClick={() => onSelectService(service.id)}
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="material-symbols-outlined text-primary text-[18px]">
                    {globalServiceIcon(service.id)}
                  </span>
                  <h4 className="text-sm font-semibold text-slate-900 dark:text-white truncate">
                    {service.name}
                  </h4>
                </div>
                <p className="text-[11px] text-slate-600 dark:text-slate-400 mt-1 truncate font-medium">
                  {service.containerName}
                </p>
              </div>
              <span
                className={`px-2 py-1 rounded-md border text-[10px] font-bold uppercase tracking-wide shrink-0 ${statusChipClass(service.status)}`}
              >
                {formatStatusLabel(service.status)}
              </span>
            </div>
            {hasRoutingImpact(service) ? (
              <div className="mt-2 rounded-md border border-amber-500/40 bg-amber-500/10 px-2 py-1.5 text-[10px] text-amber-700 dark:text-amber-300 font-medium flex items-start gap-1.5">
                <span className="material-symbols-outlined text-[13px] leading-none mt-px">
                  warning
                </span>
                <span>
                  Routing warning: {service.name} is stopped. Proxy/domain routing
                  may fail.
                </span>
              </div>
            ) : null}
            <div className="mt-3 flex items-center gap-2">
              <button
                type="button"
                data-testid="global-service-primary"
                data-service={service.id}
                data-operation={primaryAction}
                data-loading-label={primaryAction === "restart" ? "Restarting..." : "Starting..."}
                data-loading-icon-only="true"
                className="h-8 w-8 rounded-lg bg-primary text-background-secondary hover:bg-primary-hover transition-all active:scale-95 flex items-center justify-center disabled:opacity-70 disabled:cursor-not-allowed disabled:active:scale-100 shadow-xs"
                title={primaryLabel}
                aria-busy={busy[service.id] ? "true" : undefined}
                disabled={Boolean(busy[service.id])}
                onClick={(event) => void runAction(event, service.id, primaryAction)}
              >
                {renderGlyph(service.id, primaryIcon)}
              </button>
              <button
                type="button"
                data-testid="global-service-stop"
                data-service={service.id}
                data-loading-label="Stopping..."
                data-loading-icon-only="true"
                className={
                  isActive
                    ? "h-8 w-8 rounded-lg bg-red-600 text-white border border-red-500 hover:bg-red-500 transition-all active:scale-95 flex items-center justify-center disabled:opacity-70 disabled:cursor-not-allowed disabled:active:scale-100 shadow-xs"
                    : "h-8 w-8 rounded-lg bg-background-secondary text-slate-500 dark:text-text-tertiary border border-border-primary opacity-90 dark:opacity-60 flex items-center justify-center"
                }
                title="Stop"
                aria-busy={busy[service.id] ? "true" : undefined}
                disabled={!isActive || Boolean(busy[service.id])}
                onClick={(event) => void runAction(event, service.id, "stop")}
              >
                {busy[service.id] ? (
                  <span className={LOADING_GLYPH_CLASS}>progress_activity</span>
                ) : (
                  <span
                    className="material-symbols-outlined text-[18px] fill-1"
                    style={{ fontVariationSettings: '"FILL" 1' }}
                  >
                    stop
                  </span>
                )}
              </button>
              <button
                type="button"
                data-testid="global-service-open"
                data-service={service.id}
                data-loading-label="Opening..."
                data-loading-icon-only="true"
                className={
                  service.openable
                    ? "h-8 w-8 rounded-lg bg-slate-100 dark:bg-white/10 text-slate-600 dark:text-slate-300 border border-slate-200 dark:border-white/20 hover:bg-slate-200 dark:hover:bg-white/20 hover:text-primary dark:hover:text-white transition-all active:scale-95 flex items-center justify-center disabled:opacity-70 disabled:cursor-not-allowed disabled:active:scale-100 shadow-xs"
                    : "h-8 w-8 rounded-lg border border-border-primary text-slate-500 dark:text-text-tertiary bg-background-secondary opacity-90 dark:opacity-60 flex items-center justify-center"
                }
                title="Open"
                aria-busy={busy[service.id] ? "true" : undefined}
                disabled={!service.openable || Boolean(busy[service.id])}
                onClick={(event) => void runAction(event, service.id, "open")}
              >
                {busy[service.id] ? (
                  <span className={LOADING_GLYPH_CLASS}>progress_activity</span>
                ) : (
                  <span className="material-symbols-outlined text-[20px]">open_in_new</span>
                )}
              </button>
            </div>
          </article>
        );
      })}
    </>
  );
}
