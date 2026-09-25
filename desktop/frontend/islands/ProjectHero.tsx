import { domainLabel, localEnvironmentURL, projectKey } from "../modules/dashboard.js";
import { useStore } from "../state/useStore.js";

type Env = Record<string, unknown>;
type Props = {
  onAction(action: string, project: string): void;
};

const STATUS_BADGE_BASE =
  "px-3 py-1 rounded-full border text-xs font-bold uppercase tracking-wide neon-glow flex items-center gap-1.5";

const statusTone = (status: string) => {
  if (status === "running") return "bg-primary/20 border-primary/30 text-primary";
  if (status === "warning") return "bg-amber-500/20 border-amber-500/30 text-amber-500";
  return "bg-slate-500/10 border-slate-500/20 text-slate-600 dark:text-slate-400";
};

const dotTone = (status: string) => {
  if (status === "running") {
    return "bg-primary shadow-[0_0_12px_rgba(13,242,89,0.9)] animate-pulse";
  }
  if (status === "warning") return "bg-amber-500";
  return "bg-slate-500";
};

/** The technology dot palette renderProjectHero picked by name substring. */
const technologyTone = (tech: string) => {
  const name = String(tech || "").toLowerCase();
  if (name.includes("mysql") || name.includes("maria")) {
    return { color: "bg-yellow-500", shadow: "rgba(234, 179, 8, 0.5)" };
  }
  if (name.includes("redis") || name.includes("cache")) {
    return { color: "bg-red-500", shadow: "rgba(239, 68, 68, 0.5)" };
  }
  if (name.includes("python")) {
    return { color: "bg-green-600", shadow: "rgba(22, 163, 74, 0.5)" };
  }
  if (name.includes("node")) {
    return { color: "bg-green-500", shadow: "rgba(34, 197, 94, 0.5)" };
  }
  return { color: "bg-blue-500", shadow: "rgba(59, 130, 246, 0.5)" };
};

/**
 * The project hero. It reads the shared store (spec D6) so a dashboard refresh
 * reaches it without main.js re-rendering anything. The markup mirrors the
 * static section it replaces, element for element and class for class, and the
 * dynamic parts - status badge tone, the status dot, the restart button's label
 * and glyph, the stop button's disabled look, the git-branch badge and the
 * technology dots - are the same computations renderProjectHero performed, so
 * the runtime data-action assignment becomes a plain onClick (spec D5).
 */
