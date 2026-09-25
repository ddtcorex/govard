import { useEffect, useState } from "react";
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
  registerSkeleton(api: { show(): void; hide(): void }): void;
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
  onSelect,
  onToggle,
}: {
  env: Env;
  isSelected: boolean;
  onSelect(project: string): void;
  onToggle(project: string): void;
}) {
  const key = projectKey(env);
  const domain = domainLabel(env);
  const meta = classifyEnvironmentStatus(env);
  const titleClass = meta.active ? "text-text-primary" : "text-text-secondary";

  return (
    <button
      type="button"
      data-testid="env-card"
      data-env={key}
      className={itemClass(isSelected, meta.active)}
      style={{ gridTemplateColumns: "24px minmax(0,1fr)" }}
      title={`Select ${domain}`}
      onClick={() => onSelect(key)}
    >
      {isSelected ? (
        <div className="absolute inset-y-0 left-0 w-1 bg-primary"></div>
      ) : null}
      <div className="relative flex h-6 w-6 items-center justify-center self-center z-10">
        {/* The glyph keeps its own click, exactly as the vanilla delegate
            resolved the nearest data-action: without stopPropagation one click
            would also select the project. */}
        <span
          data-testid="toggle-env"
          data-env={key}
          className={`material-symbols-outlined leading-none ${meta.iconClass} transition-colors hover:text-slate-900 dark:hover:text-white text-[20px]`}
          style={meta.fill ? { fontVariationSettings: "'FILL' 1" } : undefined}
          onClick={(event) => {
            event.stopPropagation();
            onToggle(key);
          }}
        >
          {meta.iconName}
        </span>
        {meta.showPulseDot ? (
          <span className="absolute -top-0.5 -right-0.5 w-2 h-2 rounded-full bg-primary border border-(--bg-primary) animate-pulse"></span>
        ) : null}
      </div>
      <div className="min-w-0 flex flex-col justify-center pointer-events-none">
        <div className={`text-sm font-semibold leading-tight truncate ${titleClass}`}>{domain}</div>
        <div
          className={`text-[11px] ${meta.active ? meta.detailClass : "text-text-tertiary"} flex items-center gap-1 font-medium`}
        >
          <span className={`w-1 h-1 rounded-full ${meta.dotClass}`}></span>
          <span>{meta.detailText}</span>
        </div>
      </div>
    </button>
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
}: {
  title: string;
  items: Env[];
  empty: string;
  mt?: string;
  onSelect(project: string): void;
  onToggle(project: string): void;
  selectedProject: string;
  sidebarMode: string;
}) {
  return (
    <>
      <div
        className={`px-1 ${mt || "mt-6"} pb-2 text-[10px] font-bold text-primary/70 uppercase tracking-[0.12em]`}
      >
        {title}
      </div>
      {items.length ? (
        items.map((env) => (
          <EnvironmentItem
            key={projectKey(env)}
            env={env}
            isSelected={sidebarMode === "environments" && projectKey(env) === selectedProject}
            onSelect={onSelect}
            onToggle={onToggle}
          />
        ))
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
  registerSkeleton,
}: Props) {
  const state = useStore();
  const [loading, setLoading] = useState(true);

  // main.js owns both ends of the loading frame: it shows the skeleton before a
  // non-silent refresh and hides it when it publishes the environments. Inferring
  // "finished" from the environments array's identity does not work here - the
  // generated model hands back the SAME array for the same source object, so a
  // refetch can leave the identity untouched and freeze the skeleton on screen.
  useEffect(() => {
    registerSkeleton({
      show: () => setLoading(true),
      hide: () => setLoading(false),
    });
  }, [registerSkeleton]);

  if (loading) return <Skeletons />;

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

  const environments = Array.isArray(state.environments) ? state.environments : [];
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
            <span className="material-symbols-outlined text-[20px]">hub</span>
          </div>
          <div className="flex flex-col items-start min-w-0 pointer-events-none">
            <span className={globalTitleClass}>Global Services</span>
            <span className={globalSubtitleClass}>Shared system services</span>
          </div>
        </div>
      </button>
      <Group
        title="Active Environments"
        items={active}
        empty="No active environments."
        onSelect={onSelect}
        onToggle={onToggle}
        selectedProject={state.selectedProject}
        sidebarMode={sidebarMode}
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
      />
    </>
  );
}
