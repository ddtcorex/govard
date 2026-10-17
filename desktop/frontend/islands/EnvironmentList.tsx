import {
  classifyEnvironmentStatus,
  domainLabel,
  projectKey,
} from "../modules/dashboard.js";
import { useStore } from "../state/useStore.js";

type Env = Record<string, unknown>;
type Props = {
  onSelect(project: string): void;
  onToggle(project: string): void;
  onSwitchSidebarMode(): void;
  onRetry(): void;
  onOpenOnboarding(): void;
};

const itemClass = (isSelected: boolean, active: boolean) =>
  `group grid w-full items-center gap-x-2 py-2.5 px-3 rounded-lg text-left cursor-pointer relative overflow-hidden transition-all ${
    isSelected
      ? "active-env bg-primary/10 border border-primary/20 text-text-primary shadow-[0_2px_8px_rgba(var(--primary-rgb),0.08)]"
      : "text-text-tertiary dark:text-slate-200 hover:bg-background-primary/80 dark:hover:text-white"
  } ${!active ? "dark:opacity-50" : "opacity-100"}`;

function EnvironmentItem({
  env,
  isSelected,
  isBusy,
  busyLabel,
  onSelect,
  onToggle,
}: {
  env: Env;
  isSelected: boolean;
  isBusy: boolean;
  busyLabel: string;
  onSelect(project: string): void;
  onToggle(project: string): void;
}) {
  const key = projectKey(env);
  const domain = domainLabel(env);
  const meta = classifyEnvironmentStatus(env);
  const titleClass = meta.active ? "text-text-primary" : "text-text-secondary";

  return (
    <div
      data-testid="env-card"
      data-env={key}
      className={itemClass(isSelected, meta.active)}
      style={{ gridTemplateColumns: "24px minmax(0,1fr)" }}
      // Mouse-only convenience: clicking anywhere on the row selects the
      // project (the behaviour suite clicks the card itself). Keyboard and
      // screen-reader users get the two native sibling buttons below, one
      // tab stop each, so this container stays a plain layout element.
      onClick={() => {
        if (!isBusy) onSelect(key);
      }}
    >
      {isSelected ? (
        <div className="absolute inset-y-0 left-0 w-1 bg-primary"></div>
      ) : null}
      <div className="relative flex h-6 w-6 items-center justify-center self-center z-10">
        {/* Start/stop is its own button next to the select button, not a
            control nested inside it: nested interactives are a double tab
            stop and invalid HTML. While the env lock is held the toggle is
            replaced by a spinner, so a double-click during start/stop is
            impossible instead of a status-line surprise. */}
        {isBusy ? (
          <span
            data-testid="env-busy-spinner"
            data-env={key}
            className="material-symbols-outlined leading-none text-[20px] animate-spin"
            aria-hidden="true"
            title={`${busyLabel} in progress...`}
          >
            sync
          </span>
        ) : (
          <button
            type="button"
            data-testid="toggle-env"
            data-env={key}
            aria-label={`${meta.active ? "Stop" : "Start"} ${domain}`}
            title={`${meta.active ? "Stop" : "Start"} ${domain}`}
            className="flex h-6 w-6 items-center justify-center rounded-md cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40"
            onClick={(event) => {
              event.stopPropagation();
              onToggle(key);
            }}
          >
            <span
              aria-hidden="true"
              className={`material-symbols-outlined leading-none ${meta.iconClass} transition-colors hover:text-slate-900 dark:hover:text-white text-[20px]`}
              style={meta.fill ? { fontVariationSettings: "'FILL' 1" } : undefined}
            >
              {meta.iconName}
            </span>
          </button>
        )}
        {meta.showPulseDot ? (
          <span className="absolute -top-0.5 -right-0.5 w-2 h-2 rounded-full bg-primary border border-(--bg-primary) animate-pulse"></span>
        ) : null}
      </div>
      <button
        type="button"
        data-testid="env-select"
        data-env={key}
        className="min-w-0 flex flex-col justify-center text-left bg-transparent cursor-pointer rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40"
        title={isBusy ? `${busyLabel} in progress...` : `Select ${domain}`}
        disabled={isBusy}
        aria-disabled={isBusy || undefined}
        onClick={(event) => {
          event.stopPropagation();
          onSelect(key);
        }}
      >
        <div className={`text-sm font-semibold leading-tight truncate ${titleClass}`}>{domain}</div>
        <div
          className={`text-[11px] ${meta.active ? meta.detailClass : "text-text-tertiary"} flex items-center gap-1 font-medium`}
        >
          <span className={`w-1 h-1 rounded-full ${meta.dotClass}`}></span>
          <span>{isBusy ? `${busyLabel}...` : meta.detailText}</span>
        </div>
      </button>
    </div>
  );
}