export function ProjectHero({ onAction }: Props) {
  const state = useStore();
  const environments = Array.isArray(state.environments) ? state.environments : [];
  const env = (environments.find(
    (item: Env) => projectKey(item) === state.selectedProject,
  ) || null) as Env | null;
  const project = env ? projectKey(env) : String(state.selectedProject || "");

  if (!env) {
    return (
      <div className="relative rounded-2xl overflow-hidden group border border-slate-200 dark:border-border-primary w-full">
        <HeroBackdrop />
        <div className="relative z-20 px-8 py-6 flex flex-col md:flex-row md:items-stretch justify-between gap-6">
          <div className="flex items-start gap-6 flex-1 min-w-0">
            <HeroGlyph />
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-3 mb-2">
                <h2 id="projectTitle" className="text-2xl font-bold text-slate-900 dark:text-white tracking-tight">
                  Loading...
                </h2>
              </div>
              <a
                id="projectUrl"
                href="#"
                target="_blank"
                className="mb-2 text-emerald-700 dark:text-primary hover:text-emerald-800 dark:hover:text-primary/80 transition-colors text-sm font-bold flex items-center gap-1 leading-none hidden"
              >
                <span id="projectUrlText"></span>
                <span className="material-symbols-outlined text-[16px] transition-colors">open_in_new</span>
              </a>
              <div
                id="projectTechnologies"
                className="flex flex-wrap gap-2 text-xs text-text-secondary dark:text-slate-400 min-h-[26px]"
              ></div>
            </div>
          </div>
        </div>
      </div>
    );
  }

  const title = domainLabel(env);
  const status = String(env.Status || env.status || "stopped").toLowerCase();
  const url = localEnvironmentURL(env);
  const gitBranch = String(env.GitBranch || env.gitBranch || "");
  const techs = Array.isArray(env.Technologies)
    ? (env.Technologies as string[])
    : Array.isArray(env.technologies)
      ? (env.technologies as string[])
      : [];
  const isStopped = status !== "running" && status !== "warning";
  const restartLabel = isStopped ? "Start" : "Restart";
  const restartIcon = isStopped ? "play_arrow" : "restart_alt";

  return (
    <div className="relative rounded-2xl overflow-hidden group border border-slate-200 dark:border-border-primary w-full">
      <HeroBackdrop />
      <div className="relative z-20 px-8 py-6 flex flex-col md:flex-row md:items-stretch justify-between gap-6">
        <div className="flex items-start gap-6 flex-1 min-w-0">
          <HeroGlyph />
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-3 mb-2">
              <h2 id="projectTitle" className="text-2xl font-bold text-slate-900 dark:text-white tracking-tight">
                {title}
              </h2>
            </div>
            <a
              id="projectUrl"
              href={url || "#"}
              target="_blank"
              className={`mb-2 text-emerald-700 dark:text-primary hover:text-emerald-800 dark:hover:text-primary/80 transition-colors text-sm font-bold flex items-center gap-1 leading-none${url ? "" : " hidden"}`}
              onClick={(event) => {
                if (!url) return;
                event.preventDefault();
                onAction("env-open", project);
              }}
            >
              <span id="projectUrlText">{url}</span>
              <span className="material-symbols-outlined text-[16px] transition-colors">open_in_new</span>
            </a>
            <div
              id="projectTechnologies"
              className="flex flex-wrap gap-2 text-xs text-text-secondary dark:text-slate-400 min-h-[26px]"
            >
              {techs.map((tech) => {
                const tone = technologyTone(tech);
                return (
                  <span
                    key={tech}
                    className="flex items-center gap-1.5 bg-slate-100 dark:bg-surface-secondary px-2 py-0.5 rounded-sm border border-slate-300 dark:border-border-primary"
                  >
                    <span
                      className={`w-1.5 h-1.5 rounded-full ${tone.color}`}
                      style={{ boxShadow: `0 0 8px ${tone.shadow}` }}
                    ></span>
                    <span className="text-[11px] font-black text-slate-800 dark:text-slate-200">{tech}</span>
                  </span>
                );
              })}
            </div>
          </div>
        </div>
        <div className="flex flex-col items-end justify-between gap-4">
          <div className="flex items-center gap-3">
            <span
              id="projectGitBranchBadge"
              className={`max-w-[200px] px-3 py-1 rounded-full bg-slate-500/10 dark:bg-slate-500/20 border border-slate-500/30 text-text-secondary dark:text-slate-400 text-xs font-bold tracking-wide flex items-center gap-1.5${gitBranch ? "" : " hidden"}`}
              title={gitBranch ? `Git Branch: ${gitBranch}` : "Current Git Branch"}
            >
              <span className="material-symbols-outlined text-[14px]">call_split</span>
              <span id="projectGitBranchText" className="font-mono truncate">
                {gitBranch}
              </span>
            </span>
            <span
              id="projectStatusBadge"
              className={`${STATUS_BADGE_BASE} ${statusTone(status)}`}
            >
              <span data-role="project-status-dot" className={`w-2 h-2 rounded-full ${dotTone(status)}`}></span>
              <span id="projectStatusText">
                {status.charAt(0).toUpperCase() + status.slice(1)}
              </span>
            </span>
          </div>
          <div className="flex items-center gap-3">
            <button
              id="heroRestartBtn"
              type="button"
              data-testid="hero-restart"
              data-env={project}
              className="h-12 px-6 bg-primary text-slate-900 rounded-lg text-sm font-bold hover:bg-primary/90 transition-all flex items-center gap-2 shadow-lg shadow-primary/10 active:scale-95"
              title={`${restartLabel} Environment`}
              onClick={() => onAction(isStopped ? "env-start" : "env-restart", project)}
            >
              <span className="material-symbols-outlined text-lg">{restartIcon}</span>
              {restartLabel}
            </button>
            <button
              id="heroStopBtn"
              type="button"
              data-testid="hero-stop"
              data-env={project}
              className={
                isStopped
                  ? "h-12 w-12 bg-slate-100 dark:bg-(--surface-secondary) text-slate-400 dark:text-slate-500 border border-slate-200 dark:border-border-primary rounded-lg transition-all flex items-center justify-center cursor-not-allowed opacity-70"
                  : "h-12 w-12 bg-red-600 text-white border border-red-500 rounded-lg hover:bg-red-500 transition-all active:scale-95 flex items-center justify-center shadow-lg shadow-red-500/20"
              }
              disabled={isStopped}
              title={isStopped ? "Environment is not running" : "Stop Environment"}
              onClick={() => onAction("env-stop", project)}
            >
              <span className="material-symbols-outlined fill-1" style={{ fontVariationSettings: '"FILL" 1' }}>
                stop
              </span>
            </button>
            <button
              id="heroPullBtn"
              type="button"
              data-testid="hero-pull"
              data-env={project}
              className="h-12 w-12 bg-surface-secondary text-primary border border-border-primary rounded-lg hover:bg-surface-primary hover:text-primary transition-all active:scale-95 flex items-center justify-center shadow-lg shadow-primary/10"
              title="Pull Images"
              onClick={() => onAction("env-pull", project)}
            >
              <span className="material-symbols-outlined">download</span>
            </button>
            <button
              id="heroDeleteBtn"
              type="button"
              data-testid="hero-delete"
              data-env={project}
              className="h-12 w-12 bg-surface-secondary text-red-500 border border-red-500/30 rounded-lg hover:bg-red-500 hover:text-white transition-all active:scale-95 flex items-center justify-center shadow-lg shadow-red-500/10"
              title={`Delete ${title}`}
              onClick={() => onAction("env-delete", project)}
            >
              <span className="material-symbols-outlined">delete</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

function HeroBackdrop() {
  return (
    <div className="absolute inset-0 z-0">
      <div className="absolute inset-0 bg-linear-to-t from-surface-secondary via-surface-secondary/80 to-surface-secondary/30 z-10"></div>
      <div className="absolute inset-0 opacity-30 overflow-hidden">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_50%_50%,rgba(var(--primary-rgb),0.07)_0%,transparent_70%)]"></div>
        <div className="absolute inset-0 bg-[linear-gradient(45deg,var(--bg-primary)_25%,transparent_25%,transparent_50%,var(--bg-primary)_50%,var(--bg-primary)_75%,transparent_75%,transparent)] bg-size-[48px_48px]"></div>
        <div className="absolute inset-0 bg-[repeating-linear-gradient(0deg,transparent,transparent_1px,#0df25905_1px,#0df25905_2px)] bg-size-[100%_4px]"></div>
      </div>
    </div>
  );
}

function HeroGlyph() {
  return (
    <div className="h-24 w-24 rounded-xl bg-surface-primary dark:bg-white/5 backdrop-blur-md border border-border-primary dark:border-white/10 flex items-center justify-center shadow-2xl shrink-0 group-hover:scale-105 transition-transform duration-500">
      <span
        className="material-symbols-outlined text-primary drop-shadow-[0_0_15px_rgba(var(--primary-rgb),0.3)]"
        style={{ fontSize: "48px" }}
      >
        layers
      </span>
    </div>
  );
}
