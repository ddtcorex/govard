import { useEffect } from "react";
import { byId } from "../utils/dom.js";

type OnboardingController = {
  browseProject(): Promise<void>;
  addProject(): Promise<void>;
  confirmBootstrapPrompt(): Promise<void>;
  skipBootstrapPrompt(): Promise<void>;
  toggleBootstrapOption(option: string): void;
  toggleModal(open: boolean): void;
  handleInputChange(): void;
  loadFrameworkOptions(): Promise<void>;
};
/** What main.js calls for the shell's own button. */
export type OnboardingIslandApi = {
  open(): void;
};
type Props = {
  controller: OnboardingController;
  registerRefs(refs: Record<string, unknown>): void;
  registerApi(api: OnboardingIslandApi | null): void;
};

/** Every element the onboarding controller writes into, resolved once it exists. */
const collectRefs = () => ({
  onboardingModal: byId("onboardingModal"),
  projectPath: byId("projectPath"),
  displayProjectPath: byId("displayProjectPath"),
  projectPathHint: byId("projectPathHint"),
  projectDomain: byId("projectDomain"),
  projectDomainHint: byId("projectDomainHint"),
  projectFramework: byId("projectFramework"),
  projectFrameworkVersion: byId("projectFrameworkVersion"),
  projectFrameworkVersionHint: byId("projectFrameworkVersionHint"),
  onboardFromGit: byId("onboardFromGit"),
  gitCloneFields: byId("gitCloneFields"),
  gitProtocol: byId("gitProtocol"),
  gitUrl: byId("gitUrl"),
  gitUrlHint: byId("gitUrlHint"),
  gitConfirmContainer: byId("gitConfirmContainer"),
  gitConfirmOverride: byId("gitConfirmOverride"),
  gitConfirmHint: byId("gitConfirmHint"),
  onboardingSummaryProject: byId("onboardingSummaryProject"),
  onboardingSummaryFramework: byId("onboardingSummaryFramework"),
  onboardingSummaryDomain: byId("onboardingSummaryDomain"),
  onboardingSubmitSpinner: byId("onboardingSubmitSpinner"),
  onboardingSubmitHint: byId("onboardingSubmitHint"),
  onboardingSubmit: byId("onboardingSubmit"),
  onboardVarnish: byId("onboardVarnish"),
  onboardRedis: byId("onboardRedis"),
  onboardRabbitMQ: byId("onboardRabbitMQ"),
  onboardElasticsearch: byId("onboardElasticsearch"),
  migrationSourceNotice: byId("migrationSourceNotice"),
  migrationSourceText: byId("migrationSourceText"),
  onboardingBootstrapPrompt: byId("onboardingBootstrapPrompt"),
  onboardingBootstrapRemote: byId("onboardingBootstrapRemote"),
  onboardingBootstrapSummary: byId("onboardingBootstrapSummary"),
  onboardingBootstrapOptions: byId("onboardingBootstrapOptions"),
});

const closeOnEnter =
  (run: () => void) => (event: { key: string; preventDefault(): void }) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      run();
    }
  };

/**
 * The onboarding modal. The markup mirrors what renderOnboardingModal injected,
 * element for element and class for class (no look change), minus the
 * data-action attributes the global delegate resolved (D5).
 *
 * This is the settings drawer's shape, for the same reason (spec, island
 * contract): createOnboardingController is a 700-line wizard - validation, the
 * git sub-flow, the framework cache, the migration probe, the bootstrap prompt -
 * that writes into 33 elements and is covered by DOM-coupled unit tests. The
 * island therefore renders the structure ONCE and hands the controller the real
 * elements through the seam it already has (updateRefs), and holds NO state of
 * its own: any state would re-render the nodes the controller mutates, and
 * populateFrameworkSelect plus the bootstrap prompt build their options with
 * innerHTML into containers this markup only declares.
 *
 * The keyboard path the delegate owned moves into the island with the attribute:
 * main.js's document-wide keydown handler fired `[data-action][role="button"]`
 * on Enter or Space, and the path card keeps that behaviour through closeOnEnter.
 * The seven input listeners main.js bound by hand are React onChange handlers here
 * for the same reason - the elements are React's now.
 */