function Group({
  title,
  items,
  empty,
  mt,
  onSelect,
  onToggle,
  selectedProject,
  sidebarMode,
  busyEnvs,
}: {
  title: string;
  items: Env[];
  empty: string;
  mt?: string;
  onSelect(project: string): void;
  onToggle(project: string): void;
  selectedProject: string;
  sidebarMode: string;
  busyEnvs: Record<string, string>;
}) {
  return (
    <>
      <div
        className={`px-1 ${mt || "mt-6"} pb-2 text-[10px] font-bold text-primary/70 uppercase tracking-[0.12em]`}
      >
        {title}
      </div>
      {items.length ? (
        items.map((env) => {
          const key = projectKey(env);
          const busyLabel = String(busyEnvs[key] || "");
          return (
            <EnvironmentItem
              key={key}
              env={env}
              isSelected={sidebarMode === "environments" && key === selectedProject}
              isBusy={busyLabel !== ""}
              busyLabel={busyLabel}
              onSelect={onSelect}
              onToggle={onToggle}
            />
          );
        })
      ) : (
        <div className="px-3 py-2 text-xs text-text-tertiary italic">{empty}</div>
      )}
    </>
  );
}

/** The sidebar list's loading frame, ported from renderEnvironmentSkeletons. */
function Skeletons() {
  const row = (
    <div className="w-full mb-1 flex items-center gap-3 px-3 py-2.5 rounded-lg border border-transparent">
      <div className="h-5 w-5 rounded-full skeleton"></div>
      <div className="flex-1 space-y-2">
        <div className="h-3 w-24 skeleton"></div>
        <div className="h-2 w-12 skeleton"></div>
      </div>
    </div>
  );
  return (
    <>
      <div className="w-full mt-3 mb-4 flex items-center gap-3 px-3 py-3 rounded-xl border border-border-primary bg-background-secondary">
        <div className="h-8 w-8 rounded-lg skeleton"></div>
        <div className="flex-1 space-y-2">
          <div className="h-3 w-28 skeleton"></div>
          <div className="h-2 w-20 skeleton"></div>
        </div>
      </div>
      <div className="px-1 mt-4 pb-4 text-[10px] font-semibold text-primary/80 uppercase tracking-[0.12em]">
        Active Environments
      </div>
      {row}
      {row}
      {row}
      <div className="px-1 mt-4 pb-4 text-[10px] font-semibold text-primary/80 uppercase tracking-[0.12em]">
        Inactive Environments
      </div>
      {row}
      {row}
      {row}
    </>
  );
}

/**
 * The sidebar's environment list. It reads the shared store (spec D6) rather
 * than taking environments as props, because main.js re-reads the dashboard on
 * every refresh and a prop captured at mount would freeze the list. The markup
 * mirrors renderEnvironmentList, element for element and class for class (no
 * look change), minus the data-action attributes - the island owns the card
 * click, the status glyph click and the global-services row (spec D5).
 */
