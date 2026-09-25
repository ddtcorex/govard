import { normalizeRemotesPayload } from "./remotes.js";

// Legacy shorthand aliases kept as a static fallback so normalization still
// works correctly for callers that run before loadFrameworkOptions() has
// populated frameworkOptionsCache (e.g. very early app lifecycle, or any
// caller that doesn't go through main.js's bootstrap). Once the registry
// loads, buildFrameworkLookups(frameworkOptionsCache) takes precedence.
const legacyFrameworkAliases = {
  m2: "magento2",
  "mage-os": "mageos",
  m1: "magento1",
  wp: "wordpress",
};

export const normalizeOnboardingFramework = (framework = "") => {
  const normalized = String(framework || "")
    .trim()
    .toLowerCase();

  if (["", "auto", "detect"].includes(normalized)) {
    return "";
  }
  const { aliasToName } = buildFrameworkLookups(frameworkOptionsCache);
  return (
    aliasToName.get(normalized) ||
    legacyFrameworkAliases[normalized] ||
    normalized
  );
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

// extraPathInferenceAliases covers path-matching hints that are NOT
// registered as Go-side Aliases (that field also drives CLI/config alias
// normalization, a broader surface out of scope for path inference) but
// were historically recognized when inferring a framework from a project
// path (e.g. a folder named "my-next-app" or "mage-os-store").
const extraPathInferenceAliases = {
  nextjs: ["next"],
  mageos: ["mage-os"],
};

const inferFrameworkFromPath = (projectPath = "") => {
  const value = String(projectPath || "").toLowerCase();
  if (!value) {
    return "";
  }
  // frameworkOptionsCache is sourced from a Go map with no defined
  // iteration order, so "first match wins" would be nondeterministic
  // whenever two frameworks' candidates overlap as substrings (e.g.
  // magento2's "magento" alias is itself a substring of "magento1").
  // Scanning every candidate across every framework and keeping the
  // single longest match instead makes the more specific name/alias win
  // deterministically, independent of cache order.
  let bestName = "";
  let bestLength = 0;
  for (const option of frameworkOptionsCache) {
    const name = String(option?.name || "").toLowerCase();
    if (!name) {
      continue;
    }
    const candidates = [
      name,
      ...(option?.aliases || []).map((a) => String(a).toLowerCase()),
      ...(extraPathInferenceAliases[name] || []),
    ];
    for (const candidate of candidates) {
      if (candidate && value.includes(candidate) && candidate.length > bestLength) {
        bestName = name;
        bestLength = candidate.length;
      }
    }
  }
  return bestName;
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
  if (normalized === "") {
    return "Auto-detect";
  }
  if (normalized === "custom") {
    return "Custom";
  }
  const match = frameworkOptionsCache.find(
    (option) => String(option?.name || "").toLowerCase() === normalized,
  );
  return match?.displayName || framework || "Auto-detect";
};

const normalizeOnboardingFrameworkVersion = (frameworkVersion = "") =>
  String(frameworkVersion || "").trim();

const formatFrameworkSummary = (framework = "", frameworkVersion = "") => {
  const label = formatFrameworkLabel(framework);
  const version = normalizeOnboardingFrameworkVersion(frameworkVersion);
  if (!version) {
    return label;
  }
  return `${label} (${version})`;
};

const frameworkVersionPlaceholderByFramework = {
  magento2: "2.4.7-p3",
  mageos: "1.3.1",
  magento1: "1.9.4",
  laravel: "11",
  symfony: "7.0",
  wordpress: "6.5",
  nextjs: "15",
};

let frameworkOptionsCache = [];

const buildFrameworkLookups = (options = []) => {
  const byName = new Map();
  const aliasToName = new Map();
  options.forEach((option) => {
    const name = String(option?.name || "").trim().toLowerCase();
    if (!name) {
      return;
    }
    byName.set(name, {
      name,
      displayName: String(option?.displayName || "").trim() || name,
      aliases: Array.isArray(option?.aliases)
        ? option.aliases.map((alias) => String(alias || "").trim().toLowerCase()).filter(Boolean)
        : [],
    });
    (option?.aliases || []).forEach((alias) => {
      const normalizedAlias = String(alias || "").trim().toLowerCase();
      if (normalizedAlias) {
        aliasToName.set(normalizedAlias, name);
      }
    });
  });
  return { byName, aliasToName };
};

const defaultServiceOptions = {
  varnish: false,
  redis: false,
  rabbitmq: false,
  elasticsearch: false,
};

export const createOnboardingController = ({
  bridge,
  refs,
  onStatus,
  onToast,
  onProjectAdded,
  getExistingDomains,
  onRunBootstrapSync,
  onSelectProject,
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
    const frameworkVersion = normalizeOnboardingFrameworkVersion(
      refs.projectFrameworkVersion?.value,
    );
    if ((!framework || framework === "auto") && projectPath) {
      const inferredFramework = inferFrameworkFromPath(projectPath);
      if (inferredFramework && refs.projectFramework) {
        refs.projectFramework.value = inferredFramework;
        framework = inferredFramework;
      }
    }
    const normalizedFramework = normalizeOnboardingFramework(framework);

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
        formatFrameworkSummary(normalizedFramework, frameworkVersion);
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
    if (refs.projectFrameworkVersion) {
      refs.projectFrameworkVersion.placeholder =
        frameworkVersionPlaceholderByFramework[normalizedFramework] ||
        "Optional";
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

    if (!frameworkVersion) {
      setHint(
        refs.projectFrameworkVersionHint,
        "Optional: lock Govard to a specific framework profile version.",
        "muted",
      );
    } else {
      setHint(
        refs.projectFrameworkVersionHint,
        `Govard will initialize with framework version ${frameworkVersion}.`,
        "success",
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
      framework: normalizedFramework,
      frameworkVersion,
      duplicateMessage,
      cloneFromGit,
      gitProtocol,
      gitURL,
      confirmFolderOverride,
      gitValidationMessage,
    };
  };

  const populateFrameworkSelect = () => {
    if (!refs.projectFramework) {
      return;
    }
    const select = refs.projectFramework;
    const previousValue = select.value;
    Array.from(select.querySelectorAll("option[data-dynamic-framework]")).forEach(
      (node) => node.remove(),
    );
    const customOption = select.querySelector('option[value="custom"]');
    frameworkOptionsCache
      .slice()
      .sort((a, b) => String(a.displayName || a.name).localeCompare(String(b.displayName || b.name)))
      .forEach((option) => {
        const node = document.createElement("option");
        node.value = option.name;
        node.textContent = option.displayName || option.name;
        node.dataset.dynamicFramework = "true";
        if (customOption) {
          select.insertBefore(node, customOption);
        } else {
          select.appendChild(node);
        }
      });
    if (previousValue) {
      select.value = previousValue;
    }
  };

  const loadFrameworkOptions = async () => {
    try {
      const list = await bridge.listFrameworks();
      frameworkOptionsCache = Array.isArray(list) ? list : [];
      populateFrameworkSelect();
    } catch (err) {
      console.error("Failed to load framework list:", err);
    }
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
    if (refs.projectFrameworkVersion) refs.projectFrameworkVersion.value = "";
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

  // The modal's markup is an island's now, so the prompt's elements come from
  // the refs the island hands over instead of a document lookup - which also
  // means they cannot be found before the island has committed.
  const getBootstrapPromptRefs = () => ({
    container: refs.onboardingBootstrapPrompt,
    remoteSelect: refs.onboardingBootstrapRemote,
    summary: refs.onboardingBootstrapSummary,
    options: refs.onboardingBootstrapOptions,
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
      empty.className = "text-xs text-text-tertiary";
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
      description.className = "text-xs text-text-tertiary";
      description.textContent = String(option.description || "");
      left.appendChild(title);
      left.appendChild(description);

      const input = document.createElement("input");
      input.type = "checkbox";
      input.className = "size-4 accent-primary";
      // No data-action here: the options land inside a subtree React owns, and
      // main.js's document-wide delegate would resolve the attribute, preventDefault
      // the click and cancel the checkbox. The island reads `data-option` from the
      // click instead. (This is also the one emitter a source grep for the attribute
      // form cannot see, which is how it survived the modal's migration.)
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
      const payload = await bridge.getSyncPresetOptions(projectPath, "full");
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

      // Proactive migration detection (Warden/DDEV)
      try {
        const source = await bridge.detectMigrationSource(path);
        const notice = refs.migrationSourceNotice;
        const text = refs.migrationSourceText;
        
        if (source && source !== "" && notice && text) {
          const capitalized = source.charAt(0).toUpperCase() + source.slice(1);
          text.textContent = `Found ${capitalized} setup! Govard will automatically import these settings.`;
          notice.classList.remove("hidden");
          notice.classList.add("flex");
        } else if (notice) {
          notice.classList.add("hidden");
          notice.classList.remove("flex");
        }
      } catch (detectErr) {
        console.error("Migration detection failed:", detectErr);
      }
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
          frameworkVersion: preview.frameworkVersion,
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

      if (typeof onSelectProject === "function") {
        const projectName = inferProjectNameFromPath(preview.projectPath);
        await onSelectProject(projectName);
      }
      toggleModal(false);
    } catch (err) {
      onStatus("Failed to onboard project.");
      onToast(`Failed to onboard project: ${err}`, "error");
    } finally {
      setSubmitting(false);
    }
  };

  const skipBootstrapPrompt = async () => {
    closeBootstrapPrompt();
    if (typeof onSelectProject === "function") {
      const projectName = inferProjectNameFromPath(pendingBootstrapContext.projectPath);
      await onSelectProject(projectName);
    }
    toggleModal(false);
    onStatus("Onboarding complete. Bootstrap skipped.");
  };

  const toggleBootstrapOption = (optionKey) => {
    if (!pendingBootstrapContext || !optionKey) {
      return;
    }
    const currentConfig = pendingBootstrapContext.config || {};
    pendingBootstrapContext.config = {
      ...currentConfig,
      [optionKey]: !currentConfig[optionKey],
    };
    renderBootstrapOptions();
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
      window.dispatchEvent(new CustomEvent("govard:blur", { detail: { isVisible: true } }));
      refs.onboardingModal.classList.remove("hidden");
      refs.onboardingModal.classList.add("flex");
      resetForm();
      closeBootstrapPrompt();
      return;
    }

    window.dispatchEvent(new CustomEvent("govard:blur", { detail: { isVisible: false } }));
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
    loadFrameworkOptions,
  };
};