export function OnboardingModal({
  controller,
  registerRefs,
  registerApi,
}: Props) {
  // Opening is the island's now, and it re-asks for the framework list every
  // time: the list is fetched data and the wizard can be reopened long after the
  // boot-time load (which is also the load that may have run before this markup
  // existed, leaving the select on its two static options).
  const open = () => {
    controller.toggleModal(true);
    void controller.loadFrameworkOptions();
  };

  useEffect(() => {
    registerRefs(collectRefs());
    // main.js's bootstrap loads the framework options before React has committed
    // this markup, so the select would stay on its two static options; whichever
    // call finds no refs is a no-op inside the controller.
    void controller.loadFrameworkOptions();
  }, [controller, registerRefs]);

  // A separate effect owns the API registration, so unmounting can hand it back:
  // the island's own work dies with it, but the closure main.js kept would not.
  useEffect(() => {
    registerApi({ open });
    return () => registerApi(null);
  }, [registerApi, open]);

  return (
    <div
      id="onboardingModal"
      className="hidden fixed inset-0 z-100 bg-slate-900/60 dark:bg-background-primary/95 backdrop-blur-md flex items-center justify-center p-4 md:p-8 transition-all duration-500"
    >
      <div className="dark:bg-surface-secondary w-full max-w-6xl h-[85vh] rounded-3xl flex flex-col overflow-hidden shadow-[0_0_100px_rgba(0,0,0,0.5)] relative border border-slate-200 dark:border-white/10 bg-white">
        <header className="flex items-center justify-between border-b border-slate-200 dark:border-white/10 px-8 py-6 bg-slate-50 dark:bg-surface-primary shrink-0 relative z-20">
          <div className="flex items-center gap-4">
            <div className="size-12 text-primary flex items-center justify-center bg-primary/10 rounded-2xl border border-primary/20 shadow-lg shadow-primary/5">
              <span className="material-symbols-outlined text-3xl">add_circle</span>
            </div>
            <div>
              <h2 className="text-slate-900 dark:text-white text-2xl font-black leading-tight tracking-tight">
                New Project Environment
              </h2>
              <p className="text-slate-500 dark:text-slate-400 text-xs mt-1 font-medium italic opacity-80">
                Carefully verified configurations for optimal development
              </p>
            </div>
          </div>
          <button
            type="button"
            data-testid="close-onboarding"
            className="group p-2.5 rounded-xl hover:bg-black/10 dark:hover:bg-white/10 text-slate-400 hover:text-slate-900 dark:hover:text-white transition-all border border-transparent hover:border-slate-200 dark:hover:border-white/10"
            aria-label="Close"
            onClick={() => controller.toggleModal(false)}
          >
            <span className="material-symbols-outlined group-hover:rotate-90 transition-transform">
              close
            </span>
          </button>
        </header>

        <main className="flex-1 overflow-hidden relative flex flex-col lg:grid lg:grid-cols-[380px_1fr] dark:bg-surface-secondary bg-white">
          {/* Sidebar: Source & Summary */}
          <aside className="min-w-0 border-r border-slate-200 dark:border-white/10 bg-slate-50/50 dark:bg-black/40 overflow-y-auto custom-scrollbar relative z-10">
            <div className="p-8 flex flex-col gap-8">
              <section className="flex flex-col gap-4">
                <div className="flex items-center gap-2.5">
                  <span className="material-symbols-outlined text-primary/60 text-[20px]">
                    source
                  </span>
                  <h3 className="text-xs font-black uppercase tracking-[0.2em] text-slate-400 dark:text-slate-300/60">
                    Project Source
                  </h3>
                </div>

                <div className="rounded-2xl bg-white dark:bg-black/20 border border-slate-200 dark:border-white/10 p-5 shadow-xs">
                  <label className="flex items-start justify-between gap-4 cursor-pointer group">
                    <div className="flex-1">
                      <div className="text-sm font-bold text-slate-800 dark:text-slate-100 group-hover:text-primary transition-colors">
                        Clone from Git
                      </div>
                      <p className="text-[11px] text-slate-500 dark:text-slate-400/90 mt-1 leading-relaxed font-medium">
                        Fetch repository contents before initialization.
                      </p>
                    </div>
                    <input
                      id="onboardFromGit"
                      type="checkbox"
                      className="mt-1 size-5 accent-primary rounded-md border-slate-300 dark:border-white/20 bg-white dark:bg-transparent transition-all cursor-pointer"
                      onChange={() => controller.handleInputChange()}
                    />
                  </label>

                  <div
                    id="gitCloneFields"
                    className="hidden mt-6 space-y-4 pt-4 border-t border-slate-100 dark:border-white/5"
                  >
                    <div className="flex flex-col gap-1.5">
                      <label
                        htmlFor="gitProtocol"
                        className="text-[10px] font-black uppercase tracking-widest text-slate-400 dark:text-slate-400/80 ml-1"
                      >
                        Protocol
                      </label>
                      <div className="relative">
                        <select
                          id="gitProtocol"
                          className="w-full rounded-xl border border-slate-200 dark:border-white/10 bg-slate-50 dark:bg-slate-800 text-xs font-bold text-slate-900 dark:text-slate-100 px-4 py-2.5 appearance-none cursor-pointer focus:ring-2 focus:ring-primary/20 outline-hidden"
                          onChange={() => controller.handleInputChange()}
                        >
                          <option value="ssh">SSH</option>
                          <option value="https">HTTPS</option>
                        </select>
                        <span className="material-symbols-outlined absolute right-3 top-1/2 -translate-y-1/2 text-slate-400 dark:text-slate-500 pointer-events-none text-lg">
                          unfold_more
                        </span>
                      </div>
                    </div>

                    <div className="flex flex-col gap-1.5">
                      <label
                        htmlFor="gitUrl"
                        className="text-[10px] font-black uppercase tracking-widest text-slate-400 dark:text-slate-400/80 ml-1"
                      >
                        Repository URL
                      </label>
                      <input
                        id="gitUrl"
                        type="text"
                        placeholder="git@github.com:org/repo.git"
                        className="w-full rounded-xl border border-slate-200 dark:border-white/10 bg-slate-50 dark:bg-slate-800 text-xs font-mono text-slate-900 dark:text-slate-100 px-4 py-2.5 focus:ring-2 focus:ring-primary/20 outline-hidden transition-all placeholder:text-slate-400/50"
                        onChange={() => controller.handleInputChange()}
                      />
                    </div>

                    <div id="gitConfirmContainer" className="hidden pt-2">
                      <label className="flex items-center gap-3 cursor-pointer group">
                        <input
                          id="gitConfirmOverride"
                          type="checkbox"
                          className="size-4 accent-red-500 rounded-sm border-slate-300 dark:border-white/20 bg-white dark:bg-transparent transition-all"
                          onChange={() => controller.handleInputChange()}
                        />
                        <span className="text-[10px] text-slate-500 dark:text-slate-400/80 font-bold leading-tight group-hover:text-red-400 transition-colors">
                          Wipe folder contents before cloning
                        </span>
                      </label>
                    </div>
                  </div>
                  <p
                    id="gitUrlHint"
                    className="text-[10px] text-slate-400 dark:text-slate-400/60 mt-3 font-medium px-1"
                  ></p>
                  <p
                    id="gitConfirmHint"
                    className="text-[10px] text-slate-400 dark:text-slate-400/60 mt-1 font-medium px-1"
                  ></p>
                </div>

                <div
                  id="projectPathCard"
                  data-testid="browse-project"
                  role="button"
                  tabIndex={0}
                  aria-label="Select project root folder"
                  className="group relative rounded-2xl bg-white dark:bg-black/30 border border-slate-200 dark:border-white/10 p-6 cursor-pointer hover:border-primary/50 hover:shadow-xl hover:shadow-primary/5 focus:outline-hidden focus:ring-4 focus:ring-primary/10 transition-all duration-300"
                  onClick={() => void controller.browseProject()}
                  onKeyDown={closeOnEnter(() => void controller.browseProject())}
                >
                  <div className="flex items-center justify-between mb-4">
                    <div className="flex items-center gap-3">
                      <div className="size-10 rounded-xl bg-surface-secondary dark:bg-white/5 flex items-center justify-center text-text-tertiary dark:text-slate-400 group-hover:text-primary group-hover:bg-primary/10 transition-all">
                        <span className="material-symbols-outlined text-[22px]">
                          folder_open
                        </span>
                      </div>
                      <span className="text-sm font-black text-text-primary dark:text-slate-200">
                        Local Path
                      </span>
                    </div>
                  </div>
                  <div className="px-5 py-4 rounded-2xl border border-slate-200 dark:border-white/10 bg-slate-50/50 dark:bg-black/40 group-hover:bg-white dark:group-hover:bg-black/60 transition-all shadow-inner-sm">
                    <div
                      id="displayProjectPath"
                      className="font-mono text-[11px] text-slate-800 dark:text-slate-200 font-bold break-all leading-relaxed line-clamp-3 opacity-90 group-hover:opacity-100 transition-opacity"
                    >
                      No folder selected
                    </div>
                  </div>
                  <input type="hidden" id="projectPath" />
                  <p
                    id="projectPathHint"
                    className="text-[10px] text-text-tertiary dark:text-slate-400/60 mt-4 px-1 font-medium italic"
                  ></p>
                </div>
              </section>

              <section className="flex flex-col gap-4 mt-2">
                <div className="flex items-center gap-2.5">
                  <span className="material-symbols-outlined text-primary/60 text-[20px]">
                    assignment_turned_in
                  </span>
                  <h3 className="text-xs font-black uppercase tracking-[0.2em] text-text-tertiary dark:text-slate-300/60">
                    Environment Summary
                  </h3>
                </div>

                <div className="rounded-2xl bg-primary/5 border border-primary/10 p-6 flex flex-col gap-4 shadow-xs relative overflow-hidden">
                  <div className="absolute -right-4 -bottom-4 size-24 bg-primary/5 rounded-full blur-2xl pointer-events-none"></div>
                  <div className="grid grid-cols-[100px_1fr] gap-x-4 gap-y-3 relative z-10">
                    <div className="text-[10px] font-black uppercase tracking-widest text-text-tertiary dark:text-slate-400/60">
                      Project
                    </div>
                    <div
                      id="onboardingSummaryProject"
                      className="text-xs font-black text-text-primary dark:text-white truncate"
                    >
                      Not selected
                    </div>

                    <div className="text-[10px] font-black uppercase tracking-widest text-text-tertiary dark:text-slate-500">
                      Framework
                    </div>
                    <div
                      id="onboardingSummaryFramework"
                      className="text-xs font-black text-primary"
                    >
                      Auto-detect
                    </div>

                    <div className="text-[10px] font-black uppercase tracking-widest text-text-tertiary dark:text-slate-400/60">
                      Domain
                    </div>
                    <div
                      id="onboardingSummaryDomain"
                      className="font-mono text-[11px] font-black text-text-primary dark:text-white truncate"
                    >
                      -
                    </div>
                  </div>
                </div>
              </section>

              <div
                id="migrationSourceNotice"
                className="hidden flex items-center gap-3 px-5 py-4 bg-primary/5 dark:bg-primary/10 border border-primary/20 rounded-2xl text-primary animate-in fade-in slide-in-from-bottom-2 duration-500"
              >
                <div className="size-8 rounded-lg bg-primary/20 flex items-center justify-center shrink-0">
                  <span className="material-symbols-outlined text-[20px]">auto_fix_high</span>
                </div>
                <div className="flex-1">
                  <p className="text-[10px] font-black uppercase tracking-widest leading-none">
                    Source Detected
                  </p>
                  <p
                    id="migrationSourceText"
                    className="text-xs font-bold mt-1 text-slate-700 dark:text-slate-200"
                  >
                    Importing settings from Warden...
                  </p>
                </div>
              </div>
            </div>
          </aside>

          {/* Main Content: Settings */}
          <div className="min-w-0 overflow-y-auto custom-scrollbar dark:bg-transparent">
            <div className="p-8 lg:p-12 max-w-4xl mx-auto w-full flex flex-col gap-12 pb-32">
              <header className="flex flex-col gap-2">
                <h3 className="text-3xl font-black text-text-primary dark:text-white tracking-tight">
                  Configuration
                </h3>
                <p className="text-text-secondary dark:text-slate-400 font-medium">
                  Fine-tune your local domain and services for this environment.
                </p>
              </header>

              <section className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-8 pt-4 border-t border-border-primary dark:border-white/5">
                <div className="flex flex-col gap-3 group">
                  <label className="text-[11px] font-black uppercase tracking-[0.2em] text-text-tertiary dark:text-slate-500 ml-1 group-focus-within:text-primary transition-colors">
                    Local Domain
                  </label>
                  <div className="relative">
                    <input
                      id="projectDomain"
                      className="w-full bg-surface-secondary dark:bg-black/40 border border-border-primary dark:border-white/10 rounded-2xl px-5 py-4 text-text-primary dark:text-white font-bold focus:ring-4 focus:ring-primary/15 transition-all text-sm outline-hidden placeholder:text-text-tertiary"
                      placeholder="e.g. project-name"
                      type="text"
                      onChange={() => controller.handleInputChange()}
                    />
                    <span className="material-symbols-outlined absolute right-5 top-1/2 -translate-y-1/2 text-text-tertiary dark:text-slate-600 text-xl font-light">
                      language
                    </span>
                  </div>
                  <p
                    id="projectDomainHint"
                    className="text-[10px] font-medium text-text-tertiary dark:text-slate-400/80 ml-1"
                  ></p>
                </div>

                <div className="flex flex-col gap-3 group">
                  <label className="text-[11px] font-black uppercase tracking-[0.2em] text-text-tertiary dark:text-slate-500 ml-1 group-focus-within:text-primary transition-colors">
                    Framework Type
                  </label>
                  <div className="relative">
                    <select
                      id="projectFramework"
                      className="w-full bg-surface-secondary dark:bg-black/40 border border-border-primary dark:border-white/10 rounded-2xl px-5 py-4 text-text-primary dark:text-white font-bold focus:ring-4 focus:ring-primary/15 transition-all text-sm outline-hidden appearance-none cursor-pointer"
                      onChange={() => controller.handleInputChange()}
                    >
                      <option value="auto">🔍 Auto-detect</option>
                      <option value="custom">Custom System</option>
                    </select>
                    <span className="material-symbols-outlined absolute right-5 top-1/2 -translate-y-1/2 text-text-tertiary dark:text-slate-500 pointer-events-none text-xl">
                      expand_more
                    </span>
                  </div>
                  <p className="text-[10px] font-medium text-text-tertiary dark:text-slate-500 ml-1">
                    Govard optimizes settings based on framework.
                  </p>
                </div>

                <div className="flex flex-col gap-3 group">
                  <label className="text-[11px] font-black uppercase tracking-[0.2em] text-text-tertiary dark:text-slate-500 ml-1 group-focus-within:text-primary transition-colors">
                    Framework Version
                  </label>
                  <div className="relative">
                    <input
                      id="projectFrameworkVersion"
                      className="w-full bg-surface-secondary dark:bg-black/40 border border-border-primary dark:border-white/10 rounded-2xl px-5 py-4 text-text-primary dark:text-white font-bold focus:ring-4 focus:ring-primary/15 transition-all text-sm outline-hidden placeholder:text-text-tertiary"
                      placeholder="Optional"
                      type="text"
                      onChange={() => controller.handleInputChange()}
                    />
                    <span className="material-symbols-outlined absolute right-5 top-1/2 -translate-y-1/2 text-text-tertiary dark:text-slate-600 text-xl font-light">
                      tune
                    </span>
                  </div>
                  <p
                    id="projectFrameworkVersionHint"
                    className="text-[10px] font-medium text-text-tertiary dark:text-slate-500 ml-1"
                  >
                    Optional: lock Govard to a specific framework profile version.
                  </p>
                </div>
              </section>

              <section className="flex flex-col gap-6">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <div className="size-2 h-2 bg-primary rounded-full animate-pulse"></div>
                    <h4 className="text-[11px] font-black uppercase tracking-[0.2em] text-text-tertiary dark:text-slate-500">
                      Optional Stack Components
                    </h4>
                  </div>
                  <span className="text-[10px] font-black text-text-tertiary dark:text-slate-400 uppercase tracking-widest bg-surface-secondary dark:bg-white/5 px-2 py-1 rounded-sm">
                    Scale as needed
                  </span>
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <label className="group relative flex items-center justify-between rounded-2xl border border-slate-200 dark:border-white/10 bg-slate-50/50 dark:bg-black/20 p-5 cursor-pointer hover:border-primary/40 hover:bg-white dark:hover:bg-primary/5 transition-all duration-300">
                    <div className="flex flex-col gap-1">
                      <div className="text-sm font-black text-text-primary dark:text-slate-100">
                        Varnish Cache
                      </div>
                      <div className="text-[10px] text-text-tertiary font-bold uppercase tracking-wider">
                        Edge Acceleration
                      </div>
                    </div>
                    <input
                      id="onboardVarnish"
                      type="checkbox"
                      value="varnish"
                      className="size-5 accent-primary rounded-md transition-transform active:scale-90"
                    />
                  </label>

                  <label className="group relative flex items-center justify-between rounded-2xl border border-slate-200 dark:border-white/10 bg-slate-50/50 dark:bg-black/20 p-5 cursor-pointer hover:border-primary/40 hover:bg-white dark:hover:bg-primary/5 transition-all duration-300">
                    <div className="flex flex-col gap-1">
                      <div className="text-sm font-black text-text-primary dark:text-slate-100">
                        Redis
                      </div>
                      <div className="text-[10px] text-text-tertiary font-bold uppercase tracking-wider">
                        Key-Value Storage
                      </div>
                    </div>
                    <input
                      id="onboardRedis"
                      type="checkbox"
                      value="redis"
                      defaultChecked
                      className="size-5 accent-primary rounded-md transition-transform active:scale-90"
                    />
                  </label>

                  <label className="group relative flex items-center justify-between rounded-2xl border border-slate-200 dark:border-white/10 bg-slate-50/50 dark:bg-black/20 p-5 cursor-pointer hover:border-primary/40 hover:bg-white dark:hover:bg-primary/5 transition-all duration-300">
                    <div className="flex flex-col gap-1">
                      <div className="text-sm font-black text-text-primary dark:text-slate-100">
                        RabbitMQ
                      </div>
                      <div className="text-[10px] text-text-tertiary font-bold uppercase tracking-wider">
                        Message Broker
                      </div>
                    </div>
                    <input
                      id="onboardRabbitMQ"
                      type="checkbox"
                      value="rabbitmq"
                      className="size-5 accent-primary rounded-md transition-transform active:scale-90"
                    />
                  </label>

                  <label className="group relative flex items-center justify-between rounded-2xl border border-slate-200 dark:border-white/10 bg-slate-50/50 dark:bg-black/20 p-5 cursor-pointer hover:border-primary/40 hover:bg-white dark:hover:bg-primary/5 transition-all duration-300">
                    <div className="flex flex-col gap-1">
                      <div className="text-sm font-black text-text-primary dark:text-slate-100">
                        Elasticsearch
                      </div>
                      <div className="text-[10px] text-text-tertiary font-bold uppercase tracking-wider">
                        Search Engine
                      </div>
                    </div>
                    <input
                      id="onboardElasticsearch"
                      type="checkbox"
                      value="elasticsearch"
                      defaultChecked
                      className="size-5 accent-primary rounded-md transition-transform active:scale-90"
                    />
                  </label>
                </div>
              </section>

              <div className="p-6 rounded-2xl bg-blue-500/5 border border-blue-500/10 flex items-start gap-4">
                <span className="material-symbols-outlined text-blue-500 text-[24px]">info</span>
                <div className="flex flex-col gap-1">
                  <div className="text-xs font-black text-blue-900 dark:text-blue-300 uppercase tracking-widest leading-none mt-1">
                    Note on Initialization
                  </div>
                  <p className="text-[11px] text-blue-800/80 dark:text-blue-400 font-medium leading-relaxed">
                    Govard will automatically generate necessary SSH keys, local host
                    entries, and Docker configuration based on your framework
                    selection.
                  </p>
                </div>
              </div>
            </div>
          </div>
        </main>

        <footer className="shrink-0 px-8 py-6 bg-slate-50 dark:bg-surface-primary border-t border-slate-200 dark:border-white/10 flex flex-col sm:flex-row justify-between items-center gap-6 relative z-20">
          <div className="flex items-center gap-4 flex-1">
            <div id="onboardingSubmitSpinner" className="hidden relative">
              <div className="size-5 rounded-full border-2 border-primary/20 border-t-primary animate-spin"></div>
            </div>
            <p
              id="onboardingSubmitHint"
              className="text-xs font-bold text-text-tertiary dark:text-slate-500 uppercase tracking-wider"
            >
              Select a project path to continue.
            </p>
          </div>
          <div className="flex items-center gap-4">
            <button
              type="button"
              data-testid="close-onboarding"
              className="px-8 py-3 rounded-2xl text-text-tertiary dark:text-slate-400 text-sm font-black uppercase tracking-widest hover:bg-slate-200 dark:hover:bg-white/5 transition-all active:scale-95"
              onClick={() => controller.toggleModal(false)}
            >
              Cancel
            </button>
            <button
              type="button"
              id="onboardingSubmit"
              data-testid="add-project"
              className="group flex items-center gap-3 bg-primary hover:bg-primary/90 text-slate-900 px-10 py-4 rounded-2xl font-black uppercase tracking-widest text-xs shadow-[0_15px_30px_rgba(13,242,89,0.2)] transition-all transform active:scale-95 disabled:scale-100 disabled:opacity-30 disabled:grayscale disabled:shadow-none"
              onClick={() => void controller.addProject()}
            >
              <span>Initialize Environment</span>
              <span className="material-symbols-outlined text-[20px] group-hover:translate-x-1 transition-transform">
                arrow_forward
              </span>
            </button>
          </div>
        </footer>

        {/* Floating Bootstrap Prompt (Modal-in-Modal) */}
        <div
          id="onboardingBootstrapPrompt"
          className="hidden absolute inset-0 z-120 bg-black/60 backdrop-blur-xl flex items-center justify-center p-6 animate-in fade-in"
        >
          <div className="w-full max-w-xl rounded-[2.5rem] border border-white/10 bg-white dark:bg-slate-900 shadow-2xl overflow-hidden shadow-black/80">
            <div className="px-10 py-8 border-b border-border-primary dark:border-white/5 relative bg-primary/5">
              <h3 className="text-text-primary dark:text-white text-2xl font-black tracking-tight">
                Sync Services Now?
              </h3>
              <p className="text-xs text-text-tertiary dark:text-primary/60 mt-1 font-bold uppercase tracking-widest">
                Found configured remotes for this project
              </p>
            </div>
            <div className="px-10 py-10 space-y-8 max-h-[50vh] overflow-y-auto custom-scrollbar">
              <p
                id="onboardingBootstrapSummary"
                className="text-sm font-medium text-slate-500 dark:text-slate-300 leading-relaxed"
              ></p>

              <div className="flex flex-col gap-3">
                <label
                  htmlFor="onboardingBootstrapRemote"
                  className="text-[10px] font-black uppercase tracking-[0.2em] text-text-tertiary dark:text-slate-500 ml-1"
                >
                  Select Remote Target
                </label>
                <div className="relative">
                  <select
                    id="onboardingBootstrapRemote"
                    className="w-full bg-surface-secondary dark:bg-slate-800 border border-border-primary dark:border-white/10 rounded-2xl px-5 py-4 text-text-primary dark:text-white font-black text-sm outline-hidden focus:ring-4 focus:ring-primary/15 transition-all appearance-none cursor-pointer"
                  ></select>
                  <span className="material-symbols-outlined absolute right-5 top-1/2 -translate-y-1/2 text-text-tertiary dark:text-slate-500 pointer-events-none text-xl font-light">
                    database
                  </span>
                </div>
              </div>

              <div className="flex flex-col gap-4">
                <div className="text-[10px] font-black uppercase tracking-[0.2em] text-text-tertiary dark:text-slate-500 ml-1">
                  Bootstrap Options
                </div>
                <div
                  id="onboardingBootstrapOptions"
                  className="grid grid-cols-1 gap-3"
                  // The rows are the controller's (it builds them with
                  // createElement into this container), so a click is resolved here
                  // from the input's own `data-option`. Handling only clicks whose
                  // target IS the input keeps a label-text click from counting twice:
                  // the browser forwards that click to the input, and the forwarded
                  // event is the one this handler sees.
                  onClick={(event) => {
                    const field = (event.target as HTMLElement).closest?.(
                      "input[data-option]",
                    ) as HTMLInputElement | null;
                    if (field?.dataset.option) {
                      controller.toggleBootstrapOption(field.dataset.option);
                    }
                  }}
                ></div>
              </div>
            </div>

            <div className="px-10 py-8 bg-black/10 dark:bg-black/40 border-t border-slate-100 dark:border-white/5 flex justify-end gap-5">
              <button
                type="button"
                data-testid="skip-onboarding-bootstrap"
                className="px-8 py-3 rounded-xl text-xs font-black uppercase tracking-widest text-slate-400 hover:text-slate-600 dark:hover:text-white transition-colors"
                onClick={() => void controller.skipBootstrapPrompt()}
              >
                I&apos;ll do it later
              </button>
              <button
                type="button"
                data-testid="confirm-onboarding-bootstrap"
                className="px-10 py-3.5 bg-primary hover:bg-primary/90 border border-primary/20 rounded-2xl text-xs text-slate-900 font-black uppercase tracking-widest shadow-xl shadow-primary/10 transition-all hover:scale-[1.02] active:scale-95"
                onClick={() => void controller.confirmBootstrapPrompt()}
              >
                Sync Now
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
