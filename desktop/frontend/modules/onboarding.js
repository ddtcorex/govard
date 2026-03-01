export const normalizeOnboardingFramework = (framework = "") => {
  const normalized = String(framework || "")
    .trim()
    .toLowerCase();

  if (["", "auto", "detect"].includes(normalized)) {
    return "";
  }
  if (normalized === "m2") {
    return "magento2";
  }
  if (normalized === "m1") {
    return "magento1";
  }
  if (normalized === "wp") {
    return "wordpress";
  }
  return normalized;
};

const inferProjectNameFromPath = (projectPath = "") => {
  const parts = String(projectPath || "")
    .split(/[\\/]+/)
    .map((part) => part.trim())
    .filter(Boolean);
  return parts.at(-1) || "";
};

const normalizeOnboardingDomain = (domain = "", projectPath = "") => {
  const trimmed = String(domain || "")
    .trim()
    .toLowerCase();
  const base = trimmed || inferProjectNameFromPath(projectPath).toLowerCase();
  if (!base) {
    return "";
  }
  if (base.includes(".")) {
    return base;
  }
  return `${base}.test`;
};

export const createOnboardingController = ({
  bridge,
  refs,
  onStatus,
  onToast,
  onProjectAdded,
}) => {
  const browseProject = async () => {
    try {
      const path = String((await bridge.pickProjectDirectory()) || "").trim();
      if (!path) {
        return;
      }
      if (refs.projectPath) {
        refs.projectPath.value = path;
      }
      if (refs.displayProjectPath) {
        refs.displayProjectPath.textContent = path;
      }
      if (
        refs.projectDomain &&
        !String(refs.projectDomain.value || "").trim()
      ) {
        refs.projectDomain.value = inferProjectNameFromPath(path);
      }
      onStatus(`Selected project path: ${path}`);
    } catch (err) {
      onStatus(`Failed to select directory: ${err}`);
      onToast(`Error: ${err}`, "error");
    }
  };

  const addProject = async () => {
    const projectPath = String(refs.projectPath?.value || "").trim();
    if (!projectPath) {
      onStatus("Project root directory is required.");
      onToast("Project root directory is required.", "warning");
      return;
    }

    const framework = normalizeOnboardingFramework(
      refs.projectFramework?.value || "",
    );
    const domain = normalizeOnboardingDomain(
      refs.projectDomain?.value || "",
      projectPath,
    );
    const serviceOptions = {
      varnish: Boolean(refs.onboardVarnish?.checked),
      redis: Boolean(refs.onboardRedis?.checked),
      rabbitmq: Boolean(refs.onboardRabbitMQ?.checked),
      elasticsearch: Boolean(refs.onboardElasticsearch?.checked),
    };

    onStatus("Starting project onboarding...");
    try {
      const message = String(
        (await bridge.onboardProject({
          projectPath,
          framework,
          domain,
          varnishEnabled: serviceOptions.varnish,
          redisEnabled: serviceOptions.redis,
          rabbitMQEnabled: serviceOptions.rabbitmq,
          elasticsearchEnabled: serviceOptions.elasticsearch,
          applyOverrides: true,
        })) || "",
      );

      onStatus("Project onboarded successfully.");
      onToast(message || "Project onboarded successfully.", "success");

      if (typeof onProjectAdded === "function") {
        await onProjectAdded();
      }
      toggleModal(false);
    } catch (err) {
      onStatus("Failed to onboard project.");
      onToast(`Failed to onboard project: ${err}`, "error");
    }
  };

  const toggleModal = (open) => {
    if (!refs.onboardingModal) {
      return;
    }
    if (open) {
      refs.onboardingModal.classList.remove("hidden");
      refs.onboardingModal.classList.add("flex");
    } else {
      refs.onboardingModal.classList.add("hidden");
      refs.onboardingModal.classList.remove("flex");
    }
  };

  return {
    browseProject,
    addProject,
    toggleModal,
  };
};