export function EnvironmentList({
  onSelect,
  onToggle,
  onSwitchSidebarMode,
  onRetry,
  onOpenOnboarding,
}: Props) {
  const state = useStore();

  // The loading frame is store state, like the list itself. It used to be local
  // state that main.js closed through a registrar the island handed over from
  // its effect, but main.js can publish the dashboard before React runs that
  // effect (measured on a production boot: ~540 ms against ~640 ms), and the
  // hide then landed on the placeholder and left the skeleton up for good.
  // Reading the flag from the store makes the mount order irrelevant.
  if (state.envListLoading) return <Skeletons />;

  const environments = Array.isArray(state.environments) ? state.environments : [];
  const dashboardError = String(state.dashboardError || "");
  const isEmpty = environments.length === 0;
  // Project-scoped env locks, published by modules/operations.js: the row for
  // a busy project disables with a spinner while other projects stay enabled.
  const busyEnvs = (state.busyEnvs || {}) as Record<string, string>;

  const sidebarMode =
    state.sidebarMode === "global-services" ? "global-services" : "environments";
  const globalSelected = sidebarMode === "global-services";
  const globalClass = globalSelected
    ? "w-full mt-3 mb-4 text-left p-3 rounded-xl bg-primary/10 border-l-4 border-primary border border-primary/25 transition-all relative overflow-hidden shadow-[0_0_16px_var(--primary-glow)]"
    : "w-full mt-3 mb-4 text-left p-3 rounded-xl bg-background-secondary border border-border-primary hover:bg-background-primary hover:border-primary/20 transition-all relative overflow-hidden group";
  const globalIconWrapClass = globalSelected
    ? "h-9 w-9 rounded-lg bg-primary/20 border border-primary/30 flex items-center justify-center text-primary"
    : "h-9 w-9 rounded-lg bg-background-primary border border-border-primary flex items-center justify-center text-text-tertiary group-hover:text-primary transition-colors";
  const globalTitleClass = globalSelected
    ? "text-text-primary text-sm font-semibold truncate w-full text-left"
    : "text-text-secondary text-sm font-semibold truncate w-full text-left group-hover:text-primary transition-colors";
  const globalSubtitleClass = globalSelected
    ? "text-xs text-text-secondary"
    : "text-xs text-text-tertiary group-hover:text-text-secondary transition-colors";

  const active: Env[] = [];
  const inactive: Env[] = [];
  const seen = new Set<string>();
  environments.forEach((env) => {
    const label = domainLabel(env);
    if (seen.has(label)) return;
    if (classifyEnvironmentStatus(env).active) {
      active.push(env);
      seen.add(label);
    }
  });
  environments.forEach((env) => {
    const label = domainLabel(env);
    if (seen.has(label)) return;
    inactive.push(env);
    seen.add(label);
  });

  // A failed dashboard fetch must never render fake projects: with nothing
  // real to show, the list renders its offline state (with its own Retry
  // control) on error, or an invite into onboarding when simply empty. The
  // Global Services row stays above either way: that panel never depended on
  // the dashboard fetch.
  const emptyState = !isEmpty ? null : dashboardError ? (
    <div className="w-full mt-3 mb-4 text-left p-4 rounded-xl bg-background-secondary border border-border-primary">
      <div className="text-text-primary text-sm font-semibold">Dashboard offline</div>
      <div className="mt-1 text-xs text-text-tertiary">
        Could not reach the desktop bridge. No projects to show.
      </div>
      <button
        type="button"
        data-testid="env-list-retry"
        className="mt-3 h-9 px-4 bg-primary text-slate-900 rounded-lg text-xs font-bold hover:bg-primary/90 transition-all flex items-center gap-2 active:scale-95"
        title="Retry loading the dashboard"
        onClick={onRetry}
      >
        <span className="material-symbols-outlined text-[16px]" aria-hidden="true">refresh</span>
        Retry
      </button>
    </div>
  ) : (
    <div className="w-full mt-3 mb-4 text-left p-4 rounded-xl bg-background-secondary border border-border-primary">
      <div className="text-text-primary text-sm font-semibold">No environments yet</div>
      <div className="mt-1 text-xs text-text-tertiary">
        Onboard your first project to get started.
      </div>
      <button
        type="button"
        data-testid="env-list-empty-onboarding"
        className="mt-3 h-9 px-4 bg-primary text-slate-900 rounded-lg text-xs font-bold hover:bg-primary/90 transition-all flex items-center gap-2 active:scale-95"
        title="Add your first project"
        onClick={onOpenOnboarding}
      >
        <span className="material-symbols-outlined text-[16px]" aria-hidden="true">add</span>
        Add project
      </button>
    </div>
  );

  return (
    <>
      <button
        type="button"
        data-testid="global-services-row"
        className={globalClass}
        title="Open Global Services"
        onClick={onSwitchSidebarMode}
      >
        {globalSelected ? (
          <div className="absolute inset-y-0 left-0 w-1 bg-primary/80"></div>
        ) : null}
        <div className="flex items-center gap-3">
          <div className={globalIconWrapClass}>
            <span className="material-symbols-outlined text-[20px]" aria-hidden="true">hub</span>
          </div>
          <div className="flex flex-col items-start min-w-0 pointer-events-none">
            <span className={globalTitleClass}>Global Services</span>
            <span className={globalSubtitleClass}>Shared system services</span>
          </div>
        </div>
      </button>
      {emptyState}
      {!isEmpty ? (
        <>
          <Group
            title="Active Environments"
            items={active}
            empty="No active environments."
            onSelect={onSelect}
            onToggle={onToggle}
            selectedProject={state.selectedProject}
            sidebarMode={sidebarMode}
            busyEnvs={busyEnvs}
          />
          <Group
            title="Inactive Environments"
            items={inactive}
            empty="No projects found."
            mt="mt-8"
            onSelect={onSelect}
            onToggle={onToggle}
            selectedProject={state.selectedProject}
            sidebarMode={sidebarMode}
            busyEnvs={busyEnvs}
          />
        </>
      ) : null}
    </>
  );
}
