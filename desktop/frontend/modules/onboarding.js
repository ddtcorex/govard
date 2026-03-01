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

const formatPathForDisplay = (projectPath = "", maxLength = 64) => {
  const value = String(projectPath || "").trim();
  if (!value) {
    return "No folder selected";
  }
  if (value.length <= maxLength) {
    return value;
  }
  const headLength = Math.max(20, Math.floor(maxLength * 0.5));
  const tailLength = Math.max(18, maxLength - headLength - 3);
  return `${value.slice(0, headLength)}...${value.slice(-tailLength)}`;
};

export const normalizeOnboardingDomain = (domain = "", projectPath = "") => {
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

const inferFrameworkFromPath = (projectPath = "") => {
  const value = String(projectPath || "").toLowerCase();
  if (!value) {
    return "";
  }
  if (value.includes("magento2") || value.includes("m2")) {
    return "magento2";
  }
  if (value.includes("magento1") || value.includes("m1")) {
    return "magento1";
  }
  if (value.includes("laravel")) {
    return "laravel";
  }
  if (value.includes("symfony")) {
    return "symfony";
  }
  if (value.includes("wordpress") || value.includes("wp")) {
    return "wordpress";
  }
  if (value.includes("next")) {
    return "nextjs";
  }
  return "";
};

const levelToHintClass = {
  muted: "text-slate-500 dark:text-slate-400",
  success: "text-emerald-600 dark:text-emerald-300",
  warning: "text-amber-600 dark:text-amber-300",
  error: "text-red-600 dark:text-red-300",
};

const setHint = (element, message, level = "muted") => {
  if (!element) {
    return;
  }
  element.textContent = String(message || "");
  if (!element.dataset.baseClass) {
    const levelClassSet = new Set(
      Object.values(levelToHintClass).flatMap((value) =>
        String(value || "")
          .split(/\s+/)
          .filter(Boolean),
      ),
    );
    element.dataset.baseClass = String(element.className || "")
      .split(/\s+/)
      .filter((name) => name && !levelClassSet.has(name))
      .join(" ")
      .trim();
  }

  const baseClass = element.dataset.baseClass || "text-xs";
  const levelClass = levelToHintClass[level] || levelToHintClass.muted;
  element.className = `${baseClass} ${levelClass}`.trim();
};

const formatFrameworkLabel = (framework = "") => {
  const normalized = normalizeOnboardingFramework(framework);
  const labels = {
    "": "Auto-detect",
    magento2: "Magento 2",
    magento1: "Magento 1",
    laravel: "Laravel",
    symfony: "Symfony",
    wordpress: "WordPress",
    nextjs: "Next.js",
    custom: "Custom",
  };
  return labels[normalized] || framework || "Auto-detect";
};

const defaultServiceOptions = {
  varnish: false,
  redis: true,
  rabbitmq: false,
  elasticsearch: true,
};

export const createOnboardingController = ({
  bridge,
  refs,
  onStatus,
  onToast,
  onProjectAdded,
  getExistingDomains,
}) => {
  let hasAttemptedSubmit = false;

  const readExistingDomains = () => {
    if (typeof getExistingDomains !== "function") {
      return [];
    }
    const values = getExistingDomains();
    if (!Array.isArray(values)) {
      return [];
    }
    return values
      .map((entry) => {
        if (entry && typeof entry === "object") {
          return {
            domain: String(entry.domain || "").trim().toLowerCase(),
            project: String(
              entry.project ||
                entry.projectName ||
                entry.name ||
                entry.key ||
                "",
            )
              .trim()
              .toLowerCase(),
          };
        }
        return {
          domain: String(entry || "").trim().toLowerCase(),
          project: "",
        };
      })
      .filter((entry) => entry.domain);
  };

  const validateUniqueDomain = (normalizedDomain, projectPath) => {
    if (!normalizedDomain) {
      return null;
    }

    const inferredProject = inferProjectNameFromPath(projectPath).toLowerCase();
    const existing = readExistingDomains();
    for (const entry of existing) {
      if (entry.domain !== normalizedDomain) {
        continue;
      }
      // Re-onboarding same project with existing domain is allowed.
      if (inferredProject && entry.project && inferredProject === entry.project) {
        continue;
      }
      return `Domain ${normalizedDomain} is already used by another environment.`;
    }
    return null;
  };

  const setSubmitState = ({
    canSubmit = false,
    message = "Complete required fields to continue.",
    level = "muted",
    submitting = false,
  } = {}) => {
    if (refs.onboardingSubmit) {
      const disabled = submitting || !canSubmit;
      refs.onboardingSubmit.disabled = disabled;
      refs.onboardingSubmit.classList.toggle("opacity-60", disabled);
      refs.onboardingSubmit.classList.toggle("cursor-not-allowed", disabled);
      refs.onboardingSubmit.title = disabled
        ? message
        : "Initialize project environment";
    }
    setHint(refs.onboardingSubmitHint, message, level);
  };

  const syncPreview = ({ forceValidation = false } = {}) => {
    const shouldShowErrors = forceValidation || hasAttemptedSubmit;
    const projectPath = String(refs.projectPath?.value || "").trim();
    const inferredName = inferProjectNameFromPath(projectPath);

    let framework = String(refs.projectFramework?.value || "").trim();
    if ((!framework || framework === "auto") && projectPath) {
      const inferredFramework = inferFrameworkFromPath(projectPath);
      if (inferredFramework && refs.projectFramework) {
        refs.projectFramework.value = inferredFramework;
        framework = inferredFramework;
      }
    }

    const normalizedDomain = normalizeOnboardingDomain(
      refs.projectDomain?.value || "",
      projectPath,
    );
    const duplicateMessage = validateUniqueDomain(normalizedDomain, projectPath);

    if (refs.onboardingSummaryProject) {
      refs.onboardingSummaryProject.textContent = inferredName || "Not selected";
    }
    if (refs.onboardingSummaryFramework) {
      refs.onboardingSummaryFramework.textContent = formatFrameworkLabel(framework);
    }
    if (refs.onboardingSummaryDomain) {
      refs.onboardingSummaryDomain.textContent = normalizedDomain || "-";
    }

    if (!projectPath && shouldShowErrors) {
      setHint(
        refs.projectPathHint,
        "Project root directory is required.",
        "warning",
      );
    } else if (!projectPath) {
      setHint(
        refs.projectPathHint,
        "Click this card or Browse to choose the project root folder.",
        "muted",
      );
    } else {
      setHint(refs.projectPathHint, "Project root selected.", "success");
    }

    if (!normalizedDomain) {
      setHint(
        refs.projectDomainHint,
        "Domain will default to <project>.test once a path is selected.",
        "muted",
      );
    } else if (duplicateMessage) {
      setHint(refs.projectDomainHint, duplicateMessage, "warning");
    } else if (String(refs.projectDomain?.value || "").includes(".")) {
      setHint(
        refs.projectDomainHint,
        `Using full domain ${normalizedDomain}`,
        "success",
      );
    } else {
      setHint(
        refs.projectDomainHint,
        `Govard will use ${normalizedDomain}`,
        "muted",
      );
    }

    if (!projectPath && shouldShowErrors) {
      setSubmitState({
        canSubmit: false,
        message: "Project root directory is required.",
        level: "warning",
      });
    } else if (!projectPath) {
      setSubmitState({
        canSubmit: false,
        message: "Select a project path to continue.",
        level: "muted",
      });
    } else if (!normalizedDomain) {
      setSubmitState({
        canSubmit: false,
        message: "Enter a valid domain to continue.",
        level: "warning",
      });
    } else if (duplicateMessage) {
      setSubmitState({
        canSubmit: false,
        message: duplicateMessage,
        level: "warning",
      });
    } else {
      setSubmitState({
        canSubmit: true,
        message: "Ready to initialize.",
        level: "success",
      });
    }

    return {
      projectPath,
      normalizedDomain,
      framework: normalizeOnboardingFramework(framework),
      duplicateMessage,
    };
  };

  const resetForm = () => {
    hasAttemptedSubmit = false;
    if (refs.projectPath) refs.projectPath.value = "";
    if (refs.displayProjectPath) {
      refs.displayProjectPath.textContent = "No folder selected";
      refs.displayProjectPath.title = "";
    }
    if (refs.projectDomain) refs.projectDomain.value = "";
    if (refs.projectFramework) refs.projectFramework.value = "auto";
    if (refs.onboardVarnish) {
      refs.onboardVarnish.checked = defaultServiceOptions.varnish;
    }
    if (refs.onboardRedis) {
      refs.onboardRedis.checked = defaultServiceOptions.redis;
    }
    if (refs.onboardRabbitMQ) {
      refs.onboardRabbitMQ.checked = defaultServiceOptions.rabbitmq;
    }
    if (refs.onboardElasticsearch) {
      refs.onboardElasticsearch.checked = defaultServiceOptions.elasticsearch;
    }
    syncPreview();
  };

  const setSubmitting = (submitting) => {
    if (submitting) {
      setSubmitState({
        canSubmit: false,
        message: "Initializing environment...",
        level: "muted",
        submitting: true,
      });
      return;
    }
    syncPreview();
  };

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
        refs.displayProjectPath.textContent = formatPathForDisplay(path);
        refs.displayProjectPath.title = path;
      }
      if (
        refs.projectDomain &&
        !String(refs.projectDomain.value || "").trim()
      ) {
        refs.projectDomain.value = inferProjectNameFromPath(path);
      }

      syncPreview();
      onStatus(`Selected project path: ${path}`);
    } catch (err) {
      onStatus(`Failed to select directory: ${err}`);
      onToast(`Error: ${err}`, "error");
    }
  };

  const addProject = async () => {
    hasAttemptedSubmit = true;
    const preview = syncPreview({ forceValidation: true });
    if (!preview.projectPath) {
      onStatus("Project root directory is required.");
      onToast("Project root directory is required.", "warning");
      return;
    }
    if (!preview.normalizedDomain) {
      onStatus("A valid domain is required.");
      onToast("A valid domain is required.", "warning");
      return;
    }
    if (preview.duplicateMessage) {
      onStatus(preview.duplicateMessage);
      onToast(preview.duplicateMessage, "warning");
      return;
    }

    const serviceOptions = {
      varnish: Boolean(refs.onboardVarnish?.checked),
      redis: Boolean(refs.onboardRedis?.checked),
      rabbitmq: Boolean(refs.onboardRabbitMQ?.checked),
      elasticsearch: Boolean(refs.onboardElasticsearch?.checked),
    };

    setSubmitting(true);
    onStatus("Starting project onboarding...");
    try {
      const message = String(
        (await bridge.onboardProject({
          projectPath: preview.projectPath,
          framework: preview.framework,
          domain: preview.normalizedDomain,
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
    } finally {
      setSubmitting(false);
    }
  };

  const toggleModal = (open) => {
    if (!refs.onboardingModal) {
      return;
    }
    if (open) {
      refs.onboardingModal.classList.remove("hidden");
      refs.onboardingModal.classList.add("flex");
      resetForm();
      return;
    }

    refs.onboardingModal.classList.add("hidden");
    refs.onboardingModal.classList.remove("flex");
  };

  return {
    browseProject,
    addProject,
    toggleModal,
    handleInputChange: () => syncPreview(),
    resetForm,
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
            class="flex items-center justify-between border-b border-slate-200 dark:border-white/10 px-6 py-4 bg-surface-light dark:bg-[#1a3322] shrink-0"
          >
            <div class="flex items-center gap-3">
              <div class="size-10 text-primary flex items-center justify-center bg-primary/10 rounded-xl">
                <span class="material-symbols-outlined text-2xl">add_circle</span>
              </div>
              <div>
                <h2 class="text-slate-900 dark:text-white text-xl font-bold leading-tight tracking-tight">
                  New Environment
                </h2>
                <p class="text-slate-500 dark:text-slate-400 text-xs mt-0.5">
                  Onboard one project with clear, validated settings
                </p>
              </div>
            </div>
            <button
              data-action="close-onboarding"
              class="text-slate-400 hover:text-white transition-colors"
              aria-label="Close"
            >
              <span class="material-symbols-outlined">close</span>
            </button>
          </header>

          <main class="flex-1 overflow-hidden relative flex flex-col lg:flex-row bg-background-light dark:bg-[#102316]/50">
            <aside class="lg:w-[360px] border-r border-slate-200 dark:border-white/10 bg-surface-light dark:bg-[#1a3322] overflow-y-auto shrink-0">
              <div class="p-6 flex flex-col gap-6">
                <div>
                  <h3 class="text-lg font-bold text-slate-900 dark:text-white">Project Source</h3>
                  <p class="text-slate-500 dark:text-slate-400 text-sm mt-1">Select local project root and review resolved values.</p>
                </div>

                <div
                  id="projectPathCard"
                  data-action="browse-project"
                  role="button"
                  tabindex="0"
                  aria-label="Select project root folder"
                  class="relative rounded-xl bg-slate-100 dark:bg-black/30 border border-slate-200 dark:border-white/10 p-4 cursor-pointer hover:border-primary/40 focus:outline-none focus:ring-1 focus:ring-primary/30 transition-colors"
                >
                  <div class="flex items-center justify-between gap-2 mb-3">
                    <div class="flex items-center gap-2 text-slate-600 dark:text-slate-300">
                      <span class="material-symbols-outlined text-[18px]">folder_open</span>
                      <span class="text-sm font-semibold">Project Path</span>
                    </div>
                    <span class="text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400">Required</span>
                  </div>
                  <div class="rounded-lg border border-slate-200 dark:border-white/10 bg-white/70 dark:bg-black/20 p-3">
                    <div id="displayProjectPath" class="font-mono text-xs text-slate-700 dark:text-slate-200 break-all">No folder selected</div>
                  </div>
                  <input type="hidden" id="projectPath" />
                  <button
                    type="button"
                    class="mt-3 inline-flex items-center gap-2 px-3 py-1.5 rounded-lg border border-slate-300 dark:border-white/15 text-xs font-semibold text-slate-700 dark:text-slate-200 hover:border-primary/50 hover:text-primary transition-colors"
                  >
                    <span class="material-symbols-outlined text-[16px]">folder_open</span>
                    Browse
                  </button>
                  <p id="projectPathHint" class="text-xs text-slate-500 dark:text-slate-400 mt-6"></p>
                </div>

                <div class="rounded-xl bg-slate-100 dark:bg-black/20 border border-slate-200 dark:border-white/10 p-4">
                  <h4 class="text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400 mb-3">Resolved Summary</h4>
                  <div class="grid grid-cols-[120px_1fr] gap-y-2 text-sm">
                    <div class="text-slate-500 dark:text-slate-400">Project</div>
                    <div id="onboardingSummaryProject" class="font-semibold text-slate-900 dark:text-white">Not selected</div>
                    <div class="text-slate-500 dark:text-slate-400">Framework</div>
                    <div id="onboardingSummaryFramework" class="font-semibold text-slate-900 dark:text-white">Auto-detect</div>
                    <div class="text-slate-500 dark:text-slate-400">Domain</div>
                    <div id="onboardingSummaryDomain" class="font-mono text-xs font-semibold text-slate-900 dark:text-white">-</div>
                  </div>
                </div>

                <div class="rounded-lg border border-slate-200 dark:border-white/10 bg-white/70 dark:bg-black/20 p-3 text-xs text-slate-500 dark:text-slate-400">
                  Govard auto-appends <code>.test</code> when you enter a plain name.
                </div>
              </div>
            </aside>

            <div class="flex-1 overflow-y-auto">
              <div class="p-8 lg:p-10 max-w-3xl mx-auto w-full flex flex-col gap-8 pb-24">
                <div>
                  <h3 class="text-2xl font-bold text-slate-900 dark:text-white mb-2">Environment Settings</h3>
                  <p class="text-slate-500 dark:text-slate-400">Set domain, framework, and optional services before onboarding.</p>
                </div>

                <section class="grid grid-cols-1 md:grid-cols-2 gap-6">
                  <div class="flex flex-col gap-2">
                    <label class="text-sm font-medium text-slate-700 dark:text-slate-300">Local Domain</label>
                    <input
                      id="projectDomain"
                      class="w-full bg-white dark:bg-surface-dark border border-slate-300 dark:border-white/10 rounded-lg px-4 py-3 text-slate-900 dark:text-white focus:ring-2 focus:ring-primary focus:border-transparent outline-none transition-shadow"
                      placeholder="project-name or project-name.test"
                      type="text"
                    />
                    <p id="projectDomainHint" class="text-xs text-slate-500 dark:text-slate-400"></p>
                  </div>
                  <div class="flex flex-col gap-2">
                    <label class="text-sm font-medium text-slate-700 dark:text-slate-300">Framework</label>
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
                      <div class="absolute right-4 top-1/2 -translate-y-1/2 pointer-events-none text-slate-400">
                        <span class="material-symbols-outlined">expand_more</span>
                      </div>
                    </div>
                  </div>
                </section>

                <hr class="border-slate-200 dark:border-white/10" />

                <section class="flex flex-col gap-4">
                  <div class="flex items-center justify-between">
                    <h4 class="text-lg font-bold text-slate-900 dark:text-white">Optional Services</h4>
                    <span class="text-xs text-slate-500 dark:text-slate-400">Toggle what you need</span>
                  </div>
                  <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                    <label class="flex items-center justify-between rounded-xl border border-slate-200 dark:border-white/10 bg-white/70 dark:bg-black/20 p-4 cursor-pointer">
                      <div>
                        <div class="font-semibold text-slate-900 dark:text-white">Varnish Cache</div>
                        <div class="text-xs text-slate-500 dark:text-slate-400">HTTP accelerator</div>
                      </div>
                      <input id="onboardVarnish" type="checkbox" value="varnish" class="size-4 accent-primary" />
                    </label>

                    <label class="flex items-center justify-between rounded-xl border border-slate-200 dark:border-white/10 bg-white/70 dark:bg-black/20 p-4 cursor-pointer">
                      <div>
                        <div class="font-semibold text-slate-900 dark:text-white">Redis</div>
                        <div class="text-xs text-slate-500 dark:text-slate-400">Cache and storage</div>
                      </div>
                      <input id="onboardRedis" type="checkbox" value="redis" checked class="size-4 accent-primary" />
                    </label>

                    <label class="flex items-center justify-between rounded-xl border border-slate-200 dark:border-white/10 bg-white/70 dark:bg-black/20 p-4 cursor-pointer">
                      <div>
                        <div class="font-semibold text-slate-900 dark:text-white">RabbitMQ</div>
                        <div class="text-xs text-slate-500 dark:text-slate-400">Message queue</div>
                      </div>
                      <input id="onboardRabbitMQ" type="checkbox" value="rabbitmq" class="size-4 accent-primary" />
                    </label>

                    <label class="flex items-center justify-between rounded-xl border border-slate-200 dark:border-white/10 bg-white/70 dark:bg-black/20 p-4 cursor-pointer">
                      <div>
                        <div class="font-semibold text-slate-900 dark:text-white">Elasticsearch</div>
                        <div class="text-xs text-slate-500 dark:text-slate-400">Search engine</div>
                      </div>
                      <input id="onboardElasticsearch" type="checkbox" value="elasticsearch" checked class="size-4 accent-primary" />
                    </label>
                  </div>
                </section>
              </div>
            </div>
          </main>

          <footer class="shrink-0 p-5 bg-surface-light dark:bg-[#102316] border-t border-slate-200 dark:border-white/10 flex justify-between items-center gap-3">
            <p id="onboardingSubmitHint" class="text-xs text-slate-500 dark:text-slate-400">Select a project path to continue.</p>
            <div class="flex items-center gap-3">
            <button
              data-action="close-onboarding"
              class="px-5 py-2.5 rounded-lg text-slate-600 dark:text-slate-400 font-medium hover:bg-slate-100 dark:hover:bg-white/5 transition-colors"
            >
              Cancel
            </button>
            <button
              id="onboardingSubmit"
              data-action="add-project"
              class="flex items-center gap-2 bg-primary hover:bg-primary/90 text-background-dark px-6 py-2.5 rounded-lg font-bold shadow-lg shadow-primary/20 transition-all transform active:scale-95"
            >
              <span>Initialize Project</span>
              <span class="material-symbols-outlined text-[20px]">arrow_forward</span>
            </button>
            </div>
          </footer>
        </div>
      </div>
  `;
};