export const renderOnboardingModal = (container) => {
  if (!container) return;
  container.innerHTML = `
      <div
        id="onboardingModal"
        class="hidden fixed inset-0 z-[100] bg-[#0c1810]/80 backdrop-blur-sm flex items-center justify-center p-4 md:p-8"
      >
        <div
          class="bg-white dark:bg-[#102316] w-full max-w-5xl h-[85vh] rounded-2xl flex flex-col overflow-hidden shadow-2xl relative border border-slate-200 dark:border-white/10"
        >
          <header
            class="flex items-center justify-between whitespace-nowrap border-b border-solid border-slate-200 dark:border-white/10 px-8 py-5 bg-surface-light dark:bg-[#1a3322] shrink-0"
          >
            <div class="flex items-center gap-4">
              <div
                class="size-10 text-primary flex items-center justify-center bg-primary/10 rounded-xl"
              >
                <span class="material-symbols-outlined text-2xl"
                  >add_circle</span
                >
              </div>
              <div>
                <h2
                  class="text-slate-900 dark:text-white text-xl font-bold leading-tight tracking-tight"
                >
                  Project Onboarding
                </h2>
                <p class="text-slate-500 dark:text-slate-400 text-xs mt-0.5">
                  Initialize a new project environment
                </p>
              </div>
            </div>
            <button
              data-action="close-onboarding"
              class="text-slate-400 hover:text-white transition-colors"
            >
              <span class="material-symbols-outlined">close</span>
            </button>
          </header>

          <main
            class="flex-1 overflow-hidden relative flex flex-col lg:flex-row bg-background-light dark:bg-[#102316]/50"
          >
            <!-- Sidebar/Steps -->
            <aside
              class="lg:w-1/3 flex flex-col border-r border-slate-200 dark:border-white/10 bg-surface-light dark:bg-[#1a3322] overflow-y-auto shrink-0"
            >
              <div class="p-8 flex flex-col gap-8 h-full">
                <div class="flex flex-col gap-4">
                  <div
                    class="text-xs font-bold text-primary uppercase tracking-wider"
                  >
                    Step 1 of 3
                  </div>
                  <h1
                    class="text-3xl font-black tracking-tight text-slate-900 dark:text-white"
                  >
                    Project Source
                  </h1>
                  <p class="text-slate-500 dark:text-slate-400 text-sm">
                    Review detected framework and settings.
                  </p>
                </div>

                <!-- Project Path Input -->
                <div
                  class="group relative rounded-xl bg-slate-100 dark:bg-black/40 border border-slate-200 dark:border-white/5 p-4 transition-all hover:border-primary/50"
                >
                  <div class="flex items-center gap-3 mb-2">
                    <span class="material-symbols-outlined text-slate-400"
                      >folder_open</span
                    >
                    <span
                      class="text-xs font-medium text-slate-500 dark:text-slate-400 uppercase tracking-wider"
                      >Local Path</span
                    >
                  </div>
                  <div
                    class="font-mono text-sm text-slate-700 dark:text-slate-200 break-all"
                    id="displayProjectPath"
                  >
                    Click 'Edit' to select project
                  </div>
                  <input type="hidden" id="projectPath" />
                  <button
                    data-action="browse-project"
                    class="absolute top-3 right-3 p-1 rounded hover:bg-white/10 text-slate-400 hover:text-primary transition-colors"
                  >
                    <span class="material-symbols-outlined text-[18px]"
                      >edit</span
                    >
                  </button>
                </div>

                <!-- Detection Card -->
                <div
                  id="detectionState"
                  class="relative overflow-hidden rounded-xl bg-slate-100 dark:bg-black/20 border border-slate-200 dark:border-white/5 p-6 flex flex-col gap-4 text-center justify-center items-center h-48"
                >
                  <div
                    class="rounded-full bg-slate-200 dark:bg-white/5 p-3 text-slate-400"
                  >
                    <span class="material-symbols-outlined text-3xl"
                      >search</span
                    >
                  </div>
                  <div>
                    <h3
                      class="text-sm font-bold text-slate-900 dark:text-slate-300 mb-1"
                    >
                      No Directory Selected
                    </h3>
                    <p class="text-xs text-slate-500 dark:text-slate-500">
                      Click the Edit button above to choose your project folder.
                    </p>
                  </div>
                </div>

                <!-- Timeline -->
                <div class="mt-auto pt-8">
                  <div class="flex flex-col gap-0 relative">
                    <div
                      class="absolute left-[15px] top-3 bottom-3 w-0.5 bg-slate-200 dark:bg-white/10"
                    ></div>
                    <div class="flex items-center gap-4 relative z-10 pb-6">
                      <div
                        class="w-8 h-8 rounded-full bg-primary text-primary-content flex items-center justify-center font-bold text-sm shadow-[0_0_10px_rgba(13,242,89,0.4)]"
                      >
                        1
                      </div>
                      <span class="font-medium text-slate-900 dark:text-white"
                        >Source & Detection</span
                      >
                    </div>
                    <div
                      class="flex items-center gap-4 relative z-10 pb-6 opacity-40"
                    >
                      <div
                        class="w-8 h-8 rounded-full bg-slate-200 dark:bg-white/10 text-slate-500 dark:text-slate-400 flex items-center justify-center font-bold text-sm border border-slate-300 dark:border-white/10"
                      >
                        2
                      </div>
                      <span
                        class="font-medium text-slate-500 dark:text-slate-400"
                        >Environment Config</span
                      >
                    </div>
                    <div
                      class="flex items-center gap-4 relative z-10 opacity-40"
                    >
                      <div
                        class="w-8 h-8 rounded-full bg-slate-200 dark:bg-white/10 text-slate-500 dark:text-slate-400 flex items-center justify-center font-bold text-sm border border-slate-300 dark:border-white/10"
                      >
                        3
                      </div>
                      <span
                        class="font-medium text-slate-500 dark:text-slate-400"
                        >Build Container</span
                      >
                    </div>
                  </div>
                </div>
              </div>
            </aside>

            <!-- Main Config Area -->
            <div class="flex-1 overflow-y-auto relative">
              <div
                class="p-8 lg:p-12 max-w-3xl mx-auto w-full flex flex-col gap-10 pb-32"
              >
                <div>
                  <h2
                    class="text-2xl font-bold text-slate-900 dark:text-white mb-2"
                  >
                    Govard Configuration
                  </h2>
                  <p class="text-slate-500 dark:text-slate-400">
                    Set up your local domain and required services.
                  </p>
                </div>

                <section class="grid grid-cols-1 md:grid-cols-2 gap-6">
                  <div class="flex flex-col gap-2">
                    <label
                      class="text-sm font-medium text-slate-700 dark:text-slate-300 flex justify-between"
                    >
                      Local Domain
                      <span class="text-xs text-slate-400 dark:text-slate-500"
                        >Must be unique</span
                      >
                    </label>
                    <div class="relative flex items-center">
                      <input
                        id="projectDomain"
                        class="w-full bg-white dark:bg-surface-dark border border-slate-300 dark:border-white/10 rounded-lg pl-4 pr-16 py-3 text-slate-900 dark:text-white focus:ring-2 focus:ring-primary focus:border-transparent outline-none transition-shadow"
                        placeholder="project-name"
                        type="text"
                      />
                      <span
                        class="absolute right-4 text-slate-400 dark:text-slate-500 pointer-events-none font-mono text-sm"
                        >.test</span
                      >
                    </div>
                  </div>
                  <div class="flex flex-col gap-2">
                    <label
                      class="text-sm font-medium text-slate-700 dark:text-slate-300 flex justify-between"
                    >
                      Architecture / Framework
                      <span class="text-xs text-primary"
                        >Recommended: auto</span
                      >
                    </label>
                    <div class="relative">
                      <select
                        id="projectFramework"
                        class="w-full appearance-none bg-white dark:bg-surface-dark border border-slate-300 dark:border-white/10 rounded-lg px-4 py-3 text-slate-900 dark:text-white focus:ring-2 focus:ring-primary focus:border-transparent outline-none transition-shadow cursor-pointer"
                      >
                        <option value="auto">Auto-detect</option>
                        <option value="magento2">Magento 2</option>
                        <option value="magento1">Magento 1</option>
                        <option value="laravel">Laravel</option>
                        <option value="symfony">Symfony</option>
                        <option value="wordpress">WordPress</option>
                        <option value="nextjs">Next.js</option>
                        <option value="custom">Custom</option>
                      </select>
                      <div
                        class="absolute right-4 top-1/2 -translate-y-1/2 pointer-events-none text-slate-400"
                      >
                        <span class="material-symbols-outlined"
                          >expand_more</span
                        >
                      </div>
                    </div>
                  </div>
                </section>

                <div
                  class="bg-primary/10 border border-primary/20 rounded-lg p-4"
                >
                  <h4
                    class="text-xs font-bold text-primary uppercase mb-2 flex items-center gap-2"
                  >
                    <span class="material-symbols-outlined text-[14px]"
                      >info</span
                    >
                    Framework Guide & Best Practices
                  </h4>
                  <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <ul
                      class="text-[11px] text-slate-400 space-y-1 list-disc pl-4"
                    >
                      <li>
                        <b>Magento 2:</b> Best with PHP 8.1/8.2 and MariaDB
                        10.6. Varnish & Redis highly recommended for
                        performance.
                      </li>
                      <li>
                        <b>Laravel:</b> Optimized for PHP 8.3. Supports Vite
                        hot-reloading out of the box.
                      </li>
                    </ul>
                    <ul
                      class="text-[11px] text-slate-400 space-y-1 list-disc pl-4"
                    >
                      <li>
                        <b>WordPress:</b> Pre-configured with WP-CLI and
                        optimized Nginx rules.
                      </li>
                      <li>
                        <b>Custom:</b> Mix and match any services. Ideal for
                        specialized microservices.
                      </li>
                    </ul>
                  </div>
                </div>

                <hr class="border-slate-200 dark:border-white/5" />

                <section class="flex flex-col gap-6">
                  <div class="flex items-center justify-between">
                    <h3
                      class="text-lg font-bold text-slate-900 dark:text-white"
                    >
                      Auxiliary Services
                    </h3>
                    <span
                      class="text-xs font-medium px-2 py-1 rounded bg-slate-200 dark:bg-white/5 text-slate-500 dark:text-slate-400"
                      >Optional</span
                    >
                  </div>
                  <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <!-- Varnish -->
                    <label
                      class="glass-panel group relative flex flex-col gap-4 rounded-xl p-5 cursor-pointer hover:border-primary/50 transition-colors"
                    >
                      <div class="flex items-center justify-between">
                        <div
                          class="p-2 rounded-lg bg-orange-500/10 text-orange-500"
                        >
                          <span class="material-symbols-outlined">speed</span>
                        </div>
                        <div
                          class="relative inline-block w-10 h-6 align-middle select-none transition duration-200 ease-in"
                        >
                          <input
                            id="onboardVarnish"
                            class="toggle-checkbox absolute block w-4 h-4 rounded-full bg-white border-4 border-slate-600 appearance-none cursor-pointer transition-all duration-300 top-1 left-1 checked:left-5 checked:bg-white checked:border-white/0"
                            type="checkbox"
                            value="varnish"
                          />
                          <span
                            class="toggle-label block overflow-hidden h-6 rounded-full bg-slate-700 cursor-pointer transition-colors duration-300 group-hover:bg-slate-600"
                          ></span>
                        </div>
                      </div>
                      <div>
                        <h4
                          class="font-bold text-slate-900 dark:text-white group-hover:text-primary transition-colors"
                        >
                          Varnish Cache
                        </h4>
                        <p
                          class="text-xs text-slate-500 dark:text-slate-400 mt-1"
                        >
                          HTTP accelerator.
                        </p>
                      </div>
                    </label>
                    <!-- Redis -->
                    <label
                      class="glass-panel group relative flex flex-col gap-4 rounded-xl p-5 cursor-pointer hover:border-primary/50 transition-colors"
                    >
                      <div class="flex items-center justify-between">
                        <div class="p-2 rounded-lg bg-red-500/10 text-red-500">
                          <span class="material-symbols-outlined"
                            >database</span
                          >
                        </div>
                        <div
                          class="relative inline-block w-10 h-6 align-middle select-none transition duration-200 ease-in"
                        >
                          <input
                            checked
                            id="onboardRedis"
                            class="toggle-checkbox absolute block w-4 h-4 rounded-full bg-white border-4 border-slate-600 appearance-none cursor-pointer transition-all duration-300 top-1 left-1 checked:left-5 checked:bg-white checked:border-white/0"
                            type="checkbox"
                            value="redis"
                          />
                          <span
                            class="toggle-label block overflow-hidden h-6 rounded-full bg-slate-700 cursor-pointer transition-colors duration-300 group-hover:bg-slate-600"
                          ></span>
                        </div>
                      </div>
                      <div>
                        <h4
                          class="font-bold text-slate-900 dark:text-white group-hover:text-primary transition-colors"
                        >
                          Redis
                        </h4>
                        <p
                          class="text-xs text-slate-500 dark:text-slate-400 mt-1"
                        >
                          Cache & Database.
                        </p>
                      </div>
                    </label>
                    <!-- RabbitMQ -->
                    <label
                      class="glass-panel group relative flex flex-col gap-4 rounded-xl p-5 cursor-pointer hover:border-primary/50 transition-colors"
                    >
                      <div class="flex items-center justify-between">
                        <div
                          class="p-2 rounded-lg bg-orange-600/10 text-orange-600"
                        >
                          <span class="material-symbols-outlined">mail</span>
                        </div>
                        <div
                          class="relative inline-block w-10 h-6 align-middle select-none transition duration-200 ease-in"
                        >
                          <input
                            id="onboardRabbitMQ"
                            class="toggle-checkbox absolute block w-4 h-4 rounded-full bg-white border-4 border-slate-600 appearance-none cursor-pointer transition-all duration-300 top-1 left-1 checked:left-5 checked:bg-white checked:border-white/0"
                            type="checkbox"
                            value="rabbitmq"
                          />
                          <span
                            class="toggle-label block overflow-hidden h-6 rounded-full bg-slate-700 cursor-pointer transition-colors duration-300 group-hover:bg-slate-600"
                          ></span>
                        </div>
                      </div>
                      <div>
                        <h4
                          class="font-bold text-slate-900 dark:text-white group-hover:text-primary transition-colors"
                        >
                          RabbitMQ
                        </h4>
                        <p
                          class="text-xs text-slate-500 dark:text-slate-400 mt-1"
                        >
                          Message broker.
                        </p>
                      </div>
                    </label>
                    <!-- Elasticsearch -->
                    <label
                      class="glass-panel group relative flex flex-col gap-4 rounded-xl p-5 cursor-pointer hover:border-primary/50 transition-colors"
                    >
                      <div class="flex items-center justify-between">
                        <div
                          class="p-2 rounded-lg bg-yellow-400/10 text-yellow-400"
                        >
                          <span class="material-symbols-outlined">search</span>
                        </div>
                        <div
                          class="relative inline-block w-10 h-6 align-middle select-none transition duration-200 ease-in"
                        >
                          <input
                            checked
                            id="onboardElasticsearch"
                            class="toggle-checkbox absolute block w-4 h-4 rounded-full bg-white border-4 border-slate-600 appearance-none cursor-pointer transition-all duration-300 top-1 left-1 checked:left-5 checked:bg-white checked:border-white/0"
                            type="checkbox"
                            value="elasticsearch"
                          />
                          <span
                            class="toggle-label block overflow-hidden h-6 rounded-full bg-slate-700 cursor-pointer transition-colors duration-300 group-hover:bg-slate-600"
                          ></span>
                        </div>
                      </div>
                      <div>
                        <h4
                          class="font-bold text-slate-900 dark:text-white group-hover:text-primary transition-colors"
                        >
                          Elasticsearch
                        </h4>
                        <p
                          class="text-xs text-slate-500 dark:text-slate-400 mt-1"
                        >
                          Search engine.
                        </p>
                      </div>
                    </label>
                  </div>
                </section>
              </div>
            </div>
          </main>

          <footer
            class="shrink-0 p-6 bg-surface-light dark:bg-[#102316] border-t border-slate-200 dark:border-white/10 flex justify-end items-center gap-4 z-20 shadow-[0_-4px_12px_rgba(0,0,0,0.1)]"
          >
            <button
              data-action="close-onboarding"
              class="px-6 py-3 rounded-lg text-slate-600 dark:text-slate-400 font-medium hover:bg-slate-100 dark:hover:bg-white/5 transition-colors"
            >
              Cancel
            </button>
            <button
              id="onboardingSubmit"
              data-action="add-project"
              class="flex items-center gap-2 bg-primary hover:bg-primary/90 text-background-dark px-8 py-3 rounded-lg font-bold shadow-lg shadow-primary/20 transition-all transform active:scale-95"
            >
              <span>Initialize Project</span>
              <span class="material-symbols-outlined text-[20px]"
                >arrow_forward</span
              >
            </button>
          </footer>
        </div>
      </div>
  `;
};
