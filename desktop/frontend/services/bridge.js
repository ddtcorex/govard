const getBridge = () => window.go?.desktop?.App;

const call = async (fn, ...args) => {
  if (!fn) {
    throw new Error("Desktop bridge not available");
  }
  return fn(...args);
};

export const desktopBridge = {
  get runtime() {
    return window.runtime;
  },
  async getDashboard(...args) {
    if (args && args.length > 0) {
      console.warn("ROGUE ARGS SENT TO GETDASHBOARD:", args);
    }
    const bridge = getBridge();
    return call(bridge?.GetDashboard?.bind(bridge)); // explicitly drop args
  },
  async getCurrentUser() {
    const bridge = getBridge();
    return call(bridge?.GetUserInfo?.bind(bridge));
  },
  async getVersion() {
    const bridge = getBridge();
    return call(bridge?.GetVersion?.bind(bridge));
  },
  async getSystemMetrics() {
    const bridge = getBridge();
    return call(bridge?.GetSystemMetrics?.bind(bridge));
  },
  async getResourceMetrics() {
    const bridge = getBridge();
    return call(bridge?.GetResourceMetrics?.bind(bridge));
  },
  async pickProjectDirectory() {
    const bridge = getBridge();
    return call(bridge?.PickProjectDirectory?.bind(bridge));
  },
  async onboardProject(inputOrPath, framework, domain = "", serviceOptions = {}) {
    const bridge = getBridge();

    // Support both object payload (current onboarding flow) and legacy positional args.
    if (
      inputOrPath &&
      typeof inputOrPath === "object" &&
      !Array.isArray(inputOrPath)
    ) {
      const input = inputOrPath;
      return call(bridge?.OnboardProject?.bind(bridge), {
        projectPath: String(input.projectPath || "").trim(),
        framework: String(input.framework || "").trim(),
        domain: String(input.domain || "").trim(),
        varnishEnabled: Boolean(input.varnishEnabled),
        redisEnabled: Boolean(input.redisEnabled),
        rabbitMQEnabled: Boolean(input.rabbitMQEnabled),
        elasticsearchEnabled: Boolean(input.elasticsearchEnabled),
        applyOverrides:
          input.applyOverrides === undefined ? true : Boolean(input.applyOverrides),
        skipIDE: Boolean(input.skipIDE),
      });
    }

    const opts = serviceOptions || {};
    return call(bridge?.OnboardProject?.bind(bridge), {
      projectPath: String(inputOrPath || "").trim(),
      framework: String(framework || "").trim(),
      domain: String(domain || "").trim(),
      varnishEnabled: Boolean(opts.varnish),
      redisEnabled: Boolean(opts.redis),
      rabbitMQEnabled: Boolean(opts.rabbitmq),
      elasticsearchEnabled: Boolean(opts.elasticsearch),
      applyOverrides: false,
      skipIDE: false,
    });
  },
  async getRemotes(project) {
    const bridge = getBridge();
    return call(bridge?.GetRemotes?.bind(bridge), project);
  },
  async testRemote(project, remoteName) {
    const bridge = getBridge();
    return call(bridge?.TestRemote?.bind(bridge), project, remoteName);
  },
  async openRemoteURL(project, remoteName) {
    const bridge = getBridge();
    return call(bridge?.OpenRemoteURL?.bind(bridge), project, remoteName);
  },
  async runRemoteSyncPreset(project, remoteName, preset, syncConfig = {}) {
    const bridge = getBridge();
    return call(
      bridge?.RunRemoteSyncPreset?.bind(bridge),
      project,
      remoteName,
      preset,
      syncConfig || {},
    );
  },
  async runRemoteSyncBackground(project, remoteName, preset, syncConfig = {}) {
    const bridge = getBridge();
    return call(
      bridge?.RunRemoteSyncBackground?.bind(bridge),
      project,
      remoteName,
      preset,
      syncConfig || {},
    );
  },
  async getSyncPresetOptions(preset) {
    const bridge = getBridge();
    return call(bridge?.GetSyncPresetOptions?.bind(bridge), preset);
  },
  async startEnvironment(project) {
    const bridge = getBridge();
    return call(bridge?.StartEnvironment?.bind(bridge), project);
  },
  async stopEnvironment(project) {
    const bridge = getBridge();
    return call(bridge?.StopEnvironment?.bind(bridge), project);
  },
  async restartEnvironment(project) {
    const bridge = getBridge();
    return call(bridge?.RestartEnvironment?.bind(bridge), project);
  },
  async toggleEnvironment(project) {
    const bridge = getBridge();
    return call(bridge?.ToggleEnvironment?.bind(bridge), project);
  },
  async openEnvironment(project) {
    const bridge = getBridge();
    return call(bridge?.OpenEnvironment?.bind(bridge), project);
  },
  async quickActionForProject(action, project) {
    const bridge = getBridge();
    return call(bridge?.QuickActionForProject?.bind(bridge), action, project);
  },
  async getLogsForService(project, service) {
    const bridge = getBridge();
    return call(bridge?.GetLogsForService?.bind(bridge), project, service);
  },
  async startLogStreamForService(project, service) {
    const bridge = getBridge();
    return call(
      bridge?.StartLogStreamForService?.bind(bridge),
      project,
      service,
    );
  },
  async stopLogStream() {
    const bridge = getBridge();
    return call(bridge?.StopLogStream?.bind(bridge));
  },
  async startTerminal(project, service, user, shell) {
    const bridge = getBridge();
    return call(
      bridge?.StartTerminal?.bind(bridge),
      project,
      service,
      user,
      shell,
    );
  },
  async startGovardTerminal(project, argsList) {
    const bridge = getBridge();
    return call(bridge?.StartGovardTerminal?.bind(bridge), project, argsList);
  },
  async writeTerminal(id, data) {
    const bridge = getBridge();
    return call(bridge?.WriteTerminal?.bind(bridge), id, data);
  },
  async resizeTerminal(id, cols, rows) {
    const bridge = getBridge();
    return call(bridge?.ResizeTerminal?.bind(bridge), id, cols, rows);
  },
  async openShellForService(project, service, user, shell) {
    const bridge = getBridge();
    return call(
      bridge?.OpenShellForService?.bind(bridge),
      project,
      service,
      user,
      shell,
    );
  },
  async getShellUser(project) {
    const bridge = getBridge();
    return call(bridge?.GetShellUser?.bind(bridge), project);
  },
  async setShellUser(project, user) {
    const bridge = getBridge();
    return call(bridge?.SetShellUser?.bind(bridge), project, user);
  },
  async resetShellUsers() {
    const bridge = getBridge();
    return call(bridge?.ResetShellUsers?.bind(bridge));
  },
  async getSettings() {
    const bridge = getBridge();
    return call(bridge?.GetSettings?.bind(bridge));
  },
  async getMailpitURL() {
    const bridge = getBridge();
    return call(bridge?.GetMailpitURL?.bind(bridge));
  },
  async updateSettings(settings = {}) {
    const bridge = getBridge();
    const payload = {
      theme: String(settings.theme || "system"),
      proxyTarget: String(settings.proxyTarget || ""),
      preferredBrowser: String(settings.preferredBrowser || ""),
      codeEditor: String(settings.codeEditor || ""),
      dbClientPreference: String(settings.dbClientPreference || "desktop"),
    };
    return call(bridge?.UpdateSettings?.bind(bridge), payload);
  },
  async resetSettings() {
    const bridge = getBridge();
    return call(bridge?.ResetSettings?.bind(bridge));
  },
};
