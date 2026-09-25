import { projectKey } from "../modules/dashboard.js";
import { useStore } from "../state/useStore.js";

type Props = {
  onCopy(text: string): void;
};

/**
 * The Environment Variables card. It reads the shared store (spec D6) and keeps
 * the static "No variables loaded" placeholder for the frame before the first
 * dashboard refresh. The markup mirrors renderEnvVars element for element and
 * class for class, minus the data-action the copy row used to carry (spec D5).
 */
export function EnvVars({ onCopy }: Props) {
  const state = useStore();
  const environments = Array.isArray(state.environments) ? state.environments : [];
  const env = environments.find((item) => projectKey(item) === state.selectedProject) as
    | Record<string, unknown>
    | undefined;

  if (!env) {
    return <div className="text-xs text-slate-500 italic">No variables loaded</div>;
  }

  const envVars = (env.EnvVars || env.envVars || {}) as Record<string, unknown>;
  const keys = Object.keys(envVars);

  if (keys.length === 0) {
    return (
      <div className="text-xs text-text-tertiary italic">No environment variables defined</div>
    );
  }

  return (
    <>
      {keys.map((key) => {
        const value = String(envVars[key] ?? "");
        return (
          <div
            key={key}
            data-testid="env-var-row"
            data-text={value}
            className="flex justify-between items-center group cursor-pointer hover:bg-background-secondary/50 p-1.5 -mx-1.5 rounded-sm transition-colors"
            title="Click to copy"
            onClick={() => onCopy(value)}
          >
            <span className="text-xs text-emerald-700 dark:text-primary font-mono font-bold">{key}</span>
            <span className="text-xs text-slate-800 dark:text-white font-mono bg-surface-secondary px-2 py-0.5 rounded-sm border border-border-primary break-all max-w-[60%] font-medium">
              {value}
            </span>
          </div>
        );
      })}
    </>
  );
}
