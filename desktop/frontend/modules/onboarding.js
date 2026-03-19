import { normalizeRemotesPayload } from "./remotes.js";

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

export const normalizeOnboardingGitProtocol = (protocol = "") => {
  const normalized = String(protocol || "")
    .trim()
    .toLowerCase();
  if (normalized === "https") {
    return "https";
  }
  return "ssh";
};

const gitURLMatchesProtocol = (protocol = "ssh", gitURL = "") => {
  const normalizedProtocol = normalizeOnboardingGitProtocol(protocol);
  const value = String(gitURL || "")
    .trim()
    .toLowerCase();
  if (!value) {
    return false;
  }
  if (normalizedProtocol === "https") {
    return value.startsWith("https://");
  }
  return value.startsWith("git@") || value.startsWith("ssh://");
};

const gitURLPlaceholderByProtocol = {
  ssh: "git@github.com:org/repository.git",
  https: "https://github.com/org/repository.git",
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
  muted: "text-text-tertiary",
  success: "text-primary",
  warning: "text-amber-500",
  error: "text-red-500",
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
  onRunBootstrapSync,
}) => {
  let hasAttemptedSubmit = false;
  let pendingBootstrapContext = null;
  let bootstrapOptionDefs = [];

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
            domain: String(entry.domain || "")
              .trim()
              .toLowerCase(),
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
          domain: String(entry || "")
            .trim()
            .toLowerCase(),
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
      if (
        inferredProject &&
        entry.project &&
        inferredProject === entry.project
      ) {
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
    const showSpinner =
      submitting && Boolean(refs.onboardFromGit?.checked) && level !== "error";
    if (refs.onboardingSubmitSpinner) {
      refs.onboardingSubmitSpinner.classList.toggle("hidden", !showSpinner);
    }
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
    const cloneFromGit = Boolean(refs.onboardFromGit?.checked);
    const gitProtocol = normalizeOnboardingGitProtocol(refs.gitProtocol?.value);
    const gitURL = String(refs.gitUrl?.value || "").trim();
    const gitURLMatches = gitURLMatchesProtocol(gitProtocol, gitURL);
    const gitURLMissing = cloneFromGit && !gitURL;
    const gitURLInvalid = cloneFromGit && gitURL && !gitURLMatches;
    const confirmFolderOverride = Boolean(refs.gitConfirmOverride?.checked);
    const confirmOverrideMissing = cloneFromGit && !confirmFolderOverride;
    const gitValidationMessage = gitURLMissing
      ? "Repository URL is required when Git onboarding is enabled."
      : gitURLInvalid
        ? `Repository URL must match ${gitProtocol.toUpperCase()} format.`
        : "";

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
    const duplicateMessage = validateUniqueDomain(
      normalizedDomain,
      projectPath,
    );

    if (refs.onboardingSummaryProject) {
      refs.onboardingSummaryProject.textContent =
        inferredName || "Not selected";
    }
    if (refs.onboardingSummaryFramework) {
      refs.onboardingSummaryFramework.textContent =
        formatFrameworkLabel(framework);
    }
    if (refs.onboardingSummaryDomain) {
      refs.onboardingSummaryDomain.textContent = normalizedDomain || "-";
    }
    if (refs.gitCloneFields) {
      refs.gitCloneFields.classList.toggle("hidden", !cloneFromGit);
    }
    if (refs.gitProtocol) {
      refs.gitProtocol.value = gitProtocol;
    }
    if (refs.gitUrl) {
      refs.gitUrl.placeholder =
        gitURLPlaceholderByProtocol[gitProtocol] ||
        gitURLPlaceholderByProtocol.ssh;
    }
    if (refs.gitConfirmContainer) {
      refs.gitConfirmContainer.classList.toggle("hidden", !cloneFromGit);
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

    if (!cloneFromGit) {
      setHint(
        refs.gitUrlHint,
        "Optional: enable Git onboarding to clone source before initialization.",
        "muted",
      );
    } else if (gitURLMissing) {
      setHint(refs.gitUrlHint, gitValidationMessage, "warning");
    } else if (gitURLInvalid) {
      setHint(refs.gitUrlHint, gitValidationMessage, "warning");
    } else {
      setHint(
        refs.gitUrlHint,
        `Git ${gitProtocol.toUpperCase()} URL looks valid. Connection will be validated before clone.`,
        "success",
      );
    }

    if (!cloneFromGit) {
      setHint(
        refs.gitConfirmHint,
        "Enable Git onboarding to require folder override confirmation.",
        "muted",
      );
    } else if (confirmOverrideMissing) {
      setHint(
        refs.gitConfirmHint,
        "Please confirm folder override before cloning from Git.",
        "warning",
      );
    } else {
      setHint(refs.gitConfirmHint, "Folder override confirmed.", "success");
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
    } else if (gitValidationMessage) {
      setSubmitState({
        canSubmit: false,
        message: gitValidationMessage,
        level: "warning",
      });
    } else if (confirmOverrideMissing) {
      setSubmitState({
        canSubmit: false,
        message: "Please confirm folder override before cloning from Git.",
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
      cloneFromGit,
      gitProtocol,
      gitURL,
      confirmFolderOverride,
      gitValidationMessage,
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
    if (refs.onboardFromGit) refs.onboardFromGit.checked = false;
    if (refs.gitProtocol) refs.gitProtocol.value = "ssh";
    if (refs.gitUrl) refs.gitUrl.value = "";
    if (refs.gitConfirmOverride) refs.gitConfirmOverride.checked = false;
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

  const setSubmittingProgress = (message) => {
    setSubmitState({
      canSubmit: false,
      message: String(message || "Initializing environment..."),
      level: "muted",
      submitting: true,
    });
  };

  const getBootstrapPromptRefs = () => ({
    container: document.getElementById("onboardingBootstrapPrompt"),
    remoteSelect: document.getElementById("onboardingBootstrapRemote"),
    summary: document.getElementById("onboardingBootstrapSummary"),
    options: document.getElementById("onboardingBootstrapOptions"),
  });

  const closeBootstrapPrompt = () => {
    const promptRefs = getBootstrapPromptRefs();
    if (promptRefs.container) {
      promptRefs.container.classList.add("hidden");
    }
    if (promptRefs.remoteSelect) {
      promptRefs.remoteSelect.innerHTML = "";
    }
    if (promptRefs.options) {
      promptRefs.options.innerHTML = "";
    }
    bootstrapOptionDefs = [];
    pendingBootstrapContext = null;
  };

  const renderBootstrapOptions = () => {
    const promptRefs = getBootstrapPromptRefs();
    if (!promptRefs.options) {
      return;
    }

    promptRefs.options.innerHTML = "";
    if (!pendingBootstrapContext || bootstrapOptionDefs.length === 0) {
      const empty = document.createElement("p");
      empty.className = "text-xs text-slate-500 dark:text-slate-400";
      empty.textContent = "No additional bootstrap flags available.";
      promptRefs.options.appendChild(empty);
      return;
    }

    bootstrapOptionDefs.forEach((option) => {
      const row = document.createElement("label");
      row.className =
        "flex items-center justify-between rounded-lg border border-slate-200 dark:border-white/10 bg-slate-50 dark:bg-black/20 px-3 py-2 cursor-pointer";

      const left = document.createElement("div");
      left.className = "pr-3";
      const title = document.createElement("div");
      title.className =
        "text-sm font-medium text-slate-800 dark:text-slate-100";
      title.textContent = String(option.label || option.key || "Option");
      const description = document.createElement("div");
      description.className = "text-xs text-slate-500 dark:text-slate-400";
      description.textContent = String(option.description || "");
      left.appendChild(title);
      left.appendChild(description);

      const input = document.createElement("input");
      input.type = "checkbox";
      input.className = "size-4 accent-primary";
      input.setAttribute("data-action", "toggle-onboarding-bootstrap-option");
      input.setAttribute("data-option", String(option.key || ""));
      input.checked = Boolean(pendingBootstrapContext.config?.[option.key]);

      row.appendChild(left);
      row.appendChild(input);
      promptRefs.options.appendChild(row);
    });
  };

  const openBootstrapPrompt = async ({ projectPath, remotes }) => {
    const normalizedRemotes = normalizeRemotesPayload({
      remotes: remotes || [],
    }).remotes;
    const validRemotes = normalizedRemotes.filter(
      (remote) => String(remote.name || "").trim() !== "",
    );
    if (!projectPath || validRemotes.length === 0) {
      return false;
    }

    const promptRefs = getBootstrapPromptRefs();
    if (!promptRefs.container || !promptRefs.remoteSelect) {
      return false;
    }

    promptRefs.remoteSelect.innerHTML = "";
    validRemotes.forEach((remote) => {
      const option = document.createElement("option");
      option.value = String(remote.name);
      option.textContent = String(remote.name);
      promptRefs.remoteSelect.appendChild(option);
    });
    if (promptRefs.summary) {
      promptRefs.summary.textContent = `Found ${validRemotes.length} remote(s) for ${inferProjectNameFromPath(projectPath) || "this project"}.`;
    }

    let optionsDef = [];
    try {
      const payload = await bridge.getSyncPresetOptions("full");
      optionsDef = Array.isArray(payload?.options) ? payload.options : [];
    } catch (_err) {
      optionsDef = [];
    }

    const config = {};
    optionsDef.forEach((option) => {
      if (!option || !option.key) {
        return;
      }
      config[option.key] = Boolean(option.defaultValue);
    });

    pendingBootstrapContext = {
      projectPath: String(projectPath).trim(),
      remotes: validRemotes,
      config,
    };
    bootstrapOptionDefs = optionsDef;
    renderBootstrapOptions();

    promptRefs.container.classList.remove("hidden");
    return true;
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
    if (preview.gitValidationMessage) {
      onStatus(preview.gitValidationMessage);
      onToast(preview.gitValidationMessage, "warning");
      return;
    }
    if (preview.cloneFromGit && !preview.confirmFolderOverride) {
      const confirmMessage =
        "Please confirm folder override before cloning from Git.";
      onStatus(confirmMessage);
      onToast(confirmMessage, "warning");
      return;
    }

    const serviceOptions = {
      varnish: Boolean(refs.onboardVarnish?.checked),
      redis: Boolean(refs.onboardRedis?.checked),
      rabbitmq: Boolean(refs.onboardRabbitMQ?.checked),
      elasticsearch: Boolean(refs.onboardElasticsearch?.checked),
    };

    setSubmitting(true);
    if (preview.cloneFromGit) {
      setSubmittingProgress("Validating Git connection...");
      onStatus("Validating Git connection...");
    } else {
      onStatus("Starting project onboarding...");
    }
    try {
      const message = String(
        (await bridge.onboardProject({
          projectPath: preview.projectPath,
          framework: preview.framework,
          domain: preview.normalizedDomain,
          cloneFromGit: preview.cloneFromGit,
          gitProtocol: preview.gitProtocol,
          gitURL: preview.gitURL,
          confirmFolderOverride: preview.confirmFolderOverride,
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

      let remotesPayload = null;
      try {
        remotesPayload = await bridge.getRemotes(preview.projectPath);
      } catch (_err) {
        remotesPayload = null;
      }

      const availableRemotes = normalizeRemotesPayload(remotesPayload).remotes;
      if (availableRemotes.length > 0) {
        const opened = await openBootstrapPrompt({
          projectPath: preview.projectPath,
          remotes: availableRemotes,
        });
        if (opened) {
          onStatus("Onboarding complete. Select remote to run bootstrap.");
          return;
        }
      }

      toggleModal(false);
    } catch (err) {
      onStatus("Failed to onboard project.");
      onToast(`Failed to onboard project: ${err}`, "error");
    } finally {
      setSubmitting(false);
    }
  };

  const skipBootstrapPrompt = () => {
    closeBootstrapPrompt();
    toggleModal(false);
    onStatus("Onboarding complete. Bootstrap skipped.");
  };

  const toggleBootstrapOption = (optionKey, nextValue) => {
    if (!pendingBootstrapContext || !optionKey) {
      return;
    }
    pendingBootstrapContext.config = {
      ...(pendingBootstrapContext.config || {}),
      [optionKey]: Boolean(nextValue),
    };
  };

  const confirmBootstrapPrompt = async () => {
    if (!pendingBootstrapContext) {
      return;
    }

    const promptRefs = getBootstrapPromptRefs();
    const selectedRemote = String(promptRefs.remoteSelect?.value || "").trim();
    if (!selectedRemote) {
      onToast("Please select a remote.", "warning");
      return;
    }

    if (typeof onRunBootstrapSync !== "function") {
      closeBootstrapPrompt();
      toggleModal(false);
      return;
    }

    try {
      if (promptRefs.remoteSelect) {
        promptRefs.remoteSelect.disabled = true;
      }
      onStatus(`Starting bootstrap from ${selectedRemote}...`);
      await onRunBootstrapSync({
        projectPath: pendingBootstrapContext.projectPath,
        remoteName: selectedRemote,
        preset: "full",
        config: { ...(pendingBootstrapContext.config || {}) },
      });
      closeBootstrapPrompt();
      toggleModal(false);
    } catch (err) {
      onStatus(`Failed to start bootstrap from ${selectedRemote}.`);
      onToast(`Failed to start bootstrap: ${err}`, "error");
    } finally {
      if (promptRefs.remoteSelect) {
        promptRefs.remoteSelect.disabled = false;
      }
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
      closeBootstrapPrompt();
      return;
    }

    closeBootstrapPrompt();
    refs.onboardingModal.classList.add("hidden");
    refs.onboardingModal.classList.remove("flex");
  };

  const handleProgress = (payload = {}) => {
    const message =
      typeof payload === "string"
        ? String(payload).trim()
        : String(payload?.message || "").trim();
    if (!message) {
      return;
    }
    setSubmittingProgress(message);
    onStatus(message);
  };

  return {
    browseProject,
    addProject,
    confirmBootstrapPrompt,
    skipBootstrapPrompt,
    toggleBootstrapOption,
    toggleModal,
    handleProgress,
    handleInputChange: () => syncPreview(),
    resetForm,
  };
};

export const renderOnboardingModal = (container) => {
  if (!container) return;
  container.innerHTML = `
      <div
        id="onboardingModal"
        class="hidden fixed inset-0 z-[100] bg-slate-900/40 dark:bg-background-primary/95 backdrop-blur-md flex items-center justify-center p-4 md:p-8 transition-all duration-500"
      >
        <div
          class="bg-white dark:bg-surface-secondary w-full max-w-6xl h-[85vh] rounded-3xl flex flex-col overflow-hidden shadow-[0_0_100px_rgba(0,0,0,0.5)] relative border border-slate-200 dark:border-white/10"
        >
          <header
            class="flex items-center justify-between border-b border-slate-200 dark:border-white/10 px-8 py-6 bg-slate-50 dark:bg-surface-primary shrink-0 relative z-20"
          >
            <div class="flex items-center gap-4">
              <div class="size-12 text-primary flex items-center justify-center bg-primary/10 rounded-2xl border border-primary/20 shadow-lg shadow-primary/5">
                <span class="material-symbols-outlined text-3xl">add_circle</span>
              </div>
              <div>
                <h2 class="text-slate-900 dark:text-white text-2xl font-black leading-tight tracking-tight">
                  New Project Environment
                </h2>
                <p class="text-slate-500 dark:text-slate-400 text-xs mt-1 font-medium italic opacity-80">
                  Carefully verified configurations for optimal development
                </p>
              </div>
            </div>
            <button
              data-action="close-onboarding"
              class="group p-2.5 rounded-xl hover:bg-black/10 dark:hover:bg-white/10 text-slate-400 hover:text-slate-900 dark:hover:text-white transition-all border border-transparent hover:border-slate-200 dark:hover:border-white/10"
              aria-label="Close"
            >
              <span class="material-symbols-outlined group-hover:rotate-90 transition-transform">close</span>
            </button>
          </header>

          <main class="flex-1 overflow-hidden relative flex flex-col lg:grid lg:grid-cols-[380px_1fr] bg-white dark:bg-surface-secondary/50">
            <!-- Sidebar: Source & Summary -->
            <aside class="min-w-0 border-r border-slate-200 dark:border-white/10 bg-slate-50/50 dark:bg-surface-primary/40 overflow-y-auto custom-scrollbar relative z-10">
              <div class="p-8 flex flex-col gap-8">
                <section class="flex flex-col gap-4">
                  <div class="flex items-center gap-2.5">
                    <span class="material-symbols-outlined text-primary/60 text-[20px]">source</span>
                    <h3 class="text-xs font-black uppercase tracking-[0.2em] text-slate-400">Project Source</h3>
                  </div>

                  <div class="rounded-2xl bg-white dark:bg-black/20 border border-slate-200 dark:border-white/10 p-5 shadow-sm">
                    <label class="flex items-start justify-between gap-4 cursor-pointer group">
                      <div class="flex-1">
                        <div class="text-sm font-bold text-slate-800 dark:text-slate-100 group-hover:text-primary transition-colors">Clone from Git</div>
                        <p class="text-[11px] text-slate-500 dark:text-slate-400 mt-1 leading-relaxed font-medium">
                          Fetch repository contents before initialization.
                        </p>
                      </div>
                      <input id="onboardFromGit" type="checkbox" class="mt-1 size-5 accent-primary rounded-md border-slate-300 dark:border-white/20 transition-all cursor-pointer" />
                    </label>
                    
                    <div id="gitCloneFields" class="hidden mt-6 space-y-4 pt-4 border-t border-slate-100 dark:border-white/5">
                      <div class="flex flex-col gap-1.5">
                        <label for="gitProtocol" class="text-[10px] font-black uppercase tracking-widest text-slate-400 ml-1">Protocol</label>
                        <div class="relative">
                          <select
                            id="gitProtocol"
                            class="w-full rounded-xl border border-slate-200 dark:border-white/10 bg-slate-50 dark:bg-[#1f3a28] text-xs font-bold text-slate-900 dark:text-slate-100 px-4 py-2.5 appearance-none cursor-pointer focus:ring-2 focus:ring-primary/20 outline-none"
                          >
                            <option value="ssh">SSH</option>
                            <option value="https">HTTPS</option>
                          </select>
                          <span class="material-symbols-outlined absolute right-3 top-1/2 -translate-y-1/2 text-slate-400 pointer-events-none text-lg">unfold_more</span>
                        </div>
                      </div>
                      
                      <div class="flex flex-col gap-1.5">
                        <label for="gitUrl" class="text-[10px] font-black uppercase tracking-widest text-slate-400 ml-1">Repository URL</label>
                        <input
                          id="gitUrl"
                          type="text"
                          placeholder="git@github.com:org/repo.git"
                          class="w-full rounded-xl border border-slate-200 dark:border-white/10 bg-slate-50 dark:bg-[#1f3a28] text-xs font-mono text-slate-900 dark:text-slate-100 px-4 py-2.5 focus:ring-2 focus:ring-primary/20 outline-none transition-all placeholder:text-slate-400/50"
                        />
                      </div>
                      
                      <div id="gitConfirmContainer" class="hidden pt-2">
                        <label class="flex items-center gap-3 cursor-pointer group">
                          <input id="gitConfirmOverride" type="checkbox" class="size-4 accent-red-500 rounded border-slate-300 transition-all" />
                          <span class="text-[10px] text-slate-500 dark:text-slate-400 font-bold leading-tight group-hover:text-red-400 transition-colors">
                            Wipe folder contents before cloning
                          </span>
                        </label>
                      </div>
                    </div>
                    <p id="gitUrlHint" class="text-[10px] text-slate-400 mt-3 font-medium px-1"></p>
                    <p id="gitConfirmHint" class="text-[10px] text-slate-400 mt-1 font-medium px-1"></p>
                  </div>

                  <div
                    id="projectPathCard"
                    data-action="browse-project"
                    role="button"
                    tabindex="0"
                    aria-label="Select project root folder"
                    class="group relative rounded-2xl bg-white dark:bg-black/30 border border-slate-200 dark:border-white/10 p-6 cursor-pointer hover:border-primary/50 hover:shadow-xl hover:shadow-primary/5 focus:outline-none focus:ring-4 focus:ring-primary/10 transition-all duration-300"
                  >
                    <div class="flex items-center justify-between mb-4">
                      <div class="flex items-center gap-3">
                        <div class="size-10 rounded-xl bg-slate-100 dark:bg-white/5 flex items-center justify-center text-slate-400 group-hover:text-primary group-hover:bg-primary/10 transition-all">
                          <span class="material-symbols-outlined text-[22px]">folder_open</span>
                        </div>
                        <span class="text-sm font-black text-slate-700 dark:text-slate-200">Local Path</span>
                      </div>
                    </div>
                    <div class="rounded-xl border border-slate-100 dark:border-white/5 bg-slate-50 dark:bg-black/40 p-4 min-h-[60px] flex items-center shadow-inner">
                      <div id="displayProjectPath" class="font-mono text-xs text-slate-600 dark:text-slate-300 break-all leading-relaxed line-clamp-2">No folder selected</div>
                    </div>
                    <input type="hidden" id="projectPath" />
                    <p id="projectPathHint" class="text-[10px] text-slate-400 mt-4 px-1 font-medium italic"></p>
                  </div>
                </section>

                <section class="flex flex-col gap-4 mt-2">
                  <div class="flex items-center gap-2.5">
                    <span class="material-symbols-outlined text-primary/60 text-[20px]">assignment_turned_in</span>
                    <h3 class="text-xs font-black uppercase tracking-[0.2em] text-slate-400">Environment Summary</h3>
                  </div>
                  
                  <div class="rounded-2xl bg-primary/5 border border-primary/10 p-6 flex flex-col gap-4 shadow-sm relative overflow-hidden">
                    <div class="absolute -right-4 -bottom-4 size-24 bg-primary/5 rounded-full blur-2xl pointer-events-none"></div>
                    <div class="grid grid-cols-[100px_1fr] gap-x-4 gap-y-3 relative z-10">
                      <div class="text-[10px] font-black uppercase tracking-widest text-slate-500">Project</div>
                      <div id="onboardingSummaryProject" class="text-xs font-black text-slate-800 dark:text-white truncate">Not selected</div>
                      
                      <div class="text-[10px] font-black uppercase tracking-widest text-slate-500">Framework</div>
                      <div id="onboardingSummaryFramework" class="text-xs font-black text-primary">Auto-detect</div>
                      
                      <div class="text-[10px] font-black uppercase tracking-widest text-slate-500">Domain</div>
                      <div id="onboardingSummaryDomain" class="font-mono text-[11px] font-black text-slate-800 dark:text-white truncate">-</div>
                    </div>
                  </div>
                </section>

                <div class="flex items-center gap-2.5 px-4 py-3 bg-amber-500/5 border border-amber-500/10 rounded-xl text-amber-600 dark:text-amber-400/80">
                  <span class="material-symbols-outlined text-[18px]">auto_fix_high</span>
                  <p class="text-[10px] font-bold leading-tight uppercase tracking-wider">Auto-detecting config...</p>
                </div>
              </div>
            </aside>

            <!-- Main Content: Settings -->
            <div class="min-w-0 overflow-y-auto custom-scrollbar bg-white dark:bg-transparent">
              <div class="p-8 lg:p-12 max-w-4xl mx-auto w-full flex flex-col gap-12 pb-32">
                <header class="flex flex-col gap-2">
                  <h3 class="text-3xl font-black text-slate-900 dark:text-white tracking-tight">Configuration</h3>
                  <p class="text-slate-500 dark:text-slate-400 font-medium">Fine-tune your local domain and services for this environment.</p>
                </header>

                <section class="grid grid-cols-1 md:grid-cols-2 gap-8 pt-4 border-t border-slate-100 dark:border-white/5">
                  <div class="flex flex-col gap-3 group">
                    <label class="text-[11px] font-black uppercase tracking-[0.2em] text-slate-400 ml-1 group-focus-within:text-primary transition-colors">Local Domain</label>
                    <div class="relative">
                      <input
                        id="projectDomain"
                        class="w-full bg-slate-50 dark:bg-surface-dark border border-slate-200 dark:border-white/10 rounded-2xl px-5 py-4 text-slate-900 dark:text-white font-bold focus:ring-4 focus:ring-primary/15 transition-all text-sm outline-none"
                        placeholder="e.g. project-name"
                        type="text"
                      />
                      <span class="material-symbols-outlined absolute right-5 top-1/2 -translate-y-1/2 text-slate-300 dark:text-slate-600 text-xl font-light">language</span>
                    </div>
                    <p id="projectDomainHint" class="text-[10px] font-medium text-slate-400/80 ml-1"></p>
                  </div>

                  <div class="flex flex-col gap-3 group">
                    <label class="text-[11px] font-black uppercase tracking-[0.2em] text-slate-400 ml-1 group-focus-within:text-primary transition-colors">Framework Type</label>
                    <div class="relative">
                      <select
                        id="projectFramework"
                        class="w-full bg-slate-50 dark:bg-surface-dark border border-slate-200 dark:border-white/10 rounded-2xl px-5 py-4 text-slate-900 dark:text-white font-bold focus:ring-4 focus:ring-primary/15 transition-all text-sm outline-none appearance-none cursor-pointer"
                      >
                        <option value="auto">🔍 Auto-detect</option>
                        <option value="magento2">Magento 2</option>
                        <option value="magento1">Magento 1</option>
                        <option value="laravel">Laravel</option>
                        <option value="symfony">Symfony</option>
                        <option value="wordpress">WordPress</option>
                        <option value="nextjs">Next.js</option>
                        <option value="custom">Custom System</option>
                      </select>
                      <span class="material-symbols-outlined absolute right-5 top-1/2 -translate-y-1/2 text-slate-400 pointer-events-none text-xl">expand_more</span>
                    </div>
                    <p class="text-[10px] font-medium text-slate-400/80 ml-1">Govard optimizes settings based on framework.</p>
                  </div>
                </section>

                <section class="flex flex-col gap-6">
                  <div class="flex items-center justify-between">
                    <div class="flex items-center gap-3">
                      <div class="size-2 h-2 bg-primary rounded-full animate-pulse"></div>
                      <h4 class="text-[11px] font-black uppercase tracking-[0.2em] text-slate-400">Optional Stack Components</h4>
                    </div>
                    <span class="text-[10px] font-black text-slate-500 uppercase tracking-widest bg-slate-100 dark:bg-white/5 px-2 py-1 rounded">Scale as needed</span>
                  </div>
                  
                  <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <label class="group relative flex items-center justify-between rounded-2xl border border-slate-200 dark:border-white/10 bg-slate-50/50 dark:bg-black/20 p-5 cursor-pointer hover:border-primary/40 hover:bg-white dark:hover:bg-primary/5 transition-all duration-300">
                      <div class="flex flex-col gap-1">
                        <div class="text-sm font-black text-slate-800 dark:text-slate-100">Varnish Cache</div>
                        <div class="text-[10px] text-slate-500 font-bold uppercase tracking-wider">Edge Acceleration</div>
                      </div>
                      <input id="onboardVarnish" type="checkbox" value="varnish" class="size-5 accent-primary rounded-md transition-transform active:scale-90" />
                    </label>

                    <label class="group relative flex items-center justify-between rounded-2xl border border-slate-200 dark:border-white/10 bg-slate-50/50 dark:bg-black/20 p-5 cursor-pointer hover:border-primary/40 hover:bg-white dark:hover:bg-primary/5 transition-all duration-300">
                      <div class="flex flex-col gap-1">
                        <div class="text-sm font-black text-slate-800 dark:text-slate-100">Redis</div>
                        <div class="text-[10px] text-slate-500 font-bold uppercase tracking-wider">Key-Value Storage</div>
                      </div>
                      <input id="onboardRedis" type="checkbox" value="redis" checked class="size-5 accent-primary rounded-md transition-transform active:scale-90" />
                    </label>

                    <label class="group relative flex items-center justify-between rounded-2xl border border-slate-200 dark:border-white/10 bg-slate-50/50 dark:bg-black/20 p-5 cursor-pointer hover:border-primary/40 hover:bg-white dark:hover:bg-primary/5 transition-all duration-300">
                      <div class="flex flex-col gap-1">
                        <div class="text-sm font-black text-slate-800 dark:text-slate-100">RabbitMQ</div>
                        <div class="text-[10px] text-slate-500 font-bold uppercase tracking-wider">Message Broker</div>
                      </div>
                      <input id="onboardRabbitMQ" type="checkbox" value="rabbitmq" class="size-5 accent-primary rounded-md transition-transform active:scale-90" />
                    </label>

                    <label class="group relative flex items-center justify-between rounded-2xl border border-slate-200 dark:border-white/10 bg-slate-50/50 dark:bg-black/20 p-5 cursor-pointer hover:border-primary/40 hover:bg-white dark:hover:bg-primary/5 transition-all duration-300">
                      <div class="flex flex-col gap-1">
                        <div class="text-sm font-black text-slate-800 dark:text-slate-100">Elasticsearch</div>
                        <div class="text-[10px] text-slate-500 font-bold uppercase tracking-wider">Search Engine</div>
                      </div>
                      <input id="onboardElasticsearch" type="checkbox" value="elasticsearch" checked class="size-5 accent-primary rounded-md transition-transform active:scale-90" />
                    </label>
                  </div>
                </section>
                
                <div class="p-6 rounded-2xl bg-blue-500/5 border border-blue-500/10 flex items-start gap-4">
                  <span class="material-symbols-outlined text-blue-500 text-[24px]">info</span>
                  <div class="flex flex-col gap-1">
                    <div class="text-xs font-black text-blue-900 dark:text-blue-300 uppercase tracking-widest leading-none mt-1">Note on Initialization</div>
                    <p class="text-[11px] text-blue-800/70 dark:text-blue-400 font-medium leading-relaxed">
                      Govard will automatically generate necessary SSH keys, local host entries, and Docker configuration based on your framework selection.
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </main>

          <footer class="shrink-0 px-8 py-6 bg-slate-50 dark:bg-background-dark border-t border-slate-200 dark:border-white/10 flex flex-col sm:flex-row justify-between items-center gap-6 relative z-20">
            <div class="flex items-center gap-4 flex-1">
              <div id="onboardingSubmitSpinner" class="hidden relative">
                <div class="size-5 rounded-full border-2 border-primary/20 border-t-primary animate-spin"></div>
              </div>
              <p id="onboardingSubmitHint" class="text-xs font-bold text-slate-400 uppercase tracking-[0.05em]">Select a project path to continue.</p>
            </div>
            <div class="flex items-center gap-4">
              <button
                data-action="close-onboarding"
                class="px-8 py-3 rounded-2xl text-slate-500 dark:text-slate-400 text-sm font-black uppercase tracking-widest hover:bg-slate-200 dark:hover:bg-white/5 transition-all active:scale-95"
              >
                Cancel
              </button>
              <button
                id="onboardingSubmit"
                data-action="add-project"
                class="group flex items-center gap-3 bg-primary hover:bg-primary/90 text-background-dark px-10 py-4 rounded-2xl font-black uppercase tracking-widest text-xs shadow-[0_15px_30px_rgba(13,242,89,0.2)] transition-all transform active:scale-95 disabled:scale-100 disabled:opacity-30 disabled:grayscale disabled:shadow-none"
              >
                <span>Initialize Environment</span>
                <span class="material-symbols-outlined text-[20px] group-hover:translate-x-1 transition-transform">arrow_forward</span>
              </button>
            </div>
          </footer>

          <!-- Floating Bootstrap Prompt (Modal-in-Modal) -->
          <div
            id="onboardingBootstrapPrompt"
            class="hidden absolute inset-0 z-[120] bg-black/60 backdrop-blur-xl flex items-center justify-center p-6 animate-in fade-in"
          >
            <div class="w-full max-w-xl rounded-[2.5rem] border border-white/10 bg-white dark:bg-[#152a1b] shadow-2xl overflow-hidden shadow-black/80">
              <div class="px-10 py-8 border-b border-slate-100 dark:border-white/5 relative bg-primary/5">
                <h3 class="text-slate-900 dark:text-white text-2xl font-black tracking-tight">Sync Services Now?</h3>
                <p class="text-xs text-slate-500 dark:text-primary/60 mt-1 font-bold uppercase tracking-widest">
                  Found configured remotes for this project
                </p>
              </div>
              <div class="px-10 py-10 space-y-8 max-h-[50vh] overflow-y-auto custom-scrollbar">
                <p id="onboardingBootstrapSummary" class="text-sm font-medium text-slate-500 dark:text-slate-300 leading-relaxed"></p>
                
                <div class="flex flex-col gap-3">
                  <label for="onboardingBootstrapRemote" class="text-[10px] font-black uppercase tracking-[0.2em] text-slate-400 ml-1">Select Remote Target</label>
                  <div class="relative">
                    <select
                      id="onboardingBootstrapRemote"
                      class="w-full bg-slate-50 dark:bg-black/20 border border-slate-200 dark:border-white/10 rounded-2xl px-5 py-4 text-slate-900 dark:text-white font-black text-sm outline-none focus:ring-4 focus:ring-primary/15 transition-all appearance-none cursor-pointer"
                    ></select>
                    <span class="material-symbols-outlined absolute right-5 top-1/2 -translate-y-1/2 text-slate-400 pointer-events-none text-xl font-light">database</span>
                  </div>
                </div>

                <div class="flex flex-col gap-4">
                  <div class="text-[10px] font-black uppercase tracking-[0.2em] text-slate-400 ml-1">Bootstrap Options</div>
                  <div id="onboardingBootstrapOptions" class="grid grid-cols-1 gap-3"></div>
                </div>
              </div>
              
              <div class="px-10 py-8 bg-black/10 dark:bg-black/40 border-t border-slate-100 dark:border-white/5 flex justify-end gap-5">
                <button
                  data-action="skip-onboarding-bootstrap"
                  class="px-8 py-3 rounded-xl text-xs font-black uppercase tracking-widest text-slate-400 hover:text-slate-600 dark:hover:text-white transition-colors"
                >
                  I'll do it later
                </button>
                <button
                  data-action="confirm-onboarding-bootstrap"
                  class="px-10 py-3.5 bg-primary hover:bg-primary/90 border border-primary/20 rounded-2xl text-xs text-background-dark font-black uppercase tracking-widest shadow-xl shadow-primary/10 transition-all hover:scale-[1.02] active:scale-95"
                >
                  Sync Now
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

  `;
};
