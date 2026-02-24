const bridge = window.go?.desktop?.App;
const runtime = window.runtime;

const call = async (fn, ...args) => {
  if (!fn) {
    throw new Error("Desktop bridge not available");
  }
  return fn(...args);
};

export const desktopBridge = {
  runtime,
  async getDashboard() {
    return call(bridge?.GetDashboard?.bind(bridge));
  },
  async getCurrentUser() {
    return call(bridge?.GetUserInfo?.bind(bridge));
  },
  async getSystemMetrics() {
    return call(bridge?.GetSystemMetrics?.bind(bridge));
  },
  async getResourceMetrics() {
    return call(bridge?.GetResourceMetrics?.bind(bridge));
  },
  async pickProjectDirectory() {
    return call(bridge?.PickProjectDirectory?.bind(bridge));
  },
  async onboardProject(projectPath, recipe, domain = "", serviceOptions = {}) {
    const options = serviceOptions || {};
    return call(
      bridge?.OnboardProject?.bind(bridge),
      projectPath,
      recipe,
      domain,
      Boolean(options.varnish),
      Boolean(options.redis),
      Boolean(options.rabbitmq),
      Boolean(options.elasticsearch),
    );
  },
  async getRemotes(project) {
    return call(bridge?.GetRemotes?.bind(bridge), project);
  },
  async addRemote(
    project,
    name,
    host,
    user,
    path,
    port,
    environment,
    capabilities,
    authMethod,
    protectedMode,
  ) {
    return call(
      bridge?.AddRemote?.bind(bridge),
      project,
      name,
      host,
      user,
      path,
      port,
      environment,
      capabilities,
      authMethod,
      Boolean(protectedMode),
    );
  },
  async testRemote(project, remoteName) {
    return call(bridge?.TestRemote?.bind(bridge), project, remoteName);
  },
  async runRemoteSyncPreset(
    project,
    remoteName,
    preset,
    syncConfig = {},
  ) {
    const config = syncConfig || {};
    return call(
      bridge?.RunRemoteSyncPreset?.bind(bridge),
      project,
      remoteName,
      preset,
      Boolean(config.sanitize),
      Boolean(config.excludeLogs),
      Boolean(config.compress),
    );
  },
  async startEnvironment(project) {
    return call(bridge?.StartEnvironment?.bind(bridge), project);
  },
  async stopEnvironment(project) {
    return call(bridge?.StopEnvironment?.bind(bridge), project);
  },
  async restartEnvironment(project) {
    return call(bridge?.RestartEnvironment?.bind(bridge), project);
  },
  async toggleEnvironment(project) {
    return call(bridge?.ToggleEnvironment?.bind(bridge), project);
  },
  async openEnvironment(project) {
    return call(bridge?.OpenEnvironment?.bind(bridge), project);
  },
  async quickActionForProject(action, project) {
    return call(bridge?.QuickActionForProject?.bind(bridge), action, project);
  },
  async getLogsForService(project, service) {
    return call(bridge?.GetLogsForService?.bind(bridge), project, service);
  },
  async startLogStreamForService(project, service) {
    return call(
      bridge?.StartLogStreamForService?.bind(bridge),
      project,
      service,
    );
  },
  async stopLogStream() {
    return call(bridge?.StopLogStream?.bind(bridge));
  },
  async startTerminal(project, service, user, shell) {
    return call(
      bridge?.StartTerminal?.bind(bridge),
      project,
      service,
      user,
      shell,
    );
  },
  async writeTerminal(id, data) {
    return call(bridge?.WriteTerminal?.bind(bridge), id, data);
  },
  async resizeTerminal(id, cols, rows) {
    return call(bridge?.ResizeTerminal?.bind(bridge), id, cols, rows);
  },
  async openShellForService(project, service, user, shell) {
    return call(
      bridge?.OpenShellForService?.bind(bridge),
      project,
      service,
      user,
      shell,
    );
  },
  async getShellUser(project) {
    return call(bridge?.GetShellUser?.bind(bridge), project);
  },
  async setShellUser(project, user) {
    return call(bridge?.SetShellUser?.bind(bridge), project, user);
  },
  async resetShellUsers() {
    return call(bridge?.ResetShellUsers?.bind(bridge));
  },
  async getSettings() {
    return call(bridge?.GetSettings?.bind(bridge));
  },
  async getMailpitURL() {
    return call(bridge?.GetMailpitURL?.bind(bridge));
  },
  async updateSettings(theme, proxyTarget, preferredBrowser, codeEditor) {
    return call(
      bridge?.UpdateSettings?.bind(bridge),
      theme,
      proxyTarget,
      preferredBrowser,
      codeEditor,
    );
  },
  async resetSettings() {
    return call(bridge?.ResetSettings?.bind(bridge));
  },
};
