import { inferServiceTarget, projectKey } from "../modules/dashboard.js";
import { useStore } from "../state/useStore.js";

type Env = Record<string, unknown>;
type Props = {
  onOpenLogs(project: string, service: string): void;
  onOpenTerminal(project: string, service: string): void;
};

const serviceGlyph = (name: string) => {
  if (name.includes("php")) {
    return {
      icon: "php",
      wrap: "bg-indigo-500/10 text-indigo-400 border-indigo-500/20",
    };
  }
  if (name.includes("mysql") || name.includes("db") || name.includes("maria")) {
    return {
      icon: "database",
      wrap: "bg-yellow-500/10 text-yellow-400 border-yellow-500/20",
    };
  }
  if (name.includes("nginx") || name.includes("proxy") || name.includes("web")) {
    return {
      icon: "language",
      wrap: "bg-green-500/10 text-green-400 border-green-500/20",
    };
  }
  return { icon: "bolt", wrap: "bg-blue-500/10 text-blue-400 border-blue-500/20" };
};

/**
 * The Active Services card. It reads the shared store (spec D6) and keeps the
 * static "Loading services..." placeholder for the frame before the first
 * dashboard refresh, so the boot frame is unchanged. The markup mirrors
 * renderActiveServices element for element and class for class, minus the
 * data-action attributes its two buttons used to carry (spec D5).
 */
export function ActiveServices({ onOpenLogs, onOpenTerminal }: Props) {
  const state = useStore();
  const environments = Array.isArray(state.environments) ? state.environments : [];
  const env = environments.find((item) => projectKey(item) === state.selectedProject) as
    | Env
    | undefined;

  if (!env) {
    return (
      <div className="h-16 border border-border-primary border-dashed rounded-xl flex items-center justify-center text-slate-500 text-sm">
        Loading services...
      </div>
    );
  }

  const project = projectKey(env);
  const services = Array.isArray(env.Services)
    ? (env.Services as Record<string, unknown>[])
    : Array.isArray(env.services)
      ? (env.services as Record<string, unknown>[])
      : [];

  if (services.length === 0) {
    return (
      <div className="p-6 text-center text-text-tertiary border border-dashed border-border-primary rounded-xl bg-surface-primary/30">
        <span className="material-symbols-outlined text-3xl mb-2 opacity-20">inventory_2</span>
        <div className="text-sm italic">No active services detected</div>
      </div>
    );
  }

  return (
    <>
      {services.map((service) => {
        const status = String(service.Status || service.status || "stopped").toLowerCase();
        const isHealthy = status === "healthy" || status === "running" || status === "up";
        const statusColor = isHealthy ? "text-green-400" : "text-amber-400";
        const name = String(service.Name || service.name || "unknown").toLowerCase();
        const glyph = serviceGlyph(name);
        const serviceTarget = inferServiceTarget(service);
        const label = String(service.Name || service.name || "Service");
        const port = String(service.Port || service.port || "N/A");
        const statusText = String(service.Status || service.status || "Unknown");
        const buttonClass =
          "size-8 rounded-sm flex items-center justify-center bg-slate-100 dark:bg-(--surface-secondary) border border-slate-200 dark:border-border-primary text-slate-500 dark:text-slate-300 hover:text-primary dark:hover:text-white hover:bg-slate-200 dark:hover:bg-background-secondary transition-all";

        return (
          <div
            key={`${label}-${serviceTarget}`}
            data-testid="service-card"
            className="glass-panel p-4 rounded-xl border border-slate-200 dark:border-border-primary hover:border-primary/30 transition-all flex items-center justify-between h-[72px]"
          >
            <div className="flex items-center gap-4 h-full">
              <div
                className={`w-10 h-10 shrink-0 rounded-sm flex items-center justify-center ${glyph.wrap} border`}
              >
                <span className="material-symbols-outlined text-[20px]">{glyph.icon}</span>
              </div>
              <div className="flex flex-col justify-center">
                <h4 className="text-slate-800 dark:text-white font-medium text-sm leading-tight">{label}</h4>
                <div className="flex items-center gap-2 text-xs mt-1 leading-none">
                  <span className="text-slate-500 dark:text-slate-400">Port: {port}</span>
                  <span className="w-1 h-1 rounded-full bg-slate-300 dark:bg-slate-600"></span>
                  <span className={statusColor}>{statusText}</span>
                </div>
              </div>
            </div>
            <div className="flex items-center gap-2 h-full">
              <button
                type="button"
                data-testid="service-logs"
                className={buttonClass}
                title="View Logs"
                onClick={() => onOpenLogs(project, serviceTarget)}
              >
                <span className="material-symbols-outlined text-[18px]">list_alt</span>
              </button>
              <button
                type="button"
                data-testid="service-terminal"
                className={buttonClass}
                title="Open OS Terminal"
                onClick={() => onOpenTerminal(project, serviceTarget)}
              >
                <span className="material-symbols-outlined text-[18px]">terminal</span>
              </button>
            </div>
          </div>
        );
      })}
    </>
  );
}
